package dominio

import (
	"errors"
	"strings"
	"testing"
)

func TestNuevoTokenInvitacionPlano(t *testing.T) {
	secretoValido := strings.Repeat("a", 43)
	casos := []struct {
		nombre   string
		valor    string
		esValido bool
	}{
		{"válido", prefijoTokenInvitacion + secretoValido, true},
		{"válido con más entropía", prefijoTokenInvitacion + strings.Repeat("A-_9", 20), true},
		{"vacío", "", false},
		{"prefijo desconocido", "mot_rt_" + secretoValido, false},
		{"sin prefijo", secretoValido, false},
		{"secreto demasiado corto", prefijoTokenInvitacion + strings.Repeat("a", 42), false},
		{"caracter no base64url", prefijoTokenInvitacion + strings.Repeat("a", 42) + "!", false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			tok, err := NuevoTokenInvitacionPlano(c.valor)
			if c.esValido {
				if err != nil {
					t.Fatalf("no se esperaba error: %v", err)
				}
				if tok.Valor() != c.valor {
					t.Errorf("Valor() = %q, esperado %q", tok.Valor(), c.valor)
				}
				return
			}
			if err == nil {
				t.Fatal("se esperaba error")
			}
			var errTok *ErrTokenInvitacionPlanoInvalido
			if !errors.As(err, &errTok) {
				t.Errorf("se esperaba *ErrTokenInvitacionPlanoInvalido, obtuvo %T", err)
			}
		})
	}
}

// TestINV_TEN_23_TokenInvitacionPlano_SeRedacta verifica que el token en
// claro nunca se filtra por String(), GoString() ni MarshalJSON.
func TestINV_TEN_23_TokenInvitacionPlano_SeRedacta(t *testing.T) {
	tok, err := NuevoTokenInvitacionPlano(prefijoTokenInvitacion + strings.Repeat("a", 43))
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
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
	// Valor() sigue exponiendo el secreto real: es la única vía legítima
	// (NotificadorInvitaciones).
	if !strings.HasPrefix(tok.Valor(), prefijoTokenInvitacion) {
		t.Error("Valor() debe seguir exponiendo el token real para el notificador")
	}
}

func TestTokenInvitacionPlano_Hash(t *testing.T) {
	tok, _ := NuevoTokenInvitacionPlano(prefijoTokenInvitacion + strings.Repeat("a", 43))
	h1 := tok.Hash()
	h2 := HashearTokenInvitacion(tok)
	if !h1.EsIgual(h2) {
		t.Error("Hash() y HashearTokenInvitacion() deben producir el mismo resultado")
	}
	if len(h1.Valor()) != 64 {
		t.Errorf("longitud del hash = %d, esperado 64", len(h1.Valor()))
	}
}

func TestNuevoHashTokenInvitacion(t *testing.T) {
	valido := strings.Repeat("a", 64)
	if _, err := NuevoHashTokenInvitacion(valido); err != nil {
		t.Errorf("no se esperaba error: %v", err)
	}
	casos := []string{
		"",
		strings.Repeat("a", 63),
		strings.Repeat("a", 65),
		strings.Repeat("A", 64), // mayúsculas: no es hex minúscula
		strings.Repeat("g", 64), // fuera del alfabeto hex
	}
	for _, c := range casos {
		if _, err := NuevoHashTokenInvitacion(c); err == nil {
			t.Errorf("valor %q debería ser inválido", c)
		} else {
			var errHash *ErrHashTokenInvitacionInvalido
			if !errors.As(err, &errHash) {
				t.Errorf("se esperaba *ErrHashTokenInvitacionInvalido, obtuvo %T", err)
			}
		}
	}
}

func TestHashTokenInvitacion_EsIgual(t *testing.T) {
	a, _ := NuevoHashTokenInvitacion(strings.Repeat("a", 64))
	b, _ := NuevoHashTokenInvitacion(strings.Repeat("a", 64))
	c, _ := NuevoHashTokenInvitacion(strings.Repeat("b", 64))
	if !a.EsIgual(b) {
		t.Error("hashes iguales deben compararse iguales")
	}
	if a.EsIgual(c) {
		t.Error("hashes distintos no deben compararse iguales")
	}
}

func TestHashTokenInvitacion_EsVacio(t *testing.T) {
	var vacio HashTokenInvitacion
	if !vacio.EsVacio() {
		t.Error("el zero value debe estar vacío")
	}
}
