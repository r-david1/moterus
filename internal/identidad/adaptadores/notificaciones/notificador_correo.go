// Package notificaciones implementa puertos.NotificadorCorreo. Vive como
// paquete propio (no dentro de adaptadores/eventos) porque "enviar un
// correo con un secreto de un solo uso" es una responsabilidad distinta de
// "publicar un evento de dominio best-effort": el receptor natural en
// producción (automatizacion-n8n) no es el mismo canal que
// PublicadorEventos, y el día que exista un envío real (SMTP, proveedor
// transaccional) tiene su propio ciclo de configuración/credenciales que no
// debe mezclarse con adaptadores/eventos.
package notificaciones

import (
	"context"
	"log/slog"

	"github.com/r-david1/moterus/internal/identidad/dominio"
	"github.com/r-david1/moterus/internal/identidad/puertos"
)

// NotificadorCorreoLog implementa puertos.NotificadorCorreo registrando el
// correo destino y el token en el logger estructurado, sin enviar ningún
// correo real. Mismo patrón que eventos.PublicadorLog y
// confianza.EvaluadorConfianzaNoOp: stub log-only con WARN explícito de que
// no hay integración real de envío de correo (sección 3.4 del diseño:
// "envío real del correo: fuera de alcance de este hito... es trabajo del
// agente automatizacion-n8n").
//
// Este es el ÚNICO lugar del código donde el token de verificación en claro
// puede aparecer en un log (INV-ID-21 lo permite explícitamente aquí: es
// dev-only y no hay canal real de envío). Ningún otro adaptador ni caso de
// uso debe volver a loguear el token.
type NotificadorCorreoLog struct {
	log *slog.Logger
}

var _ puertos.NotificadorCorreo = (*NotificadorCorreoLog)(nil)

// NuevoNotificadorCorreoLog construye el adaptador y emite inmediatamente el
// WARN de arranque (mismo patrón que confianza.NuevoEvaluadorConfianzaNoOp).
// log puede ser nil, en cuyo caso se usa slog.Default().
func NuevoNotificadorCorreoLog(log *slog.Logger) *NotificadorCorreoLog {
	if log == nil {
		log = slog.Default()
	}
	log.Warn("identidad/adaptadores/notificaciones: NotificadorCorreo en modo log-only — " +
		"NO hay integración real de envío de correo. El token de verificación solo queda en el log del servidor. " +
		"NO USAR EN PRODUCCIÓN. El envío real (SMTP/proveedor transaccional) es trabajo del agente automatizacion-n8n.")
	return &NotificadorCorreoLog{log: log}
}

// EnviarVerificacion registra el correo destino y el token de verificación
// en claro. No hay canal real de envío: esto es dev-only a propósito.
func (n *NotificadorCorreoLog) EnviarVerificacion(ctx context.Context, correo dominio.Correo, tokenPlano string) error {
	n.log.WarnContext(ctx, "notificador de correo en modo log-only: no se envió ningún correo real (ver WARN de arranque)",
		"correo_destino", correo.Normalizado(),
		"token_verificacion", tokenPlano,
	)
	return nil
}
