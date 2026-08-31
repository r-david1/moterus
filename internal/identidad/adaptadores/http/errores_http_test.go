package http

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/r-david1/moterus/internal/identidad/dominio"
)

// TestMapearErrorDominio_accesoDenegadoSinReintentarEn_no429SinCabecera
// cubre el comportamiento previo a ADR 0018 (EvaluadorConfianzaNoOp nunca
// fija ReintentarEn): debe seguir devolviendo 429 sin Retry-After.
func TestMapearErrorDominio_accesoDenegadoSinReintentarEn_no429SinCabecera(t *testing.T) {
	err := mapearErrorDominio(context.Background(), &dominio.ErrAccesoDenegadoPorConfianza{Motivo: "x"})

	statusErr, ok := err.(huma.StatusError)
	if !ok {
		t.Fatalf("se esperaba un huma.StatusError, obtuvo %T", err)
	}
	if statusErr.GetStatus() != http.StatusTooManyRequests {
		t.Fatalf("status = %d, esperado 429", statusErr.GetStatus())
	}

	var conCabeceras huma.HeadersError
	if errors.As(err, &conCabeceras) {
		t.Fatalf("no se esperaba Retry-After sin ReintentarEn, headers = %v", conCabeceras.GetHeaders())
	}
}

// TestMapearErrorDominio_accesoDenegadoConReintentarEn_fijaRetryAfter
// cubre el caso nuevo (ADR 0018): con ReintentarEn > 0, el 429 debe traer
// la cabecera Retry-After en segundos, redondeada hacia arriba y con
// mínimo 1.
func TestMapearErrorDominio_accesoDenegadoConReintentarEn_fijaRetryAfter(t *testing.T) {
	casos := []struct {
		nombre       string
		reintentarEn time.Duration
		esperado     string
	}{
		{"90 segundos exactos", 90 * time.Second, "90"},
		{"menos de un segundo redondea a 1", 300 * time.Millisecond, "1"},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			err := mapearErrorDominio(context.Background(), &dominio.ErrAccesoDenegadoPorConfianza{
				Motivo:       "limite_ip_excedido",
				ReintentarEn: c.reintentarEn,
			})

			var conCabeceras huma.HeadersError
			if !errors.As(err, &conCabeceras) {
				t.Fatalf("se esperaba un HeadersError con Retry-After")
			}
			if got := conCabeceras.GetHeaders().Get("Retry-After"); got != c.esperado {
				t.Fatalf("Retry-After = %q, esperado %q", got, c.esperado)
			}

			// El status sigue siendo 429 pese al envoltorio de cabeceras
			// (Huma lo resuelve con errors.As sobre el error envuelto).
			var statusErr huma.StatusError
			if !errors.As(err, &statusErr) || statusErr.GetStatus() != http.StatusTooManyRequests {
				t.Fatalf("se esperaba status 429 tras envolver con cabeceras")
			}
		})
	}
}
