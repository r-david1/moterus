package dominio

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

// TestINV_ID_04_EventosNuncaTransportanSecretos recorre por reflexión los 12
// eventos de dominio y verifica que ningún campo es de tipo ContrasenaPlana
// ni HashContrasena, y que ningún nombre de campo sugiere transportar un
// token. Esto es lo que exige la sección 1.6 del diseño: "Ningún evento
// transporta ContrasenaPlana, HashContrasena, códigos OTP ni tokens". Incluye
// VerificacionCorreoFallida (sección 3.4), que a pesar de estar ligado al
// mecanismo de tokens de verificación no transporta el token en sí.
func TestINV_ID_04_EventosNuncaTransportanSecretos(t *testing.T) {
	ahora := time.Now()
	id, _ := IDUsuarioDesde("018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6d")
	idFactor, _ := IDFactorMFADesde("018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c99")
	correo, _ := NuevoCorreo("usuario@ejemplo.com")

	eventos := []EventoDominio{
		NuevoUsuarioRegistrado(id, correo, ahora),
		NuevoRegistroRechazado("usuario@ejemplo.com", "motivo", ahora),
		NuevoAutenticacionExitosa(id, correo, ahora),
		NuevoAutenticacionFallida(id.String(), "usuario@ejemplo.com", "motivo", ahora),
		NuevoAutenticacionDenegada("usuario@ejemplo.com", "motivo", ahora),
		NuevoSegundoFactorRequerido(id, "motivo", ahora),
		NuevoContrasenaCambiada(id, ahora),
		NuevoCredencialRehasheada(id, ahora),
		NuevoCorreoVerificado(id, correo, ahora),
		NuevoVerificacionCorreoFallida(id.String(), "token_expirado", ahora),
		NuevoEstadoUsuarioCambiado(id, EstadoActivo, EstadoSuspendido, "motivo", ahora),
		NuevoUsuarioConsultado(id.String(), "otro-id", ahora),
		// Extensión OTP/MFA (§1.6 de docs/design/otp-mfa.md).
		NuevoFactorMFAHabilitado(id, idFactor, ahora),
		NuevoFactorMFAConfirmado(id, idFactor, ahora),
		NuevoFactorMFADeshabilitado(id, idFactor, ahora),
		NuevoCodigoRespaldoConsumido(id, idFactor, 9, ahora),
		NuevoVerificacionOTPFallida(id, ahora),
	}

	if len(eventos) != 17 {
		t.Fatalf("se esperaban 17 eventos de dominio (tabla 1.6 + sección 3.4 + extensión OTP/MFA), hay %d", len(eventos))
	}

	tipoContrasenaPlana := reflect.TypeOf(ContrasenaPlana{})
	tipoHashContrasena := reflect.TypeOf(HashContrasena{})

	nombresPeligrosos := []string{"token", "contrasenaplana", "hashcontrasena", "secreto", "otp"}

	for _, ev := range eventos {
		v := reflect.ValueOf(ev)
		tipoEv := v.Type()
		for i := 0; i < tipoEv.NumField(); i++ {
			campo := tipoEv.Field(i)
			if campo.Type == tipoContrasenaPlana || campo.Type == tipoHashContrasena {
				t.Errorf("%s.%s tiene tipo %s: un evento no puede transportar credenciales", tipoEv.Name(), campo.Name, campo.Type)
			}
			nombreMin := strings.ToLower(campo.Name)
			for _, peligroso := range nombresPeligrosos {
				if strings.Contains(nombreMin, peligroso) {
					t.Errorf("%s.%s tiene un nombre que sugiere transportar un secreto/token", tipoEv.Name(), campo.Name)
				}
			}
		}
		if ev.NombreEvento() == "" {
			t.Errorf("%T.NombreEvento() no debe estar vacío", ev)
		}
		if !ev.OcurridoEn().Equal(ahora) {
			t.Errorf("%T.OcurridoEn() = %v, esperado %v", ev, ev.OcurridoEn(), ahora)
		}
	}
}

func TestEventos_NombresUnicos(t *testing.T) {
	ahora := time.Now()
	id, _ := IDUsuarioDesde("018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6d")
	idFactor, _ := IDFactorMFADesde("018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c99")
	correo, _ := NuevoCorreo("usuario@ejemplo.com")

	eventos := []EventoDominio{
		NuevoUsuarioRegistrado(id, correo, ahora),
		NuevoRegistroRechazado("x", "y", ahora),
		NuevoAutenticacionExitosa(id, correo, ahora),
		NuevoAutenticacionFallida("", "x", "y", ahora),
		NuevoAutenticacionDenegada("x", "y", ahora),
		NuevoSegundoFactorRequerido(id, "y", ahora),
		NuevoContrasenaCambiada(id, ahora),
		NuevoCredencialRehasheada(id, ahora),
		NuevoCorreoVerificado(id, correo, ahora),
		NuevoVerificacionCorreoFallida(id.String(), "y", ahora),
		NuevoEstadoUsuarioCambiado(id, EstadoActivo, EstadoSuspendido, "y", ahora),
		NuevoUsuarioConsultado(id.String(), "otro", ahora),
		NuevoFactorMFAHabilitado(id, idFactor, ahora),
		NuevoFactorMFAConfirmado(id, idFactor, ahora),
		NuevoFactorMFADeshabilitado(id, idFactor, ahora),
		NuevoCodigoRespaldoConsumido(id, idFactor, 9, ahora),
		NuevoVerificacionOTPFallida(id, ahora),
	}
	vistos := map[string]bool{}
	for _, ev := range eventos {
		nombre := ev.NombreEvento()
		if vistos[nombre] {
			t.Errorf("el nombre de evento %q está duplicado", nombre)
		}
		vistos[nombre] = true
	}
}

