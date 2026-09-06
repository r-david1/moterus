package cripto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"

	"github.com/r-david1/moterus/internal/identidad/dominio"
	"github.com/r-david1/moterus/internal/identidad/puertos"
)

// tamanoLlaveAESGCM es la longitud en bytes de la llave AES-256 (32 bytes =
// 256 bits, docs/design/otp-mfa.md §2.2/§2.4).
const tamanoLlaveAESGCM = 32

// CifradorSecretosAESGCM implementa puertos.CifradorSecretos con
// AES-256-GCM: cifrado SIMÉTRICO REVERSIBLE (a diferencia de
// HasherArgon2id) porque el servidor necesita descifrar el secreto TOTP en
// cada verificación para computar el código esperado (INV-MFA-02). El
// nonce (12 bytes, tamaño estándar de GCM) es aleatorio por operación
// (crypto/rand) y se prepende al ciphertext — patrón estándar de AES-GCM,
// para no necesitar una columna aparte en la migración: Descifrar separa
// los primeros NonceSize() bytes antes de abrir el resto.
type CifradorSecretosAESGCM struct {
	gcm cipher.AEAD
}

var _ puertos.CifradorSecretos = (*CifradorSecretosAESGCM)(nil)

// NuevoCifradorSecretosAESGCM construye el adaptador a partir de una llave
// AES-256 cruda (32 bytes exactos, ya decodificada de base64 por el
// llamador — ver DecodificarLlaveCifradoMFA/GenerarLlaveCifradoMFAEfimera
// para las dos formas en que cmd/api/main.go la obtiene).
func NuevoCifradorSecretosAESGCM(llave []byte) (*CifradorSecretosAESGCM, error) {
	if len(llave) != tamanoLlaveAESGCM {
		return nil, fmt.Errorf("cripto: la llave de cifrado de MFA debe tener %d bytes (AES-256), tiene %d", tamanoLlaveAESGCM, len(llave))
	}
	bloque, err := aes.NewCipher(llave)
	if err != nil {
		return nil, fmt.Errorf("cripto: no se pudo construir el cifrador AES: %w", err)
	}
	gcm, err := cipher.NewGCM(bloque)
	if err != nil {
		return nil, fmt.Errorf("cripto: no se pudo construir GCM: %w", err)
	}
	return &CifradorSecretosAESGCM{gcm: gcm}, nil
}

// Cifrar produce nonce||ciphertext||tag (todo concatenado, formato estándar
// de AES-GCM: el nonce no es secreto, solo debe ser único por llave).
func (c *CifradorSecretosAESGCM) Cifrar(secreto dominio.SecretoTOTPPlano) (dominio.SecretoTOTPCifrado, error) {
	nonce := make([]byte, c.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return dominio.SecretoTOTPCifrado{}, fmt.Errorf("cripto: no se pudo generar el nonce de cifrado: %w", err)
	}
	sellado := c.gcm.Seal(nonce, nonce, []byte(secreto.Valor()), nil)
	return dominio.NuevoSecretoTOTPCifrado(sellado)
}

// Descifrar separa el nonce (los primeros NonceSize() bytes) del resto y
// abre el ciphertext. Un valor corrupto o cifrado con otra llave produce un
// error de autenticación de GCM (nunca un pánico ni un descifrado parcial
// silencioso).
func (c *CifradorSecretosAESGCM) Descifrar(cifrado dominio.SecretoTOTPCifrado) (dominio.SecretoTOTPPlano, error) {
	valor := cifrado.Valor()
	tamanoNonce := c.gcm.NonceSize()
	if len(valor) < tamanoNonce {
		return dominio.SecretoTOTPPlano{}, fmt.Errorf("cripto: secreto TOTP cifrado demasiado corto para contener un nonce")
	}
	nonce, texto := valor[:tamanoNonce], valor[tamanoNonce:]
	plano, err := c.gcm.Open(nil, nonce, texto, nil)
	if err != nil {
		return dominio.SecretoTOTPPlano{}, fmt.Errorf("cripto: no se pudo descifrar el secreto TOTP: %w", err)
	}
	return dominio.NuevoSecretoTOTPPlano(string(plano))
}

// DecodificarLlaveCifradoMFA decodifica IDENTIDAD_LLAVE_CIFRADO_MFA (base64
// estándar o URL-safe, con o sin padding — mismo criterio flexible que
// acceso/adaptadores/jwt.cargarLlaveEd25519Privada) a los 32 bytes crudos
// que exige AES-256.
func DecodificarLlaveCifradoMFA(valor string) ([]byte, error) {
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(valor); err == nil {
			if len(b) != tamanoLlaveAESGCM {
				return nil, fmt.Errorf("cripto: IDENTIDAD_LLAVE_CIFRADO_MFA decodifica a %d bytes, se esperaban %d (AES-256)", len(b), tamanoLlaveAESGCM)
			}
			return b, nil
		}
	}
	return nil, fmt.Errorf("cripto: IDENTIDAD_LLAVE_CIFRADO_MFA no es base64 válido")
}

// GenerarLlaveCifradoMFAEfimera produce 32 bytes de crypto/rand,
// codificados en base64 estándar (mismo formato que
// DecodificarLlaveCifradoMFA acepta). Es lo que cmd/api/main.go usa cuando
// IDENTIDAD_LLAVE_CIFRADO_MFA falta fuera de APP_ENV=production (con el
// WARN explícito de que los secretos MFA cifrados con ella quedan
// indescifrables al reiniciar el proceso — mismo patrón que
// accesojwt.GenerarLlaveEfimera).
func GenerarLlaveCifradoMFAEfimera() (string, error) {
	llave := make([]byte, tamanoLlaveAESGCM)
	if _, err := rand.Read(llave); err != nil {
		return "", fmt.Errorf("cripto: no se pudo generar la llave efímera de cifrado de MFA: %w", err)
	}
	return base64.StdEncoding.EncodeToString(llave), nil
}
