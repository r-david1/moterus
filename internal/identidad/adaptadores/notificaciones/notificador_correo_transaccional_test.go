package notificaciones_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/r-david1/moterus/internal/identidad/adaptadores/notificaciones"
	"github.com/r-david1/moterus/internal/identidad/dominio"
	"github.com/r-david1/moterus/internal/plataforma/correo"
)

// enviadorFalso implementa correo.EnviadorCorreo en memoria, sin red: el
// adaptador bajo prueba (NotificadorCorreoTransaccional) es agnóstico de
// proveedor (SMTP, o el que sea en el futuro — ADR 0055), así que probar
// su contenido no necesita un servidor real, solo capturar qué le pidió
// enviar al enviador.
type enviadorFalso struct {
	ultimoMensaje correo.Mensaje
	err           error
}

func (e *enviadorFalso) Enviar(_ context.Context, m correo.Mensaje) error {
	e.ultimoMensaje = m
	return e.err
}

func TestNotificadorCorreoTransaccional_ConURLFrontend_IncluyeEnlace(t *testing.T) {
	enviador := &enviadorFalso{}
	n := notificaciones.NuevoNotificadorCorreoTransaccional(enviador, "https://app.ejemplo.test")

	correoUsuario, err := dominio.NuevoCorreo("ana@ejemplo.test")
	if err != nil {
		t.Fatalf("no se pudo construir el correo de prueba: %v", err)
	}

	if err := n.EnviarVerificacion(context.Background(), correoUsuario, "token-de-prueba"); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	if !strings.Contains(enviador.ultimoMensaje.TextoPlano, "https://app.ejemplo.test/verificar-correo?token=token-de-prueba") {
		t.Fatalf("el cuerpo no contiene el enlace esperado: %q", enviador.ultimoMensaje.TextoPlano)
	}
	if enviador.ultimoMensaje.Destinatario != "ana@ejemplo.test" {
		t.Fatalf("destinatario = %q, esperado ana@ejemplo.test", enviador.ultimoMensaje.Destinatario)
	}
}

func TestNotificadorCorreoTransaccional_SinURLFrontend_MuestraTokenEnClaro(t *testing.T) {
	enviador := &enviadorFalso{}
	n := notificaciones.NuevoNotificadorCorreoTransaccional(enviador, "")

	correoUsuario, err := dominio.NuevoCorreo("ana@ejemplo.test")
	if err != nil {
		t.Fatalf("no se pudo construir el correo de prueba: %v", err)
	}

	if err := n.EnviarVerificacion(context.Background(), correoUsuario, "token-de-prueba"); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	if !strings.Contains(enviador.ultimoMensaje.TextoPlano, "token-de-prueba") {
		t.Fatalf("el cuerpo no contiene el token en claro: %q", enviador.ultimoMensaje.TextoPlano)
	}
	if strings.Contains(enviador.ultimoMensaje.TextoPlano, "http") {
		t.Fatalf("no debería haber ningún enlace sin URLFrontend configurada: %q", enviador.ultimoMensaje.TextoPlano)
	}
}

func TestNotificadorCorreoTransaccional_FalloDeEnvio_devuelveErrorEnvuelto(t *testing.T) {
	enviador := &enviadorFalso{err: errors.New("proveedor caído")}
	n := notificaciones.NuevoNotificadorCorreoTransaccional(enviador, "")

	correoUsuario, err := dominio.NuevoCorreo("ana@ejemplo.test")
	if err != nil {
		t.Fatalf("no se pudo construir el correo de prueba: %v", err)
	}

	if err := n.EnviarVerificacion(context.Background(), correoUsuario, "token-de-prueba"); err == nil {
		t.Fatalf("se esperaba un error")
	}
}
