package dominio

import "testing"

func TestIDSalaDeEsperaDesde_RechazaFormatoInvalido(t *testing.T) {
	casos := []string{"", "no-es-un-uuid", "018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6", "018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6zz"}
	for _, c := range casos {
		if _, err := IDSalaDeEsperaDesde(c); err == nil {
			t.Errorf("IDSalaDeEsperaDesde(%q) = nil, se esperaba error", c)
		}
	}
}

func TestIDSalaDeEsperaDesde_RechazaUUIDNulo(t *testing.T) {
	if _, err := IDSalaDeEsperaDesde("00000000-0000-0000-0000-000000000000"); err == nil {
		t.Error("se esperaba error para el UUID nulo")
	}
}

func TestIDSalaDeEsperaDesde_AceptaUUIDValidoYNormalizaAMinusculas(t *testing.T) {
	id, err := IDSalaDeEsperaDesde("018E6F2A-9C3D-7C3A-8B3A-1E2F3A4B5C6D")
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if id.String() != "018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6d" {
		t.Errorf("String() = %q, esperado en minúsculas", id.String())
	}
	if id.EsVacio() {
		t.Error("no debería estar vacío")
	}
}

func TestIDSalaDeEspera_EsIgual(t *testing.T) {
	a, _ := IDSalaDeEsperaDesde("018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6d")
	b, _ := IDSalaDeEsperaDesde("018E6F2A-9C3D-7C3A-8B3A-1E2F3A4B5C6D")
	c, _ := IDSalaDeEsperaDesde("018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6e")
	if !a.EsIgual(b) {
		t.Error("mismos UUID normalizados deberían ser iguales")
	}
	if a.EsIgual(c) {
		t.Error("UUID distintos no deberían ser iguales")
	}
}

func TestIDSalaDeEspera_ZeroValue_EsVacio(t *testing.T) {
	var id IDSalaDeEspera
	if !id.EsVacio() {
		t.Error("el zero value debería estar vacío")
	}
}

func TestIDOrganizacionDesde_ValidaFormaYNulo(t *testing.T) {
	if _, err := IDOrganizacionDesde("invalido"); err == nil {
		t.Error("se esperaba error para formato inválido")
	}
	if _, err := IDOrganizacionDesde("00000000-0000-0000-0000-000000000000"); err == nil {
		t.Error("se esperaba error para el UUID nulo")
	}
	id, err := IDOrganizacionDesde("018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6d")
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if id.EsVacio() {
		t.Error("no debería estar vacío")
	}
}

func TestIDUsuarioDesde_ValidaFormaYNulo(t *testing.T) {
	if _, err := IDUsuarioDesde("invalido"); err == nil {
		t.Error("se esperaba error para formato inválido")
	}
	if _, err := IDUsuarioDesde("00000000-0000-0000-0000-000000000000"); err == nil {
		t.Error("se esperaba error para el UUID nulo")
	}
	id, err := IDUsuarioDesde("018E6F2A-9C3D-7C3A-8B3A-1E2F3A4B5C6D")
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if id.EsVacio() {
		t.Error("no debería estar vacío")
	}
	if id.String() != "018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6d" {
		t.Errorf("String() = %q, esperado en minúsculas", id.String())
	}
	otro, _ := IDUsuarioDesde("018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6d")
	if !id.EsIgual(otro) {
		t.Error("mismos UUID normalizados deberían ser iguales")
	}
}
