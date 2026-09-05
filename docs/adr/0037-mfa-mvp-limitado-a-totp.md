# ADR 0037 — MFA del MVP limitado a TOTP; email/SMS quedan en backlog

## Contexto

El boceto original del diseño de Identidad (`docs/design/identidad-bounded-context.md` §1.2) previó tres tipos de segundo factor: TOTP, email y SMS, con un agregado `CodigoOTP` pensado para códigos *enviados* con expiración corta. Al diseñar la extensión OTP/MFA (`docs/design/otp-mfa.md`) hay que decidir qué entra al primer hito, porque de eso depende si hace falta el agregado `CodigoOTP` desde el día uno o se puede diferir.

## Decisión

**El MVP implementa únicamente TOTP (RFC 6238).** Email y SMS quedan como candidatos explícitos, con sus puertos ya nombrados (`EmisorOTP`, interfaz `OTPSender`) y el agregado `CodigoOTP` declarado en la sección de puertos aplazados del diseño, para que nadie los reinvente con otro nombre cuando llegue el momento.

La diferencia técnica que motiva la decisión: **TOTP es *stateless* en el servidor**. El secreto compartido (guardado cifrado, una sola vez, al habilitar el factor) más el reloj del sistema bastan para computar el código esperado en cualquier momento — no hay nada que enviar ni que persistir en el camino caliente de cada verificación. Email y SMS, en cambio, son inherentemente *stateful*: alguien tiene que generar un código, enviarlo a través de un canal externo, y persistirlo con su expiración hasta que se consuma o caduque.

## Alternativas consideradas

- **Implementar los tres tipos desde el MVP**: descartada. Email y SMS exigen un proveedor transaccional real. Hoy el stack no tiene ninguno: `NotificadorCorreoLog` (Identidad) y `NotificadorInvitaciones` (Tenencia) son stubs *log-only* explícitamente marcados como no aptos para producción, con el envío real delegado al agente `automatizacion-n8n`, que tampoco está implementado. Construir un `EmisorOTP` sobre esa misma base sería fabricar un canal de segundo factor que no puede probarse de verdad — el código de un ataque de fuerza bruta terminaría en el log del servidor junto con el legítimo, sin ninguna garantía de entrega real.
- **Solo email (sin TOTP)**: descartada — además de heredar el problema anterior, sería el único factor del sistema sin ninguna dependencia externa (WebAuthn/llaves físicas comparten esa propiedad pero son una superficie de implementación mucho mayor), perdiendo la opción más simple, más barata de operar, y la que la mayoría de los usuarios avanzados ya prefiere.
- **Diseñar "a medias" (dejar el campo `tipo` con los tres valores pero solo implementar TOTP)**: descartada por el mismo criterio que ya fijó ADR 0002 para la capa de producto — un catálogo con valores no implementados genera ambigüedad sobre si el campo es requerido o no. El catálogo cerrado `TipoFactor` empieza con un único valor (`"totp"`) y crece de forma aditiva.

## Consecuencias

- `internal/identidad/dominio` no necesita el agregado `CodigoOTP` en este hito: `FactorMFA.VerificarCodigo` computa el TOTP esperado en el momento de la verificación, sin ninguna tabla de códigos pendientes.
- La migración `000015_crear_factores_mfa` no incluye una tabla de códigos enviados — solo `factores_mfa` y `codigos_respaldo_mfa` (los códigos de un solo uso que el propio sistema genera al confirmar el factor, no un OTP enviado por canal externo).
- Cuando se implemente email/SMS, el agregado `CodigoOTP` y el puerto `EmisorOTP` ya tienen nombre fijado — el trabajo futuro es aditivo, no una migración de lo que TOTP ya construyó.
- El usuario que pierde su dispositivo TOTP depende de los códigos de respaldo (ADR 0040) para recuperar acceso — no hay una vía de recuperación por email/SMS en este hito.

## Estado

Aceptado.
