// Package confianza es el ACL de Tenencia sobre el contexto Confianza
// (mismo criterio que acceso/adaptadores/confianza e
// identidad/adaptadores/confianza): traduce entre
// tenencia/puertos.SolicitudEvaluacion/DecisionConfianza/ResultadoIntento y
// confianza/puertos.Solicitud/Decision/ResultadoIntento, delegando en
// confianza/puertos.EvaluadorDeRiesgo (aplicacion.EvaluarTrustSignalCasoDeUso,
// ya construido — no se reimplementa el motor Lua de Redis).
package confianza

import (
	"context"
	"log/slog"

	confianzadominio "github.com/r-david1/moterus/internal/confianza/dominio"
	confianzapuertos "github.com/r-david1/moterus/internal/confianza/puertos"
	"github.com/r-david1/moterus/internal/tenencia/puertos"
)

// EvaluadorConfianzaReal implementa tenencia/puertos.EvaluadorConfianza
// delegando en confianza/puertos.EvaluadorDeRiesgo.
//
// Reutiliza el campo confianza/puertos.Solicitud.CorreoNormalizado para
// transportar la ClaveCuenta de Tenencia ("usuario:<id>" u
// "organizacion:<id>", nunca un correo real) — mismo criterio ya adoptado
// por acceso/adaptadores/confianza, documentado en su §11.2: el campo hoy
// solo sirve como clave del segundo nivel de rate limiting dentro de
// EvaluarTrustSignalCasoDeUso, que la trata como una cadena opaca. Con tres
// contextos (Identidad, Acceso, Tenencia) pidiendo lo mismo, el rename a
// "ClaveCuenta" que §11.2 del diseño de Acceso propuso deja de ser opinión
// — se deja anotado aquí para quien lo aborde, sin tocar confianza/puertos
// en este cambio.
//
// TenantID SÍ se puebla aquí, a diferencia de Identidad y Acceso: es la
// pieza que cierra el comentario "Vacío hoy siempre" de
// confianza/puertos.Solicitud.TenantID (§11.3 del diseño de Tenencia).
type EvaluadorConfianzaReal struct {
	riesgo confianzapuertos.EvaluadorDeRiesgo
}

var _ puertos.EvaluadorConfianza = (*EvaluadorConfianzaReal)(nil)

// NuevoEvaluadorConfianzaReal construye el ACL sobre un
// confianza/puertos.EvaluadorDeRiesgo ya ensamblado.
func NuevoEvaluadorConfianzaReal(riesgo confianzapuertos.EvaluadorDeRiesgo) *EvaluadorConfianzaReal {
	return &EvaluadorConfianzaReal{riesgo: riesgo}
}

// Evaluar traduce tenencia/puertos.SolicitudEvaluacion ->
// confianza/puertos.Solicitud, delega, y traduce la decisión resultante.
func (a *EvaluadorConfianzaReal) Evaluar(ctx context.Context, s puertos.SolicitudEvaluacion) (puertos.DecisionConfianza, error) {
	decision, err := a.riesgo.Evaluar(ctx, confianzapuertos.Solicitud{
		Accion:            confianzadominio.Accion(s.Accion),
		IPOrigen:          s.Origen.IP().String(),
		CorreoNormalizado: s.ClaveCuenta,
		TokenCaptcha:      s.TokenCaptcha,
		TenantID:          s.TenantID,
	})
	if err != nil {
		return puertos.DecisionConfianza{}, err
	}
	return puertos.DecisionConfianza{
		Permitido:    decision.Permitido,
		Puntaje:      decision.Puntaje,
		Motivo:       decision.Motivo,
		ReintentarEn: decision.ReintentarEn,
	}, nil
}

// RegistrarResultado traduce tenencia/puertos.ResultadoIntento ->
// confianza/puertos.ResultadoIntento.
func (a *EvaluadorConfianzaReal) RegistrarResultado(ctx context.Context, r puertos.ResultadoIntento) error {
	return a.riesgo.RegistrarResultado(ctx, confianzapuertos.ResultadoIntento{
		Accion:            confianzadominio.Accion(r.Accion),
		IPOrigen:          r.Origen.IP().String(),
		CorreoNormalizado: r.ClaveCuenta,
		Exitoso:           r.Exitoso,
	})
}

// EvaluadorConfianzaNoOp implementa tenencia/puertos.EvaluadorConfianza
// mientras Redis no está configurado (REDIS_URL vacío) — mismo criterio y
// mismo WARN de arranque que los adaptadores homónimos de Identidad y
// Acceso: siempre permite el intento, sin rate limiting real. No usar en
// producción.
type EvaluadorConfianzaNoOp struct {
	log *slog.Logger
}

var _ puertos.EvaluadorConfianza = (*EvaluadorConfianzaNoOp)(nil)

// NuevoEvaluadorConfianzaNoOp construye el adaptador no-op y emite
// inmediatamente el WARN de arranque. log puede ser nil (usa
// slog.Default()).
func NuevoEvaluadorConfianzaNoOp(log *slog.Logger) *EvaluadorConfianzaNoOp {
	if log == nil {
		log = slog.Default()
	}
	log.Warn("tenencia/adaptadores/confianza: EvaluadorConfianza en modo no-op — " +
		"sin rate limiting en crear_organizacion, invitar_miembro ni aceptar_invitacion. " +
		"NO USAR EN PRODUCCIÓN. Definir REDIS_URL para montar el evaluador real.")
	return &EvaluadorConfianzaNoOp{log: log}
}

// Evaluar siempre permite el intento.
func (e *EvaluadorConfianzaNoOp) Evaluar(_ context.Context, _ puertos.SolicitudEvaluacion) (puertos.DecisionConfianza, error) {
	return puertos.DecisionConfianza{Permitido: true}, nil
}

// RegistrarResultado no hace nada.
func (e *EvaluadorConfianzaNoOp) RegistrarResultado(_ context.Context, _ puertos.ResultadoIntento) error {
	return nil
}
