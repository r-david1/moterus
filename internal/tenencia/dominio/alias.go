package dominio

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// longitudMinimaAlias y longitudMaximaAlias acotan AliasOrganizacion
// (tabla 1.3 del diseño).
const (
	longitudMinimaAlias = 3
	longitudMaximaAlias = 48
)

// aliasesReservados es la lista corta de alias que ninguna organización
// puede tomar, porque colisionarían con rutas propias de la API o de otros
// contextos (tabla 1.3 del diseño).
var aliasesReservados = map[string]bool{
	"admin":      true,
	"api":        true,
	"acceso":     true,
	"identidad":  true,
	"tenencia":   true,
	"well-known": true,
	"nuevo":      true,
}

// AliasOrganizacion es el value object que representa el identificador
// legible y único de una Organizacion (p. ej. usado en rutas o subdominios
// futuros). Es inmutable: solo puede construirse mediante NuevoAlias, que
// aplica las invariantes de la tabla 1.3 del diseño (INV-TEN-01, INV-TEN-02).
//
// Nota de implementación: la normalización Unicode usa NFC completo vía
// golang.org/x/text/unicode/norm — la misma excepción documentada, única y
// explícita a INV-TEN-27 ("cero dependencias externas en el dominio") que
// ya usa identidad/dominio.Correo: x/text es mantenida por el propio equipo
// de Go y dos formas Unicode distintas del mismo alias visual no deben
// tratarse como valores distintos.
type AliasOrganizacion struct {
	valor string
}

// NuevoAlias valida y normaliza un alias crudo: TrimSpace, NFC, minúsculas
// completas, solo [a-z0-9-], longitud entre 3 y 48, no empieza ni termina
// en '-', sin '--' consecutivos, y no pertenece a la lista de reservados.
func NuevoAlias(crudo string) (AliasOrganizacion, error) {
	v := strings.TrimSpace(crudo)
	if v == "" {
		return AliasOrganizacion{}, &ErrAliasInvalido{Motivo: "no puede estar vacío"}
	}
	v = norm.NFC.String(v)
	v = strings.ToLower(v)

	if utf8RuneCount(v) < longitudMinimaAlias || utf8RuneCount(v) > longitudMaximaAlias {
		return AliasOrganizacion{}, &ErrAliasInvalido{Motivo: "la longitud debe estar entre 3 y 48 caracteres"}
	}
	for _, r := range v {
		if !esCaracterDeAlias(r) {
			return AliasOrganizacion{}, &ErrAliasInvalido{Motivo: "solo se permiten letras minúsculas, dígitos y guiones"}
		}
	}
	if strings.HasPrefix(v, "-") || strings.HasSuffix(v, "-") {
		return AliasOrganizacion{}, &ErrAliasInvalido{Motivo: "no puede empezar ni terminar en guion"}
	}
	if strings.Contains(v, "--") {
		return AliasOrganizacion{}, &ErrAliasInvalido{Motivo: "no puede contener guiones consecutivos"}
	}
	if aliasesReservados[v] {
		return AliasOrganizacion{}, &ErrAliasInvalido{Motivo: "es un alias reservado"}
	}
	return AliasOrganizacion{valor: v}, nil
}

// Normalizado devuelve la forma canónica (minúsculas, NFC) del alias.
func (a AliasOrganizacion) Normalizado() string { return a.valor }

// String implementa fmt.Stringer devolviendo la forma normalizada. No es un
// secreto: viaja en la ruta HTTP y potencialmente en un subdominio futuro.
func (a AliasOrganizacion) String() string { return a.valor }

// EsVacio indica si el value object nunca fue construido (zero value).
func (a AliasOrganizacion) EsVacio() bool { return a.valor == "" }

// EsIgual compara dos alias por su forma normalizada.
func (a AliasOrganizacion) EsIgual(otro AliasOrganizacion) bool { return a.valor == otro.valor }

func esCaracterDeAlias(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-'
}

func utf8RuneCount(s string) int {
	return len([]rune(s))
}

// longitudMaximaNombre acota NombreOrganizacion (tabla 1.3 del diseño).
const longitudMaximaNombre = 120

// NombreOrganizacion es el value object que representa el nombre de
// presentación de una Organizacion. A diferencia de AliasOrganizacion no se
// pasa a minúsculas (es texto de presentación) y no es único.
type NombreOrganizacion struct {
	valor string
}

// NuevoNombre valida y normaliza un nombre crudo: no vacío tras TrimSpace,
// NFC, longitud <= 120, y rechaza caracteres de control (incluido CRLF).
func NuevoNombre(crudo string) (NombreOrganizacion, error) {
	for _, r := range crudo {
		if unicode.IsControl(r) {
			return NombreOrganizacion{}, &ErrNombreOrganizacionInvalido{Motivo: "contiene caracteres de control no permitidos"}
		}
	}
	v := strings.TrimSpace(crudo)
	if v == "" {
		return NombreOrganizacion{}, &ErrNombreOrganizacionInvalido{Motivo: "no puede estar vacío"}
	}
	v = norm.NFC.String(v)
	if utf8RuneCount(v) > longitudMaximaNombre {
		return NombreOrganizacion{}, &ErrNombreOrganizacionInvalido{Motivo: "supera la longitud máxima permitida"}
	}
	return NombreOrganizacion{valor: v}, nil
}

// Valor devuelve el texto normalizado (NFC, espacios recortados) del
// nombre.
func (n NombreOrganizacion) Valor() string { return n.valor }

// String implementa fmt.Stringer.
func (n NombreOrganizacion) String() string { return n.valor }

// EsVacio indica si el value object nunca fue construido (zero value).
func (n NombreOrganizacion) EsVacio() bool { return n.valor == "" }

// EsIgual compara dos nombres por su valor normalizado.
func (n NombreOrganizacion) EsIgual(otro NombreOrganizacion) bool { return n.valor == otro.valor }
