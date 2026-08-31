package dominio

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// longitudMaximaCorreo es el límite de 254 octetos exigido por la tabla 1.3
// del diseño (alineado con el límite práctico de RFC 5321/5322).
const longitudMaximaCorreo = 254

// Correo es el value object que representa una dirección de correo
// electrónico normalizada. Es inmutable: solo puede construirse mediante
// NuevoCorreo, que aplica las invariantes de la tabla 1.3 del diseño
// (INV-ID-01, INV-ID-02).
//
// Nota de implementación: la normalización Unicode usa NFC completo vía
// golang.org/x/text/unicode/norm — excepción documentada y explícita a
// INV-ID-18 ("cero dependencias externas en el dominio"), aprobada porque
// x/text es mantenida por el propio equipo de Go y dos formas Unicode
// distintas del mismo correo visual (p. ej. "é" precompuesto vs. "e" +
// acento combinante) no deben tratarse como cuentas distintas. Es la única
// excepción permitida a INV-ID-18; cualquier otra dependencia en este
// paquete requiere el mismo nivel de justificación explícita.
type Correo struct {
	valor string
}

// NuevoCorreo valida y normaliza una dirección de correo cruda: recorta
// espacios, exige minúsculas completas, rechaza caracteres de control/CRLF
// (prevención de header injection aguas abajo), exige una sola arroba y un
// dominio con al menos un punto, y limita la longitud a 254 caracteres.
func NuevoCorreo(crudo string) (Correo, error) {
	// Se comprueban los caracteres de control/CRLF sobre la entrada cruda,
	// antes de recortar espacios: un TrimSpace previo eliminaría un CRLF al
	// final de la cadena y dejaría pasar un intento de header injection.
	for _, r := range crudo {
		if unicode.IsControl(r) {
			return Correo{}, &ErrCorreoInvalido{Motivo: "contiene caracteres de control no permitidos"}
		}
	}
	v := strings.TrimSpace(crudo)
	if v == "" {
		return Correo{}, &ErrCorreoInvalido{Motivo: "no puede estar vacío"}
	}
	if strings.ContainsAny(v, " \t") {
		return Correo{}, &ErrCorreoInvalido{Motivo: "no puede contener espacios"}
	}
	// Composición canónica ANTES del plegado a minúsculas: dos formas Unicode
	// del mismo correo visual deben converger a un único valor normalizado,
	// para que la unicidad (INV-ID-02) no dependa de qué forma tecleó el
	// cliente.
	v = norm.NFC.String(v)
	v = strings.ToLower(v)
	if len(v) > longitudMaximaCorreo {
		return Correo{}, &ErrCorreoInvalido{Motivo: "supera la longitud máxima permitida"}
	}
	partes := strings.Split(v, "@")
	if len(partes) != 2 {
		return Correo{}, &ErrCorreoInvalido{Motivo: "debe contener exactamente una arroba"}
	}
	local, dom := partes[0], partes[1]
	if local == "" {
		return Correo{}, &ErrCorreoInvalido{Motivo: "la parte local no puede estar vacía"}
	}
	if dom == "" || !strings.Contains(dom, ".") {
		return Correo{}, &ErrCorreoInvalido{Motivo: "el dominio debe contener al menos un punto"}
	}
	if strings.HasPrefix(dom, ".") || strings.HasSuffix(dom, ".") || strings.Contains(dom, "..") {
		return Correo{}, &ErrCorreoInvalido{Motivo: "el dominio tiene un formato inválido"}
	}
	return Correo{valor: v}, nil
}

// Normalizado devuelve la forma canónica (minúsculas, sin espacios) del
// correo.
func (c Correo) Normalizado() string { return c.valor }

// Dominio devuelve la parte del correo posterior a la arroba.
func (c Correo) Dominio() string {
	i := strings.LastIndex(c.valor, "@")
	if i == -1 {
		return ""
	}
	return c.valor[i+1:]
}

// String implementa fmt.Stringer devolviendo la forma normalizada. A
// diferencia de ContrasenaPlana/HashContrasena, el correo no es un secreto.
func (c Correo) String() string { return c.valor }

// EsVacio indica si el value object nunca fue construido (zero value).
func (c Correo) EsVacio() bool { return c.valor == "" }
