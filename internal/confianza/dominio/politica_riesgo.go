package dominio

import (
	"strings"
	"time"
)

// Límites estructurales de PoliticaRiesgo (§1.5 y §1.7 del diseño).
const (
	vidaPerfilMinima            = 7 * 24 * time.Hour
	vidaPerfilMaxima            = 2 * 365 * 24 * time.Hour
	maximoOrigenesRecordadosMin = 1
	maximoOrigenesRecordadosMax = 100

	// Valores por defecto (tabla §1.5 del diseño).
	pesoDispositivoDesconocidoPorDefecto = 0.40
	pesoHuellaAusentePorDefecto          = 0.40
	pesoRedDesconocidaPorDefecto         = 0.25
	umbralElevadoPorDefecto              = 0.50
	umbralAltoPorDefecto                 = 0.80
	minimoExitosParaJuzgarPorDefecto     = 3
	vidaPerfilPorDefecto                 = 180 * 24 * time.Hour
	maximoOrigenesRecordadosPorDefecto   = 20
)

// --- ModoRiesgo ---------------------------------------------------------

// ModoRiesgo es el value object de catálogo cerrado que decide si el
// reconocimiento de origen puede cambiar el desenlace de un login (§1.3 y
// §3.3 del diseño). observar es el default (INV-RIES-14): desplegar esta
// extensión no cambia el desenlace de ningún login hasta que alguien pase
// explícitamente a exigir_captcha.
type ModoRiesgo struct {
	valor string
}

var (
	// ModoRiesgoObservar solo registra señales, puntaje y nivel (logs y
	// métricas); nunca agrega fricción. Es el default.
	ModoRiesgoObservar = ModoRiesgo{valor: "observar"}
	// ModoRiesgoExigirCaptcha exige un captcha cuando el nivel de riesgo es
	// al menos "elevado" y la solicitud no trae ya un token de captcha
	// aceptable (§3.1 paso 2.5.e del diseño).
	ModoRiesgoExigirCaptcha = ModoRiesgo{valor: "exigir_captcha"}
)

// ModoRiesgoDesde valida un valor contra el catálogo cerrado de dos modos.
func ModoRiesgoDesde(valor string) (ModoRiesgo, error) {
	v := strings.TrimSpace(valor)
	switch v {
	case ModoRiesgoObservar.valor, ModoRiesgoExigirCaptcha.valor:
		return ModoRiesgo{valor: v}, nil
	default:
		return ModoRiesgo{}, &ErrPoliticaRiesgoInvalida{Motivo: "modo de riesgo desconocido: " + valor}
	}
}

// String devuelve el código canónico del modo.
func (m ModoRiesgo) String() string { return m.valor }

// EsVacio indica si el value object nunca fue construido (zero value).
func (m ModoRiesgo) EsVacio() bool { return m.valor == "" }

// EsIgual compara dos modos por su valor.
func (m ModoRiesgo) EsIgual(otro ModoRiesgo) bool { return m.valor == otro.valor }

// EsObservar indica si el modo es "observar" (no cambia desenlaces).
func (m ModoRiesgo) EsObservar() bool { return m.EsIgual(ModoRiesgoObservar) }

// --- PoliticaRiesgo ------------------------------------------------------

// PoliticaRiesgo es el value object de configuración que fija los pesos,
// umbrales y techos del reconocimiento de origen (§1.3 y §1.5 del diseño).
// Sigue al pie de la letra el patrón de PoliticaOrganizacion (Tenencia) y
// PoliticaSala (Confianza): rangos admisibles y defaults en código, VO
// validado en el constructor, ajustable sin migración.
type PoliticaRiesgo struct {
	pesoDispositivoDesconocido float64
	pesoHuellaAusente          float64
	pesoRedDesconocida         float64
	umbralElevado              float64
	umbralAlto                 float64
	minimoExitosParaJuzgar     int64
	modo                       ModoRiesgo
	vidaPerfil                 time.Duration
	maximoOrigenesRecordados   int
}

// NuevaPoliticaRiesgo valida y construye una PoliticaRiesgo. Invariantes
// (§1.5 y §1.7 del diseño):
//
//   - los tres pesos ∈ [0.0, 1.0]
//   - umbralElevado, umbralAlto ∈ [0.0, 1.0] y umbralElevado < umbralAlto
//   - minimoExitosParaJuzgar ≥ 1
//   - modo ya validado por ModoRiesgoDesde (no puede ser zero value)
//   - vidaPerfil ∈ [7 días, 2 años]
//   - maximoOrigenesRecordados ∈ [1, 100]
func NuevaPoliticaRiesgo(
	pesoDispositivoDesconocido float64,
	pesoHuellaAusente float64,
	pesoRedDesconocida float64,
	umbralElevado float64,
	umbralAlto float64,
	minimoExitosParaJuzgar int64,
	modo ModoRiesgo,
	vidaPerfil time.Duration,
	maximoOrigenesRecordados int,
) (PoliticaRiesgo, error) {
	var violaciones []string

	if pesoDispositivoDesconocido < 0 || pesoDispositivoDesconocido > 1 {
		violaciones = append(violaciones, "pesoDispositivoDesconocido debe estar entre 0.0 y 1.0")
	}
	if pesoHuellaAusente < 0 || pesoHuellaAusente > 1 {
		violaciones = append(violaciones, "pesoHuellaAusente debe estar entre 0.0 y 1.0")
	}
	if pesoRedDesconocida < 0 || pesoRedDesconocida > 1 {
		violaciones = append(violaciones, "pesoRedDesconocida debe estar entre 0.0 y 1.0")
	}
	if umbralElevado < 0 || umbralElevado > 1 {
		violaciones = append(violaciones, "umbralElevado debe estar entre 0.0 y 1.0")
	}
	if umbralAlto < 0 || umbralAlto > 1 {
		violaciones = append(violaciones, "umbralAlto debe estar entre 0.0 y 1.0")
	}
	if umbralElevado >= umbralAlto {
		violaciones = append(violaciones, "umbralElevado debe ser menor que umbralAlto")
	}
	if minimoExitosParaJuzgar < 1 {
		violaciones = append(violaciones, "minimoExitosParaJuzgar debe ser al menos 1")
	}
	if modo.EsVacio() {
		violaciones = append(violaciones, "modo no puede estar vacío")
	}
	if vidaPerfil < vidaPerfilMinima || vidaPerfil > vidaPerfilMaxima {
		violaciones = append(violaciones, "vidaPerfil debe estar entre 7 días y 2 años")
	}
	if maximoOrigenesRecordados < maximoOrigenesRecordadosMin || maximoOrigenesRecordados > maximoOrigenesRecordadosMax {
		violaciones = append(violaciones, "maximoOrigenesRecordados debe estar entre 1 y 100")
	}

	if len(violaciones) > 0 {
		return PoliticaRiesgo{}, &ErrPoliticaRiesgoInvalida{Motivo: strings.Join(violaciones, "; ")}
	}

	return PoliticaRiesgo{
		pesoDispositivoDesconocido: pesoDispositivoDesconocido,
		pesoHuellaAusente:          pesoHuellaAusente,
		pesoRedDesconocida:         pesoRedDesconocida,
		umbralElevado:              umbralElevado,
		umbralAlto:                 umbralAlto,
		minimoExitosParaJuzgar:     minimoExitosParaJuzgar,
		modo:                       modo,
		vidaPerfil:                 vidaPerfil,
		maximoOrigenesRecordados:   maximoOrigenesRecordados,
	}, nil
}

