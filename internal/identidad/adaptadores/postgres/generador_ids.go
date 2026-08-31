package postgres

import (
	"github.com/r-david1/moterus/internal/identidad/dominio"
	"github.com/r-david1/moterus/internal/identidad/puertos"
	"github.com/r-david1/moterus/internal/plataforma/ids"
)

// GeneradorIDs implementa puertos.GeneradorIDs generando UUIDv7 (ADR
// candidato 0012: ordenable en el tiempo, mejor localidad de índice B-tree
// que UUIDv4 en la tabla usuarios). Vive en el paquete postgres siguiendo la
// agrupación del diseño (sección "Qué implementar" del encargo), aunque no
// toca la base de datos: envuelve el generador técnico neutro de
// internal/plataforma/ids y construye el value object de dominio, que
// plataforma no puede construir sin importar tipos de negocio.
type GeneradorIDs struct{}

var _ puertos.GeneradorIDs = (*GeneradorIDs)(nil)

// NuevoGeneradorIDs construye el adaptador.
func NuevoGeneradorIDs() *GeneradorIDs { return &GeneradorIDs{} }

// NuevoIDUsuario genera un UUIDv7 nuevo y lo envuelve como dominio.IDUsuario.
func (GeneradorIDs) NuevoIDUsuario() (dominio.IDUsuario, error) {
	crudo, err := ids.GenerarUUIDv7()
	if err != nil {
		return dominio.IDUsuario{}, err
	}
	return dominio.IDUsuarioDesde(crudo)
}
