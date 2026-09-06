package aplicacion

import (
	"context"

	"github.com/r-david1/moterus/internal/acceso/dominio"
	"github.com/r-david1/moterus/internal/acceso/puertos"
)

// accionConfianzaVerificarOTP es la acción de Confianza para el paso de
// step-up (§3.6/§7 del diseño otp-mfa.md, rate limit 5/15min por usuario,
// 20/15min por IP). confianza/dominio/accion.go (contexto Confianza) no
// declara todavía una constante AccionVerificarOTP: extenderla es trabajo
// de un agente futuro sobre Confianza, fuera del alcance de este cambio
// (que solo toca Identidad/Acceso). Se usa el literal que el diseño ya fijó.
const accionConfianzaVerificarOTP = "verificar_otp"

// VerificadorSegundoFactor es el puerto de salida que
// CompletarSegundoFactorCasoDeUso consume para verificar el código OTP
// presentado contra Identidad (§3.6 del diseño otp-mfa.md).
//
// Nota de ubicación (gap reportado, no resuelto en silencio): por forma y
// criterio (tipos propios de Acceso a ambos lados, implementado por un ACL
// sobre identidad/puertos.VerificadorOTP en acceso/adaptadores/identidad/)
// esta interfaz debería vivir junto a AutenticadorIdentidad/
// ConsultorEstadoSujeto en acceso/puertos/salida.go — exactamente el mismo
// patrón que ya usa este contexto para cruzar hacia Identidad sin que
// acceso/aplicacion importe jamás un tipo de identidad/dominio ni
// identidad/puertos (INV-ACC-19). Se declara aquí, en la capa de
// aplicación, únicamente porque acceso/puertos está señalado como cerrado
// para este encargo. Quien lo posea debe trasladarla a salida.go sin
// cambiar su forma, e implementar el ACL correspondiente (el "método más"
// que el diseño ya anticipa para acceso/adaptadores/identidad/) — ninguno
// de los dos es trabajo de esta capa.
type VerificadorSegundoFactor interface {
	Verificar(ctx context.Context, idUsuario string, codigo string, origen dominio.OrigenSolicitud) (bool, error)
}

// ComandoCompletarSegundoFactor transporta la entrada de
// CompletarSegundoFactorCasoDeUso (§3.6 del diseño otp-mfa.md). No existe un
// puerto de entrada declarado en acceso/puertos/entrada.go para este caso
// de uso (a diferencia de IniciadorDeSesion/RenovadorDeSesion/...): ese
// archivo también está cerrado, así que el tipo vive junto al caso de uso
// que lo consume, sin alias en comandos.go (no hay interfaz que implementar
// que obligue a definirlo en puertos).
type ComandoCompletarSegundoFactor struct {
	TokenStepUp string
	Codigo      string
	Origen      dominio.OrigenSolicitud
}

// CompletarSegundoFactorCasoDeUso implementa el paso 2 del login con
// segundo factor (§3.6 del diseño otp-mfa.md): valida el token de step-up
// emitido por IniciarSesion, evalúa Confianza sobre la propia cuenta,
// verifica el código OTP contra Identidad y, si es válido, emite la sesión
// completa con amr=["pwd","otp"] — la misma lógica de emisión que
// IniciarSesion ya tenía, reutilizada tal cual (emitirSesionCompleta) en
// lugar de duplicada.
type CompletarSegundoFactorCasoDeUso struct {
	emisorStepUp puertos.EmisorTokenStepUp
	confianza    puertos.EvaluadorConfianza
	verificador  VerificadorSegundoFactor
	// emisorSesion es el mismo caso de uso IniciarSesion ya ensamblado: se
	// reutiliza únicamente por su método interno compartido
	// emitirSesionCompleta (mismo paquete aplicacion, método no exportado),
	// para no duplicar las ~10 dependencias de construir/persistir/firmar
	// una sesión completa en dos structs distintos.
	emisorSesion *IniciarSesionCasoDeUso
}

