# Bounded Context: Acceso

> Este README es la puerta de entrada para cualquier equipo (frontend u otro
> servicio backend) que necesite **consumir** este contexto por HTTP hoy.
> Para el diseño completo (agregados, invariantes, puertos, casos de uso),
> ver `docs/design/acceso-bounded-context.md`. Para las decisiones de
> arquitectura no obvias citadas aquí, ver `docs/adr/` (en particular ADR
> 0009, 0019 y 0020).

## Responsabilidad

Acceso responde una sola pregunta: **este sujeto ya demostró quién es —
¿qué credencial de sesión le doy, por cuánto tiempo, y sigue siendo
válida?** Orquesta el login (sin verificar contraseñas por sí mismo), emite
y firma el token de acceso (JWT), emite y rota el token de refresco,
gestiona el ciclo de vida de la sesión (activa/revocada/expirada), y
publica el JWKS con el que cualquier servicio verifica los tokens.

Acceso **no** hace (y por diseño no debe hacerse aquí):

- Verificar contraseñas, MFA/OTP o estado de la cuenta — eso es
  **Identidad** (ADR 0009). Acceso nunca lee `contrasena_hash`, nunca
  instancia un hasher, nunca hace `JOIN` contra `usuarios`.
- Decidir qué puede hacer el sujeto (roles, permisos, organización) —
  contexto **Tenencia**, que todavía no existe en este repositorio.
- Rate limiting, captcha, score de riesgo — contexto **Confianza**. Acceso
  sí lo evalúa en `renovación` y `cierre masivo`, no en el login (ya lo
  hace Identidad ahí; ver `docs/design/acceso-bounded-context.md` §0). Su
  ACL hacia Confianza (`acceso/adaptadores/confianza/evaluador_confianza.go`)
  puebla los mismos campos aditivos de reconocimiento de origen
  (`HuellaDispositivo`, `IDUsuario`, `IDSolicitud`) que Identidad, aunque
  ese mecanismo solo evalúa señales en `login` (INV-RIES-15) — ver
  `docs/design/fingerprinting-comportamiento.md`.
- Persistir la bitácora forense — contexto **Auditoría** (mismo mecanismo
  ya construido para Identidad: tabla `auditoria`, hash-chaining, ADR
  0005), consumido vía el puerto `RegistroAuditoria`.

Acceso **es más consumidor que proveedor de puertos de otros contextos**:
llama a `identidad/puertos.AutenticadorDeCredenciales`,
`identidad/puertos.ConsultorDeUsuarios` y, desde el cierre de la
extensión OTP/MFA, `identidad/puertos.VerificadorOTP` (para
`POST /acceso/sesiones/segundo-factor` — ver más abajo), todos a través de
su propio ACL (`internal/acceso/adaptadores/identidad/`, el único
paquete de Acceso autorizado a importar algo de Identidad — INV-ACC-19).
Lo que Acceso sí
**expone** a otros contextos es `puertos.ValidadorDeAccesos`: es el puerto
que cualquier middleware HTTP del sistema consume para autenticar una
petición Bearer. Hoy tiene un consumidor real: el middleware de
autenticación de Identidad (`internal/identidad/adaptadores/http/middleware_autenticacion.go`),
que cierra el hueco que dejó `GET /identidad/usuarios/{id}` público como
placeholder (ver la sección de Identidad más abajo). `RevocadorDeSesiones`
y `ConsultorDeSesiones` están declarados como puertos que Acceso expone
(§2.4 del diseño) pero hoy no tienen consumidor: quedan documentados para
que el día que Identidad implemente `SuspenderUsuario`/`CambiarContrasena`,
o Tenencia necesite un panel de "dispositivos conectados", no se invente un
acceso directo a las tablas `sesiones`/`tokens_refresco`.

## Cómo levantarlo en local

```bash
make docker-up     # levanta Postgres y Redis
make migrate-up     # aplica las migraciones, incluidas 000006/000007/000008 (sesiones, catálogo de auditoría, fix de FK diferible)
make run             # arranca el servidor en :8080 (PORT por defecto) — monta Identidad Y Acceso sobre el mismo *fiber.App
```

Acceso se monta siempre junto con Identidad en `cmd/api/main.go`
(`montarIdentidadYAcceso` → `montarAcceso`); no existe un modo de arrancar
solo uno de los dos contextos vía `make run`.

