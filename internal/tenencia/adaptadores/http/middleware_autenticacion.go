package http

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v2"

	accesodominio "github.com/r-david1/moterus/internal/acceso/dominio"
	accesopuertos "github.com/r-david1/moterus/internal/acceso/puertos"
)

// Este archivo es el precedente EXACTO de
// identidad/adaptadores/http/middleware_autenticacion.go, con una única
// diferencia deliberada (§11.1 del diseño de Tenencia): SÍ publica el
// puertos.Acceso resultante en el context.Context, porque todos los casos
// de uso de Tenencia necesitan el `sub` del token para construir el
// IDSujeto de cada comando (INV-TEN-12) — a diferencia de
// GET /identidad/usuarios/{id}, que hasta este mismo cambio no lo
// necesitaba.
//
// Depende ÚNICAMENTE de acceso/puertos.ValidadorDeAccesos, nunca de
// acceso/adaptadores (INV-TEN-28: el único paquete de Tenencia autorizado a
// importar algo de acceso/puertos es este, el middleware HTTP). La
// duplicación de ~30 líneas frente a acceso/adaptadores/http/middleware.go
// es deliberada y acotada, mismo criterio que la duplicación de
// OrigenSolicitud entre los tres contextos (§1.7 del diseño).
const cabeceraAutorizacion = "Authorization"
const prefijoBearer = "Bearer "

// claveAcceso es la clave no exportada bajo la que este middleware publica
// el puertos.Acceso (de acceso/puertos) ya validado.
type claveAcceso struct{}

// AccesoDesdeContexto recupera el acceso/puertos.Acceso publicado por
// middlewareAutenticacionTenencia. El segundo valor es false si el handler
// se invocó sin pasar por ese middleware.
func AccesoDesdeContexto(ctx context.Context) (accesopuertos.Acceso, bool) {
	acceso, ok := ctx.Value(claveAcceso{}).(accesopuertos.Acceso)
	return acceso, ok
}

// middlewareAutenticacionTenencia construye el middleware Huma por
// operación que exige un token de acceso Bearer válido (§3.3 del diseño de
// Acceso, ComandoValidarAcceso con ExigirSesionViva=false: cero consultas a
// Postgres en el camino feliz).
func middlewareAutenticacionTenencia(api huma.API, validador accesopuertos.ValidadorDeAccesos) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		cabecera := ctx.Header(cabeceraAutorizacion)
		if !strings.HasPrefix(cabecera, prefijoBearer) {
			escribirErrorAutenticacion(api, ctx, "falta la cabecera Authorization: Bearer <token>")
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(cabecera, prefijoBearer))
		if token == "" {
			escribirErrorAutenticacion(api, ctx, "falta la cabecera Authorization: Bearer <token>")
			return
		}

		// Construye un acceso/dominio.OrigenSolicitud directamente aquí, NO
		// el tenencia/dominio.OrigenSolicitud que publica
		// middlewareOrigenSolicitud de este mismo paquete: son tipos
		// distintos a propósito (§1.7 del diseño) y
		// ComandoValidarAcceso.Origen exige el de acceso/dominio.
		origen, err := accesodominio.NuevoOrigenSolicitud(ctx.RemoteAddr(), ctx.Header(fiber.HeaderUserAgent), ctx.Header(cabeceraHuellaDispositivo), ctx.Header(cabeceraIDSolicitud))
		if err != nil {
			origen, _ = accesodominio.NuevoOrigenSolicitud("", ctx.Header(fiber.HeaderUserAgent), ctx.Header(cabeceraHuellaDispositivo), ctx.Header(cabeceraIDSolicitud))
		}

		acceso, err := validador.Validar(ctx.Context(), accesopuertos.ComandoValidarAcceso{
			TokenCompacto:    token,
			ExigirSesionViva: false,
			Origen:           origen,
		})
		if err != nil {
			escribirErrorAutenticacion(api, ctx, "token de acceso inválido o expirado")
			return
		}

		next(huma.WithValue(ctx, claveAcceso{}, acceso))
	}
}

func escribirErrorAutenticacion(api huma.API, ctx huma.Context, mensaje string) {
	ctx.SetHeader("WWW-Authenticate", `Bearer error="invalid_token"`)
	_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, mensaje)
}
