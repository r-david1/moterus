package dominio

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strings"
)

// prefijoTokenInvitacion marca estructuralmente un token de invitación de
// Tenencia, para poder rechazar de un vistazo (sin tocar la base de datos)
// cualquier cosa que no tenga esta forma. Mismo criterio que "mot_rt_" en
// Acceso (§1.3 del diseño).
const prefijoTokenInvitacion = "mot_inv_"

// longitudMinimaSecretoInvitacion es la longitud mínima del secreto que
// sigue al prefijo: base64url sin relleno de 32 bytes de entropía codifica
// en 43 caracteres.
const longitudMinimaSecretoInvitacion = 43

// TokenInvitacionPlano es un value object efímero que envuelve un token de
// invitación en texto claro. Nunca se persiste, ni se registra en logs, ni
// viaja en un evento de dominio, ni aparece en un mensaje de error
// (INV-TEN-23): implementa String()/GoString()/MarshalJSON devolviendo
// "[REDACTADO]", igual que TokenRefrescoPlano en acceso/dominio y
// ContrasenaPlana en identidad/dominio. Solo NotificadorInvitaciones puede
// leer su valor real, y solo una vez, para entregarlo al destinatario.
type TokenInvitacionPlano struct {
	valor string
}

// NuevoTokenInvitacionPlano valida la forma estructural de un token de
// invitación recibido del cliente: prefijo "mot_inv_" y al menos 43
// caracteres base64url de secreto. No consulta la base de datos: es un
// rechazo barato para tráfico obviamente malformado, antes de gastar una
// consulta (§3.5 del diseño, paso 3 de AceptarInvitacion).
func NuevoTokenInvitacionPlano(valor string) (TokenInvitacionPlano, error) {
	v := strings.TrimSpace(valor)
	if v == "" {
		return TokenInvitacionPlano{}, &ErrTokenInvitacionPlanoInvalido{Motivo: "no puede estar vacío"}
	}
	if !strings.HasPrefix(v, prefijoTokenInvitacion) {
		return TokenInvitacionPlano{}, &ErrTokenInvitacionPlanoInvalido{Motivo: "prefijo desconocido"}
	}
	secreto := v[len(prefijoTokenInvitacion):]
	if len(secreto) < longitudMinimaSecretoInvitacion {
		return TokenInvitacionPlano{}, &ErrTokenInvitacionPlanoInvalido{Motivo: "longitud de secreto insuficiente"}
	}
	if !esBase64URL(secreto) {
		return TokenInvitacionPlano{}, &ErrTokenInvitacionPlanoInvalido{Motivo: "el secreto contiene caracteres no permitidos"}
	}
	return TokenInvitacionPlano{valor: v}, nil
}

// Hash calcula el HashTokenInvitacion (SHA-256, hex) de este token en
// claro. Es la única vía por la que un TokenInvitacionPlano se convierte en
// algo persistible.
func (t TokenInvitacionPlano) Hash() HashTokenInvitacion {
	return HashearTokenInvitacion(t)
}

// Valor expone el token en claro. Solo debe invocarlo el caso de uso que
// acaba de emitirlo (InvitarMiembro) para entregarlo a
// NotificadorInvitaciones; nunca debe registrarse en logs, serializarse ni
// devolverse por HTTP (INV-TEN-23).
func (t TokenInvitacionPlano) Valor() string { return t.valor }

// EsVacio indica si el value object nunca fue construido (zero value).
func (t TokenInvitacionPlano) EsVacio() bool { return t.valor == "" }

// String redacta el token (INV-TEN-23).
func (t TokenInvitacionPlano) String() string { return "[REDACTADO]" }

// GoString redacta el token en el formato %#v.
func (t TokenInvitacionPlano) GoString() string { return "[REDACTADO]" }

// MarshalJSON redacta el token si el value object se serializa por
// descuido.
func (t TokenInvitacionPlano) MarshalJSON() ([]byte, error) {
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

// longitudHashTokenInvitacion es la longitud en caracteres hexadecimales de
// un digest SHA-256 (32 bytes -> 64 caracteres hex).
const longitudHashTokenInvitacion = 64

// HashTokenInvitacion es el value object que envuelve el hash SHA-256 (hex,
// en minúsculas) de un token de invitación. Es lo único que llega a la base
// de datos: el token en claro nunca se persiste (INV-TEN-23).
//
// Por qué SHA-256 y no Argon2id (mismo razonamiento ya establecido dos
// veces en el repositorio: identidad/aplicacion.hashTokenVerificacion y
// acceso/dominio.HashTokenRefresco): es un secreto aleatorio de alta
// entropía generado por el sistema, no hay ataque de diccionario que
// encarecer.
type HashTokenInvitacion struct {
	valor string
}

// NuevoHashTokenInvitacion valida que el valor tenga la forma de un digest
// SHA-256 en hexadecimal minúscula (64 caracteres). Se usa para envolver un
// hash ya calculado, típicamente al leerlo de la base de datos.
func NuevoHashTokenInvitacion(valor string) (HashTokenInvitacion, error) {
	v := strings.TrimSpace(valor)
	if len(v) != longitudHashTokenInvitacion {
		return HashTokenInvitacion{}, &ErrHashTokenInvitacionInvalido{Motivo: "longitud distinta de 64 caracteres"}
	}
	for _, r := range v {
		if !esHexadecimalMinuscula(r) {
			return HashTokenInvitacion{}, &ErrHashTokenInvitacionInvalido{Motivo: "no es hexadecimal en minúsculas"}
		}
	}
	return HashTokenInvitacion{valor: v}, nil
}

// HashearTokenInvitacion calcula el hash SHA-256 (hex, minúsculas) de un
// TokenInvitacionPlano ya validado.
func HashearTokenInvitacion(plano TokenInvitacionPlano) HashTokenInvitacion {
	suma := sha256.Sum256([]byte(plano.valor))
	return HashTokenInvitacion{valor: hex.EncodeToString(suma[:])}
}

// Valor expone el hash en hexadecimal para que los adaptadores de
// persistencia lo almacenen o lo usen como clave de búsqueda. No es un
// secreto de por sí (es un digest de un solo sentido).
func (h HashTokenInvitacion) Valor() string { return h.valor }

// EsVacio indica si el value object nunca fue construido (zero value).
func (h HashTokenInvitacion) EsVacio() bool { return h.valor == "" }

// EsIgual compara dos hashes en tiempo constante (crypto/subtle), para que
// ninguna comparación de un secreto de alta entropía filtre información por
// temporización.
func (h HashTokenInvitacion) EsIgual(otro HashTokenInvitacion) bool {
	return subtle.ConstantTimeCompare([]byte(h.valor), []byte(otro.valor)) == 1
}

// String implementa fmt.Stringer. A diferencia de TokenInvitacionPlano, el
// hash no es el secreto en sí (es un digest de un solo sentido) y no se
// redacta.
func (h HashTokenInvitacion) String() string { return h.valor }

func esHexadecimalMinuscula(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')
}
