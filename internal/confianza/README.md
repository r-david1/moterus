# Bounded Context: Confianza

> Este README es la puerta de entrada para cualquier equipo (frontend u otro
> servicio backend) que necesite **consumir** este contexto hoy. Confianza
> tiene tres features independientes, cerradas en momentos distintos:
> rate limiting/captcha (ADR 0018) — invisible para un integrador, corre
> detrás de los endpoints de Identidad/Acceso/Tenencia —, colas de acceso
> virtual / salas de espera, que sí expone endpoints HTTP propios, y
> reconocimiento de origen (fingerprinting/comportamiento), también
> invisible, que corre solo en el login. Para el diseño completo de la
> segunda, ver `docs/design/colas-virtuales.md` — su cabecera ya está
> actualizada a "implementada"; para el de la tercera, ver
> `docs/design/fingerprinting-comportamiento.md` — misma cabecera
> "implementada". Este README es la referencia operativa que las
> complementa, no las reemplaza. Para las decisiones de arquitectura no
> obvias citadas aquí, ver
> `docs/adr/0018-rate-limiting-captcha-confianza-redis.md`,
> `docs/adr/0041-*.md` a `docs/adr/0046-*.md` (colas) y
> `docs/adr/0047-*.md` a `docs/adr/0051-*.md` (reconocimiento de origen).

## Responsabilidad

Confianza responde una sola pregunta: **¿confío en este request, y si no,
qué hago con él?** Todo lo que decide "dejar pasar / frenar / diferir" un
request en el perímetro del sistema, antes de que llegue al dominio de
negocio de otro contexto.

Hoy cubre tres mecanismos, deliberadamente relacionados pero distintos (ver
la tabla comparativa en `docs/design/colas-virtuales.md` §0.2 y su
extensión en `docs/design/fingerprinting-comportamiento.md` §0.2):

| | Rate limiting + captcha (ADR 0018) | Colas de acceso virtual / salas de espera (`docs/design/colas-virtuales.md`) | Reconocimiento de origen (`docs/design/fingerprinting-comportamiento.md`) |
|---|---|---|---|
| Protege a | un recurso concreto contra **un cliente** (IP, cuenta) | al **sistema entero** contra la demanda agregada | una **cuenta** contra el uso desde un origen que nunca usó |
| Se activa por | comportamiento **anómalo** de un origen | volumen **legítimo** que supera la capacidad configurada | un intento de **login** desde una huella/red que la cuenta no tiene registrada |
| Desenlace | rechazo (`429`), con culpa implícita | espera con turno, posición y ETA — nadie es rechazado, se difiere | fricción: un captcha resoluble en el mismo intento (y solo si el modo no es `observar`) |
| Corre | siempre, en todos los endpoints con hook | solo cuando una sala está abierta (opt-in) | solo en `login`, y por defecto solo observa/audita, sin cambiar el desenlace |
| Estado | contadores por clave en Redis, TTL de minutos | agregado `SalaDeEspera` en Postgres (config) + cola FIFO en Redis (estado efímero) | HASH por cuenta en Redis (índice derivado, TTL de 180 días) — ninguna tabla nueva |
| Expone HTTP propio | no — es un puerto (`EvaluadorDeRiesgo`) que consumen otros contextos | sí — `/confianza/salas-espera/*` | no — ningún endpoint nuevo (§7 del diseño) |

Confianza **no** hace (y por diseño no debe hacerse aquí):

- Verificar quién es el sujeto o si puede probarlo — **Identidad**
  (ADR 0009).
- Emitir o validar tokens/sesiones — **Acceso**.
- Decidir si un sujeto puede ejecutar una acción en una organización —
  **Tenencia**. Confianza consume `tenencia/puertos.VerificadorDeAutorizacion`
  **solo desde su propio ACL** (`confianza/adaptadores/tenencia/`), único
  paquete de Confianza autorizado a importar `tenencia/puertos` — misma
  regla de frontera que INV-TEN-28.
