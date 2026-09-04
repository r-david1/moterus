package postgres

import (
	"github.com/r-david1/moterus/internal/plataforma/ids"
	"github.com/r-david1/moterus/internal/tenencia/dominio"
	"github.com/r-david1/moterus/internal/tenencia/puertos"
)

// GeneradorIDs implementa puertos.GeneradorIDs sobre
// internal/plataforma/ids.GenerarUUIDv7 (mismo patrón que
// identidad/adaptadores/postgres.GeneradorIDs y
// acceso/adaptadores/postgres.GeneradorIDs): los tres identificadores de
// Tenencia son UUIDv7 (ordenables, mejor localidad de índice; no son
// secretos, viajan en la ruta HTTP — tabla 1.3 del diseño).
type GeneradorIDs struct{}

var _ puertos.GeneradorIDs = GeneradorIDs{}

// NuevoGeneradorIDs construye el adaptador.
func NuevoGeneradorIDs() GeneradorIDs { return GeneradorIDs{} }

// NuevoIDOrganizacion genera un UUIDv7 nuevo y lo envuelve como
// dominio.IDOrganizacion.
func (GeneradorIDs) NuevoIDOrganizacion() (dominio.IDOrganizacion, error) {
	crudo, err := ids.GenerarUUIDv7()
	if err != nil {
		return dominio.IDOrganizacion{}, err
	}
	return dominio.IDOrganizacionDesde(crudo)
}

// NuevoIDMembresia genera un UUIDv7 nuevo y lo envuelve como
// dominio.IDMembresia.
func (GeneradorIDs) NuevoIDMembresia() (dominio.IDMembresia, error) {
	crudo, err := ids.GenerarUUIDv7()
	if err != nil {
		return dominio.IDMembresia{}, err
	}
	return dominio.IDMembresiaDesde(crudo)
}

// NuevoIDInvitacion genera un UUIDv7 nuevo y lo envuelve como
// dominio.IDInvitacion.
func (GeneradorIDs) NuevoIDInvitacion() (dominio.IDInvitacion, error) {
	crudo, err := ids.GenerarUUIDv7()
	if err != nil {
		return dominio.IDInvitacion{}, err
	}
	return dominio.IDInvitacionDesde(crudo)
}