// TestEventos_IDAgregado_ApuntaAlUsuario verifica que cada evento con
// usuario ya persistido reporta ese IDUsuario como IDAgregado.
func TestEventos_IDAgregado_ApuntaAlUsuario(t *testing.T) {
	ahora := time.Now()
	id, _ := IDUsuarioDesde("018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6d")
	idFactor, _ := IDFactorMFADesde("018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c99")
	correo, _ := NuevoCorreo("usuario@ejemplo.com")

	casos := []EventoDominio{
		NuevoUsuarioRegistrado(id, correo, ahora),
		NuevoAutenticacionExitosa(id, correo, ahora),
		NuevoAutenticacionFallida(id.String(), correo.Normalizado(), "motivo", ahora),
		NuevoSegundoFactorRequerido(id, "motivo", ahora),
		NuevoContrasenaCambiada(id, ahora),
		NuevoCredencialRehasheada(id, ahora),
		NuevoCorreoVerificado(id, correo, ahora),
		NuevoVerificacionCorreoFallida(id.String(), "motivo", ahora),
		NuevoEstadoUsuarioCambiado(id, EstadoActivo, EstadoSuspendido, "motivo", ahora),
		NuevoUsuarioConsultado(id.String(), "otro-id", ahora),
		NuevoFactorMFAHabilitado(id, idFactor, ahora),
		NuevoFactorMFAConfirmado(id, idFactor, ahora),
		NuevoFactorMFADeshabilitado(id, idFactor, ahora),
		NuevoCodigoRespaldoConsumido(id, idFactor, 9, ahora),
		NuevoVerificacionOTPFallida(id, ahora),
	}
	for _, ev := range casos {
		if ev.IDAgregado() != id.String() {
			t.Errorf("%s.IDAgregado() = %q, esperado %q", ev.NombreEvento(), ev.IDAgregado(), id.String())
		}
	}
}

func TestEvento_IDAgregado_VacioCuandoNoHayUsuarioPersistido(t *testing.T) {
	ahora := time.Now()
	rechazo := NuevoRegistroRechazado("x@y.com", "motivo", ahora)
	if rechazo.IDAgregado() != "" {
		t.Errorf("RegistroRechazado.IDAgregado() debe estar vacío, obtuvo %q", rechazo.IDAgregado())
	}
	denegada := NuevoAutenticacionDenegada("x@y.com", "motivo", ahora)
	if denegada.IDAgregado() != "" {
		t.Errorf("AutenticacionDenegada.IDAgregado() debe estar vacío, obtuvo %q", denegada.IDAgregado())
	}
	// VerificacionCorreoFallida: si el token no existe, no se pudo resolver
	// a qué usuario pertenecía (sección 3.4 del diseño).
	verificacionFallida := NuevoVerificacionCorreoFallida("", "token_invalido", ahora)
	if verificacionFallida.IDAgregado() != "" {
		t.Errorf("VerificacionCorreoFallida.IDAgregado() debe estar vacío cuando no hay usuario resuelto, obtuvo %q", verificacionFallida.IDAgregado())
	}
}

// TestVerificacionCorreoFallida_ConstruyeConMotivoYUsuario verifica el
// constructor del evento nuevo de la sección 3.4 del diseño: cuando el
// token sí se pudo resolver a un usuario (por ejemplo, expiró pero el hash
// se encontró), IDAgregado debe reportar ese usuario.
func TestVerificacionCorreoFallida_ConstruyeConMotivoYUsuario(t *testing.T) {
	ahora := time.Now()
	id, _ := IDUsuarioDesde("018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6d")

	ev := NuevoVerificacionCorreoFallida(id.String(), "token_expirado", ahora)

	if ev.NombreEvento() != "VerificacionCorreoFallida" {
		t.Errorf("NombreEvento() = %q, esperado %q", ev.NombreEvento(), "VerificacionCorreoFallida")
	}
	if ev.IDAgregado() != id.String() {
		t.Errorf("IDAgregado() = %q, esperado %q", ev.IDAgregado(), id.String())
	}
	if ev.Motivo != "token_expirado" {
		t.Errorf("Motivo = %q, esperado %q", ev.Motivo, "token_expirado")
	}
	if !ev.OcurridoEn().Equal(ahora) {
		t.Errorf("OcurridoEn() = %v, esperado %v", ev.OcurridoEn(), ahora)
	}
}
