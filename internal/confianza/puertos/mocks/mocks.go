// Package mocks contiene test doubles escritos a mano para los puertos de
// salida de Confianza (puertos/salida.go), siguiendo el mismo patrón que
// identidad/puertos/mocks: un struct por interfaz con un campo FnX
// opcional por método y un comportamiento por defecto razonable cuando no
// se configura.
package mocks

import (
	"context"
	"strings"
	"time"

	"github.com/r-david1/moterus/internal/confianza/dominio"
	"github.com/r-david1/moterus/internal/confianza/puertos"
)

// --- EvaluadorDeRiesgo (entrada) ---------------------------------------------

// EvaluadorDeRiesgo es el test double de puertos.EvaluadorDeRiesgo. Por
// defecto (FnEvaluar sin configurar) siempre permite, sin motivo ni
// reintento: el camino feliz más común para un test de
// PorteroDeSalaCasoDeUso.Ingresar que no está ejercitando el freno de
// farming de tickets (§12 del diseño colas-virtuales.md).
type EvaluadorDeRiesgo struct {
	FnEvaluar            func(ctx context.Context, s puertos.Solicitud) (dominio.Decision, error)
	FnRegistrarResultado func(ctx context.Context, r puertos.ResultadoIntento) error

	LlamadasEvaluar            []puertos.Solicitud
	LlamadasRegistrarResultado []puertos.ResultadoIntento
}

var _ puertos.EvaluadorDeRiesgo = (*EvaluadorDeRiesgo)(nil)

func (m *EvaluadorDeRiesgo) Evaluar(ctx context.Context, s puertos.Solicitud) (dominio.Decision, error) {
	m.LlamadasEvaluar = append(m.LlamadasEvaluar, s)
	if m.FnEvaluar != nil {
		return m.FnEvaluar(ctx, s)
	}
	return dominio.Decision{Permitido: true}, nil
}

func (m *EvaluadorDeRiesgo) RegistrarResultado(ctx context.Context, r puertos.ResultadoIntento) error {
	m.LlamadasRegistrarResultado = append(m.LlamadasRegistrarResultado, r)
	if m.FnRegistrarResultado != nil {
		return m.FnRegistrarResultado(ctx, r)
	}
	return nil
}

// --- LimitadorTasa -----------------------------------------------------------

// LimitadorTasa es el test double de puertos.LimitadorTasa. Por defecto
// (FnPermitir sin configurar) siempre permite, sin restantes ni backoff:
// el camino feliz más común en los tests del caso de uso.
type LimitadorTasa struct {
	FnPermitir  func(ctx context.Context, clave string, umbral dominio.Umbral) (bool, int, time.Duration, error)
	FnReiniciar func(ctx context.Context, clave string) error

	LlamadasPermitir  []LlamadaPermitir
	LlamadasReiniciar []string
}

// LlamadaPermitir captura los argumentos de una llamada a Permitir.
type LlamadaPermitir struct {
	Clave  string
	Umbral dominio.Umbral
}

var _ puertos.LimitadorTasa = (*LimitadorTasa)(nil)

func (m *LimitadorTasa) Permitir(ctx context.Context, clave string, umbral dominio.Umbral) (bool, int, time.Duration, error) {
	m.LlamadasPermitir = append(m.LlamadasPermitir, LlamadaPermitir{Clave: clave, Umbral: umbral})
	if m.FnPermitir != nil {
		return m.FnPermitir(ctx, clave, umbral)
	}
	return true, umbral.Limite, 0, nil
}

func (m *LimitadorTasa) Reiniciar(ctx context.Context, clave string) error {
	m.LlamadasReiniciar = append(m.LlamadasReiniciar, clave)
	if m.FnReiniciar != nil {
		return m.FnReiniciar(ctx, clave)
	}
	return nil
}

// --- VerificadorCaptcha --------------------------------------------------

