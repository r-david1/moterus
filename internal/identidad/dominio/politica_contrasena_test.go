package dominio

import (
	"errors"
	"strings"
	"testing"
)

func correoDePrueba(t *testing.T, crudo string) Correo {
	t.Helper()
	c, err := NuevoCorreo(crudo)
	if err != nil {
		t.Fatalf("no se pudo construir el correo de prueba %q: %v", crudo, err)
	}
	return c
}

func contrasenaDePrueba(t *testing.T, valor string) ContrasenaPlana {
	t.Helper()
	p, err := NuevaContrasenaPlana(valor)
	if err != nil {
		t.Fatalf("no se pudo construir la contraseña de prueba: %v", err)
	}
	return p
}

func TestPoliticaContrasena_AceptaContrasenaFuerte(t *testing.T) {
	politica := NuevaPoliticaContrasena()
	correo := correoDePrueba(t, "usuario@ejemplo.com")
	p := contrasenaDePrueba(t, "correcto caballo batería grapa")
	if err := politica.Evaluar(p, correo); err != nil {
		t.Errorf("se esperaba que una contraseña larga sin patrones triviales fuera aceptada: %v", err)
	}
}

// TestINV_ID_PoliticaContrasena_LongitudMinima verifica el mínimo NIST SP
// 800-63B de 12 caracteres.
func TestINV_ID_PoliticaContrasena_LongitudMinima(t *testing.T) {
	politica := NuevaPoliticaContrasena()
	correo := correoDePrueba(t, "usuario@ejemplo.com")
	corta := contrasenaDePrueba(t, "corta12345") // 10 caracteres
	err := politica.Evaluar(corta, correo)
	if err == nil {
		t.Fatal("se esperaba ErrContrasenaDebil por longitud insuficiente")
	}
	var errDebil *ErrContrasenaDebil
	if !errors.As(err, &errDebil) {
		t.Fatalf("se esperaba *ErrContrasenaDebil, obtuvo %T", err)
	}
	if !contieneRegla(errDebil.Reglas, "12 caracteres") {
		t.Errorf("las reglas incumplidas deben mencionar el mínimo de 12 caracteres: %v", errDebil.Reglas)
	}
}

func TestPoliticaContrasena_RechazaLongitudExcesiva(t *testing.T) {
	politica := NuevaPoliticaContrasena()
	correo := correoDePrueba(t, "usuario@ejemplo.com")
	larga := contrasenaDePrueba(t, strings.Repeat("x", 129))
	if err := politica.Evaluar(larga, correo); err == nil {
		t.Error("se esperaba ErrContrasenaDebil por longitud superior a 128")
	}
}

// TestPoliticaContrasena_SinReglasDeComposicion documenta explícitamente
// que la política NIST SP 800-63B no exige mayúsculas, números ni símbolos.
func TestPoliticaContrasena_SinReglasDeComposicion(t *testing.T) {
	politica := NuevaPoliticaContrasena()
	correo := correoDePrueba(t, "usuario@ejemplo.com")
	todaMinuscula := contrasenaDePrueba(t, "todaminusculaynonumeros")
	if err := politica.Evaluar(todaMinuscula, correo); err != nil {
		t.Errorf("una contraseña larga sin mayúsculas/números/símbolos debe aceptarse: %v", err)
	}
}

func TestPoliticaContrasena_RechazaSiContieneElCorreo(t *testing.T) {
	politica := NuevaPoliticaContrasena()
	correo := correoDePrueba(t, "ana.perez@ejemplo.com")
	p := contrasenaDePrueba(t, "miclaveana.perez@ejemplo.com")
	err := politica.Evaluar(p, correo)
	if err == nil {
		t.Fatal("se esperaba ErrContrasenaDebil por contener el correo")
	}
	var errDebil *ErrContrasenaDebil
	errors.As(err, &errDebil)
	if !contieneRegla(errDebil.Reglas, "correo") {
		t.Errorf("las reglas incumplidas deben mencionar el correo: %v", errDebil.Reglas)
	}
}

func TestPoliticaContrasena_RechazaSiContieneElDominioDelCorreo(t *testing.T) {
	politica := NuevaPoliticaContrasena()
	correo := correoDePrueba(t, "ana@empresaacme.com")
	p := contrasenaDePrueba(t, "trabajoenempresaacme.com123")
	if err := politica.Evaluar(p, correo); err == nil {
		t.Error("se esperaba ErrContrasenaDebil por contener el dominio del correo")
	}
}

func TestPoliticaContrasena_RechazaSecuenciasTriviales(t *testing.T) {
	politica := NuevaPoliticaContrasena()
	correo := correoDePrueba(t, "usuario@ejemplo.com")
	casos := []string{
		"aaaaaaaaaaaa",
		"contrasena12345",
		"password12345",
		"123456789012",
	}
	for _, c := range casos {
		p := contrasenaDePrueba(t, c)
		if err := politica.Evaluar(p, correo); err == nil {
			t.Errorf("Evaluar(%q) debía rechazar la secuencia trivial", c)
		}
	}
}

// TestPoliticaContrasena_RechazaSecuenciaNumericaConsecutivaNoDiccionario
// cubre la corrida de dígitos ascendentes (>= 6) que no coincide con
// ninguna palabra del catálogo mínimo, para ejercer esa rama específica de
// tieneSecuenciaNumericaConsecutiva.
func TestPoliticaContrasena_RechazaSecuenciaNumericaConsecutivaNoDiccionario(t *testing.T) {
	politica := NuevaPoliticaContrasena()
	correo := correoDePrueba(t, "usuario@ejemplo.com")
	p := contrasenaDePrueba(t, "ab345678cdxy")
	if err := politica.Evaluar(p, correo); err == nil {
		t.Error("se esperaba ErrContrasenaDebil por la corrida numérica ascendente 345678")
	}
}

// TestEsSecuenciaTrivial_CadenaVacia cubre la guarda defensiva de la
// función interna esSecuenciaTrivial, que Evaluar nunca alcanza con una
// cadena vacía porque NuevaContrasenaPlana ya rechaza contraseñas vacías.
func TestEsSecuenciaTrivial_CadenaVacia(t *testing.T) {
	if esSecuenciaTrivial("") {
		t.Error("una cadena vacía no debe considerarse una secuencia trivial")
	}
}

func contieneRegla(reglas []string, fragmento string) bool {
	for _, r := range reglas {
		if strings.Contains(r, fragmento) {
			return true
		}
	}
	return false
}
