// Package mocks contiene test doubles escritos a mano para los puertos de
// salida de Acceso (puertos/salida.go). Sigue exactamente el patrón de
// internal/identidad/puertos/mocks: un struct por interfaz con un campo
// FnX por método, que el test configura solo cuando necesita un
// comportamiento distinto del valor cero, y un comportamiento por defecto
// razonable cuando no se configura. No se usa un generador externo por la
// misma razón que en Identidad: las interfaces son pequeñas y el mock a
// mano es más legible.
package mocks

import (
	"context"
	"time"

	"github.com/r-david1/moterus/internal/acceso/dominio"
	"github.com/r-david1/moterus/internal/acceso/puertos"
)

// --- RepositorioSesiones -----------------------------------------------------

// RepositorioSesiones es el test double de puertos.RepositorioSesiones.
type RepositorioSesiones struct {
	FnGuardar                 func(ctx context.Context, s *dominio.Sesion) error
	FnBuscarPorID             func(ctx context.Context, id dominio.IDSesion) (*dominio.Sesion, error)
	FnBuscarPorHashRefresco   func(ctx context.Context, h dominio.HashTokenRefresco) (*dominio.Sesion, puertos.SituacionRefresco, int, error)
	FnListarActivasDeUsuario  func(ctx context.Context, u dominio.IDUsuario) ([]*dominio.Sesion, error)
	FnContarActivasDeUsuario  func(ctx context.Context, u dominio.IDUsuario) (int, error)
	FnRevocarActivasDeUsuario func(ctx context.Context, u dominio.IDUsuario, excepto dominio.IDSesion,
		motivo dominio.MotivoRevocacion, ahora time.Time) ([]dominio.IDSesion, error)

	LlamadasGuardar                 []*dominio.Sesion
	LlamadasBuscarPorID             []dominio.IDSesion
	LlamadasBuscarPorHashRefresco   []dominio.HashTokenRefresco
	LlamadasListarActivasDeUsuario  []dominio.IDUsuario
	LlamadasContarActivasDeUsuario  []dominio.IDUsuario
	LlamadasRevocarActivasDeUsuario []RevocarActivasDeUsuarioLlamada
}

// RevocarActivasDeUsuarioLlamada captura los argumentos de una llamada a
// RevocarActivasDeUsuario, para que los tests puedan hacer aserciones
// sobre ellos.
type RevocarActivasDeUsuarioLlamada struct {
	Usuario dominio.IDUsuario
	Excepto dominio.IDSesion
	Motivo  dominio.MotivoRevocacion
	Ahora   time.Time
}

var _ puertos.RepositorioSesiones = (*RepositorioSesiones)(nil)

func (m *RepositorioSesiones) Guardar(ctx context.Context, s *dominio.Sesion) error {
	m.LlamadasGuardar = append(m.LlamadasGuardar, s)
	if m.FnGuardar != nil {
		return m.FnGuardar(ctx, s)
	}
	return nil
}

func (m *RepositorioSesiones) BuscarPorID(ctx context.Context, id dominio.IDSesion) (*dominio.Sesion, error) {
	m.LlamadasBuscarPorID = append(m.LlamadasBuscarPorID, id)
	if m.FnBuscarPorID != nil {
		return m.FnBuscarPorID(ctx, id)
	}
	return nil, nil
}

func (m *RepositorioSesiones) BuscarPorHashRefresco(ctx context.Context, h dominio.HashTokenRefresco) (*dominio.Sesion, puertos.SituacionRefresco, int, error) {
	m.LlamadasBuscarPorHashRefresco = append(m.LlamadasBuscarPorHashRefresco, h)
	if m.FnBuscarPorHashRefresco != nil {
		return m.FnBuscarPorHashRefresco(ctx, h)
	}
	return nil, puertos.RefrescoDesconocido, 0, nil
}

func (m *RepositorioSesiones) ListarActivasDeUsuario(ctx context.Context, u dominio.IDUsuario) ([]*dominio.Sesion, error) {
	m.LlamadasListarActivasDeUsuario = append(m.LlamadasListarActivasDeUsuario, u)
	if m.FnListarActivasDeUsuario != nil {
		return m.FnListarActivasDeUsuario(ctx, u)
	}
	return nil, nil
}

func (m *RepositorioSesiones) ContarActivasDeUsuario(ctx context.Context, u dominio.IDUsuario) (int, error) {
	m.LlamadasContarActivasDeUsuario = append(m.LlamadasContarActivasDeUsuario, u)
	if m.FnContarActivasDeUsuario != nil {
		return m.FnContarActivasDeUsuario(ctx, u)
	}
	return 0, nil
}

