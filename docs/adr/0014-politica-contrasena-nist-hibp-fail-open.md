# ADR 0014 — Política de contraseñas alineada a NIST SP 800-63B, con verificación de brechas fail-open

## Contexto

Toda cuenta necesita una política de contraseñas. La convención más extendida en formularios web ("mínimo 8 caracteres, una mayúscula, un número, un símbolo") es también la que la guía moderna de identidad digital de NIST (SP 800-63B) explícitamente desaconseja: obliga a los usuarios a patrones predecibles (`Contraseña1!`) y no correlaciona con contraseñas más difíciles de adivinar. Identidad necesitaba fijar qué política aplicar antes de escribir el primer caso de uso de registro.

`docs/design/identidad-bounded-context.md` §7 nombró esta decisión como candidata (0014); se implementó de facto (`internal/identidad/dominio/politica_contrasena.go`) sin disputarse durante la implementación.

## Decisión

**La política de contraseñas sigue NIST SP 800-63B**: longitud mínima de 12 caracteres, máxima evaluada de 128, **sin ninguna regla de composición** (no exige mayúsculas, números ni símbolos), sin expiración forzada. Rechaza contraseñas que contengan el correo completo o el dominio del correo del propio usuario, y rechaza un catálogo mínimo de secuencias triviales (`"12345678"`, `"password"`, `"contraseña"`, caracteres repetidos, corridas numéricas consecutivas de 6+ dígitos) — no como sustituto de una verificación de brechas reales, sino como filtro barato de los casos más obvios.

**La verificación contra brechas conocidas (HaveIBeenPwned, vía `puertos.VerificadorContrasenasFiltradas`) es fail-open deliberado**: si el servicio externo no responde, el registro/cambio de contraseña continúa igual, con un `slog.Warn` explícito (`registrar_usuario.go`: "una caída del [servicio] no puede bloquear altas"). Es la misma asimetría fail-open que ADR 0018 fijaría después para el rate limiting de Confianza, aplicada acá a una dependencia de red distinta y anterior en el tiempo.

## Alternativas consideradas

- **Reglas de composición tradicionales (mayúscula+número+símbolo)**: descartadas explícitamente por seguir la recomendación NIST — rompen con la expectativa habitual de un usuario, lo cual el propio diseño anticipó como algo que "alguien va a cuestionar", y por eso se documenta acá el motivo.
- **Expiración forzada de contraseña cada N días**: descartada — NIST SP 800-63B también la desaconseja (empuja a los usuarios a incrementar la contraseña anterior de forma predecible, `Contraseña1` → `Contraseña2`).
- **HIBP fail-closed** (bloquear el registro si el servicio de brechas no responde): descartado — convertiría una dependencia de terceros no crítica para la seguridad del sistema (es una capa adicional, no la única defensa) en un punto único de falla para toda alta de usuario.

## Consecuencias

- `PoliticaContrasena.Evaluar` es un servicio de dominio puro, sin estado ni E/S — se prueba exhaustivamente sin mocks (`politica_contrasena_test.go`).
- `VerificadorContrasenasFiltradas` es un puerto de salida separado, precisamente para que la política de dominio (síncrona, sin red) y la verificación de brechas (asíncrona respecto a la red, puede fallar) tengan ciclos de vida y pruebas independientes.
- Cualquier futuro ajuste a la política (p. ej. subir el mínimo, agregar un catálogo de secuencias más grande) se hace en este único archivo de dominio, sin tocar la capa de aplicación ni el puerto.

## Estado

Aceptado (implementado de facto; documentado retroactivamente).
