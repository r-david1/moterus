# Bounded Context: Identidad

> Este README es la puerta de entrada para cualquier equipo (frontend u otro
> servicio backend) que necesite **consumir** este contexto por HTTP hoy.
> Para el diseño completo (agregados, invariantes, puertos, casos de uso),
> ver `docs/design/identidad-bounded-context.md`. Para las decisiones de
> arquitectura no obvias citadas aquí, ver `docs/adr/`.

## Responsabilidad

Identidad responde una sola pregunta: **¿quién es este sujeto y puede
demostrarlo?** Alta de usuario, verificación de credenciales y estado del
ciclo de vida de la cuenta (`pendiente_verificacion` / `activo` /
`suspendido` / `bloqueado` / `anonimizado`).

Identidad **no** hace (y por diseño no debe hacerse aquí):

- Emitir o validar JWT/sesiones/refresh tokens — eso es el contexto
  **Acceso** (ADR 0009), implementado desde ADR 0019/0020 — ver
  `internal/acceso/README.md`. `GET /identidad/usuarios/{id}` ya exige un
  token de acceso válido emitido por Acceso (ver más abajo); los otros
  cuatro endpoints de Identidad siguen sin requerir token, porque son
  precisamente el paso previo a obtener uno (alta de cuenta, verificación
  de credenciales sin sesión, confirmación de correo).
- Roles, organizaciones, membresías — contexto **Tenencia** (implementado;
  ver `docs/design/tenencia-bounded-context.md`). Identidad consume su
  `VerificadorDeAutorizacion`/`ConsultorDeMembresias` solo a través de
  `identidad/adaptadores/tenencia` (el único paquete autorizado a importar
  `tenencia/puertos`), para cerrar la autorización de
  `GET /identidad/usuarios/{id}` — ver más abajo.
- Rate limiting, captcha, score de riesgo — contexto **Confianza**. Desde
  ADR 0018 hay una implementación real (`EvaluadorConfianzaReal`, Redis +
  Cloudflare Turnstile) además del adaptador *no-op* original — ver más
  abajo cuál se monta según la configuración.
- Persistir la bitácora forense — contexto **Auditoría** (implementado como
  tablas/triggers en Postgres dentro de este mismo repo, consumido por
  Identidad vía el puerto `RegistroAuditoria`).

## Puertos que este contexto expone a otros contextos

| Puerto | Para qué | Implementado por |
|---|---|---|
| `puertos.RegistradorDeUsuarios` | Alta de usuario | `aplicacion.RegistrarUsuarioCasoDeUso` |
| `puertos.AutenticadorDeCredenciales` | Verificar correo+contraseña (sin emitir sesión) | `aplicacion.AutenticarUsuarioCasoDeUso` |
| `puertos.ConsultorDeUsuarios` | Resolver datos mínimos de un usuario por ID | `aplicacion.ObtenerUsuarioCasoDeUso` |
| `puertos.GestorDeMFA` | Autoservicio de MFA del propio sujeto (habilitar/confirmar/deshabilitar un factor TOTP) | `aplicacion.Habilitar/Confirmar/DeshabilitarMFACasoDeUso`, compuestos en `cmd/api/main.go` (`gestorMFACompuesto`, porque `GestorDeMFA` agrupa tres operaciones y ningún struct de aplicación las implementa por sí solo) |
| `puertos.VerificadorOTP` | Verificar un código OTP (TOTP o de respaldo) contra el sujeto — el camino caliente del step-up | `aplicacion.VerificarOTPCasoDeUso` |

Ver `internal/identidad/puertos/entrada.go` para las firmas exactas. Desde
ADR 0019/0020, el contexto **Acceso** ya es el consumidor de
`AutenticadorDeCredenciales` (en `IniciarSesion`, vía su ACL
`internal/acceso/adaptadores/identidad/`) y de `ConsultorDeUsuarios` (en
`RenovarSesion`, para revalidar en cada renovación que la cuenta siga
`activa` — es el mecanismo por el que Acceso se entera de una suspensión
sin depender de un broker de eventos). Ver `internal/acceso/README.md`.
Desde el cierre de la extensión OTP/MFA (`docs/design/otp-mfa.md`, ADR
0037-0040), **Acceso** es también el único consumidor de
`VerificadorOTP`, vía el mismo ACL, para completar
`POST /acceso/sesiones/segundo-factor` — ver la sección de MFA más abajo
y `internal/acceso/README.md`.

## Cómo levantarlo en local

