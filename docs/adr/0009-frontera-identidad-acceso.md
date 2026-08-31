# ADR 0009 — Identidad autentica, Acceso emite tokens: `login` es una orquestación de Acceso

## Contexto

El caso de uso `AutenticarUsuario` de Identidad verifica credenciales, aplica el chequeo de Confianza y decide si hace falta step-up de MFA — pero alguien tiene que emitir el JWT/sesión resultante. Había que fijar en qué bounded context vive esa responsabilidad antes de que `go-aplicacion` terminara `ResultadoAutenticacion` y antes de implementar el flujo `login` HTTP.

## Decisión

**Identidad nunca emite tokens ni conoce el concepto de sesión.** `AutenticarUsuario` devuelve `ResultadoAutenticacion` (IDUsuario, estado, si requiere segundo factor y por qué, puntaje de confianza) — datos de negocio puros, sin JWT ni cookie ni refresh token.

El endpoint público `POST /login` (o como se llame en Acceso) es una **orquestación** que vive en el contexto **Acceso**: llama a `identidad.puertos.AutenticadorDeCredenciales.Autenticar`, y si el resultado es exitoso y no requiere step-up, recién ahí Acceso genera y firma el JWT, gestiona el refresh token y decide expiración/audiencia.

## Alternativas consideradas

- **Identidad emite el JWT directamente en `AutenticarUsuario`**: descartado — acoplaría el dominio de Identidad a JWT/JWKS/claims, que son conceptos de sesión/autorización, no de "quién es este usuario y son válidas sus credenciales". También impediría reusar `AutenticarUsuario` en flujos que verifican credenciales sin emitir sesión (p. ej. reautenticación para confirmar una operación sensible, "ingresa tu contraseña para continuar").
- **Fusionar Identidad y Acceso en un solo contexto**: descartado — la separación ya está fijada por el diseño de bounded contexts (`docs/design/identidad-bounded-context.md` sección 0); mezclar "quién eres" con "qué puedes hacer y con qué token" viola la separación de responsabilidades que motivó tener contextos separados desde el principio.

## Consecuencias

- `identidad/puertos.AutenticadorDeCredenciales` es exactamente el contrato que Acceso consume — si no existiera esta interfaz, Acceso terminaría importando el struct concreto de `identidad/aplicacion` y el acoplamiento entre contextos dejaría de ser inspeccionable (ya señalado en el comentario de `entrada.go`).
- `AutenticarUsuario` es reutilizable para reautenticación (step-up de una operación sensible) sin tocar nada de Acceso.
- El contexto Acceso, cuando se diseñe, es responsable de: emisión/firma JWT (`RS256`/`EdDSA`, JWKS con rotación de `kid`), refresh tokens, y la decisión final de qué devolver al cliente HTTP tras un login exitoso.

## Estado

Aceptado. Corresponde a INV-ID-14 del diseño de Identidad. Implementación del contexto Acceso todavía no iniciada.
