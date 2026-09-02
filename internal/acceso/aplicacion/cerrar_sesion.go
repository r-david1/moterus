package aplicacion

import (
	"context"
	"log/slog"

	"github.com/r-david1/moterus/internal/acceso/dominio"
	"github.com/r-david1/moterus/internal/acceso/puertos"
)

// CerrarSesionCasoDeUso implementa puertos.CerradorDeSesiones: el logout
// individual (Cerrar) y el logout de todos los dispositivos (CerrarTodas),
// sección 3.4 del diseño. Ambos son iniciativa del propio usuario
// (dominio.MotivoCierreUsuario / MotivoCierreMasivoUsuario), a diferencia
// de RevocarSesionesUsuarioCasoDeUso.
type CerrarSesionCasoDeUso struct {
	sesiones        puertos.RepositorioSesiones
	confianza       puertos.EvaluadorConfianza
	listaRevocacion puertos.ListaRevocacion
	auditoria       puertos.RegistroAuditoria
	reloj           puertos.Reloj
	uow             puertos.UnidadDeTrabajo
	politica        dominio.PoliticaSesion
}

var _ puertos.CerradorDeSesiones = (*CerrarSesionCasoDeUso)(nil)

// NuevoCerrarSesionCasoDeUso construye el caso de uso con sus
// dependencias inyectadas por puerto.
func NuevoCerrarSesionCasoDeUso(
	sesiones puertos.RepositorioSesiones,
	confianza puertos.EvaluadorConfianza,
	listaRevocacion puertos.ListaRevocacion,
	auditoria puertos.RegistroAuditoria,
	reloj puertos.Reloj,
	uow puertos.UnidadDeTrabajo,
	politica dominio.PoliticaSesion,
) *CerrarSesionCasoDeUso {
	return &CerrarSesionCasoDeUso{
		sesiones:        sesiones,
		confianza:       confianza,
		listaRevocacion: listaRevocacion,
		auditoria:       auditoria,
		reloj:           reloj,
		uow:             uow,
		politica:        politica,
	}
}

// Cerrar implementa el logout individual (§3.4 del diseño). Resuelve el
// IDSesion tal como lo entregó el adaptador (INV-ACC-23: nunca un valor
// del cuerpo sin validar), verifica que pertenezca a cmd.IDUsuario
// (ErrSesionAjena -> 404, no 403: no confirma la existencia del ID ajeno)
// y revoca. Es idempotente: cerrar una sesión ya revocada o expirada
// devuelve nil sin auditar de nuevo (no hubo transición de estado, no hay
// hecho nuevo que registrar).
func (c *CerrarSesionCasoDeUso) Cerrar(ctx context.Context, cmd puertos.ComandoCerrarSesion) error {
	idSesion, err := dominio.IDSesionDesde(cmd.IDSesion)
	if err != nil {
		return err
	}
	idUsuario, err := dominio.IDUsuarioDesde(cmd.IDUsuario)
	if err != nil {
		return err
	}

	sesion, err := c.sesiones.BuscarPorID(ctx, idSesion)
	if err != nil {
		return err
	}
	if sesion == nil {
		return &dominio.ErrSesionNoEncontrada{IDSesion: cmd.IDSesion}
	}
	if !sesion.UsuarioID().EsIgual(idUsuario) {
		return &dominio.ErrSesionAjena{}
	}

	if !sesion.Estado().EsIgual(dominio.EstadoSesionActiva) {
		return nil
	}

	ahora := c.reloj.Ahora()
	if err := sesion.Revocar(dominio.MotivoCierreUsuario, ahora); err != nil {
		return err
	}
	evento := dominio.NuevoSesionCerrada(sesion.ID(), idUsuario, dominio.AlcanceCierreIndividual, 1, ahora)
	if err := c.uow.Ejecutar(ctx, func(ctx context.Context) error {
		if err := c.sesiones.Guardar(ctx, sesion); err != nil {
			return err
		}
		return c.auditoria.Registrar(ctx, evento, cmd.Origen)
	}); err != nil {
		return err
	}

	if c.listaRevocacion.Disponible() {
		if err := c.listaRevocacion.RevocarSesion(ctx, sesion.ID(), hastaRevocacionListaAcceso(ahora, c.politica)); err != nil {
			slog.WarnContext(ctx, "cerrar sesión: fallo al propagar la revocación a la lista",
				"error", err, "sesion_id", sesion.ID().String())
		}
	}
	return nil
}

// CerrarTodas implementa el logout de todos los dispositivos (§3.4 del
// diseño). Evalúa Confianza porque es una operación destructiva que un
// atacante con un token robado podría usar para molestar a la víctima; si
// se deniega, no se audita (mismo criterio que la denegación de
// RenovarSesion). Audita una fila SesionCerrada por sesión revocada, nunca
// una fila agregada: la bitácora forense necesita poder responder "¿cuándo
// murió esta sesión concreta?".
func (c *CerrarSesionCasoDeUso) CerrarTodas(ctx context.Context, cmd puertos.ComandoCerrarTodasLasSesiones) (puertos.ResultadoCierreMasivo, error) {
	idUsuario, err := dominio.IDUsuarioDesde(cmd.IDUsuario)
	if err != nil {
		return puertos.ResultadoCierreMasivo{}, err
	}

	decision, err := c.confianza.Evaluar(ctx, puertos.SolicitudEvaluacion{
		Accion:      "cierre_masivo_sesiones",
		ClaveCuenta: claveCuentaPorUsuario(idUsuario),
		Origen:      cmd.Origen,
	})
	if err != nil {
		return puertos.ResultadoCierreMasivo{}, err
	}
	if !decision.Permitido {
		return puertos.ResultadoCierreMasivo{}, &dominio.ErrAccesoDenegadoPorConfianza{Motivo: decision.Motivo, ReintentarEn: decision.ReintentarEn}
	}

	var excepto dominio.IDSesion
	if cmd.IDSesionAPreservar != "" {
		excepto, err = dominio.IDSesionDesde(cmd.IDSesionAPreservar)
		if err != nil {
			return puertos.ResultadoCierreMasivo{}, err
		}
	}

	ahora := c.reloj.Ahora()
	var idsRevocados []dominio.IDSesion
	if err := c.uow.Ejecutar(ctx, func(ctx context.Context) error {
		var errRevocar error
		idsRevocados, errRevocar = c.sesiones.RevocarActivasDeUsuario(ctx, idUsuario, excepto, dominio.MotivoCierreMasivoUsuario, ahora)
		if errRevocar != nil {
			return errRevocar
		}
		for _, id := range idsRevocados {
			evento := dominio.NuevoSesionCerrada(id, idUsuario, dominio.AlcanceCierreTodas, len(idsRevocados), ahora)
			if err := c.auditoria.Registrar(ctx, evento, cmd.Origen); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return puertos.ResultadoCierreMasivo{}, err
	}

	if c.listaRevocacion.Disponible() {
		hasta := hastaRevocacionListaAcceso(ahora, c.politica)
		for _, id := range idsRevocados {
			if err := c.listaRevocacion.RevocarSesion(ctx, id, hasta); err != nil {
				slog.WarnContext(ctx, "cerrar todas las sesiones: fallo al propagar la revocación a la lista",
					"error", err, "sesion_id", id.String())
			}
		}
	}

	return puertos.ResultadoCierreMasivo{SesionesRevocadas: len(idsRevocados)}, nil
}
