package dominio

import "time"

// Decision es el resultado de evaluar una señal de confianza. Se traduce
// 1:1 (ver identidad/adaptadores/confianza) a identidad/puertos.
// DecisionConfianza, que es el tipo que cruza la frontera de contexto —
// este vive en Confianza porque es el vocabulario propio del contexto
// (RequiereStepUp/RequiereCaptcha son decisiones de Confianza, no de
// Identidad, aunque el nombre del campo coincida).
type Decision struct {
	// Permitido decide si la acción puede continuar ahora mismo.
	Permitido bool
	// RequiereCaptcha señaliza que, para que la próxima solicitud de esta
	// misma cuenta/IP sea aceptada, debe incluir un TokenCaptcha válido.
	// No es un bloqueo por sí solo: una solicitud puede tener
	// Permitido=false y RequiereCaptcha=true a la vez (el motivo del
	// bloqueo actual es precisamente la falta de captcha).
	RequiereCaptcha bool
	// RequiereStepUp señaliza a Identidad que, aunque la contraseña sea
	// correcta, conviene exigir un segundo factor (puntaje de captcha bajo
	// pero no nulo — humano probable, riesgo elevado).
	RequiereStepUp bool
	// Puntaje es el score de captcha 0.0-1.0 (1.0 si no se evaluó
	// captcha porque no hizo falta). Se propaga hasta
	// ResultadoAutenticacion.PuntajeConfianza en la respuesta HTTP de
	// login.
	Puntaje float64
	// Motivo es un código corto y estable (snake_case) para auditoría y
	// logs — nunca un mensaje de usuario final. Ver ADR 0018 para el
	// catálogo de motivos que este contexto produce.
	Motivo string
	// ReintentarEn es cuánto falta para que la próxima solicitud tenga
	// alguna chance de ser permitida (Retry-After). Cero cuando
	// Permitido=true.
	ReintentarEn time.Duration

	// PuntajeRiesgo, NivelRiesgo y SenalesDeRiesgo son el resultado de la
	// extensión de reconocimiento de origen (§1.2 y §1.3 de
	// fingerprinting-comportamiento.md). INV-RIES-09: ninguno de los tres
	// cruza la frontera de contexto — los ACL (identidad|acceso|
	// tenencia/adaptadores/confianza) no los mapean a su DecisionConfianza,
	// así que no pueden llegar a una respuesta HTTP. Eso lo hacen cumplir
	// los ACL en una fase posterior, no el dominio: aquí solo se declaran
	// los campos y se pueblan (también cuando Permitido=true, porque son el
	// insumo del modo observación). Contrasta deliberadamente con Puntaje
	// (captcha), que sí se publica en ResultadoAutenticacion.
	// PuntajeConfianza: publicar el puntaje de riesgo le diría a un
	// atacante exactamente cuánto le falta para disparar el detector.
	PuntajeRiesgo PuntajeRiesgo
	// NivelRiesgo ver comentario de PuntajeRiesgo (INV-RIES-09).
	NivelRiesgo NivelRiesgo
	// SenalesDeRiesgo ver comentario de PuntajeRiesgo (INV-RIES-09).
	SenalesDeRiesgo []SenalRiesgo
}
