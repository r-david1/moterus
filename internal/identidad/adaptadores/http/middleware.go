package http

import (
	"context"

	"github.com/gofiber/fiber/v2"

	"github.com/r-david1/moterus/internal/identidad/dominio"
	"github.com/r-david1/moterus/internal/plataforma/ids"
)

// cabeceraIDSolicitud es la cabecera estándar de correlación de solicitud.
// Si el cliente (o un proxy aguas arriba) ya la envía, se reutiliza; si no,
// se genera una nueva. Se refleja siempre en la respuesta para que el
// llamador pueda correlacionar sus propios logs con los del servicio.
const cabeceraIDSolicitud = "X-Request-Id"

// cabeceraHuellaDispositivo transporta la huella de fingerprinting del
// cliente cuando existe. El contexto Confianza (fingerprinting-comportamiento,
// fuera de alcance aquí) es quien la calcula típicamente en el cliente o en
// un middleware propio; este adaptador solo la propaga si ya viene.
const cabeceraHuellaDispositivo = "X-Device-Fingerprint"

// claveOrigenSolicitud es la clave no exportada bajo la que el middleware
// publica el dominio.OrigenSolicitud ya construido en el
// context.Context de Fiber (vía UserContext), para que los handlers Huma lo
// recuperen sin volver a tocar la petición Fiber cruda (que Huma no expone
// dentro de un handler de huma.Register, ver adaptadores/http/rutas.go).
type claveOrigenSolicitud struct{}

// middlewareOrigenSolicitud construye el dominio.OrigenSolicitud de cada
// petición (sección 6 del diseño: "el middleware de
// plataforma/servidor/middleware genera el id_solicitud y el handler HTTP
// construye el OrigenSolicitud"). Al no existir todavía un middleware
// genérico de id_solicitud en plataforma/servidor/middleware (paquete
// placeholder, fuera del alcance de este encargo), esta implementación
// resuelve ambas responsabilidades aquí, acotada a los tres endpoints de
// Identidad. Cuando otro contexto necesite la misma correlación, conviene
// extraer la resolución de id_solicitud a plataforma/servidor/middleware y
// dejar aquí solo la construcción de OrigenSolicitud.
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
		// Un c.IP() irreconocible (poco probable, pero defensivo) no debe
		// tumbar la petición: se continúa con un OrigenSolicitud sin IP en
		// vez de bloquear el flujo de negocio por un problema de
		// parseo del contexto forense.
		origen, _ = dominio.NuevoOrigenSolicitud("", c.Get(fiber.HeaderUserAgent), c.Get(cabeceraHuellaDispositivo), idSolicitud)
	}

	c.SetUserContext(context.WithValue(c.UserContext(), claveOrigenSolicitud{}, origen))
	return c.Next()
}

// origenSolicitudDesdeContexto recupera el OrigenSolicitud publicado por
// middlewareOrigenSolicitud. Si no está presente (p. ej. un test que invoca
// el handler sin pasar por el middleware), devuelve el valor cero: un
// OrigenSolicitud sin IP, sin huella y sin id_solicitud — nunca produce un
// error o un panic por un middleware ausente.
func origenSolicitudDesdeContexto(ctx context.Context) dominio.OrigenSolicitud {
	origen, _ := ctx.Value(claveOrigenSolicitud{}).(dominio.OrigenSolicitud)
	return origen
}
