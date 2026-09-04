package dominio

import "strings"

// IDOrganizacion identifica de forma única a una Organizacion (tabla 1.3 del
// diseño). Solo valida forma (UUID sintácticamente correcto y no nulo); la
// elección concreta de generarlo como UUIDv7 (ordenable, mejora la
// localidad de índice; no es secreto, viaja en la ruta HTTP) es una
// decisión del puerto GeneradorIDs de infraestructura, no una invariante
// que el parser deba imponer aquí. El dominio nunca genera IDs por sí
// mismo: IDOrganizacionDesde también se usa para resolver IDs ya
// persistidos o recibidos como parámetro de ruta.
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

// IDMembresia identifica de forma única a una Membresia. Solo valida forma;
// mismo criterio que IDOrganizacion.
type IDMembresia struct {
	valor string
}

// IDMembresiaDesde valida y envuelve un identificador ya existente.
func IDMembresiaDesde(valor string) (IDMembresia, error) {
	v := strings.TrimSpace(valor)
	if !esUUIDValido(v) {
		return IDMembresia{}, &ErrIDMembresiaInvalido{Motivo: "no es un UUID válido"}
	}
	if esUUIDNulo(v) {
		return IDMembresia{}, &ErrIDMembresiaInvalido{Motivo: "no puede ser el UUID nulo"}
	}
	return IDMembresia{valor: strings.ToLower(v)}, nil
}

// String devuelve la representación canónica en minúsculas del UUID.
func (id IDMembresia) String() string { return id.valor }

// EsVacio indica si el value object nunca fue construido (zero value).
func (id IDMembresia) EsVacio() bool { return id.valor == "" }

// EsIgual compara dos identificadores por su valor.
func (id IDMembresia) EsIgual(otro IDMembresia) bool { return id.valor == otro.valor }

// IDInvitacion identifica de forma única a una Invitacion. No es el token:
// es el identificador administrativo con el que un admin revoca la
// invitación desde su panel; el secreto es TokenInvitacionPlano/
// HashTokenInvitacion, un campo aparte.
type IDInvitacion struct {
	valor string
}

// IDInvitacionDesde valida y envuelve un identificador ya existente.
func IDInvitacionDesde(valor string) (IDInvitacion, error) {
	v := strings.TrimSpace(valor)
	if !esUUIDValido(v) {
		return IDInvitacion{}, &ErrIDInvitacionInvalido{Motivo: "no es un UUID válido"}
	}
	if esUUIDNulo(v) {
		return IDInvitacion{}, &ErrIDInvitacionInvalido{Motivo: "no puede ser el UUID nulo"}
	}
	return IDInvitacion{valor: strings.ToLower(v)}, nil
}

// String devuelve la representación canónica en minúsculas del UUID.
func (id IDInvitacion) String() string { return id.valor }

// EsVacio indica si el value object nunca fue construido (zero value).
func (id IDInvitacion) EsVacio() bool { return id.valor == "" }

// EsIgual compara dos identificadores por su valor.
func (id IDInvitacion) EsIgual(otro IDInvitacion) bool { return id.valor == otro.valor }

// IDUsuario identifica de forma única al sujeto dueño de una Membresia (o
// destinatario de una invitación resuelta). Es un tipo propio de
// tenencia/dominio, deliberadamente duplicado del homónimo de
// identidad/dominio y de acceso/dominio (§1.7 del diseño del contexto
// Tenencia; INV-TEN-27): tenencia/dominio no puede importar
// identidad/dominio ni acceso/dominio sin romper la frontera entre
// contextos. Solo valida forma; la generación de IDs nuevos no es
// responsabilidad de Tenencia, que siempre recibe el IDUsuario ya resuelto
// por Identidad (vía el sub del token validado, o por el ACL
// VerificadorDeSujetos).
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

// --- validación de forma UUID, compartida por los cuatro identificadores ---

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
