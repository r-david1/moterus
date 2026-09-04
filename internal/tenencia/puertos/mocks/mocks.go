// Package mocks contiene test doubles escritos a mano para los puertos de
// salida de Tenencia (puertos/salida.go). Sigue exactamente el patrón de
// internal/acceso/puertos/mocks e internal/identidad/puertos/mocks: un
// struct por interfaz con un campo FnX por método, que el test configura
// solo cuando necesita un comportamiento distinto del valor cero, y un
// comportamiento por defecto razonable cuando no se configura. No se usa un
// generador externo por la misma razón que en Acceso e Identidad: las
// interfaces son pequeñas y el mock a mano es más legible.
package mocks

import (
	"context"
	"strings"
	"time"

	"github.com/r-david1/moterus/internal/tenencia/dominio"
	"github.com/r-david1/moterus/internal/tenencia/puertos"
)

// --- RepositorioOrganizaciones ------------------------------------------------

// RepositorioOrganizaciones es el test double de
// puertos.RepositorioOrganizaciones.
type RepositorioOrganizaciones struct {
	FnGuardar              func(ctx context.Context, o *dominio.Organizacion) error
	FnBuscarPorID          func(ctx context.Context, id dominio.IDOrganizacion) (*dominio.Organizacion, error)
	FnBuscarPorAlias       func(ctx context.Context, a dominio.AliasOrganizacion) (*dominio.Organizacion, error)
	FnCargarParaActualizar func(ctx context.Context, id dominio.IDOrganizacion) (*dominio.Organizacion, error)

	LlamadasGuardar              []*dominio.Organizacion
	LlamadasBuscarPorID          []dominio.IDOrganizacion
	LlamadasBuscarPorAlias       []dominio.AliasOrganizacion
	LlamadasCargarParaActualizar []dominio.IDOrganizacion
}

var _ puertos.RepositorioOrganizaciones = (*RepositorioOrganizaciones)(nil)

func (m *RepositorioOrganizaciones) Guardar(ctx context.Context, o *dominio.Organizacion) error {
	m.LlamadasGuardar = append(m.LlamadasGuardar, o)
	if m.FnGuardar != nil {
		return m.FnGuardar(ctx, o)
	}
	return nil
}

func (m *RepositorioOrganizaciones) BuscarPorID(ctx context.Context, id dominio.IDOrganizacion) (*dominio.Organizacion, error) {
	m.LlamadasBuscarPorID = append(m.LlamadasBuscarPorID, id)
	if m.FnBuscarPorID != nil {
		return m.FnBuscarPorID(ctx, id)
	}
	return nil, nil
}

func (m *RepositorioOrganizaciones) BuscarPorAlias(ctx context.Context, a dominio.AliasOrganizacion) (*dominio.Organizacion, error) {
	m.LlamadasBuscarPorAlias = append(m.LlamadasBuscarPorAlias, a)
	if m.FnBuscarPorAlias != nil {
		return m.FnBuscarPorAlias(ctx, a)
	}
	return nil, nil
}

func (m *RepositorioOrganizaciones) CargarParaActualizar(ctx context.Context, id dominio.IDOrganizacion) (*dominio.Organizacion, error) {
	m.LlamadasCargarParaActualizar = append(m.LlamadasCargarParaActualizar, id)
	if m.FnCargarParaActualizar != nil {
		return m.FnCargarParaActualizar(ctx, id)
	}
	return nil, nil
}

// --- RepositorioMembresias ----------------------------------------------------

