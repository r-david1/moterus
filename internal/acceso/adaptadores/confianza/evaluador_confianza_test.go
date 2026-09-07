package confianza_test

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	accesoconfianza "github.com/r-david1/moterus/internal/acceso/adaptadores/confianza"
	accesodominio "github.com/r-david1/moterus/internal/acceso/dominio"
	"github.com/r-david1/moterus/internal/acceso/puertos"
	"github.com/r-david1/moterus/internal/confianza/dominio"
	confianzapuertos "github.com/r-david1/moterus/internal/confianza/puertos"
)

// riesgoFalso es un test double mínimo de confianza/puertos.EvaluadorDeRiesgo
// (mismo criterio que identidad/adaptadores/confianza: este test verifica
// la traducción del ACL, no el caso de uso de Confianza).
type riesgoFalso struct {
	solicitudRecibida confianzapuertos.Solicitud
	resultadoRecibido confianzapuertos.ResultadoIntento
	decisionADevolver dominio.Decision
}

func (r *riesgoFalso) Evaluar(_ context.Context, s confianzapuertos.Solicitud) (dominio.Decision, error) {
	r.solicitudRecibida = s
	return r.decisionADevolver, nil
}

func (r *riesgoFalso) RegistrarResultado(_ context.Context, res confianzapuertos.ResultadoIntento) error {
	r.resultadoRecibido = res
	return nil
}

func origenConHuellaDePrueba(t *testing.T, ip, huella string) accesodominio.OrigenSolicitud {
	t.Helper()
	origen, err := accesodominio.NuevoOrigenSolicitud(ip, "agente-de-prueba", huella, "id-solicitud-de-prueba")
	if err != nil {
		t.Fatalf("no se pudo construir OrigenSolicitud: %v", err)
	}
	return origen
}

// TestEvaluar_pueblaHuellaDispositivo cubre el campo aditivo de
// confianza/puertos.Solicitud (§2.1 y §8 de
// docs/design/fingerprinting-comportamiento.md).
func TestEvaluar_pueblaHuellaDispositivo(t *testing.T) {
	riesgo := &riesgoFalso{}
	acl := accesoconfianza.NuevoEvaluadorConfianzaReal(riesgo)

	_, err := acl.Evaluar(context.Background(), puertos.SolicitudEvaluacion{
		Accion:      "renovacion_sesion",
		ClaveCuenta: "sesion:abc",
		Origen:      origenConHuellaDePrueba(t, "203.0.113.9", "huella-cruda-123"),
	})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if riesgo.solicitudRecibida.HuellaDispositivo != "huella-cruda-123" {
		t.Errorf("HuellaDispositivo = %q, esperado huella-cruda-123", riesgo.solicitudRecibida.HuellaDispositivo)
	}
}

// TestRegistrarResultado_pueblaCamposAditivosDeReconocimientoDeOrigen cubre
// HuellaDispositivo, IDUsuario e IDSolicitud: acceso/puertos.ResultadoIntento
// ya llevaba Origen (huella, id de solicitud) e IDUsuario, y hasta esta
// extensión el ACL los descartaba al traducir.
func TestRegistrarResultado_pueblaCamposAditivosDeReconocimientoDeOrigen(t *testing.T) {
	riesgo := &riesgoFalso{}
	acl := accesoconfianza.NuevoEvaluadorConfianzaReal(riesgo)

	err := acl.RegistrarResultado(context.Background(), puertos.ResultadoIntento{
		Accion:      "renovacion_sesion",
		ClaveCuenta: "sesion:abc",
		Origen:      origenConHuellaDePrueba(t, "203.0.113.9", "huella-cruda-456"),
		Exitoso:     true,
		IDUsuario:   "usuario-789",
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
// del ACL nunca debe mencionar PuntajeRiesgo, NivelRiesgo ni
// SenalesDeRiesgo. Mismo criterio que el test homónimo de
// identidad/adaptadores/confianza.
func TestEvaluadorConfianzaReal_NuncaExponeCamposDeRiesgo(t *testing.T) {
	const archivo = "evaluador_confianza.go"
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
			t.Errorf("%s:%d menciona %q: INV-RIES-09 prohíbe que PuntajeRiesgo/NivelRiesgo/SenalesDeRiesgo crucen la frontera de contexto hacia acceso/puertos.DecisionConfianza", archivo, pos.Line, ident.Name)
		}
		return true
	})
}
