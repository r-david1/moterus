package http

import (
	"context"

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
}

// NuevoManejadorIdentidad construye el manejador HTTP con sus puertos de
// entrada inyectados.
func NuevoManejadorIdentidad(
	registrador puertos.RegistradorDeUsuarios,
	autenticador puertos.AutenticadorDeCredenciales,
	consultor puertos.ConsultorDeUsuarios,
	verificadorDeCorreo puertos.VerificadorDeCorreo,
	reenviadorDeVerificacion puertos.ReenviadorDeVerificacion,
) *ManejadorIdentidad {
	return &ManejadorIdentidad{
		registrador:              registrador,
		autenticador:             autenticador,
		consultor:                consultor,
		verificadorDeCorreo:      verificadorDeCorreo,
		reenviadorDeVerificacion: reenviadorDeVerificacion,
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
// agregar ningún manejo de error que rompa esa garantía.
func (m *ManejadorIdentidad) ReenviarVerificacion(ctx context.Context, in *ReenviarVerificacionInput) (*ReenviarVerificacionOutput, error) {
	_ = m.reenviadorDeVerificacion.Reenviar(ctx, puertos.ComandoReenviarVerificacion{
		Correo: in.Body.Correo,
		Origen: origenSolicitudDesdeContexto(ctx),
	})
	return &ReenviarVerificacionOutput{}, nil
}
