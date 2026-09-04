package aplicacion_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/r-david1/moterus/internal/tenencia/aplicacion"
	"github.com/r-david1/moterus/internal/tenencia/dominio"
	"github.com/r-david1/moterus/internal/tenencia/puertos"
	"github.com/r-david1/moterus/internal/tenencia/puertos/mocks"
)

// mocksInvitaciones agrupa los dobles de prueba compartidos por todo el
// grupo de casos de uso de puertos.GestorDeInvitaciones (§3.5 del diseño):
// Invitar, Revocar, Aceptar.
type mocksInvitaciones struct {
	organizaciones *mocks.RepositorioOrganizaciones
	membresias     *mocks.RepositorioMembresias
	invitaciones   *mocks.RepositorioInvitaciones
	autorizador    *autorizadorFalso
	sujetos        *mocks.VerificadorDeSujetos
	confianza      *mocks.EvaluadorConfianza
	notificador    *mocks.NotificadorInvitaciones
	auditoria      *mocks.RegistroAuditoria
	eventos        *mocks.PublicadorEventos
	reloj          *mocks.Reloj
	ids            *mocks.GeneradorIDs
	tokens         *mocks.GeneradorTokens
	uow            *mocks.UnidadDeTrabajo
	politica       dominio.PoliticaOrganizacion
}

func nuevosMocksInvitaciones(t *testing.T) *mocksInvitaciones {
	t.Helper()
	return &mocksInvitaciones{
		organizaciones: &mocks.RepositorioOrganizaciones{
			FnBuscarPorID: func(ctx context.Context, id dominio.IDOrganizacion) (*dominio.Organizacion, error) {
				return organizacionActivaDePrueba(t, idOrganizacionValido1, "acme", "Acme", idUsuarioDePrueba(t, idUsuarioValido1), ahoraDePrueba()), nil
			},
		},
		membresias:   &mocks.RepositorioMembresias{},
		invitaciones: &mocks.RepositorioInvitaciones{},
		autorizador:  &autorizadorFalso{},
		sujetos:      &mocks.VerificadorDeSujetos{},
		confianza:    &mocks.EvaluadorConfianza{},
		notificador:  &mocks.NotificadorInvitaciones{},
		auditoria:    &mocks.RegistroAuditoria{},
		eventos:      &mocks.PublicadorEventos{},
		reloj:        &mocks.Reloj{Fija: ahoraDePrueba()},
		ids: &mocks.GeneradorIDs{
			FnNuevoIDInvitacion: func() (dominio.IDInvitacion, error) { return dominio.IDInvitacionDesde(idInvitacionValido1) },
			FnNuevoIDMembresia:  func() (dominio.IDMembresia, error) { return dominio.IDMembresiaDesde(idMembresiaValido2) },
		},
		tokens: &mocks.GeneradorTokens{
			FnGenerarTokenInvitacion: func() (dominio.TokenInvitacionPlano, error) {
				return dominio.NuevoTokenInvitacionPlano(tokenInvitacionValor1)
			},
		},
		uow:      &mocks.UnidadDeTrabajo{},
		politica: politicaDePrueba(t),
	}
}

func (m *mocksInvitaciones) casoDeUso() *aplicacion.InvitacionesCasoDeUso {
	return aplicacion.NuevoInvitacionesCasoDeUso(
		m.organizaciones, m.membresias, m.invitaciones, m.autorizador, m.sujetos,
		m.confianza, m.notificador, m.auditoria, m.eventos, m.reloj, m.ids, m.tokens,
		m.uow, m.politica,
	)
}

func comandoInvitarMiembroValido(t *testing.T) puertos.ComandoInvitarMiembro {
	t.Helper()
	return puertos.ComandoInvitarMiembro{
		IDOrganizacion: idOrganizacionValido1,
		IDSujeto:       idUsuarioValido1,
		Correo:         "invitado@ejemplo.com",
		Rol:            dominio.RolMiembro.Valor(),
		Origen:         origenDePrueba(t),
	}
}

func TestInvitarMiembroCasoDeUso_FlujoFeliz(t *testing.T) {
	m := nuevosMocksInvitaciones(t)
	caso := m.casoDeUso()

	vista, err := caso.Invitar(context.Background(), comandoInvitarMiembroValido(t))
	if err != nil {
		t.Fatalf("Invitar() devolvió error inesperado: %v", err)
	}
	if vista.Destinatario != "invitado@ejemplo.com" {
		t.Errorf("Destinatario = %q, esperado invitado@ejemplo.com", vista.Destinatario)
	}
	if vista.Estado != dominio.EstadoInvitacionPendiente.String() {
		t.Errorf("Estado = %q, esperado pendiente", vista.Estado)
	}
	if len(m.invitaciones.LlamadasGuardar) != 1 {
		t.Errorf("se esperaba 1 Guardar, hubo %d", len(m.invitaciones.LlamadasGuardar))
	}
	if nombres := m.auditoria.NombresEventos(); len(nombres) != 1 || nombres[0] != "MiembroInvitado" {
		t.Errorf("eventos auditados = %v, esperado [MiembroInvitado]", nombres)
	}
	// INV-TEN-23: el token en claro solo sale por NotificadorInvitaciones.
	if len(m.notificador.LlamadasEnviarInvitacion) != 1 {
		t.Fatalf("se esperaba 1 llamada a EnviarInvitacion, hubo %d", len(m.notificador.LlamadasEnviarInvitacion))
	}
	if m.notificador.LlamadasEnviarInvitacion[0].TokenPlano != tokenInvitacionValor1 {
		t.Error("el token en claro entregado al notificador no coincide con el generado")
	}
}