```bash
make docker-up     # levanta Postgres y Redis
make migrate-up     # aplica las migraciones: usuarios, auditoria, rol_login_aplicacion, tokens_verificacion_correo
make run             # arranca el servidor en :8080 (PORT por defecto)
```

`make run` usa `DATABASE_URL_APLICACION` (rol `rol_login_identidad`, de
privilegios acotados — ADR 0017), **no** `DATABASE_URL` (rol dueño, solo
para migraciones). Si `DATABASE_URL_APLICACION` no está definida, el
proceso arranca igual mostrando un `WARN` explícito y cae al rol dueño —
aceptable en desarrollo local, nunca en producción (ver `cmd/api/main.go`
y ADR 0017).

Valores de desarrollo (`Makefile`):

```
DATABASE_URL             = postgres://auth_service:auth_service_dev_password@localhost:5432/auth_service?sslmode=disable
DATABASE_URL_APLICACION  = postgres://rol_login_identidad:identidad_app_dev_password@localhost:5432/auth_service?sslmode=disable
```

### Cifrado del secreto TOTP (ADR 0038, `docs/design/otp-mfa.md` §2.2)

`IDENTIDAD_LLAVE_CIFRADO_MFA` (env, AES-256-GCM, 32 bytes en base64) es la
llave con la que `identidad/adaptadores/cripto.CifradorSecretosAESGCM`
cifra/descifra en reposo el secreto TOTP de cada `FactorMFA` —
**cifrado simétrico reversible, no un hash** (a diferencia de
`HasherContrasenas`/Argon2id): el servidor necesita poder leer el secreto
en claro para computar el código esperado en cada verificación
(INV-MFA-02). Mismo criterio de gestión que `ACCESO_LLAVE_FIRMA` (ADR
0020, ver `internal/acceso/README.md`): en `APP_ENV=production`, su
ausencia es fail-closed duro (`log.Fatalf`, el proceso no arranca); fuera
de producción, se genera una llave AES-256 efímera en memoria con un
`WARN` explícito. **Consecuencia concreta de no fijarla en desarrollo
persistente**: un usuario que confirmó MFA antes de un reinicio del
proceso queda con un secreto indescifrable tras el reinicio, y necesita
deshabilitar y volver a habilitar MFA desde cero (`construirCifradorSecretosMFA`
en `cmd/api/main.go`).

### Rate limiting y captcha (ADR 0018)

`REDIS_URL` (env, sin valor por defecto) decide qué `EvaluadorConfianza` se
monta en `cmd/api/main.go`:

- **Sin `REDIS_URL`**: `EvaluadorConfianzaNoOp` (comportamiento previo,
  sin cambios) — `WARN` de arranque, ningún límite real.
- **Con `REDIS_URL`** (p. ej. `redis://localhost:6379/0`, el Redis de
  `docker-compose.yml`): `EvaluadorConfianzaReal` — rate limiting por IP y
  por cuenta vía Redis, más verificación de captcha (Cloudflare Turnstile
  por defecto, ADR 0003) si `TURNSTILE_SECRET_KEY` está definida. Sin esa
  llave: fail-open con `WARN` en desarrollo, fail-closed con `ERROR` si
  `APP_ENV=production`. `TURNSTILE_VERIFY_URL` permite apuntar a un
  servidor de pruebas en vez del endpoint real de Cloudflare.

Probar localmente sin credenciales reales de Turnstile: basta con levantar
Redis (`make docker-up`) y definir `REDIS_URL` — el captcha queda en modo
fail-open (dev) automáticamente, sin necesidad de una cuenta de Cloudflare.
Ver `docs/adr/0018-rate-limiting-captcha-confianza-redis.md` para los
umbrales exactos y la verificación manual contra el servidor real.

Health check de infraestructura (no es un endpoint de negocio de ningún
contexto): `GET /health`.

## Documentación OpenAPI (generada, no se edita a mano)

Por ADR 0006, la spec la genera Huma v2 automáticamente a partir de los
DTOs de `internal/identidad/adaptadores/http/dtos.go` y las operaciones
registradas en `rutas.go` (`huma.DefaultConfig("Identidad", "0.1.0")`, sin
overrides de rutas). Con el servidor arriba (`make run`), la spec y la UI
interactiva quedan disponibles en:

- `GET /openapi.json` — OpenAPI 3.1, JSON.
- `GET /openapi.yaml` — OpenAPI 3.1, YAML.
- `GET /docs` — UI interactiva (Stoplight Elements), servida por Huma
  apuntando a `/openapi.json`.

