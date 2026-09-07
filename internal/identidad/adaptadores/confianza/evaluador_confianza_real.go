package confianza

import (
	"context"
	"strings"

	"github.com/r-david1/moterus/internal/confianza/dominio"
	confianzapuertos "github.com/r-david1/moterus/internal/confianza/puertos"
	"github.com/r-david1/moterus/internal/identidad/puertos"
)

// EvaluadorConfianzaReal implementa puertos.EvaluadorConfianza (Identidad)
// delegando en confianza/puertos.EvaluadorDeRiesgo (Confianza): es el ACL
// que traduce entre el vocabulario de los dos contextos, tal como exige la
// sección 5 del diseño de Identidad ("la capa anticorrupción la posee
// quien depende, no quien es dependido"). Sustituye a
// EvaluadorConfianzaNoOp cuando Redis está configurado (ver cmd/api/main.go
// y ADR 0018) — Identidad no cambia una sola línea para pasar de uno a
// otro, ambos implementan el mismo puerto.
type EvaluadorConfianzaReal struct {
	riesgo confianzapuertos.EvaluadorDeRiesgo
}

var _ puertos.EvaluadorConfianza = (*EvaluadorConfianzaReal)(nil)

// NuevoEvaluadorConfianzaReal construye el ACL sobre un
// confianza/puertos.EvaluadorDeRiesgo ya ensamblado (típicamente
// aplicacion.EvaluarTrustSignalCasoDeUso con sus adaptadores Redis/
// Turnstile).
func NuevoEvaluadorConfianzaReal(riesgo confianzapuertos.EvaluadorDeRiesgo) *EvaluadorConfianzaReal {
	return &EvaluadorConfianzaReal{riesgo: riesgo}
}

// Evaluar traduce identidad/puertos.SolicitudEvaluacion ->
// confianza/puertos.Solicitud, delega, y traduce la
// confianza/dominio.Decision resultante -> identidad/puertos.
// DecisionConfianza.
func (a *EvaluadorConfianzaReal) Evaluar(ctx context.Context, s puertos.SolicitudEvaluacion) (puertos.DecisionConfianza, error) {
	decision, err := a.riesgo.Evaluar(ctx, confianzapuertos.Solicitud{
		Accion:            accionConfianza(s.Accion),
		IPOrigen:          s.Origen.IP().String(),
		CorreoNormalizado: normalizarCorreoLaxo(s.CorreoNormalizado),
		TokenCaptcha:      s.TokenCaptcha,
		HuellaDispositivo: s.Origen.HuellaDispositivo(),
	})
	if err != nil {
		return puertos.DecisionConfianza{}, err
	}
	// NO agregar aquí PuntajeRiesgo/NivelRiesgo/SenalesDeRiesgo: INV-RIES-09
	// (docs/design/fingerprinting-comportamiento.md §4) prohíbe que esos tres
	// campos crucen la frontera de contexto. identidad/puertos.
	// DecisionConfianza no los declara a propósito — publicarlos le diría a
	// un atacante cuánto le falta para disparar el detector de riesgo. Ver
	// TestEvaluadorConfianzaReal_NuncaExponeCamposDeRiesgo.
	return puertos.DecisionConfianza{
		Permitido:       decision.Permitido,
		RequiereStepUp:  decision.RequiereStepUp,
		RequiereCaptcha: decision.RequiereCaptcha,
		Puntaje:         decision.Puntaje,
		Motivo:          decision.Motivo,
		ReintentarEn:    decision.ReintentarEn,
	}, nil
}

// RegistrarResultado traduce identidad/puertos.ResultadoIntento ->
// confianza/puertos.ResultadoIntento.
func (a *EvaluadorConfianzaReal) RegistrarResultado(ctx context.Context, r puertos.ResultadoIntento) error {
	return a.riesgo.RegistrarResultado(ctx, confianzapuertos.ResultadoIntento{
		Accion:            accionConfianza(r.Accion),
		IPOrigen:          r.Origen.IP().String(),
		CorreoNormalizado: normalizarCorreoLaxo(r.CorreoNormalizado),
		Exitoso:           r.Exitoso,
		HuellaDispositivo: r.Origen.HuellaDispositivo(),
		IDUsuario:         r.UsuarioID,
		IDSolicitud:       r.Origen.IDSolicitud(),
	})
}

// accionConfianza traduce el string suelto de identidad/puertos.
// SolicitudEvaluacion.Accion ("login" | "registro" | "reset_contrasena")
// al tipo dominio.Accion de Confianza. Un valor no reconocido (p. ej.
// "reset_contrasena", todavía no implementado en el backlog de Identidad,
// o cualquier acción futura) se deja pasar tal cual: dominio.
// PoliticaLimites.Para ya tiene un umbral fail-safe para acciones sin
// entrada explícita, así que no hace falta un catálogo cerrado aquí.
func accionConfianza(s string) dominio.Accion {
	return dominio.Accion(s)
}

// normalizarCorreoLaxo normaliza a minúsculas y sin espacios el correo
// usado como clave de rate limiting por cuenta. Deliberadamente NO usa
// identidad/dominio.NuevoCorreo (que puede devolver error en un correo
// malformado): la clave de rate limit debe poder construirse incluso para
// una entrada inválida — es precisamente el escenario de un atacante
// probando strings al azar contra el endpoint de login/registro (INV-ID-12
// se cumple igual: Evaluar se llama ANTES de que el caso de uso valide el
// VO Correo).
func normalizarCorreoLaxo(correo string) string {
	return strings.ToLower(strings.TrimSpace(correo))
}
