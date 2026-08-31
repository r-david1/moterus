package dominio

import (
	"strings"
	"unicode/utf8"
)

// longitudMaximaMotivo acota el texto de MotivoCambioEstado (tabla 1.3).
const longitudMaximaMotivo = 280

// EstadoUsuario es el value object enum que representa el ciclo de vida de
// la cuenta de un Usuario. Solo admite los valores del catálogo cerrado
// definido por las variables Estado* y conoce sus transiciones legales.
type EstadoUsuario struct {
	valor string
}

var (
	// EstadoPendienteVerificacion es el estado inicial de todo Usuario recién
	// registrado (INV-ID-07).
	EstadoPendienteVerificacion = EstadoUsuario{valor: "pendiente_verificacion"}
	// EstadoActivo es el único estado desde el que se puede iniciar sesión
	// con éxito (INV-ID-06).
	EstadoActivo = EstadoUsuario{valor: "activo"}
	// EstadoSuspendido es un estado no operativo reversible.
	EstadoSuspendido = EstadoUsuario{valor: "suspendido"}
	// EstadoBloqueado es un estado no operativo, reversible solo por acción
	// administrativa.
	EstadoBloqueado = EstadoUsuario{valor: "bloqueado"}
	// EstadoAnonimizado es terminal e irreversible (derecho al olvido).
	EstadoAnonimizado = EstadoUsuario{valor: "anonimizado"}
)

// EstadoUsuarioDesde valida un valor persistido (p. ej. una columna de base
// de datos) contra el catálogo cerrado de estados.
func EstadoUsuarioDesde(valor string) (EstadoUsuario, error) {
	switch valor {
	case EstadoPendienteVerificacion.valor,
		EstadoActivo.valor,
		EstadoSuspendido.valor,
		EstadoBloqueado.valor,
		EstadoAnonimizado.valor:
		return EstadoUsuario{valor: valor}, nil
	default:
		return EstadoUsuario{}, &ErrEstadoUsuarioInvalido{Valor: valor}
	}
}

// String devuelve la representación canónica del estado.
func (e EstadoUsuario) String() string { return e.valor }

// EsIgual compara dos estados por su valor.
func (e EstadoUsuario) EsIgual(otro EstadoUsuario) bool { return e.valor == otro.valor }

// PuedeTransicionarA indica si, en abstracto, existe alguna transición
// legal del estado origen (e) al estado destino, según la máquina de
// estados de la sección 1.4 del diseño:
//
//	pendiente_verificacion --confirmar_correo--> activo
//	pendiente_verificacion --bloquear---------> bloqueado
//	activo                 --suspender--------> suspendido
//	activo                 --bloquear---------> bloqueado
//	suspendido             --reactivar--------> activo
//	bloqueado              --reactivar--------> activo
//	cualquiera             --anonimizar-------> anonimizado (terminal)
//	anonimizado            --*----------------> ninguna
//
// Es una consulta general de alcanzabilidad (INV-ID-05). Los métodos de
// negocio de Usuario (ConfirmarCorreo, Suspender, Bloquear, Reactivar)
// exigen además el origen específico de cada acción, porque varias
// acciones distintas pueden compartir el mismo destino: "activo" se
// alcanza desde pendiente_verificacion (confirmar_correo), desde
// suspendido y desde bloqueado (ambas por reactivar), y esas acciones no
// son intercambiables entre sí.
func (e EstadoUsuario) PuedeTransicionarA(destino EstadoUsuario) bool {
	if e.EsIgual(EstadoAnonimizado) {
		return false
	}
	if destino.EsIgual(EstadoAnonimizado) {
		return true
	}
	switch {
	case e.EsIgual(EstadoPendienteVerificacion):
		return destino.EsIgual(EstadoActivo) || destino.EsIgual(EstadoBloqueado)
	case e.EsIgual(EstadoActivo):
		return destino.EsIgual(EstadoSuspendido) || destino.EsIgual(EstadoBloqueado)
	case e.EsIgual(EstadoSuspendido):
		return destino.EsIgual(EstadoActivo)
	case e.EsIgual(EstadoBloqueado):
		return destino.EsIgual(EstadoActivo)
	default:
		return false
	}
}

// MotivoCambioEstado es el texto obligatorio, acotado y no vacío, que
// documenta con qué autoridad y por qué se suspende o bloquea una cuenta
// (lo exige la auditoría).
type MotivoCambioEstado struct {
	valor string
}

// NuevoMotivoCambioEstado valida que el motivo no esté vacío y que su
// longitud esté acotada a 280 caracteres.
func NuevoMotivoCambioEstado(texto string) (MotivoCambioEstado, error) {
	v := strings.TrimSpace(texto)
	if v == "" {
		return MotivoCambioEstado{}, &ErrMotivoCambioEstadoInvalido{Motivo: "no puede estar vacío"}
	}
	if utf8.RuneCountInString(v) > longitudMaximaMotivo {
		return MotivoCambioEstado{}, &ErrMotivoCambioEstadoInvalido{Motivo: "supera la longitud máxima permitida"}
	}
	return MotivoCambioEstado{valor: v}, nil
}

// Valor devuelve el texto del motivo.
func (m MotivoCambioEstado) Valor() string { return m.valor }

// String implementa fmt.Stringer.
func (m MotivoCambioEstado) String() string { return m.valor }

// --- errores de construcción propios de este archivo ------------------------

// ErrEstadoUsuarioInvalido se produce al construir un EstadoUsuario con un
// valor fuera del catálogo cerrado.
type ErrEstadoUsuarioInvalido struct{ Valor string }

func (e *ErrEstadoUsuarioInvalido) Error() string {
	return "estado de usuario desconocido: " + e.Valor
}

// ErrMotivoCambioEstadoInvalido se produce al construir un
// MotivoCambioEstado vacío o demasiado largo.
type ErrMotivoCambioEstadoInvalido struct{ Motivo string }

func (e *ErrMotivoCambioEstadoInvalido) Error() string { return "motivo inválido: " + e.Motivo }
