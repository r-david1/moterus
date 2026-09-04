package dominio

import (
	"errors"
	"testing"
)

func TestIDOrganizacionDesde(t *testing.T) {
	casos := []struct {
		nombre   string
		valor    string
		esValido bool
	}{
		{"válido", "018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6d", true},
		{"válido con mayúsculas", "018E6F2A-9C3D-7C3A-8B3A-1E2F3A4B5C6D", true},
		{"vacío", "", false},
		{"UUID nulo", "00000000-0000-0000-0000-000000000000", false},
		{"formato inválido", "no-es-un-uuid", false},
		{"con espacios", "  018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6d  ", true},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			id, err := IDOrganizacionDesde(c.valor)
			if c.esValido && err != nil {
				t.Errorf("se esperaba éxito, obtuvo error: %v", err)
			}
			if !c.esValido {
				if err == nil {
					t.Fatal("se esperaba error")
				}
				var errID *ErrIDOrganizacionInvalido
				if !errors.As(err, &errID) {
					t.Errorf("se esperaba *ErrIDOrganizacionInvalido, obtuvo %T", err)
				}
				return
			}
			if id.EsVacio() {
				t.Error("el ID construido no debería estar vacío")
			}
		})
	}
}

func TestIDOrganizacion_StringYEsIgual(t *testing.T) {
	a, _ := IDOrganizacionDesde("018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6d")
	b, _ := IDOrganizacionDesde("018E6F2A-9C3D-7C3A-8B3A-1E2F3A4B5C6D")
	if a.String() != "018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6d" {
		t.Errorf("String() = %q", a.String())
	}
	if !a.EsIgual(b) {
		t.Error("dos UUIDs iguales salvo mayúsculas deben ser iguales tras la normalización")
	}
	var vacio IDOrganizacion
	if !vacio.EsVacio() {
		t.Error("el zero value debe estar vacío")
	}
}

func TestIDMembresiaDesde(t *testing.T) {
	if _, err := IDMembresiaDesde(""); err == nil {
		t.Fatal("se esperaba error con valor vacío")
	}
	if _, err := IDMembresiaDesde("00000000-0000-0000-0000-000000000000"); err == nil {
		t.Fatal("se esperaba error con el UUID nulo")
	} else {
		var errID *ErrIDMembresiaInvalido
		if !errors.As(err, &errID) {
			t.Errorf("se esperaba *ErrIDMembresiaInvalido, obtuvo %T", err)
		}
	}
	id, err := IDMembresiaDesde("018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6d")
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if id.EsVacio() {
		t.Error("no debería estar vacío")
	}
	otro, _ := IDMembresiaDesde("018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6e")
	if id.EsIgual(otro) {
		t.Error("identificadores distintos no deben ser iguales")
	}
	igual, _ := IDMembresiaDesde("018E6F2A-9C3D-7C3A-8B3A-1E2F3A4B5C6D")
	if !id.EsIgual(igual) {
		t.Error("el mismo UUID salvo mayúsculas debe ser igual")
	}
}

func TestIDInvitacionDesde(t *testing.T) {
	if _, err := IDInvitacionDesde("00000000-0000-0000-0000-000000000000"); err == nil {
		t.Fatal("se esperaba error con el UUID nulo")
	} else {
		var errID *ErrIDInvitacionInvalido
		if !errors.As(err, &errID) {
			t.Errorf("se esperaba *ErrIDInvitacionInvalido, obtuvo %T", err)
		}
	}
	if _, err := IDInvitacionDesde("no-es-un-uuid"); err == nil {
		t.Fatal("se esperaba error con formato inválido")
	}
	id, err := IDInvitacionDesde("018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6d")
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if id.EsVacio() {
		t.Error("no debería estar vacío")
	}
	otro, _ := IDInvitacionDesde("018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6e")
	if id.EsIgual(otro) {
		t.Error("identificadores distintos no deben ser iguales")
	}
}

func TestIDUsuarioDesde(t *testing.T) {
	if _, err := IDUsuarioDesde("abc"); err == nil {
		t.Fatal("se esperaba error con formato inválido")
	}
	id, err := IDUsuarioDesde("018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6d")
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if id.EsVacio() {
		t.Error("no debería estar vacío")
	}
	otro, _ := IDUsuarioDesde("018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6e")
	if id.EsIgual(otro) {
		t.Error("identificadores distintos no deben ser iguales")
	}
}
