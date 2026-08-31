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
	// confianza es el mismo puertos.EvaluadorConfianza que ya usan
	// internamente RegistrarUsuarioCasoDeUso y AutenticarUsuarioCasoDeUso
	// (inyectado dos veces: una vez en aplicacion, otra aquí) — se
	// necesita también en este adaptador porque
	// ReenviarVerificacionCasoDeUso (sección 3.4 del diseño) nunca llama a
	// EvaluadorConfianza: ver Guardián de perímetro de ReenviarVerificacion
	// más abajo y ADR 0018 para el porqué de esta asimetría deliberada en
	// vez de tocar la aplicación (ya cerrada) de Identidad.
	confianza puertos.EvaluadorConfianza
}

// NuevoManejadorIdentidad construye el manejador HTTP con sus puertos de
// entrada inyectados. confianza puede ser nil: en ese caso el guardián de
// perímetro de ReenviarVerificacion no hace ninguna evaluación (equivalente
// a un EvaluadorConfianzaNoOp implícito) — ver ReenviarVerificacion.
func NuevoManejadorIdentidad(
	registrador puertos.RegistradorDeUsuarios,
	autenticador puertos.AutenticadorDeCredenciales,
	consultor puertos.ConsultorDeUsuarios,
	verificadorDeCorreo puertos.VerificadorDeCorreo,
	reenviadorDeVerificacion puertos.ReenviadorDeVerificacion,
	confianza puertos.EvaluadorConfianza,
) *ManejadorIdentidad {
	return &ManejadorIdentidad{
		registrador:              registrador,
		autenticador:             autenticador,
		consultor:                consultor,
		verificadorDeCorreo:      verificadorDeCorreo,
		reenviadorDeVerificacion: reenviadorDeVerificacion,
		confianza:                confianza,
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
	vista, err := m.consultor.ObtenerPorID(ctx, puertos.ConsultaUsuarioPorID{
		IDUsuario:     in.ID,
		IDSolicitante: in.IDSolicitante,
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
