package aplicacion

import (
	"context"
	"log/slog"

	"github.com/r-david1/moterus/internal/identidad/dominio"
	"github.com/r-david1/moterus/internal/identidad/puertos"
)

// DeshabilitarMFACasoDeUso implementa puertos.GestorDeMFA.Deshabilitar
// (§3.3 del diseño otp-mfa.md, ADR 0039): exige un código propio del factor
// (TOTP o de respaldo) antes de desarmar el segundo factor de la cuenta —
// no basta con estar autenticado con un token de acceso normal
// (INV-MFA-05).
type DeshabilitarMFACasoDeUso struct {
	usuarios    puertos.RepositorioUsuarios
	factoresMFA puertos.RepositorioFactoresMFA
	cifrador    puertos.CifradorSecretos
	auditoria   puertos.RegistroAuditoria
	eventos     puertos.PublicadorEventos
	reloj       puertos.Reloj
	uow         puertos.UnidadDeTrabajo
}

// NuevoDeshabilitarMFACasoDeUso construye el caso de uso con sus
// dependencias inyectadas por puerto.
func NuevoDeshabilitarMFACasoDeUso(
	usuarios puertos.RepositorioUsuarios,
	factoresMFA puertos.RepositorioFactoresMFA,
	cifrador puertos.CifradorSecretos,
	auditoria puertos.RegistroAuditoria,
	eventos puertos.PublicadorEventos,
	reloj puertos.Reloj,
	uow puertos.UnidadDeTrabajo,
) *DeshabilitarMFACasoDeUso {
	return &DeshabilitarMFACasoDeUso{
		usuarios:    usuarios,
		factoresMFA: factoresMFA,
		cifrador:    cifrador,
		auditoria:   auditoria,
		eventos:     eventos,
		reloj:       reloj,
		uow:         uow,
	}
}

// Deshabilitar ejecuta el flujo normativo de la sección 3.3 del diseño:
//  1. Cargar el/los factor(es) confirmado(s) del sujeto (el MVP admite como
//     mucho uno, ADR 0037).
//  2. Si no hay ninguno, o el código presentado no coincide con ninguno
//     (TOTP o de respaldo), ErrCodigoOTPInvalido — un único mensaje,
//     INV-MFA-08: no se distingue "sin MFA" de "código incorrecto".
//  3. Si coincide: FactorMFA.Deshabilitar (acumula FactorMFADeshabilitado)
//     y flip coordinado Usuario.DeshabilitarMFA, persistidos junto con la
//     auditoría en la misma UnidadDeTrabajo.
func (c *DeshabilitarMFACasoDeUso) Deshabilitar(ctx context.Context, cmd puertos.ComandoDeshabilitarMFA) error {
	idSujeto, err := dominio.IDUsuarioDesde(cmd.IDSujeto)
	if err != nil {
		return err
	}

	factores, err := c.factoresMFA.BuscarConfirmadosDeUsuario(ctx, idSujeto)
	if err != nil {
		return err
	}
	if len(factores) == 0 {
		// No es una inconsistencia interna (a diferencia de VerificarOTP):
		// un usuario sin MFA que intenta deshabilitarlo es un caso legítimo
		// y esperado, no un bug. Mismo error genérico, sin slog.
		return &dominio.ErrCodigoOTPInvalido{}
	}

	ahora := c.reloj.Ahora()
	var factorElegido *dominio.FactorMFA
	var eventosVerificacion []dominio.EventoDominio
	for _, f := range factores {
		secretoDescifrado, err := c.cifrador.Descifrar(f.SecretoCifrado())
		if err != nil {
			return err
		}
		if f.VerificarCodigo(cmd.Codigo, secretoDescifrado.Valor(), ahora) {
			factorElegido = f
			break
		}
		eventosVerificacion = append(eventosVerificacion, f.EventosPendientes()...)
	}

	if factorElegido == nil {
		// El código no coincidió con ningún factor: se audita
		// VerificacionOTPFallida (ya emitido por el dominio), sin UoW
		// porque no hay ningún agregado que persistir.
		origenVacio := dominio.OrigenSolicitud{}
		for _, e := range eventosVerificacion {
			if errAud := c.auditoria.Registrar(ctx, e, origenVacio); errAud != nil {
				return errAud
			}
		}
		return &dominio.ErrCodigoOTPInvalido{}
	}

	usuario, err := c.usuarios.BuscarPorID(ctx, idSujeto)
	if err != nil {
		return err
	}
	if usuario == nil {
		return &dominio.ErrUsuarioNoEncontrado{IDUsuario: cmd.IDSujeto}
	}

	// GAP DE PUERTO (reportado, no resuelto en silencio): RepositorioFactoresMFA
	// no expone ningún método para eliminar o desactivar de forma persistente
	// un FactorMFA (solo Guardar/BuscarPorID/BuscarConfirmadosDeUsuario/
	// ContarConfirmadosDeUsuario), y dominio.FactorMFA tampoco tiene un campo
	// que Deshabilitar() pueda voltear (Confirmado() se queda en true para
	// siempre; Deshabilitar() solo acumula el evento de auditoría). Además la
	// migración 000015 revoca DELETE sobre factores_mfa al rol de la app, así
	// que ni siquiera un futuro RepositorioFactoresMFA.Eliminar podría hacer
	// un DELETE físico: haría falta una columna persistida de estado (p. ej.
	// "habilitado"/"eliminado_en") que hoy no existe en el dominio ni en la
	// migración. Se llama Guardar aquí, replicando el patrón de
	// "mutar + guardar" del resto del sistema, para que el evento de
	// auditoría quede registrado y el flip de Usuario.tieneMFA se persista,
	// pero ADVERTENCIA: sin esa pieza adicional del puerto/dominio, el
	// FactorMFA queda persistido igual que antes (confirmado=true, mismo
	// secreto), así que un ContarConfirmadosDeUsuario posterior seguiría
	// contándolo y HabilitarMFA seguiría rechazando un nuevo factor con
	// ErrLimiteFactoresMFAExcedido. Esto se reporta explícitamente para que
	// quien posea identidad/puertos y el adaptador Postgres lo resuelva.
	factorElegido.Deshabilitar(ahora)
	usuario.DeshabilitarMFA(ahora)

	eventosPendientes := append(eventosVerificacion, factorElegido.EventosPendientes()...)
	origenVacio := dominio.OrigenSolicitud{}
	if err := c.uow.Ejecutar(ctx, func(ctx context.Context) error {
		if err := c.factoresMFA.Guardar(ctx, factorElegido); err != nil {
			return err
		}
		if err := c.usuarios.Guardar(ctx, usuario); err != nil {
			return err
		}
		for _, e := range eventosPendientes {
			if err := c.auditoria.Registrar(ctx, e, origenVacio); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}

	if errPub := c.eventos.Publicar(ctx, eventosPendientes...); errPub != nil {
		slog.WarnContext(ctx, "publicación de eventos de deshabilitación de MFA falló",
			"error", errPub, "usuario_id", idSujeto.String())
	}

	return nil
}
