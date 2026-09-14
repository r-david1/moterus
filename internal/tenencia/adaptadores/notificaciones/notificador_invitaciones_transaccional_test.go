package notificaciones_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/r-david1/moterus/internal/plataforma/correo"
	"github.com/r-david1/moterus/internal/tenencia/adaptadores/notificaciones"
	"github.com/r-david1/moterus/internal/tenencia/dominio"
)

// enviadorFalso implementa correo.EnviadorCorreo en memoria, sin red: el
// adaptador bajo prueba (NotificadorInvitacionesTransaccional) es
// agnóstico de proveedor (SMTP, o el que sea en el futuro — ADR 0055), así
// que probar su contenido no necesita un servidor real, solo capturar qué
// le pidió enviar al enviador.
type enviadorFalso struct {
	ultimoMensaje correo.Mensaje
	err           error
}

func (e *enviadorFalso) Enviar(_ context.Context, m correo.Mensaje) error {
	e.ultimoMensaje = m
	return e.err
}

func TestNotificadorInvitacionesTransaccional_ConURLFrontend_IncluyeEnlace(t *testing.T) {
	enviador := &enviadorFalso{}
	n := notificaciones.NuevoNotificadorInvitacionesTransaccional(enviador, "https://app.ejemplo.test")

	destinatario, err := dominio.NuevoCorreoDestinatario("bruno@ejemplo.test")
	if err != nil {
		t.Fatalf("no se pudo construir el correo de prueba: %v", err)
	}
	rol, err := dominio.RolDesde("miembro")
	if err != nil {
		t.Fatalf("no se pudo construir el rol de prueba: %v", err)
	}

	err = n.EnviarInvitacion(context.Background(), destinatario, "Acme", rol, "token-de-invitacion", time.Now().Add(48*time.Hour))
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	if !strings.Contains(enviador.ultimoMensaje.TextoPlano, "https://app.ejemplo.test/invitaciones/aceptar?token=token-de-invitacion") {
		t.Fatalf("el cuerpo no contiene el enlace esperado: %q", enviador.ultimoMensaje.TextoPlano)
	}
	if !strings.Contains(enviador.ultimoMensaje.TextoPlano, "Acme") || !strings.Contains(enviador.ultimoMensaje.TextoPlano, "miembro") {
		t.Fatalf("el cuerpo no menciona la organización/rol: %q", enviador.ultimoMensaje.TextoPlano)
	}
	if enviador.ultimoMensaje.Destinatario != "bruno@ejemplo.test" {
		t.Fatalf("destinatario = %q, esperado bruno@ejemplo.test", enviador.ultimoMensaje.Destinatario)
	}
}

func TestNotificadorInvitacionesTransaccional_SinURLFrontend_MuestraTokenEnClaro(t *testing.T) {
	enviador := &enviadorFalso{}
	n := notificaciones.NuevoNotificadorInvitacionesTransaccional(enviador, "")

	destinatario, err := dominio.NuevoCorreoDestinatario("bruno@ejemplo.test")
	if err != nil {
		t.Fatalf("no se pudo construir el correo de prueba: %v", err)
	}
	rol, err := dominio.RolDesde("administrador")
	if err != nil {
		t.Fatalf("no se pudo construir el rol de prueba: %v", err)
	}

	err = n.EnviarInvitacion(context.Background(), destinatario, "Acme", rol, "token-de-invitacion", time.Now().Add(48*time.Hour))
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	if !strings.Contains(enviador.ultimoMensaje.TextoPlano, "token-de-invitacion") {
		t.Fatalf("el cuerpo no contiene el token en claro: %q", enviador.ultimoMensaje.TextoPlano)
	}
	if strings.Contains(enviador.ultimoMensaje.TextoPlano, "http") {
		t.Fatalf("no debería haber ningún enlace sin URLFrontend configurada: %q", enviador.ultimoMensaje.TextoPlano)
	}
}

func TestNotificadorInvitacionesTransaccional_FalloDeEnvio_devuelveErrorEnvuelto(t *testing.T) {
	enviador := &enviadorFalso{err: errors.New("proveedor caído")}
	n := notificaciones.NuevoNotificadorInvitacionesTransaccional(enviador, "")

	destinatario, err := dominio.NuevoCorreoDestinatario("bruno@ejemplo.test")
	if err != nil {
		t.Fatalf("no se pudo construir el correo de prueba: %v", err)
	}
	rol, err := dominio.RolDesde("miembro")
	if err != nil {
		t.Fatalf("no se pudo construir el rol de prueba: %v", err)
	}

	err = n.EnviarInvitacion(context.Background(), destinatario, "Acme", rol, "token-de-invitacion", time.Now().Add(48*time.Hour))
	if err == nil {
		t.Fatalf("se esperaba un error")
	}
}
