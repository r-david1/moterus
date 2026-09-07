package aplicacion

import (
	"context"

	"github.com/r-david1/moterus/internal/confianza/dominio"
	"github.com/r-david1/moterus/internal/confianza/puertos"
)

// CambiarRitmoDeAdmisionCasoDeUso implementa
// puertos.GestorDeSalasDeEspera.CambiarRitmo (§3.2 del diseño
// colas-virtuales.md): el caso de uso que se usa DURANTE el pico, con el
// sistema real bajo carga y un operador mirando un dashboard.
type CambiarRitmoDeAdmisionCasoDeUso struct {
	salas      puertos.RepositorioSalasDeEspera
	estadoCola puertos.EstadoDeCola
	auditoria  puertos.RegistroAuditoria
	reloj      puertos.Reloj
	uow        puertos.UnidadDeTrabajo
}

// NuevoCambiarRitmoDeAdmisionCasoDeUso construye el caso de uso con sus
// dependencias inyectadas por puerto.
func NuevoCambiarRitmoDeAdmisionCasoDeUso(
	salas puertos.RepositorioSalasDeEspera,
	estadoCola puertos.EstadoDeCola,
	auditoria puertos.RegistroAuditoria,
	reloj puertos.Reloj,
	uow puertos.UnidadDeTrabajo,
) *CambiarRitmoDeAdmisionCasoDeUso {
	return &CambiarRitmoDeAdmisionCasoDeUso{
		salas:      salas,
		estadoCola: estadoCola,
		auditoria:  auditoria,
		reloj:      reloj,
		uow:        uow,
	}
}

// CambiarRitmo carga la sala, calcula la longitud aproximada de la cola en
// este instante (EstadoDeCola.Instantanea — el dominio no la consulta por sí
// mismo, INV-COLA-08), invoca dominio.SalaDeEspera.CambiarRitmo (que
// recalcula el ancla del reloj de admisión para continuidad, §1.5),
// reproyecta a Redis con la Version incrementada y, solo si eso tuvo éxito,
// persiste y audita RitmoDeAdmisionCambiado en la misma unidad de trabajo
// (INV-COLA-11). Mismo criterio fail-closed que AbrirSalaCasoDeUso: si la
// proyección a Redis falla, el ritmo nunca se persiste como cambiado.
func (c *CambiarRitmoDeAdmisionCasoDeUso) CambiarRitmo(ctx context.Context, cmd puertos.ComandoCambiarRitmoAdmision) (puertos.VistaSala, error) {
	id, err := dominio.IDSalaDeEsperaDesde(cmd.IDSala)
	if err != nil {
		return puertos.VistaSala{}, err
	}
	nuevoRitmo, err := dominio.NuevoRitmoAdmision(cmd.RitmoAdmision)
	if err != nil {
		return puertos.VistaSala{}, err
	}

	sala, err := c.salas.BuscarPorID(ctx, id)
	if err != nil {
		return puertos.VistaSala{}, err
	}
	if sala == nil {
		return puertos.VistaSala{}, &dominio.ErrSalaNoEncontrada{Referencia: cmd.IDSala}
	}

	instantanea, err := c.estadoCola.Instantanea(ctx, sala.Clave().String())
	if err != nil {
		return puertos.VistaSala{}, err
	}

	ahora := c.reloj.Ahora()
	if err := sala.CambiarRitmo(nuevoRitmo, instantanea.LongitudAproximada, ahora); err != nil {
		return puertos.VistaSala{}, err
	}

	if err := c.estadoCola.Proyectar(ctx, proyeccionDesde(sala)); err != nil {
		return puertos.VistaSala{}, err
	}

	eventos := sala.EventosPendientes()
	if err := c.uow.Ejecutar(ctx, func(ctx context.Context) error {
		if err := c.salas.Guardar(ctx, sala); err != nil {
			return err
		}
		for _, e := range eventos {
			if err := c.auditoria.Registrar(ctx, e, cmd.Origen); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return puertos.VistaSala{}, err
	}

	return vistaSalaDesde(sala), nil
}