// NuevoCompletarSegundoFactorCasoDeUso construye el caso de uso con sus
// dependencias inyectadas por puerto. emisorSesion debe ser la misma
// instancia (o una equivalente) que ya se le pasa a
// NuevoManejadorAcceso/NuevoIniciarSesionCasoDeUso: comparten política,
// emisor y audiencia de configuración.
func NuevoCompletarSegundoFactorCasoDeUso(
	emisorStepUp puertos.EmisorTokenStepUp,
	confianza puertos.EvaluadorConfianza,
	verificador VerificadorSegundoFactor,
	emisorSesion *IniciarSesionCasoDeUso,
) *CompletarSegundoFactorCasoDeUso {
	return &CompletarSegundoFactorCasoDeUso{
		emisorStepUp: emisorStepUp,
		confianza:    confianza,
		verificador:  verificador,
		emisorSesion: emisorSesion,
	}
}

// CompletarSegundoFactor ejecuta el flujo normativo de la sección 3.6 del
// diseño:
//  1. Validar el token de step-up (firma, expiración de 5 minutos, `typ`
//     distintivo — INV-MFA-03/04, responsabilidad del adaptador detrás de
//     EmisorTokenStepUp.Validar).
//  2. Evaluar Confianza con Accion="verificar_otp", clave por IDUsuario
//     (ataque dirigido a una cuenta concreta, no a una IP genérica).
//  3. Verificar el código OTP contra Identidad.
//  4. Éxito: emitir la sesión completa con amr=["pwd","otp"].
//  5. Cualquier fallo (token inválido/expirado, Confianza deniega, código
//     incorrecto) se colapsa en un único error observable
//     (ErrCredencialesRechazadas/ErrAccesoDenegadoPorConfianza, ya
//     genéricos en todo el sistema): INV-MFA-08, no se le da a quien prueba
//     fuerza bruta información de diagnóstico sobre cuál de las dos cosas
//     falló.
func (c *CompletarSegundoFactorCasoDeUso) CompletarSegundoFactor(ctx context.Context, cmd ComandoCompletarSegundoFactor) (puertos.ResultadoSesion, error) {
	claims, err := c.emisorStepUp.Validar(ctx, cmd.TokenStepUp)
	if err != nil {
		// No se distingue "expirado" de "inválido" ni de ningún otro motivo
		// (INV-MFA-08): mismo error genérico que ya usa el resto del
		// sistema para credenciales rechazadas.
		return puertos.ResultadoSesion{}, &dominio.ErrCredencialesRechazadas{}
	}

	usuarioID, err := dominio.IDUsuarioDesde(claims.IDUsuario)
	if err != nil {
		return puertos.ResultadoSesion{}, &dominio.ErrCredencialesRechazadas{}
	}

	decision, err := c.confianza.Evaluar(ctx, puertos.SolicitudEvaluacion{
		Accion:      accionConfianzaVerificarOTP,
		ClaveCuenta: claveCuentaPorUsuario(usuarioID),
		Origen:      cmd.Origen,
	})
	if err != nil {
		return puertos.ResultadoSesion{}, err
	}
	if !decision.Permitido {
		return puertos.ResultadoSesion{}, &dominio.ErrAccesoDenegadoPorConfianza{Motivo: decision.Motivo, ReintentarEn: decision.ReintentarEn}
	}

	valido, err := c.verificador.Verificar(ctx, claims.IDUsuario, cmd.Codigo, cmd.Origen)
	if err != nil {
		return puertos.ResultadoSesion{}, err
	}
	if !valido {
		return puertos.ResultadoSesion{}, &dominio.ErrCredencialesRechazadas{}
	}

	return c.emisorSesion.emitirSesionCompleta(ctx, usuarioID, cmd.Origen, metodosAutenticacionConSegundoFactor)
}
