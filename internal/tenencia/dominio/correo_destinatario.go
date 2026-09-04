package dominio

import (
	"crypto/subtle"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// longitudMaximaCorreoDestinatario es el límite de 254 octetos exigido por
// la tabla 1.3 del diseño (alineado con el límite práctico de RFC 5321/5322).
const longitudMaximaCorreoDestinatario = 254

// CorreoDestinatario es el value object que representa el destinatario de
// una Invitacion, normalizado. Tipo propio de tenencia/dominio, duplicado a
// conciencia de identidad/dominio.Correo (§1.7 del diseño): Tenencia
// necesita normalizar un correo para comparar destinatarios, pero no puede
// importar el VO de Identidad sin acoplar el modelo de invitaciones a la
// evolución del modelo de credenciales (INV-TEN-27). Mismas reglas
// estructurales y de normalización que identidad/dominio.Correo: minúsculas
// completas, NFC, una sola arroba, <=254, sin caracteres de control ni CRLF.
type CorreoDestinatario struct {
	valor string
}

// NuevoCorreoDestinatario valida y normaliza una dirección de correo cruda:
// recorta espacios, exige minúsculas completas, rechaza caracteres de
// control/CRLF (prevención de header injection aguas abajo), exige una sola
// arroba y un dominio con al menos un punto, y limita la longitud a 254
// caracteres.
func NuevoCorreoDestinatario(crudo string) (CorreoDestinatario, error) {
	// Se comprueban los caracteres de control/CRLF sobre la entrada cruda,
	// antes de recortar espacios: un TrimSpace previo eliminaría un CRLF al
	// final de la cadena y dejaría pasar un intento de header injection.
	for _, r := range crudo {
		if unicode.IsControl(r) {
			return CorreoDestinatario{}, &ErrCorreoDestinatarioInvalido{Motivo: "contiene caracteres de control no permitidos"}
		}
	}
	v := strings.TrimSpace(crudo)
	if v == "" {
		return CorreoDestinatario{}, &ErrCorreoDestinatarioInvalido{Motivo: "no puede estar vacío"}
	}
	if strings.ContainsAny(v, " \t") {
		return CorreoDestinatario{}, &ErrCorreoDestinatarioInvalido{Motivo: "no puede contener espacios"}
	}
	// Composición canónica ANTES del plegado a minúsculas: dos formas
	// Unicode del mismo correo visual deben converger a un único valor
	// normalizado, para que la comparación de destinatarios (INV-TEN-21) no
	// dependa de qué forma tecleó el cliente.
	v = norm.NFC.String(v)
	v = strings.ToLower(v)
	if len(v) > longitudMaximaCorreoDestinatario {
		return CorreoDestinatario{}, &ErrCorreoDestinatarioInvalido{Motivo: "supera la longitud máxima permitida"}
	}
	partes := strings.Split(v, "@")
	if len(partes) != 2 {
		return CorreoDestinatario{}, &ErrCorreoDestinatarioInvalido{Motivo: "debe contener exactamente una arroba"}
	}
	local, dom := partes[0], partes[1]
	if local == "" {
		return CorreoDestinatario{}, &ErrCorreoDestinatarioInvalido{Motivo: "la parte local no puede estar vacía"}
	}
	if dom == "" || !strings.Contains(dom, ".") {
		return CorreoDestinatario{}, &ErrCorreoDestinatarioInvalido{Motivo: "el dominio debe contener al menos un punto"}
	}
	if strings.HasPrefix(dom, ".") || strings.HasSuffix(dom, ".") || strings.Contains(dom, "..") {
		return CorreoDestinatario{}, &ErrCorreoDestinatarioInvalido{Motivo: "el dominio tiene un formato inválido"}
	}
	return CorreoDestinatario{valor: v}, nil
}

// Normalizado devuelve la forma canónica (minúsculas, NFC) del correo.
func (c CorreoDestinatario) Normalizado() string { return c.valor }

// String implementa fmt.Stringer devolviendo la forma normalizada. No es un
// secreto de por sí, pero el que una invitación exista para un correo dado
// solo debe poder comprobarse por quien la creó (el destinatario en sí no
// se expone fuera del contexto de la propia organización).
func (c CorreoDestinatario) String() string { return c.valor }

// EsVacio indica si el value object nunca fue construido (zero value).
func (c CorreoDestinatario) EsVacio() bool { return c.valor == "" }

// EsIgual compara dos correos por su forma normalizada.
func (c CorreoDestinatario) EsIgual(otro CorreoDestinatario) bool { return c.valor == otro.valor }

// EsIgualConstante compara dos correos en tiempo constante (crypto/subtle).
// Es la comprobación que exige INV-TEN-21 al aceptar una invitación: el
// correo del sujeto autenticado debe coincidir con el destinatario, y esa
// comparación no debe filtrar información por temporización (a diferencia
// de EsIgual, pensada para el camino no sensible de deduplicar
// invitaciones pendientes).
func (c CorreoDestinatario) EsIgualConstante(otro CorreoDestinatario) bool {
	return subtle.ConstantTimeCompare([]byte(c.valor), []byte(otro.valor)) == 1
}
