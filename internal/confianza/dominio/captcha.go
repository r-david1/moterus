package dominio

// UmbralPuntajeAceptable es el puntaje mínimo (0.0-1.0) por debajo del cual
// un token de captcha verificado se trata como "no humano" y la solicitud
// se deniega. Cloudflare Turnstile (proveedor por defecto, ADR 0003) no
// expone un score continuo como reCAPTCHA v3: su API de verificación
// solo informa éxito/fallo binario, así que en la práctica el adaptador
// turnstile.VerificadorCaptcha devuelve 1.0 o 0.0 — el umbral funciona
// igual (0.0 < 0.5 deniega) pero nunca produce un valor intermedio salvo
// que se cambie a un proveedor con score real (reCAPTCHA v3, ADR 0003).
const UmbralPuntajeAceptable = 0.5

// UmbralPuntajeSospechoso marca la banda entre "aceptable" y "sospechoso
// pero no bloqueante": con un proveedor de score continuo, un puntaje en
// [UmbralPuntajeSospechoso, UmbralPuntajeAceptable) no bloquea el intento
// pero sí exige step-up (segundo factor) en vez de confiar ciegamente en
// la contraseña. Con Turnstile (binario) esta banda nunca se activa en la
// práctica — queda lista para cuando se swapee a un proveedor de score
// continuo sin tocar la lógica de EvaluarTrustSignalCasoDeUso.
const UmbralPuntajeSospechoso = 0.3

// EvaluarPuntajeCaptcha clasifica un puntaje ya verificado. Función pura,
// sin E/S: la verificación en sí (llamar al proveedor) es responsabilidad
// del puerto VerificadorCaptcha.
func EvaluarPuntajeCaptcha(puntaje float64) (aceptable, sospechoso bool) {
	aceptable = puntaje >= UmbralPuntajeAceptable
	sospechoso = !aceptable && puntaje >= UmbralPuntajeSospechoso
	return aceptable, sospechoso
}