func TestInvitarMiembroCasoDeUso_ReinvitarRevocaLaPendienteAnterior(t *testing.T) {
	m := nuevosMocksInvitaciones(t)
	idOrg := idOrganizacionDePrueba(t, idOrganizacionValido1)
	correo := correoDePrueba(t, "invitado@ejemplo.com")
	pendiente := invitacionPendienteDePrueba(t, idOrg, correo, dominio.RolMiembro, idUsuarioDePrueba(t, idUsuarioValido1),
		tokenInvitacionValor2, ahoraDePrueba().Add(-time.Hour), ahoraDePrueba().Add(6*24*time.Hour))
	m.invitaciones.FnBuscarPendiente = func(ctx context.Context, o dominio.IDOrganizacion, c dominio.CorreoDestinatario) (*dominio.Invitacion, error) {
		return pendiente, nil
	}
	caso := m.casoDeUso()

	_, err := caso.Invitar(context.Background(), comandoInvitarMiembroValido(t))
	if err != nil {
		t.Fatalf("Invitar() devolvió error inesperado: %v", err)
	}
	if !pendiente.Estado().EsIgual(dominio.EstadoInvitacionRevocada) {
		t.Error("la invitación pendiente anterior debía quedar revocada")
	}
	// 2 Guardar: la revocada + la nueva.
	if len(m.invitaciones.LlamadasGuardar) != 2 {
		t.Errorf("se esperaban 2 Guardar, hubo %d", len(m.invitaciones.LlamadasGuardar))
	}
	nombres := m.auditoria.NombresEventos()
	if len(nombres) != 2 || nombres[0] != "InvitacionResuelta" || nombres[1] != "MiembroInvitado" {
		t.Errorf("eventos auditados = %v, esperado [InvitacionResuelta MiembroInvitado]", nombres)
	}
}

func TestInvitarMiembroCasoDeUso_RolSuperiorAlPropio(t *testing.T) {
	m := nuevosMocksInvitaciones(t)
	m.autorizador = &autorizadorFalso{
		FnAutorizar: func(ctx context.Context, q puertos.ConsultaAutorizacion) (puertos.Autorizacion, error) {
			return puertos.Autorizacion{Permitido: true, IDOrganizacion: q.IDOrganizacion, Rol: dominio.RolAdministrador.Valor()}, nil
		},
	}
	caso := m.casoDeUso()

	cmd := comandoInvitarMiembroValido(t)
	cmd.Rol = dominio.RolPropietario.Valor()
	_, err := caso.Invitar(context.Background(), cmd)
	var errDominancia *dominio.ErrRolSuperiorAlPropio
	if !errors.As(err, &errDominancia) {
		t.Fatalf("se esperaba *ErrRolSuperiorAlPropio, obtuvo %T: %v", err, err)
	}
	if len(m.confianza.LlamadasEvaluar) != 0 {
		t.Error("no debía evaluarse Confianza si la dominancia ya rechazó")
	}
}

func TestInvitarMiembroCasoDeUso_AccesoDenegadoPorConfianza(t *testing.T) {
	m := nuevosMocksInvitaciones(t)
	m.confianza.FnEvaluar = func(ctx context.Context, s puertos.SolicitudEvaluacion) (puertos.DecisionConfianza, error) {
		return puertos.DecisionConfianza{Permitido: false, Motivo: "riesgo_alto"}, nil
	}
	caso := m.casoDeUso()

	_, err := caso.Invitar(context.Background(), comandoInvitarMiembroValido(t))
	var errConfianza *dominio.ErrAccesoDenegadoPorConfianza
	if !errors.As(err, &errConfianza) {
		t.Fatalf("se esperaba *ErrAccesoDenegadoPorConfianza, obtuvo %T: %v", err, err)
	}
	if m.tokens.LlamadasGenerarTokenInvitacion != 0 {
		t.Error("no debía generarse ningún token si Confianza denegó")
	}
}

func TestInvitarMiembroCasoDeUso_LimiteInvitacionesExcedido(t *testing.T) {
	m := nuevosMocksInvitaciones(t)
	m.invitaciones.FnContarPendientesDeOrganizacion = func(ctx context.Context, o dominio.IDOrganizacion) (int, error) {
		return m.politica.MaximoInvitacionesPendientes(), nil
	}
	caso := m.casoDeUso()

	_, err := caso.Invitar(context.Background(), comandoInvitarMiembroValido(t))
	var errLimite *dominio.ErrLimiteInvitacionesExcedido
	if !errors.As(err, &errLimite) {
		t.Fatalf("se esperaba *ErrLimiteInvitacionesExcedido, obtuvo %T: %v", err, err)
	}
}

func TestInvitarMiembroCasoDeUso_OrganizacionNoOperativa(t *testing.T) {
	m := nuevosMocksInvitaciones(t)
	m.organizaciones.FnBuscarPorID = func(ctx context.Context, id dominio.IDOrganizacion) (*dominio.Organizacion, error) {
		return organizacionSuspendidaDePrueba(t, idOrganizacionValido1, "acme", "Acme", idUsuarioDePrueba(t, idUsuarioValido1), ahoraDePrueba()), nil
	}
	caso := m.casoDeUso()

	_, err := caso.Invitar(context.Background(), comandoInvitarMiembroValido(t))
	var errNoOperativa *dominio.ErrOrganizacionNoOperativa
	if !errors.As(err, &errNoOperativa) {
		t.Fatalf("se esperaba *ErrOrganizacionNoOperativa, obtuvo %T: %v", err, err)
	}
}
