package dominio

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strings"
	"time"
)

// longitudCodigoRespaldo es la longitud, en caracteres, de un código de
// respaldo de un solo uso (§1.4 del diseño otp-mfa.md, ADR 0040).
const longitudCodigoRespaldo = 10

// alfabetoCodigoRespaldo excluye deliberadamente los caracteres ambiguos
// 0/O, 1/I/L, que un usuario podría confundir al transcribir un código de
// respaldo a mano desde donde lo guardó.
const alfabetoCodigoRespaldo = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

// CodigoRespaldoPlano es un value object efímero que envuelve, en texto
// claro, uno de los 10 códigos de respaldo de un solo uso generados al
// confirmar un FactorMFA (ADR 0040). Nunca se persiste, ni se registra en
// logs, ni viaja en un evento de dominio (INV-MFA-07): implementa
// String()/GoString()/MarshalJSON devolviendo "[REDACTADO]", mismo patrón
// que SecretoTOTPPlano. Solo el caso de uso ConfirmarFactorMFA lo expone, y
// solo una vez, en su respuesta.
type CodigoRespaldoPlano struct {
	valor string
}

// NuevoCodigoRespaldoPlano valida la forma estructural de un código de
// respaldo: exactamente 10 caracteres del alfabeto alfanumérico restringido
// (sin 0/O/1/I/L). Se usa tanto para envolver un código recién generado por
// el puerto GeneradorSecretoTOTP.GenerarCodigosRespaldo como para el código
// que el usuario presenta al hacer login o al deshabilitar MFA (ADR 0039).
func NuevoCodigoRespaldoPlano(valor string) (CodigoRespaldoPlano, error) {
	v := strings.ToUpper(strings.TrimSpace(valor))
	if len(v) != longitudCodigoRespaldo {
		return CodigoRespaldoPlano{}, &ErrCodigoRespaldoPlanoInvalido{Motivo: "longitud distinta de 10 caracteres"}
	}
	for _, r := range v {
		if !strings.ContainsRune(alfabetoCodigoRespaldo, r) {
			return CodigoRespaldoPlano{}, &ErrCodigoRespaldoPlanoInvalido{Motivo: "contiene caracteres fuera del alfabeto permitido"}
		}
	}
	return CodigoRespaldoPlano{valor: v}, nil
}

// Valor expone el código en claro. Solo debe usarlo el caso de uso que
// acaba de generarlo para devolverlo al cliente, o el propio dominio para
// calcular su Hash(); nunca debe registrarse ni persistirse (INV-MFA-07).
func (c CodigoRespaldoPlano) Valor() string { return c.valor }

// Hash calcula el HashCodigoRespaldo (SHA-256, hex) de este código en
// claro. Es la única vía por la que un CodigoRespaldoPlano se convierte en
// algo persistible.
func (c CodigoRespaldoPlano) Hash() HashCodigoRespaldo { return HashearCodigoRespaldo(c) }

// EsVacio indica si el value object nunca fue construido (zero value).
func (c CodigoRespaldoPlano) EsVacio() bool { return c.valor == "" }

// String redacta el código (INV-MFA-07).
func (c CodigoRespaldoPlano) String() string { return "[REDACTADO]" }

// GoString redacta el código en el formato %#v.
func (c CodigoRespaldoPlano) GoString() string { return "[REDACTADO]" }

// MarshalJSON redacta el código si el value object se serializa por
// descuido.
func (c CodigoRespaldoPlano) MarshalJSON() ([]byte, error) {
	return []byte(`"[REDACTADO]"`), nil
}

// longitudHashCodigoRespaldo es la longitud en caracteres hexadecimales de
// un digest SHA-256 (32 bytes -> 64 caracteres hex).
const longitudHashCodigoRespaldo = 64

// HashCodigoRespaldo es el value object que envuelve el hash SHA-256 (hex,
// minúsculas) de un código de respaldo. Es lo único que llega a la base de
// datos: el código en claro nunca se persiste (INV-MFA-07).
//
// Por qué SHA-256 y no Argon2id (mismo razonamiento ya usado dos veces en
// el repositorio: identidad/aplicacion.hashTokenVerificacion y
// tenencia/dominio.HashTokenInvitacion): es un secreto aleatorio de alta
// entropía generado por el sistema, no hay ataque de diccionario que
// encarecer.
type HashCodigoRespaldo struct {
	valor string
}

// NuevoHashCodigoRespaldo valida que el valor tenga la forma de un digest
// SHA-256 en hexadecimal minúscula (64 caracteres). Se usa para envolver un
// hash ya calculado, típicamente al leerlo de la base de datos.
func NuevoHashCodigoRespaldo(valor string) (HashCodigoRespaldo, error) {
	v := strings.TrimSpace(valor)
	if len(v) != longitudHashCodigoRespaldo {
		return HashCodigoRespaldo{}, &ErrHashCodigoRespaldoInvalido{Motivo: "longitud distinta de 64 caracteres"}
	}
	for _, r := range v {
		if !esHexadecimalMinusculaMFA(r) {
			return HashCodigoRespaldo{}, &ErrHashCodigoRespaldoInvalido{Motivo: "no es hexadecimal en minúsculas"}
		}
	}
	return HashCodigoRespaldo{valor: v}, nil
}

