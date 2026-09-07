package http

import (
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/r-david1/moterus/internal/confianza/puertos"
)

// nombreParametroOrganizacion es el nombre del parámetro de ruta
// {idOrganizacion} en las operaciones org-scoped de administración de
// salas de espera (§7.2 del diseño docs/design/colas-virtuales.md).
const nombreParametroOrganizacion = "idOrganizacion"

// middlewareAutorizacion construye el middleware Huma por operación que
// resuelve {idOrganizacion} de la ruta (EXPLÍCITO, nunca del token —
// mismo criterio que INV-TEN-13) y llama a
// puertos.VerificadorDeAutorizacion.Autorizar con el permiso que
// corresponde a la operación (§7.2 del diseño: "organizacion.editar" para
// mutar, "organizacion.ver" para leer, reutilizados tal cual del catálogo
// cerrado de Tenencia).
//
// A diferencia de middlewareAutorizacionTenencia (el precedente exacto en
// tenencia/adaptadores/http/middleware_autorizacion.go), este middleware NO
// puede distinguir 404 (sin membresía) de 403 (rol insuficiente): el puerto
// puertos.VerificadorDeAutorizacion de Confianza es deliberadamente
// primitivo (comentario de su propia declaración en puertos/salida.go) y
// solo expone un booleano "permitido", nunca el motivo de una denegación.
// Es una decisión de la firma del puerto (Confianza no debe importar el
// vocabulario de tenencia/dominio.Permiso ni tenencia/puertos.Autorizacion
// fuera de su ACL), documentada aquí como el hueco que resuelve este
// adaptador: toda denegación se traduce uniformemente a 403 "no
// autorizado", nunca a un 404 que fingiría distinguir el motivo.
//
// Debe montarse SIEMPRE después de middlewareAutenticacion en la cadena de
// Middlewares de la operación: depende de accesoDesdeContexto para obtener
// el `sub` ya validado.
func middlewareAutorizacion(api huma.API, autorizador puertos.VerificadorDeAutorizacion, permiso string) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		acceso, ok := accesoDesdeContexto(ctx.Context())
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

		permitido, err := autorizador.Autorizar(ctx.Context(), puertos.ConsultaAutorizacionOrganizacion{
			IDSujeto:       acceso.IDUsuario,
			IDOrganizacion: idOrganizacion,
			Permiso:        permiso,
		})
		if err != nil {
			slog.ErrorContext(ctx.Context(), "confianza: error al autorizar una operación de administración de salas de espera", "error", err)
			_ = huma.WriteErr(api, ctx, http.StatusInternalServerError, "error interno al autorizar la operación")
			return
		}
		if !permitido {
			_ = huma.WriteErr(api, ctx, http.StatusForbidden, "no autorizado")
			return
		}

		next(ctx)
	}
}
