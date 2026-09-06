package aplicacion

import (
	"context"
	"log/slog"

	"github.com/r-david1/moterus/internal/identidad/dominio"
	"github.com/r-david1/moterus/internal/identidad/puertos"
)

// VerificarOTPCasoDeUso implementa puertos.VerificadorOTP.Verificar (§3.4
// del diseño otp-mfa.md): el camino caliente que ACCESO consume, vía su ACL
// hacia Identidad, para completar el flujo de step-up en cada login con
// segundo factor. Deliberadamente angosto: nunca revela a quien lo consume
// más que un booleano (INV-MFA-08).
type VerificarOTPCasoDeUso struct {
	factoresMFA puertos.RepositorioFactoresMFA
	cifrador    puertos.CifradorSecretos
	auditoria   puertos.RegistroAuditoria
	reloj       puertos.Reloj
	uow         puertos.UnidadDeTrabajo
}

var _ puertos.VerificadorOTP = (*VerificarOTPCasoDeUso)(nil)

// NuevoVerificarOTPCasoDeUso construye el caso de uso con sus dependencias
// inyectadas por puerto.
func NuevoVerificarOTPCasoDeUso(
	factoresMFA puertos.RepositorioFactoresMFA,
	cifrador puertos.CifradorSecretos,
	auditoria puertos.RegistroAuditoria,
	reloj puertos.Reloj,
	uow puertos.UnidadDeTrabajo,
) *VerificarOTPCasoDeUso {
	return &VerificarOTPCasoDeUso{
		factoresMFA: factoresMFA,
		cifrador:    cifrador,
		auditoria:   auditoria,
		reloj:       reloj,
		uow:         uow,
	}
}

// Verificar ejecuta el flujo normativo de la sección 3.4 del diseño:
//  1. Cargar los factores confirmados del usuario.
//  2. Si no hay ninguno, es una inconsistencia interna (INV-MFA-06:
//     RequiereSegundoFactor ya debería haber exigido tieneMFA o una
//     decisión de Confianza) — se registra en logs de aplicación, no se
//     audita como fallo del usuario, y se devuelve false.
//  3. Probar el código contra cada factor confirmado
//     (FactorMFA.VerificarCodigo, que ya internamente distingue TOTP de
//     código de respaldo y ya emite VerificacionOTPFallida si falla — no se
//     duplica esa auditoría aquí, solo se persiste lo que el dominio ya
//     generó).
//  4. Éxito: persistir el agregado (por si consumió un código de
//     respaldo) y cualquier evento que haya quedado pendiente
//     (CodigoRespaldoConsumido, si aplica); no se audita ningún "éxito" ad
//     hoc — eso ya lo hace Acceso al completar el login.
//  5. Fallo total: auditar VerificacionOTPFallida de cada factor probado y
//     devolver false.
func (c *VerificarOTPCasoDeUso) Verificar(ctx context.Context, q puertos.ConsultaVerificarOTP) (bool, error) {
	idUsuario, err := dominio.IDUsuarioDesde(q.IDUsuario)
	if err != nil {
		return false, err
	}

	factores, err := c.factoresMFA.BuscarConfirmadosDeUsuario(ctx, idUsuario)
	if err != nil {
		return false, err
	}
	if len(factores) == 0 {
		// INV-MFA-06: esto no debería poder pasar nunca (quien invoca este
		// puerto ya decidió que hacía falta un segundo factor). Se trata
		// como una inconsistencia interna, no como un intento fallido del
		// usuario.
		slog.ErrorContext(ctx, "VerificarOTP invocado sin ningún factor MFA confirmado: inconsistencia interna",
			"usuario_id", q.IDUsuario)
		return false, nil
	}

	ahora := c.reloj.Ahora()
	var eventosVerificacion []dominio.EventoDominio
	for _, f := range factores {
		secretoDescifrado, err := c.cifrador.Descifrar(f.SecretoCifrado())
		if err != nil {
			return false, err
		}
		if f.VerificarCodigo(q.Codigo, secretoDescifrado.Valor(), ahora) {
			eventosExito := f.EventosPendientes() // p. ej. CodigoRespaldoConsumido
			if err := c.uow.Ejecutar(ctx, func(ctx context.Context) error {
				if err := c.factoresMFA.Guardar(ctx, f); err != nil {
					return err
				}
				for _, e := range eventosExito {
					if err := c.auditoria.Registrar(ctx, e, q.Origen); err != nil {
						return err
					}
				}
				return nil
			}); err != nil {
				return false, err
			}
			return true, nil
		}
		eventosVerificacion = append(eventosVerificacion, f.EventosPendientes()...)
	}

	for _, e := range eventosVerificacion {
		if errAud := c.auditoria.Registrar(ctx, e, q.Origen); errAud != nil {
			return false, errAud
		}
	}
	return false, nil
}
