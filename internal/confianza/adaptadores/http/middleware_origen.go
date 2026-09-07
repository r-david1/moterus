package http

import (
	"context"

	"github.com/gofiber/fiber/v2"

	"github.com/r-david1/moterus/internal/confianza/dominio"
	"github.com/r-david1/moterus/internal/plataforma/ids"
)

// cabeceraIDSolicitud / cabeceraHuellaDispositivo: mismo criterio que
// identidad/adaptadores/http/middleware.go, acceso/adaptadores/http/middleware.go
// y tenencia/adaptadores/http/middleware_origen.go.
const cabeceraIDSolicitud = "X-Request-Id"
const cabeceraHuellaDispositivo = "X-Device-Fingerprint"

// cabeceraTicketCola es la cabecera por la que viaja el ticket de cola en
// las rutas públicas de sala de espera (§7.1 del diseño
// docs/design/colas-virtuales.md: "el ticket viaja en una cabecera, no en
// la ruta").
const cabeceraTicketCola = "X-Ticket-Cola"

// claveOrigenSolicitud es la clave no exportada bajo la que este paquete
// publica el dominio.OrigenSolicitud (tipo PROPIO de confianza/dominio,
// quinta duplicación deliberada del homónimo de Identidad/Acceso/Tenencia,
// §1.7 del diseño de Tenencia) en el context.Context de Fiber.
type claveOrigenSolicitud struct{}

// middlewareOrigenSolicitud construye el dominio.OrigenSolicitud de cada
// petición de Confianza. Duplicado a propósito frente a los homónimos de
// Identidad, Acceso y Tenencia: cada bounded context tiene su propio VO
// OrigenSolicitud.
func middlewareOrigenSolicitud(c *fiber.Ctx) error {
	idSolicitud := c.Get(cabeceraIDSolicitud)
	if idSolicitud == "" {
		generado, err := ids.GenerarUUIDv7()
		if err == nil {
			idSolicitud = generado
		}
	}
	c.Set(cabeceraIDSolicitud, idSolicitud)

	origen, err := dominio.NuevoOrigenSolicitud(
		c.IP(),
		c.Get(fiber.HeaderUserAgent),
		c.Get(cabeceraHuellaDispositivo),
		idSolicitud,
	)
	if err != nil {
		origen, _ = dominio.NuevoOrigenSolicitud("", c.Get(fiber.HeaderUserAgent), c.Get(cabeceraHuellaDispositivo), idSolicitud)
	}

	c.SetUserContext(context.WithValue(c.UserContext(), claveOrigenSolicitud{}, origen))
	return c.Next()
}

// origenSolicitudDesdeContexto recupera el OrigenSolicitud publicado por
// middlewareOrigenSolicitud. Si no está presente, devuelve el valor cero.
func origenSolicitudDesdeContexto(ctx context.Context) dominio.OrigenSolicitud {
	origen, _ := ctx.Value(claveOrigenSolicitud{}).(dominio.OrigenSolicitud)
	return origen
}
