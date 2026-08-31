package aplicacion_test

// Fixtures compartidos entre los tests de los tres casos de uso de
// aplicacion. Viven en package aplicacion_test (caja negra): los tests solo
// ejercen la API pública de aplicacion/puertos/dominio, tal como lo haría un
// consumidor real (p. ej. el contexto Acceso).

import (
	"testing"
	"time"

	"github.com/r-david1/moterus/internal/identidad/dominio"
)

// contrasenaFuerteValor es una contraseña que cumple PoliticaContrasena
// (>= 12 caracteres, sin reglas de composición, sin secuencias triviales, no
// contiene el correo/dominio de correoValidoValor) — el mismo fixture que
// usa dominio/politica_contrasena_test.go.
const contrasenaFuerteValor = "correcto caballo batería grapa"

// correoValidoValor es el correo de prueba por defecto usado en los tests
// de aplicacion. No debe cambiarse sin revisar que contrasenaFuerteValor
// sigue sin contenerlo.
const correoValidoValor = "ana@ejemplo.com"

func correoDePrueba(t *testing.T, crudo string) dominio.Correo {
	t.Helper()
	c, err := dominio.NuevoCorreo(crudo)
	if err != nil {
		t.Fatalf("no se pudo construir el correo de prueba %q: %v", crudo, err)
	}
	return c
}

func hashDePrueba(t *testing.T, sufijo string) dominio.HashContrasena {
	t.Helper()
	h, err := dominio.NuevoHashContrasena("$argon2id$v=19$m=65536,t=3,p=1$sal$" + sufijo)
	if err != nil {
		t.Fatalf("no se pudo construir el hash de prueba: %v", err)
	}
	return h
}

func idDePrueba(t *testing.T, uuid string) dominio.IDUsuario {
	t.Helper()
	id, err := dominio.IDUsuarioDesde(uuid)
	if err != nil {
		t.Fatalf("no se pudo construir el IDUsuario de prueba %q: %v", uuid, err)
	}
	return id
}

// idUsuarioValido1/2 son dos UUIDs sintácticamente válidos y distintos, para
// distinguir "el usuario correcto" de "otro usuario" en los tests de
// auditoría condicional.
const (
	idUsuarioValido1 = "123e4567-e89b-12d3-a456-426614174000"
	idUsuarioValido2 = "223e4567-e89b-12d3-a456-426614174001"
)

func origenDePrueba(t *testing.T) dominio.OrigenSolicitud {
	t.Helper()
	o, err := dominio.NuevoOrigenSolicitud("203.0.113.5", "agente-test/1.0", "huella-test", "req-123")
	if err != nil {
		t.Fatalf("no se pudo construir el origen de prueba: %v", err)
	}
	return o
}

// ahoraDePrueba es una hora fija y determinista para todos los tests.
func ahoraDePrueba() time.Time {
	return time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
}

// usuarioPendienteDePrueba construye un Usuario recién registrado
// (pendiente_verificacion) y drena sus eventos pendientes, para que los
// tests que lo usan como fixture de entrada (p. ej. vía
// RepositorioUsuarios.FnBuscarPorCorreo) no arrastren el evento
// UsuarioRegistrado de su propia construcción.
func usuarioPendienteDePrueba(t *testing.T, id dominio.IDUsuario, correo dominio.Correo, hash dominio.HashContrasena) *dominio.Usuario {
	t.Helper()
	u, err := dominio.RegistrarUsuario(id, correo, hash, ahoraDePrueba())
	if err != nil {
		t.Fatalf("no se pudo construir el usuario de prueba: %v", err)
	}
	u.EventosPendientes() // drenar
	return u
}

// usuarioActivoDePrueba construye un Usuario en estado activo (correo ya
// confirmado) con la contraseña hasheada indicada, y drena sus eventos
// pendientes.
func usuarioActivoDePrueba(t *testing.T, id dominio.IDUsuario, correo dominio.Correo, hash dominio.HashContrasena) *dominio.Usuario {
	t.Helper()
	u := usuarioPendienteDePrueba(t, id, correo, hash)
	if err := u.ConfirmarCorreo(ahoraDePrueba()); err != nil {
		t.Fatalf("no se pudo confirmar el correo del usuario de prueba: %v", err)
	}
	u.EventosPendientes() // drenar
	return u
}

// usuarioSuspendidoDePrueba construye un Usuario activo y lo suspende.
func usuarioSuspendidoDePrueba(t *testing.T, id dominio.IDUsuario, correo dominio.Correo, hash dominio.HashContrasena) *dominio.Usuario {
	t.Helper()
	u := usuarioActivoDePrueba(t, id, correo, hash)
	motivo, err := dominio.NuevoMotivoCambioEstado("comportamiento sospechoso reportado por soporte")
	if err != nil {
		t.Fatalf("no se pudo construir el motivo de prueba: %v", err)
	}
	if err := u.Suspender(motivo, ahoraDePrueba()); err != nil {
		t.Fatalf("no se pudo suspender el usuario de prueba: %v", err)
	}
	u.EventosPendientes()
	return u
}

// usuarioBloqueadoDePrueba construye un Usuario pendiente de verificación y
// lo bloquea.
func usuarioBloqueadoDePrueba(t *testing.T, id dominio.IDUsuario, correo dominio.Correo, hash dominio.HashContrasena) *dominio.Usuario {
	t.Helper()
	u := usuarioPendienteDePrueba(t, id, correo, hash)
	motivo, err := dominio.NuevoMotivoCambioEstado("fraude confirmado por el equipo de riesgo")
	if err != nil {
		t.Fatalf("no se pudo construir el motivo de prueba: %v", err)
	}
	if err := u.Bloquear(motivo, ahoraDePrueba()); err != nil {
		t.Fatalf("no se pudo bloquear el usuario de prueba: %v", err)
	}
	u.EventosPendientes()
	return u
}

// usuarioConMFADePrueba construye, vía dominio.Reconstituir, un Usuario
// activo con tieneMFA=true. Es la única forma de obtener un usuario con MFA
// habilitado sin el agregado FactorMFA (fase 2, fuera de alcance): el
// dominio no expone ningún setter de negocio para activar tieneMFA sin
// confirmar un factor.
func usuarioConMFADePrueba(t *testing.T, id dominio.IDUsuario, correo dominio.Correo, hash dominio.HashContrasena) *dominio.Usuario {
	t.Helper()
	credencial, err := dominio.NuevaCredencial(hash, ahoraDePrueba())
	if err != nil {
		t.Fatalf("no se pudo construir la credencial de prueba: %v", err)
	}
	return dominio.Reconstituir(id, correo, credencial, dominio.EstadoActivo, true, ahoraDePrueba(), ahoraDePrueba(), nil)
}
