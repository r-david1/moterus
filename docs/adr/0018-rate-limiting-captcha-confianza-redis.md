# ADR 0018 — Rate limiting y captcha invisible viven en Confianza, respaldados por Redis (no Postgres)

## Contexto

El encargo de seguridad perimetral (agente `seguridad-perimetral`) pedía cerrar dos huecos explícitamente documentados como conocidos desde el cierre del MVP de Identidad:

1. `puertos.EvaluadorConfianza` estaba montado como `EvaluadorConfianzaNoOp` (`internal/identidad/adaptadores/confianza/evaluador_confianza.go`): el puerto se invocaba (INV-ID-12 se cumplía formalmente) pero sin ningún efecto real — Identidad quedaba sin defensa contra credential stuffing, fuerza bruta ni alta masiva de cuentas.
2. `docs/design/identidad-bounded-context.md` (sección 0) ya asignaba "rate limiting, captcha, fingerprinting, score de riesgo" al contexto **Confianza**, no a Identidad — pero `internal/confianza/` solo existía como esqueleto de carpetas (`dominio/doc.go`, `aplicacion/doc.go`, `puertos/doc.go`, `adaptadores/{http,postgres}/doc.go`), sin ningún código.

El encargo debía decidir dos cosas no obvias antes de escribir código:

- **Dónde vive la lógica**: ¿middleware de Fiber ciego que envuelve las rutas desde fuera, o el puerto `EvaluadorConfianza` que Identidad ya expone?
- **Qué motor de estado usar**: el repo solo tenía Postgres provisionado activamente en el flujo de Identidad; Redis ya estaba en `deployments/docker-compose.yml` (servicio `redis:7-alpine`, con `appendonly yes`) y `internal/plataforma/cache/doc.go` ya documentaba la intención ("cliente Redis compartido... usado por rate limiting, colas virtuales y cachés de lectura") — pero nada lo consumía todavía.

## Decisión 1 — Vive detrás de puertos, no como middleware ciego

Todo el rate limiting y la verificación de captcha se implementan como un caso de uso nuevo del bounded context **Confianza** (`internal/confianza/{dominio,aplicacion,puertos,adaptadores}`), consumido por Identidad de dos formas distintas según si el endpoint ya tenía el hook o no:

- **`POST /identidad/usuarios` (registro) y `POST /identidad/autenticaciones` (login)**: ya llamaban a `puertos.EvaluadorConfianza.Evaluar(...)` antes de tocar la base de datos (INV-ID-12). Se sustituyó `EvaluadorConfianzaNoOp` por `EvaluadorConfianzaReal` (`internal/identidad/adaptadores/confianza/evaluador_confianza_real.go`), un ACL que traduce `identidad/puertos.SolicitudEvaluacion` ↔ `confianza/puertos.Solicitud` y delega en `confianza/aplicacion.EvaluarTrustSignalCasoDeUso`. **Cero cambios en `identidad/aplicacion` para estos dos endpoints**: el hook ya existía, cambia solo qué implementación concreta se inyecta en `cmd/api/main.go`.
- **`POST /identidad/verificaciones-correo/reenvios` (reenvío)**: `ReenviarVerificacionCasoDeUso.Reenviar` (sección 3.4 del diseño de Identidad) **nunca llamaba** a `EvaluadorConfianza` — un hueco real del diseño original, no un descuido de esta implementación. Identidad está cerrada/verificada end-to-end y el encargo pide explícitamente no tocar su dominio/aplicación salvo necesidad estricta. Se decidió **no** agregar la llamada dentro de `aplicacion/reenviar_verificacion.go`; en su lugar, `identidad/adaptadores/http/handlers.go` (capa de adaptador, no aplicación) invoca directamente el mismo `puertos.EvaluadorConfianza` **antes** de llamar al caso de uso, con `Accion="reenvio_verificacion"`. Es el mismo motor (mismo Redis, mismos umbrales, mismo adaptador Turnstile) — el único cambio es el punto de invocación, no el mecanismo. `ManejadorIdentidad` recibe `confianza` como sexta dependencia inyectada (puede ser `nil`, en cuyo caso el guardián no hace nada — no rompe despliegues que todavía no configuren Redis).

