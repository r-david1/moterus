package dominio

import "strings"

// SenalRiesgo es el value object de catálogo cerrado de las señales de
// riesgo de origen que este MVP puede producir (§1.1, §1.3 y §1.4 del
// diseño). Cerrado por el mismo motivo que DesenlaceDeAdmision (colas
// virtuales) y MotivoDenegacion (Tenencia): sin catálogo cerrado no se
// puede responder "¿cuántos logins de la semana pasada dispararon
// huella_ausente?" sin parsear texto libre.
type SenalRiesgo struct {
	valor string
}

var (
	// SenalDispositivoDesconocido se produce cuando la petición trae huella
	// de dispositivo pero esa huella no está en el perfil de la cuenta.
	SenalDispositivoDesconocido = SenalRiesgo{valor: "dispositivo_desconocido"}
	// SenalHuellaAusente se produce cuando la petición no trae huella de
	// dispositivo, pero la cuenta sí venía mandándola en sus logins
	// exitosos anteriores (§1.4: mira exitosConHuella, no exitos).
	SenalHuellaAusente = SenalRiesgo{valor: "huella_ausente"}
	// SenalRedDesconocida se produce cuando el prefijo de red de la
	// petición no está en el perfil de la cuenta.
	SenalRedDesconocida = SenalRiesgo{valor: "red_desconocida"}
)

// SenalRiesgoDesde valida un valor contra el catálogo cerrado de tres
// señales.
func SenalRiesgoDesde(valor string) (SenalRiesgo, error) {
	v := strings.TrimSpace(valor)
	switch v {
	case SenalDispositivoDesconocido.valor, SenalHuellaAusente.valor, SenalRedDesconocida.valor:
		return SenalRiesgo{valor: v}, nil
	default:
		return SenalRiesgo{}, &ErrSenalRiesgoDesconocida{Valor: valor}
	}
}

// String devuelve el código canónico de la señal.
func (s SenalRiesgo) String() string { return s.valor }

// EsVacia indica si el value object nunca fue construido (zero value).
func (s SenalRiesgo) EsVacia() bool { return s.valor == "" }

// EsIgual compara dos señales por su valor.
func (s SenalRiesgo) EsIgual(otra SenalRiesgo) bool { return s.valor == otra.valor }

// --- PuntajeRiesgo -----------------------------------------------------------

// PuntajeRiesgo es el value object que envuelve un puntaje de riesgo de
// origen en [0.0, 1.0] (§1.3 del diseño). A diferencia de casi todos los
// demás constructores de este paquete, NuevoPuntajeRiesgo nunca falla: se
// satura al límite más cercano. Un error de dominio en el camino caliente
// por un redondeo de punto flotante que se pasó de 1.0 sería absurdo — la
// suma ponderada de PoliticaRiesgo.Evaluar puede excederse por construcción
// si algún día se agregan más señales, y saturar es la respuesta correcta.
type PuntajeRiesgo struct {
	valor float64
}

// NuevoPuntajeRiesgo construye un PuntajeRiesgo saturando el valor recibido
// a [0.0, 1.0].
func NuevoPuntajeRiesgo(valor float64) PuntajeRiesgo {
	if valor < 0 {
		valor = 0
	}
	if valor > 1 {
		valor = 1
	}
	return PuntajeRiesgo{valor: valor}
}

// Valor devuelve el puntaje, ya saturado a [0.0, 1.0].
func (p PuntajeRiesgo) Valor() float64 { return p.valor }

// Nivel clasifica el puntaje según los dos umbrales de la política dada
// (§1.3 del diseño: misma forma exacta que EvaluarPuntajeCaptcha, con dos
// umbrales — se copia la forma a propósito, para que el contexto tenga un
// solo idioma de "puntaje con dos umbrales" y no dos).
func (p PuntajeRiesgo) Nivel(politica PoliticaRiesgo) NivelRiesgo {
	if p.valor >= politica.umbralAlto {
		return NivelRiesgoAlto
	}
	if p.valor >= politica.umbralElevado {
		return NivelRiesgoElevado
	}
	return NivelRiesgoNormal
}

// --- NivelRiesgo -------------------------------------------------------------

// NivelRiesgo es el value object de catálogo cerrado que clasifica un
// PuntajeRiesgo (§1.3 del diseño): normal < elevado < alto.
type NivelRiesgo struct {
	valor string
	orden int
}

var (
	// NivelRiesgoNormal es el nivel de nacimiento: puntaje por debajo de
	// umbralElevado.
	NivelRiesgoNormal = NivelRiesgo{valor: "normal", orden: 0}
	// NivelRiesgoElevado es el nivel intermedio: puntaje en
	// [umbralElevado, umbralAlto).
	NivelRiesgoElevado = NivelRiesgo{valor: "elevado", orden: 1}
	// NivelRiesgoAlto es el nivel máximo: puntaje ≥ umbralAlto.
	NivelRiesgoAlto = NivelRiesgo{valor: "alto", orden: 2}
)

// String devuelve el código canónico del nivel.
func (n NivelRiesgo) String() string { return n.valor }

// EsVacio indica si el value object nunca fue construido (zero value).
func (n NivelRiesgo) EsVacio() bool { return n.valor == "" }

// EsIgual compara dos niveles por su valor.
func (n NivelRiesgo) EsIgual(otro NivelRiesgo) bool { return n.valor == otro.valor }

// AlMenos indica si este nivel es igual o más severo que el dado (p. ej.
// NivelRiesgoAlto.AlMenos(NivelRiesgoElevado) es true). Es la comparación
// que necesita el paso 2.5 de EvaluarTrustSignal (§3.1 del diseño: "nivel >=
// elevado").
func (n NivelRiesgo) AlMenos(otro NivelRiesgo) bool { return n.orden >= otro.orden }