// HashearCodigoRespaldo calcula el hash SHA-256 (hex, minúsculas) de un
// CodigoRespaldoPlano ya validado.
func HashearCodigoRespaldo(plano CodigoRespaldoPlano) HashCodigoRespaldo {
	suma := sha256.Sum256([]byte(plano.valor))
	return HashCodigoRespaldo{valor: hex.EncodeToString(suma[:])}
}

// Valor expone el hash en hexadecimal para que los adaptadores de
// persistencia lo almacenen o lo usen como clave de búsqueda. No es un
// secreto de por sí (es un digest de un solo sentido).
func (h HashCodigoRespaldo) Valor() string { return h.valor }

// EsVacio indica si el value object nunca fue construido (zero value).
func (h HashCodigoRespaldo) EsVacio() bool { return h.valor == "" }

// EsIgual compara dos hashes en tiempo constante (crypto/subtle), para que
// ninguna comparación de un secreto de alta entropía filtre información por
// temporización.
func (h HashCodigoRespaldo) EsIgual(otro HashCodigoRespaldo) bool {
	return subtle.ConstantTimeCompare([]byte(h.valor), []byte(otro.valor)) == 1
}

// String implementa fmt.Stringer. A diferencia de CodigoRespaldoPlano, el
// hash no es el secreto en sí (es un digest de un solo sentido) y no se
// redacta.
func (h HashCodigoRespaldo) String() string { return h.valor }

func esHexadecimalMinusculaMFA(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')
}

// CodigoRespaldoMFA es el value object que vive dentro del agregado
// FactorMFA (§1.2 del diseño otp-mfa.md): representa uno de los 10 códigos
// de respaldo de un solo uso, identificado únicamente por su hash. No es un
// agregado propio: FactorMFA es el único que muta su colección de
// CodigoRespaldoMFA, siempre a través de sus propios métodos de negocio.
type CodigoRespaldoMFA struct {
	hashCodigo HashCodigoRespaldo
	usadoEn    *time.Time
}

// NuevoCodigoRespaldoMFA construye un CodigoRespaldoMFA nuevo, todavía
// disponible, a partir de un hash ya calculado.
func NuevoCodigoRespaldoMFA(hash HashCodigoRespaldo) (CodigoRespaldoMFA, error) {
	if hash.EsVacio() {
		return CodigoRespaldoMFA{}, &ErrHashCodigoRespaldoInvalido{Motivo: "no puede estar vacío"}
	}
	return CodigoRespaldoMFA{hashCodigo: hash}, nil
}

// ReconstituirCodigoRespaldoMFA reconstruye un CodigoRespaldoMFA a partir de
// datos ya persistidos. A diferencia de NuevoCodigoRespaldoMFA, admite un
// usadoEn ya establecido (código históricamente consumido).
func ReconstituirCodigoRespaldoMFA(hash HashCodigoRespaldo, usadoEn *time.Time) CodigoRespaldoMFA {
	c := CodigoRespaldoMFA{hashCodigo: hash}
	if usadoEn != nil {
		copia := *usadoEn
		c.usadoEn = &copia
	}
	return c
}

// HashCodigo devuelve el hash SHA-256 de este código de respaldo.
func (c CodigoRespaldoMFA) HashCodigo() HashCodigoRespaldo { return c.hashCodigo }

// UsadoEn devuelve la marca de tiempo en que se consumió el código, y un
// booleano que indica si ya se consumió.
func (c CodigoRespaldoMFA) UsadoEn() (time.Time, bool) {
	if c.usadoEn == nil {
		return time.Time{}, false
	}
	return *c.usadoEn, true
}

// EstaDisponible indica si el código todavía no se consumió (INV-MFA-06).
func (c CodigoRespaldoMFA) EstaDisponible() bool { return c.usadoEn == nil }

// Consumir marca el código como usado en el instante dado, devolviendo un
// nuevo CodigoRespaldoMFA (value object inmutable). Produce
// ErrCodigoRespaldoYaUsado si el código ya se había consumido antes
// (INV-MFA-06: un código de respaldo se consume una sola vez, sin
// excepción).
func (c CodigoRespaldoMFA) Consumir(ahora time.Time) (CodigoRespaldoMFA, error) {
	if !c.EstaDisponible() {
		return c, &ErrCodigoRespaldoYaUsado{}
	}
	copia := ahora
	return CodigoRespaldoMFA{hashCodigo: c.hashCodigo, usadoEn: &copia}, nil
}

// --- errores de construcción propios de este archivo ------------------------

// ErrCodigoRespaldoPlanoInvalido se produce por restricciones estructurales
// de CodigoRespaldoPlano (longitud o alfabeto).
type ErrCodigoRespaldoPlanoInvalido struct{ Motivo string }

func (e *ErrCodigoRespaldoPlanoInvalido) Error() string {
	return "código de respaldo inválido: " + e.Motivo
}

// ErrHashCodigoRespaldoInvalido se produce al construir un
// HashCodigoRespaldo con un formato irreconocible.
type ErrHashCodigoRespaldoInvalido struct{ Motivo string }

func (e *ErrHashCodigoRespaldoInvalido) Error() string {
	return "hash de código de respaldo inválido: " + e.Motivo
}
