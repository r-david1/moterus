package confianza_test

import (
	"context"
	"testing"
	"time"

	"github.com/r-david1/moterus/internal/confianza/dominio"
	confianzapuertos "github.com/r-david1/moterus/internal/confianza/puertos"
	identidadconfianza "github.com/r-david1/moterus/internal/identidad/adaptadores/confianza"
	iddominio "github.com/r-david1/moterus/internal/identidad/dominio"
	"github.com/r-david1/moterus/internal/identidad/puertos"
)

// riesgoFalso es un test double mínimo de confianza/puertos.EvaluadorDeRiesgo
// (no se reutiliza confianza/puertos/mocks a propósito: este test verifica
// la traducción del ACL, no el caso de uso de Confianza).
type riesgoFalso struct {
	solicitudRecibida     confianzapuertos.Solicitud
	resultadoRecibido     confianzapuertos.ResultadoIntento
	decisionADevolver     dominio.Decision
	errorEvaluarADevolver error
}

func (r *riesgoFalso) Evaluar(_ context.Context, s confianzapuertos.Solicitud) (dominio.Decision, error) {
	r.solicitudRecibida = s
	return r.decisionADevolver, r.errorEvaluarADevolver
}

func (r *riesgoFalso) RegistrarResultado(_ context.Context, res confianzapuertos.ResultadoIntento) error {
	r.resultadoRecibido = res
	return nil
}

func origenDePrueba(t *testing.T, ip string) iddominio.OrigenSolicitud {
	t.Helper()
	origen, err := iddominio.NuevoOrigenSolicitud(ip, "agente-de-prueba", "", "id-solicitud-de-prueba")
	if err != nil {
		t.Fatalf("no se pudo construir OrigenSolicitud: %v", err)
	}
	return origen
}

func TestEvaluar_traduceSolicitudYDecision(t *testing.T) {
	riesgo := &riesgoFalso{
		decisionADevolver: dominio.Decision{
			Permitido:       false,
			RequiereCaptcha: true,
			RequiereStepUp:  true,
			Puntaje:         0.42,
			Motivo:          "limite_cuenta_excedido_requiere_captcha",
			ReintentarEn:    90 * time.Second,
		},
	}
	acl := identidadconfianza.NuevoEvaluadorConfianzaReal(riesgo)

	decision, err := acl.Evaluar(context.Background(), puertos.SolicitudEvaluacion{
		Accion:            "login",
		CorreoNormalizado: "  Ana@Ejemplo.com ",
		Origen:            origenDePrueba(t, "203.0.113.9"),
		TokenCaptcha:      "un-token",
	})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	if riesgo.solicitudRecibida.Accion != dominio.AccionLogin {
		t.Errorf("Accion = %q, esperado login", riesgo.solicitudRecibida.Accion)
	}
	if riesgo.solicitudRecibida.IPOrigen != "203.0.113.9" {
		t.Errorf("IPOrigen = %q", riesgo.solicitudRecibida.IPOrigen)
	}
	if riesgo.solicitudRecibida.CorreoNormalizado != "ana@ejemplo.com" {
		t.Errorf("CorreoNormalizado = %q, esperado ana@ejemplo.com (normalizado)", riesgo.solicitudRecibida.CorreoNormalizado)
	}
	if riesgo.solicitudRecibida.TokenCaptcha != "un-token" {
		t.Errorf("TokenCaptcha = %q", riesgo.solicitudRecibida.TokenCaptcha)
	}

	if decision.Permitido {
		t.Errorf("Permitido = true, esperado false")
	}
	if !decision.RequiereCaptcha || !decision.RequiereStepUp {
		t.Errorf("RequiereCaptcha/RequiereStepUp no se tradujeron: %+v", decision)
	}
	if decision.Puntaje != 0.42 {
		t.Errorf("Puntaje = %v, esperado 0.42", decision.Puntaje)
	}
	if decision.Motivo != "limite_cuenta_excedido_requiere_captcha" {
		t.Errorf("Motivo = %q", decision.Motivo)
	}
	if decision.ReintentarEn != 90*time.Second {
		t.Errorf("ReintentarEn = %v", decision.ReintentarEn)
	}
}

func TestRegistrarResultado_traduce(t *testing.T) {
	riesgo := &riesgoFalso{}
	acl := identidadconfianza.NuevoEvaluadorConfianzaReal(riesgo)

	err := acl.RegistrarResultado(context.Background(), puertos.ResultadoIntento{
		Accion:            "registro",
		CorreoNormalizado: "Ana@Ejemplo.com",
		Origen:            origenDePrueba(t, "203.0.113.9"),
		Exitoso:           true,
		UsuarioID:         "id-de-prueba",
	})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if riesgo.resultadoRecibido.Accion != dominio.AccionRegistro {
		t.Errorf("Accion = %q", riesgo.resultadoRecibido.Accion)
	}
	if riesgo.resultadoRecibido.CorreoNormalizado != "ana@ejemplo.com" {
		t.Errorf("CorreoNormalizado = %q", riesgo.resultadoRecibido.CorreoNormalizado)
	}
	if !riesgo.resultadoRecibido.Exitoso {
		t.Errorf("Exitoso no se tradujo")
	}
}
