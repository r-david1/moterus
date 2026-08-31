package puertos

import (
	"context"

	"github.com/r-david1/moterus/internal/confianza/dominio"
)

// Solicitud transporta lo que EvaluadorDeRiesgo necesita para decidir. Es
// deliberadamente más ancho que identidad/puertos.SolicitudEvaluacion (que
// solo tiene Accion/CorreoNormalizado/Origen/TokenCaptcha): aquí IP y
// TenantID viajan como primitivos propios en vez de depender del VO
// dominio.OrigenSolicitud de Identidad — Confianza no debe importar el
// paquete dominio de Identidad (cada contexto tiene su propio lenguaje
// ubicuo, sección 5.3 del diseño de Identidad).
type Solicitud struct {
	Accion            dominio.Accion
	IPOrigen          string
	CorreoNormalizado string
	TokenCaptcha      string
	// TenantID queda declarado para cuando exista resolución de tenant en
	// el borde HTTP (Tenencia todavía no la provee — ver ADR 0018, límite
	// por tenant no cubierto en este hito). Vacío hoy siempre.
	TenantID string
}

// ResultadoIntento informa el desenlace real de la acción evaluada, para
// que EvaluadorDeRiesgo pueda ajustar el estado que guarda entre
// solicitudes (p. ej. resetear el contador de una cuenta tras un login
// exitoso, para no penalizar a un usuario legítimo que erró la contraseña
// una vez).
type ResultadoIntento struct {
	Accion            dominio.Accion
	IPOrigen          string
	CorreoNormalizado string
	Exitoso           bool
}

// EvaluadorDeRiesgo es el puerto de entrada del bounded context Confianza:
// lo implementa aplicacion.EvaluarTrustSignalCasoDeUso y lo consumen (a)
// el ACL identidad/adaptadores/confianza, que lo envuelve detrás de
// identidad/puertos.EvaluadorConfianza para login/registro, y (b) el
// guardián de perímetro de identidad/adaptadores/http para el endpoint de
// reenvío de verificación (que la aplicación de Identidad no evalúa hoy —
// ver ADR 0018).
type EvaluadorDeRiesgo interface {
	Evaluar(ctx context.Context, s Solicitud) (dominio.Decision, error)
	RegistrarResultado(ctx context.Context, r ResultadoIntento) error
}