Esta decisión implica que **INV-ID-22** ("`ReenviarVerificacion` responde siempre con el mismo resultado observable") se interpreta como una invariante sobre el **desenlace de negocio** (¿existe el correo? ¿ya está verificado?), no sobre si el perímetro deja pasar la solicitud. Un 429 por límite de tasa no correlaciona con la existencia de la cuenta (la clave de rate limit es el string normalizado del correo recibido, calculada igual exista o no la cuenta) — es equivalente a que un WAF bloquee la solicitud antes de que llegue al proceso.

Se descartó la alternativa de middleware Fiber genérico (montado en `cmd/api/main.go` o `plataforma/servidor/middleware`) envolviendo las rutas desde fuera: ese middleware no conocería qué acción de negocio protege sin volver a parsear el body y duplicar conocimiento de los DTOs, y el diseño de Identidad ya había dejado el hook (`EvaluadorConfianza`) precisamente para evitar ese acoplamiento ciego.

## Decisión 2 — Redis, no Postgres, para los contadores

**Redis** es el motor de los contadores de rate limiting (`internal/confianza/adaptadores/redis`, `go-redis/v9`), no Postgres. Razones:

- **Ya estaba provisionado y anticipado.** `deployments/docker-compose.yml` ya define el servicio `redis` desde el cierre del hito de infraestructura; `internal/plataforma/cache/doc.go` ya documentaba "cliente Redis compartido... usado por rate limiting" antes de que existiera código que lo usara. No es una pieza nueva en el stack: es completar una que ya estaba reservada.
- **Patrón de acceso.** Cada intento de login/registro/reenvío incrementa como mínimo dos contadores (IP + cuenta) con TTL corto (1-15 min) y se descartan solos. Es exactamente el caso de uso de Redis (`INCR`/`EXPIRE`/`PEXPIRE`, O(1), sin necesidad de durabilidad a largo plazo) y no el de Postgres, cuyo costo por escritura (WAL, índices, vacuum) es var órdenes de magnitud mayor para un dato que vive minutos y no necesita transacciones ACID con el resto del dominio.
- **No compite con el pool de conexiones de negocio.** Confianza se evalúa *antes* de tocar Postgres en login (INV-ID-12, motivo explícito en el diseño de Identidad: "evita que el credential stuffing consuma conexiones a Postgres"). Si el propio limitador viviera en Postgres, un ataque de fuerza bruta seguiría saturando el mismo pool que se intenta proteger — Redis es un motor separado, con su propio límite de recursos.
- **Atomicidad sin transacciones distribuidas.** El algoritmo (ver más abajo) necesita "incrementar, leer TTL y opcionalmente extender TTL" como una sola operación atómica bajo concurrencia. En Redis esto es un script Lua (`EVAL`) de una sola ida y vuelta; en Postgres exigiría una transacción con bloqueo de fila (`SELECT ... FOR UPDATE`) en la tabla más caliente del sistema en el peor momento posible (un ataque en curso).

**Alternativa considerada y descartada**: contador en Postgres (tabla `confianza_contadores` con `UPSERT ... ON CONFLICT DO UPDATE` + limpieza por cron/TTL simulado). Es más simple operacionalmente (una infraestructura menos que mantener) y hubiera sido consistente con "todo lo demás ya vive en Postgres", pero se descartó porque el propio objetivo de este mecanismo es **no** depender de la misma base de datos que un ataque intenta agotar, y porque Redis ya era la pieza prevista por el diseño (ver arriba) — introducirla ahora no es una desviación, es completar el plan original.

## Algoritmo del limitador (`internal/confianza/adaptadores/redis/limitador_tasa.go`)

