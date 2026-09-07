package aplicacion

import (
	"context"

	"github.com/r-david1/moterus/internal/confianza/dominio"
	"github.com/r-david1/moterus/internal/confianza/puertos"
)

// CambiarEstadoSalaCasoDeUso implementa
// puertos.GestorDeSalasDeEspera.CambiarEstado (§3.3 del diseño
// colas-virtuales.md): cubre las tres transiciones operables desde afuera de
// la máquina de estados de §1.6 — drenar, cerrar y reabrir (reapertura
// durante el drenaje) — según cmd.Destino. Es un único caso de uso porque
// las tres comparten la misma forma exacta (cargar, mutar según destino,
// reproyectar/retirar en Redis, persistir y auditar en la misma unidad de
// trabajo): igual criterio que
// tenencia/aplicacion.OrganizacionesCasoDeUso.CambiarEstado, que también
// cubre tres transiciones (suspender/reactivar/archivar) en un solo método.
type CambiarEstadoSalaCasoDeUso struct {
	salas      puertos.RepositorioSalasDeEspera
	estadoCola puertos.EstadoDeCola
	auditoria  puertos.RegistroAuditoria
	reloj      puertos.Reloj
	uow        puertos.UnidadDeTrabajo
}

// NuevoCambiarEstadoSalaCasoDeUso construye el caso de uso con sus
// dependencias inyectadas por puerto.
func NuevoCambiarEstadoSalaCasoDeUso(
	salas puertos.RepositorioSalasDeEspera,
	estadoCola puertos.EstadoDeCola,
	auditoria puertos.RegistroAuditoria,
	reloj puertos.Reloj,
	uow puertos.UnidadDeTrabajo,
) *CambiarEstadoSalaCasoDeUso {
	return &CambiarEstadoSalaCasoDeUso{
		salas:      salas,
		estadoCola: estadoCola,
		auditoria:  auditoria,
		reloj:      reloj,
		uow:        uow,
	}
}

// CambiarEstado carga la sala, valida cmd.Destino contra el catálogo cerrado
// de EstadoSala y despacha a Abrir/Drenar/Cerrar del agregado según
// corresponda:
//
//   - destino "abierta" (reapertura durante el drenaje): sala.Abrir(ahora),
//     sin necesitar longitud ni totales (misma firma que en
//     AbrirSalaCasoDeUso) — luego reproyecta la configuración a Redis.
//   - destino "drenando"/"cerrada": primero EstadoDeCola.Instantanea para
//     obtener ingresosTotales/admitidosTotales (el dominio no los consulta
//     por sí mismo, INV-COLA-08); Instantanea.Ingresos es el total de
//     ingresos emitidos (contador `seq`) e Instantanea.Cursor es el total
//     efectivamente admitido hasta ahora (el cursor de admisión, que solo
//     avanza) — luego sala.Drenar/Cerrar con esos totales.
//   - "drenando" reproyecta (la sala sigue viva en Redis, ya no admite
//     ingresos nuevos pero los tickets vivos siguen avanzando); "cerrada"
//     retira la proyección (libera las claves de Redis; los tickets vivos
//     caducan solos por TTL, §3.3 del diseño).
//
// Cualquier destino fuera de las tres transiciones soportadas por este
// comando (incluida "programada", que nunca es un destino administrable)
// produce ErrTransicionEstadoSalaInvalida. Persiste y audita
// SalaDeEsperaCerrada (o SalaDeEsperaAbierta, en la reapertura) en la misma
// unidad de trabajo (INV-COLA-11), solo tras reproyectar/retirar con éxito
// en Redis (mismo criterio fail-closed que los otros dos casos de uso de
// administración).
func (c *CambiarEstadoSalaCasoDeUso) CambiarEstado(ctx context.Context, cmd puertos.ComandoCambiarEstadoSala) (puertos.VistaSala, error) {
	id, err := dominio.IDSalaDeEsperaDesde(cmd.IDSala)
	if err != nil {
		return puertos.VistaSala{}, err
	}
	destino, err := dominio.EstadoSalaDesde(cmd.Destino)
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

	ahora := c.reloj.Ahora()
	clave := sala.Clave().String()

	switch {
	case destino.EsIgual(dominio.EstadoSalaAbierta):
		if err := sala.Abrir(ahora); err != nil {
			return puertos.VistaSala{}, err
		}
		if err := c.estadoCola.Proyectar(ctx, proyeccionDesde(sala)); err != nil {
			return puertos.VistaSala{}, err
		}
	case destino.EsIgual(dominio.EstadoSalaDrenando):
		instantanea, err := c.estadoCola.Instantanea(ctx, clave)
		if err != nil {
			return puertos.VistaSala{}, err
		}
		if err := sala.Drenar(instantanea.Ingresos, instantanea.Cursor, ahora); err != nil {
			return puertos.VistaSala{}, err
		}
		if err := c.estadoCola.Proyectar(ctx, proyeccionDesde(sala)); err != nil {
			return puertos.VistaSala{}, err
		}
	case destino.EsIgual(dominio.EstadoSalaCerrada):
		instantanea, err := c.estadoCola.Instantanea(ctx, clave)
		if err != nil {
			return puertos.VistaSala{}, err
		}
		if err := sala.Cerrar(instantanea.Ingresos, instantanea.Cursor, ahora); err != nil {
			return puertos.VistaSala{}, err
		}
		if err := c.estadoCola.Retirar(ctx, clave); err != nil {
			return puertos.VistaSala{}, err
		}
	default:
		return puertos.VistaSala{}, &dominio.ErrTransicionEstadoSalaInvalida{Origen: sala.Estado(), Destino: destino}
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
