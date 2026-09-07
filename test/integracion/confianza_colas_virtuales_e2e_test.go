// Este archivo cubre la batería de tests de integración end-to-end que
// exige §11 paso 10 del diseño docs/design/colas-virtuales.md, contra
// Redis y Postgres REALES (no mocks): FIFO estricto bajo concurrencia
// (INV-COLA-14), reclamo concurrente con exactamente un ganador
// (INV-COLA-06), ETA que nunca empeora (INV-COLA-05), recuperación tras
// pérdida de estado en Redis (INV-COLA-13), modo degradado en sus dos
// valores (§8), un ticket de cola que nunca sirve como token de acceso
// (INV-COLA-03) y el endpoint público agregado que nunca revela la
// organización ni la ruta protegida (INV-COLA-15).
//
// Los tests de dominio (internal/confianza/dominio/*_test.go, sin mocks) y
// de aplicación (internal/confianza/aplicacion/*_test.go, con mocks) ya
// cubren las reglas de negocio y cada camino de error por separado; los de
// confianza_cola_redis_test.go ya cubren la consistencia dominio↔Lua y
// confianza_salas_espera_postgres_test.go ya cubren el adaptador Postgres.
// Este archivo se queda deliberadamente en la capa de aplicación (contra
// EstadoCola/RepositorioSalasDeEspera reales) para los escenarios de
// concurrencia y recuperación (más simple y determinístico que levantar
// HTTP para verificar una permutación de rangos), y sube a HTTP real
// (Fiber+Huma, mismo patrón que acceso_test.go/tenencia_test.go) solo para
// los tres casos que son inherentemente de borde HTTP: el middleware en
// modo degradado, el rechazo de un ticket como Bearer y el endpoint
// público agregado.
//
// La prueba de carga con k6 de §11 (5000 usuarios/10s) queda fuera de este
// archivo: k6 no está disponible en este entorno (documentado aparte).
//
// Todos los tests requieren DATABASE_URL_APLICACION Y REDIS_URL; se omiten
// limpiamente con t.Skip si no están definidas (mismo criterio que
// confianza_cola_redis_test.go).
package integracion

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humafiber"
	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	confianzaauditoria "github.com/r-david1/moterus/internal/confianza/adaptadores/auditoria"
	confianzacripto "github.com/r-david1/moterus/internal/confianza/adaptadores/cripto"
	confianzahttp "github.com/r-david1/moterus/internal/confianza/adaptadores/http"
	confianzapostgres "github.com/r-david1/moterus/internal/confianza/adaptadores/postgres"
	confianzaredis "github.com/r-david1/moterus/internal/confianza/adaptadores/redis"
	confianzaaplicacion "github.com/r-david1/moterus/internal/confianza/aplicacion"
	"github.com/r-david1/moterus/internal/confianza/dominio"
	confianzapuertos "github.com/r-david1/moterus/internal/confianza/puertos"

	"github.com/r-david1/moterus/internal/plataforma/cache"
	"github.com/r-david1/moterus/internal/plataforma/ids"
	"github.com/r-david1/moterus/internal/plataforma/reloj"
)

// --- entorno compartido -------------------------------------------------

// riesgoPermiteSiempreDePrueba es un puertos.EvaluadorDeRiesgo permisivo.
// Los tests de este archivo ejercitan decenas de ingresos concurrentes
// desde el mismo proceso de test (misma IP de origen), lo que chocaría con
// el umbral REAL de ingreso_a_sala (20/min por IP, §12 del diseño,
// dominio/umbral.go) si se usara el EvaluadorDeRiesgo real — eso ya lo
// cubren confianza_redis_test.go y los tests de aplicación con mocks; lo
// que este archivo verifica es el mecanismo de la cola en sí, no el
// limitador de tasa que la antecede.
type riesgoPermiteSiempreDePrueba struct{}

func (riesgoPermiteSiempreDePrueba) Evaluar(context.Context, confianzapuertos.Solicitud) (dominio.Decision, error) {
	return dominio.Decision{Permitido: true, Puntaje: 1}, nil
}

func (riesgoPermiteSiempreDePrueba) RegistrarResultado(context.Context, confianzapuertos.ResultadoIntento) error {
	return nil
}

// origenConfianzaDePrueba construye un confianza/dominio.OrigenSolicitud
// mínimo válido. Nombre distinto de origenDePrueba (entorno_test.go, que
// construye el homónimo de identidad/dominio): son VOs distintos a
// propósito (§0.3 y "cuarta duplicación deliberada" de origen_solicitud.go).
func origenConfianzaDePrueba(t *testing.T) dominio.OrigenSolicitud {
	t.Helper()
	origen, err := dominio.NuevoOrigenSolicitud("127.0.0.1", "go-test-integracion-colas-virtuales", "", "id-solicitud-prueba-colas")
	if err != nil {
		t.Fatalf("dominio.NuevoOrigenSolicitud: %v", err)
	}
	return origen
}

