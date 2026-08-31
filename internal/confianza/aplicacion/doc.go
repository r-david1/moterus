// Package aplicacion contiene el caso de uso del bounded context
// Confianza: EvaluarTrustSignalCasoDeUso, que orquesta rate limiting por
// IP y por cuenta (vía puertos.LimitadorTasa) y verificación de captcha
// invisible (vía puertos.VerificadorCaptcha) para producir una
// dominio.Decision. Ver ADR 0018.
package aplicacion