- Persistir la bitácora forense — **Auditoría** (mismo mecanismo que los
  demás contextos: tabla `auditoria`, hash-chaining, ADR 0005), vía el ACL
  `confianza/adaptadores/auditoria/`. Confianza es el contexto que llegó
  **último** a tener bitácora propia — antes de las salas de espera,
  `confianza/adaptadores/postgres/doc.go` y `.../http/doc.go` decían
  literalmente "Pendiente de implementación".

## Estructura de carpetas

```
internal/confianza/
├── dominio/              # Umbral/PoliticaLimites/Decision/EvaluarPuntajeCaptcha (ADR 0018)
│                         # + SalaDeEspera, TicketDeCola, AlcanceSala, RutaProtegida,
│                         #   PoliticaSala, EstadoSala, DesenlaceDeAdmision, eventos, errores
│                         # + ClaveCuenta/HashHuella/HashRed/HuellaDeOrigen, SenalRiesgo,
│                         #   PuntajeRiesgo/NivelRiesgo, PerfilDeOrigen, PoliticaRiesgo/ModoRiesgo,
│                         #   OrigenNuevoObservado (reconocimiento de origen)
├── aplicacion/           # EvaluarTrustSignalCasoDeUso (ADR 0018)
│                         # + AbrirSala/CambiarRitmoDeAdmision/CambiarEstadoSala/
│                         #   PorteroDeSala/ConsultarSala/ReconciliarSalas
│                         # + paso 2.5 y promoción dentro de EvaluarTrustSignalCasoDeUso, y
│                         #   OlvidarPerfilDeOrigen (reconocimiento de origen)
├── puertos/
│   ├── entrada.go        # EvaluadorDeRiesgo, PorteroDeSala, GestorDeSalasDeEspera, ConsultorDeSalas
│   ├── salida.go         # LimitadorTasa, VerificadorCaptcha, RepositorioSalasDeEspera, EstadoDeCola,
│                         #   GeneradorTickets, GeneradorIDs, Reloj, RegistroAuditoria, UnidadDeTrabajo,
│                         #   VerificadorDeAutorizacion, PerfilDeOrigenes
│   └── mocks/
└── adaptadores/
    ├── redis/            # limitador_tasa.go (ADR 0018) + estado_cola.go (3 scripts Lua, colas)
    │                     # + perfil_origenes.go (HMGET + 1 script Lua, reconocimiento de origen)
    ├── turnstile/        # verificador_captcha.go (Cloudflare Turnstile, ADR 0018)
    ├── postgres/         # repositorio_salas_espera.go, unidad_trabajo.go, generador_ids.go, sqlc/
    ├── cripto/           # generador_tickets.go (crypto/rand + prefijo mot_cola_ + SHA-256)
    ├── tenencia/         # ACL: VerificadorAutorizacion (único paquete que importa tenencia/puertos)
    ├── auditoria/        # ACL: RegistroAuditoria (implementa puertos.RegistroAuditoria);
    │                     #   mapeo.go incluye el case OrigenNuevoObservado (origen.nuevo)
    ├── porteronoop/      # PorteroDeSala no-op para cuando REDIS_URL no está configurado
    └── http/             # middlewares (origen, autenticación, autorización, sala de espera),
                          # handlers.go, dtos.go, rutas.go, errores_http.go
```

**Reconocimiento de origen no agrega ningún puerto de entrada ni ninguna
carpeta de adaptador nueva** (§2.1/§5.1 de `docs/design/fingerprinting-comportamiento.md`):
reutiliza `EvaluadorDeRiesgo` (los DTOs `Solicitud`/`ResultadoIntento`
ganan campos aditivos) y el mismo ACL de auditoría que las colas
virtuales ya tenían.

## Cómo se prueba