> **Nota de verificación en vivo (servidor real, Postgres+Redis reales, no
> una lectura estática de código):** estas tres rutas son los valores por
> defecto de `huma.DefaultConfig` y `rutas.go` no los sobreescribe. Desde
> que Acceso se implementó (ADR 0019/0020) y se monta en el mismo
> `*fiber.App`, hubo un riesgo real de colisión de rutas — ver la nota
> equivalente en `internal/acceso/README.md` sobre el namespace propio que
> Acceso le da a sus rutas de metadatos (`/acceso/openapi.json`,
> `/acceso/docs`, `/acceso/schemas`) precisamente para no pisar estas. Ya
> verificado: `GET /openapi.json` (raíz) sirve la spec de **Identidad**,
> con `.paths` incluyendo `/identidad/usuarios`,
> `/identidad/autenticaciones`, `/identidad/usuarios/{id}`,
> `/identidad/verificaciones-correo` y
> `/identidad/verificaciones-correo/reenvios` (dos endpoints que se
> agregaron después del MVP original de 3 — ver el aviso más abajo); y
> `GET /docs` (raíz) → `200`.

## Los 3 endpoints (MVP)

> **Aviso de alcance:** este README documenta en detalle los 3 endpoints
> del MVP original, más los 3 endpoints de autoservicio de MFA (sección
> dedicada más abajo, agregada en el cierre de la extensión
> `docs/design/otp-mfa.md`). `rutas.go` registra hoy **8** operaciones en
> total: las 6 ya nombradas más `POST /identidad/verificaciones-correo` y
> `POST /identidad/verificaciones-correo/reenvios` (backlog §3.4/3.5 del
> diseño de Identidad, ya implementados). Esos dos últimos siguen fuera
> del alcance de detalle de este README — no se describen aquí con el
> mismo nivel todavía; ver `rutas.go` y
> `internal/identidad/adaptadores/http/dtos.go` mientras tanto.

De los tres, **dos siguen públicos, sin token** (`POST /identidad/usuarios`
y `POST /identidad/autenticaciones` — son el paso previo a obtener uno) y
**sin rate limiting real** (el adaptador de Confianza es no-op hoy salvo
que `REDIS_URL` esté configurado, ver ADR 0018 más arriba). El tercero,
`GET /identidad/usuarios/{id}`, **ya no es público**: ver su sección más
abajo. Estos hechos quedan también en el campo `Metadata` (`x-auth-nivel`,
`x-rate-limit`) de cada operación en el OpenAPI generado — no son solo un
comentario de código, ver `rutas.go`.

Los ejemplos de request/response de abajo son los campos reales definidos
en `internal/identidad/adaptadores/http/dtos.go`, ejercitados por
`test/integracion/http_flujo_test.go` (que a su vez, según su propio
comentario de cabecera, verificó los status codes empíricamente con `curl`
contra el servidor real antes de escribir los tests).

### `POST /identidad/usuarios` — Registrar un usuario

Alta de usuario. Nace siempre en `pendiente_verificacion` (INV-ID-07).

Request:

```json
{
  "correo": "ana@ejemplo.com",
  "contrasena": "Integr4cion-Segura-Prueba!"
}
```

Response `200 OK`:

```json
{
  "id_usuario": "0192b1e2-....",
  "estado": "pendiente_verificacion",
  "requiere_verificacion_correo": true
}
```

Errores posibles:

| Status | Causa | Error de dominio |
|---|---|---|
| `422` | Correo con formato inválido | `ErrCorreoInvalido` |
| `422` | Contraseña débil (< 12 caracteres, contiene el correo, secuencia trivial, etc. — NIST SP 800-63B, ver ADR de política de contraseñas en el diseño) | `ErrContrasenaDebil` (incluye el detalle de qué regla falló) |
| `422` | Contraseña filtrada en brechas conocidas (HIBP) | `ErrContrasenaFiltrada` |
| `409` | El correo ya está registrado (deliberadamente **sí** se revela en registro, a diferencia de login — ver el comportamiento de `POST /identidad/autenticaciones` más abajo) | `ErrCorreoYaRegistrado` |
| `429` | Confianza bloqueó el intento (hoy nunca ocurre: el adaptador de Confianza es no-op) | `ErrAccesoDenegadoPorConfianza` |

`aud` específico: ninguno (no hay tokens todavía). Rate limit especial:
ninguno implementado (ver nota arriba).

