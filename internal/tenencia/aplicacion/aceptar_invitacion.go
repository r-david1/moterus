package aplicacion

import (
	"context"
	"log/slog"
	"time"

	"github.com/r-david1/moterus/internal/tenencia/dominio"
	"github.com/r-david1/moterus/internal/tenencia/puertos"
)

// Aceptar implementa AceptarInvitacion (§3.5 del diseño), el caso de uso
// con más aristas del contexto:
//
//  1. Requiere autenticación (IDSujeto ya resuelto por el middleware) pero
//     NO autorización de Tenencia: quien acepta no es miembro todavía.
//  2. Evalúa Confianza por IP (mismo criterio que RenovarSesion en Acceso:
//     el endpoint es un oráculo de fuerza bruta sobre un secreto opaco).
//  3. Construye el token en claro (rechazo estructural sin tocar la base)
//     y lo hashea.
//  4. Busca la invitación por hash. Token desconocido, malformado,
//     revocado, ya aceptado o expirado producen la MISMA respuesta
//     observable, ErrInvitacionInvalida (INV-TEN-24); dominio.Invitacion.Aceptar
//     ya centraliza la comprobación de vigencia y de coincidencia de
//     correo (en tiempo constante, INV-TEN-21), así que este caso de uso no
//     la duplica.
//  5. Si el sujeto ya tiene una membresía vigente en la organización
//     (aceptó dos veces, o lo agregaron mientras tanto), la invitación se
//     marca aceptada igualmente pero NO se crea una segunda membresía ni se
//     audita un alta que no ocurrió: idempotente.
//  6. Invitacion.Aceptar (mutación en memoria) + creación de la Membresia
//     ocurren, cuando corresponde, en la MISMA UnidadDeTrabajo.
func (c *InvitacionesCasoDeUso) Aceptar(ctx context.Context, cmd puertos.ComandoAceptarInvitacion) (puertos.VistaMiembro, error) {
	idSujeto, err := dominio.IDUsuarioDesde(cmd.IDSujeto)
	if err != nil {
		return puertos.VistaMiembro{}, err
	}

	confianzaDecision, err := c.confianza.Evaluar(ctx, puertos.SolicitudEvaluacion{
		Accion:      "aceptar_invitacion",
		ClaveCuenta: claveCuentaPorIP(cmd.Origen),
		Origen:      cmd.Origen,
	})
	if err != nil {
		return puertos.VistaMiembro{}, err
	}
	if !confianzaDecision.Permitido {
		return puertos.VistaMiembro{}, &dominio.ErrAccesoDenegadoPorConfianza{Motivo: confianzaDecision.Motivo, ReintentarEn: confianzaDecision.ReintentarEn}
	}

	ahora := c.reloj.Ahora()

	tokenPlano, err := dominio.NuevoTokenInvitacionPlano(cmd.TokenPlano)
	if err != nil {
		if errAud := c.auditarInvitacionFallida(ctx, "", dominio.IDOrganizacion{}, ahora, cmd.Origen); errAud != nil {
			return puertos.VistaMiembro{}, errAud
		}
		return puertos.VistaMiembro{}, &dominio.ErrInvitacionInvalida{}
	}

	invitacion, err := c.invitaciones.BuscarPorHash(ctx, tokenPlano.Hash())
	if err != nil {
		return puertos.VistaMiembro{}, err
	}
	if invitacion == nil {
		if errAud := c.auditarInvitacionFallida(ctx, "", dominio.IDOrganizacion{}, ahora, cmd.Origen); errAud != nil {
			return puertos.VistaMiembro{}, errAud
		}
		return puertos.VistaMiembro{}, &dominio.ErrInvitacionInvalida{}
	}

	sujeto, err := c.sujetos.EsElegible(ctx, idSujeto.String())
	if err != nil {
		return puertos.VistaMiembro{}, err
	}
	if !sujeto.Existe || !sujeto.Activo {
		return puertos.VistaMiembro{}, &dominio.ErrSujetoNoElegible{}
	}
	correoSujeto, err := dominio.NuevoCorreoDestinatario(sujeto.CorreoNormalizado)
	if err != nil {
		return puertos.VistaMiembro{}, err
	}

	if errAceptar := invitacion.Aceptar(correoSujeto, ahora); errAceptar != nil {
		if errAud := c.auditarInvitacionFallida(ctx, invitacion.ID().String(), invitacion.OrganizacionID(), ahora, cmd.Origen); errAud != nil {
			return puertos.VistaMiembro{}, errAud
		}
		return puertos.VistaMiembro{}, errAceptar
	}

	idOrganizacion := invitacion.OrganizacionID()
	var membresiaResultado *dominio.Membresia
	var eventosMembresia []dominio.EventoDominio
	if err := c.uow.Ejecutar(ctx, func(ctx context.Context) error {
		org, err := c.organizaciones.BuscarPorID(ctx, idOrganizacion)
		if err != nil {
			return err
		}
		if org == nil {
			return &dominio.ErrOrganizacionNoEncontrada{ID: idOrganizacion.String()}
		}
		if err := org.EstaOperativa(); err != nil {
			return err
		}

		existente, err := c.membresias.BuscarVigente(ctx, idSujeto, idOrganizacion)
		if err != nil {
			return err
		}
		if existente != nil {
			// Idempotente: la invitación se consume, pero no se duplica la
			// membresía ni se audita un alta que no ocurrió.
			if err := c.invitaciones.Guardar(ctx, invitacion); err != nil {
				return err
			}
			for _, e := range invitacion.EventosPendientes() {
				if err := c.auditoria.Registrar(ctx, e, idOrganizacion, cmd.Origen); err != nil {
					return err
				}
			}
			membresiaResultado = existente
			return nil
		}

		activas, err := c.membresias.ContarActivasDeOrganizacion(ctx, idOrganizacion)
		if err != nil {
			return err
		}
		if activas >= c.politica.MaximoMiembrosActivos() {
			return &dominio.ErrLimiteMiembrosExcedido{Limite: c.politica.MaximoMiembrosActivos()}
		}

		idMembresia, err := c.ids.NuevoIDMembresia()
		if err != nil {
			return err
		}
		nueva, err := dominio.CrearMembresiaDesdeInvitacion(idMembresia, idOrganizacion, idSujeto, invitacion.RolPropuesto(), invitacion.InvitadaPor(), ahora)
		if err != nil {
			return err
		}

		if err := c.invitaciones.Guardar(ctx, invitacion); err != nil {
			return err
		}
		if err := c.membresias.Guardar(ctx, nueva); err != nil {
			return err
		}
		for _, e := range invitacion.EventosPendientes() {
			if err := c.auditoria.Registrar(ctx, e, idOrganizacion, cmd.Origen); err != nil {
				return err
			}
		}
		eventosMembresia = nueva.EventosPendientes()
		for _, e := range eventosMembresia {
			if err := c.auditoria.Registrar(ctx, e, idOrganizacion, cmd.Origen); err != nil {
				return err
			}
		}
		membresiaResultado = nueva
		return nil
	}); err != nil {
		return puertos.VistaMiembro{}, err
	}

	if len(eventosMembresia) > 0 {
		if errPub := c.eventos.Publicar(ctx, eventosMembresia...); errPub != nil {
			slog.WarnContext(ctx, "aceptar invitación: fallo al publicar eventos",
				"error", errPub, "organizacion_id", idOrganizacion.String())
		}
	}

	return vistaMiembroDesde(membresiaResultado), nil
}

// auditarInvitacionFallida registra InvitacionResuelta con
// Desenlace=DesenlaceIntentoFallido, Resultado=ResultadoFallo, fuera de
// cualquier UnidadDeTrabajo (no hay agregado que persistir: mismo patrón
// que auditarRenovacionRechazada en acceso/aplicacion). idInvitacion y
// idOrganizacion van vacíos cuando el token no resolvió a ninguna
// invitación (INV-TEN-24).
func (c *InvitacionesCasoDeUso) auditarInvitacionFallida(ctx context.Context, idInvitacion string, idOrganizacion dominio.IDOrganizacion, ahora time.Time, origen dominio.OrigenSolicitud) error {
	evento := dominio.NuevoInvitacionResuelta(idInvitacion, idOrganizacion.String(), dominio.DesenlaceIntentoFallido, dominio.ResultadoFallo, ahora)
	return c.auditoria.Registrar(ctx, evento, idOrganizacion, origen)
}
