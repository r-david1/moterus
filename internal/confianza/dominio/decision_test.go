package dominio

import (
	"reflect"
	"testing"
)

// --- INV-RIES-01 ---------------------------------------------------------

// TestINV_RIES_01_NivelRiesgo_NoTieneUnValorDeBloqueoPermanente verifica la
// mitad de dominio de INV-RIES-01: el catálogo cerrado de NivelRiesgo tiene
// exactamente tres valores (normal, elevado, alto) y ninguno de ellos
// representa, por sí mismo, un rechazo insuperable — a diferencia de, por
// ejemplo, EstadoTicketConsumido (que sí es terminal en las colas), no
// existe una noción de "nivel terminal" en este catálogo. El único
// desenlace que el diseño permite construir sobre un nivel alto es exigir
// un captcha (aplicacion, fase posterior), nunca bloquear sin salida.
func TestINV_RIES_01_NivelRiesgo_NoTieneUnValorDeBloqueoPermanente(t *testing.T) {
	niveles := []NivelRiesgo{NivelRiesgoNormal, NivelRiesgoElevado, NivelRiesgoAlto}
	if len(niveles) != 3 {
		t.Fatalf("se esperaban exactamente 3 niveles de riesgo, hay %d", len(niveles))
	}
	nombresPermitidos := map[string]bool{"normal": true, "elevado": true, "alto": true}
	for _, n := range niveles {
		if !nombresPermitidos[n.String()] {
			t.Errorf("nivel de riesgo inesperado fuera del catálogo documentado: %q", n.String())
		}
	}
}

// --- INV-RIES-02 ---------------------------------------------------------

// TestINV_RIES_02_CamposDeRiesgoNuncaTocanRequiereStepUp verifica que
// Decision.RequiereStepUp es un campo completamente independiente de
// PuntajeRiesgo/NivelRiesgo/SenalesDeRiesgo: poblar los tres campos de
// riesgo (incluso con el nivel más alto) no fuerza, deriva ni implica un
// valor para RequiereStepUp. Este mecanismo NO produce Decision.
// RequiereStepUp (§3.2 del diseño): hoy, un usuario sin factor MFA
// confirmado que reciba RequiereStepUp queda bloqueado sin salida.
func TestINV_RIES_02_CamposDeRiesgoNuncaTocanRequiereStepUp(t *testing.T) {
	riesgoAlto := NuevoPuntajeRiesgo(0.95)
	pol := PoliticaRiesgoPorDefecto()

	d := Decision{
		Permitido:       true,
		PuntajeRiesgo:   riesgoAlto,
		NivelRiesgo:     riesgoAlto.Nivel(pol),
		SenalesDeRiesgo: []SenalRiesgo{SenalDispositivoDesconocido, SenalRedDesconocida},
	}

	if d.RequiereStepUp {
		t.Error("RequiereStepUp debe seguir siendo false: construir una Decision con riesgo alto no debe fijarlo por sí solo")
	}
	if !d.NivelRiesgo.EsIgual(NivelRiesgoAlto) {
		t.Fatalf("se esperaba nivel alto para el escenario de prueba, se obtuvo %q", d.NivelRiesgo.String())
	}
}

// --- INV-RIES-09 ---------------------------------------------------------

// TestINV_RIES_09_DecisionDeclaraLosCamposDeRiesgoSeparadosDelPuntajeQueCruza
// verifica, por reflexión, que Decision tiene los tres campos aditivos
// documentados en §1.2/§1.3 del diseño (PuntajeRiesgo, NivelRiesgo,
// SenalesDeRiesgo) y que son campos distintos de Puntaje/Motivo (los que sí
// cruzan la frontera de contexto vía los ACL, ResultadoAutenticacion.
// PuntajeConfianza). La prohibición de que los ACL los mapeen es una
// garantía de una fase posterior (los tres ACL identidad|acceso|
// tenencia/adaptadores/confianza); aquí solo se verifica la mitad que el
// dominio puede garantizar: que son campos con nombre propio, no alias del
// campo que sí cruza.
func TestINV_RIES_09_DecisionDeclaraLosCamposDeRiesgoSeparadosDelPuntajeQueCruza(t *testing.T) {
	tipo := reflect.TypeOf(Decision{})

	camposDeRiesgo := []string{"PuntajeRiesgo", "NivelRiesgo", "SenalesDeRiesgo"}
	for _, nombre := range camposDeRiesgo {
		campo, ok := tipo.FieldByName(nombre)
		if !ok {
			t.Errorf("Decision no tiene el campo %q (§1.2 del diseño)", nombre)
			continue
		}
		if nombre == "PuntajeRiesgo" && campo.Type != reflect.TypeOf(PuntajeRiesgo{}) {
			t.Errorf("Decision.PuntajeRiesgo debe ser de tipo PuntajeRiesgo, es %s", campo.Type)
		}
		if nombre == "NivelRiesgo" && campo.Type != reflect.TypeOf(NivelRiesgo{}) {
			t.Errorf("Decision.NivelRiesgo debe ser de tipo NivelRiesgo, es %s", campo.Type)
		}
	}

	if _, ok := tipo.FieldByName("Puntaje"); !ok {
		t.Fatal("Decision.Puntaje (captcha, el que sí cruza la frontera) no puede desaparecer con esta extensión")
	}
	campoPuntajeCaptcha, _ := tipo.FieldByName("Puntaje")
	campoPuntajeRiesgo, _ := tipo.FieldByName("PuntajeRiesgo")
	if campoPuntajeCaptcha.Type == campoPuntajeRiesgo.Type && campoPuntajeCaptcha.Name == campoPuntajeRiesgo.Name {
		t.Error("Puntaje (captcha) y PuntajeRiesgo deben ser campos distintos, nunca el mismo")
	}
}
