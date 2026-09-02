package dominio

// EstadoSesion es el value object enum que representa el ciclo de vida de
// una Sesion. Solo admite los valores del catálogo cerrado definido por las
// variables EstadoSesion* y conoce sus transiciones legales (§1.4 del
// diseño: MaquinaEstadosSesion, implementada como método de este tipo, mismo
// patrón que EstadoUsuario en identidad/dominio).
type EstadoSesion struct {
	valor string
}

var (
	// EstadoSesionActiva es el único estado desde el que una sesión puede
	// renovarse o emitir tokens de acceso.
	EstadoSesionActiva = EstadoSesion{valor: "activa"}
	// EstadoSesionRevocada es terminal: una sesión revocada nunca vuelve a
	// activa (INV-ACC-07). Volver a entrar crea un IDSesion nuevo.
	EstadoSesionRevocada = EstadoSesion{valor: "revocada"}
	// EstadoSesionExpirada es terminal, por la misma razón que
	// EstadoSesionRevocada.
	EstadoSesionExpirada = EstadoSesion{valor: "expirada"}
)

// EstadoSesionDesde valida un valor persistido contra el catálogo cerrado
// de estados.
func EstadoSesionDesde(valor string) (EstadoSesion, error) {
	switch valor {
	case EstadoSesionActiva.valor, EstadoSesionRevocada.valor, EstadoSesionExpirada.valor:
		return EstadoSesion{valor: valor}, nil
	default:
		return EstadoSesion{}, &ErrEstadoSesionInvalido{Valor: valor}
	}
}

// String devuelve la representación canónica del estado.
func (e EstadoSesion) String() string { return e.valor }

// EsIgual compara dos estados por su valor.
func (e EstadoSesion) EsIgual(otro EstadoSesion) bool { return e.valor == otro.valor }

// EsTerminal indica si el estado no admite ninguna transición de salida
// (revocada y expirada; INV-ACC-07).
func (e EstadoSesion) EsTerminal() bool {
	return e.EsIgual(EstadoSesionRevocada) || e.EsIgual(EstadoSesionExpirada)
}

// PuedeTransicionarA indica si existe una transición legal del estado
// origen (e) al estado destino, según la máquina de estados de la sección
// 1.4 del diseño:
//
//	activa   --revocar-----> revocada    (terminal)
//	activa   --expirar-----> expirada    (terminal)
//	revocada --*-----------> ✗
//	expirada --*-----------> ✗
//
// Ambos estados finales son terminales: una sesión nunca resucita.
func (e EstadoSesion) PuedeTransicionarA(destino EstadoSesion) bool {
	if e.EsTerminal() {
		return false
	}
	return destino.EsIgual(EstadoSesionRevocada) || destino.EsIgual(EstadoSesionExpirada)
}

// ErrEstadoSesionInvalido se produce al construir un EstadoSesion con un
// valor fuera del catálogo cerrado.
type ErrEstadoSesionInvalido struct{ Valor string }

func (e *ErrEstadoSesionInvalido) Error() string {
	return "estado de sesión desconocido: " + e.Valor
}
