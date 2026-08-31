package dominio

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

const hashValido = "$argon2id$v=19$m=65536,t=3,p=1$c2FsdHNhbHQ$aGFzaGhhc2g"

func TestNuevoHashContrasena_Valido(t *testing.T) {
	h, err := NuevoHashContrasena(hashValido)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if h.Algoritmo() != "argon2id" {
		t.Errorf("Algoritmo() = %q, esperado argon2id", h.Algoritmo())
	}
	if h.Valor() != hashValido {
		t.Errorf("Valor() = %q, esperado %q", h.Valor(), hashValido)
	}
}

func TestNuevoHashContrasena_Invalido(t *testing.T) {
	casos := []string{"", "  ", "no-es-phc", "sinformato$argon2id", "$", "$$resto"}
	for _, c := range casos {
		if _, err := NuevoHashContrasena(c); err == nil {
			t.Errorf("NuevoHashContrasena(%q) debía fallar", c)
		}
	}
}

func TestHashContrasena_Algoritmo_ValorVacioSinFormato(t *testing.T) {
	var h HashContrasena
	if h.Algoritmo() != "" {
		t.Errorf("Algoritmo() sobre un HashContrasena vacío = %q, esperado \"\"", h.Algoritmo())
	}
}

func TestErroresDeConstruccion_Credencial(t *testing.T) {
	errHash := &ErrHashContrasenaInvalido{Motivo: "x"}
	if errHash.Error() == "" {
		t.Error("ErrHashContrasenaInvalido.Error() no debe estar vacío")
	}
	errCred := &ErrCredencialInvalida{Motivo: "x"}
	if errCred.Error() == "" {
		t.Error("ErrCredencialInvalida.Error() no debe estar vacío")
	}
	errPlana := &ErrContrasenaPlanaInvalida{Motivo: "x"}
	if errPlana.Error() == "" {
		t.Error("ErrContrasenaPlanaInvalida.Error() no debe estar vacío")
	}
}

// TestINV_ID_04_HashContrasena_Redactado verifica que el hash nunca se
// serializa en logs ni respuestas (tabla 1.3).
func TestINV_ID_04_HashContrasena_Redactado(t *testing.T) {
	h, err := NuevoHashContrasena(hashValido)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if h.String() != "[REDACTADO]" {
		t.Errorf("String() = %q, esperado [REDACTADO]", h.String())
	}
	if h.GoString() != "[REDACTADO]" {
		t.Errorf("GoString() = %q, esperado [REDACTADO]", h.GoString())
	}
	b, err := json.Marshal(h)
	if err != nil {
		t.Fatalf("error inesperado al serializar: %v", err)
	}
	if string(b) != `"[REDACTADO]"` {
		t.Errorf("MarshalJSON() = %s, esperado \"[REDACTADO]\"", b)
	}
	if strings.Contains(h.String()+h.GoString()+string(b), "argon2id") {
		t.Error("la representación redactada no debe filtrar ningún fragmento del hash real")
	}
}

func TestNuevaCredencial(t *testing.T) {
	h, _ := NuevoHashContrasena(hashValido)
	ahora := time.Now()
	c, err := NuevaCredencial(h, ahora)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if c.Hash().Valor() != hashValido {
		t.Errorf("Hash().Valor() = %q, esperado %q", c.Hash().Valor(), hashValido)
	}
	if !c.ActualizadaEn().Equal(ahora) {
		t.Errorf("ActualizadaEn() = %v, esperado %v", c.ActualizadaEn(), ahora)
	}
}

func TestNuevaCredencial_RechazaHashVacio(t *testing.T) {
	var h HashContrasena
	if _, err := NuevaCredencial(h, time.Now()); err == nil {
		t.Error("se esperaba error al construir una credencial con hash vacío (INV-ID-01)")
	}
}

func TestNuevaContrasenaPlana_Valida(t *testing.T) {
	p, err := NuevaContrasenaPlana("una-contrasena-cualquiera")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if p.Longitud() != len("una-contrasena-cualquiera") {
		t.Errorf("Longitud() = %d, esperado %d", p.Longitud(), len("una-contrasena-cualquiera"))
	}
	if p.Valor() != "una-contrasena-cualquiera" {
		t.Error("Valor() debe devolver la contraseña en claro para el hasher")
	}
}

func TestNuevaContrasenaPlana_RechazaVacia(t *testing.T) {
	if _, err := NuevaContrasenaPlana(""); err == nil {
		t.Error("se esperaba error con contraseña vacía")
	}
}

func TestNuevaContrasenaPlana_RechazaExcesivamenteLarga(t *testing.T) {
	larga := strings.Repeat("a", 4097)
	if _, err := NuevaContrasenaPlana(larga); err == nil {
		t.Error("se esperaba error por longitud excesiva (prevención de DoS de hashing)")
	}
	limite := strings.Repeat("a", 4096)
	if _, err := NuevaContrasenaPlana(limite); err != nil {
		t.Errorf("4096 bytes debe ser aceptado, obtuvo error: %v", err)
	}
}

// TestINV_ID_04_ContrasenaPlana_Redactada verifica que la contraseña en
// claro nunca aparece en su propia representación textual.
func TestINV_ID_04_ContrasenaPlana_Redactada(t *testing.T) {
	secreto := "SuperSecreta123!"
	p, err := NuevaContrasenaPlana(secreto)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if p.String() != "[REDACTADO]" {
		t.Errorf("String() = %q, esperado [REDACTADO]", p.String())
	}
	if p.GoString() != "[REDACTADO]" {
		t.Errorf("GoString() = %q, esperado [REDACTADO]", p.GoString())
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("error inesperado al serializar: %v", err)
	}
	if string(b) != `"[REDACTADO]"` {
		t.Errorf("MarshalJSON() = %s, esperado \"[REDACTADO]\"", b)
	}
	todo := p.String() + p.GoString() + string(b)
	if strings.Contains(todo, secreto) {
		t.Error("la representación redactada no debe filtrar la contraseña real")
	}

	// Verificación adicional: fmt.Sprintf con %v/%#v tampoco debe filtrar.
	viaSprintf := fmt.Sprintf("%v %#v", p, p)
	if strings.Contains(viaSprintf, secreto) {
		t.Errorf("fmt.Sprintf no debe filtrar la contraseña: %q", viaSprintf)
	}
}
