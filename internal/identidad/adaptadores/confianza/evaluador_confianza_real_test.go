package confianza_test

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
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

// origenConHuellaDePrueba es como origenDePrueba pero con una huella de
// dispositivo no vacía, para los tests del reconocimiento de origen
// (docs/design/fingerprinting-comportamiento.md §8).
func origenConHuellaDePrueba(t *testing.T, ip, huella string) iddominio.OrigenSolicitud {
	t.Helper()
	origen, err := iddominio.NuevoOrigenSolicitud(ip, "agente-de-prueba", huella, "id-solicitud-de-prueba")
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

// TestEvaluar_pueblaHuellaDispositivo cubre el campo aditivo de
// confianza/puertos.Solicitud (§2.1 y §8 del diseño de reconocimiento de
// origen): el ACL debe pasar la huella que ya trae identidad/dominio.
// OrigenSolicitud, sin inventar una firma nueva.
func TestEvaluar_pueblaHuellaDispositivo(t *testing.T) {
	riesgo := &riesgoFalso{}
	acl := identidadconfianza.NuevoEvaluadorConfianzaReal(riesgo)

	_, err := acl.Evaluar(context.Background(), puertos.SolicitudEvaluacion{
		Accion:            "login",
		CorreoNormalizado: "ana@ejemplo.com",
		Origen:            origenConHuellaDePrueba(t, "203.0.113.9", "huella-cruda-123"),
	})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if riesgo.solicitudRecibida.HuellaDispositivo != "huella-cruda-123" {
		t.Errorf("HuellaDispositivo = %q, esperado huella-cruda-123", riesgo.solicitudRecibida.HuellaDispositivo)
	}
}

// TestRegistrarResultado_pueblaCamposAditivosDeReconocimientoDeOrigen cubre
// HuellaDispositivo, IDUsuario e IDSolicitud (§2.1 y §8 del diseño de
// reconocimiento de origen): los tres ya llegaban al ACL vía
// puertos.ResultadoIntento (UsuarioID) y Origen (huella, id de solicitud),
// y hasta esta extensión se descartaban al traducir.
func TestRegistrarResultado_pueblaCamposAditivosDeReconocimientoDeOrigen(t *testing.T) {
	riesgo := &riesgoFalso{}
	acl := identidadconfianza.NuevoEvaluadorConfianzaReal(riesgo)

	err := acl.RegistrarResultado(context.Background(), puertos.ResultadoIntento{
		Accion:            "login",
		CorreoNormalizado: "ana@ejemplo.com",
		Origen:            origenConHuellaDePrueba(t, "203.0.113.9", "huella-cruda-456"),
		Exitoso:           true,
		UsuarioID:         "usuario-789",
	})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if riesgo.resultadoRecibido.HuellaDispositivo != "huella-cruda-456" {
		t.Errorf("HuellaDispositivo = %q, esperado huella-cruda-456", riesgo.resultadoRecibido.HuellaDispositivo)
	}
	if riesgo.resultadoRecibido.IDUsuario != "usuario-789" {
		t.Errorf("IDUsuario = %q, esperado usuario-789", riesgo.resultadoRecibido.IDUsuario)
	}
	if riesgo.resultadoRecibido.IDSolicitud != "id-solicitud-de-prueba" {
		t.Errorf("IDSolicitud = %q, esperado id-solicitud-de-prueba", riesgo.resultadoRecibido.IDSolicitud)
	}
}

// TestEvaluadorConfianzaReal_NuncaExponeCamposDeRiesgo custodia INV-RIES-09
// (docs/design/fingerprinting-comportamiento.md §4 y §8): el archivo fuente
// del ACL nunca debe mencionar los identificadores PuntajeRiesgo,
// NivelRiesgo ni SenalesDeRiesgo. Ninguno de los tres puede llegar a
// identidad/puertos.DecisionConfianza y, de ahí, a una respuesta HTTP. Un
// test de reflexión sobre DecisionConfianza no alcanzaría por sí solo: hoy
// el tipo ni siquiera declara esos campos, así que el peligro real es que
// alguien los agregue Y los mapee en el mismo cambio; este test falla en
// cuanto aparece la mención en el código fuente del ACL, sin esperar a que
// el tipo cambie.
func TestEvaluadorConfianzaReal_NuncaExponeCamposDeRiesgo(t *testing.T) {
	const archivo = "evaluador_confianza_real.go"
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, archivo, nil, 0)
	if err != nil {
		t.Fatalf("no se pudo parsear %s: %v", archivo, err)
	}
	prohibidos := map[string]bool{
		"PuntajeRiesgo":   true,
		"NivelRiesgo":     true,
		"SenalesDeRiesgo": true,
	}
	ast.Inspect(f, func(n ast.Node) bool {
		ident, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		if prohibidos[ident.Name] {
			pos := fset.Position(ident.Pos())
			t.Errorf("%s:%d menciona %q: INV-RIES-09 prohíbe que PuntajeRiesgo/NivelRiesgo/SenalesDeRiesgo crucen la frontera de contexto hacia identidad/puertos.DecisionConfianza", archivo, pos.Line, ident.Name)
		}
		return true
	})
}