```bash
make docker-up     # levanta Postgres y Redis
make migrate-up     # aplica las migraciones, incluidas 000017 (tabla salas_espera),
                     # 000018 (catálogo de auditoría de colas) y 000019 (catálogo de
                     # auditoría de reconocimiento de origen — sin tabla ni GRANT propios)
make run             # arranca el servidor en :8080 — monta Identidad, Acceso, Tenencia y Confianza
```

Tests unitarios (`dominio`/`aplicacion`, sin Redis ni Postgres reales, el
cursor y el turno se prueban pasando `ahora` como parámetro):

```bash
go test ./internal/confianza/...
```

Confianza **no tiene modo aislado**: sin `REDIS_URL` configurado, el
limitador de tasa/captcha, el motor de colas de acceso virtual y el
reconocimiento de origen arrancan todos en modo no-op
(`EvaluadorConfianzaNoOp` en cada contexto consumidor,
`porteronoop.PorteroDeSala` aquí, `EvaluarTrustSignalCasoDeUso` ni
siquiera se monta), con un `WARN` explícito en el arranque — nunca de
forma silenciosa. Con `REDIS_URL` configurado pero sin
`TURNSTILE_SECRET_KEY`, el rate limiting real funciona igual; solo el
captcha queda deshabilitado (ver ADR 0018). El reconocimiento de origen no
depende de Turnstile para observar/auditar; solo lo necesita si algún día
se pasa a `modo = exigir_captcha` (ver INV-RIES-14 más abajo).

Log de arranque real observado (con Redis configurado):

```
api: motor de Confianza real montado (rate limiting por IP y por cuenta vía Redis + captcha Cloudflare Turnstile)
api: reconocimiento de origen activo (modo observar por defecto — no cambia el desenlace de ningún login, solo lo audita/loguea)
api: motor de colas de acceso virtual (Confianza) montado (postgres+redis, reconciliador cada 15s)
api: contexto Confianza (colas de acceso virtual) montado: rutas propias + middleware en acceso.iniciar_sesion/identidad.registrar_usuario/tenencia.aceptar_invitacion
```

Sin `REDIS_URL`:

```
api: reconocimiento de origen desactivado — REDIS_URL no está definido, EvaluarTrustSignalCasoDeUso ni siquiera se monta
api: colas de acceso virtual en modo no-op (REDIS_URL no configurado) — Acceso/Identidad/Tenencia montan el middleware, pero nunca hay sala vigente; sin rutas propias de Confianza
```

**Verificado en vivo con `curl` contra un servidor real** (Postgres+Redis
reales, sin mocks): abrir una sala sobre `acceso.iniciar_sesion` → pegarle
a `POST /acceso/sesiones` sin ticket devuelve `503` con
`desenlace: ticket_requerido` → `POST /confianza/salas-espera/{alias}/tickets`
devuelve un ticket con turno → tras el turno, `POST /acceso/sesiones` con
la cabecera `X-Ticket-Cola` **admite el login real** (Argon2id, Postgres,
JWT emitido) → cerrar la sala (`PATCH .../salas-espera/{id}` con
`estado: cerrada`) → `POST /acceso/sesiones` vuelve a su comportamiento
normal sin necesitar ticket.

## Puertos que expone a otros contextos, y los que consume

**Expone:**