// VerificadorCaptcha es el test double de puertos.VerificadorCaptcha. Por
// defecto (FnVerificar sin configurar) devuelve puntaje 1.0 sin error.
type VerificadorCaptcha struct {
	FnVerificar func(ctx context.Context, token, accion, ip string) (float64, error)

	LlamadasVerificar []LlamadaVerificar
}

// LlamadaVerificar captura los argumentos de una llamada a Verificar.
type LlamadaVerificar struct {
	Token  string
	Accion string
	IP     string
}

var _ puertos.VerificadorCaptcha = (*VerificadorCaptcha)(nil)

func (m *VerificadorCaptcha) Verificar(ctx context.Context, token, accion, ip string) (float64, error) {
	m.LlamadasVerificar = append(m.LlamadasVerificar, LlamadaVerificar{Token: token, Accion: accion, IP: ip})
	if m.FnVerificar != nil {
		return m.FnVerificar(ctx, token, accion, ip)
	}
	return 1.0, nil
}

// =============================================================================
// Colas de acceso virtual (docs/design/colas-virtuales.md §2). Extensión
// aditiva sobre el archivo existente: nada de lo de arriba cambia.
// =============================================================================

// --- PorteroDeSala (entrada) -------------------------------------------------

// PorteroDeSala es el test double de puertos.PorteroDeSala. Por defecto
// (funciones sin configurar): SalaVigentePara responde "no hay sala
// vigente" (zero value, false) — el camino feliz más común para un test que
// no está ejercitando la sala de espera; Ingresar/ConsultarTurno/Reclamar
// devuelven un ResultadoTurno vacío sin error.
type PorteroDeSala struct {
	FnSalaVigentePara func(ctx context.Context, q puertos.ConsultaSalaVigente) (puertos.VistaSalaVigente, bool)
	FnIngresar        func(ctx context.Context, cmd puertos.ComandoIngresarASala) (puertos.ResultadoTurno, error)
	FnConsultarTurno  func(ctx context.Context, q puertos.ConsultaTurno) (puertos.ResultadoTurno, error)
	FnReclamar        func(ctx context.Context, cmd puertos.ComandoReclamarTurno) (puertos.ResultadoTurno, error)

	LlamadasSalaVigentePara []puertos.ConsultaSalaVigente
	LlamadasIngresar        []puertos.ComandoIngresarASala
	LlamadasConsultarTurno  []puertos.ConsultaTurno
	LlamadasReclamar        []puertos.ComandoReclamarTurno
}

var _ puertos.PorteroDeSala = (*PorteroDeSala)(nil)

func (m *PorteroDeSala) SalaVigentePara(ctx context.Context, q puertos.ConsultaSalaVigente) (puertos.VistaSalaVigente, bool) {
	m.LlamadasSalaVigentePara = append(m.LlamadasSalaVigentePara, q)
	if m.FnSalaVigentePara != nil {
		return m.FnSalaVigentePara(ctx, q)
	}
	return puertos.VistaSalaVigente{}, false
}

func (m *PorteroDeSala) Ingresar(ctx context.Context, cmd puertos.ComandoIngresarASala) (puertos.ResultadoTurno, error) {
	m.LlamadasIngresar = append(m.LlamadasIngresar, cmd)
	if m.FnIngresar != nil {
		return m.FnIngresar(ctx, cmd)
	}
	return puertos.ResultadoTurno{}, nil
}

func (m *PorteroDeSala) ConsultarTurno(ctx context.Context, q puertos.ConsultaTurno) (puertos.ResultadoTurno, error) {
	m.LlamadasConsultarTurno = append(m.LlamadasConsultarTurno, q)
	if m.FnConsultarTurno != nil {
		return m.FnConsultarTurno(ctx, q)
	}
	return puertos.ResultadoTurno{}, nil
}

func (m *PorteroDeSala) Reclamar(ctx context.Context, cmd puertos.ComandoReclamarTurno) (puertos.ResultadoTurno, error) {
	m.LlamadasReclamar = append(m.LlamadasReclamar, cmd)
	if m.FnReclamar != nil {
		return m.FnReclamar(ctx, cmd)
	}
	return puertos.ResultadoTurno{}, nil
}

