package dominio

import "strings"

// IDSalaDeEspera identifica de forma única al agregado raíz SalaDeEspera
// (§1.2/§1.3 de docs/design/colas-virtuales.md). Solo valida forma (UUID
// sintácticamente correcto y no nulo); la elección concreta de generarlo
// como UUIDv7 vive en el puerto GeneradorIDs de infraestructura — el
// dominio nunca genera IDs por sí mismo.
type IDSalaDeEspera struct {
	valor string
}

// IDSalaDeEsperaDesde valida y envuelve un identificador ya existente (p.
// ej. al leerlo de la base de datos o de un parámetro de ruta).
func IDSalaDeEsperaDesde(valor string) (IDSalaDeEspera, error) {
	v := strings.TrimSpace(valor)
	if !esUUIDValido(v) {
		return IDSalaDeEspera{}, &ErrIDSalaDeEsperaInvalido{Motivo: "no es un UUID válido"}
	}
	if esUUIDNulo(v) {
		return IDSalaDeEspera{}, &ErrIDSalaDeEsperaInvalido{Motivo: "no puede ser el UUID nulo"}
	}
	return IDSalaDeEspera{valor: strings.ToLower(v)}, nil
}

// String devuelve la representación canónica en minúsculas del UUID.
func (id IDSalaDeEspera) String() string { return id.valor }

// EsVacio indica si el value object nunca fue construido (zero value).
func (id IDSalaDeEspera) EsVacio() bool { return id.valor == "" }

// EsIgual compara dos identificadores por su valor.
func (id IDSalaDeEspera) EsIgual(otro IDSalaDeEspera) bool { return id.valor == otro.valor }

// IDOrganizacion es un tipo propio de confianza/dominio, deliberadamente
// duplicado del homónimo de tenencia/dominio, identidad/dominio y
// acceso/dominio (§1.7 del diseño de Tenencia; misma frontera hexagonal que
// INV-TEN-27/INV-COLA no numera pero hereda): confianza/dominio no puede
// importar tenencia/dominio sin romper la frontera entre contextos. Aquí es
// una clave opaca (§1.1 del diseño de colas virtuales): Confianza no posee
// la tabla organizaciones y solo la usa para discriminar el AlcanceSala de
// una sala org-scoped.
type IDOrganizacion struct {
	valor string
}

// IDOrganizacionDesde valida y envuelve un identificador ya existente.
func IDOrganizacionDesde(valor string) (IDOrganizacion, error) {
	v := strings.TrimSpace(valor)
	if !esUUIDValido(v) {
		return IDOrganizacion{}, &ErrIDOrganizacionInvalido{Motivo: "no es un UUID válido"}
	}
	if esUUIDNulo(v) {
		return IDOrganizacion{}, &ErrIDOrganizacionInvalido{Motivo: "no puede ser el UUID nulo"}
	}
	return IDOrganizacion{valor: strings.ToLower(v)}, nil
}

// String devuelve la representación canónica en minúsculas del UUID.
func (id IDOrganizacion) String() string { return id.valor }

// EsVacio indica si el value object nunca fue construido (zero value).
func (id IDOrganizacion) EsVacio() bool { return id.valor == "" }

// EsIgual compara dos identificadores por su valor.
func (id IDOrganizacion) EsIgual(otro IDOrganizacion) bool { return id.valor == otro.valor }

// IDUsuario es un tipo propio de confianza/dominio, deliberadamente
// duplicado del homónimo de identidad/dominio, acceso/dominio y
// tenencia/dominio (misma razón que IDOrganizacion, arriba). Se usa para
// SalaDeEspera.creadaPor: el operador que abrió una sala org-scoped.
type IDUsuario struct {
	valor string
}

// IDUsuarioDesde valida y envuelve un identificador ya existente.
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

// --- validación de forma UUID, compartida por los tres identificadores -----

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
