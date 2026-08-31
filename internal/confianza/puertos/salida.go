package puertos

import (
	"context"
	"time"

	"github.com/r-david1/moterus/internal/confianza/dominio"
)

// LimitadorTasa es el puerto de salida del limitador de tasa. La
// implementación real (adaptadores/redis) usa Redis con INCR+EXPIRE: un
// contador de ventana fija (fixed window), no una ventana deslizante
// exacta (sliding log) — ver ADR 0018 para la justificación de por qué esa
// aproximación es suficiente aquí. Cualquier adaptador que satisfaga este
// contrato (incluido uno en memoria para tests) es intercambiable sin
// tocar aplicacion.
type LimitadorTasa interface {
	// Permitir incrementa atómicamente el contador de clave y decide si la
	// solicitud actual cabe dentro de umbral.Limite en umbral.Ventana.
	// permitido=false cuando el contador (ya incrementado) supera el
	// límite. reintentarEn es el tiempo restante hasta que la clave
	// expire — cuando el límite ya estaba superado, la implementación
	// real extiende ese TTL de forma exponencial (cooldown creciente,
	// tope documentado en el adaptador) en vez de dejarlo fijo.
	Permitir(ctx context.Context, clave string, umbral dominio.Umbral) (permitido bool, restantes int, reintentarEn time.Duration, err error)
	// Reiniciar borra el contador de clave. Se usa para no penalizar una
	// cuenta tras un intento exitoso (p. ej. login correcto tras un solo
	// error de tipeo previo).
	Reiniciar(ctx context.Context, clave string) error
}

// VerificadorCaptcha es el puerto de salida agnóstico de proveedor
// (Cloudflare Turnstile por defecto, ADR 0003) para validar un token de
// captcha invisible server-side. Firma equivalente a la especificada en el
// encargo original ("CaptchaVerifier.Verify"), renombrada al español por
// ADR 0007 (identificadores de puertos van en español).
type VerificadorCaptcha interface {
	// Verificar valida token contra el proveedor configurado y devuelve un
	// puntaje 0.0-1.0 (accion e ip se reenvían al proveedor para
	// validación adicional, cuando la soporta). Un token vacío es un error
	// de uso del llamador, no de este puerto: EvaluarTrustSignalCasoDeUso
	// nunca debe llamarlo con token="".
	Verificar(ctx context.Context, token string, accion string, ip string) (puntaje float64, err error)
}
