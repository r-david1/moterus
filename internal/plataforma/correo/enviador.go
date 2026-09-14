// Package correo implementa el envío de correo transaccional real. Vive en
// plataforma, no en un bounded context, porque es kernel técnico puro —
// "mandar este asunto/cuerpo a esta dirección" no tiene ningún tipo de
// negocio involucrado, igual criterio que plataforma/cache para Redis o
// plataforma/bd para Postgres: "si dos contextos necesitan la misma pieza
// de infraestructura, eso no es una señal de que la frontera está mal
// trazada, es kernel técnico compartido" (§5.2 del diseño de Identidad).
//
// Cada bounded context que necesite enviar un correo real (Identidad:
// verificación; Tenencia: invitaciones) construye su propio mensaje
// (asunto, cuerpo, a partir de sus propios tipos de dominio) en su propio
// adaptador de notificaciones, y usa un EnviadorCorreo solo para el
// transporte — ningún tipo de dominio de Identidad o Tenencia aparece en
// este paquete.
//
// Única implementación de EnviadorCorreo hoy: ClienteSMTP, contra
// cualquier servidor SMTP con STARTTLS+AUTH (Gmail, Workspace, un
// proveedor propio) — elegida sobre un proveedor de API transaccional
// (Resend, evaluado y descartado el mismo día: ver ADR 0055) porque no
// exige verificar un dominio propio, que este proyecto no tiene. El
// puerto queda igual de agnóstico de proveedor que Turnstile/reCAPTCHA
// para captcha (ADR 0003): si en el futuro hace falta un proveedor de API
// (Resend u otro), se construye un tipo nuevo con este mismo contrato,
// sin tocar los adaptadores de Identidad/Tenencia — no se mantiene código
// de un proveedor sin usar mientras tanto.
package correo

import "context"

// Mensaje es el contenido de un correo a enviar, agnóstico de quién lo
// pide y de qué proveedor lo transporta. TextoPlano es obligatorio; HTML
// es opcional (un cliente de correo sin soporte HTML cae a TextoPlano).
type Mensaje struct {
	Destinatario string
	Asunto       string
	TextoPlano   string
	HTML         string
}

// EnviadorCorreo es el contrato mínimo que necesita un adaptador de
// notificaciones de cualquier bounded context (identidad/adaptadores/
// notificaciones, tenencia/adaptadores/notificaciones) para enviar un
// correo real, sin conocer el proveedor concreto detrás.
type EnviadorCorreo interface {
	Enviar(ctx context.Context, m Mensaje) error
}
