package aplicacion

import (
	"context"
	"time"

	"github.com/r-david1/moterus/internal/confianza/dominio"
	"github.com/r-david1/moterus/internal/confianza/puertos"
)

// ConsultarSalaCasoDeUso implementa puertos.ConsultorDeSalas (§7.1 del
// diseño colas-virtuales.md: GET /confianza/salas-espera/{aliasSala}, el
// endpoint público y cacheable). Igual que PorteroDeSalaCasoDeUso, resuelve
// el alias contra la instantánea en memoria del reconciliador (§3.7) sin
// tocar Postgres, y solo entonces consulta EstadoDeCola.Instantanea (Redis)
// para la longitud aproximada — nunca revela la organización dueña, la ruta
// protegida ni la existencia de otras salas (INV-COLA-15).
type ConsultarSalaCasoDeUso struct {
	instantanea *InstantaneaSalasVigentes
	estadoCola  puertos.EstadoDeCola
}

var _ puertos.ConsultorDeSalas = (*ConsultarSalaCasoDeUso)(nil)

// NuevoConsultarSalaCasoDeUso construye el caso de uso con sus dependencias
// inyectadas por puerto. instantanea se comparte con PorteroDeSalaCasoDeUso
// y ReconciliarSalasCasoDeUso (ver el comentario de InstantaneaSalasVigentes
// en portero_sala.go).
func NuevoConsultarSalaCasoDeUso(instantanea *InstantaneaSalasVigentes, estadoCola puertos.EstadoDeCola) *ConsultarSalaCasoDeUso {
	return &ConsultarSalaCasoDeUso{instantanea: instantanea, estadoCola: estadoCola}
}

// ObtenerPorAlias resuelve la vista pública minimalista de una sala vigente.
// Una sala inexistente o ya cerrada (fuera de la instantánea) es
// ErrSalaNoEncontrada, mismo criterio que PorteroDeSalaCasoDeUso.Ingresar.
func (c *ConsultarSalaCasoDeUso) ObtenerPorAlias(ctx context.Context, q puertos.ConsultaSalaPorAlias) (puertos.VistaSalaPublica, error) {
	alias, err := dominio.NuevoAliasSala(q.Alias)
	if err != nil {
		return puertos.VistaSalaPublica{}, err
	}
	vista, ok := c.instantanea.LeerPorAlias(alias.Normalizado())
	if !ok {
		return puertos.VistaSalaPublica{}, &dominio.ErrSalaNoEncontrada{Referencia: q.Alias}
	}

	instantaneaCola, err := c.estadoCola.Instantanea(ctx, vista.Clave)
	if err != nil {
		return puertos.VistaSalaPublica{}, err
	}

	longitud := instantaneaCola.LongitudAproximada
	if longitud < 0 {
		longitud = 0
	}
	var espera time.Duration
	if vista.RitmoAdmision > 0 {
		espera = time.Duration(longitud) * time.Second / time.Duration(vista.RitmoAdmision)
	}

	return puertos.VistaSalaPublica{
		Alias:              vista.Alias,
		Estado:             vista.Estado,
		LongitudAproximada: longitud,
		EsperaEstimada:     espera,
		ReconsultarEn:      reconsultarEnDesdePosicion(longitud),
	}, nil
}