func (m *RepositorioSesiones) RevocarActivasDeUsuario(ctx context.Context, u dominio.IDUsuario, excepto dominio.IDSesion,
	motivo dominio.MotivoRevocacion, ahora time.Time) ([]dominio.IDSesion, error) {
	m.LlamadasRevocarActivasDeUsuario = append(m.LlamadasRevocarActivasDeUsuario, RevocarActivasDeUsuarioLlamada{
		Usuario: u, Excepto: excepto, Motivo: motivo, Ahora: ahora,
	})
	if m.FnRevocarActivasDeUsuario != nil {
		return m.FnRevocarActivasDeUsuario(ctx, u, excepto, motivo, ahora)
	}
	return nil, nil
}

// --- FirmadorTokensAcceso -----------------------------------------------------

// FirmadorTokensAcceso es el test double de puertos.FirmadorTokensAcceso.
// Por defecto (FnFirmar sin configurar) devuelve un JWT compacto de
// prueba, no vacío, para que los tests a los que no les interesa el valor
// concreto no tengan que configurarlo.
type FirmadorTokensAcceso struct {
	FnFirmar         func(ctx context.Context, r dominio.ReclamacionesAcceso) (string, error)
	FnVerificar      func(ctx context.Context, tokenCompacto string) (dominio.ReclamacionesAcceso, error)
	FnLlavesPublicas func(ctx context.Context) ([]puertos.LlavePublica, error)

	LlamadasFirmar    []dominio.ReclamacionesAcceso
	LlamadasVerificar []string
}

var _ puertos.FirmadorTokensAcceso = (*FirmadorTokensAcceso)(nil)

func (m *FirmadorTokensAcceso) Firmar(ctx context.Context, r dominio.ReclamacionesAcceso) (string, error) {
	m.LlamadasFirmar = append(m.LlamadasFirmar, r)
	if m.FnFirmar != nil {
		return m.FnFirmar(ctx, r)
	}
	return "jwt-de-prueba-compacto", nil
}

func (m *FirmadorTokensAcceso) Verificar(ctx context.Context, tokenCompacto string) (dominio.ReclamacionesAcceso, error) {
	m.LlamadasVerificar = append(m.LlamadasVerificar, tokenCompacto)
	if m.FnVerificar != nil {
		return m.FnVerificar(ctx, tokenCompacto)
	}
	return dominio.ReclamacionesAcceso{}, nil
}

func (m *FirmadorTokensAcceso) LlavesPublicas(ctx context.Context) ([]puertos.LlavePublica, error) {
	if m.FnLlavesPublicas != nil {
		return m.FnLlavesPublicas(ctx)
	}
	return nil, nil
}

// --- GeneradorTokensRefresco --------------------------------------------------

// GeneradorTokensRefresco es el test double de
// puertos.GeneradorTokensRefresco. Por defecto (FnGenerar sin configurar)
// devuelve un token de refresco con forma válida.
type GeneradorTokensRefresco struct {
	FnGenerar func() (dominio.TokenRefrescoPlano, error)

	LlamadasGenerar int
}

var _ puertos.GeneradorTokensRefresco = (*GeneradorTokensRefresco)(nil)

func (m *GeneradorTokensRefresco) Generar() (dominio.TokenRefrescoPlano, error) {
	m.LlamadasGenerar++
	if m.FnGenerar != nil {
		return m.FnGenerar()
	}
	return dominio.NuevoTokenRefrescoPlano("mot_rt_0123456789012345678901234567890123456789012")
}

// --- ListaRevocacion -----------------------------------------------------

// ListaRevocacion es el test double de puertos.ListaRevocacion. Por
// defecto Disponible() devuelve true y SesionRevocada() devuelve false,
// simulando Redis disponible y ninguna sesión revocada.
type ListaRevocacion struct {
	FnRevocarSesion  func(ctx context.Context, idSesion dominio.IDSesion, hasta time.Time) error
	FnSesionRevocada func(ctx context.Context, idSesion dominio.IDSesion) (bool, error)
	FnDisponible     func() bool

	LlamadasRevocarSesion  []RevocarSesionLlamada
	LlamadasSesionRevocada []dominio.IDSesion
}

// RevocarSesionLlamada captura los argumentos de una llamada a
// RevocarSesion.
type RevocarSesionLlamada struct {
	IDSesion dominio.IDSesion
	Hasta    time.Time
}