// entornoColaVirtualDePrueba agrupa los adaptadores reales (Postgres +
// Redis) y los casos de uso de aplicación necesarios para ejercitar el
// mecanismo de sala de espera de punta a punta sin HTTP, más el
// riesgoPermiteSiempreDePrueba en vez del EvaluadorDeRiesgo real (ver su
// comentario). Reconciliador/instantánea se comparten entre todos los
// tests de un mismo entorno, exactamente como en cmd/api/main.go.
type entornoColaVirtualDePrueba struct {
	pool          *pgxpool.Pool
	repo          *confianzapostgres.RepositorioSalasDeEspera
	estadoCola    *confianzaredis.EstadoCola
	instantanea   *confianzaaplicacion.InstantaneaSalasVigentes
	reconciliador *confianzaaplicacion.ReconciliarSalasCasoDeUso
	abrir         *confianzaaplicacion.AbrirSalaCasoDeUso
	cambiarEstado *confianzaaplicacion.CambiarEstadoSalaCasoDeUso
	portero       *confianzaaplicacion.PorteroDeSalaCasoDeUso
}

// nuevoEntornoColaVirtualDePrueba requiere DATABASE_URL_APLICACION (vía
// poolAplicacion, entorno_test.go) y REDIS_URL; se omite limpiamente si
// falta cualquiera de las dos (mismo criterio que
// confianza_cola_redis_test.go#estadoColaRedis).
func nuevoEntornoColaVirtualDePrueba(t *testing.T) *entornoColaVirtualDePrueba {
	t.Helper()
	pool := poolAplicacion(t)

	url := os.Getenv("REDIS_URL")
	if url == "" {
		t.Skip("REDIS_URL no está definido: se omiten los tests e2e de colas virtuales")
	}
	cliente, err := cache.NuevoClienteRedis(url)
	if err != nil {
		t.Fatalf("cache.NuevoClienteRedis: %v", err)
	}
	if err := cliente.Ping(context.Background()).Err(); err != nil {
		t.Fatalf("no se pudo conectar a Redis en %s: %v", url, err)
	}
	t.Cleanup(func() { _ = cliente.Close() })

	relojReal := reloj.NuevoReal()
	repo := confianzapostgres.NuevoRepositorioSalasDeEspera(pool)
	estadoCola := confianzaredis.NuevoEstadoCola(cliente)
	generadorIDs := confianzapostgres.NuevoGeneradorIDs()
	generadorTickets := confianzacripto.NuevoGeneradorTickets()
	registroAuditoria := confianzaauditoria.NuevoRegistroAuditoria(pool)
	uow := confianzapostgres.NuevaUnidadDeTrabajo(pool)

	instantanea := confianzaaplicacion.NuevaInstantaneaSalasVigentes()
	reconciliador := confianzaaplicacion.NuevoReconciliarSalasCasoDeUso(repo, estadoCola, relojReal, instantanea)
	abrirCasoDeUso := confianzaaplicacion.NuevoAbrirSalaCasoDeUso(repo, estadoCola, registroAuditoria, relojReal, generadorIDs, uow)
	cambiarEstadoCasoDeUso := confianzaaplicacion.NuevoCambiarEstadoSalaCasoDeUso(repo, estadoCola, registroAuditoria, relojReal, uow)
	portero := confianzaaplicacion.NuevoPorteroDeSalaCasoDeUso(instantanea, estadoCola, generadorTickets, relojReal, riesgoPermiteSiempreDePrueba{})

	return &entornoColaVirtualDePrueba{
		pool:          pool,
		repo:          repo,
		estadoCola:    estadoCola,
		instantanea:   instantanea,
		reconciliador: reconciliador,
		abrir:         abrirCasoDeUso,
		cambiarEstado: cambiarEstadoCasoDeUso,
		portero:       portero,
	}
}