| Puerto | Consumidor real hoy | Para qué |
|---|---|---|
| `puertos.EvaluadorDeRiesgo` (`aplicacion.EvaluarTrustSignalCasoDeUso`) | **Identidad** (vía `identidad/adaptadores/confianza`), **Acceso** (vía `acceso/adaptadores/confianza`), **Tenencia** (vía `tenencia/adaptadores/confianza`), y el propio `PorteroDeSala` de las colas (acción `ingreso_a_sala`) | Rate limiting por IP/cuenta + captcha invisible antes de tocar Postgres (ADR 0018); único freno contra el farming de tickets de cola. Desde el reconocimiento de origen, **el mismo puerto** también evalúa/registra la huella de dispositivo en `Accion == login` (§3.1/§3.4 de `docs/design/fingerprinting-comportamiento.md`) — no agrega ningún puerto de entrada nuevo (INV-RIES-01/§2.1) |
| `puertos.PorteroDeSala` | `confianza/adaptadores/http.MiddlewareSalaDeEspera`, montado en `acceso.iniciar_sesion`, `identidad.registrar_usuario` y `tenencia.aceptar_invitacion` | Camino caliente: `SalaVigentePara` (sin E/S), `Ingresar`, `ConsultarTurno`, `Reclamar` — nunca toca Postgres (INV-COLA-08) |
| `puertos.GestorDeSalasDeEspera` / `puertos.ConsultorDeSalas` | los propios endpoints HTTP de administración de Confianza | Abrir/cambiar ritmo/cambiar estado de una sala org-scoped, y su consulta pública |

`puertos.PerfilDeOrigenes` (salida, reconocimiento de origen) **no se
expone a otros contextos**: lo consume únicamente
`EvaluarTrustSignalCasoDeUso`, dentro del propio Confianza, contra el
adaptador `confianza/adaptadores/redis/perfil_origenes.go` (`HMGET` + un
script Lua). Se documenta acá porque es el único puerto de salida nuevo de
esta tercera extensión — ver §2.2 del diseño.

**Consume:**

| Contexto | Puerto/mecanismo | Vía | Para qué |
|---|---|---|---|
| Tenencia | `puertos.VerificadorDeAutorizacion` | ACL `confianza/adaptadores/tenencia/` | Autorizar los endpoints de administración de salas org-scoped (`organizacion.editar`, reutilizado — ADR 0046) |
| Auditoría | registro forense | ACL `confianza/adaptadores/auditoria/` | Bitácora de apertura/cambio de ritmo/cierre de una sala (nunca de ingresos, consultas ni reclamos — INV-COLA-11); también la única fila de auditoría del reconocimiento de origen (`origen.nuevo`, best-effort, fuera de transacción — INV-RIES-13) |

`Solicitud.TenantID` en `confianza/puertos.EvaluadorDeRiesgo` es poblado
por el middleware de autorización de Tenencia con el `{idOrganizacion}` ya
autorizado de la ruta — Confianza no consume un puerto de Tenencia
directamente para esto, solo recibe el valor que Tenencia ya resolvió (ver
`internal/tenencia/README.md`). El reconocimiento de origen no consume
ningún puerto de Tenencia: no evalúa nada por organización (INV-RIES-15,
solo `login`).

Los tres ACL de Identidad/Acceso/Tenencia hacia
`confianza/puertos.EvaluadorDeRiesgo` (`identidad|acceso|tenencia/adaptadores/confianza`)
pueblan, desde el cierre de esta extensión, los tres campos aditivos de
`Solicitud`/`ResultadoIntento` — `HuellaDispositivo`, `IDUsuario`,
`IDSolicitud` — con datos que ya recibían y antes descartaban al traducir.
**Ninguno de los tres mapea hacia el otro lado** `PuntajeRiesgo`,
`NivelRiesgo` ni `SenalesDeRiesgo`: esos campos de `Decision` nunca cruzan
la frontera de contexto (INV-RIES-09), a diferencia de `Decision.Puntaje`
(captcha), que sí llega al cliente vía `PuntajeConfianza`.

## Endpoints HTTP

Rate limiting/captcha (ADR 0018) **no tiene endpoints propios**: es
invisible, corre dentro de los casos de uso de Identidad/Acceso/Tenencia a
través de `EvaluadorDeRiesgo`. El reconocimiento de origen **tampoco
tiene ningún endpoint propio** — ni de consulta ni de administración
(§7 de `docs/design/fingerprinting-comportamiento.md`): lo único
observable desde afuera es un `429` con `riesgo_de_origen_requiere_captcha`
en el cuerpo RFC 9457 que Identidad ya produce, y solo si el modo se pasó
de `observar` a `exigir_captcha` (§3.3). Los únicos endpoints HTTP de
Confianza son los de colas de acceso virtual, prefijo `/confianza`,
registrados **solo si `REDIS_URL` está configurado** (`RegistrarRutas` en
`cmd/api/main.go`).

