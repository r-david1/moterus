// Package cripto implementa los adaptadores criptográficos propios del
// bounded context Acceso que no dependen de JWT (el firmador de tokens de
// acceso vive en su propio paquete, adaptadores/jwt, ADR 0020).
package cripto

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"github.com/r-david1/moterus/internal/acceso/dominio"
	"github.com/r-david1/moterus/internal/acceso/puertos"
)

// bytesTokenRefresco es la cantidad de bytes de entropía por token (tabla
// 1.3 del diseño: "≥32 bytes de entropía de crypto/rand"). 32 bytes en
// base64url sin relleno codifican en 43 caracteres — exactamente
// longitudMinimaSecretoRefresco en acceso/dominio/tokens.go.
const bytesTokenRefresco = 32

// prefijoTokenRefresco debe coincidir con el prefijo que
// acceso/dominio.NuevoTokenRefrescoPlano exige (tokens.go); se declara
// aquí también, en vez de exportarlo desde dominio, porque dominio no
// expone constantes de implementación (INV-ACC-18) y este es el único
// productor de tokens nuevos del sistema.
const prefijoTokenRefresco = "mot_rt_"

// GeneradorTokensRefresco implementa puertos.GeneradorTokensRefresco:
// secretos aleatorios de alta entropía vía crypto/rand, prefijados
// "mot_rt_" (mismo criterio que
// identidad/adaptadores/cripto.GeneradorTokens para los tokens de
// verificación de correo — ver ADR 0019).
type GeneradorTokensRefresco struct{}

var _ puertos.GeneradorTokensRefresco = (*GeneradorTokensRefresco)(nil)

// NuevoGeneradorTokensRefresco construye el adaptador.
func NuevoGeneradorTokensRefresco() *GeneradorTokensRefresco { return &GeneradorTokensRefresco{} }

// Generar produce bytesTokenRefresco bytes de crypto/rand, los codifica en
// base64url sin padding y los envuelve en un dominio.TokenRefrescoPlano ya
// validado (INV-ACC-11: nunca se persiste en claro; solo su hash llega a
// la base de datos).
func (GeneradorTokensRefresco) Generar() (dominio.TokenRefrescoPlano, error) {
	buf := make([]byte, bytesTokenRefresco)
	if _, err := rand.Read(buf); err != nil {
		return dominio.TokenRefrescoPlano{}, fmt.Errorf("cripto: no se pudo generar el token de refresco: %w", err)
	}
	valor := prefijoTokenRefresco + base64.RawURLEncoding.EncodeToString(buf)
	return dominio.NuevoTokenRefrescoPlano(valor)
}
