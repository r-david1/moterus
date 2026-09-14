# ADR 0055 — SMTP genérico en vez de Resend: no hay dominio propio que verificar

## Contexto

ADR 0054 decidió el envío real de correo directo desde Go, sin n8n, contra Resend (API HTTP transaccional). El mismo día, al preparar la cuenta real, surgió una restricción que ADR 0054 no había puesto sobre la mesa: **este proyecto no tiene un dominio propio, y no va a tener uno**. Resend —como todo proveedor de API transaccional serio (SendGrid, Postmark, Mailgun)— exige verificar un dominio propio (registros DNS SPF/DKIM) para poder enviar a destinatarios arbitrarios; sin uno, el remitente de prueba que ofrecen (`onboarding@resend.dev`) solo entrega al correo con el que se creó la cuenta de Resend, nunca a un usuario real. Eso deja exactamente el mismo problema que este trabajo buscaba cerrar: ningún usuario real recibiría su correo de verificación.

## Decisión

**El envío real de correo usa SMTP genérico (`internal/plataforma/correo.ClienteSMTP`, stdlib `net/smtp` + `crypto/tls`, sin biblioteca de terceros) en vez de un proveedor de API transaccional.** Una cuenta de correo existente (Gmail con una "contraseña de aplicación", Google Workspace, un proveedor propio) puede mandar a cualquier destinatario real sin verificar ningún dominio — el límite pasa a ser de volumen/reputación del remitente, no de a quién puede llegarle el correo.

**El código de Resend se elimina, no se deja sin usar.** `internal/plataforma/correo/cliente_resend.go` (el cliente HTTP) y `notificador_correo_resend.go`/`notificador_invitaciones_resend.go` (los adaptadores por contexto) se borraron del repositorio — no quedan como código muerto "por si algún día hay dominio". Si en el futuro aparece un dominio propio y vale la pena volver a un proveedor de API (mejor entregabilidad, métricas de apertura, etc.), se construye de nuevo: es un adaptador de ~150 líneas detrás de una interfaz ya existente (`correo.EnviadorCorreo`), no una pieza cara de reconstruir.

`identidad/adaptadores/notificaciones.NotificadorCorreoTransaccional` y `tenencia/adaptadores/notificaciones.NotificadorInvitacionesTransaccional` (renombrados desde `...Resend`, ya no llevan el nombre de un proveedor específico) no cambiaron de lógica: siguen recibiendo un `correo.EnviadorCorreo` por interfaz, nunca un tipo concreto — el cambio de proveedor fue posible sin tocarlos, exactamente la garantía que ADR 0054 ya buscaba con esa separación puerto/adaptador.

## Alternativas consideradas

- **Conseguir un dominio barato solo para el remitente** (~$10-15 USD/año, discutido explícitamente): descartado por decisión del usuario del proyecto — no van a tener dominio, ni siquiera uno auxiliar solo para esto.
- **Mantener Resend en modo sandbox** (`onboarding@resend.dev`), aceptando que solo el desarrollador recibe correos reales: descartado — no resuelve el problema que motivó todo este trabajo (que un usuario real complete el flujo), solo lo pospone detrás de un proveedor configurado que igual no sirve para producción.
- **Dejar el adaptador de Resend en el código, sin usar, detrás de una variable de entorno que nunca se define**: descartado explícitamente por el usuario del proyecto — "no quiero en código cosas que no se usen". Es la misma disciplina que ya aplicó este proyecto en otros lados (no dejar un puerto "por si acaso" sin un consumidor real).

## Consecuencias

- `RESEND_API_KEY`/`RESEND_REMITENTE` desaparecen de `configuracion.Config` y de cualquier checklist de producción; los reemplazan `SMTP_HOST`, `SMTP_PUERTO` (default `587`), `SMTP_USUARIO`, `SMTP_CONTRASENA`, `SMTP_REMITENTE`. El fail-fast en `APP_ENV=production` sigue vigente (ADR 0054), ahora sobre estas variables.
- La entregabilidad de una cuenta SMTP personal/de Workspace es peor que la de un proveedor transaccional dedicado a mediano-largo plazo (mayor probabilidad de terminar en spam a volumen alto, límites de envíos diarios más bajos, sin métricas de apertura/rebote) — aceptado como el costo de no depender de un dominio propio. Si el volumen de correo crece lo suficiente para que esto importe, es la señal concreta de que vale la pena reconsiderar un proveedor de API (con dominio propio en ese momento).
- `ClienteSMTP.Enviar` abre una conexión TCP nueva por cada correo (sin pool de conexiones SMTP): correcto para el volumen de este proyecto (un correo por registro/invitación, nunca en el camino caliente) — no se optimiza una ruta que no es un cuello de botella real.

## Estado

Aceptado.
