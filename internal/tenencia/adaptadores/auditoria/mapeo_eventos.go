package auditoria

import (
	"github.com/r-david1/moterus/internal/tenencia/dominio"
)

// Recursos que emite Tenencia (docs/catalogos/acciones-auditoria.md,
// sección "Contexto Tenencia").
const (
	recursoOrganizacion = "organizacion"
	recursoMembresia    = "membresia"
	recursoInvitacion   = "invitacion"
	recursoAutorizacion = "autorizacion"
)

// Resultados válidos según el catálogo cerrado (CHECK auditoria_resultado_valido).
const (
	resultadoExito    = "exito"
	resultadoFallo    = "fallo"
	resultadoDenegado = "denegado"
)

// filaAuditoria es la proyección mínima que este ACL necesita para
// insertar una fila en la tabla auditoria.
//
// Nota sobre usuarioID (§8 del diseño: "usuario_id = el ejecutor, no el
// objetivo"): varios eventos de dominio de Tenencia (OrganizacionActualizada,
// EstadoOrganizacionCambiado, MiembroInvitado, InvitacionResuelta) no
// transportan un identificador de quién ejecutó la acción — solo el estado
// resultante del agregado — porque tenencia/dominio (ya cerrado) no lo
// almacena como campo del evento. Para esos casos usuarioID queda vacío
// (NULL en la fila de auditoria); para el resto se usa el identificador más
// cercano al "quién" que el propio evento sí transporta (p. ej. el
// destinatario de MiembroAgregado, que en la práctica casi siempre coincide
// con el ejecutor salvo alta administrativa). Ver el resumen de la tarea
// para el detalle de esta limitación heredada del dominio cerrado.
type filaAuditoria struct {
	accion    string
	recurso   string
	recursoID string
	resultado string
	usuarioID string
	detalles  map[string]any
}

// mapearEvento traduce un dominio.EventoDominio de Tenencia a la fila de
// auditoria correspondiente (tabla 1.6 del diseño). El hash-chaining no se
// calcula aquí: lo asigna el trigger auditoria_asignar_cadena en la base de
// datos.
func mapearEvento(e dominio.EventoDominio) (filaAuditoria, bool) {
	switch ev := e.(type) {
	case dominio.OrganizacionCreada:
		return filaAuditoria{
			accion:    "organizacion.creada",
			recurso:   recursoOrganizacion,
			recursoID: ev.IDOrganizacion,
			resultado: resultadoExito,
			usuarioID: ev.CreadaPor,
			detalles:  map[string]any{"alias": ev.Alias, "creada_por": ev.CreadaPor},
		}, true

	case dominio.OrganizacionActualizada:
		return filaAuditoria{
			accion:    "organizacion.actualizada",
			recurso:   recursoOrganizacion,
			recursoID: ev.IDOrganizacion,
			resultado: resultadoExito,
			detalles:  map[string]any{"campos": ev.Campos},
		}, true

	case dominio.EstadoOrganizacionCambiado:
		return filaAuditoria{
			accion:    "organizacion.estado_cambiado",
			recurso:   recursoOrganizacion,
			recursoID: ev.IDOrganizacion,
			resultado: resultadoExito,
			detalles:  map[string]any{"origen": ev.Origen, "destino": ev.Destino, "motivo": ev.Motivo},
		}, true

	case dominio.MiembroAgregado:
		return filaAuditoria{
			accion:    "membresia.creada",
			recurso:   recursoMembresia,
			recursoID: ev.IDMembresia,
			resultado: resultadoExito,
			usuarioID: ev.IDUsuario,
			detalles:  map[string]any{"usuario_id": ev.IDUsuario, "rol": ev.Rol, "via": ev.Via},
		}, true

	case dominio.RolDeMiembroCambiado:
		return filaAuditoria{
			accion:    "membresia.rol_cambiado",
			recurso:   recursoMembresia,
			recursoID: ev.IDMembresia,
			resultado: resultadoExito,
			usuarioID: ev.IDUsuario,
			detalles: map[string]any{
				"usuario_id":   ev.IDUsuario,
				"rol_anterior": ev.RolAnterior,
				"rol_nuevo":    ev.RolNuevo,
			},
		}, true

	case dominio.EstadoMembresiaCambiado:
		return filaAuditoria{
			accion:    "membresia.estado_cambiado",
			recurso:   recursoMembresia,
			recursoID: ev.IDMembresia,
			resultado: resultadoExito,
			usuarioID: ev.IDUsuario,
			detalles:  map[string]any{"usuario_id": ev.IDUsuario, "origen": ev.Origen, "destino": ev.Destino},
		}, true

	case dominio.MiembroRemovido:
		return filaAuditoria{
			accion:    "membresia.removida",
			recurso:   recursoMembresia,
			recursoID: ev.IDMembresia,
			resultado: resultadoExito,
			usuarioID: ev.IDUsuario,
			detalles: map[string]any{
				"usuario_id":            ev.IDUsuario,
				"rol_al_remover":        ev.RolAlRemover,
				"por_iniciativa_propia": ev.PorIniciativaPropia,
			},
		}, true

	case dominio.MiembroInvitado:
		return filaAuditoria{
			accion:    "membresia.invitada",
			recurso:   recursoInvitacion,
			recursoID: ev.IDInvitacion,
			resultado: resultadoExito,
			detalles:  map[string]any{"correo_destinatario": ev.CorreoDestinatario, "rol_propuesto": ev.RolPropuesto},
		}, true

	case dominio.InvitacionResuelta:
		resultado := resultadoExito
		if ev.Resultado == dominio.ResultadoFallo {
			resultado = resultadoFallo
		}
		return filaAuditoria{
			accion:    "membresia.invitacion_resuelta",
			recurso:   recursoInvitacion,
			recursoID: ev.IDInvitacion,
			resultado: resultado,
			detalles:  map[string]any{"desenlace": ev.Desenlace},
		}, true

	case dominio.AutorizacionDenegada:
		return filaAuditoria{
			accion:    "autorizacion.denegada",
			recurso:   recursoAutorizacion,
			resultado: resultadoDenegado,
			usuarioID: ev.IDUsuario,
			detalles:  map[string]any{"permiso": ev.Permiso, "motivo": ev.Motivo, "rol_actual": ev.RolActual},
		}, true

	default:
		return filaAuditoria{}, false
	}
}
