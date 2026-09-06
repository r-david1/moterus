package aplicacion

import (
	"context"
	"errors"
	"log/slog"

	"github.com/r-david1/moterus/internal/identidad/dominio"
	"github.com/r-david1/moterus/internal/identidad/puertos"
)

// codigosRespaldoGenerados es la cantidad fija de códigos de respaldo que se
// generan al confirmar un factor (ADR 0040).
const codigosRespaldoGenerados = 10

// ConfirmarFactorMFACasoDeUso implementa puertos.GestorDeMFA.ConfirmarFactor
// (§3.2 del diseño otp-mfa.md): valida el primer código TOTP contra el
// factor recién habilitado, y si es válido genera los códigos de respaldo y
// aplica el flip coordinado de Usuario.tieneMFA en la misma transacción
// (INV-ID-08/INV-MFA-01).
type ConfirmarFactorMFACasoDeUso struct {
	usuarios         puertos.RepositorioUsuarios
	factoresMFA      puertos.RepositorioFactoresMFA
	generadorSecreto puertos.GeneradorSecretoTOTP
	cifrador         puertos.CifradorSecretos
	auditoria        puertos.RegistroAuditoria
	eventos          puertos.PublicadorEventos
	reloj            puertos.Reloj
	uow              puertos.UnidadDeTrabajo
}

// NuevoConfirmarFactorMFACasoDeUso construye el caso de uso con sus
// dependencias inyectadas por puerto.
func NuevoConfirmarFactorMFACasoDeUso(
	usuarios puertos.RepositorioUsuarios,
	factoresMFA puertos.RepositorioFactoresMFA,
	generadorSecreto puertos.GeneradorSecretoTOTP,
	cifrador puertos.CifradorSecretos,
	auditoria puertos.RegistroAuditoria,
	eventos puertos.PublicadorEventos,
	reloj puertos.Reloj,
	uow puertos.UnidadDeTrabajo,
) *ConfirmarFactorMFACasoDeUso {
	return &ConfirmarFactorMFACasoDeUso{
		usuarios:         usuarios,
		factoresMFA:      factoresMFA,
		generadorSecreto: generadorSecreto,
		cifrador:         cifrador,
		auditoria:        auditoria,
		eventos:          eventos,
		reloj:            reloj,
		uow:              uow,
	}
}

// ConfirmarFactor ejecuta el flujo normativo de la sección 3.2 del diseño:
//  1. Cargar el FactorMFA por ID y verificar que pertenece al sujeto
//     (ErrFactorMFANoEncontrado en caso contrario, sin distinguir "no
//     existe" de "es de otro usuario").
//  2. Descifrar el secreto y verificar el código contra FactorMFA.Confirmar
//     (que ya produce ErrFactorMFAYaConfirmado / ErrCodigoOTPInvalido sin
//     mutar el agregado si falla).
//  3. Si es válido: generar y hashear 10 códigos de respaldo (ADR 0040),
//     asignarlos al factor, y aplicar el flip coordinado
//     Usuario.HabilitarMFA en la misma UnidadDeTrabajo que la persistencia
//     de ambos agregados y la auditoría de FactorMFAConfirmado.
//  4. Devolver los 10 códigos de respaldo en claro (única vez, ADR 0040).
func (c *ConfirmarFactorMFACasoDeUso) ConfirmarFactor(ctx context.Context, cmd puertos.ComandoConfirmarFactorMFA) (puertos.ResultadoConfirmarMFA, error) {
	idSujeto, err := dominio.IDUsuarioDesde(cmd.IDSujeto)
	if err != nil {
		return puertos.ResultadoConfirmarMFA{}, err
	}
	idFactor, err := dominio.IDFactorMFADesde(cmd.IDFactor)
	if err != nil {
		return puertos.ResultadoConfirmarMFA{}, err
	}
	codigo, err := dominio.NuevoCodigoTOTP(cmd.Codigo)
	if err != nil {
		return puertos.ResultadoConfirmarMFA{}, err
	}

	factor, err := c.buscarFactorDelSujeto(ctx, idFactor, idSujeto)
	if err != nil {
		return puertos.ResultadoConfirmarMFA{}, err
	}

	usuario, err := c.usuarios.BuscarPorID(ctx, idSujeto)
	if err != nil {
		return puertos.ResultadoConfirmarMFA{}, err
	}
	if usuario == nil {
		return puertos.ResultadoConfirmarMFA{}, &dominio.ErrUsuarioNoEncontrado{IDUsuario: cmd.IDSujeto}
	}

	secretoDescifrado, err := c.cifrador.Descifrar(factor.SecretoCifrado())
	if err != nil {
		return puertos.ResultadoConfirmarMFA{}, err
	}

	ahora := c.reloj.Ahora()
	if err := factor.Confirmar(codigo, secretoDescifrado.Valor(), ahora); err != nil {
		// Un código incorrecto (o un factor ya confirmado) no muta el
		// agregado: no hay nada que persistir ni que auditar.
		return puertos.ResultadoConfirmarMFA{}, err
	}

	// ADR 0040: los códigos de respaldo se generan exactamente aquí, nunca
	// en HabilitarMFA.
	codigosPlanos, err := c.generadorSecreto.GenerarCodigosRespaldo(codigosRespaldoGenerados)
	if err != nil {
		return puertos.ResultadoConfirmarMFA{}, err
	}
	hashes := make([]dominio.HashCodigoRespaldo, 0, len(codigosPlanos))
	codigosEnClaro := make([]string, 0, len(codigosPlanos))
	for _, cp := range codigosPlanos {
		hashes = append(hashes, cp.Hash())
		codigosEnClaro = append(codigosEnClaro, cp.Valor())
	}
	factor.AsignarCodigosRespaldo(hashes)

	// Flip coordinado (INV-ID-08/INV-MFA-01): ocurre exactamente aquí, en la
	// misma unidad de trabajo que la confirmación del factor.
	usuario.HabilitarMFA(ahora)

	eventosPendientes := factor.EventosPendientes()
	origenVacio := dominio.OrigenSolicitud{} // ver nota en habilitar_mfa.go
	if err := c.uow.Ejecutar(ctx, func(ctx context.Context) error {
		if err := c.factoresMFA.Guardar(ctx, factor); err != nil {
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
		return puertos.ResultadoConfirmarMFA{}, err
	}

	if errPub := c.eventos.Publicar(ctx, eventosPendientes...); errPub != nil {
		slog.WarnContext(ctx, "publicación de eventos de confirmación de MFA falló",
			"error", errPub, "usuario_id", idSujeto.String())
	}

	return puertos.ResultadoConfirmarMFA{CodigosRespaldo: codigosEnClaro}, nil
}

// buscarFactorDelSujeto carga un FactorMFA por ID y verifica que pertenece
// al sujeto que lo solicita. No distingue, en el error observable, "no
// existe" de "es de otro usuario" (mismo criterio de no filtrar existencia
// que el resto del sistema aplica a otros recursos ajenos).
func (c *ConfirmarFactorMFACasoDeUso) buscarFactorDelSujeto(ctx context.Context, idFactor dominio.IDFactorMFA, idSujeto dominio.IDUsuario) (*dominio.FactorMFA, error) {
	factor, err := c.factoresMFA.BuscarPorID(ctx, idFactor)
	if err != nil {
		var noEncontrado *dominio.ErrFactorMFANoEncontrado
		if !errors.As(err, &noEncontrado) {
			return nil, err
		}
		factor = nil
	}
	if factor == nil || !factor.UsuarioID().EsIgual(idSujeto) {
		return nil, &dominio.ErrFactorMFANoEncontrado{}
	}
	return factor, nil
}
