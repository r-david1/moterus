package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"github.com/r-david1/moterus/internal/tenencia/dominio"
)

// mapearErrorDominio es la única función que traduce errores de dominio de
// Tenencia a respuestas HTTP (RFC 9457, mismo criterio que
// identidad/adaptadores/http/errores_http.go y
// acceso/adaptadores/http/errores_http.go, tabla del §7 del diseño de
// Tenencia).
func mapearErrorDominio(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}

	var (
		// 404 — INV-TEN-17: colapsados a propósito con el mismo cuerpo.
		errNoEsMiembro              *dominio.ErrNoEsMiembro
		errOrganizacionNoEncontrada *dominio.ErrOrganizacionNoEncontrada
		errMembresiaNoEncontrada    *dominio.ErrMembresiaNoEncontrada
		// 404 — INV-TEN-24: indistinguibles entre sí.
		errInvitacionInvalida *dominio.ErrInvitacionInvalida
		errInvitacionAjena    *dominio.ErrInvitacionAjena

		// 403
		errNoAutorizado            *dominio.ErrNoAutorizado
		errRolSuperiorAlPropio     *dominio.ErrRolSuperiorAlPropio
		errMembresiaDominante      *dominio.ErrMembresiaDominante
		errOrganizacionNoOperativa *dominio.ErrOrganizacionNoOperativa
		errSujetoNoElegible        *dominio.ErrSujetoNoElegible

		// 409
		errAliasYaRegistrado            *dominio.ErrAliasYaRegistrado
		errMembresiaDuplicada           *dominio.ErrMembresiaDuplicada
		errInvitacionDuplicada          *dominio.ErrInvitacionDuplicada
		errUltimoPropietario            *dominio.ErrUltimoPropietario
		errLimiteMiembrosExcedido       *dominio.ErrLimiteMiembrosExcedido
		errLimiteInvitacionesExcedido   *dominio.ErrLimiteInvitacionesExcedido
		errLimiteOrganizacionesExcedido *dominio.ErrLimiteOrganizacionesExcedido
		errConcurrenciaMembresia        *dominio.ErrConcurrenciaMembresia
		errTransicionEstadoOrganizacion *dominio.ErrTransicionEstadoOrganizacionInvalida
		errTransicionEstadoMembresia    *dominio.ErrTransicionEstadoMembresiaInvalida
		errTransicionEstadoInvitacion   *dominio.ErrTransicionEstadoInvitacionInvalida

		// 422 — entrada del cliente
		errAliasInvalido                *dominio.ErrAliasInvalido
		errNombreOrganizacionInvalido   *dominio.ErrNombreOrganizacionInvalido
		errCorreoDestinatarioInvalido   *dominio.ErrCorreoDestinatarioInvalido
		errMotivoCambioEstadoInvalido   *dominio.ErrMotivoCambioEstadoInvalido
		errRolInvalido                  *dominio.ErrRolInvalido
		errEstadoOrganizacionInvalido   *dominio.ErrEstadoOrganizacionInvalido
		errEstadoMembresiaInvalido      *dominio.ErrEstadoMembresiaInvalido
		errEstadoInvitacionInvalido     *dominio.ErrEstadoInvitacionInvalido
		errIDOrganizacionInvalido       *dominio.ErrIDOrganizacionInvalido
		errIDUsuarioInvalido            *dominio.ErrIDUsuarioInvalido
		errIDMembresiaInvalido          *dominio.ErrIDMembresiaInvalido
		errIDInvitacionInvalido         *dominio.ErrIDInvitacionInvalido
		errTokenInvitacionPlanoInvalido *dominio.ErrTokenInvitacionPlanoInvalido
		errHashTokenInvitacionInvalido  *dominio.ErrHashTokenInvitacionInvalido
		errMotivoDenegacionInvalido     *dominio.ErrMotivoDenegacionInvalido

		// 429
		errAccesoDenegado *dominio.ErrAccesoDenegadoPorConfianza

		// 500 — bug del llamador o de configuración, no una condición de negocio.
		errPermisoDesconocido           *dominio.ErrPermisoDesconocido
		errPoliticaOrganizacionInvalida *dominio.ErrPoliticaOrganizacionInvalida
	)

	switch {
	case errors.As(err, &errNoEsMiembro), errors.As(err, &errOrganizacionNoEncontrada), errors.As(err, &errMembresiaNoEncontrada):
		return huma.Error404NotFound("organización o membresía no encontrada")

	case errors.As(err, &errInvitacionInvalida), errors.As(err, &errInvitacionAjena):
		return huma.Error404NotFound("invitación inválida")

	case errors.As(err, &errNoAutorizado):
		return huma.Error403Forbidden("no autorizado: " + errNoAutorizado.Motivo.Valor())

	case errors.As(err, &errRolSuperiorAlPropio):
		return huma.Error403Forbidden(errRolSuperiorAlPropio.Error())

	case errors.As(err, &errMembresiaDominante):
		return huma.Error403Forbidden(errMembresiaDominante.Error())

	case errors.As(err, &errOrganizacionNoOperativa):
		return huma.Error403Forbidden(errOrganizacionNoOperativa.Error())

	case errors.As(err, &errSujetoNoElegible):
		return huma.Error403Forbidden(errSujetoNoElegible.Error())

	case errors.As(err, &errAliasYaRegistrado):
		return huma.Error409Conflict("el alias ya está en uso")

	case errors.As(err, &errMembresiaDuplicada):
		return huma.Error409Conflict(errMembresiaDuplicada.Error())

	case errors.As(err, &errInvitacionDuplicada):
		return huma.Error409Conflict(errInvitacionDuplicada.Error())

	case errors.As(err, &errUltimoPropietario):
		// Mensaje accionable ya lo trae Error(): "transferí la propiedad
		// antes de salir" (§3.2 del diseño).
		return huma.Error409Conflict(errUltimoPropietario.Error())

	case errors.As(err, &errLimiteMiembrosExcedido):
		return huma.Error409Conflict(errLimiteMiembrosExcedido.Error())

	case errors.As(err, &errLimiteInvitacionesExcedido):
		return huma.Error409Conflict(errLimiteInvitacionesExcedido.Error())

	case errors.As(err, &errLimiteOrganizacionesExcedido):
		return huma.Error409Conflict(errLimiteOrganizacionesExcedido.Error())

	case errors.As(err, &errConcurrenciaMembresia):
		return huma.Error409Conflict("conflicto de concurrencia; reintenta la operación")

	case errors.As(err, &errTransicionEstadoOrganizacion):
		return huma.Error409Conflict("transición de estado de organización inválida")

	case errors.As(err, &errTransicionEstadoMembresia):
		return huma.Error409Conflict("transición de estado de membresía inválida")

	case errors.As(err, &errTransicionEstadoInvitacion):
		return huma.Error409Conflict("transición de estado de invitación inválida")

	case errors.As(err, &errAliasInvalido):
		return huma.Error422UnprocessableEntity(errAliasInvalido.Error())

	case errors.As(err, &errNombreOrganizacionInvalido):
		return huma.Error422UnprocessableEntity(errNombreOrganizacionInvalido.Error())

	case errors.As(err, &errCorreoDestinatarioInvalido):
		return huma.Error422UnprocessableEntity(errCorreoDestinatarioInvalido.Error())

	case errors.As(err, &errMotivoCambioEstadoInvalido):
		return huma.Error422UnprocessableEntity(errMotivoCambioEstadoInvalido.Error())

	case errors.As(err, &errRolInvalido):
		return huma.Error422UnprocessableEntity(errRolInvalido.Error())

	case errors.As(err, &errEstadoOrganizacionInvalido):
		return huma.Error422UnprocessableEntity(errEstadoOrganizacionInvalido.Error())

	case errors.As(err, &errEstadoMembresiaInvalido):
		return huma.Error422UnprocessableEntity(errEstadoMembresiaInvalido.Error())

	case errors.As(err, &errEstadoInvitacionInvalido):
		return huma.Error422UnprocessableEntity(errEstadoInvitacionInvalido.Error())

	case errors.As(err, &errIDOrganizacionInvalido):
		return huma.Error422UnprocessableEntity(errIDOrganizacionInvalido.Error())

	case errors.As(err, &errIDUsuarioInvalido):
		return huma.Error422UnprocessableEntity(errIDUsuarioInvalido.Error())

	case errors.As(err, &errIDMembresiaInvalido):
		return huma.Error422UnprocessableEntity(errIDMembresiaInvalido.Error())

	case errors.As(err, &errIDInvitacionInvalido):
		return huma.Error422UnprocessableEntity(errIDInvitacionInvalido.Error())

	case errors.As(err, &errTokenInvitacionPlanoInvalido):
		return huma.Error422UnprocessableEntity(errTokenInvitacionPlanoInvalido.Error())

	case errors.As(err, &errHashTokenInvitacionInvalido):
		return huma.Error422UnprocessableEntity(errHashTokenInvitacionInvalido.Error())

	case errors.As(err, &errMotivoDenegacionInvalido):
		return huma.Error422UnprocessableEntity(errMotivoDenegacionInvalido.Error())

	case errors.As(err, &errAccesoDenegado):
		errBase := huma.Error429TooManyRequests("acceso denegado por evaluación de confianza")
		if errAccesoDenegado.ReintentarEn <= 0 {
			return errBase
		}
		segundos := int(errAccesoDenegado.ReintentarEn.Seconds())
		if segundos < 1 {
			segundos = 1
		}
		return huma.ErrorWithHeaders(errBase, http.Header{"Retry-After": []string{strconv.Itoa(segundos)}})

	case errors.As(err, &errPermisoDesconocido):
		// Bug del llamador (otro contexto pidiendo un permiso fuera del
		// catálogo cerrado), nunca 403: enmascararlo como denegación
		// escondería un error de programación tras un comportamiento
		// plausible (§3.7 del diseño).
		slog.ErrorContext(ctx, "tenencia: permiso desconocido solicitado a Autorizar", "error", err)
		return huma.NewError(http.StatusInternalServerError, "error interno del servidor")

	case errors.As(err, &errPoliticaOrganizacionInvalida):
		slog.ErrorContext(ctx, "tenencia: política de organización inválida", "error", err)
		return huma.NewError(http.StatusInternalServerError, "error interno del servidor")

	default:
		slog.ErrorContext(ctx, "error no mapeado en el adaptador HTTP de tenencia", "error", err)
		return huma.NewError(http.StatusInternalServerError, "error interno del servidor")
	}
}