// abrirSala abre una sala de alcance sistema (§7.2 del diseño: es el único
// alcance que admiten las tres rutas del catálogo cerrado) con
// AbrirSalaCasoDeUso —el mismo caso de uso que el subcomando de CLI de
// producción invocaría con IDSujeto vacío—, reconcilia inmediatamente para
// que la instantánea en memoria compartida vea la sala vigente (§3.7 del
// diseño, sin depender del Ticker de 15s de producción) y registra su
// cierre al finalizar el test (INV-COLA-01: a lo sumo una sala no cerrada
// por (alcance, ruta), así que sin este cleanup una corrida posterior sobre
// la misma ruta chocaría).
func (e *entornoColaVirtualDePrueba) abrirSala(t *testing.T, alias string, ruta dominio.RutaProtegida, ritmo int, capacidad int64, ventanaReclamo time.Duration, modoDegradado string) confianzapuertos.VistaSala {
	t.Helper()
	ctx := context.Background()
	origen := origenConfianzaDePrueba(t)

	vista, err := e.abrir.Abrir(ctx, confianzapuertos.ComandoAbrirSala{
		Alias:               alias,
		Ruta:                ruta.String(),
		IDOrganizacion:      "",
		RitmoAdmision:       ritmo,
		CapacidadMaximaCola: capacidad,
		VentanaReclamo:      ventanaReclamo,
		ModoDegradado:       modoDegradado,
		IDSujeto:            "",
		Origen:              origen,
	})
	if err != nil {
		t.Fatalf("AbrirSalaCasoDeUso.Abrir(%q, %q): %v", alias, ruta.String(), err)
	}

	if err := e.reconciliador.Reconciliar(ctx); err != nil {
		t.Fatalf("Reconciliar tras abrir %q: %v", alias, err)
	}

	t.Cleanup(func() {
		if _, err := e.cambiarEstado.CambiarEstado(context.Background(), confianzapuertos.ComandoCambiarEstadoSala{
			IDSala:         vista.ID,
			IDOrganizacion: "",
			Destino:        "cerrada",
			IDSujeto:       "",
			Origen:         origen,
		}); err != nil {
			t.Logf("no se pudo cerrar la sala de prueba %q al finalizar: %v", alias, err)
		}
	})

	return vista
}

// claveSistema reconstruye el valor textual de ClaveSala para alcance
// sistema ("sistema:<ruta>", ver dominio.nuevaClaveSala/AlcanceSala.Clave),
// necesario para invocar EstadoCola.Retirar y PorteroDeSala.Reclamar
// directamente (sin pasar por el middleware HTTP, que es quien normalmente
// la resuelve).
func claveSistema(ruta dominio.RutaProtegida) string {
	return "sistema:" + ruta.String()
}

// ticketDePruebaConFormatoValido genera un ticket sintácticamente válido
// (prefijo mot_cola_, entropía suficiente) SIN ingresarlo en ninguna sala:
// alcanza para ejercitar el middleware en el camino de "falla de
// infraestructura al reclamar", donde el desenlace no depende de que el
// ticket exista de verdad en Redis.
func ticketDePruebaConFormatoValido(t *testing.T) string {
	t.Helper()
	generador := confianzacripto.NuevoGeneradorTickets()
	plano, err := generador.GenerarTicket()
	if err != nil {
		t.Fatalf("GenerarTicket: %v", err)
	}
	return plano.Valor()
}

// =============================================================================
// 1. FIFO estricto bajo concurrencia (INV-COLA-14).
// =============================================================================

// TestColasVirtuales_FIFOEstrictoBajoConcurrencia ingresa N goroutines
// concurrentemente a la misma sala recién abierta y verifica que los
// rangos asignados (ResultadoTurno.LongitudCola, que en la respuesta de
// Ingresar es exactamente el valor que devolvió el INCR de Redis para ese
// ticket — ver el comentario de scriptIngresarLua en estado_cola.go: la
// tupla reusa el resultado de INCR en dos posiciones) son una permutación
// exacta de 1..N: sin repetidos y sin huecos. La garantía es estructural
// (INCR es atómico), no de aplicación; este test la ejercita contra Redis
// real bajo concurrencia real de Go, no simulada.
func TestColasVirtuales_FIFOEstrictoBajoConcurrencia(t *testing.T) {
	entorno := nuevoEntornoColaVirtualDePrueba(t)
	alias := unicoDePrueba(t, "fifo")
	// Ritmo mínimo posible (1/s): durante la ráfaga concurrente (que debe
	// completarse en milisegundos), el cursor derivado no debe avanzar, así
	// que la posición de cada ticket coincide con su rango. No es lo que
	// este test verifica (usa LongitudCola, no Posicion) pero evita
	// contaminar el resultado con desenlaces "admitido" a mitad de la ráfaga.
	entorno.abrirSala(t, alias, dominio.RutaAccesoIniciarSesion, 1, 100_000, 2*time.Minute, "permitir")
	origen := origenConfianzaDePrueba(t)

	const n = 50
	rangos := make([]int64, n)
	errs := make([]error, n)

	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			resultado, err := entorno.portero.Ingresar(context.Background(), confianzapuertos.ComandoIngresarASala{
				Alias:  alias,
				Origen: origen,
			})
			errs[i] = err
			rangos[i] = resultado.LongitudCola
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("Ingresar concurrente (goroutine %d): %v", i, err)
		}
	}

	vistos := make(map[int64]bool, n)
	for i, rango := range rangos {
		if rango < 1 || rango > n {
			t.Fatalf("rango fuera del rango esperado [1,%d]: %d (goroutine %d)", n, rango, i)
		}
		if vistos[rango] {
			t.Fatalf("rango %d asignado más de una vez: INCR no fue atómico (INV-COLA-14 violada)", rango)
		}
		vistos[rango] = true
	}
	if len(vistos) != n {
		t.Fatalf("se esperaban %d rangos únicos formando 1..%d, hubo %d únicos: %v", n, n, len(vistos), rangos)
	}
}

