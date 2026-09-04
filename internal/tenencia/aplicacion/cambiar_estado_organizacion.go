package aplicacion

import (
	"context"
	"log/slog"

	"github.com/r-david1/moterus/internal/tenencia/dominio"
	"github.com/r-david1/moterus/internal/tenencia/puertos"
)

// CambiarEstado implementa SuspenderOrganizacion / ReactivarOrganizacion /
// ArchivarOrganizacion (§3.4 del diseño) según cmd.Destino. Las tres
// transiciones comparten el mismo permiso del catálogo cerrado,
// organizacion.archivar ("permite suspender, reactivar o archivar la
// organización", dominio/permiso.go): no hay un permiso separado por
// transición. No toca ninguna membresía (INV-TEN-05): el acceso de todos
// los miembros se apaga o restituye por conjunción en tiempo de
// autorización, no por escribir N filas.
func (c *OrganizacionesCasoDeUso) CambiarEstado(ctx context.Context, cmd puertos.ComandoCambiarEstadoOrganizacion) (puertos.VistaOrganizacion, error) {
	idOrganizacion, err := dominio.IDOrganizacionDesde(cmd.IDOrganizacion)
	if err != nil {
		return puertos.VistaOrganizacion{}, err
	}

	if _, err := autorizar(ctx, c.autorizador, cmd.IDSujeto, cmd.IDOrganizacion, dominio.PermisoOrganizacionArchivar, cmd.Origen); err != nil {
		return puertos.VistaOrganizacion{}, err
	}

	var org *dominio.Organizacion
	var eventos []dominio.EventoDominio
	if err := c.uow.Ejecutar(ctx, func(ctx context.Context) error {
		var err error
		org, err = c.organizaciones.BuscarPorID(ctx, idOrganizacion)
		if err != nil {
			return err
		}
		if org == nil {
			return &dominio.ErrOrganizacionNoEncontrada{ID: cmd.IDOrganizacion}
		}

		ahora := c.reloj.Ahora()
		switch cmd.Destino {
		case dominio.EstadoOrganizacionSuspendida.String():
			motivo, err := dominio.NuevoMotivo(cmd.Motivo)
			if err != nil {
				return err
			}
			if err := org.Suspender(motivo, ahora); err != nil {
				return err
			}
		case dominio.EstadoOrganizacionActiva.String():
			if err := org.Reactivar(ahora); err != nil {
				return err
			}
		case dominio.EstadoOrganizacionArchivada.String():
			motivo, err := dominio.NuevoMotivo(cmd.Motivo)
			if err != nil {
				return err
			}
			if err := org.Archivar(motivo, ahora); err != nil {
				return err
			}
		default:
			return &dominio.ErrEstadoOrganizacionInvalido{Valor: cmd.Destino}
		}

		if err := c.organizaciones.Guardar(ctx, org); err != nil {
			return err
		}
		eventos = org.EventosPendientes()
		for _, e := range eventos {
			if err := c.auditoria.Registrar(ctx, e, idOrganizacion, cmd.Origen); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return puertos.VistaOrganizacion{}, err
	}

	if len(eventos) > 0 {
		if errPub := c.eventos.Publicar(ctx, eventos...); errPub != nil {
			slog.WarnContext(ctx, "cambiar estado de organización: fallo al publicar eventos",
				"error", errPub, "organizacion_id", idOrganizacion.String())
		}
	}

	miembrosActivos, err := c.membresias.ContarActivasDeOrganizacion(ctx, idOrganizacion)
	if err != nil {
		return puertos.VistaOrganizacion{}, err
	}
	return vistaOrganizacionDesde(org, miembrosActivos), nil
}
