package http

import (
	"context"
	"encoding/base64"

	"github.com/r-david1/moterus/internal/acceso/aplicacion"
	"github.com/r-david1/moterus/internal/acceso/dominio"
	"github.com/r-david1/moterus/internal/acceso/puertos"
)

// ManejadorAcceso agrupa los casos de uso del MVP de Acceso (sección 3 del
// diseño) detrás de los puertos de entrada, nunca de los structs concretos
// de aplicacion. firmador es la única dependencia de un puerto de SALIDA en
// este adaptador: el endpoint JWKS (§7 del diseño) no tiene caso de uso
// propio en acceso/puertos/entrada.go (PublicadorDeLlaves quedó en el
// documento de diseño pero go-aplicacion no lo implementó — ver el informe
// de la tarea), así que el handler llama directamente a
// FirmadorTokensAcceso.LlavesPublicas: es un endpoint puramente de
// infraestructura (serializar JWKS), sin transacción ni auditoría que
// orquestar.
type ManejadorAcceso struct {
	iniciador                puertos.IniciadorDeSesion
	renovador                puertos.RenovadorDeSesion
	cerrador                 puertos.CerradorDeSesiones
	consultor                puertos.ConsultorDeSesiones
	firmador                 puertos.FirmadorTokensAcceso
	completadorSegundoFactor *aplicacion.CompletarSegundoFactorCasoDeUso
}

// NuevoManejadorAcceso construye el manejador HTTP con sus puertos
// inyectados. completadorSegundoFactor es el struct concreto de
// acceso/aplicacion (§3.6 del diseño otp-mfa.md), no un puerto de entrada:
// acceso/puertos/entrada.go no declara ninguno para este caso de uso
// (ComandoCompletarSegundoFactor vive junto al caso de uso por el mismo
// motivo, ver completar_segundo_factor.go) — es la única dependencia de
// este adaptador sobre un tipo concreto de aplicacion, en vez de una
// interfaz de puertos.
func NuevoManejadorAcceso(
	iniciador puertos.IniciadorDeSesion,
	renovador puertos.RenovadorDeSesion,
	cerrador puertos.CerradorDeSesiones,
	consultor puertos.ConsultorDeSesiones,
	firmador puertos.FirmadorTokensAcceso,
	completadorSegundoFactor *aplicacion.CompletarSegundoFactorCasoDeUso,
) *ManejadorAcceso {
	return &ManejadorAcceso{
		iniciador:                iniciador,
		renovador:                renovador,
		cerrador:                 cerrador,
		consultor:                consultor,
		firmador:                 firmador,
		completadorSegundoFactor: completadorSegundoFactor,
	}
}

// IniciarSesion implementa el handler Huma de POST /acceso/sesiones
// (sección 3.1 del diseño, ADR 0009: orquesta el login completo sin
// verificar contraseñas por sí mismo).
func (m *ManejadorAcceso) IniciarSesion(ctx context.Context, in *IniciarSesionInput) (*IniciarSesionOutput, error) {
	resultado, err := m.iniciador.Iniciar(ctx, puertos.ComandoIniciarSesion{
		Correo:       in.Body.Correo,
		Contrasena:   in.Body.Contrasena,
		Origen:       OrigenSolicitudDesdeContexto(ctx),
		TokenCaptcha: in.Body.TokenCaptcha,
	})
	if err != nil {
		return nil, mapearErrorDominio(ctx, err)
	}
	return &IniciarSesionOutput{Body: resultadoSesionRespuestaDesde(resultado)}, nil
}

// RenovarSesion implementa el handler Huma de POST
// /acceso/sesiones/renovaciones (sección 3.2 del diseño). Público a nivel
// de transporte: la credencial que autentica esta petición es el propio
// token de refresco del cuerpo, no un Bearer (§7 del diseño, tabla de
// endpoints: "Auth: refresco (cookie o cuerpo)").
func (m *ManejadorAcceso) RenovarSesion(ctx context.Context, in *RenovarSesionInput) (*RenovarSesionOutput, error) {
	resultado, err := m.renovador.Renovar(ctx, puertos.ComandoRenovarSesion{
		TokenRefresco: in.Body.TokenRefresco,
		Origen:        OrigenSolicitudDesdeContexto(ctx),
	})
	if err != nil {
		return nil, mapearErrorDominio(ctx, err)
	}
	return &RenovarSesionOutput{Body: resultadoSesionRespuestaDesde(resultado)}, nil
}

