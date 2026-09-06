// Package mocks contiene test doubles escritos a mano para los puertos de
// salida de Identidad (puertos/salida.go). Siguen el patrón idiomático de Go
// para dobles de prueba simples: un struct por interfaz con un campo `FnX`
// por método, que el test configura solo cuando necesita un comportamiento
// distinto del valor cero. Cuando el campo no se configura, el mock aplica
// un comportamiento por defecto razonable (normalmente "no-op, sin error")
// para que los tests que no les interesa un puerto concreto no tengan que
// configurarlo.
//
// No se usa un generador externo (mockgen/gomock, testify/mock): el
// proyecto no tenía esas dependencias y las interfaces de este contexto son
// pequeñas, así que el mock a mano es más legible y no añade una
// dependencia nueva al módulo.
package mocks

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/r-david1/moterus/internal/identidad/dominio"
	"github.com/r-david1/moterus/internal/identidad/puertos"
)

// --- RepositorioUsuarios -----------------------------------------------------

// RepositorioUsuarios es el test double de puertos.RepositorioUsuarios.
type RepositorioUsuarios struct {
	FnGuardar         func(ctx context.Context, u *dominio.Usuario) error
	FnBuscarPorID     func(ctx context.Context, id dominio.IDUsuario) (*dominio.Usuario, error)
	FnBuscarPorCorreo func(ctx context.Context, c dominio.Correo) (*dominio.Usuario, error)

	// Llamadas queda registrado para poder hacer aserciones sin necesidad de
	// una librería de mocking (p. ej. "se guardó exactamente una vez, y con
	// qué usuario").
	LlamadasGuardar         []*dominio.Usuario
	LlamadasBuscarPorID     []dominio.IDUsuario
	LlamadasBuscarPorCorreo []dominio.Correo
}

var _ puertos.RepositorioUsuarios = (*RepositorioUsuarios)(nil)

func (m *RepositorioUsuarios) Guardar(ctx context.Context, u *dominio.Usuario) error {
	m.LlamadasGuardar = append(m.LlamadasGuardar, u)
	if m.FnGuardar != nil {
		return m.FnGuardar(ctx, u)
	}
	return nil
}

func (m *RepositorioUsuarios) BuscarPorID(ctx context.Context, id dominio.IDUsuario) (*dominio.Usuario, error) {
	m.LlamadasBuscarPorID = append(m.LlamadasBuscarPorID, id)
	if m.FnBuscarPorID != nil {
		return m.FnBuscarPorID(ctx, id)
	}
	return nil, nil
}

func (m *RepositorioUsuarios) BuscarPorCorreo(ctx context.Context, c dominio.Correo) (*dominio.Usuario, error) {
	m.LlamadasBuscarPorCorreo = append(m.LlamadasBuscarPorCorreo, c)
	if m.FnBuscarPorCorreo != nil {
		return m.FnBuscarPorCorreo(ctx, c)
	}
	return nil, nil
}

// --- HasherContrasenas --------------------------------------------------------

// HasherContrasenas es el test double de puertos.HasherContrasenas.
type HasherContrasenas struct {
	FnHashear                   func(ctx context.Context, p dominio.ContrasenaPlana) (dominio.HashContrasena, error)
	FnVerificar                 func(ctx context.Context, h dominio.HashContrasena, p dominio.ContrasenaPlana) (bool, error)
	FnNecesitaRehash            func(h dominio.HashContrasena) bool
	FnConsumirTiempoEquivalente func(ctx context.Context)

	LlamadasHashear                   int
	LlamadasVerificar                 int
	LlamadasNecesitaRehash            int
	LlamadasConsumirTiempoEquivalente int
}

var _ puertos.HasherContrasenas = (*HasherContrasenas)(nil)

func (m *HasherContrasenas) Hashear(ctx context.Context, p dominio.ContrasenaPlana) (dominio.HashContrasena, error) {
	m.LlamadasHashear++
	if m.FnHashear != nil {
		return m.FnHashear(ctx, p)
	}
	return dominio.HashContrasena{}, nil
}

func (m *HasherContrasenas) Verificar(ctx context.Context, h dominio.HashContrasena, p dominio.ContrasenaPlana) (bool, error) {
	m.LlamadasVerificar++
	if m.FnVerificar != nil {
		return m.FnVerificar(ctx, h, p)
	}
	return false, nil
}

func (m *HasherContrasenas) NecesitaRehash(h dominio.HashContrasena) bool {
	m.LlamadasNecesitaRehash++
	if m.FnNecesitaRehash != nil {
		return m.FnNecesitaRehash(h)
	}
	return false
}

