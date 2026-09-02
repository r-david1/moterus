package dominio

import (
	"strings"
	"time"
)

// Límites estructurales de vidaTokenAcceso (tabla 1.3 del diseño).
const (
	vidaTokenAccesoMinima = time.Minute
	vidaTokenAccesoMaxima = 60 * time.Minute
)

// PoliticaSesion es el value object de configuración que fija las ventanas
// temporales y el límite de concurrencia de las sesiones de Acceso. Se
// modela como VO de dominio (no como config suelta) para que las relaciones
// entre las ventanas sean una invariante verificable en el constructor, no
// una convención tácita en un .env (tabla 1.3 del diseño).
type PoliticaSesion struct {
	vidaTokenAcceso       time.Duration
	vidaTokenRefresco     time.Duration
	inactividadMaxima     time.Duration
	vidaAbsolutaSesion    time.Duration
	toleranciaReloj       time.Duration
	maximoSesionesActivas int
}

// NuevaPoliticaSesion valida y construye una PoliticaSesion. Invariantes
// (tabla 1.3 del diseño):
//
//   - vidaTokenAcceso ∈ [1min, 60min]
//   - inactividadMaxima ≤ vidaAbsolutaSesion
//   - vidaTokenRefresco ≤ inactividadMaxima
//   - maximoSesionesActivas ≥ 1
//   - toleranciaReloj ≥ 0
//   - inactividadMaxima y vidaAbsolutaSesion estrictamente positivas
func NuevaPoliticaSesion(
	vidaTokenAcceso time.Duration,
	vidaTokenRefresco time.Duration,
	inactividadMaxima time.Duration,
	vidaAbsolutaSesion time.Duration,
	toleranciaReloj time.Duration,
	maximoSesionesActivas int,
) (PoliticaSesion, error) {
	var violaciones []string

	if vidaTokenAcceso < vidaTokenAccesoMinima || vidaTokenAcceso > vidaTokenAccesoMaxima {
		violaciones = append(violaciones, "vidaTokenAcceso debe estar entre 1 y 60 minutos")
	}
	if inactividadMaxima <= 0 {
		violaciones = append(violaciones, "inactividadMaxima debe ser positiva")
	}
	if vidaAbsolutaSesion <= 0 {
		violaciones = append(violaciones, "vidaAbsolutaSesion debe ser positiva")
	}
	if inactividadMaxima > 0 && vidaAbsolutaSesion > 0 && inactividadMaxima > vidaAbsolutaSesion {
		violaciones = append(violaciones, "inactividadMaxima no puede superar vidaAbsolutaSesion")
	}
	if vidaTokenRefresco <= 0 {
		violaciones = append(violaciones, "vidaTokenRefresco debe ser positiva")
	}
	if vidaTokenRefresco > 0 && inactividadMaxima > 0 && vidaTokenRefresco > inactividadMaxima {
		violaciones = append(violaciones, "vidaTokenRefresco no puede superar inactividadMaxima")
	}
	if toleranciaReloj < 0 {
		violaciones = append(violaciones, "toleranciaReloj no puede ser negativa")
	}
	if maximoSesionesActivas < 1 {
		violaciones = append(violaciones, "maximoSesionesActivas debe ser al menos 1")
	}

	if len(violaciones) > 0 {
		return PoliticaSesion{}, &ErrPoliticaSesionInvalida{Motivo: strings.Join(violaciones, "; ")}
	}

	return PoliticaSesion{
		vidaTokenAcceso:       vidaTokenAcceso,
		vidaTokenRefresco:     vidaTokenRefresco,
		inactividadMaxima:     inactividadMaxima,
		vidaAbsolutaSesion:    vidaAbsolutaSesion,
		toleranciaReloj:       toleranciaReloj,
		maximoSesionesActivas: maximoSesionesActivas,
	}, nil
}

// PoliticaSesionPorDefecto devuelve los valores por defecto documentados en
// la tabla del §1.3 del diseño: 10 minutos de token de acceso, 60 segundos
// de tolerancia de reloj, 30 días de token de refresco e inactividad máxima,
// 90 días de vida absoluta, y 10 sesiones concurrentes como máximo. Son
// ajustables sin migración (viven en código/config, no en el esquema); lo
// estructural es la relación entre las ventanas, no estos números concretos
// (ADR candidato 0024).
func PoliticaSesionPorDefecto() PoliticaSesion {
	p, err := NuevaPoliticaSesion(
		10*time.Minute,
		30*24*time.Hour,
		30*24*time.Hour,
		90*24*time.Hour,
		60*time.Second,
		10,
	)
	if err != nil {
		// Los valores por defecto están cubiertos por un test de dominio
		// (TestPoliticaSesionPorDefecto_EsValida); si esto entra en pánico,
		// es un error de programación en esta misma función, no una
		// condición de runtime alcanzable con datos externos.
		panic("acceso/dominio: PoliticaSesionPorDefecto produce una política inválida: " + err.Error())
	}
	return p
}

// VidaTokenAcceso devuelve la vida útil del token de acceso (JWT).
func (p PoliticaSesion) VidaTokenAcceso() time.Duration { return p.vidaTokenAcceso }

// VidaTokenRefresco devuelve la vida útil de cada token de refresco emitido.
func (p PoliticaSesion) VidaTokenRefresco() time.Duration { return p.vidaTokenRefresco }

// InactividadMaxima devuelve la ventana deslizante de inactividad: se
// reinicia en cada renovación exitosa, siempre acotada por
// ExpiraAbsolutoEn (INV-ACC-16).
func (p PoliticaSesion) InactividadMaxima() time.Duration { return p.inactividadMaxima }

// VidaAbsolutaSesion devuelve la vida absoluta de la sesión, que no se
// extiende jamás por renovación (INV-ACC-16).
func (p PoliticaSesion) VidaAbsolutaSesion() time.Duration { return p.vidaAbsolutaSesion }

// ToleranciaReloj devuelve el desfase aceptado al validar `exp`/`nbf`.
func (p PoliticaSesion) ToleranciaReloj() time.Duration { return p.toleranciaReloj }

// MaximoSesionesActivas devuelve el número máximo de sesiones activas
// simultáneas por usuario (INV-ACC-22).
func (p PoliticaSesion) MaximoSesionesActivas() int { return p.maximoSesionesActivas }

// CalcularExpiraInactividad implementa el servicio de dominio
// PoliticaRotacion (§1.4 del diseño): dada la hora actual y la vida
// absoluta ya fijada de la sesión, calcula la nueva ventana de
// inactividad como min(ahora + inactividadMaxima, expiraAbsolutoEn). El
// mínimo es la garantía dura de INV-ACC-16: ninguna renovación puede
// empujar la sesión más allá de su vida absoluta.
func CalcularExpiraInactividad(ahora, expiraAbsolutoEn time.Time, politica PoliticaSesion) time.Time {
	return minTime(ahora.Add(politica.inactividadMaxima), expiraAbsolutoEn)
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

// ErrPoliticaSesionInvalida se produce al construir una PoliticaSesion que
// incumple alguna de sus invariantes. Falla al arrancar el proceso (config
// inválida), no en caliente.
type ErrPoliticaSesionInvalida struct{ Motivo string }

func (e *ErrPoliticaSesionInvalida) Error() string {
	return "política de sesión inválida: " + e.Motivo
}
