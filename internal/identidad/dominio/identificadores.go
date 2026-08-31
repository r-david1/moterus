package dominio

import "strings"

// IDUsuario identifica de forma única a un Usuario en todo el sistema. El
// value object es agnóstico de la estrategia de generación (UUIDv7 u otra);
// esa decisión vive en el puerto GeneradorIDs de infraestructura (ADR
// candidato 0012). Aquí solo se valida que el valor sea un UUID
// sintácticamente correcto y no nulo.
type IDUsuario struct {
	valor string
}

// IDUsuarioDesde valida y envuelve un identificador ya existente (p. ej. al
// leerlo de la base de datos o de un DTO de entrada). La generación de IDs
// nuevos la hace el puerto GeneradorIDs; el dominio nunca genera UUIDs por
// sí mismo.
func IDUsuarioDesde(valor string) (IDUsuario, error) {
	v := strings.TrimSpace(valor)
	if !esUUIDValido(v) {
		return IDUsuario{}, &ErrIDUsuarioInvalido{Motivo: "no es un UUID válido"}
	}
	if esUUIDNulo(v) {
		return IDUsuario{}, &ErrIDUsuarioInvalido{Motivo: "no puede ser el UUID nulo"}
	}
	return IDUsuario{valor: strings.ToLower(v)}, nil
}

// String devuelve la representación canónica en minúsculas del UUID.
func (id IDUsuario) String() string { return id.valor }

// EsVacio indica si el value object nunca fue construido (zero value).
func (id IDUsuario) EsVacio() bool { return id.valor == "" }

// EsIgual compara dos identificadores por su valor.
func (id IDUsuario) EsIgual(otro IDUsuario) bool { return id.valor == otro.valor }

func esUUIDValido(v string) bool {
	if len(v) != 36 {
		return false
	}
	for i, r := range v {
		switch i {
		case 8, 13, 18, 23:
			if r != '-' {
				return false
			}
		default:
			if !esHexadecimal(r) {
				return false
			}
		}
	}
	return true
}

func esHexadecimal(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
}

func esUUIDNulo(v string) bool {
	return strings.EqualFold(v, "00000000-0000-0000-0000-000000000000")
}

// ErrIDUsuarioInvalido se produce al construir un IDUsuario con un formato
// inválido o con el UUID nulo.
type ErrIDUsuarioInvalido struct{ Motivo string }

func (e *ErrIDUsuarioInvalido) Error() string {
	return "identificador de usuario inválido: " + e.Motivo
}
