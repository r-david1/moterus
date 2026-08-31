// Package dominio contiene las reglas puras (sin E/S) del bounded context
// Confianza: la política de umbrales de rate limiting por IP/cuenta y la
// decisión de riesgo resultante (permitir / exigir captcha / exigir
// step-up / bloquear). Fingerprinting y scoring de comportamiento
// avanzado siguen pendientes de diseño e implementación (ver ADR 0018 para
// el alcance de este primer hito: solo rate limiting + captcha invisible).
package dominio