func (m *HasherContrasenas) ConsumirTiempoEquivalente(ctx context.Context) {
	m.LlamadasConsumirTiempoEquivalente++
	if m.FnConsumirTiempoEquivalente != nil {
		m.FnConsumirTiempoEquivalente(ctx)
	}
}

// --- VerificadorContrasenasFiltradas ------------------------------------------

// VerificadorContrasenasFiltradas es el test double de
// puertos.VerificadorContrasenasFiltradas.
type VerificadorContrasenasFiltradas struct {
	FnEstaFiltrada func(ctx context.Context, p dominio.ContrasenaPlana) (bool, error)

	LlamadasEstaFiltrada int
}

var _ puertos.VerificadorContrasenasFiltradas = (*VerificadorContrasenasFiltradas)(nil)

func (m *VerificadorContrasenasFiltradas) EstaFiltrada(ctx context.Context, p dominio.ContrasenaPlana) (bool, error) {
	m.LlamadasEstaFiltrada++
	if m.FnEstaFiltrada != nil {
		return m.FnEstaFiltrada(ctx, p)
	}
	return false, nil
}

// --- Reloj ---------------------------------------------------------------------

// Reloj es el test double de puertos.Reloj. Si FnAhora no está configurado,
// devuelve el campo Fija (conveniencia para fijar el tiempo en los tests sin
// tener que escribir una función).
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
	FnNuevoIDUsuario   func() (dominio.IDUsuario, error)
	FnNuevoIDFactorMFA func() (dominio.IDFactorMFA, error)
}

var _ puertos.GeneradorIDs = (*GeneradorIDs)(nil)

func (m *GeneradorIDs) NuevoIDUsuario() (dominio.IDUsuario, error) {
	if m.FnNuevoIDUsuario != nil {
		return m.FnNuevoIDUsuario()
	}
	return dominio.IDUsuario{}, nil
}

func (m *GeneradorIDs) NuevoIDFactorMFA() (dominio.IDFactorMFA, error) {
	if m.FnNuevoIDFactorMFA != nil {
		return m.FnNuevoIDFactorMFA()
	}
	return dominio.IDFactorMFA{}, nil
}

// --- UnidadDeTrabajo ------------------------------------------------------

// UnidadDeTrabajo es el test double de puertos.UnidadDeTrabajo. Por defecto
// (FnEjecutar sin configurar) simplemente invoca fn(ctx), simulando una
// transacción que siempre confirma: así la mayoría de los tests no necesitan
// configurarlo explícitamente. Para simular un fallo de la unidad de
// trabajo en sí (no del fn), se configura FnEjecutar para que devuelva un
// error sin invocar fn.
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

// --- EvaluadorConfianza -----------------------------------------------------

// EvaluadorConfianza es el test double de puertos.EvaluadorConfianza. Por
// defecto (FnEvaluar sin configurar) permite el intento (Permitido: true),
// que es el camino feliz más común en los tests de los casos de uso.
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

// --- GeneradorTokens ---------------------------------------------------------

// GeneradorTokens es el test double de puertos.GeneradorTokens. Por
// defecto (FnGenerar sin configurar) devuelve un token fijo no vacío, para
// que los tests que no les interesa el valor concreto no tengan que
// configurarlo.
type GeneradorTokens struct {
	FnGenerar func() (string, error)

	LlamadasGenerar int
}

var _ puertos.GeneradorTokens = (*GeneradorTokens)(nil)

func (m *GeneradorTokens) Generar() (string, error) {
	m.LlamadasGenerar++
	if m.FnGenerar != nil {
		return m.FnGenerar()
	}
	return "token-de-prueba-opaco", nil
}

// --- RepositorioTokensVerificacion -------------------------------------------

// RepositorioTokensVerificacion es el test double de
// puertos.RepositorioTokensVerificacion.
type RepositorioTokensVerificacion struct {
	FnGuardar       func(ctx context.Context, usuarioID dominio.IDUsuario, hashToken string, expiraEn time.Time) error
	FnBuscarPorHash func(ctx context.Context, hashToken string) (dominio.IDUsuario, time.Time, bool, error)
	FnEliminar      func(ctx context.Context, usuarioID dominio.IDUsuario) error

	LlamadasGuardar       []TokenVerificacionGuardado
	LlamadasBuscarPorHash []string
	LlamadasEliminar      []dominio.IDUsuario
}

// TokenVerificacionGuardado captura los argumentos de una llamada a
// Guardar, para que los tests puedan hacer aserciones sobre ellos.
type TokenVerificacionGuardado struct {
	UsuarioID dominio.IDUsuario
	HashToken string
	ExpiraEn  time.Time
}

var _ puertos.RepositorioTokensVerificacion = (*RepositorioTokensVerificacion)(nil)

