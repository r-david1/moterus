package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"github.com/r-david1/moterus/internal/acceso/dominio"
)

// mapearErrorDominio es la única función que traduce errores de dominio a
// respuestas HTTP para los handlers Huma normales de este paquete (RFC
// 9457, mismo criterio que identidad/adaptadores/http/errores_http.go,
// tabla del §7 del diseño de Acceso). El middleware de autenticación
// (middleware.go) usa su propio subconjunto reducido
// (estadoErrorValidacionAcceso) porque no puede devolver un error de Go —
// tiene que escribir la respuesta él mismo dentro de la cadena de
// middlewares de Huma.
func mapearErrorDominio(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}

	var (
		errCredencialesRechazadas *dominio.ErrCredencialesRechazadas
		errCuentaNoOperativa      *dominio.ErrCuentaNoOperativa
		errSegundoFactor          *dominio.ErrSegundoFactorRequerido
		errRefrescoInvalido       *dominio.ErrRefrescoInvalido
		errSesionExpirada         *dominio.ErrSesionExpirada
		errSesionRevocada         *dominio.ErrSesionRevocada
		errSesionNoEncontrada     *dominio.ErrSesionNoEncontrada
		errSesionAjena            *dominio.ErrSesionAjena
		errTokenInvalido          *dominio.ErrTokenAccesoInvalido
		errTokenExpirado          *dominio.ErrTokenAccesoExpirado
		errSesionRevocadaEnLista  *dominio.ErrSesionRevocadaEnLista
		errAccesoDenegado         *dominio.ErrAccesoDenegadoPorConfianza
		errConcurrencia           *dominio.ErrConcurrenciaSesion
		errIDSesionInvalido       *dominio.ErrIDSesionInvalido
		errIDUsuarioInvalido      *dominio.ErrIDUsuarioInvalido
		errTokenRefrescoInvalido  *dominio.ErrTokenRefrescoPlanoInvalido
		errMotivoInvalido         *dominio.ErrMotivoRevocacionInvalido
		errTransicionInvalida     *dominio.ErrTransicionEstadoSesionInvalida
	)

	switch {
	case errors.As(err, &errCredencialesRechazadas):
		return conAutenticacion(huma.Error401Unauthorized("credenciales rechazadas"))

	case errors.As(err, &errSegundoFactor):
		// "type" de problema distinguible (step-up-requerido), MotivoStepUp
		// en el cuerpo — §7 del diseño.
		return &huma.ErrorModel{
			Type:   "https://moterus.dev/problemas/step-up-requerido",
			Title:  "se requiere un segundo factor",
			Status: http.StatusUnauthorized,
			Detail: "la autenticación es válida pero exige un paso adicional antes de emitir una sesión",
			Errors: detallesSegundoFactor(errSegundoFactor),
		}

	case errors.As(err, &errRefrescoInvalido):
		return conAutenticacion(huma.Error401Unauthorized("token de refresco inválido"))

	case errors.As(err, &errSesionExpirada):
		return conAutenticacion(huma.Error401Unauthorized("sesión expirada"))

	case errors.As(err, &errSesionRevocada):
		return conAutenticacion(huma.Error401Unauthorized("sesión revocada"))

	case errors.As(err, &errTokenExpirado):
		return conAutenticacion(huma.Error401Unauthorized("token de acceso expirado"))

	case errors.As(err, &errTokenInvalido):
		return conAutenticacion(huma.Error401Unauthorized("token de acceso inválido"))

	case errors.As(err, &errSesionRevocadaEnLista):
		return conAutenticacion(huma.Error401Unauthorized("la sesión del token fue revocada"))

	case errors.As(err, &errCuentaNoOperativa):
		return huma.Error403Forbidden("cuenta no operativa: " + errCuentaNoOperativa.Motivo)

	case errors.As(err, &errSesionAjena):
		// 404, no 403: un 403 confirmaría que ese IDSesion existe (§7 del
		// diseño, mismo criterio que INV-ID-11 de Identidad).
		return huma.Error404NotFound("sesión no encontrada")

	case errors.As(err, &errSesionNoEncontrada):
		return huma.Error404NotFound("sesión no encontrada")

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

	case errors.As(err, &errConcurrencia):
		return huma.Error409Conflict("conflicto de concurrencia al rotar la sesión; reintenta la operación")

	case errors.As(err, &errIDSesionInvalido):
		return huma.Error422UnprocessableEntity("identificador de sesión inválido")

	case errors.As(err, &errIDUsuarioInvalido):
		return huma.Error422UnprocessableEntity("identificador de usuario inválido")

	case errors.As(err, &errTokenRefrescoInvalido):
		// Estructuralmente inválido (prefijo/longitud/alfabeto): mismo 401
		// genérico que un refresco desconocido (INV-ACC-21), nunca un 422
		// que le confirme al cliente que el formato importa.
		return conAutenticacion(huma.Error401Unauthorized("token de refresco inválido"))

	case errors.As(err, &errMotivoInvalido):
		return huma.Error422UnprocessableEntity("motivo de revocación inválido")

	case errors.As(err, &errTransicionInvalida):
		return huma.Error409Conflict("transición de estado de sesión inválida")

	default:
		slog.ErrorContext(ctx, "error no mapeado en el adaptador HTTP de acceso", "error", err)
		return huma.NewError(http.StatusInternalServerError, "error interno del servidor")
	}
}

