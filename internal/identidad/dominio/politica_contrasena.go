package dominio

import "strings"

const (
	// longitudMinimaContrasena es el mínimo NIST SP 800-63B.
	longitudMinimaContrasena = 12
	// longitudMaximaContrasenaEvaluada es el máximo evaluado por la
	// política (no confundir con longitudMaximaContrasenaPlana, que es un
	// límite estructural de DoS en ContrasenaPlana).
	longitudMaximaContrasenaEvaluada = 128
)

// secuenciasTriviales es un catálogo mínimo de patrones triviales o
// filtrados con extrema frecuencia. No sustituye a un verificador de
// brechas reales (eso es el puerto VerificadorContrasenasFiltradas).
var secuenciasTriviales = []string{
	"12345678", "123456789", "1234567890",
	"abcdefgh", "qwertyuiop", "password",
	"contrasena", "contraseña", "letmein123",
	"00000000",
}

// PoliticaContrasena es un servicio de dominio puro (sin estado, sin E/S)
// que evalúa una ContrasenaPlana contra las reglas alineadas a NIST SP
// 800-63B: longitud mínima de 12 y máxima evaluada de 128, sin reglas de
// composición, rechazo si contiene el correo o el dominio del correo, y
// rechazo de secuencias triviales. La verificación contra brechas
// conocidas (HIBP) no vive aquí: requiere red, por lo que es el puerto
// VerificadorContrasenasFiltradas.
type PoliticaContrasena struct{}

// NuevaPoliticaContrasena construye el servicio de dominio (no tiene
// estado ni dependencias que inyectar).
func NuevaPoliticaContrasena() PoliticaContrasena { return PoliticaContrasena{} }

// Evaluar devuelve ErrContrasenaDebil con la lista de reglas incumplidas,
// o nil si la contraseña cumple la política.
func (PoliticaContrasena) Evaluar(p ContrasenaPlana, correo Correo) error {
	var reglas []string

	longitud := p.Longitud()
	if longitud < longitudMinimaContrasena {
		reglas = append(reglas, "debe tener al menos 12 caracteres")
	}
	if longitud > longitudMaximaContrasenaEvaluada {
		reglas = append(reglas, "no puede superar 128 caracteres")
	}

	valorMin := strings.ToLower(p.valor)
	if correoNorm := correo.Normalizado(); correoNorm != "" && strings.Contains(valorMin, correoNorm) {
		reglas = append(reglas, "no puede contener el correo")
	} else if dom := correo.Dominio(); dom != "" && strings.Contains(valorMin, dom) {
		reglas = append(reglas, "no puede contener el dominio del correo")
	}

	if esSecuenciaTrivial(valorMin) {
		reglas = append(reglas, "no puede ser una secuencia trivial o predecible")
	}

	if len(reglas) > 0 {
		return &ErrContrasenaDebil{Reglas: reglas}
	}
	return nil
}

func esSecuenciaTrivial(valor string) bool {
	if valor == "" {
		return false
	}
	if todosLosCaracteresIguales(valor) {
		return true
	}
	for _, t := range secuenciasTriviales {
		if strings.Contains(valor, t) {
			return true
		}
	}
	return tieneSecuenciaNumericaConsecutiva(valor, 6)
}

func todosLosCaracteresIguales(valor string) bool {
	for i := 1; i < len(valor); i++ {
		if valor[i] != valor[0] {
			return false
		}
	}
	return true
}

// tieneSecuenciaNumericaConsecutiva detecta corridas de dígitos ascendentes
// consecutivos (p. ej. "345678") de al menos `minimo` caracteres.
func tieneSecuenciaNumericaConsecutiva(valor string, minimo int) bool {
	contador := 1
	for i := 1; i < len(valor); i++ {
		actual, anterior := valor[i], valor[i-1]
		if esDigito(actual) && esDigito(anterior) && actual == anterior+1 {
			contador++
			if contador >= minimo {
				return true
			}
		} else {
			contador = 1
		}
	}
	return false
}

func esDigito(b byte) bool { return b >= '0' && b <= '9' }
