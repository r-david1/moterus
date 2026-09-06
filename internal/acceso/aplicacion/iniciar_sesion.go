package aplicacion

import (
	"context"
	"log/slog"

	"github.com/r-david1/moterus/internal/acceso/dominio"
	"github.com/r-david1/moterus/internal/acceso/puertos"
)

// IniciarSesionCasoDeUso implementa puertos.IniciadorDeSesion: la
// orquestación completa del login (sección 3.1 del diseño, ADR 0009). No
// evalúa Confianza (ya lo hace Identidad dentro de AutenticarUsuario) y
// nunca verifica contraseñas por sí mismo (INV-ACC-02): delega en
// AutenticadorIdentidad, el ACL sobre identidad/puertos.AutenticadorDeCredenciales.
type IniciarSesionCasoDeUso struct {
	autenticador    puertos.AutenticadorIdentidad
	sesiones        puertos.RepositorioSesiones
	refrescos       puertos.GeneradorTokensRefresco
	firmador        puertos.FirmadorTokensAcceso
	listaRevocacion puertos.ListaRevocacion
	auditoria       puertos.RegistroAuditoria
	eventos         puertos.PublicadorEventos
	reloj           puertos.Reloj
	ids             puertos.GeneradorIDs
	uow             puertos.UnidadDeTrabajo
	politica        dominio.PoliticaSesion
	emisor          string
	audiencia       string
	emisorStepUp    puertos.EmisorTokenStepUp
}

var _ puertos.IniciadorDeSesion = (*IniciarSesionCasoDeUso)(nil)

// NuevoIniciarSesionCasoDeUso construye el caso de uso con sus
// dependencias inyectadas por puerto (nunca implementaciones concretas).
// emisor/audiencia son configuración (ACCESO_EMISOR/ACCESO_AUDIENCIA, §7
// del diseño): dominio.Sesion.ReclamacionesParaToken no puede leerlas por
// sí mismo (INV-ACC-18: nada de config ni de E/S en el dominio).
func NuevoIniciarSesionCasoDeUso(
	autenticador puertos.AutenticadorIdentidad,
	sesiones puertos.RepositorioSesiones,
	refrescos puertos.GeneradorTokensRefresco,
	firmador puertos.FirmadorTokensAcceso,
	listaRevocacion puertos.ListaRevocacion,
	auditoria puertos.RegistroAuditoria,
	eventos puertos.PublicadorEventos,
	reloj puertos.Reloj,
	ids puertos.GeneradorIDs,
	uow puertos.UnidadDeTrabajo,
	politica dominio.PoliticaSesion,
	emisor string,
	audiencia string,
	emisorStepUp puertos.EmisorTokenStepUp,
) *IniciarSesionCasoDeUso {
	return &IniciarSesionCasoDeUso{
		autenticador:    autenticador,
		sesiones:        sesiones,
		refrescos:       refrescos,
		firmador:        firmador,
		listaRevocacion: listaRevocacion,
		auditoria:       auditoria,
		eventos:         eventos,
		reloj:           reloj,
		ids:             ids,
		uow:             uow,
		politica:        politica,
		emisor:          emisor,
		audiencia:       audiencia,
		emisorStepUp:    emisorStepUp,
	}
}

