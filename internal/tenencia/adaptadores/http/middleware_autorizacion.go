package http

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/r-david1/moterus/internal/tenencia/dominio"
	"github.com/r-david1/moterus/internal/tenencia/puertos"
)

// nombreParametroOrganizacion es el nombre del parámetro de ruta
// {idOrganizacion} en todas las operaciones org-scoped de este contexto
// (§7 del diseño).
const nombreParametroOrganizacion = "idOrganizacion"

// middlewareAutorizacionTenencia construye el middleware Huma por
// operación que resuelve {idOrganizacion} de la ruta (EXPLÍCITO, nunca del
// token — INV-TEN-13) y llama a
// tenencia/puertos.VerificadorDeAutorizacion.Autorizar con el permiso que
// corresponde a esa operación (§3.7 del diseño). Si se permite, publica el
// AlcanceDeTenencia en el ctx para que la UnidadDeTrabujo emita el
// `SET LOCAL` correspondiente (§6.3, ADR candidato 0031); si se deniega,
// responde 404 (sin_membresia) o 403 (los otros tres motivos) — nunca el
// mismo status para ambos (INV-TEN-17).
//
// Debe montarse SIEMPRE después de middlewareAutenticacionTenencia en la
// cadena de Middlewares de la operación: depende de AccesoDesdeContexto
// para obtener el `sub` ya validado.
func middlewareAutorizacionTenencia(
	api huma.API,
	autorizador puertos.VerificadorDeAutorizacion,
	alcance puertos.AlcanceDeTenencia,
	permiso dominio.Permiso,
) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		acceso, ok := AccesoDesdeContexto(ctx.Context())
		if !ok {
			// Error de programación (falta montar el middleware de
			// autenticación antes que este), no una condición de negocio.
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "no autenticado")
			return
		}

		idOrganizacion := ctx.Param(nombreParametroOrganizacion)
		if idOrganizacion == "" {
			_ = huma.WriteErr(api, ctx, http.StatusUnprocessableEntity, "falta el parámetro de ruta idOrganizacion")
			return
		}

		origen := origenSolicitudDesdeContexto(ctx.Context())
		decision, err := autorizador.Autorizar(ctx.Context(), puertos.ConsultaAutorizacion{
			IDUsuario:      acceso.IDUsuario,
			IDOrganizacion: idOrganizacion,
			Permiso:        permiso.Valor(),
			Origen:         origen,
		})
		if err != nil {
			_ = huma.WriteErr(api, ctx, http.StatusInternalServerError, "error interno al autorizar la operación")
			return
		}
		if !decision.Permitido {
			if decision.Motivo == dominio.MotivoDenegacionSinMembresia.Valor() {
				// 404, nunca 403: un 403 confirmaría que esa organización
				// existe (INV-TEN-17).
				_ = huma.WriteErr(api, ctx, http.StatusNotFound, "organización no encontrada")
				return
			}
			_ = huma.WriteErr(api, ctx, http.StatusForbidden, "no autorizado: "+decision.Motivo)
			return
		}

		// El alcance que se le pasa a la base de datos es EXACTAMENTE el
		// que se acaba de autorizar (§3.7 del diseño, punto 3): no se
		// recalcula ni se recibe de otro lado.
		ctxConAlcance := alcance.ConAlcance(ctx.Context(), acceso.IDUsuario, idOrganizacion)
		next(huma.WithContext(ctx, ctxConAlcance))
	}
}
