package aplicacion

import (
	"context"
	"log/slog"

	"github.com/r-david1/moterus/internal/identidad/dominio"
	"github.com/r-david1/moterus/internal/identidad/puertos"
)

// emisorTOTP es el nombre que identifica a este servicio dentro de la app
// autenticadora del usuario (parámetro "issuer" de la URI otpauth://,
// §1.4 del diseño otp-mfa.md). No existía ninguna constante equivalente en
// el resto del código, así que se introduce aquí con el nombre del
// producto.
const emisorTOTP = "Moterus"

// HabilitarMFACasoDeUso implementa puertos.GestorDeMFA.Habilitar (§3.1 del
// diseño otp-mfa.md): genera un FactorMFA TOTP sin confirmar para el sujeto
// autenticado y devuelve el secreto en claro + la URI de provisionamiento,
// la única vez que ambos salen del proceso (INV-MFA-02).
type HabilitarMFACasoDeUso struct {
	usuarios         puertos.RepositorioUsuarios
	factoresMFA      puertos.RepositorioFactoresMFA
	generadorSecreto puertos.GeneradorSecretoTOTP
	cifrador         puertos.CifradorSecretos
	auditoria        puertos.RegistroAuditoria
	eventos          puertos.PublicadorEventos
	reloj            puertos.Reloj
	ids              puertos.GeneradorIDs
	uow              puertos.UnidadDeTrabajo
}

// NuevoHabilitarMFACasoDeUso construye el caso de uso con sus dependencias
// inyectadas por puerto (nunca implementaciones concretas).
func NuevoHabilitarMFACasoDeUso(
	usuarios puertos.RepositorioUsuarios,
	factoresMFA puertos.RepositorioFactoresMFA,
	generadorSecreto puertos.GeneradorSecretoTOTP,
	cifrador puertos.CifradorSecretos,
	auditoria puertos.RegistroAuditoria,
	eventos puertos.PublicadorEventos,
	reloj puertos.Reloj,
	ids puertos.GeneradorIDs,
	uow puertos.UnidadDeTrabajo,
) *HabilitarMFACasoDeUso {
	return &HabilitarMFACasoDeUso{
		usuarios:         usuarios,
		factoresMFA:      factoresMFA,
		generadorSecreto: generadorSecreto,
		cifrador:         cifrador,
		auditoria:        auditoria,
		eventos:          eventos,
		reloj:            reloj,
		ids:              ids,
		uow:              uow,
	}
}

// Habilitar ejecuta el flujo normativo de la sección 3.1 del diseño:
//  1. El sujeto debe estar en un estado desde el que pueda operar su cuenta
//     (mismo criterio que Usuario.PuedeIniciarSesion, reutilizado aquí en
//     lugar de duplicar la máquina de estados).
//  2. Como máximo un factor confirmado en el MVP (ADR 0037):
//     ErrLimiteFactoresMFAExcedido si ya tiene uno.
//  3. Generar y cifrar el secreto, crear el FactorMFA sin confirmar.
//  4. Persistir + auditar FactorMFAHabilitado en la misma UnidadDeTrabajo.
//  5. Devolver el secreto en claro y la URI de provisionamiento (única vez,
//     INV-MFA-02).
func (c *HabilitarMFACasoDeUso) Habilitar(ctx context.Context, cmd puertos.ComandoHabilitarMFA) (puertos.ResultadoHabilitarMFA, error) {
	idSujeto, err := dominio.IDUsuarioDesde(cmd.IDSujeto)
	if err != nil {
		return puertos.ResultadoHabilitarMFA{}, err
	}

	usuario, err := c.usuarios.BuscarPorID(ctx, idSujeto)
	if err != nil {
		return puertos.ResultadoHabilitarMFA{}, err
	}
	if usuario == nil {
		return puertos.ResultadoHabilitarMFA{}, &dominio.ErrUsuarioNoEncontrado{IDUsuario: cmd.IDSujeto}
	}

	// 1. Reutiliza PuedeIniciarSesion (INV-ID-06) en lugar de duplicar la
	// máquina de estados: solo una cuenta operativa puede administrar su
	// propio MFA.
	if err := usuario.PuedeIniciarSesion(); err != nil {
		return puertos.ResultadoHabilitarMFA{}, err
	}

	// 2. MVP de un solo factor confirmado (ADR 0037).
	confirmados, err := c.factoresMFA.ContarConfirmadosDeUsuario(ctx, idSujeto)
	if err != nil {
		return puertos.ResultadoHabilitarMFA{}, err
	}
	if confirmados >= 1 {
		return puertos.ResultadoHabilitarMFA{}, &dominio.ErrLimiteFactoresMFAExcedido{}
	}

	// 3. Generar y cifrar el secreto.
	secretoPlano, err := c.generadorSecreto.GenerarSecreto()
	if err != nil {
		return puertos.ResultadoHabilitarMFA{}, err
	}
	secretoCifrado, err := c.cifrador.Cifrar(secretoPlano)
	if err != nil {
		return puertos.ResultadoHabilitarMFA{}, err
	}

	idFactor, err := c.ids.NuevoIDFactorMFA()
	if err != nil {
		return puertos.ResultadoHabilitarMFA{}, err
	}

	ahora := c.reloj.Ahora()
	factor, err := dominio.HabilitarFactorMFA(idFactor, idSujeto, dominio.TipoFactorTOTP, secretoCifrado, ahora)
	if err != nil {
		return puertos.ResultadoHabilitarMFA{}, err
	}
	eventosPendientes := factor.EventosPendientes()

	// 4. Persistir + auditar en la misma transacción (ADR 0005).
	//
	// ComandoHabilitarMFA no transporta OrigenSolicitud (a diferencia de
	// ComandoRegistrarUsuario/ComandoAutenticar): es una decisión ya cerrada
	// del puerto de entrada, no algo que este caso de uso pueda cambiar. El
	// evento se audita con un OrigenSolicitud vacío (misma convención que
	// "llamada interna del sistema").
	origenVacio := dominio.OrigenSolicitud{}
	if err := c.uow.Ejecutar(ctx, func(ctx context.Context) error {
		if err := c.factoresMFA.Guardar(ctx, factor); err != nil {
			return err
		}
		for _, e := range eventosPendientes {
			if err := c.auditoria.Registrar(ctx, e, origenVacio); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return puertos.ResultadoHabilitarMFA{}, err
	}

	if errPub := c.eventos.Publicar(ctx, eventosPendientes...); errPub != nil {
		slog.WarnContext(ctx, "publicación de eventos de habilitación de MFA falló",
			"error", errPub, "usuario_id", idSujeto.String())
	}

	return puertos.ResultadoHabilitarMFA{
		IDFactor:            factor.ID().String(),
		SecretoEnClaro:      secretoPlano.Valor(),
		URIProvisionamiento: secretoPlano.URIProvisionamiento(usuario.Correo(), emisorTOTP),
	}, nil
}
