package postgres

import (
	"github.com/google/uuid"

	"github.com/r-david1/moterus/internal/acceso/dominio"
	"github.com/r-david1/moterus/internal/acceso/puertos"
	"github.com/r-david1/moterus/internal/plataforma/ids"
)

// GeneradorIDs implementa puertos.GeneradorIDs. Vive en el paquete postgres
// siguiendo la agrupación del diseño (sección 5), aunque no toca la base de
// datos: envuelve generadores técnicos neutros de internal/plataforma/ids
// (y, para el jti, google/uuid directamente) y construye los value objects
// de dominio, que plataforma no puede construir sin importar tipos de
// negocio.
type GeneradorIDs struct{}

var _ puertos.GeneradorIDs = (*GeneradorIDs)(nil)

// NuevoGeneradorIDs construye el adaptador.
func NuevoGeneradorIDs() *GeneradorIDs { return &GeneradorIDs{} }

// NuevoIDSesion genera un UUIDv7 nuevo y lo envuelve como dominio.IDSesion
// (tabla 1.3 del diseño: ordenable, mejora la localidad de índice).
func (GeneradorIDs) NuevoIDSesion() (dominio.IDSesion, error) {
	crudo, err := ids.GenerarUUIDv7()
	if err != nil {
		return dominio.IDSesion{}, err
	}
	return dominio.IDSesionDesde(crudo)
}

// NuevoIDTokenAcceso genera un UUIDv4 nuevo y lo envuelve como
// dominio.IDTokenAcceso (tabla 1.3 del diseño: deliberadamente NO UUIDv7,
// para que el jti no filtre el instante de emisión). internal/plataforma/ids
// solo expone GenerarUUIDv7 hoy (Identidad no necesitó nunca un v4), así
// que este adaptador usa google/uuid directamente en vez de extender
// plataforma/ids para un único llamador.
func (GeneradorIDs) NuevoIDTokenAcceso() (dominio.IDTokenAcceso, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return dominio.IDTokenAcceso{}, err
	}
	return dominio.IDTokenAccesoDesde(id.String())
}