### Configuración específica de Acceso (ADR 0020)

| Variable | Obligatoria en `production` | Efecto si falta fuera de producción |
|---|---|---|
| `ACCESO_EMISOR` | Sí (el proceso no arranca sin ella en `APP_ENV=production`) | Se usa el valor de desarrollo `https://accesos.moterus.local`, con un `WARN` explícito de arranque. |
| `ACCESO_AUDIENCIA` | Sí | Se usa el valor de desarrollo `moterus`, con `WARN`. |
| `ACCESO_LLAVE_FIRMA` | Sí (llave privada Ed25519 activa, PEM o seed base64) | Se genera una **llave Ed25519 efímera en memoria** vía `accesojwt.GenerarLlaveEfimera()`, con un `WARN` explícito: *"todos los tokens de acceso firmados mueren al reiniciar el proceso"*. Aceptable en desarrollo local, nunca en producción. |
| `ACCESO_LLAVES_VERIFICACION_PREVIAS` | No, opcional | Lista separada por comas de llaves públicas/privadas previas que se mantienen en el conjunto de verificación durante una ventana de rotación (≥ `vidaTokenAcceso`, 10 min). Vacía por defecto: sin ella, solo se verifica con la llave activa. |

En `APP_ENV=production`, la falta de `ACCESO_LLAVE_FIRMA` es **fail-closed
duro**: el proceso hace `log.Fatalf` y no arranca (mismo criterio que ADR
0020 §4 — "un servicio de autenticación que no puede firmar tokens no
tiene nada que hacer sirviendo tráfico"). `ACCESO_EMISOR`/`ACCESO_AUDIENCIA`
siguen la misma regla fail-closed en producción, vía el helper
`exigirEnProduccionOAdvertir` de `cmd/api/main.go`.

`REDIS_URL` decide si la lista de revocación (aceleradora, no la frontera
de seguridad — INV-ACC-15) es real o no-op: sin ella, la revocación de un
token de acceso sigue siendo correcta pero tarda hasta `vidaTokenAcceso`
(10 min); con ella, es inmediata. Mismo `REDIS_URL` que usa Confianza para
Identidad (ADR 0018) — ambos contextos comparten el motor de riesgo cuando
está configurado.

Log de arranque real observado (con Postgres+Redis reales, sin las cuatro
variables `ACCESO_*` definidas):

```
api: ACCESO_EMISOR no está definida; usando el valor de desarrollo "https://accesos.moterus.local"
api: ACCESO_AUDIENCIA no está definida; usando el valor de desarrollo "moterus"
api: ALERTA — ACCESO_LLAVE_FIRMA no está definida, usando una llave Ed25519 efímera generada en memoria. Todos los tokens de acceso firmados mueren al reiniciar el proceso. No usar así en producción (ADR 0020 §4).
api: contexto Acceso montado (postgres, jwx/ed25519 kid=<thumbprint RFC 7638>, redis(lista-revocacion-inmediata), confianza-real(redis+turnstile), auditoria, eventos-log)
```

## Documentación OpenAPI (generada, no se edita a mano)

Por ADR 0006, la spec la genera Huma v2 automáticamente a partir de los
DTOs de `internal/acceso/adaptadores/http/dtos.go` y las operaciones
registradas en `rutas.go`. **Acceso monta su propia instancia de
`huma.API` con un namespace de metadatos propio**, distinto del de
Identidad — con el servidor arriba (`make run`):

- `GET /acceso/openapi.json` — OpenAPI 3.1, JSON, solo de Acceso.
- `GET /acceso/openapi.yaml` — OpenAPI 3.1, YAML.
- `GET /acceso/docs` — UI interactiva (Stoplight Elements).
- `GET /acceso/schemas/{schema}.json` — resolución de `$ref` de los DTOs.

**No uses `/openapi.json` ni `/docs` sin prefijo para Acceso** — esas rutas
sin prefijo sirven la spec de **Identidad** (`huma.DefaultConfig` sin
overrides, montado después en el mismo `*fiber.App`). Ver la nota de la
sección siguiente para el porqué.

> **Nota — bug real encontrado y corregido en este cierre de documentación,
> no un pendiente:** Acceso e Identidad montan cada uno su propia instancia
> de `humafiber.NewV2` sobre el mismo `*fiber.App` (`cmd/api/main.go`
> registra las rutas de ambos contextos). Con `huma.DefaultConfig` sin
> overrides en los dos, ambas instancias competían por las mismas rutas
> exactas (`/openapi.json`, `/openapi.yaml`, `/docs`, `/schemas/*`), y Fiber
> solo servía la primera registrada (Acceso, que se monta antes que
> Identidad en `main.go`) — la spec/docs de Identidad quedaban
> silenciosamente inalcanzables. Se corrigió dándole a Acceso su propio
> namespace de metadatos en `rutas.go` (`cfgAcceso.OpenAPIPath =
> "/acceso/openapi"`, `DocsPath = "/acceso/docs"`, `SchemasPath =
> "/acceso/schemas"`); Identidad sigue en las rutas por defecto sin
> prefijo. **Verificación en vivo** (servidor real, Postgres+Redis reales,
> no una lectura estática de código): `GET /openapi.json` (raíz) sirve la
> spec de Identidad (paths `/identidad/usuarios`,
> `/identidad/autenticaciones`, `/identidad/usuarios/{id}`,
> `/identidad/verificaciones-correo`,
> `/identidad/verificaciones-correo/reenvios`) y `GET /docs` → 200;
> `GET /acceso/openapi.json` sirve la spec de Acceso (paths
> `/.well-known/jwks.json`, `/acceso/sesiones`, `/acceso/sesiones/actual`,
> `/acceso/sesiones/renovaciones`, `/acceso/sesiones/{id}`) y
> `GET /acceso/docs` → 200; `GET /acceso/schemas/{schema}.json` → 200 y
> resuelve `$ref` correctamente.

## Invariantes de seguridad que le importan a quien integra

La lista completa está en `docs/design/acceso-bounded-context.md` §4
(INV-ACC-01 a INV-ACC-24). Las que un cliente/integrador necesita conocer
sin leer el diseño completo:

- **INV-ACC-04** — una sesión activa tiene **exactamente un** token de
  refresco vigente, garantizado por un índice único parcial en Postgres
  (`tokens_refresco_vigente_por_sesion_idx`), no por una comprobación en
  la aplicación.
- **INV-ACC-05** — cada uso de un token de refresco lo consume. No existe
  reutilización legítima: rotación obligatoria en el 100% de las
  renovaciones (`POST /acceso/sesiones/renovaciones` siempre devuelve un
  `token_refresco` nuevo; el usado en la petición queda inservible).
- **INV-ACC-06** — presentar un refresco ya consumido revoca **toda** la
  sesión de inmediato (detección de robo/reuso, OAuth 2.1/RFC 6819). El
  cliente recibe exactamente el mismo `401` que ante un token desconocido
  o expirado (INV-ACC-21): **no intentes distinguir estos casos en el
  cliente**, es intencionalmente indistinguible.
- **INV-ACC-14** — defensa contra *algorithm confusion*: la verificación
  del token de acceso deriva el algoritmo del `kid` presente en el JWKS,
  **nunca** del campo `alg` de la cabecera del token. Un token sin `kid`,
  con `kid` desconocido, con `alg: none` o con `typ != "at+jwt"` se
  rechaza sin evaluar la firma.
- **INV-ACC-23** — ninguna operación de cierre de sesión confía en un
  `usuario_id`/`sesion_id` enviado por el cliente en el cuerpo: siempre
  salen del token Bearer ya validado (`DELETE /acceso/sesiones/actual`) o,
  para `DELETE /acceso/sesiones/{id}`, el `IDUsuario` sale del token y el
  `IDSesion` del parámetro de ruta, verificando pertenencia antes de
  revocar (una sesión ajena da **404**, nunca 403 — un 403 confirmaría que
  ese ID existe).

## Los 8 endpoints

Prefijo `/acceso`, con la única excepción de JWKS (RFC 8615 fija ese
prefijo en la raíz). Los DTOs de request/response de abajo son los campos
reales de `internal/acceso/adaptadores/http/dtos.go`.

### `POST /acceso/sesiones` — Iniciar sesión (login)

Orquesta el login completo (ADR 0009): delega la verificación de
credenciales en Identidad y, si es exitosa, emite un token de acceso (JWT,
10 min) y un token de refresco opaco rotatorio.

`aud` específico: ninguno (endpoint público). Rate limit especial:
ninguno propio de Acceso — Identidad ya evalúa Confianza dentro de
`AutenticarUsuario` con `Accion="login"`; Acceso no lo vuelve a evaluar
aquí (evitaría duplicar el consumo de cuota, ver diseño §0).

**Puede estar detrás de una sala de espera.** Si un operador abrió una
cola de acceso virtual sobre esta ruta (`docs/design/colas-virtuales.md`,
extensión del contexto Confianza), una petición sin un ticket admitido
recibe `503` con `desenlace: ticket_requerido` en vez de llegar al login —
ver `internal/confianza/README.md` para el flujo de ingreso/turno/reclamo.
Fuera de un evento con sala abierta, este mecanismo es completamente
transparente: el costo es una lectura de un puntero atómico.

Request:

```json
{
  "correo": "ana@ejemplo.com",
  "contrasena": "Integr4cion-Segura-Prueba!",
  "token_captcha": "opcional, se reenvía a Identidad"
}
```

Response `201 Created`:

```json
{
  "token_acceso": "eyJhbGciOi...",
  "tipo_token": "Bearer",
  "expira_en_segundos": 600,
  "token_refresco": "mot_rt_...",
  "refresco_expira_en": "2026-10-03T12:00:00Z",
  "id_sesion": "0192...",
  "id_usuario": "0192...",
  "sesion_expira_en": "2026-12-02T12:00:00Z"
}
```

Errores posibles:

| Status | Causa | Error de dominio |
|---|---|---|
| `401` | Correo/contraseña incorrectos (traducido en el ACL desde `identidad/dominio.ErrCredencialesInvalidas`) | `ErrCredencialesRechazadas` |
| `401` | Identidad indicó que se requiere segundo factor — **no se emite sesión** (INV-ACC-03) | `ErrSegundoFactorRequerido` (`type` de problema distinguible `step-up-requerido`; el cuerpo trae `motivo_step_up` y, desde el cierre de OTP/MFA, `token_step_up` — ver el formato exacto justo abajo) |
| `403` | Cuenta no operativa (correo no verificado / suspendida / bloqueada) | `ErrCuentaNoOperativa` (con `Motivo`) |
| `429` | Confianza bloqueó el intento **dentro de Identidad** (propagado con el mismo `ReintentarEn`) | `ErrAccesoDenegadoPorConfianza` (`Retry-After`) |

**Formato exacto del `401` de step-up — no son campos JSON de primer
nivel.** `huma.ErrorModel` (RFC 9457) no tiene un campo propio para
`motivo_step_up`/`token_step_up`, así que ambos viajan como
`huma.ErrorDetail` dentro de `errors[]`, con el prefijo literal
`"clave: valor"` en `message` (`errores_http.go`,
`detallesSegundoFactor`):

```json
{
  "status": 401,
  "title": "se requiere un segundo factor",
  "type": "https://moterus.dev/problemas/step-up-requerido",
  "detail": "la autenticación es válida pero exige un paso adicional antes de emitir una sesión",
  "errors": [
    { "message": "motivo_step_up: mfa_habilitado" },
    { "message": "token_step_up: eyJhbGciOiJFZERTQSJ9...." }
  ]
}
```

Un cliente que espere `body.token_step_up` como campo de primer nivel (el
criterio "obvio" para RFC 9457) va a romperse en la integración: hay que
iterar `errors[]` y separar cada `message` por el primer `": "`. Ver
`errorSegundoFactorRespuesta`/`valor(clave)` en
`test/integracion/acceso_test.go` para una implementación de referencia
ya probada contra el servidor real. Ver también la sección dedicada al
token de step-up más abajo.

### `POST /acceso/sesiones/segundo-factor` — Completar el login con el segundo factor (MFA)

Paso 2 del login cuando `POST /acceso/sesiones` devolvió `token_step_up`
(§3.6 de `docs/design/otp-mfa.md`). **Sin Bearer**: la credencial de este
endpoint es el propio `token_step_up` del cuerpo — nunca un token de
acceso normal, que el validador de step-up rechaza explícitamente (ver
"Token de step-up" más abajo, con la verificación en vivo de ambos
sentidos del rechazo). Si el código es válido, emite la sesión completa
con exactamente la misma lógica que el login normal
(`emitirSesionCompleta`, compartida), con `amr: ["pwd", "otp"]` en el JWT
resultante — **verificado en vivo decodificando el token**, no solo
asumido: es el único punto del sistema que emite ese claim con dos
elementos.

`aud` específico: ninguno. Rate limit especial: **sí** — ADR 0018 /
`docs/design/otp-mfa.md` §7, `EvaluadorConfianza` con
`Accion="verificar_otp"`, clave por `IDUsuario` (no por IP: es un ataque
dirigido a una cuenta concreta). Umbrales: 5/15min por usuario, 20/15min
por IP — agresivo a propósito, es un oráculo de fuerza bruta clásico
sobre un código de 6 dígitos con ventanas de validez de 30s.

Request:

```json
{ "token_step_up": "eyJhbGciOiJFZERTQSJ9....", "codigo": "123456" }
```

Response `201 Created`: mismo `resultadoSesionRespuesta` que el login
normal (ver arriba).

Errores posibles:

| Status | Causa | Nota |
|---|---|---|
| `401` | `token_step_up` inválido, expirado o de un `typ` distinto del esperado, **o** el código OTP/de respaldo es incorrecto | **Un único error genérico en los tres casos** (INV-MFA-08): no se le da a quien prueba fuerza bruta información de diagnóstico sobre cuál de las tres cosas falló |
| `429` | Confianza denegó el intento | `ErrAccesoDenegadoPorConfianza` (`Retry-After`) |

El código aceptado es tanto un TOTP vigente como cualquiera de los 10
códigos de respaldo no usados del usuario — verificado por Identidad vía
`puertos.VerificadorOTP`, consumido por el mismo ACL de Acceso hacia
Identidad que ya usa `IniciarSesion` (`internal/acceso/adaptadores/identidad/`).

### `POST /acceso/sesiones/renovaciones` — Renovar la sesión (rotar el refresco)

Consume el token de refresco presentado y emite uno nuevo. Es el único
endpoint autenticado por el propio token de refresco (no por Bearer).

`aud` específico: ninguno. Rate limit especial: **sí** — ADR 0018/0019,
`EvaluadorConfianza` con `Accion="renovacion_sesion"` (propuesto: 30/min
por IP, 10/min por sesión — ver §11.2 del diseño para los umbrales
propuestos y su justificación).

Request:

```json
{ "token_refresco": "mot_rt_..." }
```

Response `200 OK`: mismo `resultadoSesionRespuesta` que el login (nuevo
`token_acceso` y nuevo `token_refresco`; el token de acceso anterior no se
revoca, le quedan ≤10 minutos).

Errores posibles:

| Status | Causa | Nota |
|---|---|---|
| `401` | Refresco desconocido, expirado, ya consumido (reuso) o estructuralmente inválido | **Misma respuesta observable en los cuatro casos** (INV-ACC-21) — `ErrRefrescoInvalido` |
| `401` | Sesión expirada (ventana de inactividad o vida absoluta agotada, con el refresco vigente presentado) | `ErrSesionExpirada` |
| `401` | Sesión revocada (con el refresco vigente presentado) | `ErrSesionRevocada` |
| `429` | Confianza bloqueó la renovación | `ErrAccesoDenegadoPorConfianza` (`Retry-After`) |

### `DELETE /acceso/sesiones/actual` — Cerrar la sesión actual (logout individual)

Revoca la sesión del propio token Bearer. `IDSesion` **nunca** viene del
cliente (INV-ACC-23): sale del token ya validado. Idempotente: cerrar una
sesión ya revocada devuelve el mismo `204` sin auditar de nuevo.

`aud` específico: ninguno. Auth: Bearer. Rate limit especial: ninguno.

Response: `204 No Content`, cuerpo vacío.

Errores: `401` (sin token o token inválido/expirado/revocado — ver
"Formato de error 401" más abajo).

### `DELETE /acceso/sesiones/{id}` — Cerrar una sesión concreta

Revoca la sesión `{id}` si pertenece al sujeto autenticado.

`aud` específico: ninguno. Auth: Bearer. Rate limit especial: ninguno.

Response: `204 No Content`.

Errores: `401` (sin token válido); `404` si `{id}` no existe **o** es de
otro usuario — nunca `403` (confirmaría que el ID existe, `ErrSesionAjena`
mapea a `404` a propósito).

### `DELETE /acceso/sesiones` — Cerrar todas las sesiones (logout de todos los dispositivos)

Revoca **todas** las sesiones activas del usuario, **incluida la actual**
(no hay mecanismo de "preservar la sesión actual" implementado para esta
ruta — el comando `ComandoCerrarTodasLasSesiones` sí declara un campo
`IDSesionAPreservar` opcional en el diseño, pero el handler HTTP no lo
expone hoy). Operación de alto valor: exige `ExigirSesionViva=true` (una
ventana de revocación de hasta 10 minutos no es aceptable aquí) y evalúa
Confianza (`Accion="cierre_masivo_sesiones"`, propuesto: 5/min por IP,
3/15min por usuario).

`aud` específico: ninguno. Auth: Bearer, con verificación adicional contra
`sesiones` en Postgres (no solo la firma). Rate limit especial: sí.

Response: `204 No Content`.

Errores: `401`; `429` (Confianza).

### `GET /acceso/sesiones` — Listar mis sesiones activas

Pantalla de "dispositivos conectados": devuelve las sesiones activas del
usuario autenticado, marcando cuál es la actual. No se audita (equivalente
a consultar el perfil propio).

`aud` específico: ninguno. Auth: Bearer. Rate limit especial: ninguno.

Response `200 OK`:

```json
[
  {
    "id": "0192...",
    "es_sesion_actual": true,
    "estado": "activa",
    "creada_en": "2026-09-01T10:00:00Z",
    "ultima_renovacion_en": "2026-09-03T09:50:00Z",
    "expira_absoluto_en": "2026-12-01T10:00:00Z",
    "ip_origen": "203.0.113.4",
    "agente_usuario": "Mozilla/5.0 ..."
  }
]
```

Nunca expone hashes de refresco ni la cadena de rotación (modelo de
lectura, estructuralmente incapaz de filtrar ese dato).

Errores: `401`.

### `GET /.well-known/jwks.json` — Conjunto de llaves públicas (JWKS)

**En la raíz, sin prefijo `/acceso`** — RFC 8615 fija ese prefijo y las
bibliotecas cliente de JWKS lo asumen. Público, sin autenticación ni paso
por Confianza (es exactamente su propósito: descubrimiento de llaves
públicas). Respuesta con `Cache-Control: public, max-age=300`.

Response `200 OK` (llave Ed25519 activa, formato RFC 7517):

```json
{
  "keys": [
    {
      "kty": "OKP",
      "kid": "<thumbprint RFC 7638 de la llave pública>",
      "use": "sig",
      "alg": "EdDSA",
      "crv": "Ed25519",
      "x": "<material público, base64url>"
    }
  ]
}
```

**Verificado en vivo contra el servidor real** (Postgres+Redis reales):
`GET /.well-known/jwks.json` → `200`, sirve la llave pública Ed25519
(`kty=OKP`, `crv=Ed25519`, `kid` = thumbprint RFC 7638), consistente con
el `kid` que loguea `main.go` al arrancar (`llavero.KIDActivo()`).

### Formato del error `401` (todos los endpoints protegidos)

Todos los `401` de Acceso llevan la cabecera `WWW-Authenticate: Bearer
error="invalid_token"` (RFC 6750) y un cuerpo RFC 9457
(`application/problem+json`). El middleware de autenticación
(`internal/acceso/adaptadores/http/middleware.go`,
`MiddlewareAutenticacion`) distingue mensaje pero **nunca** distingue el
status entre "falta la cabecera", "token expirado", "token inválido",
"sesión revocada" o "sesión no encontrada": todos son `401` — un cliente
no debería intentar rama-ificar el comportamiento a partir del mensaje.

## Formato del token de acceso (JWT, `typ: at+jwt` — perfil RFC 9068)

| Claim | Valor | Por qué |
|---|---|---|
| `iss` | `ACCESO_EMISOR` | RFC 7519; el verificador rechaza tokens de otro emisor |
| `sub` | ID del usuario | el sujeto |
| `aud` | `ACCESO_AUDIENCIA` (fijo) | ADR 0002: un solo producto ⇒ `aud` fijo |
| `exp`, `iat`, `nbf` | según `PoliticaSesion` (10 min de vida, tolerancia de reloj 60 s) | `nbf = iat` |
| `jti` | UUIDv4 | correlación forense y replay detection; no ordenable a propósito |
| `sid` | UUIDv7, el `IDSesion` | **clave de revocación** y correlación con la tabla `sesiones` |
| `amr` | `["pwd"]` (login normal, sin segundo factor) **o** `["pwd","otp"]` (tras completar `POST /acceso/sesiones/segundo-factor`) | RFC 8176 — verificado en vivo decodificando ambas variantes del JWT emitido |
| `auth_time` | instante de la autenticación original de la sesión | políticas de reautenticación |
| `ver` | `1` | versión del formato de claims |

**Claims que deliberadamente NO existen**: `email`, `roles`, `permisos`,
`org_id`, `tenant_id`, `huella_dispositivo`, `ip`. Si tu integración
necesita saber el rol o la organización del usuario, ese dato **no está en
el JWT** — hoy no hay ningún contexto que lo resuelva (Tenencia no existe
todavía); no lo infieras del token.

## Token de step-up (segundo factor pendiente) — ADR 0038

Cuando `POST /acceso/sesiones` responde `401` con `type` de problema
`step-up-requerido`, el cuerpo trae un `token_step_up` (ver el formato
exacto en la sección del endpoint más arriba): un JWT **propio** de
Acceso, firmado con la misma llave de firma de un token de acceso normal
(ADR 0020, mismo `Llavero` — no hace falta una segunda llave ni un
segundo JWKS), que representa "la contraseña es correcta, falta el
segundo factor". **No es una sesión**: no tiene fila en `sesiones`, no
tiene refresco, y nunca puede usarse donde se espera un token de acceso
normal, ni viceversa.

| Propiedad | Valor |
|---|---|
| TTL | **5 minutos**, sin refresco posible (INV-MFA-04) — la mitad de la vida del token de acceso normal (10 min, ADR 0019); expirado, no hay más camino que volver a loguear con usuario+contraseña desde cero |
| `typ` de cabecera | `step-up+jwt` — **nunca** `at+jwt` |
| Claims | `sub` (usuario) y `motivo_step_up` (`mfa_habilitado` \| `confianza_baja`, el mismo valor que Identidad ya calculó) — sin claims de autorización ni PII |
| Dónde se presenta | Campo `token_step_up` del cuerpo de `POST /acceso/sesiones/segundo-factor` — **nunca** como cabecera `Authorization: Bearer` |

**Por qué un `typ` de cabecera distinto y no un claim (`step_up_pendiente:
true`) sobre el token de acceso normal:** cada validador del sistema
(Acceso, Identidad, Tenencia) ya exige `typ == "at+jwt"` como parte de
INV-ACC-14 (defensa contra *algorithm confusion*). Un token de step-up
con un `typ` distinto queda automáticamente rechazado por cualquier
endpoint que no sea explícitamente `POST /acceso/sesiones/segundo-factor`,
sin necesitar ningún código nuevo en esos validadores — la alternativa
del claim habría obligado a todos los validadores existentes a aprender a
revisarlo, exactamente el tipo de responsabilidad repartida que ADR 0038
descarta.

**Verificación en vivo de los dos sentidos de este rechazo**
(servidor real, Postgres+Redis reales, con `curl`, no solo tests
automatizados):

1. Presentar el `token_step_up` como `Authorization: Bearer` contra un
   endpoint protegido normal (`GET /acceso/sesiones`) → `401`. El
   validador de tokens de acceso normales lo rechaza por su `typ`
   (`step-up+jwt` no es `at+jwt`).
2. Presentar un token de acceso normal ya emitido y válido como
   `token_step_up` en `POST /acceso/sesiones/segundo-factor` → `401`. El
   validador de step-up lo rechaza por el mismo motivo, en la dirección
   opuesta (`at+jwt` no es `step-up+jwt`).

Ninguno de los dos rechazos necesitó ningún caso especial en el código:
es consecuencia directa de que cada validador exige su propio `typ`
exacto — el mismo mecanismo, aplicado dos veces.

## Auditoría

Emisión, renovación (éxito y fallo), reuso, cierre y revocación se
auditan **siempre**, en la misma transacción que la escritura de negocio
(ADR 0005). La validación de un token **no** se audita, salvo los rechazos
con valor de señal (firma inválida, `kid`/`alg`/`typ` inesperado, sesión
en lista de revocación) — un `401` por token simplemente expirado es el
evento más frecuente del sistema y no se audita (INV-ACC-17). Catálogo
completo de las 6 acciones nuevas: `docs/catalogos/acciones-auditoria.md`
(sección "Contexto Acceso").

**`CompletarSegundoFactor` no agrega ninguna acción de auditoría nueva a
este contexto**: reutiliza `emitirSesionCompleta`, así que el evento que
persiste al completar el segundo factor es el mismo `sesion.iniciada` que
emite un login normal — no hay un `sesion.iniciada_con_otp` distinto. Lo
único que distingue ambos casos es el claim `amr` **dentro del JWT**, no
la fila de auditoría. Verificado en vivo con la cadena de auditoría
completa de un flujo E2E (`verificar_cadena_auditoria()`, 0
discrepancias): `usuario.mfa_habilitado`, `usuario.mfa_confirmado`
(ambos en Identidad, ver `internal/identidad/README.md`),
`usuario.step_up_requerido` (emitido por Identidad en el login que exige
el segundo factor, catálogo previo a esta extensión) y `sesion.iniciada`
**dos veces** — una del primer login sin MFA activo, otra al completar el
segundo factor.

## Gotcha real de implementación (documentado en la migración, no un pendiente)

`db/migraciones/000008_tokens_refresco_hash_sucesor_deferrable.up.sql`
corrige un bug bloqueante encontrado al verificar manualmente la rotación:
la secuencia correcta dentro de una transacción es primero marcar el token
viejo como consumido (con `hash_sucesor` apuntando al nuevo, que todavía
no existe) y **después** insertar el token nuevo — es el único orden
compatible con el índice único parcial `tokens_refresco_vigente_por_sesion_idx`
(INV-ACC-04). Eso requiere que la FK `tokens_refresco_hash_sucesor_fkey`
sea `DEFERRABLE INITIALLY DEFERRED` (su chequeo se pospone al `COMMIT`);
en su modo por defecto (no diferible), el `UPDATE` del paso 1 fallaba de
inmediato con `SQLSTATE 23503`. Ver el comentario de cabecera de esa
migración y de `Guardar` en
`internal/acceso/adaptadores/postgres/repositorio_sesiones.go`.

## Referencias

- Diseño completo: `docs/design/acceso-bounded-context.md`
- ADR 0009 (frontera Identidad/Acceso): `docs/adr/0009-frontera-identidad-acceso.md`
- ADR 0019 (mecanismo de sesión): `docs/adr/0019-mecanismo-sesion-jwt-refresco-rotatorio.md`
- ADR 0020 (algoritmo de firma y rotación de llaves): `docs/adr/0020-algoritmo-firma-jwt-rotacion-llaves.md`
- README de Identidad (contexto consumidor de `ValidadorDeAccesos`, y
  proveedor del autoservicio de MFA que este contexto completa):
  `internal/identidad/README.md`
- Diseño de la extensión OTP/MFA (extiende Identidad y Acceso, no un
  contexto nuevo): `docs/design/otp-mfa.md`
- ADR 0037 (MFA del MVP limitado a TOTP): `docs/adr/0037-mfa-mvp-limitado-a-totp.md`
- ADR 0038 (token de step-up, este contexto): `docs/adr/0038-token-step-up-jwt-propio-ttl-corto.md`
- ADR 0039 (deshabilitar MFA exige código propio): `docs/adr/0039-deshabilitar-mfa-exige-codigo-propio.md`
- ADR 0040 (códigos de respaldo generados en la confirmación): `docs/adr/0040-codigos-respaldo-en-confirmacion.md`
- Diseño de colas de acceso virtual (extiende Confianza; puede proteger
  `POST /acceso/sesiones` con una sala de espera):
  `docs/design/colas-virtuales.md`
- README de Confianza: `internal/confianza/README.md`
- Índice completo de ADRs: `docs/adr/README.md`
