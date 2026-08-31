package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/r-david1/moterus/internal/identidad/dominio"
)

// mapearErrorDominio es la única función que traduce errores de dominio a
// respuestas HTTP (regla dura del encargo: "una única función que traduce
// dominio -> status HTTP"). Usa errors.As sobre los tipos de la tabla 1.5
// del diseño (dominio/errores.go), nunca comparación de strings.
//
// Regla de INV-ID-11 aplicada aquí: ErrCredencialesInvalidas SIEMPRE se
// traduce al mismo 401 genérico, sin distinguir "correo no encontrado" de
// "contraseña incorrecta" ni en el status ni en el mensaje.
func mapearErrorDominio(ctx context.Context, err error) huma.StatusError {
	if err == nil {
		return nil
	}

	var (
		errCorreoInvalido        *dominio.ErrCorreoInvalido
		errCorreoYaRegistrado    *dominio.ErrCorreoYaRegistrado
		errContrasenaDebil       *dominio.ErrContrasenaDebil
		errContrasenaFiltrada    *dominio.ErrContrasenaFiltrada
		errCredencialesInvalidas *dominio.ErrCredencialesInvalidas
		errUsuarioNoEncontrado   *dominio.ErrUsuarioNoEncontrado
		errCorreoNoVerificado    *dominio.ErrCorreoNoVerificado
		errCuentaSuspendida      *dominio.ErrCuentaSuspendida
		errCuentaBloqueada       *dominio.ErrCuentaBloqueada
		errTransicionInvalida    *dominio.ErrTransicionEstadoInvalida
		errAccesoDenegado        *dominio.ErrAccesoDenegadoPorConfianza
		errConcurrencia          *dominio.ErrConcurrenciaUsuario
		errIDUsuarioInvalido     *dominio.ErrIDUsuarioInvalido
		errTokenInvalido         *dominio.ErrTokenVerificacionInvalido
		errTokenExpirado         *dominio.ErrTokenVerificacionExpirado
	)

	switch {
	case errors.As(err, &errCorreoInvalido):
		return huma.Error422UnprocessableEntity("correo inválido", err)

	case errors.As(err, &errCorreoYaRegistrado):
		// ADR candidato 0013: en registro SÍ se revela el conflicto (mitigado
		// con rate limiting de Confianza y verificación de correo), a
		// diferencia de login, que es siempre genérico.
		return huma.Error409Conflict("el correo ya está registrado")

	case errors.As(err, &errContrasenaDebil):
		detalles := make([]*huma.ErrorDetail, 0, len(errContrasenaDebil.Reglas))
		for _, regla := range errContrasenaDebil.Reglas {
			detalles = append(detalles, &huma.ErrorDetail{Message: regla, Location: "body.contrasena"})
		}
		return &huma.ErrorModel{
			Title:  http.StatusText(http.StatusUnprocessableEntity),
			Status: http.StatusUnprocessableEntity,
			Detail: "contraseña débil",
			Errors: detalles,
		}

	case errors.As(err, &errContrasenaFiltrada):
		return huma.Error422UnprocessableEntity("la contraseña aparece en brechas de datos conocidas; elige otra")

	case errors.As(err, &errCredencialesInvalidas):
		// Genérico a propósito (INV-ID-11): nunca reveles si el problema fue
		// el correo o la contraseña.
		return huma.Error401Unauthorized("credenciales inválidas")

	case errors.As(err, &errUsuarioNoEncontrado):
		return huma.Error404NotFound("usuario no encontrado")

	case errors.As(err, &errCorreoNoVerificado):
		return huma.Error403Forbidden("correo no verificado")

	case errors.As(err, &errCuentaSuspendida):
		return huma.Error403Forbidden("cuenta suspendida")

	case errors.As(err, &errCuentaBloqueada):
		return huma.Error403Forbidden("cuenta bloqueada")

	case errors.As(err, &errTransicionInvalida):
		return huma.Error409Conflict("transición de estado inválida")

	case errors.As(err, &errAccesoDenegado):
		// El diseño (sección 1.5) pide 429/403 con Retry-After; el error de
		// dominio no transporta ReintentarEn (eso vive en
		// puertos.DecisionConfianza, no en el error), así que hoy se
		// devuelve 429 sin esa cabecera. Gap documentado en el reporte de
		// cierre: cuando EvaluadorConfianza deje de ser no-op, conviene que
		// el caso de uso o este adaptador tengan forma de propagar
		// ReintentarEn hasta aquí.
		return huma.Error429TooManyRequests("acceso denegado por evaluación de riesgo")

	case errors.As(err, &errConcurrencia):
		return huma.Error409Conflict("conflicto de concurrencia; reintenta la operación")

	case errors.As(err, &errIDUsuarioInvalido):
		return huma.Error422UnprocessableEntity("identificador de usuario inválido")

	case errors.As(err, &errTokenExpirado):
		// Se verifica ANTES que errTokenInvalido a propósito: un token
		// expirado es un caso más específico de "ya no sirve" (410 Gone,
		// el recurso existió y dejó de ser válido) que "nunca existió"
		// (404), aunque ambos tipos son mutuamente excluyentes en el
		// caso de uso (ver VerificarCorreoCasoDeUso.Verificar), así que el
		// orden del switch no cambia el resultado — se deja explícito por
		// legibilidad.
		return huma.Error410Gone("token de verificación expirado")

	case errors.As(err, &errTokenInvalido):
		return huma.Error404NotFound("token de verificación inválido")

	default:
		// Nunca se expone el error interno al cliente (podría filtrar
		// detalles de infraestructura); se registra en el logger del
		// servidor y se devuelve un 500 genérico.
		slog.ErrorContext(ctx, "error no mapeado en el adaptador HTTP de identidad", "error", err)
		return huma.NewError(http.StatusInternalServerError, "error interno del servidor")
	}
}
