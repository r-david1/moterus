# ADRs — decisiones de arquitectura ya tomadas

Estos archivos van en `/docs/adr/` dentro del repo del sistema de auth (no en `.claude/agents/`). Documentan el análisis y las alternativas descartadas detrás de las decisiones que los agentes ya asumen como fijas — así Claude Code (o cualquiera del equipo) puede consultar *por qué* antes de reabrir una decisión, en vez de que la razón viva solo en el historial de chat.

- **0001** — Reconstrucción en Go + Fiber + hexagonal/DDD (vs. el prototipo original en Next.js + FastAPI + Supabase).
- **0002** — Alcance de un solo producto por ahora (multi-producto pausado, no descartado).
- **0003** — Captcha invisible: Cloudflare Turnstile por defecto vs. reCAPTCHA v3 (con los números de precio actuales de Google).
- **0004** — Convención de nombres de tablas: español, sin prefijo (incluye la reversión documentada del prefijo `mot_` que se probó primero).
- **0005** — Auditoría forense: hash-chaining SHA-256 + append-only por permisos de rol.
- **0006** — Huma v2 sobre Fiber para generación automática de OpenAPI 3.1 y validación (reemplaza `swaggo/swag` y `go-playground/validator` para operaciones registradas en Huma).
- **0007** — Idioma de los identificadores de código: español en dominio/aplicación/puertos, inglés solo donde Go lo impone.
- **0008** — Argon2id como algoritmo de hashing de contraseñas, con parámetros fijos (64 MiB, t=3, p=1) y ruta de rehash oportunista.
- **0009** — Identidad autentica, Acceso emite tokens: `login` es una orquestación de Acceso, no un caso de uso de Identidad.
- **0017** — El proceso `api` corre con un rol de login de privilegios acotados (`rol_login_identidad`), distinto del rol dueño usado para migraciones — cierra un hueco donde el servicio corría como superusuario y el `REVOKE UPDATE/DELETE` de ADR 0005 quedaba sin efecto. (Numerado 0017, no 0010, para no colisionar con los "ADR candidato" 0010-0016 ya reservados en `docs/design/identidad-bounded-context.md` sección 7.)
- **0018** — Rate limiting (por IP y por cuenta) y captcha invisible viven detrás de `puertos.EvaluadorConfianza`/el bounded context Confianza, respaldados por Redis (no Postgres): implementa el motor real que sustituye a `EvaluadorConfianzaNoOp`, con Cloudflare Turnstile (ADR 0003) como proveedor de captcha. (Numerado 0018, siguiente libre tras 0017, para no colisionar con los candidatos 0010-0016 todavía sin materializar.)
- **0019** — Mecanismo de sesión del contexto Acceso: JWT de acceso de vida corta (10 min, firma asimétrica + JWKS) sobre una sesión autoritativa en Postgres, con token de refresco opaco rotatorio de un solo uso y detección de reuso. Fija los parámetros que ADR 0009 dejó abiertos. (Numerado 0019, siguiente libre tras 0018; los candidatos 0010-0016 siguen reservados para Identidad y 0020-0028 quedan reservados para Acceso en `docs/design/acceso-bounded-context.md` sección 9.)
- **0020** — Algoritmo de firma del token de acceso (EdDSA/Ed25519 primario, RS256 soportado por el verificador) sobre `github.com/lestrrat-go/jwx/v2`, formato de `kid` (thumbprint RFC 7638) y rotación de llaves por despliegue (fase 1: configuración, sin tabla propia todavía). Cierra el candidato que ADR 0019 dejó abierto.

El agente `documentacion` es responsable de mantener este índice actualizado y de crear el ADR correspondiente cada vez que se cierre una decisión de arquitectura no obvia — a partir de ahora, no solo dejarlo mencionado en la descripción del agente.