// RepositorioMembresias es el test double de puertos.RepositorioMembresias.
type RepositorioMembresias struct {
	FnGuardar                              func(ctx context.Context, m *dominio.Membresia) error
	FnBuscarPorID                          func(ctx context.Context, id dominio.IDMembresia) (*dominio.Membresia, error)
	FnBuscarVigente                        func(ctx context.Context, u dominio.IDUsuario, o dominio.IDOrganizacion) (*dominio.Membresia, error)
	FnListarDeOrganizacion                 func(ctx context.Context, o dominio.IDOrganizacion) ([]*dominio.Membresia, error)
	FnListarDeUsuario                      func(ctx context.Context, u dominio.IDUsuario) ([]*dominio.Membresia, error)
	FnContarPropietariosActivos            func(ctx context.Context, o dominio.IDOrganizacion) (int, error)
	FnContarActivasDeOrganizacion          func(ctx context.Context, o dominio.IDOrganizacion) (int, error)
	FnContarOrganizacionesPropiasDeUsuario func(ctx context.Context, u dominio.IDUsuario) (int, error)

	LlamadasGuardar                              []*dominio.Membresia
	LlamadasBuscarPorID                          []dominio.IDMembresia
	LlamadasBuscarVigente                        []BuscarVigenteLlamada
	LlamadasListarDeOrganizacion                 []dominio.IDOrganizacion
	LlamadasListarDeUsuario                      []dominio.IDUsuario
	LlamadasContarPropietariosActivos            []dominio.IDOrganizacion
	LlamadasContarActivasDeOrganizacion          []dominio.IDOrganizacion
	LlamadasContarOrganizacionesPropiasDeUsuario []dominio.IDUsuario
}

// BuscarVigenteLlamada captura los argumentos de una llamada a
// BuscarVigente, para que los tests puedan hacer aserciones sobre ellos.
type BuscarVigenteLlamada struct {
	Usuario      dominio.IDUsuario
	Organizacion dominio.IDOrganizacion
}

var _ puertos.RepositorioMembresias = (*RepositorioMembresias)(nil)

func (m *RepositorioMembresias) Guardar(ctx context.Context, mem *dominio.Membresia) error {
	m.LlamadasGuardar = append(m.LlamadasGuardar, mem)
	if m.FnGuardar != nil {
		return m.FnGuardar(ctx, mem)
	}
	return nil
}

func (m *RepositorioMembresias) BuscarPorID(ctx context.Context, id dominio.IDMembresia) (*dominio.Membresia, error) {
	m.LlamadasBuscarPorID = append(m.LlamadasBuscarPorID, id)
	if m.FnBuscarPorID != nil {
		return m.FnBuscarPorID(ctx, id)
	}
	return nil, nil
}

func (m *RepositorioMembresias) BuscarVigente(ctx context.Context, u dominio.IDUsuario, o dominio.IDOrganizacion) (*dominio.Membresia, error) {
	m.LlamadasBuscarVigente = append(m.LlamadasBuscarVigente, BuscarVigenteLlamada{Usuario: u, Organizacion: o})
	if m.FnBuscarVigente != nil {
		return m.FnBuscarVigente(ctx, u, o)
	}
	return nil, nil
}

func (m *RepositorioMembresias) ListarDeOrganizacion(ctx context.Context, o dominio.IDOrganizacion) ([]*dominio.Membresia, error) {
	m.LlamadasListarDeOrganizacion = append(m.LlamadasListarDeOrganizacion, o)
	if m.FnListarDeOrganizacion != nil {
		return m.FnListarDeOrganizacion(ctx, o)
	}
	return nil, nil
}

func (m *RepositorioMembresias) ListarDeUsuario(ctx context.Context, u dominio.IDUsuario) ([]*dominio.Membresia, error) {
	m.LlamadasListarDeUsuario = append(m.LlamadasListarDeUsuario, u)
	if m.FnListarDeUsuario != nil {
		return m.FnListarDeUsuario(ctx, u)
	}
	return nil, nil
}

func (m *RepositorioMembresias) ContarPropietariosActivos(ctx context.Context, o dominio.IDOrganizacion) (int, error) {
	m.LlamadasContarPropietariosActivos = append(m.LlamadasContarPropietariosActivos, o)
	if m.FnContarPropietariosActivos != nil {
		return m.FnContarPropietariosActivos(ctx, o)
	}
	return 0, nil
}

func (m *RepositorioMembresias) ContarActivasDeOrganizacion(ctx context.Context, o dominio.IDOrganizacion) (int, error) {
	m.LlamadasContarActivasDeOrganizacion = append(m.LlamadasContarActivasDeOrganizacion, o)
	if m.FnContarActivasDeOrganizacion != nil {
		return m.FnContarActivasDeOrganizacion(ctx, o)
	}
	return 0, nil
}

