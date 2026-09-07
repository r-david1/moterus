# Bounded Context: Confianza

> Este README es la puerta de entrada para cualquier equipo (frontend u otro
> servicio backend) que necesite **consumir** este contexto hoy. Confianza
> tiene dos features independientes, cerradas en momentos distintos:
> rate limiting/captcha (ADR 0018) — invisible para un integrador, corre
> detrás de los endpoints de Identidad/Acceso/Tenencia — y colas de acceso
> virtual / salas de espera, que sí expone endpoints HTTP propios. Para el
> diseño completo de la segunda, ver `docs/design/colas-virtuales.md` — su
> cabecera ya está actualizada a "implementada"; este README es la
> referencia operativa que la complementa, no la reemplaza. Para las
> decisiones de arquitectura no obvias citadas aquí, ver
> `docs/adr/0018-rate-limiting-captcha-confianza-redis.md` y
> `docs/adr/0041-*.md` a `docs/adr/0046-*.md`.

## Responsabilidad

Confianza responde una sola pregunta: **¿confío en este request, y si no,
qué hago con él?** Todo lo que decide "dejar pasar / frenar / diferir" un
request en el perímetro del sistema, antes de que llegue al dominio de
negocio de otro contexto.

Hoy cubre dos mecanismos, deliberadamente relacionados pero distintos (ver
la tabla comparativa en `docs/design/colas-virtuales.md` §0.2):

| | Rate limiting + captcha (ADR 0018) | Colas de acceso virtual / salas de espera (`docs/design/colas-virtuales.md`) |
|---|---|---|
| Protege a | un recurso concreto contra **un cliente** (IP, cuenta) | al **sistema entero** contra la demanda agregada |
| Se activa por | comportamiento **anómalo** de un origen | volumen **legítimo** que supera la capacidad configurada |
| Desenlace | rechazo (`429`), con culpa implícita | espera con turno, posición y ETA — nadie es rechazado, se difiere |
| Corre | siempre, en todos los endpoints con hook | solo cuando una sala está abierta (opt-in) |
| Estado | contadores por clave en Redis, TTL de minutos | agregado `SalaDeEspera` en Postgres (config) + cola FIFO en Redis (estado efímero) |
| Expone HTTP propio | no — es un puerto (`EvaluadorDeRiesgo`) que consumen otros contextos | sí — `/confianza/salas-espera/*` |

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
├── aplicacion/           # EvaluarTrustSignalCasoDeUso (ADR 0018)
│                         # + AbrirSala/CambiarRitmoDeAdmision/CambiarEstadoSala/
│                         #   PorteroDeSala/ConsultarSala/ReconciliarSalas
├── puertos/
│   ├── entrada.go        # EvaluadorDeRiesgo, PorteroDeSala, GestorDeSalasDeEspera, ConsultorDeSalas
│   ├── salida.go         # LimitadorTasa, VerificadorCaptcha, RepositorioSalasDeEspera, EstadoDeCola,
│                         #   GeneradorTickets, GeneradorIDs, Reloj, RegistroAuditoria, UnidadDeTrabajo,
│                         #   VerificadorDeAutorizacion
│   └── mocks/
└── adaptadores/
    ├── redis/            # limitador_tasa.go (ADR 0018) + estado_cola.go (3 scripts Lua, colas)
    ├── turnstile/        # verificador_captcha.go (Cloudflare Turnstile, ADR 0018)
    ├── postgres/         # repositorio_salas_espera.go, unidad_trabajo.go, generador_ids.go, sqlc/
    ├── cripto/           # generador_tickets.go (crypto/rand + prefijo mot_cola_ + SHA-256)
    ├── tenencia/         # ACL: VerificadorAutorizacion (único paquete que importa tenencia/puertos)
    ├── auditoria/        # ACL: RegistroAuditoria (implementa puertos.RegistroAuditoria)
    ├── porteronoop/      # PorteroDeSala no-op para cuando REDIS_URL no está configurado
    └── http/             # middlewares (origen, autenticación, autorización, sala de espera),
                          # handlers.go, dtos.go, rutas.go, errores_http.go
```

## Cómo se prueba

```bash
make docker-up     # levanta Postgres y Redis
make migrate-up     # aplica las migraciones, incluidas 000017 (tabla salas_espera) y
                     # 000018 (catálogo de auditoría de Confianza)