Contador de **ventana fija** (fixed window: `INCR` + `PEXPIRE` en el primer incremento), no una ventana deslizante exacta (sliding log). Se documenta la diferencia porque es una simplificación consciente: una ventana fija puede, en el peor caso teórico, permitir hasta 2× el límite nominal en el borde de dos ventanas consecutivas (ej. 5 intentos a las 23:59:59 de una ventana + 5 más a las 00:00:00 de la siguiente). Se acepta ese margen porque:

- El umbral real (5-10/min, 3-5/15min) ya es agresivo — un factor 2× en el peor caso no cambia el orden de magnitud de protección.
- Sliding log exacto (`ZADD`/`ZRANGEBYSCORE` con timestamps) cuesta O(log n) por entrada y una entrada por intento, en vez de O(1) por clave — no se justifica el costo adicional para el volumen de este proyecto (ADR 0002: un solo producto).
- El **cooldown exponencial** (ver abajo) mitiga el caso patológico: un atacante que sigue insistiendo tras el límite ve crecer el TTL de bloqueo, no solo "esperar hasta el borde de la ventana".

Además del contador simple, el script Lua (`scriptPermitir`) implementa **cooldown exponencial**: cada solicitud adicional que sigue llegando *después* de superado el límite duplica el TTL de bloqueo restante (`ventana × 2^exceso`), con un tope (`backoffMaximoPorDefecto = 2h`) para que no se vuelva un baneo de facto irreversible. Esto es lo que el encargo pedía como "cooldown exponencial" para cuentas con intentos fallidos repetidos.

## Umbrales elegidos y por qué (`internal/confianza/dominio/umbral.go`)

Dos niveles simultáneos, por acción:

| Acción | Límite por IP | Límite por cuenta (correo normalizado) |
|---|---|---|
| `login` | 5 / 1 min | 5 / 15 min |
| `registro` | 10 / 1 min | 3 / 15 min |
| `reenvio_verificacion` | 5 / 1 min | 3 / 15 min |

- **`login`** es el más agresivo por IP (5/min, tal como pedía explícitamente el encargo: *"umbral agresivo en endpoints de login/OTP (ej. 5 intentos/min)"*): es el objetivo directo de fuerza bruta de contraseñas. El límite por cuenta (5/15min) es la defensa contra credential stuffing distribuido en múltiples IPs contra una sola cuenta — se evalúa incluso si la cuenta no existe (la clave es el string normalizado del correo recibido, no un `usuario_id` resuelto), igual criterio anti-enumeración que INV-ID-11.
- **`registro`** es más laxo por IP (10/min: una oficina/NAT compartido dando de alta varias cuentas legítimas en ráfaga es un escenario normal) pero más estricto por cuenta (3/15min): nadie necesita reintentar el registro del mismo correo más de un puñado de veces en 15 minutos, exista o no ya esa cuenta (mitiga probing de existencia, complementario al 409 explícito de `ErrCorreoYaRegistrado` que ya revela el conflicto por diseño, ADR candidato 0013).
- **`reenvio_verificacion`** usa los mismos umbrales que registro por el mismo motivo (spam de un endpoint que dispara un envío de correo real en producción, aunque hoy el notificador sea log-only).
- Una acción sin entrada explícita en la tabla (`PoliticaLimites.Para`) recibe un umbral fail-safe conservador (3/min IP, 3/15min cuenta) en vez de quedar sin protección — para que una acción nueva que alguien olvide registrar aquí no quede invisible al mecanismo.

Estos números **no** están calibrados contra tráfico de producción real (el proyecto no lo tiene todavía, ADR 0002): son un punto de partida razonable y documentado, ajustable sin migración (viven en código, no en base de datos) — a diferencia de la política de contraseñas (ADR candidato 0014), que si hiciera falta cambiar exigiría solo un despliegue, no un cambio de esquema.

## Captcha: Turnstile por defecto (ADR 0003), umbral de puntaje y comportamiento sin credenciales

Se implementa `internal/confianza/adaptadores/turnstile/verificador_captcha.go` contra la API real de `siteverify` de Cloudflare Turnstile — ADR 0003 ya había decidido el proveedor; este ADR cierra la implementación y decide el detalle operacional que faltaba:

- **Turnstile no expone un score continuo** (a diferencia de reCAPTCHA v3): la API de verificación solo informa `success: true|false`. El puerto `VerificadorCaptcha.Verificar` sigue devolviendo un `float64` 0.0–1.0 porque el contrato es agnóstico de proveedor (mismo espíritu que ADR 0003: "compatible con la misma API cliente/servidor de reCAPTCHA v2/v3"); el adaptador Turnstile simplemente nunca devuelve un valor intermedio (`1.0` o `0.0`). El día que se swapee a reCAPTCHA v3 (alternativa ya soportada por ADR 0003), el gate de puntaje (`dominio.EvaluarPuntajeCaptcha`, umbrales `0.5`/`0.3`) empieza a usar la banda intermedia sin tocar `EvaluarTrustSignalCasoDeUso`.
- **Sin `TURNSTILE_SECRET_KEY` configurada** (desarrollo sin cuenta de Cloudflare): mismo patrón "modo inseguro posible pero nunca silencioso" que ADR 0017 y `NotificadorCorreoLog`, pero con una diferencia deliberada — en producción **no** se permite el modo inseguro:
  - `APP_ENV=production|prod`: **fail-closed**. `slog.Error` en el arranque; `Verificar` siempre devuelve error (puntaje 0) sin llamar a la red. Un despliegue de producción sin la llave queda con el captcha "cerrado" — todo lo que dependa de un captcha válido (el bypass del cooldown de cuenta) se deniega, en vez de "abierto" (aceptar cualquier cosa). Se decidió así porque, a diferencia del rol de base de datos de ADR 0017, la superficie de abuso de un captcha "abierto por defecto" (creación masiva de cuentas, fuerza bruta sin fricción) es demasiado alta para tolerarla con un WARN nada más.
  - Cualquier otro entorno: **fail-open** (`slog.Warn`, puntaje 1.0 sin llamar a la red) — para no bloquear desarrollo local sin cuenta de Cloudflare.
- **Verificación fail-closed ante error de red del proveedor** (a diferencia de HIBP en Identidad, que es fail-open): si Cloudflare no responde o responde con error, `EvaluarTrustSignalCasoDeUso` trata el puntaje como `0`, no como "no evaluado". Un atacante no debe poder anular la verificación provocando fallos contra el proveedor (ej. saturando su propia conexión a Cloudflare a propósito). El rate limiting en sí (Redis) sí es fail-open (una caída de Redis no puede tumbar login/registro por completo) — son asimetrías deliberadas, documentadas en los comentarios de `evaluar_trust_signal.go`.

## Límite por tenant — explícitamente no cubierto en este hito

El encargo original pedía también un límite agregado por `tenant_id` en el API Gateway/middleware. **No se implementa aquí**: ninguna petición HTTP de Identidad resuelve un `tenant_id` hoy (el contexto Tenencia existe solo como esqueleto de carpetas, sin middleware de resolución de tenant) — no hay una clave real que limitar todavía. `confianza/puertos.Solicitud.TenantID` queda declarado y vacío, listo para cuando exista esa resolución, sin necesidad de tocar `EvaluarTrustSignalCasoDeUso` de nuevo (bastaría con que `TenantID != ""` sume una tercera evaluación de `LimitadorTasa.Permitir` con su propio umbral).

## `ReintentarEn` y la cabecera `Retry-After`

`dominio.ErrAccesoDenegadoPorConfianza` (Identidad) no transportaba `ReintentarEn` — gap ya documentado en `identidad/adaptadores/http/errores_http.go` desde el cierre del MVP ("cuando `EvaluadorConfianza` deje de ser no-op, conviene que el caso de uso o este adaptador tengan forma de propagar `ReintentarEn` hasta aquí"). Se cierra agregando el campo al error de dominio (cambio aditivo, no rompe ningún constructor existente) y usando `huma.ErrorWithHeaders` para fijar `Retry-After` en segundos cuando `ReintentarEn > 0`. Verificado con `curl` contra el servidor real (Postgres + Redis reales, sin credenciales de Turnstile): tres reenvíos a la misma cuenta responden `202`, el cuarto responde `429` con `Retry-After: 900` (los 15 minutos del umbral por cuenta); seis intentos de login a la misma IP responden `401` cinco veces y `429` con `Retry-After: 60` la sexta (umbral de IP, 1 minuto).

