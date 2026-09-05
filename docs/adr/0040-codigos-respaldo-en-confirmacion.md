# ADR 0040 — Códigos de respaldo generados en la confirmación, no en la habilitación

## Contexto

El diseño de OTP/MFA (`docs/design/otp-mfa.md` §3.1-3.2) separa `HabilitarMFA` (genera el secreto TOTP, crea un `FactorMFA` sin confirmar) de `ConfirmarFactorMFA` (verifica el primer código y activa `tieneMFA`). Hay que decidir en cuál de los dos pasos se generan los 10 códigos de respaldo de un solo uso.

## Decisión

**Los códigos de respaldo se generan en `ConfirmarFactorMFA`, junto con la activación de `tieneMFA`, nunca antes.**

## Alternativas consideradas

- **Generarlos en `HabilitarMFA`, junto con el secreto**: descartada. Un usuario puede iniciar `HabilitarMFA` (por ejemplo, para ver cómo es el flujo) y nunca escanear el QR ni confirmar — el `FactorMFA` queda sin confirmar indefinidamente y, con esta alternativa, existirían códigos de respaldo válidos para un segundo factor que nunca llegó a activarse. Eso es un conjunto de credenciales de recuperación vivas sin nada que recuperar: superficie de ataque sin ningún beneficio.
- **Generarlos bajo demanda, en un endpoint separado, en cualquier momento después de confirmar**: descartada para el MVP — añade un endpoint y un caso de uso más (`RegenerarCodigosRespaldo`) sin que exista todavía una necesidad concreta que lo justifique (el criterio de ADR 0002 aplica igual aquí: no diseñar para una necesidad hipotética). Queda en backlog si algún usuario reporta haber perdido sus 10 códigos sin haber perdido el TOTP.

## Consecuencias

- `ResultadoConfirmarMFA` es el único lugar del sistema que devuelve códigos de respaldo en claro — una sola vez, en la misma respuesta que confirma el factor.
- Si el usuario pierde esos 10 códigos sin haberlos guardado, no hay forma de volver a verlos: tendría que deshabilitar el factor (con un código TOTP todavía válido) y volver a habilitarlo desde cero, generando un secreto y una tanda de códigos nuevos.
- La tabla `codigos_respaldo_mfa` siempre tiene exactamente 10 filas por `FactorMFA` confirmado, nunca 0 (no existe un `FactorMFA` confirmado sin sus códigos de respaldo ya generados en la misma transacción).

## Estado

Aceptado.