// =============================================================================
// 2. Reclamo concurrente: exactamente uno gana (INV-COLA-06).
// =============================================================================

// TestColasVirtuales_ReclamoConcurrente_ExactamenteUnoGana ingresa un
// ticket en una sala con ritmo altísimo (su turno se alcanza casi de
// inmediato) y lo reclama dos veces concurrentemente: exactamente una debe
// recibir "admitido" y la otra debe fallar con ErrTicketConsumido — nunca
// las dos "admitido", nunca las dos fallando. La atomicidad la da el
// script Lua `reclamar` (HSET de 'estado'→'consumido' en la misma
// operación que decide el desenlace); este test la ejercita contra Redis
// real bajo concurrencia real de Go.
func TestColasVirtuales_ReclamoConcurrente_ExactamenteUnoGana(t *testing.T) {
	entorno := nuevoEntornoColaVirtualDePrueba(t)
	alias := unicoDePrueba(t, "reclamo")
	ruta := dominio.RutaIdentidadRegistrarUsuario
	entorno.abrirSala(t, alias, ruta, 10000, 100_000, 2*time.Minute, "permitir")
	origen := origenConfianzaDePrueba(t)
	ctx := context.Background()

	ingreso, err := entorno.portero.Ingresar(ctx, confianzapuertos.ComandoIngresarASala{Alias: alias, Origen: origen})
	if err != nil {
		t.Fatalf("Ingresar: %v", err)
	}
	if ingreso.Desenlace != dominio.DesenlaceEsperando.String() {
		t.Fatalf("Ingresar: desenlace = %q, esperado %q", ingreso.Desenlace, dominio.DesenlaceEsperando.String())
	}

	// Con ritmo=10000/s, turnoDe(rango=1) está a menos de 1ms de la
	// apertura: este margen alcanza de sobra para que el turno ya se haya
	// alcanzado cuando lleguen los dos reclamos concurrentes.
	time.Sleep(50 * time.Millisecond)

	clave := claveSistema(ruta)
	const intentos = 2
	desenlaces := make([]string, intentos)
	errsReclamo := make([]error, intentos)

	var wg sync.WaitGroup
	wg.Add(intentos)
	for i := 0; i < intentos; i++ {
		go func(i int) {
			defer wg.Done()
			resultado, err := entorno.portero.Reclamar(context.Background(), confianzapuertos.ComandoReclamarTurno{
				Clave:       clave,
				TicketPlano: ingreso.TicketPlano,
			})
			desenlaces[i] = resultado.Desenlace
			errsReclamo[i] = err
		}(i)
	}
	wg.Wait()

	var admitidos, consumidos int
	for i := 0; i < intentos; i++ {
		switch {
		case errsReclamo[i] == nil && desenlaces[i] == dominio.DesenlaceAdmitido.String():
			admitidos++
		default:
			var errTicketConsumido *dominio.ErrTicketConsumido
			if errors.As(errsReclamo[i], &errTicketConsumido) {
				consumidos++
			} else {
				t.Errorf("reclamo %d: desenlace/err inesperado: desenlace=%q err=%v", i, desenlaces[i], errsReclamo[i])
			}
		}
	}
	if admitidos != 1 {
		t.Errorf("se esperaba exactamente 1 reclamo admitido, hubo %d (INV-COLA-06 violada)", admitidos)
	}
	if consumidos != 1 {
		t.Errorf("se esperaba exactamente 1 reclamo con ErrTicketConsumido, hubo %d", consumidos)
	}
}

// =============================================================================
// 3. ETA monótona (INV-COLA-05).
// =============================================================================

