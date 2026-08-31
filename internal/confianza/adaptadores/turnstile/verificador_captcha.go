// Package turnstile implementa puertos.VerificadorCaptcha contra la API de
// verificación server-side de Cloudflare Turnstile (proveedor por defecto,
// ADR 0003/0018). El puerto es agnóstico de proveedor a propósito: cambiar
// a reCAPTCHA v3 (alternativa soportada, ADR 0003) es escribir un
// adaptador nuevo con la misma forma, sin tocar ningún caso de uso.
package turnstile

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/r-david1/moterus/internal/confianza/puertos"
)

// urlVerificacionPorDefecto es el endpoint real de siteverify de
// Cloudflare Turnstile.
const urlVerificacionPorDefecto = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

// timeoutPorDefecto: corto a propósito, igual criterio que
// cripto.VerificadorHIBP en Identidad — este puerto se evalúa fail-closed
// en EvaluarTrustSignalCasoDeUso (un error se trata como puntaje 0), así
// que una llamada lenta no debe retrasar login/registro más de lo
// estrictamente necesario ni convertirse en una vía de denegación de
// servicio contra la propia disponibilidad de Cloudflare.
const timeoutPorDefecto = 3 * time.Second

// respuestaSiteverify es el cuerpo JSON que devuelve Cloudflare. Turnstile
// (a diferencia de reCAPTCHA v3) no expone un score continuo: solo
// success/error-codes — ver VerificadorCaptcha.Verificar para cómo se
// traduce a un puntaje 0.0/1.0.
type respuestaSiteverify struct {
	Success     bool     `json:"success"`
	ErrorCodes  []string `json:"error-codes"`
	Action      string   `json:"action"`
	ChallengeTS string   `json:"challenge_ts"`
	Hostname    string   `json:"hostname"`
}

// VerificadorCaptcha implementa puertos.VerificadorCaptcha contra
// Cloudflare Turnstile.
type VerificadorCaptcha struct {
	secretKey string
	url       string
	cliente   *http.Client
	// entornoInseguroPermitido controla el comportamiento cuando
	// secretKey está vacía (sin credenciales configuradas): en ese caso
	// NuevoVerificadorCaptcha ya emitió el WARN de arranque, y Verificar
	// decide entre fail-open (dev) y fail-closed (producción) según este
	// flag — ver NuevoVerificadorCaptcha.
	entornoInseguroPermitido bool
}

var _ puertos.VerificadorCaptcha = (*VerificadorCaptcha)(nil)

// OpcionVerificadorCaptcha configura VerificadorCaptcha en su
// construcción.
type OpcionVerificadorCaptcha func(*VerificadorCaptcha)

// ConClienteHTTP inyecta un *http.Client propio (tests, tuning de
// timeout).
func ConClienteHTTP(cliente *http.Client) OpcionVerificadorCaptcha {
	return func(v *VerificadorCaptcha) { v.cliente = cliente }
}

// ConURLVerificacion inyecta una URL de verificación propia — pensado
// para tests con un httptest.Server local, nunca hace falta en
// producción (ver config.TurnstileVerifyURL).
func ConURLVerificacion(url string) OpcionVerificadorCaptcha {
	return func(v *VerificadorCaptcha) {
		if url != "" {
			v.url = url
		}
	}
}

