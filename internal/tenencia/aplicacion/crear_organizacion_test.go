package aplicacion_test

import (
	"context"
	"errors"
	"testing"

	"github.com/r-david1/moterus/internal/tenencia/aplicacion"
	"github.com/r-david1/moterus/internal/tenencia/dominio"
	"github.com/r-david1/moterus/internal/tenencia/puertos"
	"github.com/r-david1/moterus/internal/tenencia/puertos/mocks"
)

type mocksOrganizaciones struct {
	organizaciones *mocks.RepositorioOrganizaciones
	membresias     *mocks.RepositorioMembresias
	autorizador    *autorizadorFalso
	sujetos        *mocks.VerificadorDeSujetos
	confianza      *mocks.EvaluadorConfianza
	auditoria      *mocks.RegistroAuditoria
	eventos        *mocks.PublicadorEventos
	reloj          *mocks.Reloj
	ids            *mocks.GeneradorIDs
	uow            *mocks.UnidadDeTrabajo
	politica       dominio.PoliticaOrganizacion
}

func nuevosMocksOrganizaciones(t *testing.T) *mocksOrganizaciones {
	t.Helper()
	return &mocksOrganizaciones{
		organizaciones: &mocks.RepositorioOrganizaciones{},
		membresias:     &mocks.RepositorioMembresias{},
		autorizador:    &autorizadorFalso{},
		sujetos:        &mocks.VerificadorDeSujetos{},
		confianza:      &mocks.EvaluadorConfianza{},
		auditoria:      &mocks.RegistroAuditoria{},
		eventos:        &mocks.PublicadorEventos{},
		reloj:          &mocks.Reloj{Fija: ahoraDePrueba()},
		ids: &mocks.GeneradorIDs{
			FnNuevoIDOrganizacion: func() (dominio.IDOrganizacion, error) { return idOrganizacionDePrueba(t, idOrganizacionValido1), nil },
			FnNuevoIDMembresia:    func() (dominio.IDMembresia, error) { return dominio.IDMembresiaDesde(idMembresiaValido1) },
		},
		uow:      &mocks.UnidadDeTrabajo{},
		politica: politicaDePrueba(t),
	}
}

func (m *mocksOrganizaciones) casoDeUso() *aplicacion.OrganizacionesCasoDeUso {
	return aplicacion.NuevoOrganizacionesCasoDeUso(
		m.organizaciones, m.membresias, m.autorizador, m.sujetos, m.confianza,
		m.auditoria, m.eventos, m.reloj, m.ids, m.uow, m.politica,
	)
}

func comandoCrearOrganizacionValido(t *testing.T) puertos.ComandoCrearOrganizacion {
	t.Helper()
	return puertos.ComandoCrearOrganizacion{
		Nombre:   "Acme Inc",
		Alias:    "acme",
		IDSujeto: idUsuarioValido1,
		Origen:   origenDePrueba(t),
	}
}

func TestCrearOrganizacionCasoDeUso_FlujoFeliz(t *testing.T) {
	m := nuevosMocksOrganizaciones(t)
	caso := m.casoDeUso()

	vista, err := caso.Crear(context.Background(), comandoCrearOrganizacionValido(t))
	if err != nil {
		t.Fatalf("Crear() devolvió error inesperado: %v", err)
	}
	if vista.ID != idOrganizacionValido1 {
		t.Errorf("ID = %q, esperado %q", vista.ID, idOrganizacionValido1)
	}
	if vista.Alias != "acme" {
		t.Errorf("Alias = %q, esperado acme", vista.Alias)
	}
	if vista.Estado != dominio.EstadoOrganizacionActiva.String() {
		t.Errorf("Estado = %q, esperado activa", vista.Estado)
	}
	if vista.MiembrosActivos != 1 {
		t.Errorf("MiembrosActivos = %d, esperado 1", vista.MiembrosActivos)
	}

	// INV-TEN-03: Organizacion y Membresia fundacional en la MISMA
	// UnidadDeTrabajo.
	if len(m.organizaciones.LlamadasGuardar) != 1 {
		t.Fatalf("se esperaba 1 Guardar en RepositorioOrganizaciones, hubo %d", len(m.organizaciones.LlamadasGuardar))
	}
	if len(m.membresias.LlamadasGuardar) != 1 {
		t.Fatalf("se esperaba 1 Guardar en RepositorioMembresias, hubo %d", len(m.membresias.LlamadasGuardar))
	}
	membresiaFundadora := m.membresias.LlamadasGuardar[0]
	if !membresiaFundadora.Rol().EsIgual(dominio.RolPropietario) {
		t.Errorf("rol de la membresía fundadora = %q, esperado propietario", membresiaFundadora.Rol().Valor())
	}

	nombres := m.auditoria.NombresEventos()
	if len(nombres) != 2 || nombres[0] != "OrganizacionCreada" || nombres[1] != "MiembroAgregado" {
		t.Errorf("eventos auditados = %v, esperado [OrganizacionCreada MiembroAgregado]", nombres)
	}
	for _, l := range m.auditoria.LlamadasRegistrar {
		if l.IDOrganizacion.String() != idOrganizacionValido1 {
			t.Errorf("IDOrganizacion auditado = %q, esperado %q", l.IDOrganizacion.String(), idOrganizacionValido1)
		}
	}

	if len(m.eventos.LlamadasPublicar) != 1 || len(m.eventos.LlamadasPublicar[0]) != 2 {
		t.Errorf("se esperaba publicar 2 eventos en 1 llamada, hubo %v", m.eventos.LlamadasPublicar)
	}
	if len(m.confianza.LlamadasRegistrarResultado) != 1 {
		t.Errorf("se esperaba 1 llamada a RegistrarResultado, hubo %d", len(m.confianza.LlamadasRegistrarResultado))
	}
}

