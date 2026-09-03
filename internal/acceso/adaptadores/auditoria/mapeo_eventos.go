package auditoria

import (
	"github.com/r-david1/moterus/internal/acceso/dominio"
)

// recursoSesion / recursoTokenAcceso son los dos recursos que emite Acceso
// (docs/catalogos/acciones-auditoria.md, sección "Contexto Acceso").
const (
	recursoSesion      = "sesion"
	recursoTokenAcceso = "token_acceso"
)

// Resultados válidos según el catálogo cerrado (CHECK auditoria_resultado_valido).
const (
	resultadoExito    = "exito"
	resultadoFallo    = "fallo"
	resultadoDenegado = "denegado"
)

// filaAuditoria es la proyección mínima que este ACL necesita para
// insertar una fila en la tabla auditoria: accion/recurso vienen del
// catálogo cerrado, usuarioID puede ir vacío (p. ej. refresco desconocido:
// no hay sujeto que nombrar), recursoID es el IDAgregado() del evento
// (IDSesion, vacío cuando no se pudo resolver una sesión), y detalles es
// contexto adicional NUNCA sensible: nunca el token de refresco, su hash,
// el JWT ni el jti completo (§8 del diseño — reforzado también por el
// CHECK auditoria_detalles_sin_secretos de la base de datos).
type filaAuditoria struct {
	accion    string
	recurso   string
	recursoID string
	resultado string
	usuarioID string
	detalles  map[string]any
}

// mapearEvento traduce un dominio.EventoDominio de Acceso a la fila de
// auditoria correspondiente (tabla 1.6 del diseño). El hash-chaining no se
// calcula aquí: lo asigna el trigger auditoria_asignar_cadena en la base
// de datos.
func mapearEvento(e dominio.EventoDominio) (filaAuditoria, bool) {
	switch ev := e.(type) {
	case dominio.SesionIniciada:
		return filaAuditoria{
			accion:    "sesion.iniciada",
			recurso:   recursoSesion,
			recursoID: ev.IDSesion,
			resultado: resultadoExito,
			usuarioID: ev.IDUsuario,
			detalles:  map[string]any{"usuario_id": ev.IDUsuario, "expira_absoluto_en": ev.ExpiraAbsolutoEn},
		}, true

	case dominio.SesionRenovada:
		return filaAuditoria{
			accion:    "sesion.renovada",
			recurso:   recursoSesion,
			recursoID: ev.IDSesion,
			resultado: resultadoExito,
			usuarioID: ev.IDUsuario,
			detalles:  map[string]any{"generacion": ev.Generacion},
		}, true

	case dominio.RenovacionRechazada:
		return filaAuditoria{
			accion:    "sesion.renovada",
			recurso:   recursoSesion,
			recursoID: ev.IDSesion,
			resultado: resultadoFallo,
			detalles:  map[string]any{"motivo": ev.Motivo},
		}, true

	case dominio.ReusoRefrescoDetectado:
		return filaAuditoria{
			accion:    "sesion.reuso_refresco_detectado",
			recurso:   recursoSesion,
			recursoID: ev.IDSesion,
			resultado: resultadoDenegado,
			usuarioID: ev.IDUsuario,
			detalles: map[string]any{
				"generacion_presentada": ev.GeneracionPresentada,
				"generacion_vigente":    ev.GeneracionVigente,
			},
		}, true

	case dominio.SesionCerrada:
		return filaAuditoria{
			accion:    "sesion.cerrada",
			recurso:   recursoSesion,
			recursoID: ev.IDSesion,
			resultado: resultadoExito,
			usuarioID: ev.IDUsuario,
			detalles:  map[string]any{"alcance": ev.Alcance, "cantidad": ev.Cantidad},
		}, true

	case dominio.SesionRevocada:
		return filaAuditoria{
			accion:    "sesion.revocada",
			recurso:   recursoSesion,
			recursoID: ev.IDSesion,
			resultado: resultadoExito,
			usuarioID: ev.IDUsuario,
			detalles:  map[string]any{"motivo": ev.Motivo},
		}, true

	case dominio.TokenAccesoRechazado:
		return filaAuditoria{
			accion:    "token_acceso.rechazado",
			recurso:   recursoTokenAcceso,
			recursoID: ev.IDSesion,
			resultado: resultadoDenegado,
			detalles:  map[string]any{"motivo": ev.Motivo},
		}, true

	default:
		return filaAuditoria{}, false
	}
}