**Puede estar detrás de una sala de espera.** Si un operador abrió una
cola de acceso virtual sobre esta ruta (`docs/design/colas-virtuales.md`,
extensión del contexto Confianza — el escenario típico es una apertura
masiva de inscripciones), una petición sin un ticket admitido recibe
`503` con `desenlace: ticket_requerido` en vez de llegar al registro — ver
`internal/confianza/README.md`.

### `POST /identidad/autenticaciones` — Verificar credenciales

**Importante — leer antes de integrar:** este endpoint **NO emite token ni
sesión, y sigue sin emitirlo hoy**. Es exactamente lo que dice ADR 0009:
Identidad verifica credenciales, y el contexto **Acceso** (implementado
desde ADR 0019/0020, ver `internal/acceso/README.md`) es quien llama a
este mismo caso de uso por puerto — vía su ACL
`internal/acceso/adaptadores/identidad/autenticador.go` — y, recién con el
resultado, emite el JWT. **El endpoint correcto para loguear desde un
frontend es `POST /acceso/sesiones`, no este.** Llamar directamente a este
endpoint de Identidad **solo sirve para validar que un correo+contraseña
son correctos** — no autentica una sesión de verdad, no hay cookie, no hay
`Authorization: Bearer` que emitir con la respuesta. Sigue existiendo y
siendo público porque Acceso lo consume por puerto Go, no por HTTP —
un frontend no debería llamarlo nunca directamente.

Request:

```json
{
  "correo": "ana@ejemplo.com",
  "contrasena": "Integr4cion-Segura-Prueba!"
}
```

Response `200 OK` (usuario `activo`, sin MFA):

```json
{
  "id_usuario": "0192b1e2-....",
  "correo_normalizado": "ana@ejemplo.com",
  "estado": "activo",
  "requiere_segundo_factor": false,
  "puntaje_confianza": 0
}
```

(`motivo_step_up` se omite del JSON cuando está vacío — tag `omitempty`.)

Errores posibles:

| Status | Causa | Error de dominio |
|---|---|---|
| `401` | **Correo inexistente O contraseña incorrecta** — mismo status, mismo cuerpo, mismo mensaje genérico `"credenciales inválidas"` | `ErrCredencialesInvalidas` |
| `403` | Contraseña correcta pero la cuenta sigue en `pendiente_verificacion` | `ErrCorreoNoVerificado` |
| `403` | Contraseña correcta pero la cuenta está `suspendido` | `ErrCuentaSuspendida` |
| `403` | Contraseña correcta pero la cuenta está `bloqueado` | `ErrCuentaBloqueada` |
| `429` | Confianza bloqueó el intento (hoy nunca ocurre: no-op) | `ErrAccesoDenegadoPorConfianza` |

**Comportamiento anti-timing-attack (INV-ID-11), a propósito, no un bug:**
un correo que no existe y una contraseña incorrecta contra una cuenta real
producen exactamente el mismo `401` con el mismo cuerpo JSON
(`{"detail": "credenciales inválidas", ...}`), y el caso de uso ejecuta un
hash Argon2id "señuelo" (`HasherContrasenas.ConsumirTiempoEquivalente`)
cuando el correo no existe, para que el tiempo de respuesta tampoco lo
delate. Esto está formalizado como test automatizado:
`TestHTTP_Login_ContrasenaIncorrectaYCorreoInexistente_MismoStatusYCuerpo`
en `test/integracion/http_flujo_test.go`. **No intentes distinguir "el
correo no existe" de "la contraseña está mal" en el cliente a partir de
esta respuesta — es intencionalmente indistinguible.**

**Gap del MVP original, cerrado desde entonces — dejado aquí como
histórico:** en el MVP de 3 endpoints documentado en detalle en esta
sección, un usuario recién registrado con `POST /identidad/usuarios`
quedaba en `pendiente_verificacion` sin forma de activarse por API y
**no podía loguear** (`403` con `ErrCorreoNoVerificado`). Eso ya no es
así: `POST /identidad/verificaciones-correo` y
`POST /identidad/verificaciones-correo/reenvios` existen (ver el aviso de
alcance al inicio de esta sección) e implementan exactamente el caso de
uso `VerificarCorreo` que estaba en backlog. Este README no los documenta
todavía con el mismo nivel de detalle que los 3 de abajo. Como alternativa
de solo-pruebas (la que usan hoy los tests de integración en vez de pasar
por el flujo HTTP completo), la activación directa en base de datos sigue
funcionando:

