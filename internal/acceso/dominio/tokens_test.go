package dominio

import (
	"strings"
	"testing"
)

func tokenRefrescoPlanoDePrueba(t *testing.T) TokenRefrescoPlano {
	t.Helper()
	tok, err := NuevoTokenRefrescoPlano("mot_rt_" + strings.Repeat("A", 43))
	if err != nil {
		t.Fatalf("no se pudo construir el token de refresco de prueba: %v", err)
	}
	return tok
}

func TestNuevoTokenRefrescoPlano_AceptaFormatoValido(t *testing.T) {
	casos := []string{
		"mot_rt_" + strings.Repeat("A", 43),
		"mot_rt_" + strings.Repeat("a", 60),
		"mot_rt_" + strings.Repeat("A-b_9", 20),
	}
	for _, c := range casos {
		if _, err := NuevoTokenRefrescoPlano(c); err != nil {
			t.Errorf("NuevoTokenRefrescoPlano(%q): no se esperaba error, obtuvo %v", c, err)
		}
	}
}

// TestNuevoTokenRefrescoPlano_RechazoEstructural verifica el rechazo barato
// sin tocar la base de datos descrito en §3.2 paso 2 del diseño: prefijo
// desconocido o longitud insuficiente se rechazan en el constructor.
func TestNuevoTokenRefrescoPlano_RechazoEstructural(t *testing.T) {
	casos := map[string]string{
		"vacío":                "",
		"sin prefijo":          strings.Repeat("A", 50),
		"prefijo distinto":     "otro_prefijo_" + strings.Repeat("A", 43),
		"secreto corto":        "mot_rt_" + strings.Repeat("A", 10),
		"caracteres no base64": "mot_rt_" + strings.Repeat("A", 42) + "!",
	}
	for nombre, valor := range casos {
		if _, err := NuevoTokenRefrescoPlano(valor); err == nil {
			t.Errorf("caso %q: se esperaba error para %q", nombre, valor)
		}
	}
}

// TestINV_ACC_11_TokenRefrescoPlanoSeRedacta verifica que un
// TokenRefrescoPlano nunca expone su valor por accidente vía String(),
// GoString() o MarshalJSON (INV-ACC-11).
func TestINV_ACC_11_TokenRefrescoPlanoSeRedacta(t *testing.T) {
	tok := tokenRefrescoPlanoDePrueba(t)
	if tok.String() != "[REDACTADO]" {
		t.Errorf("String() = %q, esperado [REDACTADO]", tok.String())
	}
	if tok.GoString() != "[REDACTADO]" {
		t.Errorf("GoString() = %q, esperado [REDACTADO]", tok.GoString())
	}
	b, err := tok.MarshalJSON()
	if err != nil {
		t.Fatalf("no se esperaba error de MarshalJSON: %v", err)
	}
	if string(b) != `"[REDACTADO]"` {
		t.Errorf("MarshalJSON() = %s, esperado \"[REDACTADO]\"", b)
	}
}

func TestTokenRefrescoPlano_EsVacio(t *testing.T) {
	var vacio TokenRefrescoPlano
	if !vacio.EsVacio() {
		t.Error("el zero value de TokenRefrescoPlano debe reportarse vacío")
	}
	if tokenRefrescoPlanoDePrueba(t).EsVacio() {
		t.Error("un TokenRefrescoPlano construido no debe reportarse vacío")
	}
}

func TestTokenRefrescoPlano_Hash_ProduceHashValidoYDeterminista(t *testing.T) {
	tok := tokenRefrescoPlanoDePrueba(t)
	h1 := tok.Hash()
	h2 := tok.Hash()
	if !h1.EsIgual(h2) {
		t.Error("hashear el mismo token dos veces debe producir el mismo hash")
	}
	if len(h1.Valor()) != 64 {
		t.Errorf("longitud del hash = %d, esperado 64", len(h1.Valor()))
	}
}

func TestTokenRefrescoPlano_Hash_DistinguePlanos(t *testing.T) {
	a, _ := NuevoTokenRefrescoPlano("mot_rt_" + strings.Repeat("A", 43))
	b, _ := NuevoTokenRefrescoPlano("mot_rt_" + strings.Repeat("B", 43))
	if a.Hash().EsIgual(b.Hash()) {
		t.Error("tokens distintos no deben producir el mismo hash")
	}
}

func TestNuevoHashTokenRefresco_ValidaFormato(t *testing.T) {
	valido := strings.Repeat("a", 64)
	if _, err := NuevoHashTokenRefresco(valido); err != nil {
		t.Errorf("no se esperaba error para un hash de 64 hex minúsculas, obtuvo %v", err)
	}
	invalidos := []string{
		"",
		strings.Repeat("a", 63),
		strings.Repeat("A", 64), // mayúsculas: no es la forma canónica
		strings.Repeat("g", 64), // no es hexadecimal
	}
	for _, v := range invalidos {
		if _, err := NuevoHashTokenRefresco(v); err == nil {
			t.Errorf("NuevoHashTokenRefresco(%q): se esperaba error", v)
		}
	}
}

func TestHashTokenRefresco_EsIgual(t *testing.T) {
	a, _ := NuevoHashTokenRefresco(strings.Repeat("a", 64))
	b, _ := NuevoHashTokenRefresco(strings.Repeat("a", 64))
	c, _ := NuevoHashTokenRefresco(strings.Repeat("b", 64))
	if !a.EsIgual(b) {
		t.Error("dos hashes iguales deben reportarse iguales")
	}
	if a.EsIgual(c) {
		t.Error("dos hashes distintos no deben reportarse iguales")
	}
}
