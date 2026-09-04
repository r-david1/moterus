package dominio

// MotivoDenegacion es el value object enum cerrado que explica por qué
// Autorizar denegó una petición (§1.3 y §1.4 del diseño). Es lo que
// determina si el adaptador HTTP responde 404 o 403 (INV-TEN-17) y lo que
// alimenta las métricas de intentos de escalada; un string libre haría
// imposible responder "¿cuántos intentos de escalada de privilegios hubo
// este mes?" sin parsear texto (mismo argumento que MotivoRevocacion en
// acceso/dominio).
type MotivoDenegacion struct {
	valor string
}

var (
	// MotivoDenegacionSinMembresia: el sujeto no tiene ninguna membresía no
	// removida en la organización. Se traduce a 404, nunca a 403
	// (INV-TEN-17): un 403 confirmaría que esa organización existe.
	MotivoDenegacionSinMembresia = MotivoDenegacion{valor: "sin_membresia"}
	// MotivoDenegacionMembresiaSuspendida: el sujeto es miembro pero su
	// membresía está suspendida.
	MotivoDenegacionMembresiaSuspendida = MotivoDenegacion{valor: "membresia_suspendida"}
	// MotivoDenegacionOrganizacionNoOperativa: la membresía está activa pero
	// la organización está suspendida o archivada.
	MotivoDenegacionOrganizacionNoOperativa = MotivoDenegacion{valor: "organizacion_no_operativa"}
	// MotivoDenegacionRolInsuficiente: la membresía y la organización están
	// activas, pero el rol del sujeto no incluye el permiso pedido.
	MotivoDenegacionRolInsuficiente = MotivoDenegacion{valor: "rol_insuficiente"}
)

// catalogoMotivosDenegacion es el catálogo cerrado completo.
var catalogoMotivosDenegacion = map[string]MotivoDenegacion{
	MotivoDenegacionSinMembresia.valor:            MotivoDenegacionSinMembresia,
	MotivoDenegacionMembresiaSuspendida.valor:     MotivoDenegacionMembresiaSuspendida,
	MotivoDenegacionOrganizacionNoOperativa.valor: MotivoDenegacionOrganizacionNoOperativa,
	MotivoDenegacionRolInsuficiente.valor:         MotivoDenegacionRolInsuficiente,
}

// MotivoDenegacionDesde valida un valor contra el catálogo cerrado de
// cuatro motivos de denegación.
func MotivoDenegacionDesde(valor string) (MotivoDenegacion, error) {
	m, ok := catalogoMotivosDenegacion[valor]
	if !ok {
		return MotivoDenegacion{}, &ErrMotivoDenegacionInvalido{Valor: valor}
	}
	return m, nil
}

// Valor devuelve la representación canónica del motivo.
func (m MotivoDenegacion) Valor() string { return m.valor }

// String implementa fmt.Stringer.
func (m MotivoDenegacion) String() string { return m.valor }

// EsVacio indica si el value object nunca fue construido (zero value).
func (m MotivoDenegacion) EsVacio() bool { return m.valor == "" }

// EsIgual compara dos motivos de denegación por su valor.
func (m MotivoDenegacion) EsIgual(otro MotivoDenegacion) bool { return m.valor == otro.valor }

// DecisionAutorizacion es el value object que produce EvaluadorDeAutorizacion:
// una decisión, nunca datos (INV-TEN-16). Permitido y Motivo van separados a
// propósito: el consumidor necesita distinguir "no sos miembro" (404) de
// "sos miembro pero no alcanza" (403), y esa distinción no puede deducirse
// de un simple bool.
type DecisionAutorizacion struct {
	permitido bool
	rol       Rol
	motivo    MotivoDenegacion
}

// Permitido indica si la operación se autoriza.
func (d DecisionAutorizacion) Permitido() bool { return d.permitido }

// Rol devuelve el rol efectivo del sujeto en la organización evaluada, y un
// booleano que indica si hay uno (falso cuando el motivo es
// MotivoDenegacionSinMembresia: sin membresía no hay rol que reportar).
func (d DecisionAutorizacion) Rol() (Rol, bool) {
	if d.rol.EsVacio() {
		return Rol{}, false
	}
	return d.rol, true
}

// Motivo devuelve el motivo de la denegación, y un booleano que indica si
// la decisión fue efectivamente una denegación (vacío cuando Permitido()
// es true: una concesión no tiene motivo que reportar).
func (d DecisionAutorizacion) Motivo() (MotivoDenegacion, bool) {
	if d.motivo.EsVacio() {
		return MotivoDenegacion{}, false
	}
	return d.motivo, true
}

// Autorizar implementa el servicio de dominio EvaluadorDeAutorizacion (§1.4
// del diseño): función pura que decide si un sujeto, representado por su
// membresía (o su ausencia) y el estado de la organización a la que
// pertenece, puede ejercer un permiso. El permiso efectivo es una
// conjunción, y el ORDEN DE EVALUACIÓN es normativo porque determina el
// MotivoDenegacion que llega a la auditoría y, aguas arriba, el código HTTP
// (INV-TEN-17, INV-TEN-18):
//
//  1. membresia == nil            -> denegado(sin_membresia)             -> 404
//  2. membresia.Estado() != activa -> denegado(membresia_suspendida)      -> 403
//  3. estadoOrg != activa          -> denegado(organizacion_no_operativa) -> 403
//  4. permiso ∉ rol.Permisos()     -> denegado(rol_insuficiente)          -> 403
//  5. permitido(rol)
//
// El paso 3 va después del 2 a propósito: si el sujeto ni siquiera es
// miembro (o su membresía está suspendida), no debe poder inferir el
// estado interno de la organización a partir de la respuesta.
func Autorizar(estadoOrg EstadoOrganizacion, membresia *Membresia, permiso Permiso) DecisionAutorizacion {
	if membresia == nil {
		return DecisionAutorizacion{motivo: MotivoDenegacionSinMembresia}
	}
	if !membresia.Estado().EsIgual(EstadoMembresiaActiva) {
		return DecisionAutorizacion{rol: membresia.Rol(), motivo: MotivoDenegacionMembresiaSuspendida}
	}
	if !estadoOrg.EsIgual(EstadoOrganizacionActiva) {
		return DecisionAutorizacion{rol: membresia.Rol(), motivo: MotivoDenegacionOrganizacionNoOperativa}
	}
	if !membresia.Rol().TienePermiso(permiso) {
		return DecisionAutorizacion{rol: membresia.Rol(), motivo: MotivoDenegacionRolInsuficiente}
	}
	return DecisionAutorizacion{permitido: true, rol: membresia.Rol()}
}
