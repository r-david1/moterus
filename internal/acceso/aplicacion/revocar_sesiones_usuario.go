package aplicacion

import (
	"context"
	"log/slog"

	"github.com/r-david1/moterus/internal/acceso/dominio"
	"github.com/r-david1/moterus/internal/acceso/puertos"
)

// RevocarSesionesUsuarioCasoDeUso implementa puertos.RevocadorDeSesiones
// (sección 3.6 del diseño): el puerto que Acceso EXPONE a otros contextos
// (hoy: nadie; mañana Identidad, al suspender una cuenta o cambiar la
// contraseña — ADR candidato 0022). A diferencia de CerrarTodas, el motivo
// no es iniciativa del propio usuario: se audita como SesionRevocada, no
// SesionCerrada (la distinción entre "el usuario se fue" y "al usuario lo
// echaron" es justo el tipo de dato que un auditor va a pedir), y no
// evalúa Confianza: quien invoca este puerto ya es un contexto interno de
// confianza, no un cliente externo.
type RevocarSesionesUsuarioCasoDeUso struct {
	sesiones        puertos.RepositorioSesiones
	listaRevocacion puertos.ListaRevocacion
	auditoria       puertos.RegistroAuditoria
	reloj           puertos.Reloj
	uow             puertos.UnidadDeTrabajo
	politica        dominio.PoliticaSesion
}

var _ puertos.RevocadorDeSesiones = (*RevocarSesionesUsuarioCasoDeUso)(nil)

// NuevoRevocarSesionesUsuarioCasoDeUso construye el caso de uso con sus
// dependencias inyectadas por puerto.
func NuevoRevocarSesionesUsuarioCasoDeUso(
	sesiones puertos.RepositorioSesiones,
	listaRevocacion puertos.ListaRevocacion,
	auditoria puertos.RegistroAuditoria,
	reloj puertos.Reloj,
	uow puertos.UnidadDeTrabajo,
	politica dominio.PoliticaSesion,
) *RevocarSesionesUsuarioCasoDeUso {
	return &RevocarSesionesUsuarioCasoDeUso{
		sesiones:        sesiones,
		listaRevocacion: listaRevocacion,
		auditoria:       auditoria,
		reloj:           reloj,
		uow:             uow,
		politica:        politica,
	}
}

// RevocarPorUsuario revoca todas las sesiones activas de un usuario con el
// motivo indicado (catálogo cerrado), auditando una fila SesionRevocada
// por sesión.
func (c *RevocarSesionesUsuarioCasoDeUso) RevocarPorUsuario(ctx context.Context, cmd puertos.ComandoRevocarSesionesDeUsuario) (puertos.ResultadoCierreMasivo, error) {
	idUsuario, err := dominio.IDUsuarioDesde(cmd.IDUsuario)
	if err != nil {
		return puertos.ResultadoCierreMasivo{}, err
	}
	motivo, err := dominio.MotivoRevocacionDesde(cmd.Motivo)
	if err != nil {
		return puertos.ResultadoCierreMasivo{}, err
	}

	ahora := c.reloj.Ahora()
	var idsRevocados []dominio.IDSesion
	if err := c.uow.Ejecutar(ctx, func(ctx context.Context) error {
		var errRevocar error
		idsRevocados, errRevocar = c.sesiones.RevocarActivasDeUsuario(ctx, idUsuario, dominio.IDSesion{}, motivo, ahora)
		if errRevocar != nil {
			return errRevocar
		}
		for _, id := range idsRevocados {
			evento := dominio.NuevoSesionRevocada(id, idUsuario, motivo, ahora)
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
				slog.WarnContext(ctx, "revocar sesiones de usuario: fallo al propagar la revocación a la lista",
					"error", err, "sesion_id", id.String())
			}
		}
	}

	return puertos.ResultadoCierreMasivo{SesionesRevocadas: len(idsRevocados)}, nil
}