func (m *RepositorioMembresias) ContarOrganizacionesPropiasDeUsuario(ctx context.Context, u dominio.IDUsuario) (int, error) {
	m.LlamadasContarOrganizacionesPropiasDeUsuario = append(m.LlamadasContarOrganizacionesPropiasDeUsuario, u)
	if m.FnContarOrganizacionesPropiasDeUsuario != nil {
		return m.FnContarOrganizacionesPropiasDeUsuario(ctx, u)
	}
	return 0, nil
}

// --- RepositorioInvitaciones --------------------------------------------------

// RepositorioInvitaciones es el test double de
// puertos.RepositorioInvitaciones.
type RepositorioInvitaciones struct {
	FnGuardar                        func(ctx context.Context, i *dominio.Invitacion) error
	FnBuscarPorID                    func(ctx context.Context, id dominio.IDInvitacion) (*dominio.Invitacion, error)
	FnBuscarPorHash                  func(ctx context.Context, h dominio.HashTokenInvitacion) (*dominio.Invitacion, error)
	FnBuscarPendiente                func(ctx context.Context, o dominio.IDOrganizacion, c dominio.CorreoDestinatario) (*dominio.Invitacion, error)
	FnListarPendientesDeOrganizacion func(ctx context.Context, o dominio.IDOrganizacion) ([]*dominio.Invitacion, error)
	FnContarPendientesDeOrganizacion func(ctx context.Context, o dominio.IDOrganizacion) (int, error)

	LlamadasGuardar                        []*dominio.Invitacion
	LlamadasBuscarPorID                    []dominio.IDInvitacion
	LlamadasBuscarPorHash                  []dominio.HashTokenInvitacion
	LlamadasBuscarPendiente                []BuscarPendienteLlamada
	LlamadasListarPendientesDeOrganizacion []dominio.IDOrganizacion
	LlamadasContarPendientesDeOrganizacion []dominio.IDOrganizacion
}

// BuscarPendienteLlamada captura los argumentos de una llamada a
// BuscarPendiente, para que los tests puedan hacer aserciones sobre ellos.
type BuscarPendienteLlamada struct {
	Organizacion dominio.IDOrganizacion
	Correo       dominio.CorreoDestinatario
}

var _ puertos.RepositorioInvitaciones = (*RepositorioInvitaciones)(nil)

func (m *RepositorioInvitaciones) Guardar(ctx context.Context, i *dominio.Invitacion) error {
	m.LlamadasGuardar = append(m.LlamadasGuardar, i)
	if m.FnGuardar != nil {
		return m.FnGuardar(ctx, i)
	}
	return nil
}

func (m *RepositorioInvitaciones) BuscarPorID(ctx context.Context, id dominio.IDInvitacion) (*dominio.Invitacion, error) {
	m.LlamadasBuscarPorID = append(m.LlamadasBuscarPorID, id)
	if m.FnBuscarPorID != nil {
		return m.FnBuscarPorID(ctx, id)
	}
	return nil, nil
}

func (m *RepositorioInvitaciones) BuscarPorHash(ctx context.Context, h dominio.HashTokenInvitacion) (*dominio.Invitacion, error) {
	m.LlamadasBuscarPorHash = append(m.LlamadasBuscarPorHash, h)
	if m.FnBuscarPorHash != nil {
		return m.FnBuscarPorHash(ctx, h)
	}
	return nil, nil
}

func (m *RepositorioInvitaciones) BuscarPendiente(ctx context.Context, o dominio.IDOrganizacion, c dominio.CorreoDestinatario) (*dominio.Invitacion, error) {
	m.LlamadasBuscarPendiente = append(m.LlamadasBuscarPendiente, BuscarPendienteLlamada{Organizacion: o, Correo: c})
	if m.FnBuscarPendiente != nil {
		return m.FnBuscarPendiente(ctx, o, c)
	}
	return nil, nil
}

func (m *RepositorioInvitaciones) ListarPendientesDeOrganizacion(ctx context.Context, o dominio.IDOrganizacion) ([]*dominio.Invitacion, error) {
	m.LlamadasListarPendientesDeOrganizacion = append(m.LlamadasListarPendientesDeOrganizacion, o)
	if m.FnListarPendientesDeOrganizacion != nil {
		return m.FnListarPendientesDeOrganizacion(ctx, o)
	}
	return nil, nil
}

