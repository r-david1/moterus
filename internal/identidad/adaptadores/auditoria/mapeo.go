package auditoria

import (
	"github.com/r-david1/moterus/internal/identidad/dominio"
)

// recursoUsuario es el único recurso que emite Identidad hoy (tabla del
// catálogo, docs/catalogos/acciones-auditoria.md): las 9 acciones de
// Identidad afectan siempre al recurso "usuario".
const recursoUsuario = "usuario"

// Resultados válidos según el catálogo cerrado (CHECK auditoria_resultado_valido).
const (
	resultadoExito    = "exito"
	resultadoFallo    = "fallo"
	resultadoDenegado = "denegado"
)

// filaAuditoria es la proyección mínima que este ACL necesita para insertar
// una fila en la tabla auditoria: accion/recurso vienen del catálogo cerrado
// (INV-ID-17), usuarioID puede ir vacío (actor no resuelto), recursoID
// replica el mismo valor que usuarioID salvo que se indique lo contrario, y
// detalles es contexto adicional NUNCA sensible (nunca contraseñas, hashes,
// tokens ni OTPs — reforzado también por el CHECK
// auditoria_detalles_sin_secretos de la migración).
type filaAuditoria struct {
	accion    string
	recurso   string
	recursoID string
	resultado string
	usuarioID string
	detalles  map[string]any
}

