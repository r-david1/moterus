package notificaciones

import (
	"context"
	"fmt"
	"time"

	"github.com/r-david1/moterus/internal/plataforma/correo"
	"github.com/r-david1/moterus/internal/tenencia/dominio"
	"github.com/r-david1/moterus/internal/tenencia/puertos"
)

// NotificadorInvitacionesTransaccional implementa
// puertos.NotificadorInvitaciones enviando un correo real vía un
// correo.EnviadorCorreo (SMTP genérico hoy — ADR 0054/0055, el proveedor
// concreto es intercambiable detrás de la interfaz). Es la implementación
// de producción;
// NotificadorInvitacionesLog sigue existiendo como fallback de desarrollo
// cuando no hay ningún proveedor configurado — ver cmd/api/main.go.
type NotificadorInvitacionesTransaccional struct {
	enviador    correo.EnviadorCorreo
	urlFrontend string
}

var _ puertos.NotificadorInvitaciones = (*NotificadorInvitacionesTransaccional)(nil)

// NuevoNotificadorInvitacionesTransaccional construye el adaptador.
// urlFrontend puede ir vacío (ver EnviarInvitacion).
func NuevoNotificadorInvitacionesTransaccional(enviador correo.EnviadorCorreo, urlFrontend string) *NotificadorInvitacionesTransaccional {
	return &NotificadorInvitacionesTransaccional{enviador: enviador, urlFrontend: urlFrontend}
}

// EnviarInvitacion construye el correo de invitación y lo envía. Mismo
// criterio que NotificadorCorreoTransaccional.EnviarVerificacion sobre el
// enlace vs. token en claro: el token de invitación también es de alta
// entropía (INV-TEN-23), pensado para viajar en una URL.
func (n *NotificadorInvitacionesTransaccional) EnviarInvitacion(
	ctx context.Context,
	destinatario dominio.CorreoDestinatario,
	nombreOrganizacion string,
	rol dominio.Rol,
	tokenPlano string,
	expiraEn time.Time,
) error {
	var accion string
	if n.urlFrontend != "" {
		enlace := fmt.Sprintf("%s/invitaciones/aceptar?token=%s", n.urlFrontend, tokenPlano)
		accion = fmt.Sprintf("Aceptala acá:\n\n%s", enlace)
	} else {
		accion = fmt.Sprintf(
			"Tu token de invitación es:\n\n%s\n\n"+
				"Presentalo en POST /tenencia/invitaciones/aceptaciones para unirte.",
			tokenPlano,
		)
	}

	cuerpo := fmt.Sprintf(
		"Te invitaron a unirte a %q como %s.\n\n%s\n\n"+
			"Esta invitación vence el %s. Si no esperabas esta invitación, podés ignorar este mensaje.",
		nombreOrganizacion, rol.Valor(), accion, expiraEn.Format(time.RFC1123),
	)

	if err := n.enviador.Enviar(ctx, correo.Mensaje{
		Destinatario: destinatario.Normalizado(),
		Asunto:       fmt.Sprintf("Invitación a %s", nombreOrganizacion),
		TextoPlano:   cuerpo,
	}); err != nil {
		return fmt.Errorf("tenencia/adaptadores/notificaciones: no se pudo enviar el correo de invitación: %w", err)
	}
	return nil
}
