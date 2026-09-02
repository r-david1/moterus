package dominio

import "testing"

func TestEstadoSesionDesde_CatalogoCerrado(t *testing.T) {
	validos := []string{"activa", "revocada", "expirada"}
	for _, v := range validos {
		if _, err := EstadoSesionDesde(v); err != nil {
			t.Errorf("EstadoSesionDesde(%q): no se esperaba error, obtuvo %v", v, err)
		}
	}
	if _, err := EstadoSesionDesde("pendiente_segundo_factor"); err == nil {
		t.Error("EstadoSesionDesde(\"pendiente_segundo_factor\"): se esperaba error, ese estado es fase 2 y no forma parte del catálogo del MVP")
	}
	if _, err := EstadoSesionDesde("otro"); err == nil {
		t.Error("se esperaba error para un valor fuera del catálogo cerrado")
	}
}

// TestINV_ACC_07_EstadosTerminales verifica la máquina de estados del §1.4
// del diseño: activa transiciona a revocada o expirada; ambos son
// terminales y no admiten ninguna transición de salida.
func TestINV_ACC_07_EstadosTerminales(t *testing.T) {
	if !EstadoSesionActiva.PuedeTransicionarA(EstadoSesionRevocada) {
		t.Error("activa debe poder transicionar a revocada")
	}
	if !EstadoSesionActiva.PuedeTransicionarA(EstadoSesionExpirada) {
		t.Error("activa debe poder transicionar a expirada")
	}
	terminales := []EstadoSesion{EstadoSesionRevocada, EstadoSesionExpirada}
	destinos := []EstadoSesion{EstadoSesionActiva, EstadoSesionRevocada, EstadoSesionExpirada}
	for _, origen := range terminales {
		if !origen.EsTerminal() {
			t.Errorf("%s debe reportarse como terminal", origen)
		}
		for _, destino := range destinos {
			if origen.PuedeTransicionarA(destino) {
				t.Errorf("%s no debe poder transicionar a %s: es terminal (INV-ACC-07)", origen, destino)
			}
		}
	}
	if EstadoSesionActiva.EsTerminal() {
		t.Error("activa no debe reportarse como terminal")
	}
}

func TestEstadoSesion_EsIgual(t *testing.T) {
	a, _ := EstadoSesionDesde("activa")
	if !a.EsIgual(EstadoSesionActiva) {
		t.Error("un EstadoSesion construido desde \"activa\" debe ser igual a EstadoSesionActiva")
	}
}