### Públicos (sin autenticación — anteriores a cualquier sesión, por definición)

| Método | Ruta | Auth | Permiso | Rate limit especial | Éxito | Errores |
|---|---|---|---|---|---|---|
| `POST` | `/confianza/salas-espera/{aliasSala}/tickets` | ninguna | ninguno | ADR 0018, `ingreso_a_sala` — 20/min por IP, sin límite por cuenta (pre-autenticación) | **201** + ticket + turno | `404` sala inexistente/cerrada, `429` denegado por Confianza, `503` `cola_llena` |
| `GET` | `/confianza/salas-espera/{aliasSala}/turno` | cabecera `X-Ticket-Cola` | ninguno | ninguno | **200** con el desenlace en el cuerpo (nunca error HTTP: un ticket de cola no protege ningún secreto) | `404` sala inexistente |
| `GET` | `/confianza/salas-espera/{aliasSala}` | ninguna | ninguno | ninguno (cacheable, `Cache-Control: public, max-age=5`) | **200** + estado agregado | `404` |

### De administración (org-scoped; las salas de alcance `sistema` no tienen endpoint HTTP — se abren por operaciones/CLI, ver §7.2 del diseño)

| Método | Ruta | Auth | Permiso (Tenencia) | Éxito | Errores |
|---|---|---|---|---|---|
| `POST` | `/confianza/organizaciones/{idOrganizacion}/salas-espera` | Bearer | `organizacion.editar` | **201** + `VistaSala` | `401`, `403`, `404`, `409` alias/ruta ya con sala vigente, `422` |
| `PATCH` | `/confianza/organizaciones/{idOrganizacion}/salas-espera/{idSala}` | Bearer | `organizacion.editar` | **200** + `VistaSala` | `401`, `403`, `404`, `409` transición de estado inválida, `422` |

> **Nota de discrepancia diseño vs. implementación**: `docs/design/colas-virtuales.md`
> §7.2 lista también `GET /confianza/organizaciones/{idOrganizacion}/salas-espera`
> (listado, permiso `organizacion.ver`). Ese endpoint **no está
> implementado** — `rutas.go` no lo registra. Además, el `PATCH` de arriba
> no separa "cambiar ritmo" de "cambiar estado" en dos rutas: un único
> endpoint despacha a `CambiarRitmo` y/o `CambiarEstado` según qué campo
> del cuerpo venga presente (`RitmoAdmision`/`Estado`, al menos uno
> obligatorio). El código es la fuente de verdad; si tu integración
> necesita listar las salas de una organización, ese endpoint no existe
> todavía.

El ticket viaja en la cabecera `X-Ticket-Cola`, nunca en la ruta ni en la
query (un secreto en el path termina en logs de acceso y `Referer`). El
código de bloqueo del middleware es siempre `503` (no `429`: el servidor
difiere la petición, no imputa abuso al cliente — ver `docs/design/colas-virtuales.md`
§7.4), con `Retry-After` y un campo `desenlace` que discrimina
`ticket_requerido` / `esperando` / `turno_caducado` / `ticket_desconocido`
/ `ticket_consumido` / `cola_llena` / `sala_cerrada`.

## Invariantes de negocio clave

La lista completa (INV-COLA-01 a INV-COLA-15) está en
`docs/design/colas-virtuales.md` §4. Las que un integrador necesita
conocer sin leer el diseño completo:

- **INV-COLA-02** — un ticket de cola **no autoriza nada**. Una petición
  admitida por la sala atraviesa exactamente los mismos controles
  (autenticación, Confianza/rate limiting, autorización de Tenencia,
  reglas del caso de uso) que atravesaría sin sala. La sala solo puede
  **retrasar**, nunca relajar un control.
- **INV-COLA-03** — un ticket de cola nunca puede aceptarse donde se
  espera un token de acceso, de refresco, de step-up o de invitación, ni
  viceversa. La defensa es **estructural**: el ticket es opaco
  (`mot_cola_...`), sin firma, y lo valida un componente que no conoce la
  llave de Acceso ni el formato JWT.
- **INV-COLA-04** — el sistema protegido nunca recibe más de
  `ritmoAdmision` admisiones por segundo. Los turnos no reclamados **no**
  se devuelven al cupo: el error se comete siempre por debajo de la
  capacidad, nunca por encima.
- **INV-COLA-05** — la espera estimada (`turno_estimado_en`) de un ticket
  **nunca empeora** entre dos consultas, salvo por una reducción explícita
  y auditada del ritmo. Un cliente que sondea el mismo ticket no debería
  ver crecer su ETA.
- **INV-COLA-06** — un ticket se reclama **exactamente una vez**;
  `consumido` es terminal.
- **INV-COLA-07** — el ticket en claro nunca se persiste (Redis guarda su
  SHA-256), nunca se loguea, nunca viaja en auditoría. Sale del proceso
  **una sola vez**: en la respuesta de ingreso.
- **INV-COLA-08** — ni el ingreso, ni la consulta de turno, ni el reclamo
  tocan Postgres. El camino caliente es exclusivamente Redis + CPU local.
- **INV-COLA-09** — orden en la cadena de middlewares: en rutas de alcance
  `sistema` (las tres del catálogo cerrado), la sala se evalúa **antes**
  de cualquier trabajo caro; en una hipotética ruta org-scoped, sería
  **después** de autenticación y autorización.
- **INV-COLA-11** — toda mutación del ciclo de vida de una sala se audita
  (abrir, cambiar ritmo, drenar, cerrar); ingresos, consultas y reclamos
  **no** se auditan (mismo razonamiento que INV-TEN-25 de Tenencia).
- **INV-COLA-15** — ningún endpoint de la sala revela la organización
  dueña, la ruta protegida ni la existencia de otras salas. El endpoint
  público agregado publica longitud aproximada, estado y ETA; nunca
  identidades ni claves.

La lista completa (INV-RIES-01 a INV-RIES-15) del reconocimiento de
origen está en `docs/design/fingerprinting-comportamiento.md` §4. Las que
un integrador necesita conocer sin leer el diseño completo:

- **INV-RIES-01** — el reconocimiento de origen **nunca relaja ni
  sustituye** un control existente, y **nunca produce un rechazo que el
  usuario no pueda superar en el mismo intento**: su único desenlace
  posible es exigir un captcha. Rate limiting, captcha, sala de espera y
  las reglas del caso de uso siguen corriendo exactamente igual.
- **INV-RIES-02** — este mecanismo **no produce `Decision.RequiereStepUp`**
  en el MVP: hoy, un usuario sin factor MFA confirmado que lo recibiera
  quedaría bloqueado sin salida (`VerificarOTP` devuelve `false` con cero
  factores). Habilitar riesgo → step-up exige antes cerrar ese hueco con
  su propio ADR (ver §3.2 del diseño y ADR 0051).
- **INV-RIES-03** — **fail-open incondicional y no conmutable**: cualquier
  error del puerto `PerfilDeOrigenes` (Redis caído, timeout, respuesta
  corrupta) produce cero señales y `NivelRiesgo = normal`, nunca fricción
  agregada a un login legítimo. A diferencia de la sala de espera
  (ADR 0044), acá no hay modo degradado configurable — no existe el
  escenario donde valga la pena.
