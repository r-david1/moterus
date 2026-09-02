package aplicacion

import (
	"context"
	"errors"
	"log/slog"

	"github.com/r-david1/moterus/internal/acceso/dominio"
	"github.com/r-david1/moterus/internal/acceso/puertos"
)

// ValidarAccesoCasoDeUso implementa puertos.ValidadorDeAccesos: el camino
// caliente del sistema (sección 3.3 del diseño), consumido por el
// middleware HTTP de CUALQUIER contexto. Su presupuesto es cero consultas
// a Postgres en el camino feliz: solo verifica la firma del JWT y,
// opcionalmente, consulta la lista de revocación en Redis.
type ValidarAccesoCasoDeUso struct {
	firmador        puertos.FirmadorTokensAcceso
	listaRevocacion puertos.ListaRevocacion
	sesiones        puertos.RepositorioSesiones
	auditoria       puertos.RegistroAuditoria
	reloj           puertos.Reloj
}

var _ puertos.ValidadorDeAccesos = (*ValidarAccesoCasoDeUso)(nil)

// NuevoValidarAccesoCasoDeUso construye el caso de uso con sus
// dependencias inyectadas por puerto.
func NuevoValidarAccesoCasoDeUso(
	firmador puertos.FirmadorTokensAcceso,
	listaRevocacion puertos.ListaRevocacion,
	sesiones puertos.RepositorioSesiones,
	auditoria puertos.RegistroAuditoria,
	reloj puertos.Reloj,
) *ValidarAccesoCasoDeUso {
	return &ValidarAccesoCasoDeUso{
		firmador:        firmador,
		listaRevocacion: listaRevocacion,
		sesiones:        sesiones,
		auditoria:       auditoria,
		reloj:           reloj,
	}
}

// Validar ejecuta el flujo normativo de la sección 3.3 del diseño:
//  1. Verificar la firma, estructura y ventanas temporales del JWT.
//  2. Un token expirado NO se audita (INV-ACC-17); cualquier otro rechazo
//     de validación sí, porque tiene valor de señal.
//  3. Consultar la lista de revocación en Redis si está disponible
//     (acelerador, no frontera de seguridad — INV-ACC-15).
//  4. Si ExigirSesionViva, verificar contra el repositorio de sesiones
//     para operaciones de alto valor.
func (c *ValidarAccesoCasoDeUso) Validar(ctx context.Context, cmd puertos.ComandoValidarAcceso) (puertos.Acceso, error) {
	reclamaciones, err := c.firmador.Verificar(ctx, cmd.TokenCompacto)
	if err != nil {
		var expirado *dominio.ErrTokenAccesoExpirado
		if errors.As(err, &expirado) {
			// INV-ACC-17: el rechazo más frecuente del sistema no se
			// audita; auditarlo pondría el hot path detrás del advisory
			// lock de la cadena de hashes.
			return puertos.Acceso{}, err
		}
		if errAud := c.auditarRechazo(ctx, "", motivoTokenInvalido(err), cmd.Origen); errAud != nil {
			return puertos.Acceso{}, errAud
		}
		return puertos.Acceso{}, err
	}

	sid := reclamaciones.IDSesion()

	if c.listaRevocacion.Disponible() {
		revocada, errLista := c.listaRevocacion.SesionRevocada(ctx, sid)
		if errLista != nil {
			// La lista es un acelerador, no la frontera de seguridad
			// (INV-ACC-15): un fallo de Redis no bloquea la validación, se
			// degrada al peor caso conocido (hasta vidaTokenAcceso).
			slog.WarnContext(ctx, "validar acceso: fallo al consultar la lista de revocación; continuando sin ella",
				"error", errLista, "sesion_id", sid.String())
		} else if revocada {
			if errAud := c.auditarRechazo(ctx, sid.String(), "sesion_revocada", cmd.Origen); errAud != nil {
				return puertos.Acceso{}, errAud
			}
			return puertos.Acceso{}, &dominio.ErrSesionRevocadaEnLista{}
		}
	}

	if cmd.ExigirSesionViva {
		sesion, errBuscar := c.sesiones.BuscarPorID(ctx, sid)
		if errBuscar != nil {
			return puertos.Acceso{}, errBuscar
		}
		if sesion == nil {
			return puertos.Acceso{}, &dominio.ErrSesionNoEncontrada{IDSesion: sid.String()}
		}
		ahora := c.reloj.Ahora()
		if !sesion.EstaViva(ahora) {
			return puertos.Acceso{}, sesion.PuedeRenovarse(ahora)
		}
	}

	return puertos.Acceso{
		IDUsuario:            reclamaciones.Sujeto().String(),
		IDSesion:             sid.String(),
		MetodosAutenticacion: reclamaciones.MetodosAutenticacion(),
		AutenticadoEn:        reclamaciones.AutenticadoEn(),
		TokenExpiraEn:        reclamaciones.ExpiraEn(),
	}, nil
}

func (c *ValidarAccesoCasoDeUso) auditarRechazo(ctx context.Context, idSesion, motivo string, origen dominio.OrigenSolicitud) error {
	evento := dominio.NuevoTokenAccesoRechazado(idSesion, motivo, c.reloj.Ahora())
	return c.auditoria.Registrar(ctx, evento, origen)
}

// motivoTokenInvalido extrae el motivo del catálogo cerrado de un
// *dominio.ErrTokenAccesoInvalido, o el motivo genérico de estructura
// corrupta si FirmadorTokensAcceso.Verificar devolvió otro tipo de error.
func motivoTokenInvalido(err error) string {
	var invalido *dominio.ErrTokenAccesoInvalido
	if errors.As(err, &invalido) {
		return invalido.Motivo
	}
	return dominio.MotivoTokenAccesoEstructuraCorrupta
}
