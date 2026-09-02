package aplicacion

import (
	"context"
	"errors"
	"log/slog"

	"github.com/r-david1/moterus/internal/acceso/dominio"
	"github.com/r-david1/moterus/internal/acceso/puertos"
)

// RenovarSesionCasoDeUso implementa puertos.RenovadorDeSesion: la
// rotación del token de refresco con detección de reuso (sección 3.2 del
// diseño). A diferencia de IniciarSesion, aquí SÍ se evalúa Confianza:
// Identidad no participa en este flujo y un endpoint de renovación sin
// límite es un oráculo de fuerza bruta sobre tokens de refresco (§0 del
// diseño).
type RenovarSesionCasoDeUso struct {
	confianza       puertos.EvaluadorConfianza
	sesiones        puertos.RepositorioSesiones
	refrescos       puertos.GeneradorTokensRefresco
	firmador        puertos.FirmadorTokensAcceso
	consultorEstado puertos.ConsultorEstadoSujeto
	listaRevocacion puertos.ListaRevocacion
	auditoria       puertos.RegistroAuditoria
	reloj           puertos.Reloj
	ids             puertos.GeneradorIDs
	uow             puertos.UnidadDeTrabajo
	politica        dominio.PoliticaSesion
	emisor          string
	audiencia       string
}

var _ puertos.RenovadorDeSesion = (*RenovarSesionCasoDeUso)(nil)

// NuevoRenovarSesionCasoDeUso construye el caso de uso con sus
// dependencias inyectadas por puerto.
func NuevoRenovarSesionCasoDeUso(
	confianza puertos.EvaluadorConfianza,
	sesiones puertos.RepositorioSesiones,
	refrescos puertos.GeneradorTokensRefresco,
	firmador puertos.FirmadorTokensAcceso,
	consultorEstado puertos.ConsultorEstadoSujeto,
	listaRevocacion puertos.ListaRevocacion,
	auditoria puertos.RegistroAuditoria,
	reloj puertos.Reloj,
	ids puertos.GeneradorIDs,
	uow puertos.UnidadDeTrabajo,
	politica dominio.PoliticaSesion,
	emisor string,
	audiencia string,
) *RenovarSesionCasoDeUso {
	return &RenovarSesionCasoDeUso{
		confianza:       confianza,
		sesiones:        sesiones,
		refrescos:       refrescos,
		firmador:        firmador,
		consultorEstado: consultorEstado,
		listaRevocacion: listaRevocacion,
		auditoria:       auditoria,
		reloj:           reloj,
		ids:             ids,
		uow:             uow,
		politica:        politica,
		emisor:          emisor,
		audiencia:       audiencia,
	}
}