// --- GestorDeSalasDeEspera (entrada) -----------------------------------------

// GestorDeSalasDeEspera es el test double de puertos.GestorDeSalasDeEspera.
// Por defecto, las tres operaciones devuelven una VistaSala vacía sin error.
type GestorDeSalasDeEspera struct {
	FnAbrir         func(ctx context.Context, cmd puertos.ComandoAbrirSala) (puertos.VistaSala, error)
	FnCambiarRitmo  func(ctx context.Context, cmd puertos.ComandoCambiarRitmoAdmision) (puertos.VistaSala, error)
	FnCambiarEstado func(ctx context.Context, cmd puertos.ComandoCambiarEstadoSala) (puertos.VistaSala, error)

	LlamadasAbrir         []puertos.ComandoAbrirSala
	LlamadasCambiarRitmo  []puertos.ComandoCambiarRitmoAdmision
	LlamadasCambiarEstado []puertos.ComandoCambiarEstadoSala
}

var _ puertos.GestorDeSalasDeEspera = (*GestorDeSalasDeEspera)(nil)

func (m *GestorDeSalasDeEspera) Abrir(ctx context.Context, cmd puertos.ComandoAbrirSala) (puertos.VistaSala, error) {
	m.LlamadasAbrir = append(m.LlamadasAbrir, cmd)
	if m.FnAbrir != nil {
		return m.FnAbrir(ctx, cmd)
	}
	return puertos.VistaSala{}, nil
}

func (m *GestorDeSalasDeEspera) CambiarRitmo(ctx context.Context, cmd puertos.ComandoCambiarRitmoAdmision) (puertos.VistaSala, error) {
	m.LlamadasCambiarRitmo = append(m.LlamadasCambiarRitmo, cmd)
	if m.FnCambiarRitmo != nil {
		return m.FnCambiarRitmo(ctx, cmd)
	}
	return puertos.VistaSala{}, nil
}

func (m *GestorDeSalasDeEspera) CambiarEstado(ctx context.Context, cmd puertos.ComandoCambiarEstadoSala) (puertos.VistaSala, error) {
	m.LlamadasCambiarEstado = append(m.LlamadasCambiarEstado, cmd)
	if m.FnCambiarEstado != nil {
		return m.FnCambiarEstado(ctx, cmd)
	}
	return puertos.VistaSala{}, nil
}

// --- ConsultorDeSalas (entrada) ----------------------------------------------

// ConsultorDeSalas es el test double de puertos.ConsultorDeSalas. Por
// defecto devuelve una VistaSalaPublica vacía sin error.
type ConsultorDeSalas struct {
	FnObtenerPorAlias func(ctx context.Context, q puertos.ConsultaSalaPorAlias) (puertos.VistaSalaPublica, error)

	LlamadasObtenerPorAlias []puertos.ConsultaSalaPorAlias
}

var _ puertos.ConsultorDeSalas = (*ConsultorDeSalas)(nil)

func (m *ConsultorDeSalas) ObtenerPorAlias(ctx context.Context, q puertos.ConsultaSalaPorAlias) (puertos.VistaSalaPublica, error) {
	m.LlamadasObtenerPorAlias = append(m.LlamadasObtenerPorAlias, q)
	if m.FnObtenerPorAlias != nil {
		return m.FnObtenerPorAlias(ctx, q)
	}
	return puertos.VistaSalaPublica{}, nil
}

// --- RepositorioSalasDeEspera (salida) ---------------------------------------