// TestColasVirtuales_ETANuncaEmpeora consulta repetidamente el turno de un
// mismo ticket mientras otros ingresan (y nunca reclaman: "abandonan")
// alrededor, y verifica que ni TurnoEstimadoEn cambie ni EsperaEstimada
// aumente entre consultas sucesivas — el turno es función determinista de
// (rango, cursorBase, relojDesde, ritmo), y ninguno de esos cuatro valores
// cambia por el mero hecho de que otros ingresen o abandonen (§1.5 y §4 del
// diseño: "los turnos no reclamados no se devuelven al cupo").
func TestColasVirtuales_ETANuncaEmpeora(t *testing.T) {
	entorno := nuevoEntornoColaVirtualDePrueba(t)
	alias := unicoDePrueba(t, "eta")
	// Ritmo bajo para que el ticket de referencia quede con una posición
	// lejana y no se admita durante las 10 iteraciones del test.
	entorno.abrirSala(t, alias, dominio.RutaTenenciaAceptarInvitacion, 2, 100_000, 2*time.Minute, "permitir")
	origen := origenConfianzaDePrueba(t)
	ctx := context.Background()

	referencia, err := entorno.portero.Ingresar(ctx, confianzapuertos.ComandoIngresarASala{Alias: alias, Origen: origen})
	if err != nil {
		t.Fatalf("Ingresar (ticket de referencia): %v", err)
	}

	var turnoAnterior time.Time
	var esperaAnterior time.Duration
	const iteraciones = 10
	for i := 0; i < iteraciones; i++ {
		// "Otros ingresan y abandonan alrededor": nunca se reclaman, así que
		// quedan como abandonados (§4 del diseño).
		if _, err := entorno.portero.Ingresar(ctx, confianzapuertos.ComandoIngresarASala{Alias: alias, Origen: origen}); err != nil {
			t.Fatalf("Ingresar (ruido, iteración %d): %v", i, err)
		}

		consulta, err := entorno.portero.ConsultarTurno(ctx, confianzapuertos.ConsultaTurno{
			Alias:       alias,
			TicketPlano: referencia.TicketPlano,
		})
		if err != nil {
			t.Fatalf("ConsultarTurno (iteración %d): %v", i, err)
		}

		if i == 0 {
			turnoAnterior = consulta.TurnoEstimadoEn
			esperaAnterior = consulta.EsperaEstimada
		} else {
			if !consulta.TurnoEstimadoEn.Equal(turnoAnterior) {
				t.Errorf("iteración %d: TurnoEstimadoEn cambió de %v a %v sin un cambio de ritmo (INV-COLA-05 violada)",
					i, turnoAnterior, consulta.TurnoEstimadoEn)
			}
			if consulta.EsperaEstimada > esperaAnterior {
				t.Errorf("iteración %d: EsperaEstimada empeoró: %v -> %v (INV-COLA-05 violada)",
					i, esperaAnterior, consulta.EsperaEstimada)
			}
			esperaAnterior = consulta.EsperaEstimada
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// =============================================================================
// 4. Recuperación tras pérdida de estado en Redis (INV-COLA-13).
// =============================================================================

// TestColasVirtuales_RecuperacionTrasPerdidaDeEstadoEnRedis simula un
// FLUSHALL/pérdida de estado a mitad de un evento (EstadoCola.Retirar sobre
// la clave de la sala: borra cfg+seq+tickets, exactamente las claves
// "confianza:cola:<clave>:*" de §6.2 del diseño) y verifica que, tras
// reconciliar de nuevo, el reloj de admisión arranca desde cero: un ticket
// que ingresa después de la recuperación NO queda admitido de golpe, pese a
// que —si el reconciliador reinstalara sin más el ancla real que sigue en
// Postgres (relojDesde=apertura, ya varios cientos de ms en el pasado, con
// un ritmo alto)— el cursor derivado de esa ancla ya estaría muy adelantado
// y admitiría inmediatamente al primer reingreso (exactamente el bug que
// INV-COLA-13 y el comentario de ReconciliarSalasCasoDeUso.Reconciliar
// documentan).
func TestColasVirtuales_RecuperacionTrasPerdidaDeEstadoEnRedis(t *testing.T) {
	entorno := nuevoEntornoColaVirtualDePrueba(t)
	alias := unicoDePrueba(t, "recupera")
	ruta := dominio.RutaAccesoIniciarSesion
	// Ritmo bajo (2/s) a propósito, y en tensión deliberada con el sleep de
	// 2s de abajo: si el reconciliador NO reiniciara el reloj de admisión
	// tras detectar la pérdida de estado, el cursor derivado del ancla
	// original (relojDesde = apertura) ya habría avanzado ~4 posiciones
	// durante ese sleep — de sobra para admitir de golpe a un ticket con
	// rango=1 (1 <= 4). Con el reloj correctamente reiniciado a cursorBase=0
	// justo antes del Ingresar de abajo, el cursor deriva desde CERO, así
	// que un ritmo bajo (2/s ⇒ 1 unidad cada 500ms) deja margen de sobra
	// para que la latencia local entre Reconciliar e Ingresar (microsegundos
	// a unos pocos milisegundos) no alcance a mover el cursor.
	entorno.abrirSala(t, alias, ruta, 2, 100_000, 2*time.Minute, "permitir")
	origen := origenConfianzaDePrueba(t)
	ctx := context.Background()
	clave := claveSistema(ruta)

	// Dejamos pasar tiempo real de pared para que la ventana de la
	// aritmética (ahora - relojDesde) sea grande si el reloj NO se
	// reiniciara.
	time.Sleep(2 * time.Second)

	if err := entorno.estadoCola.Retirar(ctx, clave); err != nil {
		t.Fatalf("Retirar (simulando pérdida de estado en Redis): %v", err)
	}

	if err := entorno.reconciliador.Reconciliar(ctx); err != nil {
		t.Fatalf("Reconciliar tras la pérdida de estado: %v", err)
	}

	resultado, err := entorno.portero.Ingresar(ctx, confianzapuertos.ComandoIngresarASala{Alias: alias, Origen: origen})
	if err != nil {
		t.Fatalf("Ingresar tras la recuperación: %v", err)
	}
	if resultado.LongitudCola != 1 {
		t.Fatalf("tras perder el estado, el contador de secuencia debía reiniciar en 1 (INCR sobre una clave nueva); rango obtenido = %d",
			resultado.LongitudCola)
	}
	// Nota: el script Lua `ingresar` siempre devuelve el desenlace literal
	// "esperando" (ver el comentario de scriptIngresarLua en
	// estado_cola.go): NO recalcula si el rango recién asignado ya está
	// admitido. La comprobación real de INV-COLA-13 —que el ticket no quede
	// admitido de golpe— es sobre Posicion, que sí deriva del cursor.
	if resultado.Desenlace != dominio.DesenlaceEsperando.String() {
		t.Fatalf("Ingresar: desenlace = %q, esperado %q", resultado.Desenlace, dominio.DesenlaceEsperando.String())
	}
	if resultado.Posicion <= 0 {
		t.Fatalf("posición = %d: un ticket con rango=1 quedó con cursor >= 1 inmediatamente después de reiniciar el reloj de "+
			"admisión (cursorBase=0, relojDesde=ahora) — si esto ocurre de forma reproducible, INV-COLA-13 está rota: el "+
			"reconciliador no reinició el reloj tras detectar la pérdida de estado, y un reingreso quedaría admitido de golpe",
			resultado.Posicion)
	}
}

// =============================================================================
// 5. Modo degradado, los dos valores (§8 del diseño).
// =============================================================================

// estadoColaFallaAlReclamarDePrueba envuelve un puertos.EstadoDeCola real
// (para Proyectar/Instantanea, que el reconciliador necesita para poblar la
// instantánea en memoria de verdad) y fuerza un error de infraestructura
// SOLO en Reclamar — exactamente la operación que MiddlewareSalaDeEspera
// invoca en el paso 3 de §7.3 del diseño. Es la opción que el encargo
// sugiere como más determinística frente a apagar Redis de verdad.
type estadoColaFallaAlReclamarDePrueba struct {
	confianzapuertos.EstadoDeCola
}

func (estadoColaFallaAlReclamarDePrueba) Reclamar(context.Context, string, string) (confianzapuertos.EstadoTicket, error) {
	return confianzapuertos.EstadoTicket{}, errors.New("confianza/redis: fallo simulado de infraestructura (prueba de modo degradado, §8 del diseño)")
}

// entradaEndpointProtegidoDePrueba/salidaEndpointProtegidoDePrueba son el
// input/output Huma mínimo de un endpoint de prueba cualquiera, protegido
// EXCLUSIVAMENTE por MiddlewareSalaDeEspera: lo único que este test
// necesita observar es si "la petición real" (el handler) se ejecuta o no.
type entradaEndpointProtegidoDePrueba struct{}

type salidaEndpointProtegidoDePrueba struct {
	Body struct {
		OK bool `json:"ok"`
	}
}

// nuevoServidorMiddlewareSalaDePrueba levanta un *fiber.App real con Huma
// v2 (mismo motor que el resto del repositorio, ADR 0006) y un único
// endpoint POST protegido por MiddlewareSalaDeEspera(api, portero, ruta) —
// nada más: es el harness mínimo para observar de punta a punta si la
// petición real pasa (200) o si el middleware la bloquea (503), sin acoplar
// el test a ninguno de los tres contextos consumidores reales.
func nuevoServidorMiddlewareSalaDePrueba(t *testing.T, portero confianzapuertos.PorteroDeSala, ruta dominio.RutaProtegida) *fiber.App {
	t.Helper()
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	api := humafiber.NewV2(app, huma.DefaultConfig("prueba-sala-espera", "0.0.1"))

	huma.Register(api, huma.Operation{
		OperationID:   "prueba-endpoint-protegido",
		Method:        http.MethodPost,
		Path:          "/prueba/protegida",
		DefaultStatus: http.StatusOK,
		Middlewares:   huma.Middlewares{confianzahttp.MiddlewareSalaDeEspera(api, portero, ruta)},
	}, func(_ context.Context, _ *entradaEndpointProtegidoDePrueba) (*salidaEndpointProtegidoDePrueba, error) {
		var out salidaEndpointProtegidoDePrueba
		out.Body.OK = true
		return &out, nil
	})

	return app
}

// TestColasVirtuales_ModoDegradado_PermitirDejaPasarYRechazarBloquea cubre
// los dos valores de ModoDegradado ante una falla real de EstadoDeCola al
// reclamar (§8 del diseño): con modoDegradado=permitir, la petición real
// (el handler) debe ejecutarse pese a la falla; con modoDegradado=rechazar,
// debe bloquearse con 503 y desenlace "sala_no_disponible".
func TestColasVirtuales_ModoDegradado_PermitirDejaPasarYRechazarBloquea(t *testing.T) {
	entorno := nuevoEntornoColaVirtualDePrueba(t)

	t.Run("permitir_deja_pasar_la_peticion_real", func(t *testing.T) {
		ruta := dominio.RutaIdentidadRegistrarUsuario
		alias := unicoDePrueba(t, "degradado-permitir")
		entorno.abrirSala(t, alias, ruta, 50, 100_000, 2*time.Minute, dominio.ModoDegradadoPermitir.String())

		porteroConFalla := confianzaaplicacion.NuevoPorteroDeSalaCasoDeUso(
			entorno.instantanea,
			estadoColaFallaAlReclamarDePrueba{entorno.estadoCola},
			confianzacripto.NuevoGeneradorTickets(),
			reloj.NuevoReal(),
			riesgoPermiteSiempreDePrueba{},
		)
		app := nuevoServidorMiddlewareSalaDePrueba(t, porteroConFalla, ruta)

		req := httptest.NewRequest(http.MethodPost, "/prueba/protegida", nil)
		req.Header.Set("X-Ticket-Cola", ticketDePruebaConFormatoValido(t))
		resp, err := app.Test(req, 5000)
		if err != nil {
			t.Fatalf("app.Test: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			cuerpo, _ := io.ReadAll(resp.Body)
			t.Fatalf("modoDegradado=permitir: status = %d, esperado 200 (la petición real debe ejecutarse pese a la falla de EstadoDeCola); cuerpo: %s",
				resp.StatusCode, cuerpo)
		}
	})

	t.Run("rechazar_bloquea_con_503_sala_no_disponible", func(t *testing.T) {
		ruta := dominio.RutaTenenciaAceptarInvitacion
		alias := unicoDePrueba(t, "degradado-rechazar")
		entorno.abrirSala(t, alias, ruta, 50, 100_000, 2*time.Minute, dominio.ModoDegradadoRechazar.String())

		porteroConFalla := confianzaaplicacion.NuevoPorteroDeSalaCasoDeUso(
			entorno.instantanea,
			estadoColaFallaAlReclamarDePrueba{entorno.estadoCola},
			confianzacripto.NuevoGeneradorTickets(),
			reloj.NuevoReal(),
			riesgoPermiteSiempreDePrueba{},
		)
		app := nuevoServidorMiddlewareSalaDePrueba(t, porteroConFalla, ruta)

		req := httptest.NewRequest(http.MethodPost, "/prueba/protegida", nil)
		req.Header.Set("X-Ticket-Cola", ticketDePruebaConFormatoValido(t))
		resp, err := app.Test(req, 5000)
		if err != nil {
			t.Fatalf("app.Test: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusServiceUnavailable {
			cuerpo, _ := io.ReadAll(resp.Body)
			t.Fatalf("modoDegradado=rechazar: status = %d, esperado 503; cuerpo: %s", resp.StatusCode, cuerpo)
		}
		var cuerpo struct {
			Desenlace string `json:"desenlace"`
		}
		crudo, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("leyendo el cuerpo: %v", err)
		}
		if err := json.Unmarshal(crudo, &cuerpo); err != nil {
			t.Fatalf("decodificando el cuerpo: %v\ncuerpo: %s", err, crudo)
		}
		if cuerpo.Desenlace != "sala_no_disponible" {
			t.Fatalf("desenlace = %q, esperado \"sala_no_disponible\" (§8 del diseño)", cuerpo.Desenlace)
		}
	})
}

// =============================================================================
// 6. Un ticket de cola nunca sirve como token de acceso (INV-COLA-03).
// =============================================================================

// TestColasVirtuales_TicketDeColaNuncaSirveComoTokenDeAcceso presenta un
// TicketPlano sintácticamente válido como si fuera un Bearer token contra
// un endpoint protegido real y ya existente (GET /identidad/usuarios/{id},
// protegido por middlewareAutenticacionAcceso/ValidadorDeAccesos) y
// confirma que se rechaza con 401. La defensa es estructural (§1.4 del
// diseño): el ticket no comparte formato, llave ni validador con un JWT de
// acceso, así que ni siquiera hace falta un chequeo de `typ` — este test
// confirma que esa propiedad se sostiene de punta a punta, no solo en el
// papel.
func TestColasVirtuales_TicketDeColaNuncaSirveComoTokenDeAcceso(t *testing.T) {
	pool := poolAplicacion(t)
	app := nuevoServidorAcceso(t, pool).App

	ticket := ticketDePruebaConFormatoValido(t)

	idCualquiera, err := ids.GenerarUUIDv7()
	if err != nil {
		t.Fatalf("GenerarUUIDv7: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/identidad/usuarios/"+idCualquiera, nil)
	req.Header.Set("Authorization", "Bearer "+ticket)

	status := respuestaHTTP(t, app, req, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("GET /identidad/usuarios/{id} con un ticket de cola como Bearer: status = %d, esperado 401 "+
			"(INV-COLA-03: un ticket de cola nunca debe aceptarse donde se espera un token de acceso)", status)
	}
}

// =============================================================================
// 7. El endpoint público agregado nunca revela la organización ni la ruta
//    (INV-COLA-15).
// =============================================================================

// TestColasVirtuales_EndpointPublicoAgregadoNuncaRevelaOrganizacionNiRuta
// abre una sala real, levanta el servidor HTTP propio de Confianza
// (confianzahttp.RegistrarRutas) y verifica que GET
// /confianza/salas-espera/{alias} no incluye ningún campo con el nombre de
// la organización o de la ruta protegida, ni el valor de la ruta en
// ninguna parte del cuerpo — la propiedad se verifica sobre el JSON crudo
// de la respuesta, no solo sobre el DTO tipado (que ya la garantiza por
// construcción, respuestaSalaPublica en dtos.go): este test confirma que
// eso también es cierto de punta a punta.
func TestColasVirtuales_EndpointPublicoAgregadoNuncaRevelaOrganizacionNiRuta(t *testing.T) {
	entorno := nuevoEntornoColaVirtualDePrueba(t)
	alias := unicoDePrueba(t, "publica")
	ruta := dominio.RutaAccesoIniciarSesion
	entorno.abrirSala(t, alias, ruta, 50, 100_000, 2*time.Minute, dominio.ModoDegradadoPermitir.String())

	consultor := confianzaaplicacion.NuevoConsultarSalaCasoDeUso(entorno.instantanea, entorno.estadoCola)
	manejador := confianzahttp.NuevoManejadorConfianza(entorno.portero, nil, consultor)
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	confianzahttp.RegistrarRutas(app, manejador, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/confianza/salas-espera/"+alias, nil)
	resp, err := app.Test(req, 5000)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	crudo, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("leyendo el cuerpo: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /confianza/salas-espera/{alias}: status = %d, esperado 200; cuerpo: %s", resp.StatusCode, crudo)
	}

	var campos map[string]any
	if err := json.Unmarshal(crudo, &campos); err != nil {
		t.Fatalf("decodificando el cuerpo: %v\ncuerpo: %s", err, crudo)
	}

	prohibidos := []string{"organizacion", "organizacion_id", "id_organizacion", "ruta", "ruta_protegida", "clave", "alcance", "alcance_tipo"}
	for campo := range campos {
		for _, p := range prohibidos {
			if strings.EqualFold(campo, p) {
				t.Errorf("INV-COLA-15: el endpoint público reveló el campo %q; cuerpo: %s", campo, crudo)
			}
		}
	}
	if strings.Contains(string(crudo), ruta.String()) {
		t.Errorf("INV-COLA-15: el cuerpo del endpoint público contiene el nombre de la ruta protegida (%q); cuerpo: %s", ruta.String(), crudo)
	}
}