make run             # arranca el servidor en :8080 — monta Identidad, Acceso, Tenencia y Confianza
```

Tests unitarios (`dominio`/`aplicacion`, sin Redis ni Postgres reales, el
cursor y el turno se prueban pasando `ahora` como parámetro):

```bash
go test ./internal/confianza/...
```

Confianza **no tiene modo aislado**: sin `REDIS_URL` configurado, tanto el
limitador de tasa/captcha como el motor de colas de acceso virtual arrancan
en modo no-op (`EvaluadorConfianzaNoOp` en cada contexto consumidor,
`porteronoop.PorteroDeSala` aquí), con un `WARN` explícito en el arranque —
nunca de forma silenciosa. Con `REDIS_URL` configurado pero sin
`TURNSTILE_SECRET_KEY`, el rate limiting real funciona igual; solo el
captcha queda deshabilitado (ver ADR 0018).

Log de arranque real observado (con Redis configurado):

```
api: motor de Confianza real montado (rate limiting por IP y por cuenta vía Redis + captcha Cloudflare Turnstile)
api: motor de colas de acceso virtual (Confianza) montado (postgres+redis, reconciliador cada 15s)
api: contexto Confianza (colas de acceso virtual) montado: rutas propias + middleware en acceso.iniciar_sesion/identidad.registrar_usuario/tenencia.aceptar_invitacion
```

Sin `REDIS_URL`:

```
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
| `puertos.EvaluadorDeRiesgo` (`aplicacion.EvaluarTrustSignalCasoDeUso`) | **Identidad** (vía `identidad/adaptadores/confianza`), **Acceso** (vía `acceso/adaptadores/confianza`), **Tenencia** (vía `tenencia/adaptadores/confianza`), y el propio `PorteroDeSala` de las colas (acción `ingreso_a_sala`) | Rate limiting por IP/cuenta + captcha invisible antes de tocar Postgres (ADR 0018); único freno contra el farming de tickets de cola |
| `puertos.PorteroDeSala` | `confianza/adaptadores/http.MiddlewareSalaDeEspera`, montado en `acceso.iniciar_sesion`, `identidad.registrar_usuario` y `tenencia.aceptar_invitacion` | Camino caliente: `SalaVigentePara` (sin E/S), `Ingresar`, `ConsultarTurno`, `Reclamar` — nunca toca Postgres (INV-COLA-08) |
| `puertos.GestorDeSalasDeEspera` / `puertos.ConsultorDeSalas` | los propios endpoints HTTP de administración de Confianza | Abrir/cambiar ritmo/cambiar estado de una sala org-scoped, y su consulta pública |

**Consume:**

| Contexto | Puerto/mecanismo | Vía | Para qué |
|---|---|---|---|
| Tenencia | `puertos.VerificadorDeAutorizacion` | ACL `confianza/adaptadores/tenencia/` | Autorizar los endpoints de administración de salas org-scoped (`organizacion.editar`, reutilizado — ADR 0046) |
| Auditoría | registro forense | ACL `confianza/adaptadores/auditoria/` | Bitácora de apertura/cambio de ritmo/cierre de una sala (nunca de ingresos, consultas ni reclamos — INV-COLA-11) |

`Solicitud.TenantID` en `confianza/puertos.EvaluadorDeRiesgo` es poblado
por el middleware de autorización de Tenencia con el `{idOrganizacion}` ya
autorizado de la ruta — Confianza no consume un puerto de Tenencia
directamente para esto, solo recibe el valor que Tenencia ya resolvió (ver
`internal/tenencia/README.md`).

## Endpoints HTTP

Rate limiting/captcha (ADR 0018) **no tiene endpoints propios**: es
invisible, corre dentro de los casos de uso de Identidad/Acceso/Tenencia a
través de `EvaluadorDeRiesgo`. Los únicos endpoints HTTP de Confianza son
los de colas de acceso virtual, prefijo `/confianza`, registrados **solo
si `REDIS_URL` está configurado** (`RegistrarRutas` en `cmd/api/main.go`).

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

## Caída de Redis: fail-open por defecto, conmutable por sala

Mismo criterio que ADR 0018 (rate limiting fail-open, captcha fail-closed),
extendido a un tercer caso: ante una caída de Redis, el middleware de sala
de espera deja pasar por defecto (`modoDegradado: permitir`), con
`slog.Error` y contador de métrica dedicado. Un operador que abre una sala
para un evento crítico puede conmutar `modoDegradado: rechazar` al abrirla
o en caliente con `PATCH`, sin desplegar — en ese modo, una caída de Redis
produce `503`. El modo se decide **sin leer Redis**: viene de la
instantánea en memoria que el reconciliador (`ReconciliarSalas`, cada 15s)
mantiene a partir de Postgres. Ver `docs/design/colas-virtuales.md` §8 y
ADR 0044.

## Auditoría

Se auditan únicamente las tres mutaciones del ciclo de vida de una sala:
`sala_espera.abierta`, `sala_espera.ritmo_cambiado`, `sala_espera.cerrada`.
Catálogo completo: `docs/catalogos/acciones-auditoria.md`, sección
"Contexto Confianza" (migración
`db/migraciones/000018_acciones_auditoria_confianza.up.sql`). Confianza es
el contexto que más tardó en tener bitácora propia — la primera fila de
auditoría de este contexto es de esta extensión, no de rate
limiting/captcha (esos eventos no se auditan, se miden por métricas y
`slog`, igual que los ingresos/consultas/reclamos de una sala).

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
- Índice completo de ADRs: `docs/adr/README.md`

## Referencias

- Diseño de colas de acceso virtual: `docs/design/colas-virtuales.md`
- README de Tenencia (consumidor de `TenantID`, expone
  `VerificadorDeAutorizacion` que este contexto consume por ACL):
  `internal/tenencia/README.md`
- README de Identidad (consumidor de `EvaluadorDeRiesgo` en registro/login/reenvío):
  `internal/identidad/README.md`
- README de Acceso (consumidor de `EvaluadorDeRiesgo`; middleware de sala
  de espera montado en `POST /acceso/sesiones`): `internal/acceso/README.md`
