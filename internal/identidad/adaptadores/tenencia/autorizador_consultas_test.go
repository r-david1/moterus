package tenencia

import "testing"

// TestPermisoMiembroVer_CoincideConElCatalogoDeTenencia fija la dependencia
// dura entre la constante local permisoMiembroVer y
// tenencia/dominio.PermisoMiembroVer: si el catálogo cerrado de permisos de
// Tenencia cambiara ese valor, este test lo detecta en vez de dejar que
// AutorizadorConsultas empiece a pedir un permiso inexistente en silencio.
func TestPermisoMiembroVer_CoincideConElCatalogoDeTenencia(t *testing.T) {
	if !permisoMiembroVerCoincideConElCatalogo() {
		t.Fatal("la constante local permisoMiembroVer se desincronizó de tenencia/dominio.PermisoMiembroVer")
	}
}
