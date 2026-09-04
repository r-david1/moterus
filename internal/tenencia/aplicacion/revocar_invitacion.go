package aplicacion

import (
	"context"
	"log/slog"

	"github.com/r-david1/moterus/internal/tenencia/dominio"
	"github.com/r-david1/moterus/internal/tenencia/puertos"
)

// Revocar implementa RevocarInvitacion (§3.5 del diseño): autoriza
// miembro.invitar, transiciona la invitación a revocada (sin DELETE, §6 del
// diseño) y audita. Si la invitación no existe, o existe pero pertenece a
// otra organización, se trata igual que cualquier otra transición
// inválida: dominio.ErrTransicionEstadoInvitacionInvalida (a diferencia de
// un intento de redención por un tercero, esta es una acción
// administrativa y no está sujeta al criterio de indistinguibilidad de
// INV-TEN-24).
func (c *InvitacionesCasoDeUso) Revocar(ctx context.Context, cmd puertos.ComandoRevocarInvitacion) error {
	idOrganizacion, err := dominio.IDOrganizacionDesde(cmd.IDOrganizacion)
	if err != nil {
		return err
	}
	idInvitacion, err := dominio.IDInvitacionDesde(cmd.IDInvitacion)
	if err != nil {
		return err
	}

	if _, err := autorizar(ctx, c.autorizador, cmd.IDSujeto, cmd.IDOrganizacion, dominio.PermisoMiembroInvitar, cmd.Origen); err != nil {
		return err
	}

	var eventos []dominio.EventoDominio
	if err := c.uow.Ejecutar(ctx, func(ctx context.Context) error {
		invitacion, err := c.invitaciones.BuscarPorID(ctx, idInvitacion)
		if err != nil {
			return err
		}
		if invitacion == nil || !invitacion.OrganizacionID().EsIgual(idOrganizacion) {
			return &dominio.ErrTransicionEstadoInvitacionInvalida{Destino: dominio.EstadoInvitacionRevocada}
		}

		if err := invitacion.Revocar(c.reloj.Ahora()); err != nil {
			return err
		}
		if err := c.invitaciones.Guardar(ctx, invitacion); err != nil {
			return err
		}
		eventos = invitacion.EventosPendientes()
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
			slog.WarnContext(ctx, "revocar invitación: fallo al publicar eventos",
				"error", errPub, "invitacion_id", idInvitacion.String())
		}
	}
	return nil
}