// NuevoVerificadorCaptcha construye el adaptador. secretKey vacía es una
// configuración explícitamente soportada (desarrollo sin credenciales
// reales de Turnstile) — nunca falla al arrancar, pero nunca en silencio:
//
//   - entornoApp == "production" (o "prod"): fail-closed. Se emite un
//     ERROR de arranque y Verificar siempre devuelve puntaje 0 con un
//     error explícito, sin llamar a la red — un despliegue de producción
//     sin la llave secreta queda con el captcha "cerrado" (deniega todo lo
//     que dependa de él) en vez de "abierto" (mismo patrón de "modo
//     inseguro posible pero nunca silencioso" que ADR 0017 y
//     NotificadorCorreoLog, pero aquí el modo inseguro NO se permite en
//     producción porque la superficie de abuso — creación masiva de
//     cuentas, fuerza bruta — es demasiado alta para tolerarlo).
//   - cualquier otro entorno (development, staging, test): fail-open. Se
//     emite un WARN de arranque y Verificar siempre devuelve puntaje 1.0
//     sin llamar a la red, para no bloquear el desarrollo local sin
//     cuenta de Cloudflare.
func NuevoVerificadorCaptcha(secretKey, urlVerificacion, entornoApp string, opciones ...OpcionVerificadorCaptcha) *VerificadorCaptcha {
	v := &VerificadorCaptcha{
		secretKey: secretKey,
		url:       urlVerificacionPorDefecto,
		cliente:   &http.Client{Timeout: timeoutPorDefecto},
	}
	if urlVerificacion != "" {
		v.url = urlVerificacion
	}
	for _, opcion := range opciones {
		opcion(v)
	}

	if secretKey == "" {
		if esEntornoProduccion(entornoApp) {
			v.entornoInseguroPermitido = false
			slog.Error("confianza/adaptadores/turnstile: TURNSTILE_SECRET_KEY no está definida en un entorno de producción — " +
				"VerificadorCaptcha queda FAIL-CLOSED: todo intento que dependa de captcha se deniega. " +
				"Definir TURNSTILE_SECRET_KEY antes de desplegar (ADR 0003/0018).")
		} else {
			v.entornoInseguroPermitido = true
			slog.Warn("confianza/adaptadores/turnstile: TURNSTILE_SECRET_KEY no está definida — "+
				"VerificadorCaptcha en modo fail-open (puntaje 1.0 sin verificar nada). "+
				"Aceptable en desarrollo local; NO USAR EN PRODUCCIÓN sin definir la llave (ADR 0003/0018).",
				"entorno_app", entornoApp)
		}
	}

	return v
}

func esEntornoProduccion(entornoApp string) bool {
	e := strings.ToLower(strings.TrimSpace(entornoApp))
	return e == "production" || e == "prod"
}

// Verificar llama a la API de siteverify de Cloudflare. Devuelve puntaje
// 1.0 si success==true, 0.0 en cualquier otro caso (Turnstile no expone
// score continuo — ver comentario del tipo respuestaSiteverify).
func (v *VerificadorCaptcha) Verificar(ctx context.Context, token, accion, ip string) (float64, error) {
	if v.secretKey == "" {
		if v.entornoInseguroPermitido {
			return 1.0, nil
		}
		return 0, fmt.Errorf("confianza/adaptadores/turnstile: sin TURNSTILE_SECRET_KEY configurada en un entorno de producción (fail-closed)")
	}
	if token == "" {
		return 0, fmt.Errorf("confianza/adaptadores/turnstile: token vacío")
	}

	valores := url.Values{}
	valores.Set("secret", v.secretKey)
	valores.Set("response", token)
	if ip != "" {
		valores.Set("remoteip", ip)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.url, strings.NewReader(valores.Encode()))
	if err != nil {
		return 0, fmt.Errorf("confianza/adaptadores/turnstile: no se pudo construir la solicitud: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := v.cliente.Do(req)
	if err != nil {
		return 0, fmt.Errorf("confianza/adaptadores/turnstile: siteverify no disponible: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("confianza/adaptadores/turnstile: siteverify respondió %d", resp.StatusCode)
	}

	var cuerpo respuestaSiteverify
	if err := json.NewDecoder(resp.Body).Decode(&cuerpo); err != nil {
		return 0, fmt.Errorf("confianza/adaptadores/turnstile: respuesta con formato inesperado: %w", err)
	}

	if !cuerpo.Success {
		slog.WarnContext(ctx, "confianza/adaptadores/turnstile: verificación rechazada por Cloudflare",
			"error_codes", cuerpo.ErrorCodes, "accion", accion)
		return 0, nil
	}

	// Validación adicional opcional: si el widget del cliente fijó
	// data-action y Cloudflare la devuelve, debe coincidir con la acción
	// que este llamador espera. Un action vacío (widgets que no lo
	// configuran) no es motivo de rechazo — es un campo opcional en la
	// API de Cloudflare, no una garantía.
	if cuerpo.Action != "" && accion != "" && cuerpo.Action != accion {
		slog.WarnContext(ctx, "confianza/adaptadores/turnstile: la acción del token no coincide con la esperada",
			"accion_token", cuerpo.Action, "accion_esperada", accion)
		return 0, nil
	}

	return 1.0, nil
}
