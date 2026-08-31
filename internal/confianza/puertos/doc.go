// Package puertos define los contratos de entrada/salida del bounded
// context Confianza: EvaluadorDeRiesgo (entrada, consumido por el ACL de
// Identidad y por el guardián de perímetro HTTP de reenvíos) y
// LimitadorTasa/VerificadorCaptcha (salida, implementados por los
// adaptadores redis/ y turnstile/). Ver ADR 0018.
package puertos
