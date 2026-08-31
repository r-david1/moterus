package aplicacion

import (
	"context"
	"errors"
	"log/slog"

	"github.com/r-david1/moterus/internal/identidad/dominio"
	"github.com/r-david1/moterus/internal/identidad/puertos"
)

// ReenviarVerificacionCasoDeUso implementa puertos.ReenviadorDeVerificacion
// (sección 3.4 del diseño). INV-ID-22: la respuesta es siempre neutra — no
// revela si el correo existe, si ya está verificado, ni en qué estado se
// encuentra la cuenta. Por eso Reenviar nunca devuelve un error de negocio:
// cualquier motivo por el que no se envía un correo nuevo (cuenta
// inexistente, ya activa, suspendida/bloqueada, o incluso un fallo interno
// al generar/guardar/enviar el token) se trata igual que el camino feliz
// desde el punto de vista del llamador — se registra en logs de aplicación,
// nunca como un resultado distinguible.
type ReenviarVerificacionCasoDeUso struct {
	usuarios           puertos.RepositorioUsuarios
	generadorTokens    puertos.GeneradorTokens
	tokensVerificacion puertos.RepositorioTokensVerificacion
	notificadorCorreo  puertos.NotificadorCorreo
	reloj              puertos.Reloj
}

var _ puertos.ReenviadorDeVerificacion = (*ReenviarVerificacionCasoDeUso)(nil)

// NuevoReenviarVerificacionCasoDeUso construye el caso de uso con sus
// dependencias inyectadas por puerto.
func NuevoReenviarVerificacionCasoDeUso(
	usuarios puertos.RepositorioUsuarios,
	generadorTokens puertos.GeneradorTokens,
	tokensVerificacion puertos.RepositorioTokensVerificacion,
	notificadorCorreo puertos.NotificadorCorreo,
	reloj puertos.Reloj,
) *ReenviarVerificacionCasoDeUso {
	return &ReenviarVerificacionCasoDeUso{
		usuarios:           usuarios,
		generadorTokens:    generadorTokens,
		tokensVerificacion: tokensVerificacion,
		notificadorCorreo:  notificadorCorreo,
		reloj:              reloj,
	}
}

// Reenviar ejecuta el flujo de la sección 3.4 del diseño: si el usuario
// existe y sigue en pendiente_verificacion, genera un token nuevo (invalida
// el anterior mediante upsert de RepositorioTokensVerificacion.Guardar) y lo
// envía; en cualquier otro caso no hace nada observable desde afuera.
//
// Devuelve siempre nil (INV-ID-22): ni la ausencia de la cuenta, ni un
// estado no elegible, ni un fallo interno se traducen en un resultado
// distinguible desde el puerto de entrada. No se audita este caso de uso:
// el catálogo cerrado de acciones (docs/catalogos/acciones-auditoria.md) no
// tiene una acción "reenvío de verificación" y no corresponde inventar una
// aquí (INV-ID-17) — si se quiere auditar este flujo, hace falta una
// migración que añada la acción al catálogo primero.
func (c *ReenviarVerificacionCasoDeUso) Reenviar(ctx context.Context, cmd puertos.ComandoReenviarVerificacion) error {
	correo, err := dominio.NuevoCorreo(cmd.Correo)
	if err != nil {
		// Un correo malformado tampoco debe distinguirse de uno
		// inexistente (mismo criterio anti-enumeración que INV-ID-11).
		return nil
	}

	usuario, err := c.usuarios.BuscarPorCorreo(ctx, correo)
	if err != nil {
		var noEncontrado *dominio.ErrUsuarioNoEncontrado
		if !errors.As(err, &noEncontrado) {
			slog.WarnContext(ctx, "reenvío de verificación: fallo al buscar el usuario; respuesta neutra igual",
				"error", err)
		}
		return nil
	}
	if usuario == nil {
		return nil
	}
	if !usuario.Estado().EsIgual(dominio.EstadoPendienteVerificacion) {
		return nil
	}

	tokenPlano, err := c.generadorTokens.Generar()
	if err != nil {
		slog.WarnContext(ctx, "reenvío de verificación: fallo al generar el token; respuesta neutra igual",
			"error", err, "usuario_id", usuario.ID().String())
		return nil
	}
	hashToken := hashTokenVerificacion(tokenPlano)
	expiraEn := c.reloj.Ahora().Add(vigenciaTokenVerificacionCorreo)

	// RepositorioTokensVerificacion.Guardar hace upsert por usuarioID: un
	// reenvío invalida el token anterior sin necesidad de un paso de
	// borrado previo (nota para go-infraestructura: el adaptador Postgres
	// debe implementar esto como INSERT ... ON CONFLICT (usuario_id) DO
	// UPDATE, no como dos pasos separados).
	if err := c.tokensVerificacion.Guardar(ctx, usuario.ID(), hashToken, expiraEn); err != nil {
		slog.WarnContext(ctx, "reenvío de verificación: fallo al guardar el token nuevo; respuesta neutra igual",
			"error", err, "usuario_id", usuario.ID().String())
		return nil
	}

	if err := c.notificadorCorreo.EnviarVerificacion(ctx, correo, tokenPlano); err != nil {
		slog.WarnContext(ctx, "reenvío de verificación: fallo al enviar el correo; respuesta neutra igual",
			"error", err, "usuario_id", usuario.ID().String())
	}

	return nil
}
