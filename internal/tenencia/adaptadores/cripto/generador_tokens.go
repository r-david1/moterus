// Package cripto implementa los adaptadores criptográficos propios del
// bounded context Tenencia (mismo patrón que
// acceso/adaptadores/cripto/generador_tokens_refresco.go e
// identidad/adaptadores/cripto.GeneradorTokens).
package cripto

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"github.com/r-david1/moterus/internal/tenencia/dominio"
	"github.com/r-david1/moterus/internal/tenencia/puertos"
)

// bytesTokenInvitacion es la cantidad de bytes de entropía por token (tabla
// 1.3 del diseño: "32 bytes de entropía", mismo criterio que el token de
// refresco de Acceso). 32 bytes en base64url sin relleno codifican en 43
// caracteres — longitudMinimaSecretoInvitacion en tenencia/dominio/tokens.go.
const bytesTokenInvitacion = 32

// prefijoTokenInvitacion debe coincidir con el prefijo que
// dominio.NuevoTokenInvitacionPlano exige; se declara aquí también (no se
// exporta desde dominio, INV-TEN-27 más el criterio de "dominio no expone
// constantes de implementación" ya aplicado en Acceso/Identidad) porque
// este es el único productor de tokens de invitación nuevos del sistema.
const prefijoTokenInvitacion = "mot_inv_"

// GeneradorTokens implementa puertos.GeneradorTokens: secretos aleatorios
// de alta entropía vía crypto/rand, prefijados "mot_inv_".
type GeneradorTokens struct{}

var _ puertos.GeneradorTokens = GeneradorTokens{}

// NuevoGeneradorTokens construye el adaptador.
func NuevoGeneradorTokens() GeneradorTokens { return GeneradorTokens{} }

// GenerarTokenInvitacion produce bytesTokenInvitacion bytes de
// crypto/rand, los codifica en base64url sin padding y los envuelve en un
// dominio.TokenInvitacionPlano ya validado (INV-TEN-23: nunca se persiste
// en claro; solo su hash llega a la base de datos).
func (GeneradorTokens) GenerarTokenInvitacion() (dominio.TokenInvitacionPlano, error) {
	buf := make([]byte, bytesTokenInvitacion)
	if _, err := rand.Read(buf); err != nil {
		return dominio.TokenInvitacionPlano{}, fmt.Errorf("cripto: no se pudo generar el token de invitación: %w", err)
	}
	valor := prefijoTokenInvitacion + base64.RawURLEncoding.EncodeToString(buf)
	return dominio.NuevoTokenInvitacionPlano(valor)
}
