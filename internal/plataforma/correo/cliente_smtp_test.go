package correo_test

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/r-david1/moterus/internal/plataforma/correo"
)

// servidorSMTPFalso levanta un servidor SMTP mínimo (sin STARTTLS/AUTH:
// alcanza para probar la conversación MAIL/RCPT/DATA que ClienteSMTP
// negocia — la propia negociación STARTTLS/AUTH la prueba net/smtp en la
// librería estándar, no hace falta reproducirla acá) y devuelve la
// dirección donde escucha más un canal por el que llega el mensaje crudo
// recibido en el DATA.
func servidorSMTPFalso(t *testing.T) (direccion string, mensajes <-chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("no se pudo abrir el listener de prueba: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	canal := make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		manejarConexionSMTPFalsa(conn, canal)
	}()

	return ln.Addr().String(), canal
}

func manejarConexionSMTPFalsa(conn net.Conn, canal chan<- string) {
	lector := bufio.NewReader(conn)
	escribir := func(linea string) { _, _ = conn.Write([]byte(linea + "\r\n")) }

	escribir("220 servidor-de-prueba listo")
	var enDatos bool
	var datos strings.Builder

	for {
		linea, err := lector.ReadString('\n')
		if err != nil {
			return
		}
		linea = strings.TrimRight(linea, "\r\n")

		if enDatos {
			if linea == "." {
				canal <- datos.String()
				enDatos = false
				escribir("250 OK: mensaje aceptado")
				continue
			}
			datos.WriteString(linea + "\n")
			continue
		}

		switch {
		case strings.HasPrefix(strings.ToUpper(linea), "EHLO"):
			escribir("250-servidor-de-prueba saluda")
			escribir("250 8BITMIME")
		case strings.HasPrefix(strings.ToUpper(linea), "MAIL FROM"):
			escribir("250 OK")
		case strings.HasPrefix(strings.ToUpper(linea), "RCPT TO"):
			escribir("250 OK")
		case strings.ToUpper(linea) == "DATA":
			enDatos = true
			escribir("354 enviá el mensaje, terminá con <CRLF>.<CRLF>")
		case strings.ToUpper(linea) == "QUIT":
			escribir("221 chau")
			return
		default:
			escribir("500 comando no reconocido en la prueba")
		}
	}
}

func direccionYPuerto(t *testing.T, direccion string) (string, int) {
	t.Helper()
	host, puertoTexto, err := net.SplitHostPort(direccion)
	if err != nil {
		t.Fatalf("no se pudo separar host:puerto de %q: %v", direccion, err)
	}
	var puerto int
	if _, err := fmt.Sscanf(puertoTexto, "%d", &puerto); err != nil {
		t.Fatalf("puerto no numérico %q: %v", puertoTexto, err)
	}
	return host, puerto
}

func TestClienteSMTP_Enviar_mensajeTextoPlano_llegaCorrecto(t *testing.T) {
	direccion, mensajes := servidorSMTPFalso(t)
	host, puerto := direccionYPuerto(t, direccion)

	c := correo.NuevoClienteSMTP(host, puerto, "", "", "Moterus <no-reply@ejemplo.test>", correo.ConTimeoutSMTP(2*time.Second))

	err := c.Enviar(context.Background(), correo.Mensaje{
		Destinatario: "ana@ejemplo.test",
		Asunto:       "Verificá tu correo",
		TextoPlano:   "Tu token es: abc123",
	})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	select {
	case recibido := <-mensajes:
		if !strings.Contains(recibido, "To: ana@ejemplo.test") {
			t.Fatalf("el mensaje no tiene el destinatario esperado: %q", recibido)
		}
		if !strings.Contains(recibido, "Tu token es: abc123") {
			t.Fatalf("el mensaje no tiene el cuerpo esperado: %q", recibido)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no se recibió ningún mensaje en el servidor de prueba")
	}
}

func TestClienteSMTP_Enviar_conHTML_generaMultipart(t *testing.T) {
	direccion, mensajes := servidorSMTPFalso(t)
	host, puerto := direccionYPuerto(t, direccion)

	c := correo.NuevoClienteSMTP(host, puerto, "", "", "Moterus <no-reply@ejemplo.test>", correo.ConTimeoutSMTP(2*time.Second))

	err := c.Enviar(context.Background(), correo.Mensaje{
		Destinatario: "ana@ejemplo.test",
		Asunto:       "x",
		TextoPlano:   "version texto",
		HTML:         "<p>version html</p>",
	})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	select {
	case recibido := <-mensajes:
		if !strings.Contains(recibido, "multipart/alternative") {
			t.Fatalf("se esperaba multipart/alternative con HTML presente: %q", recibido)
		}
		if !strings.Contains(recibido, "version texto") || !strings.Contains(recibido, "version html") {
			t.Fatalf("faltó alguna de las dos partes del mensaje: %q", recibido)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no se recibió ningún mensaje en el servidor de prueba")
	}
}

func TestClienteSMTP_sinHost_devuelveErrorSinConectar(t *testing.T) {
	c := correo.NuevoClienteSMTP("", 587, "", "", "Moterus <no-reply@ejemplo.test>")
	err := c.Enviar(context.Background(), correo.Mensaje{Destinatario: "ana@ejemplo.test", Asunto: "x", TextoPlano: "y"})
	if err == nil {
		t.Fatalf("se esperaba un error (sin host)")
	}
}

func TestClienteSMTP_sinRemitente_devuelveErrorSinConectar(t *testing.T) {
	c := correo.NuevoClienteSMTP("smtp.ejemplo.test", 587, "", "", "")
	err := c.Enviar(context.Background(), correo.Mensaje{Destinatario: "ana@ejemplo.test", Asunto: "x", TextoPlano: "y"})
	if err == nil {
		t.Fatalf("se esperaba un error (sin remitente)")
	}
}

func TestClienteSMTP_sinDestinatario_devuelveErrorSinConectar(t *testing.T) {
	c := correo.NuevoClienteSMTP("smtp.ejemplo.test", 587, "", "", "Moterus <no-reply@ejemplo.test>")
	err := c.Enviar(context.Background(), correo.Mensaje{Asunto: "x", TextoPlano: "y"})
	if err == nil {
		t.Fatalf("se esperaba un error (sin destinatario)")
	}
}
