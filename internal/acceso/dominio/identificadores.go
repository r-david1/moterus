package dominio

import "strings"

// IDUsuario identifica de forma única al sujeto dueño de una Sesion. Es un
// tipo propio de acceso/dominio, deliberadamente duplicado del homónimo de
// identidad/dominio (ver §1.7 del diseño del contexto Acceso y ADR candidato
// 0027): acceso/dominio no puede importar identidad/dominio (INV-ACC-18) sin
// romper la frontera entre contextos. Solo valida forma (UUID sintácticamente
// correcto y no nulo); la generación de IDs nuevos no es responsabilidad de
// Acceso, que siempre recibe el IDUsuario ya resuelto por Identidad.
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

// IDSesion identifica de forma única a una Sesion. Solo valida forma (UUID
// sintácticamente correcto y no nulo); la elección concreta de generarlo como
// UUIDv7 (ordenable, mejora la localidad de índice — tabla 1.3 del diseño) es
// una decisión del puerto GeneradorIDs de infraestructura, no una invariante
// que el parser deba rechazar aquí: IDSesionDesde también se usa para
// resolver IDs ya persistidos (p. ej. desde un parámetro de ruta), que por
// construcción ya son UUIDv7 válidos.
type IDSesion struct {
	valor string
}

// IDSesionDesde valida y envuelve un identificador de sesión ya existente.
// La generación de IDs nuevos vive en el puerto GeneradorIDs (adaptador de
// infraestructura); el dominio nunca genera UUIDs por sí mismo.
func IDSesionDesde(valor string) (IDSesion, error) {
	v := strings.TrimSpace(valor)
	if !esUUIDValido(v) {
		return IDSesion{}, &ErrIDSesionInvalido{Motivo: "no es un UUID válido"}
	}
	if esUUIDNulo(v) {
		return IDSesion{}, &ErrIDSesionInvalido{Motivo: "no puede ser el UUID nulo"}
	}
	return IDSesion{valor: strings.ToLower(v)}, nil
}

// String devuelve la representación canónica en minúsculas del UUID. No es
// secreto: viaja como claim `sid` en el token de acceso.
func (id IDSesion) String() string { return id.valor }

// EsVacio indica si el value object nunca fue construido (zero value).
func (id IDSesion) EsVacio() bool { return id.valor == "" }

// EsIgual compara dos identificadores por su valor.
func (id IDSesion) EsIgual(otro IDSesion) bool { return id.valor == otro.valor }

// IDTokenAcceso identifica de forma única a un token de acceso emitido (claim
// `jti`). Solo valida forma. Deliberadamente **no** UUIDv7: un `jti`
// ordenable filtraría el instante de emisión y permitiría correlacionar
// tokens de distintos usuarios en el tiempo (tabla 1.3 del diseño); la
// generación como UUIDv4 es responsabilidad del puerto GeneradorIDs.
type IDTokenAcceso struct {
	valor string
}

// IDTokenAccesoDesde valida y envuelve un identificador de token de acceso
// ya existente.
func IDTokenAccesoDesde(valor string) (IDTokenAcceso, error) {
	v := strings.TrimSpace(valor)
	if !esUUIDValido(v) {
		return IDTokenAcceso{}, &ErrIDTokenAccesoInvalido{Motivo: "no es un UUID válido"}
	}
	if esUUIDNulo(v) {
		return IDTokenAcceso{}, &ErrIDTokenAccesoInvalido{Motivo: "no puede ser el UUID nulo"}
	}
	return IDTokenAcceso{valor: strings.ToLower(v)}, nil
}

// String devuelve la representación canónica en minúsculas del UUID.
func (id IDTokenAcceso) String() string { return id.valor }

// EsVacio indica si el value object nunca fue construido (zero value).
func (id IDTokenAcceso) EsVacio() bool { return id.valor == "" }

// EsIgual compara dos identificadores por su valor.
func (id IDTokenAcceso) EsIgual(otro IDTokenAcceso) bool { return id.valor == otro.valor }

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
