package aplicacion

import (
	"context"
	"log/slog"
	"time"

	"github.com/r-david1/moterus/internal/identidad/dominio"
	"github.com/r-david1/moterus/internal/identidad/puertos"
)

// VerificarCorreoCasoDeUso implementa puertos.VerificadorDeCorreo: consume
// un token de verificación de correo de un solo uso y transiciona el
// Usuario de pendiente_verificacion a activo (sección 3.4 del diseño,
// promovida al MVP el 2026-08-30). Es una única transacción de negocio:
// verificar un token y nada más.
type VerificarCorreoCasoDeUso struct {
	usuarios           puertos.RepositorioUsuarios
	tokensVerificacion puertos.RepositorioTokensVerificacion
	auditoria          puertos.RegistroAuditoria
	eventos            puertos.PublicadorEventos
	reloj              puertos.Reloj
	uow                puertos.UnidadDeTrabajo
}

var _ puertos.VerificadorDeCorreo = (*VerificarCorreoCasoDeUso)(nil)

// NuevoVerificarCorreoCasoDeUso construye el caso de uso con sus
// dependencias inyectadas por puerto.
func NuevoVerificarCorreoCasoDeUso(
	usuarios puertos.RepositorioUsuarios,
	tokensVerificacion puertos.RepositorioTokensVerificacion,
	auditoria puertos.RegistroAuditoria,
	eventos puertos.PublicadorEventos,
	reloj puertos.Reloj,
	uow puertos.UnidadDeTrabajo,
) *VerificarCorreoCasoDeUso {
	return &VerificarCorreoCasoDeUso{
		usuarios:           usuarios,
		tokensVerificacion: tokensVerificacion,
		auditoria:          auditoria,
		eventos:            eventos,
		reloj:              reloj,
		uow:                uow,
	}
}

// Verificar ejecuta el flujo normativo de la sección 3.4 del diseño: hashea
// el token recibido y lo busca por hash.
//
//   - No encontrado -> ErrTokenVerificacionInvalido, auditado como
//     usuario.correo_verificado/fallo (sin usuario_id: el token no se pudo
//     resolver a ningún usuario).
//   - Expirado -> ErrTokenVerificacionExpirado, mismo tratamiento de
//     auditoría, y además se elimina el token vencido dentro de la misma
//     transacción que su auditoría (es una mutación de negocio, aunque no
//     toque el agregado Usuario).
//   - Válido -> carga el Usuario, llama ConfirmarCorreo(ahora), guarda,
//     elimina el token y audita el éxito, todo en una UnidadDeTrabajo.
//
// El token en claro nunca se compara ni se persiste (INV-ID-21): solo su
// hash SHA-256 viaja a partir de aquí.
func (c *VerificarCorreoCasoDeUso) Verificar(ctx context.Context, cmd puertos.ComandoVerificarCorreo) error {
	ahora := c.reloj.Ahora()
	hashToken := hashTokenVerificacion(cmd.TokenPlano)

	usuarioID, expiraEn, encontrado, err := c.tokensVerificacion.BuscarPorHash(ctx, hashToken)
	if err != nil {
		return err
	}
	if !encontrado {
		if errAud := c.auditarFallo(ctx, "", "token_invalido", ahora, cmd.Origen); errAud != nil {
			return errAud
		}
		return &dominio.ErrTokenVerificacionInvalido{}
	}

	if ahora.After(expiraEn) {
		errUoW := c.uow.Ejecutar(ctx, func(ctx context.Context) error {
			if err := c.tokensVerificacion.Eliminar(ctx, usuarioID); err != nil {
				return err
			}
			evento := dominio.NuevoVerificacionCorreoFallida(usuarioID.String(), "token_expirado", ahora)
			return c.auditoria.Registrar(ctx, evento, cmd.Origen)
		})
		if errUoW != nil {
			return errUoW
		}
		return &dominio.ErrTokenVerificacionExpirado{}
	}

	usuario, err := c.usuarios.BuscarPorID(ctx, usuarioID)
	if err != nil {
		return err
	}
	if usuario == nil {
		// Token huérfano: no debería ocurrir en operación normal (el
		// usuario referenciado por un token activo siempre debería
		// existir), pero se trata como token inválido en vez de filtrar un
		// error interno de integridad referencial.
		if errAud := c.auditarFallo(ctx, usuarioID.String(), "usuario_no_encontrado", ahora, cmd.Origen); errAud != nil {
			return errAud
		}
		return &dominio.ErrTokenVerificacionInvalido{}
	}

	if errConfirmar := usuario.ConfirmarCorreo(ahora); errConfirmar != nil {
		// El usuario existe pero ya no está en pendiente_verificacion (p.
		// ej. token reenviado dos veces y ambos usados, o cuenta bloqueada
		// entre medias). Se propaga el error de dominio real
		// (ErrTransicionEstadoInvalida) tal cual, con su propia auditoría
		// de fallo.
		if errAud := c.auditarFallo(ctx, usuarioID.String(), "transicion_invalida", ahora, cmd.Origen); errAud != nil {
			return errAud
		}
		return errConfirmar
	}
	eventosPendientes := usuario.EventosPendientes()

	if err := c.uow.Ejecutar(ctx, func(ctx context.Context) error {
		if err := c.usuarios.Guardar(ctx, usuario); err != nil {
			return err
		}
		if err := c.tokensVerificacion.Eliminar(ctx, usuarioID); err != nil {
			return err
		}
		for _, e := range eventosPendientes {
			if err := c.auditoria.Registrar(ctx, e, cmd.Origen); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}

	if errPub := c.eventos.Publicar(ctx, eventosPendientes...); errPub != nil {
		slog.WarnContext(ctx, "publicación de eventos de verificación de correo falló",
			"error", errPub, "usuario_id", usuarioID.String())
	}

	return nil
}

// auditarFallo registra VerificacionCorreoFallida (accion
// usuario.correo_verificado, resultado fallo — mismo patrón que
// usuario.login, sin acción de catálogo nueva) fuera de cualquier
// transacción de negocio: cubre los casos en los que no hay ninguna
// mutación que abortar (token no encontrado, token huérfano).
func (c *VerificarCorreoCasoDeUso) auditarFallo(ctx context.Context, idUsuario, motivo string, ahora time.Time, origen dominio.OrigenSolicitud) error {
	evento := dominio.NuevoVerificacionCorreoFallida(idUsuario, motivo, ahora)
	return c.auditoria.Registrar(ctx, evento, origen)
}
