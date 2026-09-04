package aplicacion

import (
	"context"
	"log/slog"

	"github.com/r-david1/moterus/internal/tenencia/dominio"
	"github.com/r-david1/moterus/internal/tenencia/puertos"
)

// Remover implementa RemoverMiembro (§3.2 del diseño): transición a
// removida (nunca DELETE, INV-TEN-08). No revoca sesiones de Acceso: no
// hay nada que revocar (las sesiones no llevan organización, INV-ACC-12);
// la siguiente petición del ex-miembro a esa organización simplemente será
// denegada por VerificadorDeAutorizacion, sin ventana de caché
// (INV-TEN-15).
func (c *MembresiasCasoDeUso) Remover(ctx context.Context, cmd puertos.ComandoRemoverMiembro) error {
	idOrganizacion, err := dominio.IDOrganizacionDesde(cmd.IDOrganizacion)
	if err != nil {
		return err
	}
	idObjetivo, err := dominio.IDUsuarioDesde(cmd.IDUsuario)
	if err != nil {
		return err
	}

	decision, err := autorizar(ctx, c.autorizador, cmd.IDSujeto, cmd.IDOrganizacion, dominio.PermisoMiembroRemover, cmd.Origen)
	if err != nil {
		return err
	}
	rolEjecutor, err := dominio.RolDesde(decision.Rol)
	if err != nil {
		return err
	}

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

		objetivo, err := c.membresias.BuscarVigente(ctx, idObjetivo, idOrganizacion)
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

		if err := objetivo.Remover(rolEjecutor, propietariosActivos, c.reloj.Ahora()); err != nil {
			return err
		}

		if err := c.membresias.Guardar(ctx, objetivo); err != nil {
			return err
		}
		eventos = objetivo.EventosPendientes()
		for _, e := range eventos {
			if err := c.auditoria.Registrar(ctx, e, idOrganizacion, cmd.Origen); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}

	if len(eventos) > 0 {
		if errPub := c.eventos.Publicar(ctx, eventos...); errPub != nil {
			slog.WarnContext(ctx, "remover miembro: fallo al publicar eventos",
				"error", errPub, "organizacion_id", idOrganizacion.String())
		}
	}
	return nil
}