func TestCrearOrganizacionCasoDeUso_LimiteOrganizacionesExcedido(t *testing.T) {
	m := nuevosMocksOrganizaciones(t)
	m.membresias.FnContarOrganizacionesPropiasDeUsuario = func(ctx context.Context, u dominio.IDUsuario) (int, error) {
		return m.politica.MaximoOrganizacionesPorUsuario(), nil
	}
	caso := m.casoDeUso()

	_, err := caso.Crear(context.Background(), comandoCrearOrganizacionValido(t))
	var errLimite *dominio.ErrLimiteOrganizacionesExcedido
	if !errors.As(err, &errLimite) {
		t.Fatalf("se esperaba *ErrLimiteOrganizacionesExcedido, obtuvo %T: %v", err, err)
	}
	if len(m.organizaciones.LlamadasGuardar) != 0 {
		t.Error("no debía persistirse ninguna organización al superar el límite")
	}
}

func TestCrearOrganizacionCasoDeUso_SujetoNoElegible(t *testing.T) {
	m := nuevosMocksOrganizaciones(t)
	m.sujetos.FnEsElegible = func(ctx context.Context, idUsuario string) (puertos.SujetoElegible, error) {
		return puertos.SujetoElegible{Existe: true, Activo: false}, nil
	}
	caso := m.casoDeUso()

	_, err := caso.Crear(context.Background(), comandoCrearOrganizacionValido(t))
	var errElegible *dominio.ErrSujetoNoElegible
	if !errors.As(err, &errElegible) {
		t.Fatalf("se esperaba *ErrSujetoNoElegible, obtuvo %T: %v", err, err)
	}
}

func TestCrearOrganizacionCasoDeUso_AccesoDenegadoPorConfianza(t *testing.T) {
	m := nuevosMocksOrganizaciones(t)
	m.confianza.FnEvaluar = func(ctx context.Context, s puertos.SolicitudEvaluacion) (puertos.DecisionConfianza, error) {
		return puertos.DecisionConfianza{Permitido: false, Motivo: "riesgo_alto"}, nil
	}
	caso := m.casoDeUso()

	_, err := caso.Crear(context.Background(), comandoCrearOrganizacionValido(t))
	var errConfianza *dominio.ErrAccesoDenegadoPorConfianza
	if !errors.As(err, &errConfianza) {
		t.Fatalf("se esperaba *ErrAccesoDenegadoPorConfianza, obtuvo %T: %v", err, err)
	}
	if len(m.sujetos.LlamadasEsElegible) != 0 {
		t.Error("no debía consultarse elegibilidad si Confianza ya denegó")
	}
}

func TestCrearOrganizacionCasoDeUso_AliasInvalido(t *testing.T) {
	m := nuevosMocksOrganizaciones(t)
	caso := m.casoDeUso()

	cmd := comandoCrearOrganizacionValido(t)
	cmd.Alias = "AL"
	_, err := caso.Crear(context.Background(), cmd)
	var errAlias *dominio.ErrAliasInvalido
	if !errors.As(err, &errAlias) {
		t.Fatalf("se esperaba *ErrAliasInvalido, obtuvo %T: %v", err, err)
	}
}

func TestCrearOrganizacionCasoDeUso_AliasYaRegistrado(t *testing.T) {
	m := nuevosMocksOrganizaciones(t)
	m.organizaciones.FnGuardar = func(ctx context.Context, o *dominio.Organizacion) error {
		return &dominio.ErrAliasYaRegistrado{Alias: "acme"}
	}
	caso := m.casoDeUso()

	_, err := caso.Crear(context.Background(), comandoCrearOrganizacionValido(t))
	var errAlias *dominio.ErrAliasYaRegistrado
	if !errors.As(err, &errAlias) {
		t.Fatalf("se esperaba *ErrAliasYaRegistrado, obtuvo %T: %v", err, err)
	}
	// La violación llega desde el adaptador dentro de la UoW: no debe
	// auditarse ni publicarse nada.
	if len(m.auditoria.LlamadasRegistrar) != 0 {
		t.Error("no debía auditarse nada si Guardar falló")
	}
	if len(m.eventos.LlamadasPublicar) != 0 {
		t.Error("no debía publicarse nada si Guardar falló")
	}
}