func (m *RepositorioTokensVerificacion) Guardar(ctx context.Context, usuarioID dominio.IDUsuario, hashToken string, expiraEn time.Time) error {
	m.LlamadasGuardar = append(m.LlamadasGuardar, TokenVerificacionGuardado{UsuarioID: usuarioID, HashToken: hashToken, ExpiraEn: expiraEn})
	if m.FnGuardar != nil {
		return m.FnGuardar(ctx, usuarioID, hashToken, expiraEn)
	}
	return nil
}

func (m *RepositorioTokensVerificacion) BuscarPorHash(ctx context.Context, hashToken string) (dominio.IDUsuario, time.Time, bool, error) {
	m.LlamadasBuscarPorHash = append(m.LlamadasBuscarPorHash, hashToken)
	if m.FnBuscarPorHash != nil {
		return m.FnBuscarPorHash(ctx, hashToken)
	}
	return dominio.IDUsuario{}, time.Time{}, false, nil
}

func (m *RepositorioTokensVerificacion) Eliminar(ctx context.Context, usuarioID dominio.IDUsuario) error {
	m.LlamadasEliminar = append(m.LlamadasEliminar, usuarioID)
	if m.FnEliminar != nil {
		return m.FnEliminar(ctx, usuarioID)
	}
	return nil
}

// --- NotificadorCorreo --------------------------------------------------------

// NotificadorCorreo es el test double de puertos.NotificadorCorreo.
type NotificadorCorreo struct {
	FnEnviarVerificacion func(ctx context.Context, correo dominio.Correo, tokenPlano string) error

	LlamadasEnviarVerificacion []NotificacionVerificacionEnviada
}

// NotificacionVerificacionEnviada captura los argumentos de una llamada a
// EnviarVerificacion. Se usa, entre otras cosas, para el test de INV-ID-21:
// el único lugar por el que debe pasar el token en claro.
type NotificacionVerificacionEnviada struct {
	Correo     dominio.Correo
	TokenPlano string
}

var _ puertos.NotificadorCorreo = (*NotificadorCorreo)(nil)

func (m *NotificadorCorreo) EnviarVerificacion(ctx context.Context, correo dominio.Correo, tokenPlano string) error {
	m.LlamadasEnviarVerificacion = append(m.LlamadasEnviarVerificacion, NotificacionVerificacionEnviada{Correo: correo, TokenPlano: tokenPlano})
	if m.FnEnviarVerificacion != nil {
		return m.FnEnviarVerificacion(ctx, correo, tokenPlano)
	}
	return nil
}

// --- RepositorioFactoresMFA ---------------------------------------------------

// RepositorioFactoresMFA es el test double de
// puertos.RepositorioFactoresMFA.
type RepositorioFactoresMFA struct {
	FnGuardar                    func(ctx context.Context, f *dominio.FactorMFA) error
	FnBuscarPorID                func(ctx context.Context, id dominio.IDFactorMFA) (*dominio.FactorMFA, error)
	FnBuscarConfirmadosDeUsuario func(ctx context.Context, u dominio.IDUsuario) ([]*dominio.FactorMFA, error)
	FnContarConfirmadosDeUsuario func(ctx context.Context, u dominio.IDUsuario) (int, error)

	LlamadasGuardar                    []*dominio.FactorMFA
	LlamadasBuscarPorID                []dominio.IDFactorMFA
	LlamadasBuscarConfirmadosDeUsuario []dominio.IDUsuario
	LlamadasContarConfirmadosDeUsuario []dominio.IDUsuario
}

var _ puertos.RepositorioFactoresMFA = (*RepositorioFactoresMFA)(nil)

func (m *RepositorioFactoresMFA) Guardar(ctx context.Context, f *dominio.FactorMFA) error {
	m.LlamadasGuardar = append(m.LlamadasGuardar, f)
	if m.FnGuardar != nil {
		return m.FnGuardar(ctx, f)
	}
	return nil
}

func (m *RepositorioFactoresMFA) BuscarPorID(ctx context.Context, id dominio.IDFactorMFA) (*dominio.FactorMFA, error) {
	m.LlamadasBuscarPorID = append(m.LlamadasBuscarPorID, id)
	if m.FnBuscarPorID != nil {
		return m.FnBuscarPorID(ctx, id)
	}
	return nil, nil
}

func (m *RepositorioFactoresMFA) BuscarConfirmadosDeUsuario(ctx context.Context, u dominio.IDUsuario) ([]*dominio.FactorMFA, error) {
	m.LlamadasBuscarConfirmadosDeUsuario = append(m.LlamadasBuscarConfirmadosDeUsuario, u)
	if m.FnBuscarConfirmadosDeUsuario != nil {
		return m.FnBuscarConfirmadosDeUsuario(ctx, u)
	}
	return nil, nil
}

