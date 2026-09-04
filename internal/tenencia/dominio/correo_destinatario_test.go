package dominio

import (
	"errors"
	"strings"
	"testing"
)

func TestNuevoCorreoDestinatario(t *testing.T) {
	casos := []struct {
		nombre   string
		crudo    string
		esValido bool
		esperado string
	}{
		{"válido simple", "usuario@ejemplo.com", true, "usuario@ejemplo.com"},
		{"se pasa a minúsculas", "Usuario@Ejemplo.COM", true, "usuario@ejemplo.com"},
		{"recorta espacios", "  usuario@ejemplo.com  ", true, "usuario@ejemplo.com"},
		{"vacío", "", false, ""},
		{"con espacio interno", "usu ario@ejemplo.com", false, ""},
		{"con caracter de control", "usuario@ejemplo.com\x00", false, ""},
		{"con CRLF", "usuario@ejemplo.com\r\n", false, ""},
		{"sin arroba", "usuarioejemplo.com", false, ""},
		{"dos arrobas", "usu@ario@ejemplo.com", false, ""},
		{"parte local vacía", "@ejemplo.com", false, ""},
		{"dominio sin punto", "usuario@ejemplo", false, ""},
		{"dominio empieza con punto", "usuario@.ejemplo.com", false, ""},
		{"dominio termina con punto", "usuario@ejemplo.com.", false, ""},
		{"dominio con puntos consecutivos", "usuario@ejemplo..com", false, ""},
		{"supera longitud máxima", strings.Repeat("a", 250) + "@e.com", false, ""},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			correo, err := NuevoCorreoDestinatario(c.crudo)
			if c.esValido {
				if err != nil {
					t.Fatalf("no se esperaba error: %v", err)
				}
				if correo.Normalizado() != c.esperado {
					t.Errorf("Normalizado() = %q, esperado %q", correo.Normalizado(), c.esperado)
				}
				return
			}
			if err == nil {
				t.Fatal("se esperaba error")
			}
			var errCorreo *ErrCorreoDestinatarioInvalido
			if !errors.As(err, &errCorreo) {
				t.Errorf("se esperaba *ErrCorreoDestinatarioInvalido, obtuvo %T", err)
			}
		})
	}
}

func TestCorreoDestinatario_EsIgual(t *testing.T) {
	a, _ := NuevoCorreoDestinatario("Usuario@Ejemplo.com")
	b, _ := NuevoCorreoDestinatario("usuario@ejemplo.com")
	if !a.EsIgual(b) {
		t.Error("dos correos que normalizan igual deben ser iguales")
	}
}

// TestNuevoCorreoDestinatario_NFC_FormasConvergen verifica que dos formas
// Unicode distintas del mismo correo visual (precompuesta vs. descompuesta)
// convergen al mismo valor normalizado, para que la comparación de
// destinatarios (INV-TEN-21) no dependa de qué forma tecleó el cliente.
func TestNuevoCorreoDestinatario_NFC_FormasConvergen(t *testing.T) {
	precompuesta := "jos\u00e9@ejemplo.com"  // é como un solo code point (U+00E9)
	descompuesta := "jose\u0301@ejemplo.com" // "e" + acento combinante (U+0065 U+0301)
	a, err := NuevoCorreoDestinatario(precompuesta)
	if err != nil {
		t.Fatalf("no se esperaba error con la forma precompuesta: %v", err)
	}
	b, err := NuevoCorreoDestinatario(descompuesta)
	if err != nil {
		t.Fatalf("no se esperaba error con la forma descompuesta: %v", err)
	}
	if !a.EsIgual(b) {
		t.Errorf("las dos formas Unicode deben normalizar al mismo valor: %q vs %q", a.Normalizado(), b.Normalizado())
	}
}

func TestCorreoDestinatario_EsIgualConstante(t *testing.T) {
	a, _ := NuevoCorreoDestinatario("usuario@ejemplo.com")
	b, _ := NuevoCorreoDestinatario("usuario@ejemplo.com")
	c, _ := NuevoCorreoDestinatario("otro@ejemplo.com")
	if !a.EsIgualConstante(b) {
		t.Error("correos iguales deben compararse iguales en tiempo constante")
	}
	if a.EsIgualConstante(c) {
		t.Error("correos distintos no deben compararse iguales")
	}
}

func TestCorreoDestinatario_EsVacio(t *testing.T) {
	var vacio CorreoDestinatario
	if !vacio.EsVacio() {
		t.Error("el zero value debe estar vacío")
	}
	c, _ := NuevoCorreoDestinatario("usuario@ejemplo.com")
	if c.EsVacio() {
		t.Error("un correo construido no debe estar vacío")
	}
}
