package dominio

import (
	"errors"
	"testing"
	"time"
)

func TestNuevoRitmoAdmision_ValidaRango(t *testing.T) {
	casosInvalidos := []int{0, -1, 10001, 100000}
	for _, v := range casosInvalidos {
		if _, err := NuevoRitmoAdmision(v); err == nil {
			t.Errorf("NuevoRitmoAdmision(%d) = nil, se esperaba error", v)
		}
	}
	casosValidos := []int{1, 50, 10000}
	for _, v := range casosValidos {
		r, err := NuevoRitmoAdmision(v)
		if err != nil {
			t.Errorf("NuevoRitmoAdmision(%d) = error %v, no se esperaba", v, err)
		}
		if r.PorSegundo() != v {
			t.Errorf("PorSegundo() = %d, esperado %d", r.PorSegundo(), v)
		}
	}
}

func TestRitmoAdmision_EsIgualYEsVacio(t *testing.T) {
	var r RitmoAdmision
	if !r.EsVacio() {
		t.Error("el zero value debería estar vacío")
	}
	a, _ := NuevoRitmoAdmision(50)
	b, _ := NuevoRitmoAdmision(50)
	c, _ := NuevoRitmoAdmision(51)
	if !a.EsIgual(b) {
		t.Error("mismos ritmos deberían ser iguales")
	}
	if a.EsIgual(c) {
		t.Error("ritmos distintos no deberían ser iguales")
	}
}

func TestModoDegradadoDesde_CatalogoCerrado(t *testing.T) {
	if _, err := ModoDegradadoDesde("bloquear"); err == nil {
		t.Error("se esperaba error para un valor fuera del catálogo")
	}
	permitir, err := ModoDegradadoDesde("permitir")
	if err != nil || !permitir.EsPermitir() {
		t.Errorf("ModoDegradadoDesde(permitir) = %v, %v", permitir, err)
	}
	rechazar, err := ModoDegradadoDesde("rechazar")
	if err != nil || rechazar.EsPermitir() {
		t.Errorf("ModoDegradadoDesde(rechazar) = %v, %v", rechazar, err)
	}
}

func politicaValidaDePrueba(t *testing.T) PoliticaSala {
	t.Helper()
	ritmo, _ := NuevoRitmoAdmision(50)
	p, err := NuevaPoliticaSala(ritmo, 500_000, 2*time.Minute, ModoDegradadoPermitir)
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	return p
}

func TestNuevaPoliticaSala_ValidaRangos(t *testing.T) {
	ritmoValido, _ := NuevoRitmoAdmision(50)

	casos := []struct {
		nombre      string
		capacidad   int64
		ventana     time.Duration
		modo        ModoDegradado
		esperaExito bool
	}{
		{"capacidad por debajo del mínimo", 99, 2 * time.Minute, ModoDegradadoPermitir, false},
		{"capacidad por encima del máximo", 5_000_001, 2 * time.Minute, ModoDegradadoPermitir, false},
		{"ventana por debajo del mínimo", 500_000, 29 * time.Second, ModoDegradadoPermitir, false},
		{"ventana por encima del máximo", 500_000, 16 * time.Minute, ModoDegradadoPermitir, false},
		{"modo degradado vacío", 500_000, 2 * time.Minute, ModoDegradado{}, false},
		{"todo válido en el borde inferior", 100, 30 * time.Second, ModoDegradadoRechazar, true},
		{"todo válido en el borde superior", 5_000_000, 15 * time.Minute, ModoDegradadoPermitir, true},
	}
	for _, c := range casos {
		_, err := NuevaPoliticaSala(ritmoValido, c.capacidad, c.ventana, c.modo)
		if c.esperaExito && err != nil {
			t.Errorf("%s: no se esperaba error, obtuvo %v", c.nombre, err)
		}
		if !c.esperaExito && err == nil {
			t.Errorf("%s: se esperaba error", c.nombre)
		}
	}
}

func TestNuevaPoliticaSala_RitmoVacio(t *testing.T) {
	if _, err := NuevaPoliticaSala(RitmoAdmision{}, 500_000, 2*time.Minute, ModoDegradadoPermitir); err == nil {
		t.Fatal("se esperaba error con ritmoAdmision vacío")
	} else {
		var errPol *ErrPoliticaSalaInvalida
		if !errors.As(err, &errPol) {
			t.Errorf("se esperaba *ErrPoliticaSalaInvalida, obtuvo %T", err)
		}
	}
}

func TestPoliticaSalaPorDefecto_EsValida(t *testing.T) {
	p := PoliticaSalaPorDefecto()
	if p.RitmoAdmision().PorSegundo() != 50 {
		t.Errorf("ritmo por defecto = %d, esperado 50", p.RitmoAdmision().PorSegundo())
	}
	if p.CapacidadMaximaCola() != 500_000 {
		t.Errorf("capacidad por defecto = %d, esperado 500000", p.CapacidadMaximaCola())
	}
	if p.VentanaReclamo() != 2*time.Minute {
		t.Errorf("ventana por defecto = %v, esperado 2m", p.VentanaReclamo())
	}
	if !p.ModoDegradado().EsPermitir() {
		t.Error("el modo por defecto debe ser permitir (fail-open)")
	}
}

func TestPoliticaSala_ConRitmoAdmision_NoMutaElOriginal(t *testing.T) {
	p := politicaValidaDePrueba(t)
	nuevo, _ := NuevoRitmoAdmision(120)
	p2 := p.ConRitmoAdmision(nuevo)
	if p.RitmoAdmision().PorSegundo() != 50 {
		t.Error("ConRitmoAdmision no debería mutar la política original (value receiver)")
	}
	if p2.RitmoAdmision().PorSegundo() != 120 {
		t.Errorf("p2.RitmoAdmision() = %d, esperado 120", p2.RitmoAdmision().PorSegundo())
	}
	if p2.CapacidadMaximaCola() != p.CapacidadMaximaCola() || p2.VentanaReclamo() != p.VentanaReclamo() {
		t.Error("ConRitmoAdmision no debería alterar los demás parámetros")
	}
}

func TestPoliticaSala_EsVacio(t *testing.T) {
	var p PoliticaSala
	if !p.EsVacio() {
		t.Error("el zero value debería estar vacío")
	}
	if politicaValidaDePrueba(t).EsVacio() {
		t.Error("una política válida no debería estar vacía")
	}
}
