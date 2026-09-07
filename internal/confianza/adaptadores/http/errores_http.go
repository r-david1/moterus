package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"github.com/r-david1/moterus/internal/confianza/dominio"
)

// retryColaLlenaSegundos / retryEstadoTicketSegundos son los valores por
// defecto de Retry-After cuando un endpoint público (no el middleware, que
// ya calcula el suyo a partir de un puertos.ResultadoTurno concreto, ver
// middleware_sala_espera.go) devuelve alguno de los desenlaces de bloqueo
// de §1.8 del diseño. El diseño no fija un número aquí (solo lo hace para
// el 503 del middleware, §7.4): se elige el mismo criterio ya documentado
// en retryDesenlaceGenericoSegundos, salvo cola_llena, que amerita un
// backoff más largo (la cola no se vacía en segundos).
const retryColaLlenaSegundos = 30
const retryEstadoTicketSegundos = 5

// mapearErrorDominio es la única función que traduce errores de dominio de
// Confianza a respuestas HTTP (RFC 9457, mismo criterio que
// identidad/adaptadores/http/errores_http.go,
// acceso/adaptadores/http/errores_http.go y
// tenencia/adaptadores/http/errores_http.go), para la extensión de colas de
// acceso virtual (tabla §1.8 del diseño docs/design/colas-virtuales.md).
func mapearErrorDominio(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}

	var (
		// 404
		errSalaNoEncontrada *dominio.ErrSalaNoEncontrada
		errSalaNoAbierta    *dominio.ErrSalaNoAbierta // §7.1: "404 sala inexistente/cerrada"

		// 409
		errAliasSalaYaRegistrado        *dominio.ErrAliasSalaYaRegistrado
		errSalaYaAbiertaParaLaRuta      *dominio.ErrSalaYaAbiertaParaLaRuta
		errTransicionEstadoSalaInvalida *dominio.ErrTransicionEstadoSalaInvalida
		errSalaNoVigente                *dominio.ErrSalaNoVigente

		// 422 — entrada del cliente
		errAliasSalaInvalido      *dominio.ErrAliasSalaInvalido
		errAlcanceSalaInvalido    *dominio.ErrAlcanceSalaInvalido
		errRutaNoProtegible       *dominio.ErrRutaNoProtegible
		errRitmoAdmisionInvalido  *dominio.ErrRitmoAdmisionInvalido
		errModoDegradadoInvalido  *dominio.ErrModoDegradadoInvalido
		errPoliticaSalaInvalida   *dominio.ErrPoliticaSalaInvalida
		errEstadoSalaInvalido     *dominio.ErrEstadoSalaInvalido
		errTicketPlanoInvalido    *dominio.ErrTicketPlanoInvalido
		errIDSalaDeEsperaInvalido *dominio.ErrIDSalaDeEsperaInvalido
		errIDOrganizacionInvalido *dominio.ErrIDOrganizacionInvalido
		errIDUsuarioInvalido      *dominio.ErrIDUsuarioInvalido
		errDireccionIPInvalida    *dominio.ErrDireccionIPInvalida

		// 429 — Confianza denegó el ingreso (§12 del diseño: accion=
		// ingreso_a_sala, único freno contra el farming de tickets)
		errIngresoDenegadoPorConfianza *dominio.ErrIngresoDenegadoPorConfianza

		// 503 — bloqueo de la propia sala (§1.8 del diseño; solo alcanzable
		// hoy desde un endpoint público si el estado de la cola cambia
		// entre SalaVigentePara y el script Lua, o desde el reconciliador)
		errColaLlena                *dominio.ErrColaLlena
		errTurnoNoAlcanzado         *dominio.ErrTurnoNoAlcanzado
		errTurnoCaducado            *dominio.ErrTurnoCaducado
		errTicketDesconocido        *dominio.ErrTicketDesconocido
		errTicketConsumido          *dominio.ErrTicketConsumido
		errEstadoDeColaNoDisponible *dominio.ErrEstadoDeColaNoDisponible

		// 500 — corrupción interna o bug del llamador, nunca una condición
		// de negocio alcanzable con entrada de cliente válida.
		errDesenlaceDeAdmisionInvalido *dominio.ErrDesenlaceDeAdmisionInvalido
		errEstadoTicketInvalido        *dominio.ErrEstadoTicketInvalido
		errHashTicketInvalido          *dominio.ErrHashTicketInvalido
		errRangoEnColaInvalido         *dominio.ErrRangoEnColaInvalido
		errClaveSalaInvalida           *dominio.ErrClaveSalaInvalida
	)

	switch {
	case errors.As(err, &errSalaNoEncontrada):
		return huma.Error404NotFound("sala de espera no encontrada")

	case errors.As(err, &errSalaNoAbierta):
		// Mismo código que "no encontrada" (§7.1: "404 sala
		// inexistente/cerrada"): para quien intenta ingresar, una sala que
		// ya no admite ingresos es indistinguible de una que no existe.
		return huma.Error404NotFound("sala de espera no encontrada")

	case errors.As(err, &errAliasSalaYaRegistrado):
		return huma.Error409Conflict("el alias ya está en uso")

	case errors.As(err, &errSalaYaAbiertaParaLaRuta):
		return huma.Error409Conflict(errSalaYaAbiertaParaLaRuta.Error())

	case errors.As(err, &errTransicionEstadoSalaInvalida):
		return huma.Error409Conflict(errTransicionEstadoSalaInvalida.Error())

	case errors.As(err, &errSalaNoVigente):
		return huma.Error409Conflict(errSalaNoVigente.Error())

	case errors.As(err, &errAliasSalaInvalido):
		return huma.Error422UnprocessableEntity(errAliasSalaInvalido.Error())

	case errors.As(err, &errAlcanceSalaInvalido):
		return huma.Error422UnprocessableEntity(errAlcanceSalaInvalido.Error())

	case errors.As(err, &errRutaNoProtegible):
		return huma.Error422UnprocessableEntity(errRutaNoProtegible.Error())

	case errors.As(err, &errRitmoAdmisionInvalido):
		return huma.Error422UnprocessableEntity(errRitmoAdmisionInvalido.Error())

	case errors.As(err, &errModoDegradadoInvalido):
		return huma.Error422UnprocessableEntity(errModoDegradadoInvalido.Error())

	case errors.As(err, &errPoliticaSalaInvalida):
		return huma.Error422UnprocessableEntity(errPoliticaSalaInvalida.Error())

	case errors.As(err, &errEstadoSalaInvalido):
		return huma.Error422UnprocessableEntity(errEstadoSalaInvalido.Error())

	case errors.As(err, &errTicketPlanoInvalido):
		return huma.Error422UnprocessableEntity(errTicketPlanoInvalido.Error())

	case errors.As(err, &errIDSalaDeEsperaInvalido):
		return huma.Error422UnprocessableEntity(errIDSalaDeEsperaInvalido.Error())

	case errors.As(err, &errIDOrganizacionInvalido):
		return huma.Error422UnprocessableEntity(errIDOrganizacionInvalido.Error())

	case errors.As(err, &errIDUsuarioInvalido):
		return huma.Error422UnprocessableEntity(errIDUsuarioInvalido.Error())

	case errors.As(err, &errDireccionIPInvalida):
		return huma.Error422UnprocessableEntity(errDireccionIPInvalida.Error())

	case errors.As(err, &errIngresoDenegadoPorConfianza):
		// Mismo criterio exacto que identidad/adaptadores/http/errores_http.go
		// para ErrAccesoDenegadoPorConfianza: 429 sin Retry-After si el
		// motivo no trae backoff, con la cabecera (redondeada hacia arriba,
		// mínimo 1s) si sí lo trae.
		if errIngresoDenegadoPorConfianza.ReintentarEn <= 0 {
			return huma.Error429TooManyRequests("ingreso a sala denegado por evaluación de riesgo")
		}
		segundos := int(errIngresoDenegadoPorConfianza.ReintentarEn.Seconds())
		if segundos < 1 {
			segundos = 1
		}
		return conRetryAfter(huma.Error429TooManyRequests("ingreso a sala denegado por evaluación de riesgo"), segundos)

	case errors.As(err, &errColaLlena):
		return conRetryAfter(huma.Error503ServiceUnavailable("la cola de la sala de espera está llena"), retryColaLlenaSegundos)

	case errors.As(err, &errTurnoNoAlcanzado):
		return conRetryAfter(huma.Error503ServiceUnavailable(errTurnoNoAlcanzado.Error()), retryEstadoTicketSegundos)

	case errors.As(err, &errTurnoCaducado):
		return conRetryAfter(huma.Error503ServiceUnavailable(errTurnoCaducado.Error()), retryEstadoTicketSegundos)

	case errors.As(err, &errTicketDesconocido):
		return conRetryAfter(huma.Error503ServiceUnavailable(errTicketDesconocido.Error()), retryEstadoTicketSegundos)

	case errors.As(err, &errTicketConsumido):
		return conRetryAfter(huma.Error503ServiceUnavailable(errTicketConsumido.Error()), retryEstadoTicketSegundos)

	case errors.As(err, &errEstadoDeColaNoDisponible):
		slog.ErrorContext(ctx, "confianza: el estado de la cola no está disponible", "error", err)
		return conRetryAfter(huma.Error503ServiceUnavailable("el estado de la cola no está disponible"), retryColaLlenaSegundos)

	case errors.As(err, &errDesenlaceDeAdmisionInvalido):
		slog.ErrorContext(ctx, "confianza: desenlace de admisión fuera del catálogo cerrado", "error", err)
		return huma.NewError(http.StatusInternalServerError, "error interno del servidor")

	case errors.As(err, &errEstadoTicketInvalido):
		slog.ErrorContext(ctx, "confianza: estado de ticket fuera del catálogo cerrado", "error", err)
		return huma.NewError(http.StatusInternalServerError, "error interno del servidor")

	case errors.As(err, &errHashTicketInvalido):
		slog.ErrorContext(ctx, "confianza: hash de ticket con formato inesperado", "error", err)
		return huma.NewError(http.StatusInternalServerError, "error interno del servidor")

	case errors.As(err, &errRangoEnColaInvalido):
		slog.ErrorContext(ctx, "confianza: rango en cola con formato inesperado", "error", err)
		return huma.NewError(http.StatusInternalServerError, "error interno del servidor")

	case errors.As(err, &errClaveSalaInvalida):
		slog.ErrorContext(ctx, "confianza: clave de sala inválida", "error", err)
		return huma.NewError(http.StatusInternalServerError, "error interno del servidor")

	default:
		slog.ErrorContext(ctx, "error no mapeado en el adaptador HTTP de confianza (colas de acceso virtual)", "error", err)
		return huma.NewError(http.StatusInternalServerError, "error interno del servidor")
	}
}

// conRetryAfter adjunta la cabecera Retry-After (en segundos) a un
// huma.StatusError ya construido (mismo patrón que
// tenencia/adaptadores/http/errores_http.go para ErrAccesoDenegadoPorConfianza).
func conRetryAfter(err huma.StatusError, segundos int) error {
	if segundos < 1 {
		segundos = 1
	}
	return huma.ErrorWithHeaders(err, http.Header{"Retry-After": []string{strconv.Itoa(segundos)}})
}