var _ puertos.ListaRevocacion = (*ListaRevocacion)(nil)

func (m *ListaRevocacion) RevocarSesion(ctx context.Context, idSesion dominio.IDSesion, hasta time.Time) error {
	m.LlamadasRevocarSesion = append(m.LlamadasRevocarSesion, RevocarSesionLlamada{IDSesion: idSesion, Hasta: hasta})
	if m.FnRevocarSesion != nil {
		return m.FnRevocarSesion(ctx, idSesion, hasta)
	}
	return nil
}

func (m *ListaRevocacion) SesionRevocada(ctx context.Context, idSesion dominio.IDSesion) (bool, error) {
	m.LlamadasSesionRevocada = append(m.LlamadasSesionRevocada, idSesion)
	if m.FnSesionRevocada != nil {
		return m.FnSesionRevocada(ctx, idSesion)
	}
	return false, nil
}

func (m *ListaRevocacion) Disponible() bool {
	if m.FnDisponible != nil {
		return m.FnDisponible()
	}
	return true
}

// --- Reloj ---------------------------------------------------------------------

// Reloj es el test double de puertos.Reloj. Si FnAhora no está
// configurado, devuelve el campo Fija.
type Reloj struct {
	FnAhora func() time.Time
	Fija    time.Time
}

var _ puertos.Reloj = (*Reloj)(nil)

func (m *Reloj) Ahora() time.Time {
	if m.FnAhora != nil {
		return m.FnAhora()
	}
	return m.Fija
}

// --- GeneradorIDs ----------------------------------------------------------

// GeneradorIDs es el test double de puertos.GeneradorIDs.
type GeneradorIDs struct {
	FnNuevoIDSesion      func() (dominio.IDSesion, error)
	FnNuevoIDTokenAcceso func() (dominio.IDTokenAcceso, error)
}

var _ puertos.GeneradorIDs = (*GeneradorIDs)(nil)

func (m *GeneradorIDs) NuevoIDSesion() (dominio.IDSesion, error) {
	if m.FnNuevoIDSesion != nil {
		return m.FnNuevoIDSesion()
	}
	return dominio.IDSesionDesde("018e7e6a-0000-7000-8000-000000000001")
}

func (m *GeneradorIDs) NuevoIDTokenAcceso() (dominio.IDTokenAcceso, error) {
	if m.FnNuevoIDTokenAcceso != nil {
		return m.FnNuevoIDTokenAcceso()
	}
	return dominio.IDTokenAccesoDesde("123e4567-e89b-42d3-a456-426614174099")
}

// --- UnidadDeTrabajo ------------------------------------------------------

// UnidadDeTrabajo es el test double de puertos.UnidadDeTrabajo. Por
// defecto (FnEjecutar sin configurar) simplemente invoca fn(ctx),
// simulando una transacción que siempre confirma.
type UnidadDeTrabajo struct {
	FnEjecutar func(ctx context.Context, fn func(ctx context.Context) error) error
}

var _ puertos.UnidadDeTrabajo = (*UnidadDeTrabajo)(nil)

func (m *UnidadDeTrabajo) Ejecutar(ctx context.Context, fn func(ctx context.Context) error) error {
	if m.FnEjecutar != nil {
		return m.FnEjecutar(ctx, fn)
	}
	return fn(ctx)
}

// --- AutenticadorIdentidad -------------------------------------------------

// AutenticadorIdentidad es el test double de puertos.AutenticadorIdentidad
// (el ACL sobre Identidad).
type AutenticadorIdentidad struct {
	FnAutenticar func(ctx context.Context, c puertos.CredencialesSujeto) (puertos.SujetoAutenticado, error)

	LlamadasAutenticar []puertos.CredencialesSujeto
}

var _ puertos.AutenticadorIdentidad = (*AutenticadorIdentidad)(nil)

func (m *AutenticadorIdentidad) Autenticar(ctx context.Context, c puertos.CredencialesSujeto) (puertos.SujetoAutenticado, error) {
	m.LlamadasAutenticar = append(m.LlamadasAutenticar, c)
	if m.FnAutenticar != nil {
		return m.FnAutenticar(ctx, c)
	}
	return puertos.SujetoAutenticado{}, nil
}

// --- ConsultorEstadoSujeto -------------------------------------------------

