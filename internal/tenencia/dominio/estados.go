package dominio

import (
	"strings"
	"unicode/utf8"
)

// --- EstadoOrganizacion -----------------------------------------------------

// EstadoOrganizacion es el value object enum que representa el ciclo de
// vida de una Organizacion. Solo admite los valores del catálogo cerrado
// definido por las variables EstadoOrganizacion* y conoce sus transiciones
// legales (§1.4 del diseño: MaquinaEstadosOrganizacion, implementada como
// método de este tipo, mismo patrón que EstadoUsuario en identidad/dominio
// y EstadoSesion en acceso/dominio).
type EstadoOrganizacion struct {
	valor string
}

var (
	// EstadoOrganizacionActiva es el único estado desde el que una
	// organización concede autorización a sus miembros (INV-TEN-18). Es
	// también el único estado de nacimiento (INV-TEN-01).
	EstadoOrganizacionActiva = EstadoOrganizacion{valor: "activa"}
	// EstadoOrganizacionSuspendida es un estado no operativo reversible: el
	// acceso de todos los miembros se apaga por conjunción en tiempo de
	// autorización, sin mutar ninguna membresía (INV-TEN-05).
	EstadoOrganizacionSuspendida = EstadoOrganizacion{valor: "suspendida"}
	// EstadoOrganizacionArchivada es terminal e irreversible (INV-TEN-04).
	EstadoOrganizacionArchivada = EstadoOrganizacion{valor: "archivada"}
)

// EstadoOrganizacionDesde valida un valor persistido contra el catálogo
// cerrado de estados.
func EstadoOrganizacionDesde(valor string) (EstadoOrganizacion, error) {
	switch valor {
	case EstadoOrganizacionActiva.valor, EstadoOrganizacionSuspendida.valor, EstadoOrganizacionArchivada.valor:
		return EstadoOrganizacion{valor: valor}, nil
	default:
		return EstadoOrganizacion{}, &ErrEstadoOrganizacionInvalido{Valor: valor}
	}
}

// String devuelve la representación canónica del estado.
func (e EstadoOrganizacion) String() string { return e.valor }

// EsIgual compara dos estados por su valor.
func (e EstadoOrganizacion) EsIgual(otro EstadoOrganizacion) bool { return e.valor == otro.valor }

// EsTerminal indica si el estado no admite ninguna transición de salida
// (archivada; INV-TEN-04).
func (e EstadoOrganizacion) EsTerminal() bool { return e.EsIgual(EstadoOrganizacionArchivada) }

// PuedeTransicionarA indica si existe una transición legal del estado
// origen (e) al estado destino, según la máquina de estados de la sección
// 1.4 del diseño:
//
//	activa      --suspender--> suspendida
//	activa      --archivar---> archivada     (terminal)
//	suspendida  --reactivar--> activa
//	suspendida  --archivar---> archivada     (terminal)
//	archivada   --*----------> ✗ (ninguna)
func (e EstadoOrganizacion) PuedeTransicionarA(destino EstadoOrganizacion) bool {
	if e.EsTerminal() {
		return false
	}
	switch {
	case e.EsIgual(EstadoOrganizacionActiva):
		return destino.EsIgual(EstadoOrganizacionSuspendida) || destino.EsIgual(EstadoOrganizacionArchivada)
	case e.EsIgual(EstadoOrganizacionSuspendida):
		return destino.EsIgual(EstadoOrganizacionActiva) || destino.EsIgual(EstadoOrganizacionArchivada)
	default:
		return false
	}
}

// --- EstadoMembresia ---------------------------------------------------------

// EstadoMembresia es el value object enum que representa el ciclo de vida
// de una Membresia. Solo admite los valores del catálogo cerrado definido
// por las variables EstadoMembresia* y conoce sus transiciones legales
// (§1.4 del diseño: MaquinaEstadosMembresia).
type EstadoMembresia struct {
	valor string
}

var (
	// EstadoMembresiaActiva es el único estado desde el que una membresía
	// puede autorizar acciones (sujeto a que la organización también esté
	// activa, INV-TEN-18).
	EstadoMembresiaActiva = EstadoMembresia{valor: "activa"}
	// EstadoMembresiaSuspendida es un estado no operativo reversible.
	EstadoMembresiaSuspendida = EstadoMembresia{valor: "suspendida"}
	// EstadoMembresiaRemovida es terminal (INV-TEN-08): una membresía nunca
	// se borra físicamente, transiciona aquí. Readmitir a alguien crea una
	// Membresia nueva con IDMembresia nuevo.
	EstadoMembresiaRemovida = EstadoMembresia{valor: "removida"}
)

// EstadoMembresiaDesde valida un valor persistido contra el catálogo
// cerrado de estados.
func EstadoMembresiaDesde(valor string) (EstadoMembresia, error) {
	switch valor {
	case EstadoMembresiaActiva.valor, EstadoMembresiaSuspendida.valor, EstadoMembresiaRemovida.valor:
		return EstadoMembresia{valor: valor}, nil
	default:
		return EstadoMembresia{}, &ErrEstadoMembresiaInvalido{Valor: valor}
	}
}

// String devuelve la representación canónica del estado.
func (e EstadoMembresia) String() string { return e.valor }

// EsIgual compara dos estados por su valor.
func (e EstadoMembresia) EsIgual(otro EstadoMembresia) bool { return e.valor == otro.valor }

// EsTerminal indica si el estado no admite ninguna transición de salida
// (removida; INV-TEN-08).
func (e EstadoMembresia) EsTerminal() bool { return e.EsIgual(EstadoMembresiaRemovida) }

