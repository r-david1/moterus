package dominio

// Accion identifica el flujo de negocio que se está evaluando. Los
// valores coinciden textualmente con los que ya usa Identidad en
// identidad/puertos.SolicitudEvaluacion.Accion ("login" | "registro") para
// que el ACL identidad/adaptadores/confianza no tenga que traducir nada
// más que el tipo — más AccionReenvioVerificacion, que Identidad no
// declaraba todavía (sección 3.4 del diseño de Identidad no llamaba a
// EvaluadorConfianza desde ReenviarVerificacion; ver ADR 0018 para el
// porqué de esa acción se evalúa desde el adaptador HTTP en vez de desde
// aplicacion).
type Accion string

const (
	AccionLogin               Accion = "login"
	AccionRegistro            Accion = "registro"
	AccionReenvioVerificacion Accion = "reenvio_verificacion"
	// AccionRenovacionSesion y AccionCierreMasivoSesiones son las dos
	// acciones que agrega el contexto Acceso (§0 y §11.2 del diseño de
	// Acceso, docs/design/acceso-bounded-context.md): renovación de sesión
	// (RenovarSesion evalúa Confianza porque Identidad no participa en ese
	// flujo) y cierre masivo de sesiones (operación destructiva que un
	// atacante con un token robado podría usar para molestar a la
	// víctima). Identidad NO usa estas dos acciones.
	AccionRenovacionSesion     Accion = "renovacion_sesion"
	AccionCierreMasivoSesiones Accion = "cierre_masivo_sesiones"
	// AccionCrearOrganizacion, AccionInvitarMiembro y AccionAceptarInvitacion
	// son las tres acciones que agrega el contexto Tenencia (§11.3 del
	// diseño de Tenencia, docs/design/tenencia-bounded-context.md). El
	// ACL tenencia/adaptadores/confianza puebla además
	// puertos.Solicitud.TenantID (antes siempre vacío): es el primer
	// consumidor que resuelve un tenant real en el borde HTTP.
	AccionCrearOrganizacion Accion = "crear_organizacion"
	AccionInvitarMiembro    Accion = "invitar_miembro"
	AccionAceptarInvitacion Accion = "aceptar_invitacion"
)