func (m *RepositorioFactoresMFA) ContarConfirmadosDeUsuario(ctx context.Context, u dominio.IDUsuario) (int, error) {
	m.LlamadasContarConfirmadosDeUsuario = append(m.LlamadasContarConfirmadosDeUsuario, u)
	if m.FnContarConfirmadosDeUsuario != nil {
		return m.FnContarConfirmadosDeUsuario(ctx, u)
	}
	return 0, nil
}

// --- GeneradorSecretoTOTP ------------------------------------------------------

// GeneradorSecretoTOTP es el test double de puertos.GeneradorSecretoTOTP.
// Por defecto (sin configurar) devuelve un secreto y códigos de respaldo
// con forma válida, para que los tests a los que no les interesa el valor
// concreto no tengan que configurarlo.
type GeneradorSecretoTOTP struct {
	FnGenerarSecreto         func() (dominio.SecretoTOTPPlano, error)
	FnGenerarCodigosRespaldo func(n int) ([]dominio.CodigoRespaldoPlano, error)

	LlamadasGenerarSecreto         int
	LlamadasGenerarCodigosRespaldo []int
}

var _ puertos.GeneradorSecretoTOTP = (*GeneradorSecretoTOTP)(nil)

func (m *GeneradorSecretoTOTP) GenerarSecreto() (dominio.SecretoTOTPPlano, error) {
	m.LlamadasGenerarSecreto++
	if m.FnGenerarSecreto != nil {
		return m.FnGenerarSecreto()
	}
	return dominio.NuevoSecretoTOTPPlano(strings.Repeat("A", 32))
}

func (m *GeneradorSecretoTOTP) GenerarCodigosRespaldo(n int) ([]dominio.CodigoRespaldoPlano, error) {
	m.LlamadasGenerarCodigosRespaldo = append(m.LlamadasGenerarCodigosRespaldo, n)
	if m.FnGenerarCodigosRespaldo != nil {
		return m.FnGenerarCodigosRespaldo(n)
	}
	// Alfabeto de dos caracteres seguido de dos dígitos, ambos tomados del
	// mismo catálogo cerrado que dominio.CodigoRespaldoPlano exige
	// ("ABCDEFGHJKMNPQRSTUVWXYZ23456789", sin 0/1/I/L/O) — un valor por
	// defecto que use dígitos fuera de ese alfabeto (p. ej. "0"/"1") hace
	// que NuevoCodigoRespaldoPlano falle en cualquier test que no
	// configure FnGenerarCodigosRespaldo.
	const alfabeto = "23456789"
	codigos := make([]dominio.CodigoRespaldoPlano, 0, n)
	for i := 0; i < n; i++ {
		sufijo := string(alfabeto[i%len(alfabeto)]) + string(alfabeto[(i/len(alfabeto))%len(alfabeto)])
		c, err := dominio.NuevoCodigoRespaldoPlano(fmt.Sprintf("ABCDEFGH%s", sufijo))
		if err != nil {
			return nil, err
		}
		codigos = append(codigos, c)
	}
	return codigos, nil
}

// --- CifradorSecretos ----------------------------------------------------------

// CifradorSecretos es el test double de puertos.CifradorSecretos. Por
// defecto, Cifrar envuelve el valor en claro sin transformarlo y Descifrar
// deshace esa envoltura: suficiente para que los casos de uso que no
// prueban la criptografía en sí puedan hacer un roundtrip sin configurar
// nada.
type CifradorSecretos struct {
	FnCifrar    func(secreto dominio.SecretoTOTPPlano) (dominio.SecretoTOTPCifrado, error)
	FnDescifrar func(cifrado dominio.SecretoTOTPCifrado) (dominio.SecretoTOTPPlano, error)

	LlamadasCifrar    []dominio.SecretoTOTPPlano
	LlamadasDescifrar []dominio.SecretoTOTPCifrado
}

var _ puertos.CifradorSecretos = (*CifradorSecretos)(nil)

func (m *CifradorSecretos) Cifrar(secreto dominio.SecretoTOTPPlano) (dominio.SecretoTOTPCifrado, error) {
	m.LlamadasCifrar = append(m.LlamadasCifrar, secreto)
	if m.FnCifrar != nil {
		return m.FnCifrar(secreto)
	}
	return dominio.NuevoSecretoTOTPCifrado([]byte(secreto.Valor()))
}

func (m *CifradorSecretos) Descifrar(cifrado dominio.SecretoTOTPCifrado) (dominio.SecretoTOTPPlano, error) {
	m.LlamadasDescifrar = append(m.LlamadasDescifrar, cifrado)
	if m.FnDescifrar != nil {
		return m.FnDescifrar(cifrado)
	}
	return dominio.NuevoSecretoTOTPPlano(string(cifrado.Valor()))
}
