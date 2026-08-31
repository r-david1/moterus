# Agentes Claude Code — Auth-as-a-Service (Go, DDD + Hexagonal)

## 1. Instalación de los agentes

Copia esta carpeta completa a `.claude/agents/` en la raíz del repo del Auth-as-a-Service:

```bash
mkdir -p .claude/agents
cp agentes-claude-code/*.md .claude/agents/
```

Claude Code los detecta automáticamente. Para invocar uno directamente: `@orquestador-auth avanza con el flujo de login`. Para dejar que Claude elija el subagente correcto según la tarea, simplemente describe lo que necesitas — el campo `description` de cada agente es el que usa Claude Code para decidir a quién delegar.

## 2. Configuración de los MCPs externos (context7, LightRAG, n8n)

Estos **no son agentes** — son servidores MCP que le dan herramientas nuevas a los agentes de arriba. Se configuran en `.claude/settings.json` (o vía `claude mcp add`).

### context7 (documentación actualizada de librerías)

```bash
claude mcp add context7 -- npx -y @upstash/context7-mcp
```
Requiere Node disponible en el entorno. No necesita API key para uso básico (hay un tier con key para mayor límite de rate — opcional).

### LightRAG (conocimiento interno del proyecto)

LightRAG corre como servidor propio (no es un paquete npx público oficial) — dos opciones:

1. **Self-hosted**: despliega el servidor LightRAG (Python, `pip install lightrag-hku`) con su API expuesta, y conéctalo como MCP con un wrapper (`mcp-server-lightrag` si usas uno existente, o un adaptador propio mínimo que exponga `query`/`insert` como tools MCP).
2. Configúralo en `.claude/settings.json`:
```json
{
  "mcpServers": {
    "lightrag": {
      "command": "python",
      "args": ["-m", "lightrag_mcp_server", "--working-dir", "./docs/lightrag-index"]
    }
  }
}
```
Indexa inicialmente toda la carpeta `/docs/adr/` y este mismo README como punto de partida.

### n8n MCP

```bash
claude mcp add n8n -- npx -y n8n-mcp
```
Necesitas tu instancia n8n corriendo (self-hosted o cloud) y configurar `N8N_API_URL` y `N8N_API_KEY` como variables de entorno del servidor MCP.

Verifica que los tres quedaron activos con `claude mcp list` antes de empezar a trabajar.

## 3. Cómo fluye el trabajo

```
Usuario → orquestador-auth
              │
              ├─→ arquitecto-ddd-hexagonal (diseño)
              │        └─→ conocimiento-lightrag (verifica que no contradiga decisiones previas)
              │
              ├─→ go-dominio → go-aplicacion → go-infraestructura
              │        ├─→ investigacion-docs (context7, cuando hay duda de API/librería)
              │        └─→ auditoria-forense (todo caso de uso auditable — no es un paso final, se diseña junto con el caso de uso)
              │
              ├─→ base-datos (en paralelo con infraestructura — incluye auditoria desde el primer momento)
              │
              ├─→ seguridad-perimetral / fingerprinting-comportamiento / otp-mfa / colas-virtuales
              │        └─→ automatizacion-n8n (para envío real de OTP/alertas, incluyendo alertas de integridad de auditoría)
              │
              ├─→ tests-qa (obligatorio, siempre al cerrar — incluye verificación de la cadena de hashes)
              ├─→ documentacion (OpenAPI, ADR, guía de integración, catálogo de acciones auditables)
              ├─→ git-gitflow (rama, commits, PR)
              └─→ cicd (solo si se tocó pipeline o se agrega servicio)
```

## 4. Decisiones ya fijadas (no las reabras sin aprobación explícita del usuario)

- Alcance: un solo producto por ahora — sin capa multi-producto (sin tabla de productos, sin `aud` variable por producto).
- Lenguaje: **Go**, framework **Fiber v2**, DB **PostgreSQL** con `sqlc` + `pgx`, cache/colas **Redis**.
- Arquitectura: **Hexagonal + DDD** en español: bounded contexts *Identidad, Tenencia, Acceso, Confianza, Auditoría*; capas `dominio/aplicacion/puertos/adaptadores`.
- Base de datos: tablas en **español, snake_case, plural, sin prefijo, sin tildes** (`usuarios`, `organizaciones`...). Columnas en singular, FK como `tabla_id`, booleanos `es_`/`tiene_`.
- Modelo de tenancy: `usuarios` (global) → `membresias` → `organizaciones` — sin capa de producto.
- **Auditoría forense obligatoria**: toda acción sensible se registra en `auditoria`, append-only por permisos de rol (no por convención), con hash-chaining SHA-256 — diseño completo en el agente `auditoria-forense`. No es una fase posterior del proyecto, se construye junto con cada caso de uso desde Identidad.
- Captcha: **Cloudflare Turnstile** por defecto (reCAPTCHA v3 ya no es gratis sin límite — 10k evaluaciones/mes gratis, luego $8/mes; con un solo producto de tráfico bajo/medio, reCAPTCHA v3 gratis también es viable si se prefiere evitar una cuenta extra).
- Gitflow: sigue la convención ya establecida en la skill `gitflow` del proyecto Movilidad.
- Estado actual: contexto **Identidad** en construcción (entidad `Usuario`, value object `Correo`, casos de uso registrar/autenticar/obtener, con tests — pendiente: migración `usuarios`, adapter Postgres, adapter HTTP).

## 5. Próximo paso recomendado

Empieza por `arquitecto-ddd-hexagonal` sobre el bounded context **Identity** (usuarios + credenciales) — es la base de la que dependen Tenancy y Access. No arranques por Trust (rate limiting, captcha, fingerprinting) hasta tener el flujo de login básico funcionando end-to-end; si no, estás optimizando seguridad sobre un sistema que aún no existe.
