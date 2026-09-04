package dominio

import (
	"errors"
	"strings"
	"testing"
)

func TestNuevoAlias(t *testing.T) {
	casos := []struct {
		nombre   string
		crudo    string
		esValido bool
		esperado string
	}{
		{"válido simple", "acme", true, "acme"},
		{"se pasa a minúsculas", "Acme", true, "acme"},
		{"con guiones internos", "acme-corp", true, "acme-corp"},
		{"con dígitos", "acme123", true, "acme123"},
		{"recorta espacios", "  acme  ", true, "acme"},
		{"longitud mínima 3", "ab", false, ""},
		{"longitud exacta mínima", "abc", true, "abc"},
		{"longitud máxima 48", strings.Repeat("a", 49), false, ""},
		{"longitud exacta máxima", strings.Repeat("a", 48), true, strings.Repeat("a", 48)},
		{"vacío", "", false, ""},
		{"solo espacios", "   ", false, ""},
		{"empieza con guion", "-acme", false, ""},
		{"termina con guion", "acme-", false, ""},
		{"guiones consecutivos", "ac--me", false, ""},
		{"caracter no permitido: espacio interno", "ac me", false, ""},
		{"caracter no permitido: guion bajo", "ac_me", false, ""},
		{"caracter no permitido: punto", "ac.me", false, ""},
		{"reservado admin", "admin", false, ""},
		{"reservado api", "api", false, ""},
		{"reservado acceso", "acceso", false, ""},
		{"reservado identidad", "identidad", false, ""},
		{"reservado tenencia", "tenencia", false, ""},
		{"reservado well-known", "well-known", false, ""},
		{"reservado nuevo", "nuevo", false, ""},
		{"caracter no ASCII rechazado (solo [a-z0-9-])", "cafe-corp-é", false, ""},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			a, err := NuevoAlias(c.crudo)
			if c.esValido {
				if err != nil {
					t.Fatalf("no se esperaba error: %v", err)
				}
				if a.Normalizado() != c.esperado {
					t.Errorf("Normalizado() = %q, esperado %q", a.Normalizado(), c.esperado)
				}
				return
			}
			if err == nil {
				t.Fatal("se esperaba error")
			}
			var errAlias *ErrAliasInvalido
			if !errors.As(err, &errAlias) {
				t.Errorf("se esperaba *ErrAliasInvalido, obtuvo %T", err)
			}
		})
	}
}

func TestAliasOrganizacion_EsIgual(t *testing.T) {
	a, _ := NuevoAlias("Acme")
	b, _ := NuevoAlias("acme")
	if !a.EsIgual(b) {
		t.Error("dos alias que normalizan igual deben ser iguales")
	}
	c, _ := NuevoAlias("acme-corp")
	if a.EsIgual(c) {
		t.Error("alias distintos no deben ser iguales")
	}
}

func TestAliasOrganizacion_EsVacio(t *testing.T) {
	var vacio AliasOrganizacion
	if !vacio.EsVacio() {
		t.Error("el zero value debe estar vacío")
	}
	a, _ := NuevoAlias("acme")
	if a.EsVacio() {
		t.Error("un alias construido no debe estar vacío")
	}
}

func TestNuevoNombre(t *testing.T) {
	casos := []struct {
		nombre   string
		crudo    string
		esValido bool
	}{
		{"válido con mayúsculas conservadas", "Acme Corp", true},
		{"vacío", "", false},
		{"solo espacios", "   ", false},
		{"longitud máxima 120", strings.Repeat("a", 120), true},
		{"supera longitud máxima", strings.Repeat("a", 121), false},
		{"con caracter de control", "Acme\x00Corp", false},
		{"con CRLF", "Acme\r\nCorp", false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			_, err := NuevoNombre(c.crudo)
			if c.esValido && err != nil {
				t.Errorf("no se esperaba error: %v", err)
			}
			if !c.esValido {
				if err == nil {
					t.Fatal("se esperaba error")
				}
				var errNombre *ErrNombreOrganizacionInvalido
				if !errors.As(err, &errNombre) {
					t.Errorf("se esperaba *ErrNombreOrganizacionInvalido, obtuvo %T", err)
				}
			}
		})
	}
}

func TestNuevoNombre_NoSePasaAMinusculas(t *testing.T) {
	n, err := NuevoNombre("Acme Corp S.A.")
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if n.Valor() != "Acme Corp S.A." {
		t.Errorf("Valor() = %q, el nombre no debe pasarse a minúsculas (es texto de presentación)", n.Valor())
	}
}

func TestNuevoNombre_RecortaEspacios(t *testing.T) {
	n, err := NuevoNombre("  Acme Corp  ")
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if n.Valor() != "Acme Corp" {
		t.Errorf("Valor() = %q, esperado %q", n.Valor(), "Acme Corp")
	}
}

func TestNombreOrganizacion_EsIgualYEsVacio(t *testing.T) {
	var vacio NombreOrganizacion
	if !vacio.EsVacio() {
		t.Error("el zero value debe estar vacío")
	}
	a, _ := NuevoNombre("Acme")
	b, _ := NuevoNombre("Acme")
	if !a.EsIgual(b) {
		t.Error("dos nombres idénticos deben ser iguales")
	}
	c, _ := NuevoNombre("acme")
	if a.EsIgual(c) {
		t.Error("nombre no se normaliza a minúsculas, por lo que 'Acme' != 'acme'")
	}
}
