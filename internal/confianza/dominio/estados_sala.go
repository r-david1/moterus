package dominio

import "strings"

// --- EstadoSala ---------------------------------------------------------

// EstadoSala es el value object enum que representa el ciclo de vida de una
// SalaDeEspera. Solo admite los valores del catálogo cerrado definido por
// las variables EstadoSala* y conoce sus transiciones legales (§1.6 del
// diseño), mismo patrón que EstadoOrganizacion/EstadoMembresia en
// tenencia/dominio.
type EstadoSala struct {
	valor string
}

var (
	// EstadoSalaProgramada es el estado de nacimiento de una sala recién
	// configurada, todavía sin aceptar ingresos.
	EstadoSalaProgramada = EstadoSala{valor: "programada"}
	// EstadoSalaAbierta es el único estado que admite ingresos nuevos.
	EstadoSalaAbierta = EstadoSala{valor: "abierta"}
	// EstadoSalaDrenando no admite ingresos nuevos, pero los tickets vivos
	// siguen avanzando: es el estado que hace operable el cierre de un
	// evento sin tirar a la calle a quienes ya esperaban.
	EstadoSalaDrenando = EstadoSala{valor: "drenando"}
	// EstadoSalaCerrada es terminal.
	EstadoSalaCerrada = EstadoSala{valor: "cerrada"}
)

// EstadoSalaDesde valida un valor persistido contra el catálogo cerrado de
// estados.
func EstadoSalaDesde(valor string) (EstadoSala, error) {
	v := strings.TrimSpace(valor)
	switch v {
	case EstadoSalaProgramada.valor, EstadoSalaAbierta.valor, EstadoSalaDrenando.valor, EstadoSalaCerrada.valor:
		return EstadoSala{valor: v}, nil
	default:
		return EstadoSala{}, &ErrEstadoSalaInvalido{Valor: valor}
	}
}

// String devuelve la representación canónica del estado.
func (e EstadoSala) String() string { return e.valor }

// EsVacio indica si el value object nunca fue construido (zero value).
func (e EstadoSala) EsVacio() bool { return e.valor == "" }

// EsIgual compara dos estados por su valor.
func (e EstadoSala) EsIgual(otro EstadoSala) bool { return e.valor == otro.valor }

// EsTerminal indica si el estado no admite ninguna transición de salida
// (cerrada).
func (e EstadoSala) EsTerminal() bool { return e.EsIgual(EstadoSalaCerrada) }

// PuedeTransicionarA indica si existe una transición legal del estado
// origen (e) al estado destino, según la máquina de estados del §1.6 del
// diseño:
//
//	programada --abrir----> abierta
//	abierta    --drenar---> drenando      (los tickets vivos siguen avanzando)
//	abierta    --cerrar---> cerrada       (terminal)
//	drenando   --cerrar---> cerrada       (terminal)
//	drenando   --abrir----> abierta       (reapertura durante el drenaje)
//	cerrada    --*--------> ✗
func (e EstadoSala) PuedeTransicionarA(destino EstadoSala) bool {
	if e.EsTerminal() {
		return false
	}
	switch {
	case e.EsIgual(EstadoSalaProgramada):
		return destino.EsIgual(EstadoSalaAbierta)
	case e.EsIgual(EstadoSalaAbierta):
		return destino.EsIgual(EstadoSalaDrenando) || destino.EsIgual(EstadoSalaCerrada)
	case e.EsIgual(EstadoSalaDrenando):
		return destino.EsIgual(EstadoSalaCerrada) || destino.EsIgual(EstadoSalaAbierta)
	default:
		return false
	}
}

// --- DesenlaceDeAdmision -----------------------------------------------------

// DesenlaceDeAdmision es el catálogo cerrado de los siete resultados
// posibles de presentar un ticket de cola (tabla 1.3 del diseño). Es el
// vocabulario con el que el middleware y el endpoint de estado hablan; un
// string libre haría imposible responder, sin parsear texto, preguntas como
// "¿cuántos turnos se caducaron en el evento del lunes?" (mismo argumento
// que MotivoDenegacion en tenencia/dominio y MotivoRevocacion en
// acceso/dominio).
//
// A diferencia de otros catálogos cerrados del repositorio (INV-ACC-21,
// INV-TEN-24, INV-MFA-08), estos siete valores NO se colapsan entre sí: un
// ticket de cola no protege ningún secreto (reingresar es gratis, público y
// sin credenciales), así que distinguir "ticket_desconocido" de
// "ticket_consumido" no construye ningún oráculo — es la diferencia entre un
// frontend que sabe qué mostrarle al usuario y uno que no (§1.8 del diseño).
type DesenlaceDeAdmision struct {
	valor string
}

var (
	// DesenlaceAdmitido: el turno del ticket ya llegó y no había sido
	// reclamado antes; la petición real puede continuar.
	DesenlaceAdmitido = DesenlaceDeAdmision{valor: "admitido"}
	// DesenlaceEsperando: el rango del ticket todavía es mayor que el
	// cursor de admisión.
	DesenlaceEsperando = DesenlaceDeAdmision{valor: "esperando"}
	// DesenlaceTurnoCaducado: el turno llegó pero la ventana de reclamo ya
	// se agotó.
	DesenlaceTurnoCaducado = DesenlaceDeAdmision{valor: "turno_caducado"}
	// DesenlaceTicketDesconocido: el hash presentado no tiene estado en
	// Redis (nunca existió, o ya expiró).
	DesenlaceTicketDesconocido = DesenlaceDeAdmision{valor: "ticket_desconocido"}
	// DesenlaceTicketConsumido: el ticket ya se reclamó una vez
	// (INV-COLA-06).
	DesenlaceTicketConsumido = DesenlaceDeAdmision{valor: "ticket_consumido"}
	// DesenlaceSalaCerrada: la sala ya no está vigente (no existe su
	// configuración en Redis, o su estado no es "abierta").
	DesenlaceSalaCerrada = DesenlaceDeAdmision{valor: "sala_cerrada"}
	// DesenlaceColaLlena: se alcanzó capacidadMaximaCola.
	DesenlaceColaLlena = DesenlaceDeAdmision{valor: "cola_llena"}
)

// DesenlaceDeAdmisionDesde valida un valor contra el catálogo cerrado de
// desenlaces.
func DesenlaceDeAdmisionDesde(valor string) (DesenlaceDeAdmision, error) {
	v := strings.TrimSpace(valor)
	switch v {
	case DesenlaceAdmitido.valor, DesenlaceEsperando.valor, DesenlaceTurnoCaducado.valor,
		DesenlaceTicketDesconocido.valor, DesenlaceTicketConsumido.valor,
		DesenlaceSalaCerrada.valor, DesenlaceColaLlena.valor:
		return DesenlaceDeAdmision{valor: v}, nil
	default:
		return DesenlaceDeAdmision{}, &ErrDesenlaceDeAdmisionInvalido{Valor: valor}
	}
}

// String devuelve el valor canónico del desenlace.
func (d DesenlaceDeAdmision) String() string { return d.valor }

// EsVacio indica si el value object nunca fue construido (zero value).
func (d DesenlaceDeAdmision) EsVacio() bool { return d.valor == "" }

// EsIgual compara dos desenlaces por su valor.
func (d DesenlaceDeAdmision) EsIgual(otro DesenlaceDeAdmision) bool { return d.valor == otro.valor }
