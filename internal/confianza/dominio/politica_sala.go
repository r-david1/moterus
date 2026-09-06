package dominio

import (
	"strings"
	"time"
)

// Límites estructurales de PoliticaSala (tabla 1.3 del diseño).
const (
	ritmoAdmisionMinimo      = 1
	ritmoAdmisionMaximo      = 10000
	capacidadMaximaColaMin   = 100
	capacidadMaximaColaMax   = 5_000_000
	ventanaReclamoMinima     = 30 * time.Second
	ventanaReclamoMaxima     = 15 * time.Minute
	ritmoAdmisionPorDefecto  = 50
	capacidadColaPorDefecto  = 500_000
	ventanaReclamoPorDefecto = 2 * time.Minute
)

// --- RitmoAdmision -----------------------------------------------------------

// RitmoAdmision es el value object que representa cuántos ingresos por
// segundo deja pasar una SalaDeEspera (§1.5 del diseño: es el parámetro
// "ritmoAdmision" de la fórmula del cursor). Es un entero, no fraccionario a
// propósito (tabla 1.3): un ritmo menor a 1/s se expresa cerrando y
// reabriendo la sala, no con aritmética de punto flotante dentro de un
// script Lua.
type RitmoAdmision struct {
	porSegundo int
}

// NuevoRitmoAdmision valida que el ritmo esté en [1, 10000]. El techo no es
// una capacidad real: es un fusible contra un PATCH con un cero de más que
// vaciaría la cola de golpe (tabla 1.3 del diseño).
func NuevoRitmoAdmision(porSegundo int) (RitmoAdmision, error) {
	if porSegundo < ritmoAdmisionMinimo || porSegundo > ritmoAdmisionMaximo {
		return RitmoAdmision{}, &ErrRitmoAdmisionInvalido{Motivo: "debe estar entre 1 y 10000 admisiones por segundo"}
	}
	return RitmoAdmision{porSegundo: porSegundo}, nil
}

// PorSegundo devuelve el ritmo en admisiones por segundo.
func (r RitmoAdmision) PorSegundo() int { return r.porSegundo }

// EsVacio indica si el value object nunca fue construido (zero value).
func (r RitmoAdmision) EsVacio() bool { return r.porSegundo == 0 }

// EsIgual compara dos ritmos por su valor.
func (r RitmoAdmision) EsIgual(otro RitmoAdmision) bool { return r.porSegundo == otro.porSegundo }

// --- ModoDegradado -----------------------------------------------------------

// ModoDegradado es el catálogo cerrado de dos valores que decide qué hace el
// middleware cuando Redis no responde (§8 del diseño, ADR candidato 0044).
type ModoDegradado struct {
	valor string
}

var (
	// ModoDegradadoPermitir es fail-open: ante una caída de Redis, el
	// middleware deja pasar la petición. Es el default (§1.3).
	ModoDegradadoPermitir = ModoDegradado{valor: "permitir"}
	// ModoDegradadoRechazar es fail-closed: ante una caída de Redis, el
	// middleware responde 503. Se declara al abrir la sala o en caliente,
	// para eventos donde tumbar el backend real cuesta más que rechazar
	// tráfico (§8).
	ModoDegradadoRechazar = ModoDegradado{valor: "rechazar"}
)

// ModoDegradadoDesde valida un valor contra el catálogo cerrado.
func ModoDegradadoDesde(valor string) (ModoDegradado, error) {
	v := strings.TrimSpace(valor)
	switch v {
	case ModoDegradadoPermitir.valor, ModoDegradadoRechazar.valor:
		return ModoDegradado{valor: v}, nil
	default:
		return ModoDegradado{}, &ErrModoDegradadoInvalido{Valor: valor}
	}
}

// String devuelve el valor canónico del modo degradado.
func (m ModoDegradado) String() string { return m.valor }

// EsVacio indica si el value object nunca fue construido (zero value).
func (m ModoDegradado) EsVacio() bool { return m.valor == "" }

// EsIgual compara dos modos degradados por su valor.
func (m ModoDegradado) EsIgual(otro ModoDegradado) bool { return m.valor == otro.valor }

// EsPermitir indica si el modo es fail-open.
func (m ModoDegradado) EsPermitir() bool { return m.EsIgual(ModoDegradadoPermitir) }

// --- PoliticaSala -----------------------------------------------------------

// PoliticaSala es el value object de configuración que fija los parámetros
// operativos de una SalaDeEspera: ritmo de admisión, capacidad máxima de la
// cola, ventana de reclamo del turno y modo degradado (tabla 1.3 del
// diseño). Sigue al pie de la letra el patrón de PoliticaOrganizacion de
// Tenencia: rangos admisibles y defaults en código, VO validado en el
// constructor, ajustable sin migración dentro de esos rangos.
type PoliticaSala struct {
	ritmoAdmision       RitmoAdmision
	capacidadMaximaCola int64
	ventanaReclamo      time.Duration
	modoDegradado       ModoDegradado
}

