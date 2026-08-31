package turnstile_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/r-david1/moterus/internal/confianza/adaptadores/turnstile"
)

// servidorFalso simula la API de siteverify de Cloudflare sin depender de
// red ni de credenciales reales — así se puede probar (y desarrollar
// localmente) el adaptador completo sin una cuenta de Cloudflare Turnstile.
// El propio VerificadorCaptcha soporta ConURLVerificacion exactamente para
// este propósito (mismo patrón que cripto.ConBaseURLHIBP en Identidad).
func servidorFalso(t *testing.T, responder func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(responder))
	t.Cleanup(srv.Close)
	return srv
}

func TestVerificar_tokenValido_devuelvePuntajeUno(t *testing.T) {
	srv := servidorFalso(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("no se pudo parsear el form: %v", err)
		}
		if r.FormValue("secret") != "secreto-de-prueba" {
			t.Fatalf("secret = %q, esperado secreto-de-prueba", r.FormValue("secret"))
		}
		if r.FormValue("response") != "token-valido" {
			t.Fatalf("response = %q, esperado token-valido", r.FormValue("response"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "action": "login"})
	})

	v := turnstile.NuevoVerificadorCaptcha("secreto-de-prueba", srv.URL, "production")

	puntaje, err := v.Verificar(context.Background(), "token-valido", "login", "203.0.113.1")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if puntaje != 1.0 {
		t.Fatalf("puntaje = %v, esperado 1.0", puntaje)
	}
}

func TestVerificar_tokenRechazado_devuelvePuntajeCero(t *testing.T) {
	srv := servidorFalso(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error-codes": []string{"invalid-input-response"}})
	})

	v := turnstile.NuevoVerificadorCaptcha("secreto-de-prueba", srv.URL, "production")

	puntaje, err := v.Verificar(context.Background(), "token-invalido", "login", "203.0.113.1")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if puntaje != 0.0 {
		t.Fatalf("puntaje = %v, esperado 0.0", puntaje)
	}
}

func TestVerificar_accionNoCoincide_devuelvePuntajeCero(t *testing.T) {
	srv := servidorFalso(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "action": "registro"})
	})

	v := turnstile.NuevoVerificadorCaptcha("secreto-de-prueba", srv.URL, "production")

	puntaje, err := v.Verificar(context.Background(), "token-valido", "login", "203.0.113.1")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if puntaje != 0.0 {
		t.Fatalf("puntaje = %v, esperado 0.0 por acción no coincidente", puntaje)
	}
}

func TestNuevoVerificadorCaptcha_sinSecretoEnDesarrollo_esFailOpen(t *testing.T) {
	v := turnstile.NuevoVerificadorCaptcha("", "http://no-deberia-llamarse.invalid", "development")

	puntaje, err := v.Verificar(context.Background(), "cualquier-token", "login", "203.0.113.1")
	if err != nil {
		t.Fatalf("error inesperado en modo fail-open: %v", err)
	}
	if puntaje != 1.0 {
		t.Fatalf("puntaje = %v, esperado 1.0 (fail-open sin credenciales en dev)", puntaje)
	}
}

func TestNuevoVerificadorCaptcha_sinSecretoEnProduccion_esFailClosed(t *testing.T) {
	v := turnstile.NuevoVerificadorCaptcha("", "http://no-deberia-llamarse.invalid", "production")

	puntaje, err := v.Verificar(context.Background(), "cualquier-token", "login", "203.0.113.1")
	if err == nil {
		t.Fatalf("se esperaba un error (fail-closed sin credenciales en producción)")
	}
	if puntaje != 0.0 {
		t.Fatalf("puntaje = %v, esperado 0.0", puntaje)
	}
}
