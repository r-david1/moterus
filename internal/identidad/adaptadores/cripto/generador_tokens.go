package cripto

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"github.com/r-david1/moterus/internal/identidad/puertos"
)

// bytesTokenVerificacion es la cantidad de bytes de entropía por token
// (sección 3.4 del diseño: "token opaco de 32 bytes vía crypto/rand").
const bytesTokenVerificacion = 32

// GeneradorTokens implementa puertos.GeneradorTokens: secretos aleatorios de
// alta entropía para flujos de un solo uso (verificación de correo). No es
// dominio.IDUsuario/GeneradorIDs: estos tokens son opacos, no ordenables ni
// predecibles (a diferencia de los UUIDv7 de identidad, ADR candidato 0012).
type GeneradorTokens struct{}

var _ puertos.GeneradorTokens = (*GeneradorTokens)(nil)

// NuevoGeneradorTokens construye el adaptador.
func NuevoGeneradorTokens() *GeneradorTokens { return &GeneradorTokens{} }

// Generar produce 32 bytes de crypto/rand y los codifica en base64url sin
// padding (base64.RawURLEncoding): seguro para viajar en una URL de
// verificación de correo sin escapar caracteres.
func (GeneradorTokens) Generar() (string, error) {
	buf := make([]byte, bytesTokenVerificacion)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("cripto: no se pudo generar el token de verificación: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
