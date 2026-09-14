// Prueba de carga (docs/design/colas-virtuales.md §11) para validar la
// premisa completa de las colas de acceso virtual: 5 000 usuarios pidiendo
// ticket en 10s contra una sala de ritmo=50/s, verificando:
//   (a) el endpoint protegido (acceso.iniciar_sesion) nunca admite más de
//       ~50 peticiones/segundo — lo hace cumplir el middleware, no el
//       propio caso de uso de login;
//   (b) la cola drena en ~100s (5000/50) sin ningún 5xx del sistema
//       protegido;
//   (c) el p99 del endpoint de INGRESO (POST .../tickets, el camino
//       caliente de INV-COLA-08: solo Redis + CPU, nunca Postgres) se
//       mantiene en milisegundos sin importar la posición del que pide el
//       ticket — a diferencia de (a), que sí depende de cuánta gente hay
//       delante en la cola.
//
// Precondición (fuera de este script, ver test/carga/abrir_sala_carga.sql):
// una sala de alcance "sistema" abierta sobre acceso.iniciar_sesion, alias
// "carga-k6-login", ritmo=50/s, capacidad=6000, ventana de reclamo=60s. Se
// abre por SQL directo, no por HTTP — el catálogo cerrado de rutas
// (acceso.iniciar_sesion, identidad.registrar_usuario,
// tenencia.aceptar_invitacion) solo admite alcance "sistema"
// (RutaProtegida.AdmiteAlcance), y el único endpoint de administración de
// salas por HTTP solo abre salas org-scoped (ADR 0045: no hay rol de
// administrador de plataforma en este producto).
//
// Cómo correrla, de punta a punta:
//   1. make docker-up && make migrate-up
//   2. REDIS_URL=redis://localhost:6379/0 make run   (otra terminal)
//   3. docker exec -i auth-service-postgres redis-cli FLUSHALL
//      (limpia cualquier cursor/cola de una corrida anterior: el estado en
//      Redis se indexa por ClaveSala="sistema:acceso.iniciar_sesion", no
//      por el alias/id de la fila de salas_espera, así que sobrevive a
//      abrir/cerrar la sala)
//   4. docker exec -i auth-service-postgres psql -U auth_service -d auth_service \
//        < test/carga/abrir_sala_carga.sql
//   5. Esperar ~20s (ReconciliarSalas relee Postgres cada 15s; el "20s" no
//      es solo margen para ese ciclo, es EXACTAMENTE el offset que
//      abrir_sala_carga.sql le suma a reloj_desde — ver el comentario ahí:
//      esperar de más deja que el cursor de admisión, que corre en tiempo
//      real desde reloj_desde sin importar si alguien pidió ticket,
//      "preadmita" posiciones fantasma antes de que llegue el primer
//      ticket real, y toda la ráfaga entra de golpe en vez de a ~ritmo/s)
//   6. k6 run --local-ips=127.0.0.0/16 --out json=resultados_colas.json test/carga/colas_virtuales_test.js
//   7. bash test/carga/analizar_admision.sh resultados_colas.json
//   8. docker exec -i auth-service-postgres psql -U auth_service -d auth_service \
//        < test/carga/cerrar_sala_carga.sql
//
// --local-ips=127.0.0.0/16 es OBLIGATORIO, no un ajuste opcional de
// rendimiento: el propio ingreso a la sala (POST .../tickets) es una
// acción limitada por Confianza (ADR 0018, ingreso_a_sala: 20/min por IP,
// ver internal/confianza/dominio/umbral.go) para acotar el farming de
// tickets (INV-COLA-04). Corriendo 5000 peticiones desde una sola IP —lo
// que hace k6 por defecto, todas desde localhost— el propio guardián de
// perímetro las tumba con 429 casi de inmediato: el primer intento de esta
// prueba, sin --local-ips, midió esto en carne propia (20 admitidas, 4981
// con 429). --local-ips=127.0.0.0/16 hace que k6 asigne una IP de origen
// real y distinta por VU (una IP por VU durante toda su vida, no por
// petición — verificado empíricamente contando claves
// confianza:rl:ip:ingreso_a_sala:<ip> en Redis), así que cada IP hace como
// mucho un puñado de ingresos — todo el rango 127.0.0.0/8 es loopback en
// Linux, no hace falta ninguna configuración de red adicional.
//
// Por qué el endpoint protegido es acceso.iniciar_sesion y NO
// identidad.registrar_usuario, a pesar de que este último es el ejemplo
// que usa ADR 0045 ("apertura masiva de inscripciones"): el primer intento
// de esta prueba usó registro y tumbó el propio servidor. El registro pasa
// por PolíticaContrasena, que llama a la API real de HIBP
// (api.pwnedpasswords.com, ADR 0014, fail-open) por cada intento — con
// miles de registros concurrentes, esas llamadas salientes se acumulan
// esperando timeout (varios segundos cada una) y agotan los recursos del
// proceso antes de poder responder nada, incluida la propia ruta de
// ingreso a la sala (memoria/goroutines compartidas con el resto del
// proceso). No es un defecto de la cola: es una dependencia de red externa
// del endpoint protegido, ajena a lo que este test quiere medir.
// acceso.iniciar_sesion no tiene esa dependencia (valida Argon2id contra
// Postgres, nunca sale a la red) y es, además, el caso canónico que la
// cabecera de docs/design/colas-virtuales.md usa como ejemplo motivador.
// Cada VU usa un correo propio (no uno fijo) para no tropezar con el
// límite de cuenta de login (ADR 0018, 5/15min) antes de tiempo: eso deja
// que la mayoría de los intentos ejerciten de verdad el camino de
// autenticación (incluido el costo real de Argon2id contra una cuenta
// inexistente, INV-ID-11/ADR 0013: el tiempo de respuesta no debe revelar
// si la cuenta existe), y solo 401 (credenciales inválidas, la cuenta no
// existe) — nunca 5xx.
import http from 'k6/http';
import { check, sleep } from 'k6';
import { Trend } from 'k6/metrics';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const ALIAS_SALA = __ENV.ALIAS_SALA || 'carga-k6-login';

