package http

import (
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v2"

	accesodominio "github.com/r-david1/moterus/internal/acceso/dominio"
	accesopuertos "github.com/r-david1/moterus/internal/acceso/puertos"
)

// Este archivo es el gancho documentado en ADR 0019 §Consecuencias: "El
// middleware que consume ValidadorDeAccesos es lo que cierra el hueco
// documentado en identidad/adaptadores/http/rutas.go, donde
// GET /identidad/usuarios/{id} está público 'como placeholder hasta que
// exista el middleware de autenticación de Acceso'".
//
// Decisión de diseño (no obvia, ver el informe de la tarea): en vez de
// importar el paquete de adaptadores HTTP de Acceso
// (internal/acceso/adaptadores/http), este archivo depende ÚNICAMENTE de
// acceso/puertos.ValidadorDeAccesos — el mismo criterio que ya usa
// identidad/adaptadores/confianza (importa confianza/puertos, nunca
// confianza/adaptadores): la capa anticorrupción la posee quien depende,
// no quien es dependido, y un puerto es exactamente la superficie
// pensada para cruzar esta frontera. La duplicación de ~20 líneas frente
// al middleware equivalente de acceso/adaptadores/http/middleware.go es
// deliberada y acotada, mismo criterio que la duplicación de
// OrigenSolicitud entre los dominios de ambos contextos.
const cabeceraAutorizacion = "Authorization"
const prefijoBearer = "Bearer "

// middlewareAutenticacionAcceso construye el middleware Huma por operación
// que exige un token de acceso Bearer válido (§3.3 del diseño de Acceso,
// ComandoValidarAcceso con ExigirSesionViva=false: cero consultas a
// Postgres en el camino feliz). No publica el puertos.Acceso resultante en
// el contexto: ObtenerPorID no lo necesita hoy (sigue leyendo
// solicitante_id de la query, comportamiento sin cambios) — esta es
// deliberadamente la extensión MÍNIMA que cierra el hueco de
// autenticación sin tocar la lógica de negocio ya cerrada de Identidad.
func middlewareAutenticacionAcceso(api huma.API, validador accesopuertos.ValidadorDeAccesos) func(huma.Context, func(huma.Context)) {
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
		// el identidad/dominio.OrigenSolicitud que publica
		// middlewareOrigenSolicitud de este mismo paquete (middleware.go):
		// son tipos distintos a propósito (§1.7 del diseño de Acceso) y
		// ComandoValidarAcceso.Origen exige el de acceso/dominio.
		origen, err := accesodominio.NuevoOrigenSolicitud(ctx.RemoteAddr(), ctx.Header(fiber.HeaderUserAgent), "", ctx.Header(cabeceraIDSolicitud))
		if err != nil {
			origen, _ = accesodominio.NuevoOrigenSolicitud("", ctx.Header(fiber.HeaderUserAgent), "", ctx.Header(cabeceraIDSolicitud))
		}
		if _, err := validador.Validar(ctx.Context(), accesopuertos.ComandoValidarAcceso{
			TokenCompacto:    token,
			ExigirSesionViva: false,
			Origen:           origen,
		}); err != nil {
			escribirErrorAutenticacion(api, ctx, "token de acceso inválido o expirado")
			return
		}

		next(ctx)
	}
}

func escribirErrorAutenticacion(api huma.API, ctx huma.Context, mensaje string) {
	ctx.SetHeader("WWW-Authenticate", `Bearer error="invalid_token"`)
	_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, mensaje)
}
