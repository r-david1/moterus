package dominio

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNuevoCodigoRespaldoPlano(t *testing.T) {
	valido := "ABCDEFGH23" // 10 caracteres del alfabeto permitido
	casos := []struct {
		nombre   string
		valor    string
		esValido bool
	}{
		{"válido", valido, true},
		{"válido minúsculas se normaliza", strings.ToLower(valido), true},
		{"vacío", "", false},
		{"corto", valido[:9], false},
		{"largo", valido + "A", false},
		{"contiene 0", "ABCDEFGH20", false},
		{"contiene O", "ABCDEFGHOO", false},
		{"contiene 1", "ABCDEFGH21", false},
		{"contiene I", "ABCDEFGHII", false},
		{"contiene L", "ABCDEFGHLL", false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			cod, err := NuevoCodigoRespaldoPlano(c.valor)
			if c.esValido {
				if err != nil {
					t.Fatalf("no se esperaba error: %v", err)
				}
				if cod.EsVacio() {
					t.Error("no debería resultar vacío")
				}
				return
			}
			if err == nil {
				t.Fatal("se esperaba error")
			}
		})
	}
}

// TestINV_MFA_07_CodigoRespaldoPlano_SeRedacta verifica que el código de
// respaldo en claro nunca se filtra por String(), GoString() ni MarshalJSON.
func TestINV_MFA_07_CodigoRespaldoPlano_SeRedacta(t *testing.T) {
	c, err := NuevoCodigoRespaldoPlano("ABCDEFGH23")
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if c.String() != "[REDACTADO]" {
		t.Errorf("String() = %q, esperado [REDACTADO]", c.String())
	}
	if c.GoString() != "[REDACTADO]" {
		t.Errorf("GoString() = %q, esperado [REDACTADO]", c.GoString())
	}
	b, err := c.MarshalJSON()
	if err != nil {
		t.Fatalf("no se esperaba error de MarshalJSON: %v", err)
	}
	if string(b) != `"[REDACTADO]"` {
		t.Errorf("MarshalJSON() = %s, esperado \"[REDACTADO]\"", b)
	}
	if c.Valor() != "ABCDEFGH23" {
		t.Error("Valor() debe seguir exponiendo el código real para la respuesta de ConfirmarFactorMFA")
	}
}

func TestCodigoRespaldoPlano_Hash(t *testing.T) {
	c, _ := NuevoCodigoRespaldoPlano("ABCDEFGH23")
	h1 := c.Hash()
	h2 := HashearCodigoRespaldo(c)
	if !h1.EsIgual(h2) {
		t.Error("Hash() y HashearCodigoRespaldo() deben producir el mismo resultado")
	}
	if len(h1.Valor()) != 64 {
		t.Errorf("longitud del hash = %d, esperado 64", len(h1.Valor()))
	}
}

func TestNuevoHashCodigoRespaldo(t *testing.T) {
	valido := strings.Repeat("a", 64)
	if _, err := NuevoHashCodigoRespaldo(valido); err != nil {
		t.Errorf("no se esperaba error: %v", err)
	}
	casos := []string{
		"",
		strings.Repeat("a", 63),
		strings.Repeat("a", 65),
		strings.Repeat("A", 64),
		strings.Repeat("g", 64),
	}
	for _, c := range casos {
		if _, err := NuevoHashCodigoRespaldo(c); err == nil {
			t.Errorf("valor %q debería ser inválido", c)
		}
	}
}

func TestHashCodigoRespaldo_EsIgual(t *testing.T) {
	a, _ := NuevoHashCodigoRespaldo(strings.Repeat("a", 64))
	b, _ := NuevoHashCodigoRespaldo(strings.Repeat("a", 64))
	c, _ := NuevoHashCodigoRespaldo(strings.Repeat("b", 64))
	if !a.EsIgual(b) {
		t.Error("hashes iguales deben compararse iguales")
	}
	if a.EsIgual(c) {
		t.Error("hashes distintos no deben compararse iguales")
	}
}