// ConsultorEstadoSujeto es el test double de puertos.ConsultorEstadoSujeto
// (el ACL sobre Identidad). Por defecto (FnEstadoDe sin configurar)
// devuelve un sujeto existente y activo: el camino feliz más común en
// RenovarSesion.
type ConsultorEstadoSujeto struct {
	FnEstadoDe func(ctx context.Context, idUsuario string) (puertos.EstadoSujeto, error)

	LlamadasEstadoDe []string
}

var _ puertos.ConsultorEstadoSujeto = (*ConsultorEstadoSujeto)(nil)

func (m *ConsultorEstadoSujeto) EstadoDe(ctx context.Context, idUsuario string) (puertos.EstadoSujeto, error) {
	m.LlamadasEstadoDe = append(m.LlamadasEstadoDe, idUsuario)
	if m.FnEstadoDe != nil {
		return m.FnEstadoDe(ctx, idUsuario)
	}
	return puertos.EstadoSujeto{Existe: true, Estado: "activo"}, nil
}

// --- EvaluadorConfianza -----------------------------------------------------

// EvaluadorConfianza es el test double de puertos.EvaluadorConfianza. Por
// defecto (FnEvaluar sin configurar) permite el intento.
type EvaluadorConfianza struct {
	FnEvaluar            func(ctx context.Context, s puertos.SolicitudEvaluacion) (puertos.DecisionConfianza, error)
	FnRegistrarResultado func(ctx context.Context, r puertos.ResultadoIntento) error

	LlamadasEvaluar            []puertos.SolicitudEvaluacion
	LlamadasRegistrarResultado []puertos.ResultadoIntento
}

var _ puertos.EvaluadorConfianza = (*EvaluadorConfianza)(nil)

func (m *EvaluadorConfianza) Evaluar(ctx context.Context, s puertos.SolicitudEvaluacion) (puertos.DecisionConfianza, error) {
	m.LlamadasEvaluar = append(m.LlamadasEvaluar, s)
	if m.FnEvaluar != nil {
		return m.FnEvaluar(ctx, s)
	}
	return puertos.DecisionConfianza{Permitido: true}, nil
}

func (m *EvaluadorConfianza) RegistrarResultado(ctx context.Context, r puertos.ResultadoIntento) error {
	m.LlamadasRegistrarResultado = append(m.LlamadasRegistrarResultado, r)
	if m.FnRegistrarResultado != nil {
		return m.FnRegistrarResultado(ctx, r)
	}
	return nil
}

// --- RegistroAuditoria ------------------------------------------------------

// RegistroAuditoria es el test double de puertos.RegistroAuditoria.
type RegistroAuditoria struct {
	FnRegistrar func(ctx context.Context, e dominio.EventoDominio, origen dominio.OrigenSolicitud) error

	LlamadasRegistrar []EventoAuditado
}

// EventoAuditado captura un evento y el origen con el que se registró.
type EventoAuditado struct {
	Evento dominio.EventoDominio
	Origen dominio.OrigenSolicitud
}

var _ puertos.RegistroAuditoria = (*RegistroAuditoria)(nil)

func (m *RegistroAuditoria) Registrar(ctx context.Context, e dominio.EventoDominio, origen dominio.OrigenSolicitud) error {
	m.LlamadasRegistrar = append(m.LlamadasRegistrar, EventoAuditado{Evento: e, Origen: origen})
	if m.FnRegistrar != nil {
		return m.FnRegistrar(ctx, e, origen)
	}
	return nil
}

// NombresEventos devuelve, en orden, el NombreEvento() de cada evento
// registrado: conveniencia para aserciones legibles en los tests.
func (m *RegistroAuditoria) NombresEventos() []string {
	nombres := make([]string, 0, len(m.LlamadasRegistrar))
	for _, l := range m.LlamadasRegistrar {
		nombres = append(nombres, l.Evento.NombreEvento())
	}
	return nombres
}

// --- PublicadorEventos ------------------------------------------------------

// PublicadorEventos es el test double de puertos.PublicadorEventos.
type PublicadorEventos struct {
	FnPublicar func(ctx context.Context, eventos ...dominio.EventoDominio) error

	LlamadasPublicar [][]dominio.EventoDominio
}

var _ puertos.PublicadorEventos = (*PublicadorEventos)(nil)

func (m *PublicadorEventos) Publicar(ctx context.Context, eventos ...dominio.EventoDominio) error {
	m.LlamadasPublicar = append(m.LlamadasPublicar, eventos)
	if m.FnPublicar != nil {
		return m.FnPublicar(ctx, eventos...)
	}
	return nil
}
