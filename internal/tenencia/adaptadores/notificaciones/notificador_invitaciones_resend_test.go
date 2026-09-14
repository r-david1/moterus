package notificaciones_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/r-david1/moterus/internal/plataforma/correo"
	"github.com/r-david1/moterus/internal/tenencia/adaptadores/notificaciones"
	"github.com/r-david1/moterus/internal/tenencia/dominio"
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

func TestNotificadorInvitacionesResend_ConURLFrontend_IncluyeEnlace(t *testing.T) {
	var capturado map[string]any
	srv := servidorResendFalso(t, &capturado)
	cliente := correo.NuevoClienteResend("clave-de-prueba", "Moterus <no-reply@ejemplo.test>", correo.ConURLEnvio(srv.URL))
	n := notificaciones.NuevoNotificadorInvitacionesResend(cliente, "https://app.ejemplo.test")

	destinatario, err := dominio.NuevoCorreoDestinatario("bruno@ejemplo.test")
	if err != nil {
		t.Fatalf("no se pudo construir el correo de prueba: %v", err)
	}
	rol, err := dominio.RolDesde("miembro")
	if err != nil {
		t.Fatalf("no se pudo construir el rol de prueba: %v", err)
	}

	err = n.EnviarInvitacion(context.Background(), destinatario, "Acme", rol, "token-de-invitacion", time.Now().Add(48*time.Hour))
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	texto, _ := capturado["text"].(string)
	if !strings.Contains(texto, "https://app.ejemplo.test/invitaciones/aceptar?token=token-de-invitacion") {
		t.Fatalf("el cuerpo no contiene el enlace esperado: %q", texto)
	}
	if !strings.Contains(texto, "Acme") || !strings.Contains(texto, "miembro") {
		t.Fatalf("el cuerpo no menciona la organización/rol: %q", texto)
	}
	if capturado["to"].([]any)[0] != "bruno@ejemplo.test" {
		t.Fatalf("to = %v, esperado bruno@ejemplo.test", capturado["to"])
	}
}

func TestNotificadorInvitacionesResend_SinURLFrontend_MuestraTokenEnClaro(t *testing.T) {
	var capturado map[string]any
	srv := servidorResendFalso(t, &capturado)
	cliente := correo.NuevoClienteResend("clave-de-prueba", "Moterus <no-reply@ejemplo.test>", correo.ConURLEnvio(srv.URL))
	n := notificaciones.NuevoNotificadorInvitacionesResend(cliente, "")

	destinatario, err := dominio.NuevoCorreoDestinatario("bruno@ejemplo.test")
	if err != nil {
		t.Fatalf("no se pudo construir el correo de prueba: %v", err)
	}
	rol, err := dominio.RolDesde("administrador")
	if err != nil {
		t.Fatalf("no se pudo construir el rol de prueba: %v", err)
	}

	err = n.EnviarInvitacion(context.Background(), destinatario, "Acme", rol, "token-de-invitacion", time.Now().Add(48*time.Hour))
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	texto, _ := capturado["text"].(string)
	if !strings.Contains(texto, "token-de-invitacion") {
		t.Fatalf("el cuerpo no contiene el token en claro: %q", texto)
	}
	if strings.Contains(texto, "http") {
		t.Fatalf("no debería haber ningún enlace sin URLFrontend configurada: %q", texto)
	}
}

func TestNotificadorInvitacionesResend_FalloDeEnvio_devuelveErrorEnvuelto(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	cliente := correo.NuevoClienteResend("clave-de-prueba", "Moterus <no-reply@ejemplo.test>", correo.ConURLEnvio(srv.URL))
	n := notificaciones.NuevoNotificadorInvitacionesResend(cliente, "")

	destinatario, err := dominio.NuevoCorreoDestinatario("bruno@ejemplo.test")
	if err != nil {
		t.Fatalf("no se pudo construir el correo de prueba: %v", err)
	}
	rol, err := dominio.RolDesde("miembro")
	if err != nil {
		t.Fatalf("no se pudo construir el rol de prueba: %v", err)
	}

	err = n.EnviarInvitacion(context.Background(), destinatario, "Acme", rol, "token-de-invitacion", time.Now().Add(48*time.Hour))
	if err == nil {
		t.Fatalf("se esperaba un error")
	}
}
