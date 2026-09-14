package correo_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/r-david1/moterus/internal/plataforma/correo"
)

// servidorFalso simula la API de envío de Resend sin depender de red ni de
// una cuenta real — mismo patrón que turnstile.servidorFalso.
func servidorFalso(t *testing.T, responder func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(responder))
	t.Cleanup(srv.Close)
	return srv
}

func TestEnviar_peticionValida_seEnviaCorrectamente(t *testing.T) {
	var cuerpoRecibido map[string]any
	var autorizacionRecibida string
	srv := servidorFalso(t, func(w http.ResponseWriter, r *http.Request) {
		autorizacionRecibida = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&cuerpoRecibido); err != nil {
			t.Fatalf("no se pudo decodificar el cuerpo: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "correo-de-prueba-id"})
	})

	c := correo.NuevoClienteResend("clave-de-prueba", "Moterus <no-reply@ejemplo.test>", correo.ConURLEnvio(srv.URL))

	err := c.Enviar(context.Background(), correo.Mensaje{
		Destinatario: "ana@ejemplo.test",
		Asunto:       "Verificá tu correo",
		TextoPlano:   "Tu token es: abc123",
	})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	if autorizacionRecibida != "Bearer clave-de-prueba" {
		t.Fatalf("Authorization = %q, esperado 'Bearer clave-de-prueba'", autorizacionRecibida)
	}
	if cuerpoRecibido["from"] != "Moterus <no-reply@ejemplo.test>" {
		t.Fatalf("from = %v, esperado el remitente configurado", cuerpoRecibido["from"])
	}
	destinatarios, ok := cuerpoRecibido["to"].([]any)
	if !ok || len(destinatarios) != 1 || destinatarios[0] != "ana@ejemplo.test" {
		t.Fatalf("to = %v, esperado [\"ana@ejemplo.test\"]", cuerpoRecibido["to"])
	}
	if cuerpoRecibido["subject"] != "Verificá tu correo" {
		t.Fatalf("subject = %v, esperado el asunto configurado", cuerpoRecibido["subject"])
	}
}

func TestEnviar_ResendRespondeError_devuelveErrorConDetalle(t *testing.T) {
	srv := servidorFalso(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{"message": "API key is invalid", "name": "validation_error"})
	})

	c := correo.NuevoClienteResend("clave-invalida", "Moterus <no-reply@ejemplo.test>", correo.ConURLEnvio(srv.URL))

	err := c.Enviar(context.Background(), correo.Mensaje{Destinatario: "ana@ejemplo.test", Asunto: "x", TextoPlano: "y"})
	if err == nil {
		t.Fatalf("se esperaba un error")
	}
}

func TestEnviar_sinAPIKey_devuelveErrorSinLlamarARed(t *testing.T) {
	c := correo.NuevoClienteResend("", "Moterus <no-reply@ejemplo.test>", correo.ConURLEnvio("http://no-deberia-llamarse.invalid"))

	err := c.Enviar(context.Background(), correo.Mensaje{Destinatario: "ana@ejemplo.test", Asunto: "x", TextoPlano: "y"})
	if err == nil {
		t.Fatalf("se esperaba un error (sin API key)")
	}
}

func TestEnviar_sinRemitente_devuelveErrorSinLlamarARed(t *testing.T) {
	c := correo.NuevoClienteResend("clave-de-prueba", "", correo.ConURLEnvio("http://no-deberia-llamarse.invalid"))

	err := c.Enviar(context.Background(), correo.Mensaje{Destinatario: "ana@ejemplo.test", Asunto: "x", TextoPlano: "y"})
	if err == nil {
		t.Fatalf("se esperaba un error (sin remitente)")
	}
}

func TestEnviar_sinDestinatario_devuelveErrorSinLlamarARed(t *testing.T) {
	c := correo.NuevoClienteResend("clave-de-prueba", "Moterus <no-reply@ejemplo.test>", correo.ConURLEnvio("http://no-deberia-llamarse.invalid"))

	err := c.Enviar(context.Background(), correo.Mensaje{Asunto: "x", TextoPlano: "y"})
	if err == nil {
		t.Fatalf("se esperaba un error (sin destinatario)")
	}
}
