package aplicacion

import (
	"context"
	"log/slog"

	"github.com/r-david1/moterus/internal/tenencia/dominio"
	"github.com/r-david1/moterus/internal/tenencia/puertos"
)

// Actualizar renombra y/o cambia el alias de una organización
// (puertos.GestorDeOrganizaciones.Actualizar, listado en la estructura de
// carpetas del §5 del diseño). Sigue el mismo esqueleto general de §3.2 sin
// la parte de conteo de propietarios (no aplica: renombrar no puede dejar
// la organización sin propietario): autorizar, cargar dentro de la
// transacción (para cerrar la ventana TOCTOU entre la lectura de Autorizar
// y la mutación), comprobar que la organización sigue operativa, aplicar
// los cambios idempotentes de dominio.Organizacion.Renombrar/CambiarAlias, y
// auditar solo si hubo mutación real.
func (c *OrganizacionesCasoDeUso) Actualizar(ctx context.Context, cmd puertos.ComandoActualizarOrganizacion) (puertos.VistaOrganizacion, error) {
	idOrganizacion, err := dominio.IDOrganizacionDesde(cmd.IDOrganizacion)
	if err != nil {
		return puertos.VistaOrganizacion{}, err
	}

	if _, err := autorizar(ctx, c.autorizador, cmd.IDSujeto, cmd.IDOrganizacion, dominio.PermisoOrganizacionEditar, cmd.Origen); err != nil {
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
		if err := org.EstaOperativa(); err != nil {
			return err
		}

		ahora := c.reloj.Ahora()
		if cmd.Nombre != nil {
			nombre, err := dominio.NuevoNombre(*cmd.Nombre)
			if err != nil {
				return err
			}
			if err := org.Renombrar(nombre, ahora); err != nil {
				return err
			}
		}
		if cmd.Alias != nil {
			alias, err := dominio.NuevoAlias(*cmd.Alias)
			if err != nil {
				return err
			}
			if err := org.CambiarAlias(alias, ahora); err != nil {
				return err
			}
		}

		eventos = org.EventosPendientes()
		if len(eventos) == 0 {
			// No-op idempotente (mismos valores enviados): sin mutación, sin
			// auditoría.
			return nil
		}
		if err := c.organizaciones.Guardar(ctx, org); err != nil {
			return err
		}
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
			slog.WarnContext(ctx, "actualizar organización: fallo al publicar eventos",
				"error", errPub, "organizacion_id", idOrganizacion.String())
		}
	}

	miembrosActivos, err := c.membresias.ContarActivasDeOrganizacion(ctx, idOrganizacion)
	if err != nil {
		return puertos.VistaOrganizacion{}, err
	}
	return vistaOrganizacionDesde(org, miembrosActivos), nil
}