// Iniciar ejecuta el flujo normativo de la sección 3.1 del diseño, en este
// orden (normativo, no cosmético — INV-ACC-24 en particular):
//  1. No se evalúa Confianza aquí: lo hace Identidad dentro de
//     AutenticarUsuario.
//  2. Delegar en AutenticadorIdentidad; sus errores (ya traducidos por el
//     ACL a tipos de acceso/dominio) se propagan tal cual, sin auditar en
//     Acceso: Identidad ya los auditó.
//  3. Si RequiereSegundoFactor, no se emite sesión (INV-ACC-03).
//  4. Construir el agregado Sesion y su primer refresco.
//  5. Persistir sesión + auditoría en la misma UnidadDeTrabajo (ADR 0005).
//  6. Firmar el token de acceso DESPUÉS de confirmar la transacción
//     (INV-ACC-24).
//  7. Fuera de la transacción, best-effort: publicar eventos y aplicar el
//     límite de sesiones concurrentes (INV-ACC-22).
func (c *IniciarSesionCasoDeUso) Iniciar(ctx context.Context, cmd puertos.ComandoIniciarSesion) (puertos.ResultadoSesion, error) {
	sujeto, err := c.autenticador.Autenticar(ctx, puertos.CredencialesSujeto{
		Correo:       cmd.Correo,
		Contrasena:   cmd.Contrasena,
		TokenCaptcha: cmd.TokenCaptcha,
		Origen:       cmd.Origen,
	})
	if err != nil {
		return puertos.ResultadoSesion{}, err
	}

	// INV-ACC-03: si Identidad exige segundo factor, no se emite sesión ni
	// token de acceso alguno. Tampoco se audita aquí: Identidad ya emitió
	// usuario.step_up_requerido. Antes de devolver el error, se emite el
	// token de step-up (ADR 0038, §3.5 del diseño otp-mfa.md) para que el
	// cliente pueda completar el login en
	// POST /acceso/sesiones/segundo-factor (CompletarSegundoFactorCasoDeUso)
	// sin tener que volver a presentar la contraseña.
	if sujeto.RequiereSegundoFactor {
		tokenStepUp, err := c.emisorStepUp.Emitir(ctx, sujeto.IDUsuario, sujeto.MotivoStepUp, c.reloj.Ahora())
		if err != nil {
			return puertos.ResultadoSesion{}, err
		}
		return puertos.ResultadoSesion{}, &dominio.ErrSegundoFactorRequerido{
			MotivoStepUp: sujeto.MotivoStepUp,
			TokenStepUp:  tokenStepUp.Compacto(),
		}
	}

	usuarioID, err := dominio.IDUsuarioDesde(sujeto.IDUsuario)
	if err != nil {
		return puertos.ResultadoSesion{}, err
	}

	return c.emitirSesionCompleta(ctx, usuarioID, cmd.Origen, metodosAutenticacionSoloContrasena)
}

// metodosAutenticacionSoloContrasena/metodosAutenticacionConSegundoFactor
// son los dos únicos valores que puede tomar el claim `amr` (RFC 8176) que
// este contexto emite (§3.5/§3.6 del diseño otp-mfa.md): ["pwd"] cuando el
// login se completó sin exigir segundo factor, ["pwd","otp"] cuando
// CompletarSegundoFactorCasoDeUso acaba de verificar el código OTP. Se
// pasan explícitamente a emitirSesionCompleta en lugar de dejar que
// Sesion.ReclamacionesParaToken decida (ese método sigue fijo en ["pwd"],
// dominio.metodosAutenticacionFase1, sin parámetro para variarlo): construir
// las reclamaciones aquí, vía dominio.NuevasReclamacionesAcceso (que si
// admite un slice de métodos), es la única forma de lograrlo sin tocar
// acceso/dominio (cerrado para este cambio).
var (
	metodosAutenticacionSoloContrasena   = []string{"pwd"}
	metodosAutenticacionConSegundoFactor = []string{"pwd", "otp"}
)