```sql
UPDATE usuarios SET estado = 'activo' WHERE id = '<id_usuario>';
```

(Así es como los propios tests de integración activan cuentas de prueba —
ver `activarUsuario` en `test/integracion/entorno_test.go`. No hay atajo
por API.)

`aud` específico: ninguno. Rate limit especial: ninguno implementado (el
adaptador de Confianza es no-op — ver `internal/identidad/adaptadores/confianza`).

### `GET /identidad/usuarios/{id}` — Consultar un usuario por ID

**Ya no es público.** Desde que el contexto Acceso quedó implementado
(ADR 0019/0020), este endpoint exige un token de acceso Bearer válido —
es exactamente el gancho que ADR 0019 §Consecuencias anunció: *"El
middleware que consume `ValidadorDeAccesos` es lo que cierra el hueco
documentado en `identidad/adaptadores/http/rutas.go`, donde
`GET /identidad/usuarios/{id}` está público 'como placeholder hasta que
exista el middleware de autenticación de Acceso'"*. El middleware que lo
cierra es
`internal/identidad/adaptadores/http/middleware_autenticacion.go`
(`middlewareAutenticacionAcceso`), que consume
`acceso/puertos.ValidadorDeAccesos` — el mismo puerto que el propio
middleware de Acceso usa para sus rutas Bearer (§2.4 del diseño de
Acceso). Obtén el token con `POST /acceso/sesiones` primero (ver
`internal/acceso/README.md`).

**Verificado en vivo contra el servidor real:** `GET
/identidad/usuarios/{id}` sin cabecera `Authorization` → `401`; con
`Authorization: Bearer <token_acceso>` válido → `200`.

Cabecera obligatoria: `Authorization: Bearer <token_acceso>`.

**Cambio de contrato (§11.1/§11.2 del diseño de Tenencia,
`docs/design/tenencia-bounded-context.md`):** el query param
`solicitante_id` **ya no tiene ningún efecto** — si lo envías, se ignora.
Quién pregunta (`IDSolicitante`) sale siempre del `sub` del token Bearer ya
validado (`middlewareAutenticacionAcceso` ahora publica el
`puertos.Acceso` resultante en el contexto, algo que antes descartaba
deliberadamente). Esto cierra exactamente el hueco que INV-TEN-12/
INV-ACC-23 prohíben: un cliente ya no puede afirmar ser cualquiera.

Regla de autorización (§11.2 del diseño de Tenencia):

- Si el `sub` del token coincide con el `{id}` de la ruta (consultar el
  propio perfil): **permitido**, sin consultar a Tenencia, y **no se
  audita** — igual que antes.
- Si difieren y la petición **no** trae `organizacion_id` en la query:
  **404** (no confirma que el `id` exista).
- Si difieren y la petición **sí** trae `organizacion_id`: se exige, vía
  `identidad/adaptadores/tenencia.AutorizadorConsultas` (el único paquete
  de Identidad autorizado a importar `tenencia/puertos`), que (a) el
  solicitante tenga `miembro.ver` en esa organización y (b) el objetivo sea
  miembro de la misma organización. Si cualquiera falla, o Tenencia no está
  disponible, **404**. Si ambas se cumplen, se audita `usuario.consultado`
  (consulta de un tercero).

Nuevo query param opcional `organizacion_id` (UUID): la organización desde
la que se consulta a un tercero — ver la regla de arriba. Irrelevante al
consultar el propio perfil.

Response `200 OK`:

```json
{
  "id": "0192b1e2-....",
  "correo": "ana@ejemplo.com",
  "estado": "activo",
  "tiene_mfa": false,
  "creado_en": "2026-08-30T12:00:00Z"
}
```

(`ultimo_acceso_en` se omite si es `null` — `omitempty`.)

**`tiene_mfa` ya refleja un valor real, desde el cierre de la extensión
OTP/MFA.** Antes de esa extensión siempre valía `false`: no existía
ningún caso de uso que lo activara. Hoy pasa a `true` exactamente en el
momento en que `POST /identidad/usuarios/actual/factores-mfa/confirmacion`
confirma el primer código TOTP (INV-ID-08 — el flag nunca se activa antes
de esa confirmación), y vuelve a `false` si el usuario deshabilita su
único factor (`DELETE /identidad/usuarios/actual/factores-mfa`). Ver la
sección de MFA más abajo.

Errores posibles:

| Status | Causa |
|---|---|
| `401` | Falta la cabecera `Authorization: Bearer <token>`, o el token es inválido/expirado/de una sesión revocada (mismo criterio de "un solo 401 genérico" que usa Acceso, ver `internal/acceso/README.md`) |
| `404` | El `id` es un UUID sintácticamente válido pero no existe ningún usuario con ese ID |
| `422` | El `id` no es un UUID válido (rechazado por la validación de formato de Huma antes de llegar al caso de uso) |

La **autorización** real ("¿puede este solicitante ver a este usuario?",
más allá de estar autenticado) **ya está cerrada** (§11.2 del diseño de
Tenencia): ver la regla completa más arriba. Un sujeto autenticado ya NO
puede consultar cualquier `id` a voluntad — solo el propio, o un tercero
con el que comparte una organización donde tiene `miembro.ver`.

`aud` específico: ninguno (mismo `ACCESO_AUDIENCIA` fijo que cualquier
token de acceso — ADR 0002, un solo producto). Rate limit especial:
ninguno implementado.

> `cmd/api/main.go` **siempre** pasa el validador real a
> `identidadhttp.RegistrarRutas`. El parámetro `validador` puede ir `nil`
> únicamente para no romper los tests de integración de este paquete que
> ejercitan Identidad de forma aislada sin montar Acceso
> (`test/integracion/entorno_test.go`): con `nil`, el endpoint se registra
> sin el middleware, queda público, y se emite un `slog.Warn` explícito
> ("NO USAR EN PRODUCCIÓN"). No es el camino real de arranque.

## MFA — segundo factor TOTP (autoservicio del propio sujeto)

Extensión sobre Identidad y Acceso cerrada en este hito
(`docs/design/otp-mfa.md`, ADR 0037-0040) — **no es un bounded context
nuevo**: MFA es una propiedad de la identidad del sujeto, no de su sesión
ni de su organización (§0.1 del diseño). Los tres endpoints exigen
**siempre** Bearer, a diferencia de los cuatro documentados arriba:
actúan sobre el propio sujeto autenticado, sin ID en la ruta ni en el
cuerpo (mismo criterio que `DELETE /acceso/sesiones/actual`). El paso que
**completa** el login cuando `POST /acceso/sesiones` exige un segundo
factor (`requiere_segundo_factor: true`, `motivo_step_up`) vive en
**Acceso**, no aquí — ver `POST /acceso/sesiones/segundo-factor` en
`internal/acceso/README.md`.

### `POST /identidad/usuarios/actual/factores-mfa` — Habilitar un segundo factor TOTP

Genera un `FactorMFA` sin confirmar y devuelve el secreto en claro + la
URI de provisionamiento (`otpauth://totp/...`) — la única vez que ambos
salen del proceso (INV-MFA-02). El cliente construye el QR (o muestra el
secreto como texto) y lo descarta: el servidor nunca vuelve a tener el
secreto en claro después de esta respuesta. **Verificado en vivo**: el
código TOTP calculado a partir de este secreto con una implementación
independiente de RFC 6238 (Python desde cero, sin bibliotecas) coincidió
exactamente con lo que el servidor esperó en la confirmación.

Request: sin cuerpo.

Response `200 OK`:

```json
{
  "id_factor": "0199...",
  "secreto_en_claro": "JBSWY3DPEHPK3PXP",
  "uri_provisionamiento": "otpauth://totp/Moterus:ana@ejemplo.com?secret=JBSWY3DPEHPK3PXP&issuer=Moterus"
}
```

Errores:

| Status | Causa | Error de dominio |
|---|---|---|
| `401` | Sin Bearer válido | — |
| `403` | La cuenta no está operativa (`Usuario.PuedeIniciarSesion` falla: correo no verificado / suspendida / bloqueada — mismos motivos que el login, reutilizados en vez de duplicar la máquina de estados) | `ErrCorreoNoVerificado` / `ErrCuentaSuspendida` / `ErrCuentaBloqueada` |
| `409` | Ya existe un factor confirmado y activo — uno por usuario en el MVP (ADR 0037); hay que deshabilitar el actual antes de habilitar uno nuevo | `ErrLimiteFactoresMFAExcedido` |

`aud` específico: ninguno (mismo token de acceso normal). Rate limit
especial: ninguno implementado todavía — a diferencia de
`verificar_otp` (ver `internal/acceso/README.md`), habilitar un factor no
es en sí mismo un oráculo de fuerza bruta.

### `POST /identidad/usuarios/actual/factores-mfa/confirmacion` — Confirmar el factor con el primer código