// CerrarSesionActual implementa el handler Huma de DELETE
// /acceso/sesiones/actual: cierra la sesión del propio token Bearer
// (INV-ACC-23, el IDSesion nunca viene del cliente).
func (m *ManejadorAcceso) CerrarSesionActual(ctx context.Context, _ *CerrarSesionActualInput) (*CerrarSesionActualOutput, error) {
	acceso, ok := AccesoDesdeContexto(ctx)
	if !ok {
		return nil, mapearErrorDominio(ctx, &dominio.ErrSesionNoEncontrada{})
	}
	if err := m.cerrador.Cerrar(ctx, puertos.ComandoCerrarSesion{
		IDSesion:  acceso.IDSesion,
		IDUsuario: acceso.IDUsuario,
		Origen:    OrigenSolicitudDesdeContexto(ctx),
	}); err != nil {
		return nil, mapearErrorDominio(ctx, err)
	}
	return &CerrarSesionActualOutput{}, nil
}

// CerrarSesion implementa el handler Huma de DELETE /acceso/sesiones/{id}:
// cierra una sesión concreta del usuario autenticado. IDUsuario SIEMPRE
// viene del token validado (INV-ACC-23); IDSesion viene del parámetro de
// ruta, y CerrarSesionCasoDeUso.Cerrar verifica que pertenezca al sujeto
// (ErrSesionAjena -> 404, nunca 403).
func (m *ManejadorAcceso) CerrarSesion(ctx context.Context, in *CerrarSesionInput) (*CerrarSesionOutput, error) {
	acceso, ok := AccesoDesdeContexto(ctx)
	if !ok {
		return nil, mapearErrorDominio(ctx, &dominio.ErrSesionNoEncontrada{})
	}
	if err := m.cerrador.Cerrar(ctx, puertos.ComandoCerrarSesion{
		IDSesion:  in.ID,
		IDUsuario: acceso.IDUsuario,
		Origen:    OrigenSolicitudDesdeContexto(ctx),
	}); err != nil {
		return nil, mapearErrorDominio(ctx, err)
	}
	return &CerrarSesionOutput{}, nil
}

// CerrarTodasLasSesiones implementa el handler Huma de DELETE
// /acceso/sesiones: cierra TODAS las sesiones activas del usuario
// autenticado, incluida la actual (decisión de diseño, ver dtos.go).
func (m *ManejadorAcceso) CerrarTodasLasSesiones(ctx context.Context, _ *CerrarTodasLasSesionesInput) (*CerrarTodasLasSesionesOutput, error) {
	acceso, ok := AccesoDesdeContexto(ctx)
	if !ok {
		return nil, mapearErrorDominio(ctx, &dominio.ErrSesionNoEncontrada{})
	}
	if _, err := m.cerrador.CerrarTodas(ctx, puertos.ComandoCerrarTodasLasSesiones{
		IDUsuario: acceso.IDUsuario,
		Origen:    OrigenSolicitudDesdeContexto(ctx),
	}); err != nil {
		return nil, mapearErrorDominio(ctx, err)
	}
	return &CerrarTodasLasSesionesOutput{}, nil
}

// ListarSesiones implementa el handler Huma de GET /acceso/sesiones
// (sección 3.5 del diseño): "dispositivos conectados" del usuario
// autenticado.
func (m *ManejadorAcceso) ListarSesiones(ctx context.Context, _ *ListarSesionesInput) (*ListarSesionesOutput, error) {
	acceso, ok := AccesoDesdeContexto(ctx)
	if !ok {
		return nil, mapearErrorDominio(ctx, &dominio.ErrSesionNoEncontrada{})
	}
	vistas, err := m.consultor.ListarDeUsuario(ctx, puertos.ConsultaSesionesDeUsuario{
		IDUsuario:      acceso.IDUsuario,
		IDSesionActual: acceso.IDSesion,
		Origen:         OrigenSolicitudDesdeContexto(ctx),
	})
	if err != nil {
		return nil, mapearErrorDominio(ctx, err)
	}
	body := make([]vistaSesionRespuesta, 0, len(vistas))
	for _, v := range vistas {
		body = append(body, vistaSesionRespuestaDesde(v))
	}
	return &ListarSesionesOutput{Body: body}, nil
}

