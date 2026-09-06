package http

import (
	"context"
	"log/slog"

	"github.com/r-david1/moterus/internal/identidad/dominio"
	"github.com/r-david1/moterus/internal/identidad/puertos"
)

// ManejadorIdentidad agrupa los casos de uso del MVP de Identidad (secciones
// 3.1-3.4 del diseño) detrás de los puertos de entrada, nunca de los structs
// concretos de aplicacion (INV-ID-19 por extensión: el adaptador tampoco
// debería depender de más que el contrato).
type ManejadorIdentidad struct {
	registrador              puertos.RegistradorDeUsuarios
	autenticador             puertos.AutenticadorDeCredenciales
	consultor                puertos.ConsultorDeUsuarios
	verificadorDeCorreo      puertos.VerificadorDeCorreo
	reenviadorDeVerificacion puertos.ReenviadorDeVerificacion
	// gestorMFA implementa los tres endpoints de autoservicio de MFA (§7 del
	// diseño otp-mfa.md): Habilitar/ConfirmarFactor/Deshabilitar.
	gestorMFA puertos.GestorDeMFA
	// confianza es el mismo puertos.EvaluadorConfianza que ya usan
	// internamente RegistrarUsuarioCasoDeUso y AutenticarUsuarioCasoDeUso
	// (inyectado dos veces: una vez en aplicacion, otra aquí) — se
	// necesita también en este adaptador porque
	// ReenviarVerificacionCasoDeUso (sección 3.4 del diseño) nunca llama a
	// EvaluadorConfianza: ver Guardián de perímetro de ReenviarVerificacion
	// más abajo y ADR 0018 para el porqué de esta asimetría deliberada en
	// vez de tocar la aplicación (ya cerrada) de Identidad.
	confianza puertos.EvaluadorConfianza
	// autorizadorConsultas cierra la autorización cruzada de
	// GET /identidad/usuarios/{id} (§11.2 del diseño de Tenencia): puede
	// ir nil (p. ej. tests que ejercitan Identidad de forma aislada sin
	// montar Tenencia, o un despliegue donde Tenencia todavía no está
	// disponible), en cuyo caso ObtenerPorID trata cualquier consulta de
	// un tercero con organizacion_id como no autorizada (fail-closed:
	// 404, nunca 200 por defecto).
	autorizadorConsultas puertos.AutorizadorDeConsultas
}

// NuevoManejadorIdentidad construye el manejador HTTP con sus puertos de
// entrada inyectados. confianza y autorizadorConsultas pueden ir nil (ver
// los comentarios de sus campos en ManejadorIdentidad).
func NuevoManejadorIdentidad(
	registrador puertos.RegistradorDeUsuarios,
	autenticador puertos.AutenticadorDeCredenciales,
	consultor puertos.ConsultorDeUsuarios,
	verificadorDeCorreo puertos.VerificadorDeCorreo,
	reenviadorDeVerificacion puertos.ReenviadorDeVerificacion,
	confianza puertos.EvaluadorConfianza,
	autorizadorConsultas puertos.AutorizadorDeConsultas,
	gestorMFA puertos.GestorDeMFA,
) *ManejadorIdentidad {
	return &ManejadorIdentidad{
		registrador:              registrador,
		autenticador:             autenticador,
		consultor:                consultor,
		verificadorDeCorreo:      verificadorDeCorreo,
		reenviadorDeVerificacion: reenviadorDeVerificacion,
		confianza:                confianza,
		autorizadorConsultas:     autorizadorConsultas,
		gestorMFA:                gestorMFA,
	}
}

// Registrar implementa el handler Huma de POST /identidad/usuarios.
func (m *ManejadorIdentidad) Registrar(ctx context.Context, in *RegistroInput) (*RegistroOutput, error) {
	resultado, err := m.registrador.Registrar(ctx, puertos.ComandoRegistrarUsuario{
		Correo:     in.Body.Correo,
		Contrasena: in.Body.Contrasena,
		Origen:     origenSolicitudDesdeContexto(ctx),
	})
	if err != nil {
		return nil, mapearErrorDominio(ctx, err)
	}
	return &RegistroOutput{Body: resultadoRegistroRespuesta{
		IDUsuario:                  resultado.IDUsuario,
		Estado:                     resultado.Estado,
		RequiereVerificacionCorreo: resultado.RequiereVerificacionCorreo,
	}}, nil
}