- **INV-RIES-04** — una cuenta con menos de `minimoExitosParaJuzgar`
  (3 por defecto) logins exitosos registrados produce **cero señales**: el
  primer login de una cuenta nueva nunca es sospechoso.
- **INV-RIES-05** — un origen se promueve a "conocido" **exclusivamente
  tras una autenticación exitosa**, nunca durante la evaluación previa:
  evita que un atacante "caliente" su propio dispositivo con intentos
  fallidos y desactive la señal que lo detectaría.
- **INV-RIES-06** — la huella de dispositivo es **dato del cliente y por
  lo tanto falsificable**: alimenta un puntaje, nunca autoriza, nunca
  identifica y nunca sustituye una credencial.
- **INV-RIES-09** — ni `PuntajeRiesgo`, ni `NivelRiesgo`, ni
  `SenalesDeRiesgo` **cruzan la frontera de contexto**: ningún ACL
  (Identidad/Acceso/Tenencia) los mapea hacia su `DecisionConfianza`, así
  que nunca pueden llegar a una respuesta HTTP — a diferencia de
  `Decision.Puntaje` (captcha), que sí se publica.
- **INV-RIES-10** — el camino caliente añade **como máximo una operación
  de Redis** (`HMGET` de cuatro campos) y **cero** consultas a Postgres.
  Nunca se consulta `auditoria` para decidir.
- **INV-RIES-13** — se audita **un solo hecho**, `origen.nuevo`, una vez
  por par (cuenta, dispositivo) y solo si la cuenta ya tenía al menos un
  origen conocido. No se auditan las evaluaciones ni las denegaciones por
  riesgo (viajan como `Motivo` dentro de `usuario.login`/`denegado`, ya
  fijado por ADR 0018).
- **INV-RIES-14** — el modo por defecto es **`observar`**: desplegar esta
  extensión **no cambia el desenlace de ningún login**. Pasar a
  `exigir_captcha` es una decisión operativa explícita, y en producción
  exige `TURNSTILE_SECRET_KEY` configurada (sin ella el captcha es
  fail-closed, ADR 0018, y la exigencia se volvería un bloqueo duro).
- **INV-RIES-15** — la evaluación de riesgo de origen se aplica **solo a
  `AccionLogin`**. Registro no tiene historial de cuenta por definición, y
  las acciones sensibles post-login ya tienen su propio freno en
  `PoliticaLimitesPorDefecto` (ADR 0018).

## Caída de Redis: fail-open por defecto, conmutable solo por sala

Mismo criterio que ADR 0018 (rate limiting fail-open, captcha fail-closed),
extendido a dos casos más. Ante una caída de Redis, el middleware de sala
de espera deja pasar por defecto (`modoDegradado: permitir`), con
`slog.Error` y contador de métrica dedicado. Un operador que abre una sala
para un evento crítico puede conmutar `modoDegradado: rechazar` al abrirla
o en caliente con `PATCH`, sin desplegar — en ese modo, una caída de Redis
produce `503`. El modo se decide **sin leer Redis**: viene de la
instantánea en memoria que el reconciliador (`ReconciliarSalas`, cada 15s)
mantiene a partir de Postgres. Ver `docs/design/colas-virtuales.md` §8 y
ADR 0044.

El reconocimiento de origen, en cambio, es **fail-open y NO conmutable**
(INV-RIES-03): una caída de Redis produce cero señales para todo login,
sin ningún interruptor operativo que lo cambie a fail-closed — a
diferencia de la sala, acá el mecanismo no protege capacidad, y agregar
fricción a un login legítimo por un fallo de caché nunca es aceptable. Ver
`docs/design/fingerprinting-comportamiento.md` §3.3.

## Auditoría