Verifica el primer código TOTP contra el factor recién habilitado. Si es
válido: activa `Usuario.tieneMFA` (INV-ID-08 — el flag se activa
exactamente aquí, nunca en el paso anterior), genera los 10 códigos de
respaldo y los devuelve en claro, la única vez que se muestran (ADR
0040). **Verificado en vivo**: la respuesta trajo exactamente 10 códigos.

Request:

```json
{ "id_factor": "0199...", "codigo": "123456" }
```

Response `200 OK`:

```json
{
  "codigos_respaldo": ["3K7H9D2F1A", "..."]
}
```

(10 elementos: alfanuméricos en mayúsculas, sin `0/O/1/I/L` para evitar
ambigüedad visual.)

Errores:

| Status | Causa | Error de dominio |
|---|---|---|
| `401` | Sin Bearer válido | — |
| `404` | `id_factor` no existe, o existe pero pertenece a otro usuario — **indistinguibles**, mismo criterio anti-enumeración que el resto del sistema | `ErrFactorMFANoEncontrado` |
| `409` | El factor ya estaba confirmado | `ErrFactorMFAYaConfirmado` |
| `422` | El código no coincide con el TOTP esperado | `ErrCodigoOTPInvalido` |

**Guarda los 10 códigos ahora**: no vuelven a estar disponibles en
ninguna respuesta futura; el servidor solo conserva su hash (SHA-256).

### `DELETE /identidad/usuarios/actual/factores-mfa` — Deshabilitar el segundo factor

Exige, además del Bearer, un código válido del propio factor (TOTP
vigente o de respaldo no usado) en el cuerpo — **no basta con estar
autenticado** (ADR 0039, INV-MFA-05): una sesión robada no debería poder
desarmar la protección que ella misma debería tener.

Request:

```json
{ "codigo": "123456" }
```

Response: `204 No Content`.

Errores:

| Status | Causa | Error de dominio |
|---|---|---|
| `401` | Sin Bearer válido | — |
| `422` | Código incorrecto, **o** el usuario no tiene ningún factor confirmado — mismo status y mismo mensaje genérico en ambos casos (INV-MFA-08: no se filtra si el problema es "no tenés MFA" o "el código está mal") | `ErrCodigoOTPInvalido` |

Un usuario que perdió tanto su dispositivo TOTP como sus 10 códigos de
respaldo **no puede autodeshabilitar MFA** por este endpoint — necesita
un canal de soporte humano (backlog, ver ADR 0039 §Consecuencias).

`aud`/rate limit: iguales a los dos endpoints anteriores.

**Gotcha real ya corregido, no un pendiente (commit `1e06146`):** el
diseño original de `FactorMFA` no distinguía "confirmado alguna vez" de
"vigente ahora mismo" — con un solo booleano `confirmado`, deshabilitar
MFA habría sido **irreversible**: `ContarConfirmadosDeUsuario` seguiría
contando el factor deshabilitado para siempre, bloqueando cualquier
`HabilitarMFA` posterior del mismo usuario. Se agregó el campo `activo`
(columna `factores_mfa.activo`, `db/migraciones/000015_crear_factores_mfa.up.sql`),
distinto de `confirmado`: `confirmado` es un hecho histórico que nunca
vuelve a `false`; `activo` sí puede volver a `false`
(`FactorMFA.Deshabilitar`) y a `true` de nuevo con un factor nuevo. El
repositorio y el índice único parcial (`factores_mfa_confirmado_activo_por_usuario_idx`)
filtran por ambas columnas — ver el comentario de cabecera de esa
migración para el razonamiento completo.

## Auditoría

Los tres endpoints del MVP (registro, autenticación en sus tres
desenlaces, y consulta por un tercero) emiten un evento a la bitácora
forense append-only con hash-chaining (ADR 0005), dentro de la misma
transacción que la escritura de negocio cuando aplica (INV-ID-15).
Catálogo completo de acciones: `docs/catalogos/acciones-auditoria.md`.

Los tres endpoints de MFA suman **5 acciones nuevas** al catálogo cerrado
(`usuario.mfa_habilitado`, `usuario.mfa_confirmado`,
`usuario.mfa_deshabilitado`, `usuario.codigo_respaldo_consumido`,
`usuario.otp_verificacion_fallida` — migración
`db/migraciones/000016_acciones_auditoria_mfa.up.sql`). `VerificarOTP`
(el camino caliente que **Acceso** consume, vía su ACL, en cada login con
step-up) audita únicamente los fallos (`VerificacionOTPFallida`) y el
consumo de un código de respaldo (`CodigoRespaldoConsumido`) — nunca un
"éxito" ad hoc de la verificación en sí: ese evento ya lo aporta Acceso
al completar el login (`sesion.iniciada`), mismo criterio de "no
dupliques la auditoría entre quien pregunta y quien decide" que usa
Tenencia con `AutorizacionDenegada`.