// Autenticar implementa el handler Huma de POST /identidad/autenticaciones.
// Deliberadamente NO emite token ni cookie de sesión (ADR 0009, INV-ID-14):
// solo expone lo que puertos.ResultadoAutenticacion ya trae. Es el contexto
// Acceso (todavía no implementado) quien debe llamar a
// AutenticadorDeCredenciales.Autenticar y, con el resultado, emitir el JWT.
func (m *ManejadorIdentidad) Autenticar(ctx context.Context, in *AutenticarInput) (*AutenticarOutput, error) {
	resultado, err := m.autenticador.Autenticar(ctx, puertos.ComandoAutenticar{
		Correo:     in.Body.Correo,
		Contrasena: in.Body.Contrasena,
		Origen:     origenSolicitudDesdeContexto(ctx),
	})
	if err != nil {
		return nil, mapearErrorDominio(ctx, err)
	}
	return &AutenticarOutput{Body: resultadoAutenticacionRespuesta{
		IDUsuario:             resultado.IDUsuario,
		CorreoNormalizado:     resultado.CorreoNormalizado,
		Estado:                resultado.Estado,
		RequiereSegundoFactor: resultado.RequiereSegundoFactor,
		MotivoStepUp:          resultado.MotivoStepUp,
		PuntajeConfianza:      resultado.PuntajeConfianza,
	}}, nil
}

// ObtenerPorID implementa el handler Huma de GET /identidad/usuarios/{id}.
// La autorización (¿puede este solicitante ver a este usuario?) no se
// resuelve aquí (sección 3.3 del diseño: es de Tenencia/Acceso); este
// endpoint solo transporta IDSolicitante para la auditoría condicional.
func (m *ManejadorIdentidad) ObtenerPorID(ctx context.Context, in *ConsultaUsuarioInput) (*ConsultaUsuarioOutput, error) {
	// IDSolicitante sale SIEMPRE del token ya validado por
	// middlewareAutenticacionAcceso (§11.1/§11.2 del diseño de Tenencia:
	// INV-TEN-12/INV-ACC-23), nunca de un parámetro de query que el
	// cliente controla. Vacío si el handler se invocó sin pasar por ese
	// middleware (p. ej. un test unitario que ejercita el handler
	// directamente): se trata igual que "no se pudo autenticar al
	// solicitante", nunca como "llamada interna del sistema".
	acceso, autenticado := accesoDesdeContexto(ctx)
	idSolicitante := acceso.IDUsuario

	// Regla de autorización exacta (§11.2 del diseño de Tenencia):
	//  1. El solicitante consulta su propio perfil -> permitido sin
	//     consultar a Tenencia, sigue sin auditarse (comportamiento
	//     inalterado del caso de uso, que compara IDSolicitante==IDUsuario).
	//  2. Solicitante distinto del objetivo y SIN organizacion_id -> 404:
	//     no hay ningún contexto en el que autorizar, y un 403 confirmaría
	//     que el usuario existe.
	//  3. Solicitante distinto del objetivo y CON organizacion_id -> se
	//     exige (a) que el solicitante tenga miembro.ver en esa
	//     organización y (b) que el objetivo sea miembro de la misma
	//     organización (identidad/adaptadores/tenencia.AutorizadorConsultas).
	//     Si cualquiera falla, o si Tenencia no está disponible
	//     (autorizadorConsultas nil) o devuelve error, 404 (fail-closed).
	if autenticado && idSolicitante != "" && idSolicitante != in.ID {
		if in.OrganizacionID == "" {
			return nil, mapearErrorDominio(ctx, &dominio.ErrUsuarioNoEncontrado{IDUsuario: in.ID})
		}
		if m.autorizadorConsultas == nil {
			return nil, mapearErrorDominio(ctx, &dominio.ErrUsuarioNoEncontrado{IDUsuario: in.ID})
		}
		puede, err := m.autorizadorConsultas.PuedeConsultar(ctx, idSolicitante, in.ID, in.OrganizacionID)
		if err != nil || !puede {
			return nil, mapearErrorDominio(ctx, &dominio.ErrUsuarioNoEncontrado{IDUsuario: in.ID})
		}
	}

	vista, err := m.consultor.ObtenerPorID(ctx, puertos.ConsultaUsuarioPorID{
		IDUsuario:     in.ID,
		IDSolicitante: idSolicitante,
		Origen:        origenSolicitudDesdeContexto(ctx),
	})
	if err != nil {
		return nil, mapearErrorDominio(ctx, err)
	}
	return &ConsultaUsuarioOutput{Body: vistaUsuarioRespuesta{
		ID:             vista.ID,
		Correo:         vista.Correo,
		Estado:         vista.Estado,
		TieneMFA:       vista.TieneMFA,
		CreadoEn:       vista.CreadoEn,
		UltimoAccesoEn: vista.UltimoAccesoEn,
	}}, nil
}

