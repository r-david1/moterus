package notificaciones_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/r-david1/moterus/internal/identidad/adaptadores/notificaciones"
	"github.com/r-david1/moterus/internal/identidad/dominio"
	"github.com/r-david1/moterus/internal/plataforma/correo"
)

func servidorResendFalso(t *testing.T, capturado *map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(capturado); err != nil {
			t.Fatalf("no se pudo decodificar el cuerpo: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "correo-de-prueba"})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestNotificadorCorreoResend_ConURLFrontend_IncluyeEnlace(t *testing.T) {
	var capturado map[string]any
	srv := servidorResendFalso(t, &capturado)
	cliente := correo.NuevoClienteResend("clave-de-prueba", "Moterus <no-reply@ejemplo.test>", correo.ConURLEnvio(srv.URL))
	n := notificaciones.NuevoNotificadorCorreoResend(cliente, "https://app.ejemplo.test")

	correoUsuario, err := dominio.NuevoCorreo("ana@ejemplo.test")
	if err != nil {
		t.Fatalf("no se pudo construir el correo de prueba: %v", err)
	}

	if err := n.EnviarVerificacion(context.Background(), correoUsuario, "token-de-prueba"); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	texto, _ := capturado["text"].(string)
	if !strings.Contains(texto, "https://app.ejemplo.test/verificar-correo?token=token-de-prueba") {
		t.Fatalf("el cuerpo no contiene el enlace esperado: %q", texto)
	}
	if capturado["to"].([]any)[0] != "ana@ejemplo.test" {
		t.Fatalf("to = %v, esperado ana@ejemplo.test", capturado["to"])
	}
}

func TestNotificadorCorreoResend_SinURLFrontend_MuestraTokenEnClaro(t *testing.T) {
	var capturado map[string]any
	srv := servidorResendFalso(t, &capturado)
	cliente := correo.NuevoClienteResend("clave-de-prueba", "Moterus <no-reply@ejemplo.test>", correo.ConURLEnvio(srv.URL))
	n := notificaciones.NuevoNotificadorCorreoResend(cliente, "")

	correoUsuario, err := dominio.NuevoCorreo("ana@ejemplo.test")
	if err != nil {
		t.Fatalf("no se pudo construir el correo de prueba: %v", err)
	}

	if err := n.EnviarVerificacion(context.Background(), correoUsuario, "token-de-prueba"); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	texto, _ := capturado["text"].(string)
	if !strings.Contains(texto, "token-de-prueba") {
		t.Fatalf("el cuerpo no contiene el token en claro: %q", texto)
	}
	if strings.Contains(texto, "http") {
		t.Fatalf("no debería haber ningún enlace sin URLFrontend configurada: %q", texto)
	}
}

func TestNotificadorCorreoResend_FalloDeEnvio_devuelveErrorEnvuelto(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	cliente := correo.NuevoClienteResend("clave-de-prueba", "Moterus <no-reply@ejemplo.test>", correo.ConURLEnvio(srv.URL))
	n := notificaciones.NuevoNotificadorCorreoResend(cliente, "")

	correoUsuario, err := dominio.NuevoCorreo("ana@ejemplo.test")
	if err != nil {
		t.Fatalf("no se pudo construir el correo de prueba: %v", err)
	}

	if err := n.EnviarVerificacion(context.Background(), correoUsuario, "token-de-prueba"); err == nil {
		t.Fatalf("se esperaba un error")
	}
}