## Alternativas consideradas

- **Middleware Fiber genérico envolviendo las rutas**: descartado, ver Decisión 1.
- **Contador en Postgres**: descartado, ver Decisión 2.
- **`redis-cell` (token bucket nativo de Redis vía módulo C)**: no disponible como imagen estándar (`redis:7-alpine` no lo incluye) y hubiera exigido una imagen Docker propia — se prefirió un script Lua sobre `INCR`/`PEXPIRE`, que corre en cualquier Redis estándar sin módulos adicionales y ya soporta el cooldown exponencial que se necesitaba.
- **reCAPTCHA v3 como proveedor de captcha**: ADR 0003 ya lo dejó como alternativa soportada, no default — se mantiene esa decisión sin reabrirla.
- **Requerir captcha en todas las solicitudes de login/registro (no invisible)**: descartado — el encargo pide explícitamente captcha *invisible*; `EvaluarTrustSignalCasoDeUso` solo empieza a exigirlo (`RequiereCaptcha=true`) cuando el límite por cuenta ya se superó (señal de riesgo real), no en el camino feliz.

## Consecuencias

- Un despliegue que no defina `REDIS_URL` sigue arrancando exactamente igual que antes (`EvaluadorConfianzaNoOp`, mismo `WARN` de siempre) — este ADR no rompe ningún entorno de desarrollo existente.
- Un despliegue que defina `REDIS_URL` pero no `TURNSTILE_SECRET_KEY` en producción queda con el captcha fail-closed: cualquier cooldown de cuenta se vuelve un bloqueo duro de 15 minutos sin posibilidad de bypass humano hasta que se configure la llave. Es información que debe estar en cualquier checklist de "listo para producción" (mismo espíritu que la nota equivalente de ADR 0017 sobre `DATABASE_URL_APLICACION`).
- `identidad/adaptadores/http/handlers.go` y `rutas.go` ahora dependen de `puertos.EvaluadorConfianza` además de los puertos de entrada — sigue siendo una dependencia hacia un puerto de Identidad, no hacia un tipo concreto de Confianza, así que INV-ID-19 (aplicación no importa adaptadores) no se ve afectada; es la capa de adaptador HTTP la que gana una dependencia nueva, que es exactamente su rol de orquestación de la petición completa.
- Sin acciones nuevas de auditoría: los motivos de denegación (`limite_ip_excedido`, `limite_cuenta_excedido_requiere_captcha`, `captcha_puntaje_bajo`, etc.) fluyen como `Motivo` dentro de las acciones ya existentes (`usuario.login`/`denegado`, `usuario.registro_rechazado`/`denegado`) para login y registro. El bloqueo del guardián de perímetro de reenvío no se audita formalmente (mismo criterio que el resto de `ReenviarVerificacionCasoDeUso`, que tampoco audita nada — no hay una acción `reenvio_verificacion.*` en el catálogo cerrado, `docs/catalogos/acciones-auditoria.md`); queda solo en logs de aplicación (`slog`). Agregar una acción auditable para reenvíos es trabajo futuro que exige una migración nueva, fuera de alcance de este hito.

## Estado

Aceptado e implementado. Verificado con tests unitarios (`internal/confianza/{dominio,aplicacion}`, `internal/confianza/adaptadores/turnstile` con un `httptest.Server` simulando Cloudflare, `internal/identidad/adaptadores/confianza` y `internal/identidad/adaptadores/http`), con un test de integración contra Redis real (`test/integracion/confianza_redis_test.go`, se salta limpiamente sin `REDIS_URL`) y con una verificación manual end-to-end contra el servidor real (Postgres + Redis reales, sin credenciales de Turnstile) descrita arriba.
