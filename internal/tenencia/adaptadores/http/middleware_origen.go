package http

import (
	"context"

	"github.com/gofiber/fiber/v2"

	"github.com/r-david1/moterus/internal/plataforma/ids"
	"github.com/r-david1/moterus/internal/tenencia/dominio"
)

// cabeceraIDSolicitud / cabeceraHuellaDispositivo: mismo criterio que
// identidad/adaptadores/http/middleware.go y
// acceso/adaptadores/http/middleware.go.
const cabeceraIDSolicitud = "X-Request-Id"
const cabeceraHuellaDispositivo = "X-Device-Fingerprint"

// claveOrigenSolicitud es la clave no exportada bajo la que este paquete
// publica el dominio.OrigenSolicitud (tipo PROPIO de tenencia/dominio,
// §1.7 del diseño) en el context.Context de Fiber.
type claveOrigenSolicitud struct{}

// middlewareOrigenSolicitud construye el dominio.OrigenSolicitud de cada
// petición de Tenencia. Duplicado a propósito frente a los homónimos de
// Identidad y Acceso: cada bounded context tiene su propio VO
// OrigenSolicitud (§1.7 del diseño de Tenencia).
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
