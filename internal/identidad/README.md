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
  **Acceso**, que **todavía no existe** en este repositorio (ver ADR 0009).
- Roles, organizaciones, membresías — contexto **Tenencia** (todavía no
  existe).
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

Ver `internal/identidad/puertos/entrada.go` para las firmas exactas. Cuando
el contexto **Acceso** se implemente, es el consumidor previsto de
`AutenticadorDeCredenciales` y `ConsultorDeUsuarios` (ver ADR 0009).

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

> **Nota de verificación honesta:** estas tres rutas son los valores por
> defecto documentados de `huma.DefaultConfig` (así lo asume ya ADR 0006,
> sección "Consecuencias": *"`/openapi.json` y la UI de docs en `/docs` se
> generan solos"*) y `rutas.go` no los sobreescribe. Sin embargo, este
> cierre de documentación **no pudo ejecutar `curl` contra el servicio real
> corriendo**: el entorno del agente de documentación en esta sesión no
> tenía una herramienta de shell disponible, solo lectura/escritura de
> archivos. La verificación aquí es estática (lectura de `rutas.go`,
> `dtos.go` y ADR 0006), no una comprobación en runtime. Antes de dar esto
> por definitivamente cerrado, alguien con acceso a shell debería correr:
>
> ```bash
> make docker-up && make migrate-up && make run &
> curl -s localhost:8080/openapi.json | jq '.paths | keys'
> curl -s -o /dev/null -w '%{http_code}\n' localhost:8080/docs
> ```
>
> y confirmar que `.paths` incluye `/identidad/usuarios`,
> `/identidad/autenticaciones` y `/identidad/usuarios/{id}`, y que `/docs`
> devuelve `200`.

## Los 3 endpoints (MVP)

Los tres son **públicos, sin token** (no existe todavía un emisor de
tokens — contexto Acceso) y **sin rate limiting real** (el adaptador de
Confianza es no-op hoy). Ambos hechos quedan también en el campo
`Metadata` (`x-auth-nivel`, `x-rate-limit`) de cada operación en el
OpenAPI generado — no son solo un comentario de código, ver `rutas.go`.

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

### `POST /identidad/autenticaciones` — Verificar credenciales

**Importante — leer antes de integrar:** este endpoint **NO emite token ni
sesión**. Es exactamente lo que dice ADR 0009: Identidad verifica
credenciales, el contexto **Acceso** (que no existe todavía en este
repositorio) es quien debería llamar a este mismo caso de uso por puerto y,
recién con el resultado, emitir un JWT. Hoy, llamar a este endpoint desde
un frontend **solo sirve para validar que un correo+contraseña son
correctos** — no autentica una sesión de verdad, no hay cookie, no hay
`Authorization: Bearer` que emitir con la respuesta.

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

**Gap conocido del MVP — no hay forma de activar una cuenta todavía:** un
usuario recién registrado con `POST /identidad/usuarios` queda en
`pendiente_verificacion` y **no puede loguear** (`403` con
`ErrCorreoNoVerificado`) hasta que alguien lo active manualmente. No existe
ningún endpoint `POST /identidad/verificaciones-correo` ni similar — el
caso de uso `VerificarCorreo` está en el backlog del diseño (sección 3.4)
pero no se implementó en este hito. Hoy, la única forma de pasar a `activo`
es una actualización directa en base de datos:

```sql
UPDATE usuarios SET estado = 'activo' WHERE id = '<id_usuario>';
```

(Así es como los propios tests de integración activan cuentas de prueba —
ver `activarUsuario` en `test/integracion/entorno_test.go`. No hay atajo
por API.)

`aud` específico: ninguno. Rate limit especial: ninguno implementado (el
adaptador de Confianza es no-op — ver `internal/identidad/adaptadores/confianza`).

### `GET /identidad/usuarios/{id}` — Consultar un usuario por ID

Query param opcional `solicitante_id`: identifica a quién pregunta, solo
para la auditoría condicional (consultar el perfil propio no se audita).
Vacío = llamada interna del sistema.

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

Errores posibles:

| Status | Causa |
|---|---|
| `404` | El `id` es un UUID sintácticamente válido pero no existe ningún usuario con ese ID |
| `422` | El `id` no es un UUID válido (rechazado por la validación de formato de Huma antes de llegar al caso de uso) |

**Este endpoint queda público a nivel de transporte a propósito, como
placeholder**, hasta que exista el middleware de autenticación del
contexto Acceso — la autorización real ("¿puede este solicitante ver a
este usuario?") es de Tenencia/Acceso, no de Identidad. **No debe
exponerse así en un despliegue real.**

`aud` específico: ninguno. Rate limit especial: ninguno implementado.

## Auditoría

Los tres endpoints (registro, autenticación en sus tres desenlaces, y
consulta por un tercero) emiten un evento a la bitácora forense
append-only con hash-chaining (ADR 0005), dentro de la misma transacción
que la escritura de negocio cuando aplica (INV-ID-15). Catálogo completo
de acciones: `docs/catalogos/acciones-auditoria.md`.

## Referencias

- Diseño completo: `docs/design/identidad-bounded-context.md`
- ADR 0008 (Argon2id): `docs/adr/0008-argon2id-parametros.md`
- ADR 0009 (frontera Identidad/Acceso — por qué este contexto no emite
  tokens): `docs/adr/0009-frontera-identidad-acceso.md`
- ADR 0017 (rol de login de runtime): `docs/adr/0017-rol-login-runtime-vs-rol-dueno-migraciones.md`
- Índice completo de ADRs: `docs/adr/README.md`
