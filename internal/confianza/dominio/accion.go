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
)