// emitirSesionCompleta es la lógica de emisión de sesión compartida entre
// IniciarSesion (cuando no hace falta segundo factor) y
// CompletarSegundoFactor (tras verificar el código OTP): construye el
// agregado Sesion, su primer refresco, persiste + audita en la misma
// UnidadDeTrabajo (ADR 0005), firma el token de acceso DESPUÉS del commit
// (INV-ACC-24) con el claim `amr` recibido, y aplica el límite de sesiones
// concurrentes (INV-ACC-22) fuera de la transacción.
func (c *IniciarSesionCasoDeUso) emitirSesionCompleta(ctx context.Context, usuarioID dominio.IDUsuario, origen dominio.OrigenSolicitud, amr []string) (puertos.ResultadoSesion, error) {
	idSesion, err := c.ids.NuevoIDSesion()
	if err != nil {
		return puertos.ResultadoSesion{}, err
	}

	ahora := c.reloj.Ahora()
	sesion, err := dominio.IniciarSesion(idSesion, usuarioID, origen, ahora, c.politica)
	if err != nil {
		return puertos.ResultadoSesion{}, err
	}

	refrescoPlano, err := c.refrescos.Generar()
	if err != nil {
		return puertos.ResultadoSesion{}, err
	}
	if err := sesion.EmitirPrimerRefresco(refrescoPlano.Hash(), ahora, c.politica); err != nil {
		return puertos.ResultadoSesion{}, err
	}

	eventosPendientes := sesion.EventosPendientes()
	if err := c.uow.Ejecutar(ctx, func(ctx context.Context) error {
		if err := c.sesiones.Guardar(ctx, sesion); err != nil {
			return err
		}
		for _, e := range eventosPendientes {
			if err := c.auditoria.Registrar(ctx, e, origen); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return puertos.ResultadoSesion{}, err
	}

	// INV-ACC-24: firmar SIEMPRE después del commit. Un token firmado sobre
	// una transacción que después falla sería una credencial válida
	// durante vidaTokenAcceso apuntando a un sid inexistente.
	jti, err := c.ids.NuevoIDTokenAcceso()
	if err != nil {
		return puertos.ResultadoSesion{}, err
	}
	reclamaciones, err := dominio.NuevasReclamacionesAcceso(
		c.emisor, usuarioID, c.audiencia, sesion.ID(), jti, amr,
		sesion.CreadaEn(), ahora, ahora.Add(c.politica.VidaTokenAcceso()), 1,
	)
	if err != nil {
		return puertos.ResultadoSesion{}, err
	}
	tokenAcceso, err := c.firmador.Firmar(ctx, reclamaciones)
	if err != nil {
		return puertos.ResultadoSesion{}, err
	}

	if errPub := c.eventos.Publicar(ctx, eventosPendientes...); errPub != nil {
		slog.WarnContext(ctx, "publicación de eventos de inicio de sesión falló",
			"error", errPub, "sesion_id", sesion.ID().String())
	}

	c.aplicarLimiteSesiones(ctx, usuarioID, origen)

	refrescoVigente, _ := sesion.RefrescoVigente()
	return puertos.ResultadoSesion{
		TokenAcceso:      tokenAcceso,
		ExpiraEnSegundos: int(c.politica.VidaTokenAcceso().Seconds()),
		TipoToken:        "Bearer",
		TokenRefresco:    refrescoPlano.Valor(),
		RefrescoExpiraEn: refrescoVigente.ExpiraEn(),
		IDSesion:         sesion.ID().String(),
		IDUsuario:        usuarioID.String(),
		SesionExpiraEn:   sesion.ExpiraAbsolutoEn(),
	}, nil
}

// aplicarLimiteSesiones implementa INV-ACC-22: si el usuario supera
// PoliticaSesion.MaximoSesionesActivas(), revoca la sesión activa más
// antigua (nunca rechaza el login nuevo, que permitiría a un atacante
// bloquear el acceso legítimo llenando el cupo de la víctima). Es
// best-effort y fuera de la transacción del login: un fallo aquí se
// registra en logs y no hace fallar Iniciar, cuya sesión ya fue
// confirmada.
func (c *IniciarSesionCasoDeUso) aplicarLimiteSesiones(ctx context.Context, usuarioID dominio.IDUsuario, origen dominio.OrigenSolicitud) {
	total, err := c.sesiones.ContarActivasDeUsuario(ctx, usuarioID)
	if err != nil {
		slog.WarnContext(ctx, "límite de sesiones: fallo al contar sesiones activas",
			"error", err, "usuario_id", usuarioID.String())
		return
	}
	if total <= c.politica.MaximoSesionesActivas() {
		return
	}

	activas, err := c.sesiones.ListarActivasDeUsuario(ctx, usuarioID)
	if err != nil {
		slog.WarnContext(ctx, "límite de sesiones: fallo al listar sesiones activas",
			"error", err, "usuario_id", usuarioID.String())
		return
	}
	masAntigua := sesionMasAntigua(activas)
	if masAntigua == nil {
		return
	}

	ahora := c.reloj.Ahora()
	if err := masAntigua.Revocar(dominio.MotivoLimiteSesionesExcedido, ahora); err != nil {
		slog.WarnContext(ctx, "límite de sesiones: fallo al revocar la sesión más antigua",
			"error", err, "usuario_id", usuarioID.String(), "sesion_id", masAntigua.ID().String())
		return
	}
	evento := dominio.NuevoSesionRevocada(masAntigua.ID(), usuarioID, dominio.MotivoLimiteSesionesExcedido, ahora)
	if err := c.uow.Ejecutar(ctx, func(ctx context.Context) error {
		if err := c.sesiones.Guardar(ctx, masAntigua); err != nil {
			return err
		}
		return c.auditoria.Registrar(ctx, evento, origen)
	}); err != nil {
		slog.WarnContext(ctx, "límite de sesiones: fallo al persistir la revocación",
			"error", err, "usuario_id", usuarioID.String(), "sesion_id", masAntigua.ID().String())
		return
	}

	if c.listaRevocacion.Disponible() {
		if err := c.listaRevocacion.RevocarSesion(ctx, masAntigua.ID(), hastaRevocacionListaAcceso(ahora, c.politica)); err != nil {
			slog.WarnContext(ctx, "límite de sesiones: fallo al propagar la revocación a la lista",
				"error", err, "sesion_id", masAntigua.ID().String())
		}
	}
}
