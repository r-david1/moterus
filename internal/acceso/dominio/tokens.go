package dominio

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strings"
)

// prefijoTokenRefresco marca estructuralmente un token de refresco de
// Acceso, para poder rechazar de un vistazo (sin tocar la base de datos)
// cualquier cosa que no tenga esta forma (§3.2 paso 2 del diseño).
const prefijoTokenRefresco = "mot_rt_"

// longitudMinimaSecretoRefresco es la longitud mínima del secreto que sigue
// al prefijo: base64url sin relleno de 32 bytes de entropía codifica en 43
// caracteres (tabla 1.3 del diseño).
const longitudMinimaSecretoRefresco = 43

// TokenRefrescoPlano es un value object efímero que envuelve un token de
// refresco en texto claro. Nunca se persiste, ni se registra en logs, ni
// viaja en un evento de dominio, ni aparece en un mensaje de error
// (INV-ACC-11): implementa String()/GoString()/MarshalJSON devolviendo
// "[REDACTADO]", igual que ContrasenaPlana en identidad/dominio. Solo el
// adaptador HTTP de salida puede leer su valor real, y solo una vez.
type TokenRefrescoPlano struct {
	valor string
}

// NuevoTokenRefrescoPlano valida la forma estructural de un token de
// refresco recibido del cliente: prefijo "mot_rt_" y al menos 43 caracteres
// base64url de secreto. No consulta la base de datos: es un rechazo barato
// para tráfico obviamente malformado, antes de gastar una consulta
// (§3.2 paso 2).
func NuevoTokenRefrescoPlano(valor string) (TokenRefrescoPlano, error) {
	v := strings.TrimSpace(valor)
	if v == "" {
		return TokenRefrescoPlano{}, &ErrTokenRefrescoPlanoInvalido{Motivo: "no puede estar vacío"}
	}
	if !strings.HasPrefix(v, prefijoTokenRefresco) {
		return TokenRefrescoPlano{}, &ErrTokenRefrescoPlanoInvalido{Motivo: "prefijo desconocido"}
	}
	secreto := v[len(prefijoTokenRefresco):]
	if len(secreto) < longitudMinimaSecretoRefresco {
		return TokenRefrescoPlano{}, &ErrTokenRefrescoPlanoInvalido{Motivo: "longitud de secreto insuficiente"}
	}
	if !esBase64URL(secreto) {
		return TokenRefrescoPlano{}, &ErrTokenRefrescoPlanoInvalido{Motivo: "el secreto contiene caracteres no permitidos"}
	}
	return TokenRefrescoPlano{valor: v}, nil
}

// Hash calcula el HashTokenRefresco (SHA-256, hex) de este token en claro.
// Es la única vía por la que un TokenRefrescoPlano se convierte en algo
// persistible.
func (t TokenRefrescoPlano) Hash() HashTokenRefresco {
	return HashearTokenRefresco(t)
}

// EsVacio indica si el value object nunca fue construido (zero value).
func (t TokenRefrescoPlano) EsVacio() bool { return t.valor == "" }

// String redacta el token (INV-ACC-11).
func (t TokenRefrescoPlano) String() string { return "[REDACTADO]" }

// GoString redacta el token en el formato %#v.
func (t TokenRefrescoPlano) GoString() string { return "[REDACTADO]" }

// MarshalJSON redacta el token si el value object se serializa por
// descuido.
func (t TokenRefrescoPlano) MarshalJSON() ([]byte, error) {
	return []byte(`"[REDACTADO]"`), nil
}

func esBase64URL(v string) bool {
	for _, r := range v {
		switch {
		case r >= 'A' && r <= 'Z':
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_':
		default:
			return false
		}
	}
	return true
}

// longitudHashTokenRefresco es la longitud en caracteres hexadecimales de un
// digest SHA-256 (32 bytes -> 64 caracteres hex).
const longitudHashTokenRefresco = 64

// HashTokenRefresco es el value object que envuelve el hash SHA-256 (hex, en
// minúsculas) de un token de refresco. Es lo único que llega a la base de
// datos: el token en claro nunca se persiste (INV-ACC-11).
//
// Por qué SHA-256 y no Argon2id (mismo razonamiento que
// identidad/aplicacion.hashTokenVerificacion): es un secreto aleatorio de
// alta entropía generado por el sistema, no hay ataque de diccionario que
// encarecer, y pagar Argon2id en cada renovación sería costo puro.
type HashTokenRefresco struct {
	valor string
}

// NuevoHashTokenRefresco valida que el valor tenga la forma de un digest
// SHA-256 en hexadecimal minúscula (64 caracteres). Se usa para envolver un
// hash ya calculado, típicamente al leerlo de la base de datos.
func NuevoHashTokenRefresco(valor string) (HashTokenRefresco, error) {
	v := strings.TrimSpace(valor)
	if len(v) != longitudHashTokenRefresco {
		return HashTokenRefresco{}, &ErrHashTokenRefrescoInvalido{Motivo: "longitud distinta de 64 caracteres"}
	}
	for _, r := range v {
		if !esHexadecimalMinuscula(r) {
			return HashTokenRefresco{}, &ErrHashTokenRefrescoInvalido{Motivo: "no es hexadecimal en minúsculas"}
		}
	}
	return HashTokenRefresco{valor: v}, nil
}

// HashearTokenRefresco calcula el hash SHA-256 (hex, minúsculas) de un
// TokenRefrescoPlano ya validado.
func HashearTokenRefresco(plano TokenRefrescoPlano) HashTokenRefresco {
	suma := sha256.Sum256([]byte(plano.valor))
	return HashTokenRefresco{valor: hex.EncodeToString(suma[:])}
}

// Valor expone el hash en hexadecimal para que los adaptadores de
// persistencia lo almacenen o lo usen como clave de búsqueda. No es un
// secreto de por sí (es un digest de un solo sentido), pero tampoco tiene
// valor fuera de la base de datos, así que no se expone en eventos de
// dominio.
func (h HashTokenRefresco) Valor() string { return h.valor }

// EsVacio indica si el value object nunca fue construido (zero value).
func (h HashTokenRefresco) EsVacio() bool { return h.valor == "" }

// EsIgual compara dos hashes en tiempo constante (crypto/subtle), para que
// ninguna comparación de un secreto de alta entropía filtre información por
// temporización (tabla 1.3 del diseño).
func (h HashTokenRefresco) EsIgual(otro HashTokenRefresco) bool {
	return subtle.ConstantTimeCompare([]byte(h.valor), []byte(otro.valor)) == 1
}

// String implementa fmt.Stringer. A diferencia de TokenRefrescoPlano, el
// hash no es el secreto en sí (es un digest de un solo sentido) y no se
// redacta.
func (h HashTokenRefresco) String() string { return h.valor }

func esHexadecimalMinuscula(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')
}

// --- errores de construcción propios de este archivo ------------------------

// ErrTokenRefrescoPlanoInvalido se produce por restricciones estructurales
// de TokenRefrescoPlano (prefijo, longitud o alfabeto incorrectos). Nunca
// incluye el valor recibido.
type ErrTokenRefrescoPlanoInvalido struct{ Motivo string }

func (e *ErrTokenRefrescoPlanoInvalido) Error() string {
	return "token de refresco inválido: " + e.Motivo
}

// ErrHashTokenRefrescoInvalido se produce al construir un HashTokenRefresco
// con un formato irreconocible.
type ErrHashTokenRefrescoInvalido struct{ Motivo string }

func (e *ErrHashTokenRefrescoInvalido) Error() string {
	return "hash de token de refresco inválido: " + e.Motivo
}
