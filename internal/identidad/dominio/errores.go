package dominio

import (
	"fmt"
	"strings"
	"time"
)

// Errores de dominio tipados (tabla 1.5 del diseño del contexto Identidad).
//
// Se implementan como tipos propios (no errors.New ad hoc) para que la capa
// de aplicación pueda mapearlos a códigos HTTP inspeccionando el tipo con
// errors.As, en lugar de comparar cadenas de texto.

// ErrCorreoInvalido se produce en el constructor de Correo. Lleva el motivo
// estructural del rechazo, nunca el valor completo recibido.
type ErrCorreoInvalido struct{ Motivo string }

func (e *ErrCorreoInvalido) Error() string {
	return fmt.Sprintf("correo inválido: %s", e.Motivo)
}

// ErrCorreoYaRegistrado se produce al intentar dar de alta un correo que ya
// existe. El propio dominio no lo levanta: lo produce el adaptador Postgres
// al traducir la violación del índice único (INV-ID-02); se declara aquí
// porque el vocabulario de errores pertenece al dominio.
type ErrCorreoYaRegistrado struct{}

func (e *ErrCorreoYaRegistrado) Error() string { return "correo ya registrado" }

// ErrContrasenaDebil se produce cuando PoliticaContrasena rechaza una
// ContrasenaPlana. Expone las reglas incumplidas para que el adaptador arme
// una respuesta 422 útil.
type ErrContrasenaDebil struct{ Reglas []string }

func (e *ErrContrasenaDebil) Error() string {
	return fmt.Sprintf("contraseña débil: %s", strings.Join(e.Reglas, "; "))
}

// ErrContrasenaFiltrada se produce cuando el puerto de verificación de
// brechas conocidas (HIBP) reporta la contraseña como comprometida. Se
// mantiene separado de ErrContrasenaDebil porque exige una acción distinta
// del usuario.
type ErrContrasenaFiltrada struct{}

func (e *ErrContrasenaFiltrada) Error() string {
	return "la contraseña aparece en brechas de datos conocidas"
}

// ErrCredencialesInvalidas se produce cuando la autenticación falla.
// Deliberadamente genérico: el mismo error se usa si el correo no existe o
// si la contraseña es incorrecta (INV-ID-11).
type ErrCredencialesInvalidas struct{}

func (e *ErrCredencialesInvalidas) Error() string { return "credenciales inválidas" }

// ErrUsuarioNoEncontrado se produce en consultas por ID. Solo debe usarse en
// flujos autenticados/administrativos, nunca en el flujo de login.
type ErrUsuarioNoEncontrado struct{ IDUsuario string }

func (e *ErrUsuarioNoEncontrado) Error() string { return "usuario no encontrado" }

// ErrCorreoNoVerificado se produce al intentar iniciar sesión con una
// cuenta en estado pendiente_verificacion. Debe devolverse solo después de
// verificar la contraseña correctamente (INV-ID-06).
type ErrCorreoNoVerificado struct{}

func (e *ErrCorreoNoVerificado) Error() string { return "correo no verificado" }

// ErrCuentaSuspendida se produce al intentar iniciar sesión con una cuenta
// suspendida. Se devuelve solo tras verificar la contraseña.
type ErrCuentaSuspendida struct{ Motivo string }

func (e *ErrCuentaSuspendida) Error() string { return "cuenta suspendida" }

// ErrCuentaBloqueada se produce al intentar iniciar sesión con una cuenta
// bloqueada (o anonimizada). Se devuelve solo tras verificar la contraseña.
type ErrCuentaBloqueada struct{ Motivo string }

func (e *ErrCuentaBloqueada) Error() string { return "cuenta bloqueada" }

// ErrTransicionEstadoInvalida se produce cuando se intenta una transición
// de EstadoUsuario no permitida por la máquina de estados. Incluye el
// estado origen y el destino solicitado.
type ErrTransicionEstadoInvalida struct{ Origen, Destino EstadoUsuario }

func (e *ErrTransicionEstadoInvalida) Error() string {
	return fmt.Sprintf("transición de estado inválida: %s -> %s", e.Origen.String(), e.Destino.String())
}

// ErrAccesoDenegadoPorConfianza se produce cuando el contexto Confianza
// bloquea un intento (login o registro) antes de que Identidad lo procese.
// El caso de uso lo propaga; el adaptador HTTP lo mapea a 429/403.
//
// ReintentarEn viaja opcionalmente (puertos.DecisionConfianza.ReintentarEn)
// para que el adaptador HTTP pueda fijar la cabecera Retry-After — antes de
// que EvaluadorConfianza dejara de ser no-op este campo no existía y el
// 429 se devolvía sin esa cabecera (gap documentado en
// identidad/adaptadores/http/errores_http.go); ADR 0018 lo cierra.
type ErrAccesoDenegadoPorConfianza struct {
	Motivo       string
	ReintentarEn time.Duration
}

func (e *ErrAccesoDenegadoPorConfianza) Error() string {
	return "acceso denegado por evaluación de confianza"
}

// ErrConcurrenciaUsuario se produce ante un conflicto de versión optimista
// al guardar un Usuario. Es reintentable.
type ErrConcurrenciaUsuario struct{}

func (e *ErrConcurrenciaUsuario) Error() string {
	return "conflicto de concurrencia al guardar el usuario"
}

// ErrTokenVerificacionInvalido se produce cuando el token de verificación
// de correo recibido no corresponde a ningún token activo: no existe (el
// hash no se encuentra) o ya fue consumido/reemplazado. El token de
// verificación no es un VO del agregado Usuario (sección 3.4 del diseño del
// contexto): vive en su propio repositorio, por eso este error no lleva
// ningún dato del agregado.
type ErrTokenVerificacionInvalido struct{}

func (e *ErrTokenVerificacionInvalido) Error() string { return "token de verificación inválido" }

// ErrTokenVerificacionExpirado se produce cuando el token de verificación
// de correo existe pero superó su vigencia de 24 horas.
type ErrTokenVerificacionExpirado struct{}

func (e *ErrTokenVerificacionExpirado) Error() string { return "token de verificación expirado" }