// ingreso_latencia mide EXCLUSIVAMENTE el POST .../tickets (el "endpoint de
// ingreso" del criterio (c) de §11) — nunca el de registro, cuyo tiempo de
// respuesta sí depende de Argon2id/Postgres/HIBP y no tiene por qué ser de
// milisegundos.
const ingresoLatencia = new Trend('ingreso_latencia', true);

// Umbral deliberadamente generoso para hardware de desarrollo/CI
// compartido, no una SLA de producción: lo que importa no es el número
// exacto sino que NO escale con la posición en la cola (position 1 y
// position 5000 deben tardar lo mismo, porque el camino caliente nunca
// consulta el estado de nadie más — INV-COLA-08).
export const options = {
  scenarios: {
    ingreso_masivo: {
      executor: 'constant-arrival-rate',
      rate: 500,
      timeUnit: '1s',
      duration: '10s',
      // Cada iteración sigue viva (sondeando su turno) hasta ~100s después
      // de pedir el ticket: casi las 5000 sesiones están en vuelo a la vez
      // cerca del final de la ventana de ingreso, así que preAllocatedVUs/
      // maxVUs tienen que cubrir esa concurrencia, no solo las 500/s de
      // llegada.
      preAllocatedVUs: 1000,
      maxVUs: 5500,
      exec: 'flujoCompleto',
    },
  },
  thresholds: {
    ingreso_latencia: ['p(99)<300'],
  },
  // 5000 iteraciones, algunas con esperas largas: subir el timeout global
  // por si el runner de k6 corre en una máquina lenta.
  setupTimeout: '30s',
  teardownTimeout: '30s',
};

// esperaMaximaSegundos acota cuánto sondea una sola iteración antes de
// darse por vencida: ~100s de drenaje esperado + margen. Sin este tope, un
// bug real en el drenaje (la cola nunca avanza) colgaría el test entero en
// vez de fallarlo con un check claro.
const esperaMaximaSegundos = 150;

export function flujoCompleto() {
  const resIngreso = http.post(
    `${BASE_URL}/confianza/salas-espera/${ALIAS_SALA}/tickets`,
    null,
    { tags: { name: 'colas_ingreso' } }
  );
  ingresoLatencia.add(resIngreso.timings.duration);

  const ingresoOk = check(resIngreso, {
    'ingreso: 201': (r) => r.status === 201,
    'ingreso: trae ticket': (r) => !!(r.json() && r.json().ticket),
  });
  if (!ingresoOk) {
    return;
  }

  const ticket = resIngreso.json().ticket;
  let desenlace = resIngreso.json().desenlace;
  let reconsultarEnMs = resIngreso.json().reconsultar_en_ms || 1000;

  const desde = Date.now();
  while (desenlace !== 'admitido' && (Date.now() - desde) / 1000 < esperaMaximaSegundos) {
    sleep(reconsultarEnMs / 1000);
    const resTurno = http.get(`${BASE_URL}/confianza/salas-espera/${ALIAS_SALA}/turno`, {
      headers: { 'X-Ticket-Cola': ticket },
      tags: { name: 'colas_consultar_turno' },
    });
    if (resTurno.status !== 200) {
      check(resTurno, { 'turno: nunca 5xx': (r) => r.status < 500 });
      return;
    }
    const cuerpo = resTurno.json();
    desenlace = cuerpo.desenlace;
    reconsultarEnMs = cuerpo.reconsultar_en_ms || reconsultarEnMs;
  }

  check(null, { 'admitido antes del tope de espera': () => desenlace === 'admitido' });
  if (desenlace !== 'admitido') {
    return;
  }

  // Petición real al endpoint protegido, con el ticket ya admitido. Se
  // etiqueta con admision:"protegido_real" para que
  // analizar_admision.sh pueda aislar exactamente este tráfico del resto
  // (ingreso, consulta de turno) al contar peticiones/segundo. Correo
  // propio por VU (nunca uno fijo) y contraseña deliberadamente
  // incorrecta: no hace falta una cuenta real para medir "el middleware
  // deja pasar como máximo ~50/s", y la cuenta no existiendo mantiene el
  // 401 de INV-ID-11/ADR 0013 (mismo error genérico, mismo tiempo, sin
  // revelar que no existe) — nunca crea sesiones reales que después haya
  // que limpiar.
  const correo = `carga-k6-${__VU}-${__ITER}-${Date.now()}@ejemplo-carga.test`;
  const resLogin = http.post(
    `${BASE_URL}/acceso/sesiones`,
    JSON.stringify({ correo, contrasena: 'lo-que-sea-1234' }),
    {
      headers: { 'Content-Type': 'application/json', 'X-Ticket-Cola': ticket },
      tags: { name: 'colas_endpoint_protegido', admision: 'protegido_real' },
    }
  );
  check(resLogin, {
    // Nunca 5xx (criterio (b)) y nunca el 503 propio del guardián de sala
    // (si esto pasa, algo falló en el flujo de arriba: el ticket se dio
    // por admitido pero el middleware lo volvió a rechazar).
    'endpoint protegido: nunca 5xx': (r) => r.status < 500,
    'endpoint protegido: no rechazado por la sala (nunca 503 aquí)': (r) => r.status !== 503,
  });
}