// RepositorioSalasDeEspera es el test double de
// puertos.RepositorioSalasDeEspera. Por defecto, las búsquedas devuelven
// (nil, nil) —"no encontrado, sin error"—, mismo criterio que
// identidad/puertos/mocks.RepositorioUsuarios: el propio caso de uso es
// quien decide si un nil se traduce en dominio.ErrSalaNoEncontrada.
type RepositorioSalasDeEspera struct {
	FnGuardar        func(ctx context.Context, s *dominio.SalaDeEspera) error
	FnBuscarPorID    func(ctx context.Context, id dominio.IDSalaDeEspera) (*dominio.SalaDeEspera, error)
	FnBuscarPorAlias func(ctx context.Context, alias dominio.AliasSala) (*dominio.SalaDeEspera, error)
	FnListarVigentes func(ctx context.Context) ([]*dominio.SalaDeEspera, error)

	LlamadasGuardar        []*dominio.SalaDeEspera
	LlamadasBuscarPorID    []dominio.IDSalaDeEspera
	LlamadasBuscarPorAlias []dominio.AliasSala
	LlamadasListarVigentes int
}

var _ puertos.RepositorioSalasDeEspera = (*RepositorioSalasDeEspera)(nil)

func (m *RepositorioSalasDeEspera) Guardar(ctx context.Context, s *dominio.SalaDeEspera) error {
	m.LlamadasGuardar = append(m.LlamadasGuardar, s)
	if m.FnGuardar != nil {
		return m.FnGuardar(ctx, s)
	}
	return nil
}

func (m *RepositorioSalasDeEspera) BuscarPorID(ctx context.Context, id dominio.IDSalaDeEspera) (*dominio.SalaDeEspera, error) {
	m.LlamadasBuscarPorID = append(m.LlamadasBuscarPorID, id)
	if m.FnBuscarPorID != nil {
		return m.FnBuscarPorID(ctx, id)
	}
	return nil, nil
}

func (m *RepositorioSalasDeEspera) BuscarPorAlias(ctx context.Context, alias dominio.AliasSala) (*dominio.SalaDeEspera, error) {
	m.LlamadasBuscarPorAlias = append(m.LlamadasBuscarPorAlias, alias)
	if m.FnBuscarPorAlias != nil {
		return m.FnBuscarPorAlias(ctx, alias)
	}
	return nil, nil
}

func (m *RepositorioSalasDeEspera) ListarVigentes(ctx context.Context) ([]*dominio.SalaDeEspera, error) {
	m.LlamadasListarVigentes++
	if m.FnListarVigentes != nil {
		return m.FnListarVigentes(ctx)
	}
	return nil, nil
}

// --- EstadoDeCola (salida) ---------------------------------------------------

// EstadoDeCola es el test double de puertos.EstadoDeCola. Por defecto,
// todas las operaciones devuelven su struct de transporte vacío sin error;
// Proyectar/Retirar devuelven nil.
type EstadoDeCola struct {
	FnProyectar   func(ctx context.Context, p puertos.ProyeccionSala) error
	FnRetirar     func(ctx context.Context, clave string) error
	FnIngresar    func(ctx context.Context, clave, hash string) (puertos.EstadoTicket, error)
	FnConsultar   func(ctx context.Context, clave, hash string) (puertos.EstadoTicket, error)
	FnReclamar    func(ctx context.Context, clave, hash string) (puertos.EstadoTicket, error)
	FnInstantanea func(ctx context.Context, clave string) (puertos.InstantaneaCola, error)

	LlamadasProyectar   []puertos.ProyeccionSala
	LlamadasRetirar     []string
	LlamadasIngresar    []LlamadaEstadoDeCola
	LlamadasConsultar   []LlamadaEstadoDeCola
	LlamadasReclamar    []LlamadaEstadoDeCola
	LlamadasInstantanea []string
}

// LlamadaEstadoDeCola captura los argumentos (clave, hash) de una llamada a
// Ingresar/Consultar/Reclamar.
type LlamadaEstadoDeCola struct {
	Clave string
	Hash  string
}

var _ puertos.EstadoDeCola = (*EstadoDeCola)(nil)

func (m *EstadoDeCola) Proyectar(ctx context.Context, p puertos.ProyeccionSala) error {
	m.LlamadasProyectar = append(m.LlamadasProyectar, p)
	if m.FnProyectar != nil {
		return m.FnProyectar(ctx, p)
	}
	return nil
}