// PuedeTransicionarA indica si existe una transición legal del estado
// origen (e) al estado destino, según la máquina de estados de la sección
// 1.4 del diseño:
//
//	activa     --suspender--> suspendida
//	activa     --remover----> removida       (terminal)
//	suspendida --reactivar--> activa
//	suspendida --remover----> removida       (terminal)
//	removida   --*----------> ✗              → readmitir crea una membresía NUEVA
func (e EstadoMembresia) PuedeTransicionarA(destino EstadoMembresia) bool {
	if e.EsTerminal() {
		return false
	}
	switch {
	case e.EsIgual(EstadoMembresiaActiva):
		return destino.EsIgual(EstadoMembresiaSuspendida) || destino.EsIgual(EstadoMembresiaRemovida)
	case e.EsIgual(EstadoMembresiaSuspendida):
		return destino.EsIgual(EstadoMembresiaActiva) || destino.EsIgual(EstadoMembresiaRemovida)
	default:
		return false
	}
}

// --- EstadoInvitacion --------------------------------------------------------

// EstadoInvitacion es el value object enum que representa el ciclo de vida
// de una Invitacion. Solo admite los valores del catálogo cerrado definido
// por las variables EstadoInvitacion* (§1.4 del diseño:
// MaquinaEstadosInvitacion): pendiente -> {aceptada | revocada | expirada},
// las tres últimas terminales.
type EstadoInvitacion struct {
	valor string
}

var (
	// EstadoInvitacionPendiente es el único estado desde el que una
	// invitación puede resolverse.
	EstadoInvitacionPendiente = EstadoInvitacion{valor: "pendiente"}
	// EstadoInvitacionAceptada es terminal: la invitación se redimió con
	// éxito y ya existe (o existía) una Membresia asociada.
	EstadoInvitacionAceptada = EstadoInvitacion{valor: "aceptada"}
	// EstadoInvitacionRevocada es terminal: un administrador la canceló
	// antes de que el destinatario la aceptara.
	EstadoInvitacionRevocada = EstadoInvitacion{valor: "revocada"}
	// EstadoInvitacionExpirada es terminal: se agotó la ventana de vigencia
	// sin ser aceptada ni revocada.
	EstadoInvitacionExpirada = EstadoInvitacion{valor: "expirada"}
)

// EstadoInvitacionDesde valida un valor persistido contra el catálogo
// cerrado de estados.
func EstadoInvitacionDesde(valor string) (EstadoInvitacion, error) {
	switch valor {
	case EstadoInvitacionPendiente.valor, EstadoInvitacionAceptada.valor,
		EstadoInvitacionRevocada.valor, EstadoInvitacionExpirada.valor:
		return EstadoInvitacion{valor: valor}, nil
	default:
		return EstadoInvitacion{}, &ErrEstadoInvitacionInvalido{Valor: valor}
	}
}

// String devuelve la representación canónica del estado.
func (e EstadoInvitacion) String() string { return e.valor }

// EsIgual compara dos estados por su valor.
func (e EstadoInvitacion) EsIgual(otro EstadoInvitacion) bool { return e.valor == otro.valor }

// EsTerminal indica si el estado no admite ninguna transición de salida
// (aceptada, revocada y expirada; las tres son terminales).
func (e EstadoInvitacion) EsTerminal() bool { return !e.EsIgual(EstadoInvitacionPendiente) }

// PuedeTransicionarA indica si existe una transición legal del estado
// origen (e) al estado destino: solo pendiente puede avanzar, a cualquiera
// de las tres terminales.
func (e EstadoInvitacion) PuedeTransicionarA(destino EstadoInvitacion) bool {
	if !e.EsIgual(EstadoInvitacionPendiente) {
		return false
	}
	return destino.EsIgual(EstadoInvitacionAceptada) ||
		destino.EsIgual(EstadoInvitacionRevocada) ||
		destino.EsIgual(EstadoInvitacionExpirada)
}

// --- MotivoCambioEstado -------------------------------------------------------

// longitudMaximaMotivoCambioEstado acota el texto de MotivoCambioEstado
// (tabla 1.3 del diseño).
const longitudMaximaMotivoCambioEstado = 280

// MotivoCambioEstado es el texto obligatorio, acotado y no vacío, que
// documenta con qué autoridad y por qué se suspende o archiva una
// organización. Mismo VO conceptual que en identidad/dominio: suspender un
// tenant completo exige decir por qué.
type MotivoCambioEstado struct {
	valor string
}

// NuevoMotivo valida que el motivo no esté vacío y que su longitud esté
// acotada a 280 caracteres.
func NuevoMotivo(texto string) (MotivoCambioEstado, error) {
	v := strings.TrimSpace(texto)
	if v == "" {
		return MotivoCambioEstado{}, &ErrMotivoCambioEstadoInvalido{Motivo: "no puede estar vacío"}
	}
	if utf8.RuneCountInString(v) > longitudMaximaMotivoCambioEstado {
		return MotivoCambioEstado{}, &ErrMotivoCambioEstadoInvalido{Motivo: "supera la longitud máxima permitida"}
	}
	return MotivoCambioEstado{valor: v}, nil
}

// Valor devuelve el texto del motivo.
func (m MotivoCambioEstado) Valor() string { return m.valor }

// String implementa fmt.Stringer.
func (m MotivoCambioEstado) String() string { return m.valor }

// EsVacio indica si el value object nunca fue construido (zero value).
func (m MotivoCambioEstado) EsVacio() bool { return m.valor == "" }
