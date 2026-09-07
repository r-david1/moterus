// Package confianza es el ACL de Acceso sobre el contexto Confianza (mismo
// criterio que identidad/adaptadores/confianza): traduce entre
// acceso/puertos.SolicitudEvaluacion/DecisionConfianza/ResultadoIntento y
// confianza/puertos.Solicitud/Decision/ResultadoIntento, delegando en
// confianza/puertos.EvaluadorDeRiesgo (aplicacion.EvaluarTrustSignalCasoDeUso
// del contexto Confianza, ya construido — no se reimplementa el motor Lua
// de Redis). Acceso lo consulta en RenovarSesion y en CerrarTodas (§0 del
// diseño: en login no, porque Identidad ya lo hizo dentro de
// AutenticarUsuario).
package confianza

import (
	"context"
	"log/slog"

	accesopuertos "github.com/r-david1/moterus/internal/acceso/puertos"
	confianzadominio "github.com/r-david1/moterus/internal/confianza/dominio"
	confianzapuertos "github.com/r-david1/moterus/internal/confianza/puertos"
)

// EvaluadorConfianzaReal implementa acceso/puertos.EvaluadorConfianza
// delegando en confianza/puertos.EvaluadorDeRiesgo.
//
// Decisión de diseño no obvia (§11.2 del diseño de Acceso — la propuesta
// de renombrar confianza/puertos.Solicitud.CorreoNormalizado a un
// "ClaveCuenta" más genérico queda declarada como trabajo de quien
// mantenga Confianza, no de este encargo): este ACL reutiliza ese mismo
// campo para transportar la ClaveCuenta de Acceso ("sesion:<id>" o
// "usuario:<id>", nunca un correo real) — el campo hoy solo sirve como
// clave del segundo nivel de rate limiting (por cuenta) dentro de
// EvaluarTrustSignalCasoDeUso, que la trata como una cadena opaca sin
// interpretarla como correo en ningún punto. No se modificó el tipo
// puertos.Solicitud de Confianza para no tocar un contexto ya cerrado por
// un motivo cosmético.
type EvaluadorConfianzaReal struct {
	riesgo confianzapuertos.EvaluadorDeRiesgo
}

var _ accesopuertos.EvaluadorConfianza = (*EvaluadorConfianzaReal)(nil)

// NuevoEvaluadorConfianzaReal construye el ACL sobre un
// confianza/puertos.EvaluadorDeRiesgo ya ensamblado (típicamente el mismo
// aplicacion.EvaluarTrustSignalCasoDeUso que ya usa Identidad, compartiendo
// el mismo LimitadorTasa/VerificadorCaptcha — ver cmd/api/main.go).
func NuevoEvaluadorConfianzaReal(riesgo confianzapuertos.EvaluadorDeRiesgo) *EvaluadorConfianzaReal {
	return &EvaluadorConfianzaReal{riesgo: riesgo}
}

// Evaluar traduce acceso/puertos.SolicitudEvaluacion ->
// confianza/puertos.Solicitud, delega, y traduce la decisión resultante.
func (a *EvaluadorConfianzaReal) Evaluar(ctx context.Context, s accesopuertos.SolicitudEvaluacion) (accesopuertos.DecisionConfianza, error) {
	decision, err := a.riesgo.Evaluar(ctx, confianzapuertos.Solicitud{
		Accion:            confianzadominio.Accion(s.Accion),
		IPOrigen:          s.Origen.IP().String(),
		CorreoNormalizado: s.ClaveCuenta,
		TokenCaptcha:      s.TokenCaptcha,
		HuellaDispositivo: s.Origen.HuellaDispositivo(),
	})
	if err != nil {
		return accesopuertos.DecisionConfianza{}, err
	}
	// NO agregar aquí PuntajeRiesgo/NivelRiesgo/SenalesDeRiesgo: INV-RIES-09
	// (docs/design/fingerprinting-comportamiento.md §4) prohíbe que esos
	// tres campos crucen la frontera de contexto. acceso/puertos.
	// DecisionConfianza no los declara a propósito. Ver
	// TestEvaluadorConfianzaReal_NuncaExponeCamposDeRiesgo.
	return accesopuertos.DecisionConfianza{
		Permitido:      decision.Permitido,
		RequiereStepUp: decision.RequiereStepUp,
		Puntaje:        decision.Puntaje,
		Motivo:         decision.Motivo,
		ReintentarEn:   decision.ReintentarEn,
	}, nil
}

// RegistrarResultado traduce acceso/puertos.ResultadoIntento ->
// confianza/puertos.ResultadoIntento.
func (a *EvaluadorConfianzaReal) RegistrarResultado(ctx context.Context, r accesopuertos.ResultadoIntento) error {
	return a.riesgo.RegistrarResultado(ctx, confianzapuertos.ResultadoIntento{
		Accion:            confianzadominio.Accion(r.Accion),
		IPOrigen:          r.Origen.IP().String(),
		CorreoNormalizado: r.ClaveCuenta,
		Exitoso:           r.Exitoso,
		HuellaDispositivo: r.Origen.HuellaDispositivo(),
		IDUsuario:         r.IDUsuario,
		IDSolicitud:       r.Origen.IDSolicitud(),
	})
}

// EvaluadorConfianzaNoOp implementa acceso/puertos.EvaluadorConfianza
// mientras Redis no está configurado (REDIS_URL vacío) — mismo criterio y
// mismo WARN de arranque que
// identidad/adaptadores/confianza.EvaluadorConfianzaNoOp: siempre permite
// el intento, sin rate limiting real. No usar en producción.
type EvaluadorConfianzaNoOp struct {
	log *slog.Logger
}

var _ accesopuertos.EvaluadorConfianza = (*EvaluadorConfianzaNoOp)(nil)

// NuevoEvaluadorConfianzaNoOp construye el adaptador no-op y emite
// inmediatamente el WARN de arranque. log puede ser nil (usa
// slog.Default()).
func NuevoEvaluadorConfianzaNoOp(log *slog.Logger) *EvaluadorConfianzaNoOp {
	if log == nil {
		log = slog.Default()
	}
	log.Warn("acceso/adaptadores/confianza: EvaluadorConfianza en modo no-op — " +
		"sin rate limiting en renovacion_sesion ni cierre_masivo_sesiones. " +
		"NO USAR EN PRODUCCIÓN. Definir REDIS_URL para montar el evaluador real.")
	return &EvaluadorConfianzaNoOp{log: log}
}

// Evaluar siempre permite el intento.
func (e *EvaluadorConfianzaNoOp) Evaluar(_ context.Context, _ accesopuertos.SolicitudEvaluacion) (accesopuertos.DecisionConfianza, error) {
	return accesopuertos.DecisionConfianza{Permitido: true}, nil
}

// RegistrarResultado no hace nada.
func (e *EvaluadorConfianzaNoOp) RegistrarResultado(_ context.Context, _ accesopuertos.ResultadoIntento) error {
	return nil
}
