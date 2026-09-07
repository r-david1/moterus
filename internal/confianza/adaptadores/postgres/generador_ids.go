package postgres

import (
	"github.com/r-david1/moterus/internal/confianza/dominio"
	"github.com/r-david1/moterus/internal/confianza/puertos"
	"github.com/r-david1/moterus/internal/plataforma/ids"
)

// GeneradorIDs implementa puertos.GeneradorIDs sobre
// internal/plataforma/ids.GenerarUUIDv7 (mismo patrón que
// identidad/adaptadores/postgres.GeneradorIDs,
// acceso/adaptadores/postgres.GeneradorIDs y
// tenencia/adaptadores/postgres.GeneradorIDs): IDSalaDeEspera es UUIDv7
// (ordenable, mejor localidad de índice B-tree que UUIDv4; no es un
// secreto, viaja en la ruta HTTP de administración — tabla 1.3 del
// diseño).
type GeneradorIDs struct{}

var _ puertos.GeneradorIDs = GeneradorIDs{}

// NuevoGeneradorIDs construye el adaptador.
func NuevoGeneradorIDs() GeneradorIDs { return GeneradorIDs{} }

// NuevoIDSalaDeEspera genera un UUIDv7 nuevo y lo envuelve como
// dominio.IDSalaDeEspera.
func (GeneradorIDs) NuevoIDSalaDeEspera() (dominio.IDSalaDeEspera, error) {
	crudo, err := ids.GenerarUUIDv7()
	if err != nil {
		return dominio.IDSalaDeEspera{}, err
	}
	return dominio.IDSalaDeEsperaDesde(crudo)
}