func (m *RepositorioInvitaciones) ContarPendientesDeOrganizacion(ctx context.Context, o dominio.IDOrganizacion) (int, error) {
	m.LlamadasContarPendientesDeOrganizacion = append(m.LlamadasContarPendientesDeOrganizacion, o)
	if m.FnContarPendientesDeOrganizacion != nil {
		return m.FnContarPendientesDeOrganizacion(ctx, o)
	}
	return 0, nil
}

// --- Reloj ---------------------------------------------------------------------

// Reloj es el test double de puertos.Reloj. Si FnAhora no está configurado,
// devuelve el campo Fija.
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
	FnNuevoIDOrganizacion func() (dominio.IDOrganizacion, error)
	FnNuevoIDMembresia    func() (dominio.IDMembresia, error)
	FnNuevoIDInvitacion   func() (dominio.IDInvitacion, error)
}

var _ puertos.GeneradorIDs = (*GeneradorIDs)(nil)

func (m *GeneradorIDs) NuevoIDOrganizacion() (dominio.IDOrganizacion, error) {
	if m.FnNuevoIDOrganizacion != nil {
		return m.FnNuevoIDOrganizacion()
	}
	return dominio.IDOrganizacionDesde("018e7e6a-0000-7000-8000-000000000001")
}

func (m *GeneradorIDs) NuevoIDMembresia() (dominio.IDMembresia, error) {
	if m.FnNuevoIDMembresia != nil {
		return m.FnNuevoIDMembresia()
	}
	return dominio.IDMembresiaDesde("018e7e6a-0000-7000-8000-000000000002")
}

func (m *GeneradorIDs) NuevoIDInvitacion() (dominio.IDInvitacion, error) {
	if m.FnNuevoIDInvitacion != nil {
		return m.FnNuevoIDInvitacion()
	}
	return dominio.IDInvitacionDesde("018e7e6a-0000-7000-8000-000000000003")
}

// --- GeneradorTokens ---------------------------------------------------------

// GeneradorTokens es el test double de puertos.GeneradorTokens. Por defecto
// (FnGenerarTokenInvitacion sin configurar) devuelve un token de invitación
// con forma válida.
type GeneradorTokens struct {
	FnGenerarTokenInvitacion func() (dominio.TokenInvitacionPlano, error)

	LlamadasGenerarTokenInvitacion int
}

var _ puertos.GeneradorTokens = (*GeneradorTokens)(nil)

func (m *GeneradorTokens) GenerarTokenInvitacion() (dominio.TokenInvitacionPlano, error) {
	m.LlamadasGenerarTokenInvitacion++
	if m.FnGenerarTokenInvitacion != nil {
		return m.FnGenerarTokenInvitacion()
	}
	return dominio.NuevoTokenInvitacionPlano("mot_inv_" + strings.Repeat("a", 43))
}

// --- UnidadDeTrabajo ------------------------------------------------------

// UnidadDeTrabajo es el test double de puertos.UnidadDeTrabajo. Por defecto
// (FnEjecutar sin configurar) simplemente invoca fn(ctx), simulando una
// transacción que siempre confirma.
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

// --- AlcanceDeTenencia ------------------------------------------------------

// AlcanceDeTenencia es el test double de puertos.AlcanceDeTenencia. Por
// defecto (FnConAlcance sin configurar) devuelve el ctx sin modificar,
// simulando un adaptador que no necesita fijar RLS en el test.
type AlcanceDeTenencia struct {
	FnConAlcance func(ctx context.Context, idUsuario, idOrganizacion string) context.Context

	LlamadasConAlcance []ConAlcanceLlamada
}

// ConAlcanceLlamada captura los argumentos de una llamada a ConAlcance.
type ConAlcanceLlamada struct {
	IDUsuario      string
	IDOrganizacion string
}

var _ puertos.AlcanceDeTenencia = (*AlcanceDeTenencia)(nil)