func (m *EstadoDeCola) Retirar(ctx context.Context, clave string) error {
	m.LlamadasRetirar = append(m.LlamadasRetirar, clave)
	if m.FnRetirar != nil {
		return m.FnRetirar(ctx, clave)
	}
	return nil
}

func (m *EstadoDeCola) Ingresar(ctx context.Context, clave, hash string) (puertos.EstadoTicket, error) {
	m.LlamadasIngresar = append(m.LlamadasIngresar, LlamadaEstadoDeCola{Clave: clave, Hash: hash})
	if m.FnIngresar != nil {
		return m.FnIngresar(ctx, clave, hash)
	}
	return puertos.EstadoTicket{}, nil
}

func (m *EstadoDeCola) Consultar(ctx context.Context, clave, hash string) (puertos.EstadoTicket, error) {
	m.LlamadasConsultar = append(m.LlamadasConsultar, LlamadaEstadoDeCola{Clave: clave, Hash: hash})
	if m.FnConsultar != nil {
		return m.FnConsultar(ctx, clave, hash)
	}
	return puertos.EstadoTicket{}, nil
}

func (m *EstadoDeCola) Reclamar(ctx context.Context, clave, hash string) (puertos.EstadoTicket, error) {
	m.LlamadasReclamar = append(m.LlamadasReclamar, LlamadaEstadoDeCola{Clave: clave, Hash: hash})
	if m.FnReclamar != nil {
		return m.FnReclamar(ctx, clave, hash)
	}
	return puertos.EstadoTicket{}, nil
}

func (m *EstadoDeCola) Instantanea(ctx context.Context, clave string) (puertos.InstantaneaCola, error) {
	m.LlamadasInstantanea = append(m.LlamadasInstantanea, clave)
	if m.FnInstantanea != nil {
		return m.FnInstantanea(ctx, clave)
	}
	return puertos.InstantaneaCola{}, nil
}

// --- GeneradorTickets (salida) -----------------------------------------------

// GeneradorTickets es el test double de puertos.GeneradorTickets. Por
// defecto (FnGenerarTicket sin configurar) devuelve un ticket con forma
// válida (prefijo mot_cola_ + 43 caracteres base64url de relleno fijo),
// suficiente para que NuevoTicketPlano no falle en los tests que no les
// interesa el valor concreto.
type GeneradorTickets struct {
	FnGenerarTicket func() (dominio.TicketPlano, error)

	LlamadasGenerarTicket int
}

var _ puertos.GeneradorTickets = (*GeneradorTickets)(nil)

func (m *GeneradorTickets) GenerarTicket() (dominio.TicketPlano, error) {
	m.LlamadasGenerarTicket++
	if m.FnGenerarTicket != nil {
		return m.FnGenerarTicket()
	}
	return dominio.NuevoTicketPlano("mot_cola_" + strings.Repeat("a", 43))
}

// --- Reloj (salida) -----------------------------------------------------------

// Reloj es el test double de puertos.Reloj. Si FnAhora no está configurado,
// devuelve el campo Fija (conveniencia para fijar el tiempo en los tests
// sin tener que escribir una función).
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

// --- GeneradorIDs (salida) ----------------------------------------------------

// GeneradorIDs es el test double de puertos.GeneradorIDs.
type GeneradorIDs struct {
	FnNuevoIDSalaDeEspera func() (dominio.IDSalaDeEspera, error)
}

var _ puertos.GeneradorIDs = (*GeneradorIDs)(nil)

func (m *GeneradorIDs) NuevoIDSalaDeEspera() (dominio.IDSalaDeEspera, error) {
	if m.FnNuevoIDSalaDeEspera != nil {
		return m.FnNuevoIDSalaDeEspera()
	}
	return dominio.IDSalaDeEsperaDesde("018e7e6a-0000-7000-8000-000000000001")
}

// --- RegistroAuditoria (salida) -----------------------------------------------

// RegistroAuditoria es el test double de puertos.RegistroAuditoria.
type RegistroAuditoria struct {
	FnRegistrar func(ctx context.Context, e dominio.EventoDominio, origen dominio.OrigenSolicitud) error

	LlamadasRegistrar []EventoAuditado
}