// detallesSegundoFactor arma los detalles RFC 9457 de ErrSegundoFactorRequerido:
// MotivoStepUp (§7 del diseño) y, cuando viene fijado, TokenStepUp (§3.5 del
// diseño otp-mfa.md, ADR 0038) — el token que el cliente reenvía en
// POST /acceso/sesiones/segundo-factor para completar el login sin volver a
// presentar la contraseña. huma.ErrorModel no tiene un campo propio para
// esto, así que viaja como un huma.ErrorDetail más, mismo criterio que ya
// usaba este endpoint para MotivoStepUp antes de que TokenStepUp existiera.
func detallesSegundoFactor(e *dominio.ErrSegundoFactorRequerido) []*huma.ErrorDetail {
	detalles := []*huma.ErrorDetail{{Message: "motivo_step_up: " + e.MotivoStepUp}}
	if e.TokenStepUp != "" {
		detalles = append(detalles, &huma.ErrorDetail{Message: "token_step_up: " + e.TokenStepUp})
	}
	return detalles
}

// conAutenticacion agrega la cabecera WWW-Authenticate exigida por RFC
// 6750 a los 401 de este contexto (§7 del diseño).
func conAutenticacion(base huma.StatusError) error {
	return huma.ErrorWithHeaders(base, http.Header{"WWW-Authenticate": []string{`Bearer error="invalid_token"`}})
}

// estadoErrorValidacionAcceso traduce el error tipado que puede devolver
// ValidadorDeAccesos.Validar (§3.3 del diseño: el conjunto acotado de
// errores del camino caliente) a un (status, mensaje) que
// MiddlewareAutenticacion puede escribir directamente, sin pasar por
// mapearErrorDominio (un middleware Huma no puede simplemente "devolver
// error" como un handler; tiene que escribir la respuesta y no llamar a
// next). Duplicación deliberada y acotada respecto de mapearErrorDominio,
// mismo criterio que la duplicación de OrigenSolicitud entre contextos
// (§1.7 del diseño): son dos puntos de la pila con formas de responder
// distintas (handler vs. middleware) que no vale la pena unificar a costa
// de acoplar el middleware al tipo de retorno de los handlers Huma.
func estadoErrorValidacionAcceso(err error) (int, string) {
	var (
		errTokenExpirado         *dominio.ErrTokenAccesoExpirado
		errTokenInvalido         *dominio.ErrTokenAccesoInvalido
		errSesionRevocadaEnLista *dominio.ErrSesionRevocadaEnLista
		errSesionNoEncontrada    *dominio.ErrSesionNoEncontrada
		errSesionExpirada        *dominio.ErrSesionExpirada
		errSesionRevocada        *dominio.ErrSesionRevocada
	)

	switch {
	case errors.As(err, &errTokenExpirado):
		return http.StatusUnauthorized, "token de acceso expirado"
	case errors.As(err, &errTokenInvalido):
		return http.StatusUnauthorized, "token de acceso inválido"
	case errors.As(err, &errSesionRevocadaEnLista):
		return http.StatusUnauthorized, "la sesión del token fue revocada"
	case errors.As(err, &errSesionNoEncontrada):
		return http.StatusUnauthorized, "la sesión del token no existe"
	case errors.As(err, &errSesionExpirada):
		return http.StatusUnauthorized, "la sesión del token expiró"
	case errors.As(err, &errSesionRevocada):
		return http.StatusUnauthorized, "la sesión del token fue revocada"
	default:
		return http.StatusUnauthorized, "token de acceso inválido"
	}
}
