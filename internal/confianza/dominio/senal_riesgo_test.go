package dominio

import (
	"errors"
	"testing"
)

// --- SenalRiesgo -----------------------------------------------------------

func TestSenalRiesgoDesde_AceptaElCatalogoCerrado(t *testing.T) {
	casos := []SenalRiesgo{SenalDispositivoDesconocido, SenalHuellaAusente, SenalRedDesconocida}
	for _, c := range casos {
		s, err := SenalRiesgoDesde(c.String())
		if err != nil {
			t.Errorf("SenalRiesgoDesde(%q) no debería fallar: %v", c.String(), err)
		}
		if !s.EsIgual(c) {
			t.Errorf("SenalRiesgoDesde(%q) = %q, esperado %q", c.String(), s.String(), c.String())
		}
	}
}

// TestINV_RIES_?? no aplica un número propio aquí: esta prueba cubre la
// forma de catálogo cerrado en la que se apoyan varias invariantes
// (INV-RIES-12 en particular, que exige que solo viajen códigos del
// catálogo).
func TestSenalRiesgoDesde_RechazaValoresFueraDelCatalogo(t *testing.T) {
	casos := []string{"", "desconocida", "DISPOSITIVO_DESCONOCIDO", "dispositivo desconocido"}
	for _, c := range casos {
		if _, err := SenalRiesgoDesde(c); err == nil {
			t.Errorf("SenalRiesgoDesde(%q) debería fallar", c)
		} else {
			var errEsperado *ErrSenalRiesgoDesconocida
			if !errors.As(err, &errEsperado) {
				t.Errorf("SenalRiesgoDesde(%q) devolvió un error de tipo inesperado: %T", c, err)
			}
		}
	}
}

func TestSenalRiesgo_EsVaciaYEsIgual(t *testing.T) {
	var cero SenalRiesgo
	if !cero.EsVacia() {
		t.Error("el cero value de SenalRiesgo debe reportarse como vacío")
	}
	if SenalDispositivoDesconocido.EsIgual(SenalHuellaAusente) {
		t.Error("dos señales distintas no deben ser iguales")
	}
	if !SenalDispositivoDesconocido.EsIgual(SenalDispositivoDesconocido) {
		t.Error("una señal debe ser igual a sí misma")
	}
}

// --- PuntajeRiesgo -----------------------------------------------------------

func TestNuevoPuntajeRiesgo_SaturaEnVezDeFallar(t *testing.T) {
	casos := []struct {
		entrada, esperado float64
	}{
		{-5.0, 0.0},
		{0.0, 0.0},
		{0.42, 0.42},
		{1.0, 1.0},
		{5.0, 1.0},
	}
	for _, c := range casos {
		p := NuevoPuntajeRiesgo(c.entrada)
		if p.Valor() != c.esperado {
			t.Errorf("NuevoPuntajeRiesgo(%v).Valor() = %v, esperado %v", c.entrada, p.Valor(), c.esperado)
		}
	}
}

// TestINV_RIES_?? no aplica: PuntajeRiesgo saturando en vez de fallar no
// tiene un número de invariante propio en la tabla del diseño, pero es la
// garantía textual de la tabla de VOs de §1.3 y se prueba explícitamente
// para que nadie la reemplace por un error de dominio en el camino
// caliente.
func TestNuevoPuntajeRiesgo_NuncaFalla(t *testing.T) {
	// NuevoPuntajeRiesgo no devuelve error: esto se verifica en tiempo de
	// compilación por su firma (func(float64) PuntajeRiesgo). Este test
	// documenta la intención y sirve de ancla si alguien cambia la firma.
	_ = NuevoPuntajeRiesgo(1e300)
}

// --- NivelRiesgo -------------------------------------------------------------

func TestPuntajeRiesgo_Nivel_ClasificaSegunLosUmbrales(t *testing.T) {
	pol := PoliticaRiesgoPorDefecto()
	casos := []struct {
		puntaje float64
		nivel   NivelRiesgo
	}{
		{0.0, NivelRiesgoNormal},
		{0.49, NivelRiesgoNormal},
		{0.50, NivelRiesgoElevado},
		{0.65, NivelRiesgoElevado},
		{0.79, NivelRiesgoElevado},
		{0.80, NivelRiesgoAlto},
		{1.0, NivelRiesgoAlto},
	}
	for _, c := range casos {
		n := NuevoPuntajeRiesgo(c.puntaje).Nivel(pol)
		if !n.EsIgual(c.nivel) {
			t.Errorf("Nivel(%v) = %q, esperado %q", c.puntaje, n.String(), c.nivel.String())
		}
	}
}

func TestNivelRiesgo_AlMenos(t *testing.T) {
	if !NivelRiesgoAlto.AlMenos(NivelRiesgoElevado) {
		t.Error("alto debe ser 'al menos' elevado")
	}
	if !NivelRiesgoElevado.AlMenos(NivelRiesgoElevado) {
		t.Error("elevado debe ser 'al menos' elevado (igualdad incluida)")
	}
	if NivelRiesgoNormal.AlMenos(NivelRiesgoElevado) {
		t.Error("normal no debe ser 'al menos' elevado")
	}
}
