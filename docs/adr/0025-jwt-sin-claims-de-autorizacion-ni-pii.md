# ADR 0025 — El token de acceso no lleva claims de autorización ni PII

## Contexto

Al diseñar el formato del JWT de acceso (`docs/design/acceso-bounded-context.md` §7), surgió la tentación obvia de meter en el token todo lo que un consumidor podría querer leer sin una consulta adicional: el correo del usuario, sus roles, el `org_id`/`tenant_id` activo, la huella de dispositivo, la IP. El diseño la nombró como candidata (0025) y la dejó explícitamente abierta.

`internal/acceso/adaptadores/jwt/firmador.go` (`Firmar`) construye el JWT con exactamente estos claims: `iss`, `sub`, `aud`, `jti`, `iat`, `nbf`, `exp` (RFC 7519), más cuatro privados — `sid`, `amr`, `auth_time`, `ver`. Ningún otro campo se agrega.

## Decisión

**El token de acceso nunca lleva `email`, `roles`, `permisos`, `org_id`, `tenant_id`, huella de dispositivo ni IP.** Solo transporta identidad del sujeto (`sub`), identidad de la sesión (`sid`), y metadatos temporales/de método de autenticación (`iat`/`nbf`/`exp`/`auth_time`/`amr`). Cualquier dato de autorización o de negocio se resuelve consultando al contexto dueño de esa información, con el `sub` ya validado como entrada.

Las razones se agrupan en tres:

1. **PII en un objeto que el cliente puede leer.** Un JWT no está cifrado — cualquiera que lo tenga (incluido el propio navegador, en `localStorage` o en memoria de JS) puede decodificar su payload. Meter el correo ahí es exponerlo a cualquier script con acceso al token, sin ninguna ganancia que compense ese riesgo.
2. **Autorización que no le pertenece a Acceso.** `roles`, `permisos`, `org_id`, `tenant_id` son territorio de Tenencia (ADR 0030: la autorización se resuelve por consulta indexada a Postgres en cada petición, no por claim ni por caché). Meter un `tenant_id` en el token de un sistema que en el momento de emitirlo puede no tener ningún contexto de organización activo es exactamente el "diseñar a medias" que ADR 0002 descartó al eliminar la capa de producto — y crea un claim que nadie sabría si es obligatorio ni cuándo refrescarlo si el usuario cambia de rol.
3. **Filtrar la señal de fingerprinting.** La huella de dispositivo y la IP alimentan el reconocimiento de origen de Confianza (ADR 0047-0051) precisamente porque el cliente no debe saber qué se está observando de él — publicarlas en un JWT que el propio cliente puede leer anularía ese propósito.

## Alternativas consideradas

- **Incluir `roles`/`org_id` para evitar una consulta a Tenencia en cada petición autorizada**: descartado — es la motivación central de ADR 0030 (consulta en cada petición, no caché ni claim), y meterlo en el JWT reintroduciría por la puerta de atrás el problema que ese ADR ya resolvió: un rol cambiado no tendría efecto hasta que el token expirara (hasta 10 minutos, `vidaTokenAcceso`), y revocar un permiso no sería inmediato.
- **Incluir `email` para que un consumidor no tenga que resolver `sub` contra Identidad**: descartado — cualquier consumidor legítimo que necesite el correo ya tiene autorización para preguntarle a Identidad por `sub`; exponerlo en el token solo agrega superficie de fuga sin ahorrar una llamada que de todos modos hay que poder hacer para otros datos del perfil.

## Consecuencias

- Todo consumidor de este servicio debe resolver rol/organización/correo consultando al contexto dueño (Tenencia, Identidad) con el `sub` que el token ya validó — nunca inferirlo del payload del JWT. `internal/acceso/README.md` ya lo documenta así para integradores ("si tu integración necesita saber el rol o la organización del usuario, ese dato no está en el JWT").
- Un cambio de rol o la suspensión de una cuenta no se refleja en un token de acceso ya emitido hasta que expira (máximo `vidaTokenAcceso`, 10 minutos) o hasta que la lista de revocación en Redis lo alcanza — el mismo trade-off que INV-ACC-15 ya documenta para la revocación en general, no uno nuevo que introduzca este ADR.
- Cualquier claim nuevo que alguien proponga agregar al token de acceso debe pasar esta misma prueba de tres preguntas (¿es PII?, ¿es autorización que pertenece a otro contexto?, ¿filtra una señal que debía quedar oculta al cliente?) antes de aceptarse.

## Estado

Aceptado (implementado de facto; documentado retroactivamente).