// VerificarCorreo implementa el handler Huma de
// POST /identidad/verificaciones-correo (sección 3.4 del diseño). 200 en
// éxito; token inválido/expirado se mapea a 404/410 por
// mapearErrorDominio, la única función que traduce dominio -> status HTTP.
func (m *ManejadorIdentidad) VerificarCorreo(ctx context.Context, in *VerificarCorreoInput) (*VerificarCorreoOutput, error) {
	err := m.verificadorDeCorreo.Verificar(ctx, puertos.ComandoVerificarCorreo{
		TokenPlano: in.Body.Token,
		Origen:     origenSolicitudDesdeContexto(ctx),
	})
	if err != nil {
		return nil, mapearErrorDominio(ctx, err)
	}
	return &VerificarCorreoOutput{}, nil
}

// ReenviarVerificacion implementa el handler Huma de
// POST /identidad/verificaciones-correo/reenvios (sección 3.4 del diseño,
// INV-ID-22). Responde siempre 202 Accepted, exista o no el correo: el
// caso de uso ReenviarVerificacionCasoDeUso.Reenviar ya garantiza que nunca
// devuelve un error observable (siempre nil), así que este handler no debe
// agregar ningún manejo de error que rompa esa garantía salvo el guardián
// de perímetro de abajo, que SÍ puede devolver 429 (ver comentario).
//
// Guardián de perímetro (ADR 0018): a diferencia de Registrar y Autenticar,
// ReenviarVerificacionCasoDeUso (aplicacion, ya cerrada) nunca consulta
// EvaluadorConfianza — el diseño original de la sección 3.4 no incluyó esa
// llamada. En vez de tocar la aplicación cerrada de Identidad para agregar
// un hook que ya existe para los otros dos endpoints, este adaptador HTTP
// llama directamente al mismo puertos.EvaluadorConfianza (mismo motor de
// rate limiting/captcha de Confianza) ANTES de invocar el caso de uso, con
// Accion="reenvio_verificacion". Que este endpoint pueda devolver 429 no
// contradice INV-ID-22 (esa invariante es sobre el DESENLACE DE NEGOCIO —
// no revelar si el correo existe/está verificado/etc — no sobre si el
// perímetro decide ni siquiera dejar pasar la solicitud, igual que un WAF
// bloqueando antes de que la petición llegue al proceso).
func (m *ManejadorIdentidad) ReenviarVerificacion(ctx context.Context, in *ReenviarVerificacionInput) (*ReenviarVerificacionOutput, error) {
	origen := origenSolicitudDesdeContexto(ctx)
	if m.confianza != nil {
		decision, err := m.confianza.Evaluar(ctx, puertos.SolicitudEvaluacion{
			Accion:            "reenvio_verificacion",
			CorreoNormalizado: in.Body.Correo,
			Origen:            origen,
		})
		switch {
		case err != nil:
			// Fail-open (mismo criterio que VerificadorContrasenasFiltradas
			// en RegistrarUsuarioCasoDeUso): una caída del perímetro no
			// puede convertir este endpoint en indisponible.
			slog.WarnContext(ctx, "guardián de perímetro de reenvío de verificación no disponible; continuando fail-open", "error", err)
		case !decision.Permitido:
			return nil, mapearErrorDominio(ctx, &dominio.ErrAccesoDenegadoPorConfianza{
				Motivo:       decision.Motivo,
				ReintentarEn: decision.ReintentarEn,
			})
		}
	}

	_ = m.reenviadorDeVerificacion.Reenviar(ctx, puertos.ComandoReenviarVerificacion{
		Correo: in.Body.Correo,
		Origen: origen,
	})
	return &ReenviarVerificacionOutput{}, nil
}

