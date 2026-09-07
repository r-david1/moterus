package auditoria

import (
	"github.com/r-david1/moterus/internal/confianza/dominio"
)

// recursoSalaEspera es el recurso que emiten las tres acciones de la
// migración 000018 (colas de acceso virtual): siempre "sala_espera".
const recursoSalaEspera = "sala_espera"

// recursoOrigen es el recurso de la única acción de la migración 000019
// (reconocimiento de origen, §1.6 de fingerprinting-comportamiento.md):
// siempre "origen".
const recursoOrigen = "origen"

// resultadoExito es el único resultado que producen los tres eventos de
// dominio de Confianza (tabla 1.7 del diseño): abrir, cambiar ritmo y
// cerrar/drenar una sala son siempre operaciones exitosas del punto de
// vista de la auditoría — no hay un "intento fallido de abrir una sala" que
// audite Confianza (los rechazos de validación ocurren antes de construir
// el agregado y nunca llegan a este ACL).
const resultadoExito = "exito"

// filaAuditoria es la proyección mínima que este ACL necesita para insertar
// una fila en la tabla auditoria: accion/recurso vienen del catálogo
// cerrado (migración 000018), usuarioID es el operador que ejecutó la
// mutación (vacío para una sala de alcance sistema operada fuera de la API,
// §7.2 del diseño — la columna usuario_id ya contempla "NULL cuando el
// actor no se resolvió"), y detalles es contexto adicional NUNCA sensible
// (nunca un TicketPlano ni su hash — reforzado también por el CHECK
// auditoria_detalles_sin_secretos de la migración 000002, pero este ACL no
// debe depender de eso, §9 del diseño).
type filaAuditoria struct {
	accion    string
	recurso   string
	recursoID string
	resultado string
	usuarioID string
	detalles  map[string]any
}

// mapearEvento traduce un dominio.EventoDominio de Confianza a la fila de
// auditoria correspondiente (tabla 1.7 del diseño). El hash-chaining
// (secuencia, hash_anterior, hash_actual) NO se calcula aquí: lo asigna el
// trigger auditoria_asignar_cadena en la base de datos.
//
// Ninguno de los tres eventos de la sala de espera transporta quién ejecutó
// la mutación (SalaDeEspera no guarda un "operador actual" como campo de sus
// eventos, a diferencia de otros contextos): usuarioID queda siempre vacío
// (NULL en la fila de auditoria) para esos tres casos. Es una limitación
// heredada del dominio ya cerrado, igual que la nota equivalente en
// identidad/adaptadores/auditoria/mapeo.go. OrigenNuevoObservado es la
// excepción: sí lleva IDUsuario (la cuenta que autenticó), así que su fila
// de auditoria SÍ puebla usuarioID.
func mapearEvento(e dominio.EventoDominio) (filaAuditoria, bool) {
	switch ev := e.(type) {
	case dominio.SalaDeEsperaAbierta:
		return filaAuditoria{
			accion:    "sala_espera.abierta",
			recurso:   recursoSalaEspera,
			recursoID: ev.IDSalaDeEspera,
			resultado: resultadoExito,
			detalles: map[string]any{
				"alias":            ev.Alias,
				"alcance":          ev.Alcance,
				"ruta":             ev.Ruta,
				"ritmo":            ev.RitmoAdmision,
				"capacidad_maxima": ev.CapacidadMaxima,
				"modo_degradado":   ev.ModoDegradado,
			},
		}, true

	case dominio.RitmoDeAdmisionCambiado:
		return filaAuditoria{
			accion:    "sala_espera.ritmo_cambiado",
			recurso:   recursoSalaEspera,
			recursoID: ev.IDSalaDeEspera,
			resultado: resultadoExito,
			detalles: map[string]any{
				"ritmo_anterior":    ev.RitmoAnterior,
				"ritmo_nuevo":       ev.RitmoNuevo,
				"cursor_al_cambiar": ev.CursorAlCambiar,
				"longitud_cola":     ev.LongitudCola,
			},
		}, true

	case dominio.SalaDeEsperaCerrada:
		return filaAuditoria{
			accion:    "sala_espera.cerrada",
			recurso:   recursoSalaEspera,
			recursoID: ev.IDSalaDeEspera,
			resultado: resultadoExito,
			detalles: map[string]any{
				"destino":           ev.Destino,
				"ingresos_totales":  ev.IngresosTotales,
				"admitidos_totales": ev.AdmitidosTotales,
			},
		}, true

	case dominio.OrigenNuevoObservado:
		// Senales nunca debería llegar nil (§1.6 del diseño: el evento solo
		// se construye cuando ya hubo al menos una señal), pero se normaliza
		// de todas formas: un slice nil serializa a JSON "null", no "[]", y
		// aunque eso no viola el CHECK de la tabla (que exige que el propio
		// `detalles` sea un objeto, no que sus valores lo sean), un
		// respondedor de incidentes que filtre por "senales contiene X" no
		// debería tener que distinguir "null" de "[]" en la misma columna.
		senales := ev.Senales
		if senales == nil {
			senales = []string{}
		}
		return filaAuditoria{
			accion:    "origen.nuevo",
			recurso:   recursoOrigen,
			recursoID: ev.IDUsuario,
			resultado: resultadoExito,
			usuarioID: ev.IDUsuario,
			// detalles no puede quedar como el mapa nil por defecto: la
			// migración 000002 exige jsonb_typeof(detalles) = 'object', y
			// json.Marshal(map[string]any(nil)) serializa "null", no "{}"
			// (bug real, encontrado durante la verificación de OTP/MFA —
			// ver identidad/adaptadores/auditoria/mapeo.go, caso
			// ContrasenaCambiada). Por eso se construye siempre el literal
			// completo, nunca una variable `var detalles map[string]any`
			// que dependa de un `if` para poblarse.
			detalles: map[string]any{
				"senales":            senales,
				"puntaje":            ev.Puntaje,
				"nivel":              ev.Nivel,
				"modo":               ev.Modo,
				"origenes_conocidos": ev.OrigenesConocidos,
			},
		}, true

	default:
		return filaAuditoria{}, false
	}
}
