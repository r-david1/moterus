package aplicacion

import (
	"context"
	"log/slog"

	"github.com/r-david1/moterus/internal/tenencia/dominio"
	"github.com/r-david1/moterus/internal/tenencia/puertos"
)

// Abandonar implementa AbandonarOrganizacion (§3.2 del diseño): no exige
// permiso (un sujeto siempre puede actuar sobre su propia membresía), pero
// sí el invariante del último propietario (INV-TEN-06). El único
// propietario no puede abandonar: primero debe transferir la propiedad
// (TransferirPropiedad); ErrUltimoPropietario ya lleva un mensaje
// accionable en ese sentido.
func (c *MembresiasCasoDeUso) Abandonar(ctx context.Context, cmd puertos.ComandoAbandonarOrganizacion) error {
	idOrganizacion, err := dominio.IDOrganizacionDesde(cmd.IDOrganizacion)
	if err != nil {
		return err
	}
	idSujeto, err := dominio.IDUsuarioDesde(cmd.IDSujeto)
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

		propia, err := c.membresias.BuscarVigente(ctx, idSujeto, idOrganizacion)
		if err != nil {
			return err
		}
		if propia == nil {
			return &dominio.ErrMembresiaNoEncontrada{}
		}

		propietariosActivos, err := c.membresias.ContarPropietariosActivos(ctx, idOrganizacion)
		if err != nil {
			return err
		}

		if err := propia.Abandonar(propietariosActivos, c.reloj.Ahora()); err != nil {
			return err
		}

		if err := c.membresias.Guardar(ctx, propia); err != nil {
			return err
		}
		eventos = propia.EventosPendientes()
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
			slog.WarnContext(ctx, "abandonar organización: fallo al publicar eventos",
				"error", errPub, "organizacion_id", idOrganizacion.String())
		}
	}
	return nil
}
