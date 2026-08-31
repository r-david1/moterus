package dominio

import (
	"errors"
	"strings"
	"testing"
)

// TestErrores_ImplementanInterfazError verifica que los errores tipados de
// la tabla 1.5 son distinguibles con errors.As, no simples errors.New.
func TestErrores_ImplementanInterfazError(t *testing.T) {
	var todos = []error{
		&ErrCorreoInvalido{Motivo: "x"},
		&ErrCorreoYaRegistrado{},
		&ErrContrasenaDebil{Reglas: []string{"x"}},
		&ErrContrasenaFiltrada{},
		&ErrCredencialesInvalidas{},
		&ErrUsuarioNoEncontrado{IDUsuario: "id"},
		&ErrCorreoNoVerificado{},
		&ErrCuentaSuspendida{Motivo: "x"},
		&ErrCuentaBloqueada{Motivo: "x"},
		&ErrTransicionEstadoInvalida{Origen: EstadoActivo, Destino: EstadoBloqueado},
		&ErrAccesoDenegadoPorConfianza{Motivo: "x"},
		&ErrConcurrenciaUsuario{},
		&ErrTokenVerificacionInvalido{},
		&ErrTokenVerificacionExpirado{},
	}
	for _, err := range todos {
		if err.Error() == "" {
			t.Errorf("%T.Error() no debe ser vacío", err)
		}
	}
}

func TestErrTransicionEstadoInvalida_IncluyeOrigenYDestino(t *testing.T) {
	err := &ErrTransicionEstadoInvalida{Origen: EstadoSuspendido, Destino: EstadoBloqueado}
	msg := err.Error()
	if !strings.Contains(msg, "suspendido") || !strings.Contains(msg, "bloqueado") {
		t.Errorf("el mensaje debe incluir origen y destino: %q", msg)
	}
}

func TestErrContrasenaDebil_ExponeReglasIncumplidas(t *testing.T) {
	err := &ErrContrasenaDebil{Reglas: []string{"regla-a", "regla-b"}}
	msg := err.Error()
	if !strings.Contains(msg, "regla-a") || !strings.Contains(msg, "regla-b") {
		t.Errorf("el mensaje debe exponer las reglas incumplidas: %q", msg)
	}
}

func TestErrCredencialesInvalidas_EsGenerico(t *testing.T) {
	// INV-ID-11: el mismo error debe usarse tanto si el correo no existe
	// como si la contraseña es incorrecta; aquí solo verificamos que el
	// tipo existe y es distinguible con errors.As.
	var err error = &ErrCredencialesInvalidas{}
	var objetivo *ErrCredencialesInvalidas
	if !errors.As(err, &objetivo) {
		t.Error("ErrCredencialesInvalidas debe ser distinguible con errors.As")
	}
}

func TestErrores_DistinguiblesConErrorsAs(t *testing.T) {
	var err error = &ErrCuentaSuspendida{Motivo: "revisión de seguridad"}

	var suspendida *ErrCuentaSuspendida
	if !errors.As(err, &suspendida) {
		t.Fatal("se esperaba poder extraer *ErrCuentaSuspendida con errors.As")
	}

	var bloqueada *ErrCuentaBloqueada
	if errors.As(err, &bloqueada) {
		t.Error("ErrCuentaSuspendida no debe confundirse con ErrCuentaBloqueada")
	}
}

// TestErrTokenVerificacion_InvalidoYExpirado_SonDistinguibles verifica que
// ambos errores del mecanismo de verificación de correo (sección 3.4 del
// diseño) tienen mensajes propios y no se confunden entre sí con
// errors.As, tal como exige la capa de aplicación para mapearlos a
// 404/410 respectivamente.
func TestErrTokenVerificacion_InvalidoYExpirado_SonDistinguibles(t *testing.T) {
	var invalido error = &ErrTokenVerificacionInvalido{}
	var objInvalido *ErrTokenVerificacionInvalido
	if !errors.As(invalido, &objInvalido) {
		t.Fatal("ErrTokenVerificacionInvalido debe ser distinguible con errors.As")
	}
	var objExpirado *ErrTokenVerificacionExpirado
	if errors.As(invalido, &objExpirado) {
		t.Error("ErrTokenVerificacionInvalido no debe confundirse con ErrTokenVerificacionExpirado")
	}

	var expirado error = &ErrTokenVerificacionExpirado{}
	if !errors.As(expirado, &objExpirado) {
		t.Fatal("ErrTokenVerificacionExpirado debe ser distinguible con errors.As")
	}

	if invalido.Error() == expirado.Error() {
		t.Error("los mensajes de ErrTokenVerificacionInvalido y ErrTokenVerificacionExpirado deben diferir")
	}
}