func TestNuevoCodigoRespaldoMFA(t *testing.T) {
	hash, _ := NuevoHashCodigoRespaldo(strings.Repeat("a", 64))
	c, err := NuevoCodigoRespaldoMFA(hash)
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if !c.EstaDisponible() {
		t.Error("un código de respaldo recién creado debe estar disponible")
	}
	if _, tiene := c.UsadoEn(); tiene {
		t.Error("un código de respaldo recién creado no debe tener UsadoEn")
	}
}

func TestNuevoCodigoRespaldoMFA_HashVacioEsInvalido(t *testing.T) {
	if _, err := NuevoCodigoRespaldoMFA(HashCodigoRespaldo{}); err == nil {
		t.Error("un hash vacío debe ser inválido")
	}
}

// TestINV_MFA_06_CodigoRespaldoMFA_SeConsumeUnaSolaVez verifica que un
// código de respaldo consumido nunca vuelve a estar disponible, y que
// intentar consumirlo de nuevo produce ErrCodigoRespaldoYaUsado.
func TestINV_MFA_06_CodigoRespaldoMFA_SeConsumeUnaSolaVez(t *testing.T) {
	hash, _ := NuevoHashCodigoRespaldo(strings.Repeat("a", 64))
	c, _ := NuevoCodigoRespaldoMFA(hash)
	ahora := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	consumido, err := c.Consumir(ahora)
	if err != nil {
		t.Fatalf("no se esperaba error al consumir por primera vez: %v", err)
	}
	if consumido.EstaDisponible() {
		t.Error("un código consumido no debe seguir disponible")
	}
	usadoEn, tiene := consumido.UsadoEn()
	if !tiene || !usadoEn.Equal(ahora) {
		t.Errorf("UsadoEn() = %v, %v; esperado %v, true", usadoEn, tiene, ahora)
	}

	// Segundo intento de consumo sobre el ya consumido: debe fallar.
	_, err = consumido.Consumir(ahora.Add(time.Hour))
	if err == nil {
		t.Fatal("se esperaba ErrCodigoRespaldoYaUsado al reconsumir")
	}
	var yaUsado *ErrCodigoRespaldoYaUsado
	if !errors.As(err, &yaUsado) {
		t.Errorf("se esperaba *ErrCodigoRespaldoYaUsado, obtuvo %T", err)
	}

	// El value object original (antes de consumir) sigue intacto:
	// inmutabilidad de value objects.
	if !c.EstaDisponible() {
		t.Error("el CodigoRespaldoMFA original no debe mutar: Consumir devuelve un nuevo valor")
	}
}

func TestReconstituirCodigoRespaldoMFA(t *testing.T) {
	hash, _ := NuevoHashCodigoRespaldo(strings.Repeat("a", 64))
	usadoEn := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	disponible := ReconstituirCodigoRespaldoMFA(hash, nil)
	if !disponible.EstaDisponible() {
		t.Error("con usadoEn=nil, el código debe reconstituirse disponible")
	}

	usado := ReconstituirCodigoRespaldoMFA(hash, &usadoEn)
	if usado.EstaDisponible() {
		t.Error("con usadoEn no nulo, el código debe reconstituirse como ya consumido")
	}
	got, tiene := usado.UsadoEn()
	if !tiene || !got.Equal(usadoEn) {
		t.Errorf("UsadoEn() = %v, %v; esperado %v, true", got, tiene, usadoEn)
	}
}

func TestErrores_CodigoRespaldo_MensajesNoVacios(t *testing.T) {
	errores := []error{
		&ErrCodigoRespaldoPlanoInvalido{Motivo: "x"},
		&ErrHashCodigoRespaldoInvalido{Motivo: "x"},
		&ErrCodigoRespaldoYaUsado{},
	}
	for _, err := range errores {
		if err.Error() == "" {
			t.Errorf("%T.Error() no debe estar vacío", err)
		}
	}
}
