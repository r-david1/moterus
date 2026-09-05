package dominio

import (
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // SHA-1 es el algoritmo que exige el estándar TOTP (RFC 6238), no una elección propia.
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// pasoTOTPSegundos es la ventana de tiempo (X en RFC 6238) sobre la que se
// deriva cada código: 30 segundos, el valor estándar de facto que usan todas
// las apps autenticadoras (Google Authenticator, Authy, 1Password, etc.).
const pasoTOTPSegundos = 30

// toleranciaPasosTOTP es la tolerancia de desfase de reloj (§1.5 del diseño
// otp-mfa.md): ±1 paso de 30 segundos, para absorber que el reloj del
// dispositivo del usuario no esté perfectamente sincronizado, sin abrir una
// ventana de fuerza bruta más ancha de lo necesario.
const toleranciaPasosTOTP = 1

// digitosTOTP es la longitud del código TOTP en el MVP (RFC 4226 permite 6,
// 7 u 8; el ecosistema de apps autenticadoras estandarizó 6).
const digitosTOTP = 6

// TipoFactor es el value object enum, catálogo cerrado, que identifica el
// mecanismo concreto de un FactorMFA. El MVP (ADR 0037) solo admite un
// valor: "totp". Se modela como catálogo (no una constante suelta) para que
// agregar "email"/"sms" en un hito futuro sea aditivo, no una migración de
// tipo — mismo criterio que Rol/EstadoUsuario.
type TipoFactor struct {
	valor string
}

// TipoFactorTOTP es el único valor del catálogo cerrado en el MVP.
var TipoFactorTOTP = TipoFactor{valor: "totp"}

var catalogoTiposFactor = map[string]TipoFactor{
	TipoFactorTOTP.valor: TipoFactorTOTP,
}

// TipoFactorDesde valida un valor persistido (o recibido en un comando)
// contra el catálogo cerrado de tipos de factor.
func TipoFactorDesde(valor string) (TipoFactor, error) {
	t, ok := catalogoTiposFactor[valor]
	if !ok {
		return TipoFactor{}, &ErrTipoFactorInvalido{Valor: valor}
	}
	return t, nil
}

// Valor devuelve la representación canónica del tipo de factor.
func (t TipoFactor) Valor() string { return t.valor }

// String implementa fmt.Stringer.
func (t TipoFactor) String() string { return t.valor }

// EsVacio indica si el value object nunca fue construido (zero value).
func (t TipoFactor) EsVacio() bool { return t.valor == "" }

// EsIgual compara dos tipos de factor por su valor.
func (t TipoFactor) EsIgual(otro TipoFactor) bool { return t.valor == otro.valor }

// longitudMinimaSecretoTOTP es la longitud mínima, en caracteres Base32 sin
// relleno, de un secreto TOTP de 160 bits de entropía (RFC 4226 recomienda
// ≥128 bits; 160 es el tamaño nativo de salida de SHA-1, el algoritmo que
// usa TOTP clásico): 160 bits / 5 bits por carácter Base32 = 32 caracteres.
const longitudMinimaSecretoTOTP = 32

// alfabetoBase32 es el alfabeto estándar (RFC 4648 §6), usado para validar
// estructuralmente un SecretoTOTPPlano sin depender de decodificarlo.
const alfabetoBase32 = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"

// SecretoTOTPPlano es un value object efímero que envuelve, en texto claro,
// el secreto compartido de un factor TOTP, codificado en Base32 sin
// relleno. Nunca se persiste, ni se registra en logs, ni viaja en un evento
// de dominio, ni aparece en un mensaje de error (INV-MFA-02): implementa
// String()/GoString()/MarshalJSON devolviendo "[REDACTADO]", mismo patrón
// que TokenInvitacionPlano (tenencia/dominio) y ContrasenaPlana. Solo el
// caso de uso HabilitarMFA lo expone, y solo una vez, en su respuesta.
type SecretoTOTPPlano struct {
	valor string
}

// NuevoSecretoTOTPPlano valida la forma estructural de un secreto TOTP ya
// generado por el puerto GeneradorSecretoTOTP: Base32 (sin relleno,
// mayúsculas) de al menos 160 bits de entropía. El dominio nunca genera
// bytes aleatorios por sí mismo; esta función solo envuelve un valor que la
// infraestructura ya produjo (o que se reconstituye desde una fuente ya
// validada).
func NuevoSecretoTOTPPlano(valor string) (SecretoTOTPPlano, error) {
	v := strings.ToUpper(strings.TrimRight(strings.TrimSpace(valor), "="))
	if v == "" {
		return SecretoTOTPPlano{}, &ErrSecretoTOTPPlanoInvalido{Motivo: "no puede estar vacío"}
	}
	if len(v) < longitudMinimaSecretoTOTP {
		return SecretoTOTPPlano{}, &ErrSecretoTOTPPlanoInvalido{Motivo: "longitud insuficiente para 160 bits de entropía"}
	}
	if !esBase32Valido(v) {
		return SecretoTOTPPlano{}, &ErrSecretoTOTPPlanoInvalido{Motivo: "no es Base32 válido"}
	}
	return SecretoTOTPPlano{valor: v}, nil
}

// Valor expone el secreto en claro. Solo debe invocarlo el caso de uso que
// acaba de generarlo (HabilitarMFA) para devolverlo al cliente una única
// vez, o el adaptador CifradorSecretos para cifrarlo; nunca debe
// registrarse en logs ni persistirse (INV-MFA-02).
func (s SecretoTOTPPlano) Valor() string { return s.valor }

// EsVacio indica si el value object nunca fue construido (zero value).
func (s SecretoTOTPPlano) EsVacio() bool { return s.valor == "" }

// String redacta el secreto (INV-MFA-02).
func (s SecretoTOTPPlano) String() string { return "[REDACTADO]" }

// GoString redacta el secreto en el formato %#v.
func (s SecretoTOTPPlano) GoString() string { return "[REDACTADO]" }

// MarshalJSON redacta el secreto si el value object se serializa por
// descuido.
func (s SecretoTOTPPlano) MarshalJSON() ([]byte, error) {
	return []byte(`"[REDACTADO]"`), nil
}

// URIProvisionamiento construye la URI "otpauth://totp/..." que el cliente
// convierte en un código QR para que su app autenticadora la escanee. Es la
// única vía, junto con Valor(), por la que el secreto en claro sale del
// proceso (§1.4 del diseño otp-mfa.md).
func (s SecretoTOTPPlano) URIProvisionamiento(correo Correo, emisor string) string {
	etiqueta := fmt.Sprintf("%s:%s", emisor, correo.Normalizado())
	consulta := url.Values{}
	consulta.Set("secret", s.valor)
	consulta.Set("issuer", emisor)
	consulta.Set("algorithm", "SHA1")
	consulta.Set("digits", fmt.Sprintf("%d", digitosTOTP))
	consulta.Set("period", fmt.Sprintf("%d", pasoTOTPSegundos))
	return "otpauth://totp/" + url.PathEscape(etiqueta) + "?" + consulta.Encode()
}

func esBase32Valido(v string) bool {
	if v == "" {
		return false
	}
	for _, r := range v {
		if !strings.ContainsRune(alfabetoBase32, r) {
			return false
		}
	}
	return true
}

// SecretoTOTPCifrado envuelve el secreto TOTP en su forma cifrada, la única
// que se persiste (INV-MFA-02). El dominio no cifra ni descifra: eso es
// responsabilidad del puerto de salida CifradorSecretos (AES-256-GCM,
// infraestructura), que este value object deliberadamente no conoce.
type SecretoTOTPCifrado struct {
	valor []byte
}

// NuevoSecretoTOTPCifrado envuelve un valor cifrado ya calculado. No valida
// su contenido (es opaco para el dominio): solo que no esté vacío.
func NuevoSecretoTOTPCifrado(valor []byte) (SecretoTOTPCifrado, error) {
	if len(valor) == 0 {
		return SecretoTOTPCifrado{}, &ErrSecretoTOTPCifradoInvalido{Motivo: "no puede estar vacío"}
	}
	copia := make([]byte, len(valor))
	copy(copia, valor)
	return SecretoTOTPCifrado{valor: copia}, nil
}

// Valor devuelve una copia de los bytes cifrados, para que el adaptador de
// persistencia los almacene o el CifradorSecretos los descifre. Devuelve
// una copia, nunca el slice interno, para que el llamador no pueda mutar el
// estado del value object (INV-ID-09).
func (s SecretoTOTPCifrado) Valor() []byte {
	copia := make([]byte, len(s.valor))
	copy(copia, s.valor)
	return copia
}

// EsVacio indica si el value object nunca fue construido (zero value).
func (s SecretoTOTPCifrado) EsVacio() bool { return len(s.valor) == 0 }

// CodigoTOTP es el value object que envuelve un código de 6 dígitos ASCII
// presentado por el usuario, ya sea al confirmar un factor o al completar
// un step-up. Rechazo estructural de formato antes de gastar un descifrado
// y un cómputo HMAC.
type CodigoTOTP struct {
	valor string
}

// NuevoCodigoTOTP valida que el código tenga exactamente 6 dígitos ASCII.
func NuevoCodigoTOTP(valor string) (CodigoTOTP, error) {
	v := strings.TrimSpace(valor)
	if len(v) != digitosTOTP {
		return CodigoTOTP{}, &ErrCodigoTOTPInvalido{Motivo: "debe tener exactamente 6 dígitos"}
	}
	for _, r := range v {
		if r < '0' || r > '9' {
			return CodigoTOTP{}, &ErrCodigoTOTPInvalido{Motivo: "debe contener solo dígitos ASCII"}
		}
	}
	return CodigoTOTP{valor: v}, nil
}

// Valor devuelve el código de 6 dígitos.
func (c CodigoTOTP) Valor() string { return c.valor }

// String implementa fmt.Stringer. A diferencia de SecretoTOTPPlano, un
// código de 6 dígitos vive 30-90 segundos y por sí solo no permite generar
// códigos futuros: no se redacta.
func (c CodigoTOTP) String() string { return c.valor }

// EsVacio indica si el value object nunca fue construido (zero value).
func (c CodigoTOTP) EsVacio() bool { return c.valor == "" }

// VerificarCodigo es el servicio de dominio puro que implementa el
// algoritmo TOTP (RFC 6238) sobre HMAC-SHA1: calcula el código esperado
// para floor(ahora.Unix()/30) y, por tolerancia de reloj, también para el
// paso inmediatamente anterior y el inmediatamente posterior (±1 paso,
// §1.5 del diseño otp-mfa.md), comparando cada candidato en tiempo
// constante. Es una función pura: nunca llama a time.Now(), recibe ahora
// como parámetro. secretoDescifrado es el secreto en su forma Base32 en
// claro (la misma representación que SecretoTOTPPlano.Valor()); el dominio
// nunca lo descifra por sí mismo, quien invoca esta función ya lo hizo vía
// CifradorSecretos.
func VerificarCodigo(secretoDescifrado string, codigo CodigoTOTP, ahora time.Time) bool {
	if codigo.EsVacio() {
		return false
	}
	clave, err := decodificarSecretoBase32(secretoDescifrado)
	if err != nil || len(clave) == 0 {
		return false
	}
	contadorActual := ahora.Unix() / pasoTOTPSegundos
	for delta := -toleranciaPasosTOTP; delta <= toleranciaPasosTOTP; delta++ {
		contador := contadorActual + int64(delta)
		if contador < 0 {
			continue
		}
		esperado := generarCodigoTOTP(clave, uint64(contador))
		if subtle.ConstantTimeCompare([]byte(esperado), []byte(codigo.valor)) == 1 {
			return true
		}
	}
	return false
}

func decodificarSecretoBase32(v string) ([]byte, error) {
	limpio := strings.ToUpper(strings.TrimRight(strings.TrimSpace(v), "="))
	return base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(limpio)
}

// generarCodigoTOTP implementa HOTP (RFC 4226) sobre HMAC-SHA1 para un
// contador dado, truncado a digitosTOTP dígitos decimales — el mismo
// algoritmo que TOTP (RFC 6238) usa con contador = floor(tiempo/paso).
func generarCodigoTOTP(clave []byte, contador uint64) string {
	var contadorBytes [8]byte
	binary.BigEndian.PutUint64(contadorBytes[:], contador)

	mac := hmac.New(sha1.New, clave)
	mac.Write(contadorBytes[:])
	suma := mac.Sum(nil)

	offset := suma[len(suma)-1] & 0x0f
	codigoBinario := (uint32(suma[offset]&0x7f) << 24) |
		(uint32(suma[offset+1]) << 16) |
		(uint32(suma[offset+2]) << 8) |
		uint32(suma[offset+3])

	modulo := uint32(1)
	for i := 0; i < digitosTOTP; i++ {
		modulo *= 10
	}
	return fmt.Sprintf("%0*d", digitosTOTP, codigoBinario%modulo)
}

// --- errores de construcción propios de este archivo ------------------------

// ErrTipoFactorInvalido se produce al construir un TipoFactor con un valor
// fuera del catálogo cerrado (solo "totp" en el MVP, ADR 0037).
type ErrTipoFactorInvalido struct{ Valor string }

func (e *ErrTipoFactorInvalido) Error() string { return "tipo de factor desconocido: " + e.Valor }

// ErrSecretoTOTPPlanoInvalido se produce por restricciones estructurales de
// SecretoTOTPPlano (formato Base32 o longitud insuficiente).
type ErrSecretoTOTPPlanoInvalido struct{ Motivo string }

func (e *ErrSecretoTOTPPlanoInvalido) Error() string { return "secreto TOTP inválido: " + e.Motivo }

// ErrSecretoTOTPCifradoInvalido se produce al construir un
// SecretoTOTPCifrado vacío.
type ErrSecretoTOTPCifradoInvalido struct{ Motivo string }

func (e *ErrSecretoTOTPCifradoInvalido) Error() string {
	return "secreto TOTP cifrado inválido: " + e.Motivo
}

// ErrCodigoTOTPInvalido se produce por restricciones estructurales de
// CodigoTOTP (longitud distinta de 6 o caracteres no numéricos).
type ErrCodigoTOTPInvalido struct{ Motivo string }

func (e *ErrCodigoTOTPInvalido) Error() string { return "código TOTP inválido: " + e.Motivo }