Se auditan las tres mutaciones del ciclo de vida de una sala
(`sala_espera.abierta`, `sala_espera.ritmo_cambiado`,
`sala_espera.cerrada`) y, del reconocimiento de origen, el único hecho
`origen.nuevo` (una vez por par cuenta-dispositivo, best-effort, fuera de
transacción — INV-RIES-13). Catálogo completo:
`docs/catalogos/acciones-auditoria.md`, sección "Contexto Confianza"
(migraciones `db/migraciones/000018_acciones_auditoria_confianza.up.sql`
y `db/migraciones/000019_acciones_auditoria_riesgo.up.sql`, esta última
sin tabla ni `GRANT` propios — no crea nada nuevo que privilegiar). Confianza
es el contexto que más tardó en tener bitácora propia — la primera fila de
auditoría de este contexto es de la extensión de colas, no de rate
limiting/captcha (esos eventos no se auditan, se miden por métricas y
`slog`, igual que los ingresos/consultas/reclamos de una sala y las
evaluaciones/denegaciones del reconocimiento de origen).

## ADRs relevantes

- ADR 0018 (rate limiting + captcha, Redis en vez de Postgres):
  `docs/adr/0018-rate-limiting-captcha-confianza-redis.md`
- ADR 0041 (colas de acceso virtual como extensión de Confianza, no un
  contexto nuevo): `docs/adr/0041-colas-virtuales-extension-de-confianza.md`
- ADR 0042 (ticket opaco, no JWT): `docs/adr/0042-ticket-cola-token-opaco-vs-jwt.md`
- ADR 0043 (admisión por cursor derivado del reloj, no ZSET+worker):
  `docs/adr/0043-admision-cursor-reloj-vs-zset-worker.md`
- ADR 0044 (caída de Redis: fail-open por defecto, conmutable por sala):
  `docs/adr/0044-sala-espera-caida-redis-modo-degradado.md`
- ADR 0045 (alcance `sistema` vs `organizacion`; salas sistémicas operadas
  fuera de la API): `docs/adr/0045-alcance-sala-sistema-vs-organizacion.md`
- ADR 0046 (reutiliza `organizacion.editar`, sin permiso nuevo):
  `docs/adr/0046-sala-espera-reutiliza-permiso-organizacion-editar.md`
- ADR 0047 (reconocimiento de origen como tercera extensión de Confianza,
  no un contexto nuevo ni parte de Acceso):
  `docs/adr/0047-reconocimiento-origen-extension-de-confianza.md`
- ADR 0048 (fingerprint pasivo del lado servidor, sin FingerprintJS ni
  proveedor de terceros): `docs/adr/0048-fingerprint-pasivo-servidor-sin-terceros.md`
- ADR 0049 (el riesgo es un input más a `Decision`, sin cruzar la
  frontera de contexto): `docs/adr/0049-riesgo-input-a-decision-sin-cruzar-frontera.md`
- ADR 0050 (sin tabla ni worker: índice derivado en Redis, evidencia ya en
  `auditoria`): `docs/adr/0050-sin-tabla-ni-worker-indice-redis-derivado.md`
- ADR 0051 (la escalada es solo captcha, nunca step-up, en el MVP):
  `docs/adr/0051-escalada-solo-captcha-nunca-step-up.md`
- Índice completo de ADRs: `docs/adr/README.md`

## Referencias

- Diseño de colas de acceso virtual: `docs/design/colas-virtuales.md`
- Diseño de reconocimiento de origen: `docs/design/fingerprinting-comportamiento.md`
- README de Tenencia (consumidor de `TenantID`, expone
  `VerificadorDeAutorizacion` que este contexto consume por ACL):
  `internal/tenencia/README.md`
- README de Identidad (consumidor de `EvaluadorDeRiesgo` en
  registro/login/reenvío; el ACL hacia Confianza puebla los campos de
  reconocimiento de origen desde el login): `internal/identidad/README.md`
- README de Acceso (consumidor de `EvaluadorDeRiesgo`; middleware de sala
  de espera montado en `POST /acceso/sesiones`): `internal/acceso/README.md`
