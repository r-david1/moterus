package dominio

import (
	"errors"
	"strings"
	"testing"
)

func TestNuevoCorreo_Valido(t *testing.T) {
	casos := []struct {
		crudo    string
		esperado string
	}{
		{"Ana@Ejemplo.COM", "ana@ejemplo.com"},
		{"  ana@ejemplo.com  ", "ana@ejemplo.com"},
		{"a@b.co", "a@b.co"},
	}
	for _, c := range casos {
		correo, err := NuevoCorreo(c.crudo)
		if err != nil {
			t.Fatalf("NuevoCorreo(%q) devolvió error inesperado: %v", c.crudo, err)
		}
		if correo.Normalizado() != c.esperado {
			t.Errorf("NuevoCorreo(%q).Normalizado() = %q, esperado %q", c.crudo, correo.Normalizado(), c.esperado)
		}
	}
}

// TestINV_ID_correo_minusculas_completas verifica la tabla 1.3: minúsculas
// completas tanto en el dominio como en la parte local (ADR 0015).
func TestINV_ID_correo_minusculas_completas(t *testing.T) {
	correo, err := NuevoCorreo("Usuario.Ejemplo@Dominio.COM")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if correo.Normalizado() != "usuario.ejemplo@dominio.com" {
		t.Errorf("se esperaba minúsculas completas, obtuvo %q", correo.Normalizado())
	}
	if correo.Dominio() != "dominio.com" {
		t.Errorf("Dominio() = %q, esperado dominio.com", correo.Dominio())
	}
}

func TestNuevoCorreo_Invalido(t *testing.T) {
	casos := []string{
		"",
		"   ",
		"sin-arroba.com",
		"dos@arrobas@dominio.com",
		"@dominio.com",
		"usuario@",
		"usuario@dominiosinpunto",
		"usuario @dominio.com",
		"usuario@.dominio.com",
		"usuario@dominio.com.",
		"usuario@dom..inio.com",
	}
	for _, crudo := range casos {
		_, err := NuevoCorreo(crudo)
		if err == nil {
			t.Errorf("NuevoCorreo(%q) debía fallar y no falló", crudo)
			continue
		}
		var errCorreo *ErrCorreoInvalido
		if !errors.As(err, &errCorreo) {
			t.Errorf("NuevoCorreo(%q) devolvió un error que no es *ErrCorreoInvalido: %T", crudo, err)
		}
	}
}

// TestNuevoCorreo_RechazaControlYCRLF cubre la prevención de header
// injection aguas abajo (tabla 1.3).
func TestNuevoCorreo_RechazaControlYCRLF(t *testing.T) {
	casos := []string{
		"usuario@dominio.com\r\nBcc: victima@otro.com",
		"usuario@dominio.com\n",
		"usu\tario@dominio.com",
		"usuario@domi\x00nio.com",
	}
	for _, crudo := range casos {
		_, err := NuevoCorreo(crudo)
		if err == nil {
			t.Errorf("NuevoCorreo(%q) debía rechazar caracteres de control/CRLF", crudo)
		}
	}
}

func TestNuevoCorreo_RechazaLongitudExcesiva(t *testing.T) {
	local := ""
	for i := 0; i < 250; i++ {
		local += "a"
	}
	crudo := local + "@dominio.com"
	if _, err := NuevoCorreo(crudo); err == nil {
		t.Errorf("se esperaba error por longitud excesiva")
	}
}

// TestErrCorreoInvalido_NoExponeElValorCompleto verifica que el mensaje de
// error no incluye el valor original recibido (tabla 1.5: "nunca el valor
// completo en el mensaje").
func TestErrCorreoInvalido_NoExponeElValorCompleto(t *testing.T) {
	secreto := "correo-muy-secreto-y-especifico@dominio-privado-unico.com"
	_, err := NuevoCorreo(secreto + "@extra@invalido")
	if err == nil {
		t.Fatal("se esperaba un error")
	}
	if strings.Contains(err.Error(), secreto) {
		t.Errorf("el mensaje de error no debe contener el valor completo del correo: %q", err.Error())
	}
}

func TestCorreo_EsVacio(t *testing.T) {
	var c Correo
	if !c.EsVacio() {
		t.Error("el zero value de Correo debe reportarse como vacío")
	}
	c2, _ := NuevoCorreo("a@b.com")
	if c2.EsVacio() {
		t.Error("un correo construido no debe reportarse como vacío")
	}
}

func TestCorreo_Dominio_VacioSinArroba(t *testing.T) {
	var c Correo
	if c.Dominio() != "" {
		t.Errorf("Dominio() sobre un Correo vacío = %q, esperado \"\"", c.Dominio())
	}
}

func TestCorreo_String(t *testing.T) {
	c, _ := NuevoCorreo("a@b.com")
	if c.String() != "a@b.com" {
		t.Errorf("String() = %q, esperado a@b.com", c.String())
	}
}

// TestNuevoCorreo_NFC_FormasEquivalentesConvergen verifica que dos formas
// Unicode distintas del mismo correo visual (precompuesta vs. descompuesta
// con acento combinante) normalicen al mismo valor — INV-ID-02 depende de
// esto para que la unicidad no se pueda burlar cambiando la codificación.
func TestNuevoCorreo_NFC_FormasEquivalentesConvergen(t *testing.T) {
	precompuesta := "josé@dominio.com"  // "é" como un solo code point (U+00E9)
	descompuesta := "josé@dominio.com" // "e" + acento combinante (U+0301)

	if precompuesta == descompuesta {
		t.Fatal("el fixture del test está mal construido: ambas cadenas ya son iguales byte a byte")
	}

	c1, err := NuevoCorreo(precompuesta)
	if err != nil {
		t.Fatalf("NuevoCorreo(precompuesta) devolvió error inesperado: %v", err)
	}
	c2, err := NuevoCorreo(descompuesta)
	if err != nil {
		t.Fatalf("NuevoCorreo(descompuesta) devolvió error inesperado: %v", err)
	}
	if c1.Normalizado() != c2.Normalizado() {
		t.Errorf("las dos formas Unicode debían normalizar igual: %q != %q", c1.Normalizado(), c2.Normalizado())
	}
}
