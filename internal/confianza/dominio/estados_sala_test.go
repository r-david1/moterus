package dominio

import "testing"

func TestEstadoSalaDesde_CatalogoCerrado(t *testing.T) {
	if _, err := EstadoSalaDesde("inexistente"); err == nil {
		t.Error("se esperaba error para un valor fuera del catálogo")
	}
	for _, v := range []EstadoSala{EstadoSalaProgramada, EstadoSalaAbierta, EstadoSalaDrenando, EstadoSalaCerrada} {
		e, err := EstadoSalaDesde(v.String())
		if err != nil || !e.EsIgual(v) {
			t.Errorf("EstadoSalaDesde(%q) = %v, %v", v.String(), e, err)
		}
	}
}

func TestEstadoSala_EsVacio(t *testing.T) {
	var e EstadoSala
	if !e.EsVacio() {
		t.Error("el zero value debería estar vacío")
	}
	if EstadoSalaAbierta.EsVacio() {
		t.Error("un estado válido no debería estar vacío")
	}
}

func TestEstadoSala_EsTerminal(t *testing.T) {
	if !EstadoSalaCerrada.EsTerminal() {
		t.Error("cerrada debería ser terminal")
	}
	for _, e := range []EstadoSala{EstadoSalaProgramada, EstadoSalaAbierta, EstadoSalaDrenando} {
		if e.EsTerminal() {
			t.Errorf("%s no debería ser terminal", e.String())
		}
	}
}

// TestEstadoSala_MaquinaDeEstados cubre exhaustivamente la máquina de
// estados del §1.6 del diseño: cada transición legal declarada, y ninguna
// otra.
func TestEstadoSala_MaquinaDeEstados(t *testing.T) {
	todos := []EstadoSala{EstadoSalaProgramada, EstadoSalaAbierta, EstadoSalaDrenando, EstadoSalaCerrada}
	legales := map[string]bool{
		"programada->abierta": true,
		"abierta->drenando":   true,
		"abierta->cerrada":    true,
		"drenando->cerrada":   true,
		"drenando->abierta":   true,
	}
	for _, origen := range todos {
		for _, destino := range todos {
			clave := origen.String() + "->" + destino.String()
			esperado := legales[clave]
			obtenido := origen.PuedeTransicionarA(destino)
			if obtenido != esperado {
				t.Errorf("PuedeTransicionarA(%s -> %s) = %v, esperado %v", origen.String(), destino.String(), obtenido, esperado)
			}
		}
	}
}

func TestDesenlaceDeAdmisionDesde_CatalogoCerrado(t *testing.T) {
	if _, err := DesenlaceDeAdmisionDesde("inexistente"); err == nil {
		t.Error("se esperaba error para un valor fuera del catálogo")
	}
	catalogo := []DesenlaceDeAdmision{
		DesenlaceAdmitido, DesenlaceEsperando, DesenlaceTurnoCaducado,
		DesenlaceTicketDesconocido, DesenlaceTicketConsumido, DesenlaceSalaCerrada, DesenlaceColaLlena,
	}
	if len(catalogo) != 7 {
		t.Fatalf("el catálogo cerrado de DesenlaceDeAdmision debe tener 7 valores, tiene %d", len(catalogo))
	}
	for _, v := range catalogo {
		d, err := DesenlaceDeAdmisionDesde(v.String())
		if err != nil || !d.EsIgual(v) {
			t.Errorf("DesenlaceDeAdmisionDesde(%q) = %v, %v", v.String(), d, err)
		}
	}
}

func TestDesenlaceDeAdmision_EsVacio(t *testing.T) {
	var d DesenlaceDeAdmision
	if !d.EsVacio() {
		t.Error("el zero value debería estar vacío")
	}
}
