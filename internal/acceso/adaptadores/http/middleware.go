package http

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v2"

	"github.com/r-david1/moterus/internal/acceso/dominio"
	"github.com/r-david1/moterus/internal/acceso/puertos"
	"github.com/r-david1/moterus/internal/plataforma/ids"
)

// cabeceraIDSolicitud / cabeceraHuellaDispositivo: mismo criterio que
// identidad/adaptadores/http/middleware.go.
const cabeceraIDSolicitud = "X-Request-Id"
const cabeceraHuellaDispositivo = "X-Device-Fingerprint"

// claveOrigenSolicitud / claveAcceso son las claves no exportadas bajo las
// que este paquete publica, respectivamente, el dominio.OrigenSolicitud de
// cada petición y el puertos.Acceso ya validado, en el context.Context.
type claveOrigenSolicitud struct{}
type claveAcceso struct{}

// middlewareOrigenSolicitud construye el dominio.OrigenSolicitud de cada
// petición (mismo patrón que identidad/adaptadores/http/middleware.go,
// duplicado a propósito: son dos paquetes HTTP de dos bounded contexts
// distintos, cada uno con su propio VO OrigenSolicitud — §1.7 del diseño
// de Acceso).
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

// OrigenSolicitudDesdeContexto recupera el OrigenSolicitud publicado por
// middlewareOrigenSolicitud. Exportada porque el gancho de autenticación de
// otros contextos (p. ej. identidad/adaptadores/http) necesita construir un
// OrigenSolicitud propio a partir de la misma petición HTTP cuando aplica
// MiddlewareAutenticacion sin pasar antes por este middleware — ver el
// comentario de MiddlewareAutenticacion.
func OrigenSolicitudDesdeContexto(ctx context.Context) dominio.OrigenSolicitud {
	origen, _ := ctx.Value(claveOrigenSolicitud{}).(dominio.OrigenSolicitud)
	return origen
}

// AccesoDesdeContexto recupera el puertos.Acceso que MiddlewareAutenticacion
// publicó tras validar el token Bearer. El segundo valor es false si el
// handler se invocó sin pasar por ese middleware (nunca debería ocurrir en
// una ruta protegida; un handler que lo necesita y no lo encuentra es un
// error de programación, no una condición de negocio).
func AccesoDesdeContexto(ctx context.Context) (puertos.Acceso, bool) {
	acceso, ok := ctx.Value(claveAcceso{}).(puertos.Acceso)
	return acceso, ok
}

// cabeceraAutorizacion es el nombre estándar de la cabecera HTTP que
// transporta el token de acceso ("Authorization: Bearer <jwt>", §7 del
// diseño: "el token de acceso nunca viaja en cookie").
const cabeceraAutorizacion = "Authorization"
const prefijoBearer = "Bearer "

// MiddlewareAutenticacion construye el middleware Huma por operación
// (huma.Operation.Middlewares) que consume puertos.ValidadorDeAccesos para
// autenticar una petición Bearer. Es EL middleware que cierra el hueco
// documentado en identidad/adaptadores/http/rutas.go (GET
// /identidad/usuarios/{id} "público como placeholder hasta que exista el
// middleware de autenticación de Acceso" — ADR 0019 §Consecuencias).
//
// Se expone como una función de nivel de paquete (no un método sobre
// ManejadorAcceso) para que CUALQUIER contexto pueda aplicarla a sus
// propias operaciones Huma pasándole el puertos.ValidadorDeAccesos ya
// ensamblado — es exactamente el "puerto que Acceso EXPONE a otros
// contextos" de la tabla §2.4 del diseño. exigirSesionViva se propaga tal
// cual a ComandoValidarAcceso (§3.3 del diseño: false en el camino normal,
// cero consultas a Postgres; true para operaciones de alto valor donde una
// ventana de revocación de hasta 10 minutos no es aceptable).
func MiddlewareAutenticacion(api huma.API, validador puertos.ValidadorDeAccesos, exigirSesionViva bool) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		cabecera := ctx.Header(cabeceraAutorizacion)
		if !strings.HasPrefix(cabecera, prefijoBearer) {
			escribirErrorAutenticacion(api, ctx, http.StatusUnauthorized, "falta la cabecera Authorization: Bearer <token>")
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(cabecera, prefijoBearer))
		if token == "" {
			escribirErrorAutenticacion(api, ctx, http.StatusUnauthorized, "falta la cabecera Authorization: Bearer <token>")
			return
		}

		acceso, err := validador.Validar(ctx.Context(), puertos.ComandoValidarAcceso{
			TokenCompacto:    token,
			ExigirSesionViva: exigirSesionViva,
			Origen:           origenDesdeHuma(ctx),
		})
		if err != nil {
			estado, mensaje := estadoErrorValidacionAcceso(err)
			escribirErrorAutenticacion(api, ctx, estado, mensaje)
			return
		}

		next(huma.WithValue(ctx, claveAcceso{}, acceso))
	}
}

// origenDesdeHuma resuelve el OrigenSolicitud para el middleware de
// autenticación: si el proceso ya montó middlewareOrigenSolicitud a nivel
// Fiber (rutas.go de Acceso lo hace siempre), lo reutiliza; si no (p. ej.
// este middleware aplicado desde las rutas de OTRO contexto que no montó
// ese middleware Fiber), construye uno mínimo a partir de la propia
// petición Huma.
func origenDesdeHuma(ctx huma.Context) dominio.OrigenSolicitud {
	if origen := OrigenSolicitudDesdeContexto(ctx.Context()); !origen.IP().EsVacia() || origen.AgenteUsuario() != "" {
		return origen
	}
	origen, err := dominio.NuevoOrigenSolicitud(ctx.RemoteAddr(), ctx.Header(fiber.HeaderUserAgent), ctx.Header(cabeceraHuellaDispositivo), ctx.Header(cabeceraIDSolicitud))
	if err != nil {
		origen, _ = dominio.NuevoOrigenSolicitud("", ctx.Header(fiber.HeaderUserAgent), ctx.Header(cabeceraHuellaDispositivo), ctx.Header(cabeceraIDSolicitud))
	}
	return origen
}

// escribirErrorAutenticacion escribe una respuesta de error 401 con
// WWW-Authenticate: Bearer, terminando la cadena de middlewares (nunca
// llama a next).
func escribirErrorAutenticacion(api huma.API, ctx huma.Context, estado int, mensaje string) {
	ctx.SetHeader("WWW-Authenticate", `Bearer error="invalid_token"`)
	_ = huma.WriteErr(api, ctx, estado, mensaje)
}