// PoliticaRiesgoPorDefecto devuelve los valores documentados en la tabla
// del §1.5 del diseño: pesos 0.40 (dispositivo_desconocido) / 0.40
// (huella_ausente) / 0.25 (red_desconocida), umbralElevado=0.50,
// umbralAlto=0.80, minimoExitosParaJuzgar=3, modo=observar,
// vidaPerfil=180 días, maximoOrigenesRecordados=20.
func PoliticaRiesgoPorDefecto() PoliticaRiesgo {
	p, err := NuevaPoliticaRiesgo(
		pesoDispositivoDesconocidoPorDefecto,
		pesoHuellaAusentePorDefecto,
		pesoRedDesconocidaPorDefecto,
		umbralElevadoPorDefecto,
		umbralAltoPorDefecto,
		minimoExitosParaJuzgarPorDefecto,
		ModoRiesgoObservar,
		vidaPerfilPorDefecto,
		maximoOrigenesRecordadosPorDefecto,
	)
	if err != nil {
		// Los valores por defecto están cubiertos por un test de dominio
		// (TestPoliticaRiesgoPorDefecto_EsValida); si esto entra en pánico,
		// es un error de programación en esta misma función, no una
		// condición de runtime alcanzable con datos externos. Mismo
		// criterio que PoliticaOrganizacionPorDefecto y
		// PoliticaSalaPorDefecto.
		panic("confianza/dominio: PoliticaRiesgoPorDefecto produce una política inválida: " + err.Error())
	}
	return p
}

// MinimoExitosParaJuzgar devuelve el número mínimo de logins exitosos que
// una cuenta necesita acumulados para que Senales() produzca algo distinto
// de la lista vacía (INV-RIES-04).
func (p PoliticaRiesgo) MinimoExitosParaJuzgar() int64 { return p.minimoExitosParaJuzgar }

// Modo devuelve el ModoRiesgo vigente (observar | exigir_captcha).
func (p PoliticaRiesgo) Modo() ModoRiesgo { return p.modo }

// VidaPerfil devuelve el TTL con el que se refresca el perfil de orígenes
// en cada login exitoso.
func (p PoliticaRiesgo) VidaPerfil() time.Duration { return p.vidaPerfil }

// MaximoOrigenesRecordados devuelve el techo de dispositivos (y, por
// separado, de redes) recordados por cuenta (INV-RIES-11).
func (p PoliticaRiesgo) MaximoOrigenesRecordados() int { return p.maximoOrigenesRecordados }

// UmbralElevado devuelve el puntaje mínimo para el nivel "elevado".
func (p PoliticaRiesgo) UmbralElevado() float64 { return p.umbralElevado }

// UmbralAlto devuelve el puntaje mínimo para el nivel "alto".
func (p PoliticaRiesgo) UmbralAlto() float64 { return p.umbralAlto }

// EsVacia indica si el value object nunca fue construido (zero value).
func (p PoliticaRiesgo) EsVacia() bool { return p.modo.EsVacio() }

// Evaluar suma los pesos de las señales presentes y satura el resultado a
// [0.0, 1.0] (§1.3, §1.4 y §1.5 del diseño). Señales repetidas o
// desconocidas no pueden colarse: SenalRiesgo es un catálogo cerrado de
// tres valores y esta función solo reconoce esos tres; cualquier otro valor
// (inalcanzable hoy salvo por una futura extensión que agregue una cuarta
// señal) simplemente no suma nada.
func (p PoliticaRiesgo) Evaluar(senales []SenalRiesgo) PuntajeRiesgo {
	var suma float64
	for _, s := range senales {
		switch {
		case s.EsIgual(SenalDispositivoDesconocido):
			suma += p.pesoDispositivoDesconocido
		case s.EsIgual(SenalHuellaAusente):
			suma += p.pesoHuellaAusente
		case s.EsIgual(SenalRedDesconocida):
			suma += p.pesoRedDesconocida
		}
	}
	return NuevoPuntajeRiesgo(suma)
}