### Gotcha real: el catálogo cerrado de auditoría bloqueaba TODA operación de MFA con 500

Encontrado y corregido durante la verificación en vivo de este cierre
(servidor real, Postgres+Redis reales) — **no un pendiente**: el ACL de
auditoría de Identidad (`internal/identidad/adaptadores/auditoria/mapeo.go`)
no reconocía los 5 eventos de dominio nuevos de MFA
(`FactorMFAHabilitado`, `FactorMFAConfirmado`, `FactorMFADeshabilitado`,
`CodigoRespaldoConsumido`, `VerificacionOTPFallida`). Por INV-ID-15 (el
catálogo cerrado de auditoría aborta la transacción de negocio si no
reconoce el evento, en vez de degradar silenciosamente), **cualquier**
operación de MFA fallaba con `500` — incluida `HabilitarMFA`, la más
inofensiva de las tres. Ya corregido: `mapeo.go` reconoce los 5 casos.
Recordatorio concreto para el próximo evento de dominio que se agregue en
cualquier contexto de este sistema: hay que tocar **tres** lugares a la
vez (el evento en sí, la migración del catálogo, y el ACL de mapeo de
auditoría) — tocar solo los dos primeros deja el tercero silenciosamente
roto hasta que alguien lo ejercite en runtime.

## Referencias

- Diseño completo: `docs/design/identidad-bounded-context.md`
- ADR 0008 (Argon2id): `docs/adr/0008-argon2id-parametros.md`
- ADR 0009 (frontera Identidad/Acceso — por qué este contexto no emite
  tokens): `docs/adr/0009-frontera-identidad-acceso.md`
- ADR 0017 (rol de login de runtime): `docs/adr/0017-rol-login-runtime-vs-rol-dueno-migraciones.md`
- README del contexto Acceso (quien orquesta el login real y cierra el
  hueco de autenticación de este contexto): `internal/acceso/README.md`
- ADR 0019 (mecanismo de sesión de Acceso): `docs/adr/0019-mecanismo-sesion-jwt-refresco-rotatorio.md`
- ADR 0020 (algoritmo de firma y rotación de llaves de Acceso): `docs/adr/0020-algoritmo-firma-jwt-rotacion-llaves.md`
- README del contexto Tenencia (quien cierra la autorización de terceros de
  `GET /identidad/usuarios/{id}` vía `VerificadorDeAutorizacion`/
  `ConsultorDeMembresias`): `internal/tenencia/README.md`
- ADR 0029/0030/0031 (modelo de roles, autorización por consulta, RLS de
  Tenencia): `docs/adr/0029-modelo-roles-catalogo-cerrado.md`,
  `docs/adr/0030-autorizacion-por-consulta-en-cada-peticion.md`,
  `docs/adr/0031-rls-multi-tenant-guc-por-transaccion.md`
- Diseño de la extensión OTP/MFA (extiende Identidad y Acceso, no un
  contexto nuevo): `docs/design/otp-mfa.md`
- ADR 0037 (MFA del MVP limitado a TOTP): `docs/adr/0037-mfa-mvp-limitado-a-totp.md`
- ADR 0038 (token de step-up, emitido por Acceso): `docs/adr/0038-token-step-up-jwt-propio-ttl-corto.md`
- ADR 0039 (deshabilitar MFA exige código propio): `docs/adr/0039-deshabilitar-mfa-exige-codigo-propio.md`
- ADR 0040 (códigos de respaldo generados en la confirmación): `docs/adr/0040-codigos-respaldo-en-confirmacion.md`
- README de Acceso (completa el login con el segundo factor —
  `POST /acceso/sesiones/segundo-factor`, el token de step-up, el claim
  `amr` con `"otp"`): `internal/acceso/README.md`
- Diseño de colas de acceso virtual (extiende Confianza; puede proteger
  `POST /identidad/usuarios` con una sala de espera):
  `docs/design/colas-virtuales.md`
- README de Confianza: `internal/confianza/README.md`
- Índice completo de ADRs: `docs/adr/README.md`
