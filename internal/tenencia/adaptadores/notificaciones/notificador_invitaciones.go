// Package notificaciones implementa puertos.NotificadorInvitaciones. Vive
// como paquete propio (no dentro de adaptadores/eventos), mismo criterio
// que identidad/adaptadores/notificaciones: "enviar un correo con un
// secreto de un solo uso" es una responsabilidad distinta de "publicar un
// evento de dominio best-effort".
package notificaciones

import (
	"context"
	"log/slog"
	"time"

	"github.com/r-david1/moterus/internal/tenencia/dominio"
	"github.com/r-david1/moterus/internal/tenencia/puertos"
)

// NotificadorInvitacionesLog implementa puertos.NotificadorInvitaciones
// registrando el destinatario y el token EN CLARO en el logger
// estructurado, sin enviar ningún correo real. Mismo patrón que
// identidad/adaptadores/notificaciones.NotificadorCorreoLog: stub log-only
// con WARN explícito de arranque — el envío real es trabajo del agente
// automatizacion-n8n (§0 del diseño de Tenencia).
//
// Este es el ÚNICO lugar del código, junto con InvitarMiembroCasoDeUso
// (que se lo entrega una sola vez), donde el token de invitación en claro
// puede aparecer en un log (INV-TEN-23 lo permite explícitamente aquí:
// dev-only, sin canal real de envío). Ningún otro adaptador ni caso de uso
// debe volver a loguearlo.
type NotificadorInvitacionesLog struct {
	log *slog.Logger
}

var _ puertos.NotificadorInvitaciones = (*NotificadorInvitacionesLog)(nil)

// NuevoNotificadorInvitacionesLog construye el adaptador y emite
// inmediatamente el WARN de arranque. log puede ser nil (usa
// slog.Default()).
func NuevoNotificadorInvitacionesLog(log *slog.Logger) *NotificadorInvitacionesLog {
	if log == nil {
		log = slog.Default()
	}
	log.Warn("tenencia/adaptadores/notificaciones: NotificadorInvitaciones en modo log-only — " +
		"NO hay integración real de envío de correo. El token de invitación solo queda en el log del servidor. " +
		"NO USAR EN PRODUCCIÓN. El envío real (SMTP/proveedor transaccional) es trabajo del agente automatizacion-n8n.")
	return &NotificadorInvitacionesLog{log: log}
}

// EnviarInvitacion registra el destinatario, la organización, el rol
// propuesto y el token de invitación en claro. No hay canal real de envío:
// esto es dev-only a propósito.
func (n *NotificadorInvitacionesLog) EnviarInvitacion(
	ctx context.Context,
	destinatario dominio.CorreoDestinatario,
	nombreOrganizacion string,
	rol dominio.Rol,
	tokenPlano string,
	expiraEn time.Time,
) error {
	n.log.WarnContext(ctx, "notificador de invitaciones en modo log-only: no se envió ningún correo real (ver WARN de arranque)",
		"correo_destinatario", destinatario.Normalizado(),
		"organizacion", nombreOrganizacion,
		"rol_propuesto", rol.Valor(),
		"token_invitacion", tokenPlano,
		"expira_en", expiraEn,
	)
	return nil
}
