package aplicacion

import (
	"context"
	"log/slog"

	"github.com/r-david1/moterus/internal/tenencia/dominio"
	"github.com/r-david1/moterus/internal/tenencia/puertos"
)

// InvitacionesCasoDeUso implementa puertos.GestorDeInvitaciones: Invitar
// (§3.5 del diseño, este archivo), Revocar (revocar_invitacion.go) y
// Aceptar (aceptar_invitacion.go).
type InvitacionesCasoDeUso struct {
	organizaciones puertos.RepositorioOrganizaciones
	membresias     puertos.RepositorioMembresias
	invitaciones   puertos.RepositorioInvitaciones
	autorizador    puertos.VerificadorDeAutorizacion
	sujetos        puertos.VerificadorDeSujetos
	confianza      puertos.EvaluadorConfianza
	notificador    puertos.NotificadorInvitaciones
	auditoria      puertos.RegistroAuditoria
	eventos        puertos.PublicadorEventos
	reloj          puertos.Reloj
	ids            puertos.GeneradorIDs
	tokens         puertos.GeneradorTokens
	uow            puertos.UnidadDeTrabajo
	politica       dominio.PoliticaOrganizacion
}

var _ puertos.GestorDeInvitaciones = (*InvitacionesCasoDeUso)(nil)

// NuevoInvitacionesCasoDeUso construye el caso de uso con sus dependencias
// inyectadas por puerto.
func NuevoInvitacionesCasoDeUso(
	organizaciones puertos.RepositorioOrganizaciones,
	membresias puertos.RepositorioMembresias,
	invitaciones puertos.RepositorioInvitaciones,
	autorizador puertos.VerificadorDeAutorizacion,
	sujetos puertos.VerificadorDeSujetos,
	confianza puertos.EvaluadorConfianza,
	notificador puertos.NotificadorInvitaciones,
	auditoria puertos.RegistroAuditoria,
	eventos puertos.PublicadorEventos,
	reloj puertos.Reloj,
	ids puertos.GeneradorIDs,
	tokens puertos.GeneradorTokens,
	uow puertos.UnidadDeTrabajo,
	politica dominio.PoliticaOrganizacion,
) *InvitacionesCasoDeUso {
	return &InvitacionesCasoDeUso{
		organizaciones: organizaciones,
		membresias:     membresias,
		invitaciones:   invitaciones,
		autorizador:    autorizador,
		sujetos:        sujetos,
		confianza:      confianza,
		notificador:    notificador,
		auditoria:      auditoria,
		eventos:        eventos,
		reloj:          reloj,
		ids:            ids,
		tokens:         tokens,
		uow:            uow,
		politica:       politica,
	}
}