// NuevaPoliticaSala valida y construye una PoliticaSala. Invariantes (tabla
// 1.3 del diseño):
//
//   - ritmoAdmision ya validado por NuevoRitmoAdmision (no puede ser zero
//     value)
//   - capacidadMaximaCola ∈ [100, 5 000 000]
//   - ventanaReclamo ∈ [30s, 15min]
//   - modoDegradado ya validado por ModoDegradadoDesde (no puede ser zero
//     value)
func NuevaPoliticaSala(
	ritmoAdmision RitmoAdmision,
	capacidadMaximaCola int64,
	ventanaReclamo time.Duration,
	modoDegradado ModoDegradado,
) (PoliticaSala, error) {
	var violaciones []string

	if ritmoAdmision.EsVacio() {
		violaciones = append(violaciones, "ritmoAdmision no puede estar vacío")
	}
	if capacidadMaximaCola < capacidadMaximaColaMin || capacidadMaximaCola > capacidadMaximaColaMax {
		violaciones = append(violaciones, "capacidadMaximaCola debe estar entre 100 y 5000000")
	}
	if ventanaReclamo < ventanaReclamoMinima || ventanaReclamo > ventanaReclamoMaxima {
		violaciones = append(violaciones, "ventanaReclamo debe estar entre 30 segundos y 15 minutos")
	}
	if modoDegradado.EsVacio() {
		violaciones = append(violaciones, "modoDegradado no puede estar vacío")
	}

	if len(violaciones) > 0 {
		return PoliticaSala{}, &ErrPoliticaSalaInvalida{Motivo: strings.Join(violaciones, "; ")}
	}

	return PoliticaSala{
		ritmoAdmision:       ritmoAdmision,
		capacidadMaximaCola: capacidadMaximaCola,
		ventanaReclamo:      ventanaReclamo,
		modoDegradado:       modoDegradado,
	}, nil
}

// PoliticaSalaPorDefecto devuelve los valores por defecto documentados en la
// tabla del §1.3 del diseño: 50 admisiones/segundo, 500 000 de capacidad
// máxima, 2 minutos de ventana de reclamo y modo degradado "permitir"
// (fail-open), todos ajustables sin migración.
func PoliticaSalaPorDefecto() PoliticaSala {
	ritmo, err := NuevoRitmoAdmision(ritmoAdmisionPorDefecto)
	if err != nil {
		panic("confianza/dominio: PoliticaSalaPorDefecto produce un ritmo inválido: " + err.Error())
	}
	p, err := NuevaPoliticaSala(ritmo, capacidadColaPorDefecto, ventanaReclamoPorDefecto, ModoDegradadoPermitir)
	if err != nil {
		// Los valores por defecto están cubiertos por un test de dominio
		// (TestPoliticaSalaPorDefecto_EsValida); si esto entra en pánico,
		// es un error de programación en esta misma función, no una
		// condición de runtime alcanzable con datos externos.
		panic("confianza/dominio: PoliticaSalaPorDefecto produce una política inválida: " + err.Error())
	}
	return p
}

// RitmoAdmision devuelve el ritmo de admisión vigente.
func (p PoliticaSala) RitmoAdmision() RitmoAdmision { return p.ritmoAdmision }

// CapacidadMaximaCola devuelve el techo de tickets vivos simultáneos.
func (p PoliticaSala) CapacidadMaximaCola() int64 { return p.capacidadMaximaCola }

// VentanaReclamo devuelve la ventana durante la cual un turno admitido
// puede reclamarse antes de caducar.
func (p PoliticaSala) VentanaReclamo() time.Duration { return p.ventanaReclamo }

// ModoDegradado devuelve el comportamiento ante una caída de Redis.
func (p PoliticaSala) ModoDegradado() ModoDegradado { return p.modoDegradado }

// EsVacio indica si el value object nunca fue construido (zero value).
func (p PoliticaSala) EsVacio() bool { return p.ritmoAdmision.EsVacio() }

// ConRitmoAdmision devuelve una copia de la política con un nuevo ritmo de
// admisión, dejando el resto de los parámetros sin cambiar. Es el patrón
// "with" inmutable que usa SalaDeEspera.CambiarRitmo (§3.2 del diseño): el
// ritmo es lo único que cambia en caliente durante un evento; los demás
// parámetros de PoliticaSala no tienen, hoy, un caso de uso que los mute.
func (p PoliticaSala) ConRitmoAdmision(nuevo RitmoAdmision) PoliticaSala {
	p.ritmoAdmision = nuevo
	return p
}
