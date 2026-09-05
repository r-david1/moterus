# ADR 0039 — Deshabilitar MFA exige el propio código, no basta con estar autenticado

## Contexto

El diseño de OTP/MFA (`docs/design/otp-mfa.md` §3.3) necesita fijar qué exige `DeshabilitarMFA`. La opción más simple — cualquier sujeto con un token de acceso Bearer válido puede desactivar su propio segundo factor — es también la que menos protege exactamente al usuario que ya decidió reforzar su cuenta.

## Decisión

**`DeshabilitarMFA` exige, además de un token de acceso Bearer válido, un código correcto (TOTP vigente o un código de respaldo no usado) del factor que se está desactivando.**

El razonamiento: MFA existe para que una sesión comprometida (token de acceso robado, dispositivo desatendido, XSS que exfiltra un Bearer) no baste para tomar control total de la cuenta. Si desactivar MFA solo exigiera el token de acceso normal, un atacante con exactamente ese token comprometido podría desarmar la protección que el propio usuario activó para mitigar justo ese escenario — MFA dejaría de proteger nada en el momento en que más importa.

## Alternativas consideradas

- **Solo el token de acceso Bearer (sin código adicional)**: descartada por el argumento de arriba — es la opción que hace que MFA no proteja contra su propia amenaza principal (sesión comprometida).
- **Exigir la contraseña actual, no el código MFA**: descartada para este hito — `CambiarContrasena` (que sí exige la contraseña actual) ya está en el backlog de Identidad como caso de uso separado, y mezclar "verificar contraseña" con el flujo de MFA acoplaría dos mecanismos de reautenticación distintos sin necesidad. El código del propio factor que se está desactivando es la prueba más directa y específica: "todavía controlo este segundo factor, así que puedo autorizar su baja".
- **Reautenticación completa (logout + login de nuevo)**: descartada por fricción excesiva sin beneficio de seguridad adicional sobre exigir el código — un atacante con el token de acceso robado tampoco tiene la contraseña, así que ambas opciones lo bloquean igual; la reautenticación completa solo añade pasos al usuario legítimo.

## Consecuencias

- `ComandoDeshabilitarMFA` lleva un campo `Codigo` obligatorio, verificado contra el `FactorMFA` confirmado del sujeto antes de eliminar nada.
- Un usuario que perdió tanto su dispositivo TOTP como todos sus códigos de respaldo **no puede autodeshabilitar MFA** por este endpoint — necesita un canal de soporte humano (fuera del alcance de este documento, backlog).
- `DELETE /identidad/usuarios/actual/factores-mfa` exige un cuerpo con el código, no solo la cabecera `Authorization`.

## Estado

Aceptado.