// mapearEvento traduce un dominio.EventoDominio de Identidad a la fila de
// auditoria correspondiente (sección 6 del diseño: "el mapeo EventoDominio
// -> (accion, recurso, recurso_id, resultado, detalles) es responsabilidad
// del ACL identidad/adaptadores/auditoria/"). El hash-chaining (secuencia,
// hash_anterior, hash_actual) NO se calcula aquí: lo asigna el trigger
// auditoria_asignar_cadena en la base de datos (ver
// db/migraciones/000002_crear_auditoria.up.sql, sección 5); este adaptador
// solo inserta los campos de negocio.
func mapearEvento(e dominio.EventoDominio) (filaAuditoria, bool) {
	switch ev := e.(type) {
	case dominio.UsuarioRegistrado:
		return filaAuditoria{
			accion:    "usuario.registrado",
			recurso:   recursoUsuario,
			recursoID: ev.IDUsuario,
			resultado: resultadoExito,
			usuarioID: ev.IDUsuario,
			detalles:  map[string]any{"correo_normalizado": ev.CorreoNormalizado},
		}, true

	case dominio.RegistroRechazado:
		// La única emisora actual (RegistrarUsuarioCasoDeUso, paso 1) lo usa
		// exclusivamente para el rechazo de Confianza; por eso el resultado
		// es "denegado". Si en el futuro se emite también para otros
		// rechazos (correo duplicado, contraseña filtrada), esta función
		// deberá poder distinguirlos — hoy el evento de dominio no lleva esa
		// información.
		return filaAuditoria{
			accion:    "usuario.registro_rechazado",
			recurso:   recursoUsuario,
			resultado: resultadoDenegado,
			detalles:  map[string]any{"correo_normalizado": ev.CorreoNormalizado, "motivo": ev.Motivo},
		}, true

	case dominio.AutenticacionExitosa:
		return filaAuditoria{
			accion:    "usuario.login",
			recurso:   recursoUsuario,
			recursoID: ev.IDUsuario,
			resultado: resultadoExito,
			usuarioID: ev.IDUsuario,
			detalles:  map[string]any{"correo_normalizado": ev.CorreoNormalizado},
		}, true

	case dominio.AutenticacionFallida:
		fila := filaAuditoria{
			accion:    "usuario.login",
			recurso:   recursoUsuario,
			resultado: resultadoFallo,
			usuarioID: ev.IDUsuario,
			detalles:  map[string]any{"correo_normalizado": ev.CorreoNormalizado, "motivo": ev.Motivo},
		}
		if ev.IDUsuario != "" {
			fila.recursoID = ev.IDUsuario
		}
		return fila, true

	case dominio.AutenticacionDenegada:
		return filaAuditoria{
			accion:    "usuario.login",
			recurso:   recursoUsuario,
			resultado: resultadoDenegado,
			detalles:  map[string]any{"correo_normalizado": ev.CorreoNormalizado, "motivo": ev.Motivo},
		}, true

	case dominio.SegundoFactorRequerido:
		return filaAuditoria{
			accion:    "usuario.step_up_requerido",
			recurso:   recursoUsuario,
			recursoID: ev.IDUsuario,
			resultado: resultadoExito,
			usuarioID: ev.IDUsuario,
			detalles:  map[string]any{"motivo": ev.Motivo},
		}, true

	case dominio.ContrasenaCambiada:
		return filaAuditoria{
			accion:    "usuario.contrasena_cambiada",
			recurso:   recursoUsuario,
			recursoID: ev.IDUsuario,
			resultado: resultadoExito,
			usuarioID: ev.IDUsuario,
			// detalles no puede quedar como el nil map por defecto: la
			// migración 000002 exige jsonb_typeof(detalles) = 'object', y
			// json.Marshal(map[string]any(nil)) serializa "null", no "{}"
			// (bug real, encontrado durante la verificación de OTP/MFA —
			// ver el caso VerificacionOTPFallida más abajo, que sí lo
			// dispara en producción).
			detalles: map[string]any{},
		}, true

	case dominio.CredencialRehasheada:
		return filaAuditoria{
			accion:    "usuario.credencial_rehasheada",
			recurso:   recursoUsuario,
			recursoID: ev.IDUsuario,
			resultado: resultadoExito,
			usuarioID: ev.IDUsuario,
			detalles:  map[string]any{}, // ver la nota de ContrasenaCambiada arriba
		}, true

	case dominio.CorreoVerificado:
		return filaAuditoria{
			accion:    "usuario.correo_verificado",
			recurso:   recursoUsuario,
			recursoID: ev.IDUsuario,
			resultado: resultadoExito,
			usuarioID: ev.IDUsuario,
			detalles:  map[string]any{"correo_normalizado": ev.CorreoNormalizado},
		}, true

	case dominio.VerificacionCorreoFallida:
		// Mismo criterio que usuario.login (sección 6 del diseño): una sola
		// acción de catálogo (usuario.correo_verificado) cubre éxito
		// (dominio.CorreoVerificado, arriba) y fallo — se distinguen por
		// resultado, no por una acción nueva. usuarioID puede ir vacío
		// (token no encontrado: no se pudo resolver a ningún usuario).
		fila := filaAuditoria{
			accion:    "usuario.correo_verificado",
			recurso:   recursoUsuario,
			resultado: resultadoFallo,
			usuarioID: ev.IDUsuario,
			detalles:  map[string]any{"motivo": ev.Motivo},
		}
		if ev.IDUsuario != "" {
			fila.recursoID = ev.IDUsuario
		}
		return fila, true

	case dominio.EstadoUsuarioCambiado:
		return filaAuditoria{
			accion:    "usuario.estado_cambiado",
			recurso:   recursoUsuario,
			recursoID: ev.IDUsuario,
			resultado: resultadoExito,
			usuarioID: ev.IDUsuario,
			detalles: map[string]any{
				"estado_anterior": ev.EstadoAnterior,
				"estado_nuevo":    ev.EstadoNuevo,
				"motivo":          ev.Motivo,
			},
		}, true

	case dominio.UsuarioConsultado:
		return filaAuditoria{
			accion:    "usuario.consultado",
			recurso:   recursoUsuario,
			recursoID: ev.IDUsuario,
			resultado: resultadoExito,
			usuarioID: ev.IDUsuario,
			detalles:  map[string]any{"solicitante_id": ev.IDSolicitante},
		}, true

	// --- MFA/OTP (docs/design/otp-mfa.md §1.6/§6, migración 000016) --------
	//
	// Las cinco acciones nuevas del catálogo cerrado. INV-MFA-07/INV-ID-17:
	// ningún detalle transporta el secreto TOTP, un código en claro ni un
	// código de respaldo en claro — los cinco eventos de dominio ya
	// garantizan eso estructuralmente (solo llevan IDs y contadores).

	case dominio.FactorMFAHabilitado:
		return filaAuditoria{
			accion:    "usuario.mfa_habilitado",
			recurso:   recursoUsuario,
			recursoID: ev.IDUsuario,
			resultado: resultadoExito,
			usuarioID: ev.IDUsuario,
			detalles:  map[string]any{"factor_id": ev.IDFactor},
		}, true

	case dominio.FactorMFAConfirmado:
		return filaAuditoria{
			accion:    "usuario.mfa_confirmado",
			recurso:   recursoUsuario,
			recursoID: ev.IDUsuario,
			resultado: resultadoExito,
			usuarioID: ev.IDUsuario,
			detalles:  map[string]any{"factor_id": ev.IDFactor},
		}, true

	case dominio.FactorMFADeshabilitado:
		return filaAuditoria{
			accion:    "usuario.mfa_deshabilitado",
			recurso:   recursoUsuario,
			recursoID: ev.IDUsuario,
			resultado: resultadoExito,
			usuarioID: ev.IDUsuario,
			detalles:  map[string]any{"factor_id": ev.IDFactor},
		}, true

	case dominio.CodigoRespaldoConsumido:
		return filaAuditoria{
			accion:    "usuario.codigo_respaldo_consumido",
			recurso:   recursoUsuario,
			recursoID: ev.IDUsuario,
			resultado: resultadoExito,
			usuarioID: ev.IDUsuario,
			detalles:  map[string]any{"factor_id": ev.IDFactor, "codigos_restantes": ev.CodigosRestantes},
		}, true

	case dominio.VerificacionOTPFallida:
		return filaAuditoria{
			accion:    "usuario.otp_verificacion_fallida",
			recurso:   recursoUsuario,
			recursoID: ev.IDUsuario,
			resultado: resultadoFallo,
			usuarioID: ev.IDUsuario,
			// Bug real encontrado durante la verificación de integración de
			// OTP/MFA: sin este campo (nil map por defecto), json.Marshal
			// serializa "null" en vez de "{}", y la migración 000002 exige
			// jsonb_typeof(detalles) = 'object' — CUALQUIER código OTP
			// incorrecto (TOTP, código de respaldo reutilizado, o el código
			// de DeshabilitarMFA) hacía fallar el INSERT de auditoría y
			// devolvía 500 en vez del 401/422 esperado. VerificacionOTPFallida
			// no tiene campos propios más allá de IDUsuario (ya en
			// usuarioID/recursoID), así que un mapa vacío es el contenido
			// correcto, no solo el que evita el crash.
			detalles: map[string]any{},
		}, true

	default:
		return filaAuditoria{}, false
	}
}
