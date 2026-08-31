# ADR 0001 — Reconstrucción del sistema de auth en Go con arquitectura hexagonal + DDD

## Contexto

Se planteó llevar el sistema de autenticación a nivel "enterprise": multi-tenant, distribuido, con requisitos de seguridad avanzados (rate limiting, fingerprinting, OTP, colas virtuales, auditoría forense). Se evaluó en qué lenguaje y arquitectura construirlo desde cero.

## Decisión

Reconstruir el sistema desde cero en **Go**, con **arquitectura hexagonal + DDD**, usando **Fiber v2** como framework HTTP, **PostgreSQL + sqlc/pgx** como persistencia, y **Redis** para cache/rate limiting/colas.

## Alternativas consideradas

- **Lenguaje dinámico con framework tipo ASGI**: descartado — el análisis identificó que un sistema de auth de este nivel se beneficia de tipado fuerte en el límite del dominio y de la concurrencia nativa de Go para los componentes de alto tráfico (validación de JWT, rate limiting).
- **Framework frontend haciendo de capa de auth**: descartado — el frontend queda como consumidor del servicio de auth, no como el servicio en sí.
- **Frameworks Go alternativos (Echo, Gin, chi)**: se evaluó Fiber por rendimiento y ecosistema de middleware maduro; Echo quedó como alternativa válida si en el futuro se prioriza compatibilidad más estricta con `net/http`.
- **ORM tradicional (GORM)**: descartado a favor de `sqlc` + `pgx` — SQL tipado explícito, mejor control sobre las sentencias `SET LOCAL` que requiere RLS, sin la capa de abstracción de un ORM completo.

## Consecuencias

- Se requiere mantener manualmente la capa de mapeo dominio↔SQL (sqlc genera el código pero las queries se escriben a mano) — trade-off aceptado por el control que da sobre RLS y performance.
- El equipo de agentes de Claude Code se organiza por capa hexagonal (`dominio`, `aplicacion`, `puertos`, `adaptadores`) y no por tipo de archivo, para mantener el dominio libre de dependencias externas.

## Estado

Aceptado. Contexto **Identidad** ya iniciado sobre esta base (entidad `Usuario`, value object `Correo`, casos de uso registrar/autenticar/obtener, con tests unitarios).