// CompletarSegundoFactor implementa el handler Huma de POST
// /acceso/sesiones/segundo-factor (§3.6/§7 del diseño otp-mfa.md). Sin
// middleware de autenticación Bearer: la credencial de este endpoint es el
// propio token de step-up del cuerpo (INV-MFA-03), que el caso de uso valida
// internamente vía EmisorTokenStepUp.Validar — nunca el token de acceso
// normal.
func (m *ManejadorAcceso) CompletarSegundoFactor(ctx context.Context, in *CompletarSegundoFactorInput) (*CompletarSegundoFactorOutput, error) {
	resultado, err := m.completadorSegundoFactor.CompletarSegundoFactor(ctx, aplicacion.ComandoCompletarSegundoFactor{
		TokenStepUp: in.Body.TokenStepUp,
		Codigo:      in.Body.Codigo,
		Origen:      OrigenSolicitudDesdeContexto(ctx),
	})
	if err != nil {
		return nil, mapearErrorDominio(ctx, err)
	}
	return &CompletarSegundoFactorOutput{Body: resultadoSesionRespuestaDesde(resultado)}, nil
}

// JWKS implementa el handler Huma de GET /.well-known/jwks.json (§7 del
// diseño). Público, sin autenticación ni paso por Confianza (ADR 0020 §4:
// "es exactamente su propósito").
func (m *ManejadorAcceso) JWKS(ctx context.Context, _ *JWKSInput) (*JWKSOutput, error) {
	llaves, err := m.firmador.LlavesPublicas(ctx)
	if err != nil {
		return nil, mapearErrorDominio(ctx, err)
	}
	keys := make([]jwkRespuesta, 0, len(llaves))
	for _, l := range llaves {
		keys = append(keys, jwkRespuestaDesde(l))
	}
	return &JWKSOutput{
		CacheControl: "public, max-age=300",
		Body:         jwksRespuesta{Keys: keys},
	}, nil
}

func resultadoSesionRespuestaDesde(r puertos.ResultadoSesion) resultadoSesionRespuesta {
	return resultadoSesionRespuesta{
		TokenAcceso:      r.TokenAcceso,
		TipoToken:        r.TipoToken,
		ExpiraEnSegundos: r.ExpiraEnSegundos,
		TokenRefresco:    r.TokenRefresco,
		RefrescoExpiraEn: r.RefrescoExpiraEn,
		IDSesion:         r.IDSesion,
		IDUsuario:        r.IDUsuario,
		SesionExpiraEn:   r.SesionExpiraEn,
	}
}

func vistaSesionRespuestaDesde(v puertos.VistaSesion) vistaSesionRespuesta {
	return vistaSesionRespuesta{
		ID:                 v.ID,
		EsSesionActual:     v.EsSesionActual,
		Estado:             v.Estado,
		CreadaEn:           v.CreadaEn,
		UltimaRenovacionEn: v.UltimaRenovacionEn,
		ExpiraAbsolutoEn:   v.ExpiraAbsolutoEn,
		IPOrigen:           v.IPOrigen,
		AgenteUsuario:      v.AgenteUsuario,
	}
}

// jwkRespuestaDesde proyecta puertos.LlavePublica (siempre OKP/Ed25519 en
// este MVP, ver adaptadores/jwt/llavero.go) al formato RFC 7517.
func jwkRespuestaDesde(l puertos.LlavePublica) jwkRespuesta {
	r := jwkRespuesta{
		Kid: l.KID,
		Use: "sig",
		Alg: l.Algoritmo,
	}
	switch l.TipoLlave {
	case "OKP":
		r.Kty = "OKP"
		r.Crv = l.Curva
		r.X = base64.RawURLEncoding.EncodeToString(l.Material)
	case "RSA":
		r.Kty = "RSA"
		// N/E quedan vacíos hasta que el llavero cargue una llave RSA real
		// (ver el comentario de cargarLlaveEd25519Privada en
		// adaptadores/jwt/llavero.go); LlavePublica no separa hoy el
		// material RSA en N/E porque no hay ningún productor de ese caso.
	default:
		r.Kty = l.TipoLlave
	}
	return r
}
