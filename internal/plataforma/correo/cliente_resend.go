// Package correo implementa el envío de correo transaccional real vía la
// API HTTP de Resend (https://resend.com/docs/api-reference/emails/send-email,
// ADR 0054). Vive en plataforma, no en un bounded context, porque es
// kernel técnico puro — "mandar este asunto/cuerpo a esta dirección" no
// tiene ningún tipo de negocio involucrado, igual criterio que
// plataforma/cache para Redis o plataforma/bd para Postgres: "si dos
// contextos necesitan la misma pieza de infraestructura, eso no es una
// señal de que la frontera está mal trazada, es kernel técnico
// compartido" (§5.2 del diseño de Identidad).
//
// Cada bounded context que necesite enviar un correo real (Identidad:
// verificación; Tenencia: invitaciones) construye su propio mensaje
// (asunto, cuerpo, a partir de sus propios tipos de dominio) en su propio
// adaptador de notificaciones, y usa este cliente solo para el transporte
// — ningún tipo de dominio de Identidad o Tenencia aparece en este
// paquete.
package correo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// urlEnvioPorDefecto es el endpoint real de envío de Resend.
const urlEnvioPorDefecto = "https://api.resend.com/emails"

// timeoutPorDefecto: corto a propósito. Enviar un correo es siempre un
// efecto secundario del camino feliz de un caso de uso (registrar
// usuario, invitar a un miembro) — nunca debe convertir una API externa
// lenta en una petición HTTP colgada; el llamador decide qué hacer si
// Enviar devuelve error (ver NotificadorCorreoResend/
// NotificadorInvitacionesResend: ninguno de los dos aborta la operación
// de negocio por un fallo de envío).
const timeoutPorDefecto = 5 * time.Second

// Mensaje es el contenido de un correo a enviar, agnóstico de quién lo
// pide. TextoPlano es obligatorio; HTML es opcional (Resend acepta uno,
// otro, o ambos — un cliente de correo sin soporte HTML cae a
// TextoPlano).
type Mensaje struct {
	Destinatario string
	Asunto       string
	TextoPlano   string
	HTML         string
}

// peticionResend es el cuerpo JSON que exige la API de Resend.
type peticionResend struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	Text    string   `json:"text,omitempty"`
	HTML    string   `json:"html,omitempty"`
}

// respuestaErrorResend es el cuerpo JSON que Resend devuelve en errores
// (4xx/5xx) — se usa solo para enriquecer el mensaje de error, nunca para
// decidir el flujo.
type respuestaErrorResend struct {
	Message string `json:"message"`
	Name    string `json:"name"`
}

// ClienteResend implementa el transporte de correo vía la API HTTP de
// Resend. No implementa ningún puerto de ningún bounded context — cada
// contexto envuelve este cliente detrás de su propio puerto
// (puertos.NotificadorCorreo, puertos.NotificadorInvitaciones).
type ClienteResend struct {
	apiKey    string
	remitente string
	url       string
	cliente   *http.Client
}

// OpcionClienteResend configura ClienteResend en su construcción.
type OpcionClienteResend func(*ClienteResend)

// ConClienteHTTP inyecta un *http.Client propio (tests, tuning de
// timeout).
func ConClienteHTTP(cliente *http.Client) OpcionClienteResend {
	return func(c *ClienteResend) { c.cliente = cliente }
}

// ConURLEnvio inyecta una URL de envío propia — pensado para tests con un
// httptest.Server local, nunca hace falta en producción.
func ConURLEnvio(url string) OpcionClienteResend {
	return func(c *ClienteResend) {
		if url != "" {
			c.url = url
		}
	}
}

// NuevoClienteResend construye el cliente. apiKey y remitente vacíos
// producen un cliente que siempre falla al enviar (ver Enviar) — la
// decisión de qué hacer ante eso (caer a un stub log-only en desarrollo,
// fallar el arranque en producción) vive en cmd/api/main.go, igual
// criterio que turnstile.NuevoVerificadorCaptcha: este paquete no conoce
// el concepto de "entorno", solo sabe transportar o fallar.
func NuevoClienteResend(apiKey, remitente string, opciones ...OpcionClienteResend) *ClienteResend {
	c := &ClienteResend{
		apiKey:    apiKey,
		remitente: remitente,
		url:       urlEnvioPorDefecto,
		cliente:   &http.Client{Timeout: timeoutPorDefecto},
	}
	for _, opcion := range opciones {
		opcion(c)
	}
	return c
}

// Enviar hace POST a la API de Resend. Devuelve error si apiKey o
// remitente están vacíos, si la petición falla, o si Resend responde con
// un status distinto de 2xx.
func (c *ClienteResend) Enviar(ctx context.Context, m Mensaje) error {
	if c.apiKey == "" {
		return fmt.Errorf("plataforma/correo: RESEND_API_KEY no configurada")
	}
	if c.remitente == "" {
		return fmt.Errorf("plataforma/correo: remitente (RESEND_REMITENTE) no configurado")
	}
	if m.Destinatario == "" {
		return fmt.Errorf("plataforma/correo: destinatario vacío")
	}

	cuerpo, err := json.Marshal(peticionResend{
		From:    c.remitente,
		To:      []string{m.Destinatario},
		Subject: m.Asunto,
		Text:    m.TextoPlano,
		HTML:    m.HTML,
	})
	if err != nil {
		return fmt.Errorf("plataforma/correo: no se pudo serializar el mensaje: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(cuerpo))
	if err != nil {
		return fmt.Errorf("plataforma/correo: no se pudo construir la solicitud: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.cliente.Do(req)
	if err != nil {
		return fmt.Errorf("plataforma/correo: Resend no disponible: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		cuerpoResp, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		var errResend respuestaErrorResend
		if err := json.Unmarshal(cuerpoResp, &errResend); err == nil && errResend.Message != "" {
			return fmt.Errorf("plataforma/correo: Resend respondió %d (%s): %s", resp.StatusCode, errResend.Name, errResend.Message)
		}
		return fmt.Errorf("plataforma/correo: Resend respondió %d", resp.StatusCode)
	}

	return nil
}
