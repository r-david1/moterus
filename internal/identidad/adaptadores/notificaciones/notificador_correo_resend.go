package notificaciones

import (
	"context"
	"fmt"

	"github.com/r-david1/moterus/internal/identidad/dominio"
	"github.com/r-david1/moterus/internal/identidad/puertos"
	"github.com/r-david1/moterus/internal/plataforma/correo"
)

// NotificadorCorreoResend implementa puertos.NotificadorCorreo enviando un
// correo real vía plataforma/correo.ClienteResend (ADR 0054). Es la
// implementación de producción; NotificadorCorreoLog sigue existiendo como
// fallback de desarrollo cuando RESEND_API_KEY no está configurada — ver
// cmd/api/main.go.
type NotificadorCorreoResend struct {
	cliente     *correo.ClienteResend
	urlFrontend string
}

var _ puertos.NotificadorCorreo = (*NotificadorCorreoResend)(nil)

// NuevoNotificadorCorreoResend construye el adaptador. urlFrontend puede
// ir vacío (ver EnviarVerificacion).
func NuevoNotificadorCorreoResend(cliente *correo.ClienteResend, urlFrontend string) *NotificadorCorreoResend {
	return &NotificadorCorreoResend{cliente: cliente, urlFrontend: urlFrontend}
}

// EnviarVerificacion construye el correo de verificación y lo envía. El
// token es de alta entropía (32 bytes base64url, cripto.GeneradorTokens) —
// pensado para viajar en una URL, no para que un humano lo transcriba —
// así que si urlFrontend está configurada, el correo lleva un enlace
// clicable; si no (no existe frontend propio todavía, ADR 0002), el correo
// presenta el token en claro con instrucciones para pasarlo directamente a
// POST /identidad/verificaciones-correo.
func (n *NotificadorCorreoResend) EnviarVerificacion(ctx context.Context, correoDestino dominio.Correo, tokenPlano string) error {
	var cuerpo string
	if n.urlFrontend != "" {
		enlace := fmt.Sprintf("%s/verificar-correo?token=%s", n.urlFrontend, tokenPlano)
		cuerpo = fmt.Sprintf(
			"Confirmá tu correo electrónico para activar tu cuenta:\n\n%s\n\n"+
				"Si no creaste esta cuenta, podés ignorar este mensaje.",
			enlace,
		)
	} else {
		cuerpo = fmt.Sprintf(
			"Tu código de verificación es:\n\n%s\n\n"+
				"Presentalo en POST /identidad/verificaciones-correo para activar tu cuenta.\n\n"+
				"Si no creaste esta cuenta, podés ignorar este mensaje.",
			tokenPlano,
		)
	}

	if err := n.cliente.Enviar(ctx, correo.Mensaje{
		Destinatario: correoDestino.Normalizado(),
		Asunto:       "Confirmá tu correo",
		TextoPlano:   cuerpo,
	}); err != nil {
		return fmt.Errorf("identidad/adaptadores/notificaciones: no se pudo enviar el correo de verificación: %w", err)
	}
	return nil
}
