package confianza

import (
	"context"
	"log/slog"

	"github.com/r-david1/moterus/internal/identidad/puertos"
)

// EvaluadorConfianzaNoOp implementa puertos.EvaluadorConfianza mientras el
// contexto Confianza no exista (sección 8 del diseño de Identidad: "el
// no-op se registra en el arranque con un WARN explícito para que nadie
// despliegue así a producción"). Siempre permite el intento y nunca exige
// step-up ni captcha: NO aplica rate limiting, fingerprinting ni score de
// riesgo. Desplegar esto en producción deja a Identidad sin ninguna defensa
// contra credential stuffing (INV-ID-12 se cumple formalmente — el puerto se
// invoca — pero sin ningún efecto real).
type EvaluadorConfianzaNoOp struct {
	log *slog.Logger
}

var _ puertos.EvaluadorConfianza = (*EvaluadorConfianzaNoOp)(nil)

// NuevoEvaluadorConfianzaNoOp construye el adaptador no-op y emite
// inmediatamente el WARN de arranque exigido por el diseño. log puede ser
// nil, en cuyo caso se usa slog.Default().
func NuevoEvaluadorConfianzaNoOp(log *slog.Logger) *EvaluadorConfianzaNoOp {
	if log == nil {
		log = slog.Default()
	}
	log.Warn("identidad/adaptadores/confianza: EvaluadorConfianza en modo no-op — " +
		"sin rate limiting, sin fingerprinting, sin score de riesgo. " +
		"NO USAR EN PRODUCCIÓN. Implementar el contexto Confianza antes de desplegar.")
	return &EvaluadorConfianzaNoOp{log: log}
}

// Evaluar siempre permite el intento, sin step-up ni captcha.
func (e *EvaluadorConfianzaNoOp) Evaluar(_ context.Context, _ puertos.SolicitudEvaluacion) (puertos.DecisionConfianza, error) {
	return puertos.DecisionConfianza{Permitido: true}, nil
}

// RegistrarResultado no hace nada: no hay contexto Confianza que alimentar
// con la señal del resultado del intento.
func (e *EvaluadorConfianzaNoOp) RegistrarResultado(_ context.Context, _ puertos.ResultadoIntento) error {
	return nil
}