// EventoAuditado captura un evento y el origen con el que se registró, para
// que los tests puedan hacer aserciones sobre ambos.
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

// --- VerificadorDeAutorizacion (salida) ---------------------------------------

// VerificadorDeAutorizacion es el test double de
// puertos.VerificadorDeAutorizacion. Por defecto (FnAutorizar sin
// configurar) permite el intento: el camino feliz más común en los tests de
// los casos de uso de administración org-scoped.
type VerificadorDeAutorizacion struct {
	FnAutorizar func(ctx context.Context, q puertos.ConsultaAutorizacionOrganizacion) (bool, error)

	LlamadasAutorizar []puertos.ConsultaAutorizacionOrganizacion
}

var _ puertos.VerificadorDeAutorizacion = (*VerificadorDeAutorizacion)(nil)

func (m *VerificadorDeAutorizacion) Autorizar(ctx context.Context, q puertos.ConsultaAutorizacionOrganizacion) (bool, error) {
	m.LlamadasAutorizar = append(m.LlamadasAutorizar, q)
	if m.FnAutorizar != nil {
		return m.FnAutorizar(ctx, q)
	}
	return true, nil
}

// --- UnidadDeTrabajo (salida) --------------------------------------------

// UnidadDeTrabajo es el test double de puertos.UnidadDeTrabajo. Por defecto
// (FnEjecutar sin configurar) simplemente invoca fn(ctx), simulando una
// transacción que siempre confirma. Mismo patrón que
// tenencia/puertos/mocks.UnidadDeTrabajo.
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

// =============================================================================
// Reconocimiento de origen (docs/design/fingerprinting-comportamiento.md
// §2.2). Extensión aditiva sobre el archivo existente: nada de lo de arriba
// cambia.
// =============================================================================

// --- PerfilDeOrigenes (salida) ------------------------------------------------

// PerfilDeOrigenes es el test double de puertos.PerfilDeOrigenes. Por
// defecto (FnConsultar sin configurar) Consultar devuelve una
// VistaPerfilOrigen vacía sin error — perfil vacío, es decir "sin
// historial", cero señales por INV-RIES-04 — mismo criterio de "sin
// configurar, comportamiento neutro" que EstadoDeCola más arriba. Registrar
// y Olvidar, por defecto, no hacen nada y no fallan.
type PerfilDeOrigenes struct {
	FnConsultar func(ctx context.Context, q puertos.ConsultaPerfilOrigen) (puertos.VistaPerfilOrigen, error)
	FnRegistrar func(ctx context.Context, cmd puertos.RegistrarOrigenObservado) error
	FnOlvidar   func(ctx context.Context, clave string) error

	LlamadasConsultar []puertos.ConsultaPerfilOrigen
	LlamadasRegistrar []puertos.RegistrarOrigenObservado
	LlamadasOlvidar   []string
}

var _ puertos.PerfilDeOrigenes = (*PerfilDeOrigenes)(nil)

func (m *PerfilDeOrigenes) Consultar(ctx context.Context, q puertos.ConsultaPerfilOrigen) (puertos.VistaPerfilOrigen, error) {
	m.LlamadasConsultar = append(m.LlamadasConsultar, q)
	if m.FnConsultar != nil {
		return m.FnConsultar(ctx, q)
	}
	return puertos.VistaPerfilOrigen{}, nil
}

func (m *PerfilDeOrigenes) Registrar(ctx context.Context, cmd puertos.RegistrarOrigenObservado) error {
	m.LlamadasRegistrar = append(m.LlamadasRegistrar, cmd)
	if m.FnRegistrar != nil {
		return m.FnRegistrar(ctx, cmd)
	}
	return nil
}

func (m *PerfilDeOrigenes) Olvidar(ctx context.Context, clave string) error {
	m.LlamadasOlvidar = append(m.LlamadasOlvidar, clave)
	if m.FnOlvidar != nil {
		return m.FnOlvidar(ctx, clave)
	}
	return nil
}
