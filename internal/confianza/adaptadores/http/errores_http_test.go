package http

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2"

	"github.com/r-david1/moterus/internal/confianza/dominio"
)

// TestMapearErrorDominio_Estados cubre una muestra representativa de la
// tabla del §1.8 del diseño docs/design/colas-virtuales.md: 404, 409, 422 y
// 500 (fallback). Los 503 con Retry-After tienen su propio test, porque
// necesitan inspeccionar la cabecera.
func TestMapearErrorDominio_Estados(t *testing.T) {
	casos := []struct {
		nombre   string
		err      error
		esperado int
	}{
		{"sala no encontrada -> 404", &dominio.ErrSalaNoEncontrada{Referencia: "x"}, http.StatusNotFound},
		{"sala no abierta -> 404 (indistinguible de inexistente, §7.1)", &dominio.ErrSalaNoAbierta{}, http.StatusNotFound},
		{"alias ya registrado -> 409", &dominio.ErrAliasSalaYaRegistrado{Alias: "x"}, http.StatusConflict},
		{"sala ya abierta para la ruta -> 409 (INV-COLA-01)", &dominio.ErrSalaYaAbiertaParaLaRuta{Clave: "x"}, http.StatusConflict},
		{"transición de estado inválida -> 409", &dominio.ErrTransicionEstadoSalaInvalida{Origen: dominio.EstadoSalaCerrada, Destino: dominio.EstadoSalaAbierta}, http.StatusConflict},
		{"alias inválido -> 422", &dominio.ErrAliasSalaInvalido{Motivo: "x"}, http.StatusUnprocessableEntity},
		{"ruta no protegible -> 422", &dominio.ErrRutaNoProtegible{Motivo: "x"}, http.StatusUnprocessableEntity},
		{"ritmo de admisión inválido -> 422", &dominio.ErrRitmoAdmisionInvalido{Motivo: "x"}, http.StatusUnprocessableEntity},
		{"política de sala inválida -> 422", &dominio.ErrPoliticaSalaInvalida{Motivo: "x"}, http.StatusUnprocessableEntity},
		{"ticket con formato inválido -> 422", &dominio.ErrTicketPlanoInvalido{Motivo: "x"}, http.StatusUnprocessableEntity},
		{"error sin mapear -> 500", errors.New("boom"), http.StatusInternalServerError},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			err := mapearErrorDominio(context.Background(), c.err)
			statusErr, ok := err.(huma.StatusError)
			if !ok {
				t.Fatalf("se esperaba un huma.StatusError, obtuvo %T", err)
			}
			if statusErr.GetStatus() != c.esperado {
				t.Fatalf("status = %d, esperado %d", statusErr.GetStatus(), c.esperado)
			}
		})
	}
}

// TestMapearErrorDominio_BloqueosDeColaFijanRetryAfter cubre los errores de
// bloqueo de §1.8 del diseño (503 + Retry-After).
func TestMapearErrorDominio_BloqueosDeColaFijanRetryAfter(t *testing.T) {
	casos := []struct {
		nombre string
		err    error
	}{
		{"cola llena", &dominio.ErrColaLlena{}},
		{"turno no alcanzado", &dominio.ErrTurnoNoAlcanzado{Posicion: 1}},
		{"turno caducado", &dominio.ErrTurnoCaducado{}},
		{"ticket desconocido", &dominio.ErrTicketDesconocido{}},
		{"ticket consumido", &dominio.ErrTicketConsumido{}},
		{"estado de cola no disponible", &dominio.ErrEstadoDeColaNoDisponible{Motivo: "redis caído"}},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			err := mapearErrorDominio(context.Background(), c.err)

			var statusErr huma.StatusError
			if !errors.As(err, &statusErr) {
				t.Fatalf("se esperaba un huma.StatusError, obtuvo %T", err)
			}
			if statusErr.GetStatus() != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, esperado 503", statusErr.GetStatus())
			}

			var conCabeceras huma.HeadersError
			if !errors.As(err, &conCabeceras) {
				t.Fatal("se esperaba un HeadersError con Retry-After")
			}
			if got := conCabeceras.GetHeaders().Get("Retry-After"); got == "" {
				t.Fatal("se esperaba la cabecera Retry-After presente")
			}
		})
	}
}

func TestMapearErrorDominio_NilDevuelveNil(t *testing.T) {
	if err := mapearErrorDominio(context.Background(), nil); err != nil {
		t.Fatalf("se esperaba nil, obtuvo %v", err)
	}
}
