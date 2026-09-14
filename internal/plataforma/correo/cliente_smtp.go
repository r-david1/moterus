package correo

import (
	"context"
	"crypto/tls"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

// timeoutSMTPPorDefecto: un correo es un efecto secundario best-effort,
// nunca debe colgar el caso de uso que lo dispara.
const timeoutSMTPPorDefecto = 5 * time.Second

// ClienteSMTP implementa EnviadorCorreo contra cualquier servidor SMTP
// estándar con STARTTLS + AUTH (Gmail, Google Workspace, un proveedor
// propio) — elegido sobre un proveedor de API transaccional (Resend,
// evaluado y descartado: ver ADR 0055) porque no exige verificar un
// dominio propio, que este proyecto no tiene: una cuenta de correo
// existente (p. ej. Gmail con una "contraseña de aplicación") ya puede
// mandar a cualquier destinatario real sin ese paso.
//
// Va con stdlib (net/smtp + crypto/tls), sin biblioteca de terceros —
// mismo criterio que turnstile.VerificadorCaptcha: la negociación
// SMTP+STARTTLS+AUTH LOGIN que exige un proveedor como Gmail es un puñado
// de líneas con la librería estándar, no justifica una dependencia nueva.
type ClienteSMTP struct {
	host       string
	puerto     int
	usuario    string
	contrasena string
	remitente  string
	timeout    time.Duration
}

var _ EnviadorCorreo = (*ClienteSMTP)(nil)

// OpcionClienteSMTP configura ClienteSMTP en su construcción.
type OpcionClienteSMTP func(*ClienteSMTP)

// ConTimeoutSMTP sobreescribe timeoutSMTPPorDefecto (tests).
func ConTimeoutSMTP(d time.Duration) OpcionClienteSMTP {
	return func(c *ClienteSMTP) { c.timeout = d }
}

// NuevoClienteSMTP construye el cliente. remitente es la dirección "From"
// (formato "Nombre <correo@dominio>" o solo "correo@dominio") — con la
// mayoría de proveedores (Gmail incluido) debe coincidir con `usuario` o
// un alias autorizado de esa cuenta, o el servidor rechaza el envío.
func NuevoClienteSMTP(host string, puerto int, usuario, contrasena, remitente string, opciones ...OpcionClienteSMTP) *ClienteSMTP {
	c := &ClienteSMTP{
		host:       host,
		puerto:     puerto,
		usuario:    usuario,
		contrasena: contrasena,
		remitente:  remitente,
		timeout:    timeoutSMTPPorDefecto,
	}
	for _, opcion := range opciones {
		opcion(c)
	}
	return c
}

// Enviar abre una conexión STARTTLS al servidor configurado, autentica
// con AUTH LOGIN/PLAIN (smtp.PlainAuth, que net/smtp usa automáticamente
// si el servidor lo anuncia) y manda el mensaje como texto plano, o
// multipart/alternative si m.HTML no está vacío.
func (c *ClienteSMTP) Enviar(ctx context.Context, m Mensaje) error {
	if c.host == "" {
		return fmt.Errorf("plataforma/correo: SMTP_HOST no configurado")
	}
	if c.remitente == "" {
		return fmt.Errorf("plataforma/correo: remitente (SMTP_REMITENTE) no configurado")
	}
	if m.Destinatario == "" {
		return fmt.Errorf("plataforma/correo: destinatario vacío")
	}

	// net.JoinHostPort, no fmt.Sprintf("%s:%d", ...): un host IPv6 literal
	// necesita corchetes ("[::1]:587") que el formato manual no agrega.
	direccion := net.JoinHostPort(c.host, strconv.Itoa(c.puerto))

	// El timeout aplica a la conexión TCP inicial; una vez establecida, la
	// negociación SMTP en sí (STARTTLS/AUTH/DATA) es rápida y no vale la
	// pena instrumentar cada paso por separado para un efecto secundario
	// best-effort.
	conexionCruda, err := net.DialTimeout("tcp", direccion, c.timeout)
	if err != nil {
		return fmt.Errorf("plataforma/correo: no se pudo conectar a %s: %w", direccion, err)
	}
	defer func() { _ = conexionCruda.Close() }()

	cliente, err := smtp.NewClient(conexionCruda, c.host)
	if err != nil {
		return fmt.Errorf("plataforma/correo: no se pudo iniciar el cliente SMTP: %w", err)
	}
	defer func() { _ = cliente.Close() }()

	if ok, _ := cliente.Extension("STARTTLS"); ok {
		if err := cliente.StartTLS(&tls.Config{ServerName: c.host}); err != nil {
			return fmt.Errorf("plataforma/correo: STARTTLS falló: %w", err)
		}
	}

	if c.usuario != "" {
		auth := smtp.PlainAuth("", c.usuario, c.contrasena, c.host)
		if ok, _ := cliente.Extension("AUTH"); ok {
			if err := cliente.Auth(auth); err != nil {
				return fmt.Errorf("plataforma/correo: autenticación SMTP falló: %w", err)
			}
		}
	}

	remitenteCorreo := direccionCorreoDesde(c.remitente)
	if err := cliente.Mail(remitenteCorreo); err != nil {
		return fmt.Errorf("plataforma/correo: MAIL FROM falló: %w", err)
	}
	if err := cliente.Rcpt(m.Destinatario); err != nil {
		return fmt.Errorf("plataforma/correo: RCPT TO falló: %w", err)
	}

	w, err := cliente.Data()
	if err != nil {
		return fmt.Errorf("plataforma/correo: DATA falló: %w", err)
	}
	if _, err := w.Write(construirMensajeMIME(c.remitente, m)); err != nil {
		_ = w.Close()
		return fmt.Errorf("plataforma/correo: no se pudo escribir el cuerpo del mensaje: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("plataforma/correo: no se pudo cerrar el mensaje: %w", err)
	}

	return cliente.Quit()
}

// direccionCorreoDesde extrae la dirección pura de un remitente en
// formato "Nombre <correo@dominio>" — MAIL FROM exige solo la dirección,
// sin el nombre para mostrar.
func direccionCorreoDesde(remitente string) string {
	inicio := strings.Index(remitente, "<")
	fin := strings.Index(remitente, ">")
	if inicio >= 0 && fin > inicio {
		return remitente[inicio+1 : fin]
	}
	return remitente
}

// construirMensajeMIME arma las cabeceras + cuerpo RFC 5322 mínimos. Sin
// HTML: text/plain simple. Con HTML: multipart/alternative con ambas
// partes, para que un cliente de correo sin soporte HTML caiga a
// TextoPlano.
func construirMensajeMIME(remitente string, m Mensaje) []byte {
	var b strings.Builder
	asunto := mime.QEncoding.Encode("UTF-8", m.Asunto)

	fmt.Fprintf(&b, "From: %s\r\n", remitente)
	fmt.Fprintf(&b, "To: %s\r\n", m.Destinatario)
	fmt.Fprintf(&b, "Subject: %s\r\n", asunto)
	fmt.Fprintf(&b, "MIME-Version: 1.0\r\n")

	if m.HTML == "" {
		fmt.Fprintf(&b, "Content-Type: text/plain; charset=\"UTF-8\"\r\n\r\n")
		b.WriteString(m.TextoPlano)
		return []byte(b.String())
	}

	const limite = "moterus-correo-boundary"
	fmt.Fprintf(&b, "Content-Type: multipart/alternative; boundary=\"%s\"\r\n\r\n", limite)
	fmt.Fprintf(&b, "--%s\r\n", limite)
	fmt.Fprintf(&b, "Content-Type: text/plain; charset=\"UTF-8\"\r\n\r\n")
	b.WriteString(m.TextoPlano)
	fmt.Fprintf(&b, "\r\n--%s\r\n", limite)
	fmt.Fprintf(&b, "Content-Type: text/html; charset=\"UTF-8\"\r\n\r\n")
	b.WriteString(m.HTML)
	fmt.Fprintf(&b, "\r\n--%s--\r\n", limite)
	return []byte(b.String())
}