// HabilitarMFA implementa el handler Huma de
// POST /identidad/usuarios/actual/factores-mfa (§3.1/§7 del diseño
// otp-mfa.md). IDSujeto sale SIEMPRE del token Bearer ya validado
// (accesoDesdeContexto), nunca de un parámetro que el cliente controle
// (mismo criterio que INV-TEN-12/INV-ACC-23). Un handler invocado sin pasar
// por middlewareAutenticacionAcceso (p. ej. un test unitario) produce el
// mismo error de dominio que un IDSujeto vacío: ErrUsuarioNoEncontrado vía
// el propio caso de uso, sin necesitar un chequeo especial aquí.
func (m *ManejadorIdentidad) HabilitarMFA(ctx context.Context, _ *HabilitarMFAInput) (*HabilitarMFAOutput, error) {
	acceso, _ := accesoDesdeContexto(ctx)
	resultado, err := m.gestorMFA.Habilitar(ctx, puertos.ComandoHabilitarMFA{
		IDSujeto: acceso.IDUsuario,
	})
	if err != nil {
		return nil, mapearErrorDominio(ctx, err)
	}
	return &HabilitarMFAOutput{Body: resultadoHabilitarMFARespuesta{
		IDFactor:            resultado.IDFactor,
		SecretoEnClaro:      resultado.SecretoEnClaro,
		URIProvisionamiento: resultado.URIProvisionamiento,
	}}, nil
}

// ConfirmarFactorMFA implementa el handler Huma de
// POST /identidad/usuarios/actual/factores-mfa/confirmacion (§3.2/§7 del
// diseño otp-mfa.md). Mismo criterio que HabilitarMFA: IDSujeto sale del
// token Bearer, nunca del cuerpo.
func (m *ManejadorIdentidad) ConfirmarFactorMFA(ctx context.Context, in *ConfirmarFactorMFAInput) (*ConfirmarFactorMFAOutput, error) {
	acceso, _ := accesoDesdeContexto(ctx)
	resultado, err := m.gestorMFA.ConfirmarFactor(ctx, puertos.ComandoConfirmarFactorMFA{
		IDSujeto: acceso.IDUsuario,
		IDFactor: in.Body.IDFactor,
		Codigo:   in.Body.Codigo,
	})
	if err != nil {
		return nil, mapearErrorDominio(ctx, err)
	}
	return &ConfirmarFactorMFAOutput{Body: resultadoConfirmarMFARespuesta{
		CodigosRespaldo: resultado.CodigosRespaldo,
	}}, nil
}

// DeshabilitarMFA implementa el handler Huma de
// DELETE /identidad/usuarios/actual/factores-mfa (§3.3/§7 del diseño
// otp-mfa.md, ADR 0039): exige un código propio del factor en el cuerpo,
// no basta con el Bearer (INV-MFA-05).
func (m *ManejadorIdentidad) DeshabilitarMFA(ctx context.Context, in *DeshabilitarMFAInput) (*DeshabilitarMFAOutput, error) {
	acceso, _ := accesoDesdeContexto(ctx)
	if err := m.gestorMFA.Deshabilitar(ctx, puertos.ComandoDeshabilitarMFA{
		IDSujeto: acceso.IDUsuario,
		Codigo:   in.Body.Codigo,
	}); err != nil {
		return nil, mapearErrorDominio(ctx, err)
	}
	return &DeshabilitarMFAOutput{}, nil
}