// Renovar ejecuta el flujo normativo de la sección 3.2 del diseño:
//  1. Evaluar Confianza (accion "renovacion_sesion"); si deniega, no se
//     audita en la cadena (mismo criterio que el guardián de perímetro de
//     ADR 0018).
//  2. Validar la forma del token y hashearlo.
//  3. Resolver el hash: desconocido -> ErrRefrescoInvalido; consumido ->
//     REUSO (revoca toda la sesión, INV-ACC-06); vigente -> continúa.
//  4. Comprobar las ventanas de la sesión (PuedeRenovarse).
//  5. Revalidar el estado del sujeto contra Identidad.
//  6. Rotar el token de refresco.
//  7. Persistir + auditar en la misma UnidadDeTrabajo.
//  8. Firmar el nuevo token de acceso DESPUÉS del commit (INV-ACC-24).
func (c *RenovarSesionCasoDeUso) Renovar(ctx context.Context, cmd puertos.ComandoRenovarSesion) (puertos.ResultadoSesion, error) {
	decision, err := c.confianza.Evaluar(ctx, puertos.SolicitudEvaluacion{
		Accion:      "renovacion_sesion",
		ClaveCuenta: claveCuentaPorIP(cmd.Origen),
		Origen:      cmd.Origen,
	})
	if err != nil {
		return puertos.ResultadoSesion{}, err
	}
	if !decision.Permitido {
		return puertos.ResultadoSesion{}, &dominio.ErrAccesoDenegadoPorConfianza{Motivo: decision.Motivo, ReintentarEn: decision.ReintentarEn}
	}

	presentado, err := dominio.NuevoTokenRefrescoPlano(cmd.TokenRefresco)
	if err != nil {
		if errAud := c.auditarRenovacionRechazada(ctx, "", "refresco_malformado", cmd.Origen); errAud != nil {
			return puertos.ResultadoSesion{}, errAud
		}
		return puertos.ResultadoSesion{}, &dominio.ErrRefrescoInvalido{}
	}
	hash := presentado.Hash()

	sesion, situacion, generacionToken, err := c.sesiones.BuscarPorHashRefresco(ctx, hash)
	if err != nil {
		return puertos.ResultadoSesion{}, err
	}

	switch situacion {
	case puertos.RefrescoDesconocido:
		if errAud := c.auditarRenovacionRechazada(ctx, "", "refresco_desconocido", cmd.Origen); errAud != nil {
			return puertos.ResultadoSesion{}, errAud
		}
		return puertos.ResultadoSesion{}, &dominio.ErrRefrescoInvalido{}

	case puertos.RefrescoConsumido:
		return puertos.ResultadoSesion{}, c.manejarReuso(ctx, sesion, generacionToken, cmd.Origen)
	}

	// RefrescoVigente: sigue el flujo normal.
	if errPuede := sesion.PuedeRenovarse(c.reloj.Ahora()); errPuede != nil {
		return puertos.ResultadoSesion{}, c.manejarRenovacionNoPermitida(ctx, sesion, errPuede, cmd.Origen)
	}

	estado, err := c.consultorEstado.EstadoDe(ctx, sesion.UsuarioID().String())
	if err != nil {
		return puertos.ResultadoSesion{}, err
	}
	if !estado.Existe || estado.Estado != "activo" {
		return puertos.ResultadoSesion{}, c.revocarPorCuentaNoOperativa(ctx, sesion, cmd.Origen)
	}

	refrescoPlano, err := c.refrescos.Generar()
	if err != nil {
		return puertos.ResultadoSesion{}, err
	}
	ahora := c.reloj.Ahora()
	if err := sesion.Rotar(refrescoPlano.Hash(), ahora, c.politica); err != nil {
		return puertos.ResultadoSesion{}, err
	}

	eventosPendientes := sesion.EventosPendientes()
	if err := c.uow.Ejecutar(ctx, func(ctx context.Context) error {
		if err := c.sesiones.Guardar(ctx, sesion); err != nil {
			return err
		}
		for _, e := range eventosPendientes {
			if err := c.auditoria.Registrar(ctx, e, cmd.Origen); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return puertos.ResultadoSesion{}, err
	}

	// INV-ACC-24: firmar después del commit. El token de acceso anterior
	// no se revoca: le quedan a lo sumo vidaTokenAcceso y su existencia no
	// implica compromiso.
	jti, err := c.ids.NuevoIDTokenAcceso()
	if err != nil {
		return puertos.ResultadoSesion{}, err
	}
	reclamaciones, err := sesion.ReclamacionesParaToken(jti, c.emisor, c.audiencia, ahora, c.politica)
	if err != nil {
		return puertos.ResultadoSesion{}, err
	}
	tokenAcceso, err := c.firmador.Firmar(ctx, reclamaciones)
	if err != nil {
		return puertos.ResultadoSesion{}, err
	}

	refrescoVigente, _ := sesion.RefrescoVigente()
	return puertos.ResultadoSesion{
		TokenAcceso:      tokenAcceso,
		ExpiraEnSegundos: int(c.politica.VidaTokenAcceso().Seconds()),
		TipoToken:        "Bearer",
		TokenRefresco:    refrescoPlano.Valor(),
		RefrescoExpiraEn: refrescoVigente.ExpiraEn(),
		IDSesion:         sesion.ID().String(),
		IDUsuario:        sesion.UsuarioID().String(),
		SesionExpiraEn:   sesion.ExpiraAbsolutoEn(),
	}, nil
}

// manejarReuso implementa INV-ACC-06: presentar un refresco ya consumido
// revoca toda la sesión de inmediato (si todavía estaba activa) y se
// audita como ReusoRefrescoDetectado. El cliente recibe el mismo error
// que ante un token desconocido (INV-ACC-21).
func (c *RenovarSesionCasoDeUso) manejarReuso(ctx context.Context, sesion *dominio.Sesion, generacionPresentada int, origen dominio.OrigenSolicitud) error {
	ahora := c.reloj.Ahora()
	evento := dominio.NuevoReusoRefrescoDetectado(sesion.ID(), sesion.UsuarioID(), generacionPresentada, sesion.Generacion(), ahora)

	if sesion.Estado().EsIgual(dominio.EstadoSesionActiva) {
		if err := sesion.Revocar(dominio.MotivoReusoRefrescoDetectado, ahora); err != nil {
			return err
		}
		if err := c.uow.Ejecutar(ctx, func(ctx context.Context) error {
			if err := c.sesiones.Guardar(ctx, sesion); err != nil {
				return err
			}
			return c.auditoria.Registrar(ctx, evento, origen)
		}); err != nil {
			return err
		}
		if c.listaRevocacion.Disponible() {
			if err := c.listaRevocacion.RevocarSesion(ctx, sesion.ID(), hastaRevocacionListaAcceso(ahora, c.politica)); err != nil {
				slog.WarnContext(ctx, "reuso de refresco: fallo al propagar la revocación a la lista",
					"error", err, "sesion_id", sesion.ID().String())
			}
		}
	} else {
		// La sesión ya no estaba activa (revocada o expirada antes de este
		// intento): no hay transición que aplicar, pero el intento de
		// reuso sigue siendo señal y se audita igual.
		if err := c.auditoria.Registrar(ctx, evento, origen); err != nil {
			return err
		}
	}

	return &dominio.ErrRefrescoInvalido{}
}

// manejarRenovacionNoPermitida traduce el resultado de Sesion.PuedeRenovarse
// (§1.4 del diseño, servicio EvaluadorVentanas) a la acción de auditoría y
// al error de dominio correspondientes.
func (c *RenovarSesionCasoDeUso) manejarRenovacionNoPermitida(ctx context.Context, sesion *dominio.Sesion, errPuede error, origen dominio.OrigenSolicitud) error {
	var expirada *dominio.ErrSesionExpirada
	if errors.As(errPuede, &expirada) {
		ahora := c.reloj.Ahora()
		if err := sesion.MarcarExpirada(ahora); err != nil {
			return err
		}
		evento := dominio.NuevoRenovacionRechazada(sesion.ID().String(), "sesion_expirada", ahora)
		if err := c.uow.Ejecutar(ctx, func(ctx context.Context) error {
			if err := c.sesiones.Guardar(ctx, sesion); err != nil {
				return err
			}
			return c.auditoria.Registrar(ctx, evento, origen)
		}); err != nil {
			return err
		}
		return errPuede
	}

	// ErrSesionRevocada (u otro caso no contemplado): la sesión ya está en
	// un estado terminal, no hay nada que persistir de nuevo. Solo se
	// audita el intento.
	if errAud := c.auditarRenovacionRechazada(ctx, sesion.ID().String(), "sesion_revocada", origen); errAud != nil {
		return errAud
	}
	return errPuede
}

// revocarPorCuentaNoOperativa implementa el paso 5 de la sección 3.2 del
// diseño: la revalidación síncrona del estado del sujeto contra Identidad
// es el mecanismo por el que una suspensión/bloqueo surte efecto sin
// broker de eventos (§11.1, ADR candidato 0022).
func (c *RenovarSesionCasoDeUso) revocarPorCuentaNoOperativa(ctx context.Context, sesion *dominio.Sesion, origen dominio.OrigenSolicitud) error {
	ahora := c.reloj.Ahora()
	if err := sesion.Revocar(dominio.MotivoCuentaNoOperativa, ahora); err != nil {
		return err
	}
	evento := dominio.NuevoSesionRevocada(sesion.ID(), sesion.UsuarioID(), dominio.MotivoCuentaNoOperativa, ahora)
	if err := c.uow.Ejecutar(ctx, func(ctx context.Context) error {
		if err := c.sesiones.Guardar(ctx, sesion); err != nil {
			return err
		}
		return c.auditoria.Registrar(ctx, evento, origen)
	}); err != nil {
		return err
	}
	if c.listaRevocacion.Disponible() {
		if err := c.listaRevocacion.RevocarSesion(ctx, sesion.ID(), hastaRevocacionListaAcceso(ahora, c.politica)); err != nil {
			slog.WarnContext(ctx, "cuenta no operativa: fallo al propagar la revocación a la lista",
				"error", err, "sesion_id", sesion.ID().String())
		}
	}
	return &dominio.ErrSesionRevocada{Motivo: dominio.MotivoCuentaNoOperativa.Valor()}
}

// auditarRenovacionRechazada registra RenovacionRechazada fuera de
// cualquier mutación de negocio (no hay sesión que persistir cuando el
// refresco es desconocido o malformado).
func (c *RenovarSesionCasoDeUso) auditarRenovacionRechazada(ctx context.Context, idSesion, motivo string, origen dominio.OrigenSolicitud) error {
	evento := dominio.NuevoRenovacionRechazada(idSesion, motivo, c.reloj.Ahora())
	return c.auditoria.Registrar(ctx, evento, origen)
}