// Invitar ejecuta el flujo normativo de §3.5 del diseño: autorizar +
// dominancia sobre el rol propuesto (INV-TEN-20), evaluar Confianza (el
// endpoint envía correo a terceros, INV-TEN-23 exige que sea el único
// camino de salida del token en claro), revocar cualquier invitación
// pendiente previa para el mismo (organización, correo) en vez de fallar
// (INV-TEN-22), crear la nueva invitación y —solo después del commit—
// entregar el token en claro vía NotificadorInvitaciones. El token NUNCA
// aparece en la respuesta de este método (VistaInvitacion no tiene campo
// para él).
func (c *InvitacionesCasoDeUso) Invitar(ctx context.Context, cmd puertos.ComandoInvitarMiembro) (puertos.VistaInvitacion, error) {
	idOrganizacion, err := dominio.IDOrganizacionDesde(cmd.IDOrganizacion)
	if err != nil {
		return puertos.VistaInvitacion{}, err
	}
	rolPropuesto, err := dominio.RolDesde(cmd.Rol)
	if err != nil {
		return puertos.VistaInvitacion{}, err
	}
	correo, err := dominio.NuevoCorreoDestinatario(cmd.Correo)
	if err != nil {
		return puertos.VistaInvitacion{}, err
	}
	idInvitador, err := dominio.IDUsuarioDesde(cmd.IDSujeto)
	if err != nil {
		return puertos.VistaInvitacion{}, err
	}

	decision, err := autorizar(ctx, c.autorizador, cmd.IDSujeto, cmd.IDOrganizacion, dominio.PermisoMiembroInvitar, cmd.Origen)
	if err != nil {
		return puertos.VistaInvitacion{}, err
	}
	rolEjecutor, err := dominio.RolDesde(decision.Rol)
	if err != nil {
		return puertos.VistaInvitacion{}, err
	}
	if err := dominio.ValidarOtorgamiento(rolEjecutor, rolPropuesto); err != nil {
		return puertos.VistaInvitacion{}, err
	}

	confianzaDecision, err := c.confianza.Evaluar(ctx, puertos.SolicitudEvaluacion{
		Accion:      "invitar_miembro",
		ClaveCuenta: claveCuentaOrganizacion(idOrganizacion),
		TenantID:    idOrganizacion.String(),
		Origen:      cmd.Origen,
	})
	if err != nil {
		return puertos.VistaInvitacion{}, err
	}
	if !confianzaDecision.Permitido {
		return puertos.VistaInvitacion{}, &dominio.ErrAccesoDenegadoPorConfianza{Motivo: confianzaDecision.Motivo, ReintentarEn: confianzaDecision.ReintentarEn}
	}

	tokenPlano, err := c.tokens.GenerarTokenInvitacion()
	if err != nil {
		return puertos.VistaInvitacion{}, err
	}
	hash := tokenPlano.Hash()
	idInvitacion, err := c.ids.NuevoIDInvitacion()
	if err != nil {
		return puertos.VistaInvitacion{}, err
	}

	var nombreOrganizacion string
	var invitacion *dominio.Invitacion
	var eventos []dominio.EventoDominio
	if err := c.uow.Ejecutar(ctx, func(ctx context.Context) error {
		org, err := c.organizaciones.BuscarPorID(ctx, idOrganizacion)
		if err != nil {
			return err
		}
		if org == nil {
			return &dominio.ErrOrganizacionNoEncontrada{ID: cmd.IDOrganizacion}
		}
		if err := org.EstaOperativa(); err != nil {
			return err
		}
		nombreOrganizacion = org.Nombre().Valor()

		pendientes, err := c.invitaciones.ContarPendientesDeOrganizacion(ctx, idOrganizacion)
		if err != nil {
			return err
		}
		if pendientes >= c.politica.MaximoInvitacionesPendientes() {
			return &dominio.ErrLimiteInvitacionesExcedido{Limite: c.politica.MaximoInvitacionesPendientes()}
		}

		ahora := c.reloj.Ahora()
		existente, err := c.invitaciones.BuscarPendiente(ctx, idOrganizacion, correo)
		if err != nil {
			return err
		}
		if existente != nil {
			if err := existente.Revocar(ahora); err != nil {
				return err
			}
			if err := c.invitaciones.Guardar(ctx, existente); err != nil {
				return err
			}
			for _, e := range existente.EventosPendientes() {
				if err := c.auditoria.Registrar(ctx, e, idOrganizacion, cmd.Origen); err != nil {
					return err
				}
			}
		}

		invitacion, err = dominio.CrearInvitacion(idInvitacion, idOrganizacion, correo, rolPropuesto, hash, idInvitador, ahora, c.politica)
		if err != nil {
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
		return puertos.VistaInvitacion{}, err
	}

	if errPub := c.eventos.Publicar(ctx, eventos...); errPub != nil {
		slog.WarnContext(ctx, "invitar miembro: fallo al publicar eventos",
			"error", errPub, "invitacion_id", invitacion.ID().String())
	}

	// INV-TEN-23: la única salida del token en claro del proceso.
	if err := c.notificador.EnviarInvitacion(ctx, correo, nombreOrganizacion, rolPropuesto, tokenPlano.Valor(), invitacion.ExpiraEn()); err != nil {
		slog.WarnContext(ctx, "invitar miembro: fallo al notificar la invitación",
			"error", err, "invitacion_id", invitacion.ID().String())
	}

	return vistaInvitacionDesde(invitacion), nil
}