func (m *AlcanceDeTenencia) ConAlcance(ctx context.Context, idUsuario, idOrganizacion string) context.Context {
	m.LlamadasConAlcance = append(m.LlamadasConAlcance, ConAlcanceLlamada{IDUsuario: idUsuario, IDOrganizacion: idOrganizacion})
	if m.FnConAlcance != nil {
		return m.FnConAlcance(ctx, idUsuario, idOrganizacion)
	}
	return ctx
}

// --- VerificadorDeSujetos ----------------------------------------------------

// VerificadorDeSujetos es el test double de puertos.VerificadorDeSujetos (el
// ACL sobre Identidad). Por defecto (FnEsElegible sin configurar) devuelve
// un sujeto existente y activo: el camino feliz más común al crear una
// membresía.
type VerificadorDeSujetos struct {
	FnEsElegible func(ctx context.Context, idUsuario string) (puertos.SujetoElegible, error)

	LlamadasEsElegible []string
}

var _ puertos.VerificadorDeSujetos = (*VerificadorDeSujetos)(nil)

func (m *VerificadorDeSujetos) EsElegible(ctx context.Context, idUsuario string) (puertos.SujetoElegible, error) {
	m.LlamadasEsElegible = append(m.LlamadasEsElegible, idUsuario)
	if m.FnEsElegible != nil {
		return m.FnEsElegible(ctx, idUsuario)
	}
	return puertos.SujetoElegible{Existe: true, Activo: true}, nil
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
	FnRegistrar func(ctx context.Context, e dominio.EventoDominio, org dominio.IDOrganizacion, origen dominio.OrigenSolicitud) error

	LlamadasRegistrar []EventoAuditado
}

// EventoAuditado captura un evento, la organización y el origen con los que
// se registró.
type EventoAuditado struct {
	Evento         dominio.EventoDominio
	IDOrganizacion dominio.IDOrganizacion
	Origen         dominio.OrigenSolicitud
}

var _ puertos.RegistroAuditoria = (*RegistroAuditoria)(nil)

func (m *RegistroAuditoria) Registrar(ctx context.Context, e dominio.EventoDominio, org dominio.IDOrganizacion, origen dominio.OrigenSolicitud) error {
	m.LlamadasRegistrar = append(m.LlamadasRegistrar, EventoAuditado{Evento: e, IDOrganizacion: org, Origen: origen})
	if m.FnRegistrar != nil {
		return m.FnRegistrar(ctx, e, org, origen)
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

// --- NotificadorInvitaciones -------------------------------------------------

// NotificadorInvitaciones es el test double de
// puertos.NotificadorInvitaciones.
type NotificadorInvitaciones struct {
	FnEnviarInvitacion func(ctx context.Context, destinatario dominio.CorreoDestinatario,
		nombreOrganizacion string, rol dominio.Rol, tokenPlano string, expiraEn time.Time) error

	LlamadasEnviarInvitacion []EnviarInvitacionLlamada
}

// EnviarInvitacionLlamada captura los argumentos de una llamada a
// EnviarInvitacion, para que los tests puedan hacer aserciones sobre ellos
// sin filtrar el token en claro por descuido (es el test quien decide si lo
// inspecciona).
type EnviarInvitacionLlamada struct {
	Destinatario       dominio.CorreoDestinatario
	NombreOrganizacion string
	Rol                dominio.Rol
	TokenPlano         string
	ExpiraEn           time.Time
}

var _ puertos.NotificadorInvitaciones = (*NotificadorInvitaciones)(nil)

func (m *NotificadorInvitaciones) EnviarInvitacion(ctx context.Context, destinatario dominio.CorreoDestinatario,
	nombreOrganizacion string, rol dominio.Rol, tokenPlano string, expiraEn time.Time) error {
	m.LlamadasEnviarInvitacion = append(m.LlamadasEnviarInvitacion, EnviarInvitacionLlamada{
		Destinatario:       destinatario,
		NombreOrganizacion: nombreOrganizacion,
		Rol:                rol,
		TokenPlano:         tokenPlano,
		ExpiraEn:           expiraEn,
	})
	if m.FnEnviarInvitacion != nil {
		return m.FnEnviarInvitacion(ctx, destinatario, nombreOrganizacion, rol, tokenPlano, expiraEn)
	}
	return nil
}
