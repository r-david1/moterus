// Package cripto implementa los adaptadores criptográficos propios del
// bounded context Confianza (mismo patrón que
// tenencia/adaptadores/cripto.GeneradorTokens,
// identidad/adaptadores/cripto.GeneradorTokens y
// acceso/adaptadores/cripto.GeneradorTokensRefresco): un paquete propio por
// contexto, nunca uno compartido, para que cada uno pueda declarar su
// propio prefijo y su propia cantidad de bytes de entropía sin acoplarse a
// los demás.
package cripto

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"github.com/r-david1/moterus/internal/confianza/dominio"
	"github.com/r-david1/moterus/internal/confianza/puertos"
)

// bytesTicket es la cantidad de bytes de entropía por ticket de cola
// (tabla 1.3 del diseño colas-virtuales.md: "32 bytes de entropía", mismo
// criterio que el token de refresco de Acceso y el token de invitación de
// Tenencia). 32 bytes en base64url sin relleno codifican en 43 caracteres
// — longitudMinimaSecretoTicket en confianza/dominio/ticket_cola.go.
const bytesTicket = 32

// prefijoTicket debe coincidir con el prefijo que dominio.NuevoTicketPlano
// exige; se declara aquí también (no se exporta desde dominio, mismo
// criterio ya aplicado en Acceso/Identidad/Tenencia: "dominio no expone
// constantes de implementación") porque este es el único productor de
// tickets de cola nuevos del sistema.
const prefijoTicket = "mot_cola_"

// GeneradorTickets implementa puertos.GeneradorTickets: secretos aleatorios
// de alta entropía vía crypto/rand, prefijados "mot_cola_" (§1.4 del
// diseño: token opaco, no JWT — el ticket no lleva ningún dato adentro,
// solo es un identificador impredecible del que Redis guarda el hash).
type GeneradorTickets struct{}

var _ puertos.GeneradorTickets = GeneradorTickets{}

// NuevoGeneradorTickets construye el adaptador.
func NuevoGeneradorTickets() GeneradorTickets { return GeneradorTickets{} }

// GenerarTicket produce bytesTicket bytes de crypto/rand, los codifica en
// base64url sin relleno y los envuelve en un dominio.TicketPlano ya
// validado (INV-COLA-07: nunca se persiste en claro; solo su SHA-256 llega
// a Redis).
func (GeneradorTickets) GenerarTicket() (dominio.TicketPlano, error) {
	buf := make([]byte, bytesTicket)
	if _, err := rand.Read(buf); err != nil {
		return dominio.TicketPlano{}, fmt.Errorf("cripto: no se pudo generar el ticket de cola: %w", err)
	}
	valor := prefijoTicket + base64.RawURLEncoding.EncodeToString(buf)
	return dominio.NuevoTicketPlano(valor)
}
