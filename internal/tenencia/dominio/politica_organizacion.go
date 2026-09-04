package dominio

import (
	"strings"
	"time"
)

// Límites estructurales de vigenciaInvitacion (tabla 1.3 del diseño).
const (
	vigenciaInvitacionMinima = time.Hour
	vigenciaInvitacionMaxima = 30 * 24 * time.Hour
)

// PoliticaOrganizacion es el value object de configuración que fija los
// techos anti-abuso del contexto Tenencia: vigencia de las invitaciones y
// los tres límites de cuota (miembros activos, invitaciones pendientes,
// organizaciones por usuario). Se modela como VO de dominio (no como config
// suelta) para que las relaciones entre sus campos sean una invariante
// verificable en el constructor, no una convención tácita en un .env
// (mismo criterio que PoliticaSesion en acceso/dominio).
type PoliticaOrganizacion struct {
	vigenciaInvitacion             time.Duration
	maximoMiembrosActivos          int
	maximoInvitacionesPendientes   int
	maximoOrganizacionesPorUsuario int
}

// NuevaPoliticaOrganizacion valida y construye una PoliticaOrganizacion.
// Invariantes (tabla 1.3 del diseño):
//
//   - vigenciaInvitacion ∈ [1h, 30d]
//   - maximoMiembrosActivos ≥ 1
//   - maximoInvitacionesPendientes ≥ 1
//   - maximoOrganizacionesPorUsuario ≥ 1
func NuevaPoliticaOrganizacion(
	vigenciaInvitacion time.Duration,
	maximoMiembrosActivos int,
	maximoInvitacionesPendientes int,
	maximoOrganizacionesPorUsuario int,
) (PoliticaOrganizacion, error) {
	var violaciones []string

	if vigenciaInvitacion < vigenciaInvitacionMinima || vigenciaInvitacion > vigenciaInvitacionMaxima {
		violaciones = append(violaciones, "vigenciaInvitacion debe estar entre 1 hora y 30 días")
	}
	if maximoMiembrosActivos < 1 {
		violaciones = append(violaciones, "maximoMiembrosActivos debe ser al menos 1")
	}
	if maximoInvitacionesPendientes < 1 {
		violaciones = append(violaciones, "maximoInvitacionesPendientes debe ser al menos 1")
	}
	if maximoOrganizacionesPorUsuario < 1 {
		violaciones = append(violaciones, "maximoOrganizacionesPorUsuario debe ser al menos 1")
	}

	if len(violaciones) > 0 {
		return PoliticaOrganizacion{}, &ErrPoliticaOrganizacionInvalida{Motivo: strings.Join(violaciones, "; ")}
	}

	return PoliticaOrganizacion{
		vigenciaInvitacion:             vigenciaInvitacion,
		maximoMiembrosActivos:          maximoMiembrosActivos,
		maximoInvitacionesPendientes:   maximoInvitacionesPendientes,
		maximoOrganizacionesPorUsuario: maximoOrganizacionesPorUsuario,
	}, nil
}

// PoliticaOrganizacionPorDefecto devuelve los valores por defecto
// documentados en la tabla del §1.3 del diseño: 7 días de vigencia de
// invitación, 200 miembros activos, 50 invitaciones pendientes y 20
// organizaciones propias por usuario, todos ajustables sin migración (viven
// en código/config, no en el esquema).
func PoliticaOrganizacionPorDefecto() PoliticaOrganizacion {
	p, err := NuevaPoliticaOrganizacion(7*24*time.Hour, 200, 50, 20)
	if err != nil {
		// Los valores por defecto están cubiertos por un test de dominio
		// (TestPoliticaOrganizacionPorDefecto_EsValida); si esto entra en
		// pánico, es un error de programación en esta misma función, no una
		// condición de runtime alcanzable con datos externos.
		panic("tenencia/dominio: PoliticaOrganizacionPorDefecto produce una política inválida: " + err.Error())
	}
	return p
}

// VigenciaInvitacion devuelve la ventana de vigencia de una invitación
// recién creada.
func (p PoliticaOrganizacion) VigenciaInvitacion() time.Duration { return p.vigenciaInvitacion }

// MaximoMiembrosActivos devuelve el techo de miembros activos por
// organización.
func (p PoliticaOrganizacion) MaximoMiembrosActivos() int { return p.maximoMiembrosActivos }

// MaximoInvitacionesPendientes devuelve el techo de invitaciones pendientes
// simultáneas por organización.
func (p PoliticaOrganizacion) MaximoInvitacionesPendientes() int {
	return p.maximoInvitacionesPendientes
}

// MaximoOrganizacionesPorUsuario devuelve el techo de organizaciones
// propias (membresías activas con rol propietario) por usuario.
func (p PoliticaOrganizacion) MaximoOrganizacionesPorUsuario() int {
	return p.maximoOrganizacionesPorUsuario
}
