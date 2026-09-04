package aplicacion

import (
	"context"
	"log/slog"

	"github.com/r-david1/moterus/internal/tenencia/dominio"
	"github.com/r-david1/moterus/internal/tenencia/puertos"
)

// CambiarRol implementa el caso de uso homónimo (§3.2 del diseño). Es un
// no-op idempotente (sin mutación, sin evento, sin auditoría) si el rol
// nuevo es igual al vigente — lo decide
// dominio.Membresia.CambiarRol, que ya contempla ese camino.
func (c *MembresiasCasoDeUso) CambiarRol(ctx context.Context, cmd puertos.ComandoCambiarRol) (puertos.VistaMiembro, error) {
	idOrganizacion, err := dominio.IDOrganizacionDesde(cmd.IDOrganizacion)
	if err != nil {
		return puertos.VistaMiembro{}, err
	}
	idObjetivo, err := dominio.IDUsuarioDesde(cmd.IDUsuario)
	if err != nil {
		return puertos.VistaMiembro{}, err
	}
	nuevoRol, err := dominio.RolDesde(cmd.NuevoRol)
	if err != nil {
		return puertos.VistaMiembro{}, err
	}

	decision, err := autorizar(ctx, c.autorizador, cmd.IDSujeto, cmd.IDOrganizacion, dominio.PermisoMiembroCambiarRol, cmd.Origen)
	if err != nil {
		return puertos.VistaMiembro{}, err
	}
	rolEjecutor, err := dominio.RolDesde(decision.Rol)
	if err != nil {
		return puertos.VistaMiembro{}, err
	}

	var objetivo *dominio.Membresia
	var eventos []dominio.EventoDominio
	if err := c.uow.Ejecutar(ctx, func(ctx context.Context) error {
		org, err := c.organizaciones.CargarParaActualizar(ctx, idOrganizacion)
		if err != nil {
			return err
		}
		if org == nil {
			return &dominio.ErrOrganizacionNoEncontrada{ID: cmd.IDOrganizacion}
		}
		if err := org.EstaOperativa(); err != nil {
			return err
		}

		objetivo, err = c.membresias.BuscarVigente(ctx, idObjetivo, idOrganizacion)
		if err != nil {
			return err
		}
		if objetivo == nil {
			return &dominio.ErrMembresiaNoEncontrada{}
		}

		propietariosActivos, err := c.membresias.ContarPropietariosActivos(ctx, idOrganizacion)
		if err != nil {
			return err
		}

		if err := objetivo.CambiarRol(nuevoRol, rolEjecutor, propietariosActivos, c.reloj.Ahora()); err != nil {
			return err
		}

		eventos = objetivo.EventosPendientes()
		if len(eventos) == 0 {
			return nil
		}
		if err := c.membresias.Guardar(ctx, objetivo); err != nil {
			return err
		}
		for _, e := range eventos {
			if err := c.auditoria.Registrar(ctx, e, idOrganizacion, cmd.Origen); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return puertos.VistaMiembro{}, err
	}

	if len(eventos) > 0 {
		if errPub := c.eventos.Publicar(ctx, eventos...); errPub != nil {
			slog.WarnContext(ctx, "cambiar rol: fallo al publicar eventos",
				"error", errPub, "organizacion_id", idOrganizacion.String())
		}
	}
	return vistaMiembroDesde(objetivo), nil
}
