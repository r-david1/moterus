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

// mocksMembresias agrupa los dobles de prueba compartidos por todo el
// grupo de casos de uso de puertos.GestorDeMembresias (§3.2 y §3.3 del
// diseño): Agregar, CambiarRol, Remover, Abandonar, TransferirPropiedad.
type mocksMembresias struct {
	organizaciones *mocks.RepositorioOrganizaciones
	membresias     *mocks.RepositorioMembresias
	autorizador    *autorizadorFalso
	sujetos        *mocks.VerificadorDeSujetos
	auditoria      *mocks.RegistroAuditoria
	eventos        *mocks.PublicadorEventos
	reloj          *mocks.Reloj
	ids            *mocks.GeneradorIDs
	uow            *mocks.UnidadDeTrabajo
	politica       dominio.PoliticaOrganizacion
}

func nuevosMocksMembresias(t *testing.T) *mocksMembresias {
	t.Helper()
	return &mocksMembresias{
		organizaciones: &mocks.RepositorioOrganizaciones{
			FnCargarParaActualizar: func(ctx context.Context, id dominio.IDOrganizacion) (*dominio.Organizacion, error) {
				return organizacionActivaDePrueba(t, idOrganizacionValido1, "acme", "Acme", idUsuarioDePrueba(t, idUsuarioValido1), ahoraDePrueba()), nil
			},
			FnBuscarPorID: func(ctx context.Context, id dominio.IDOrganizacion) (*dominio.Organizacion, error) {
				return organizacionActivaDePrueba(t, idOrganizacionValido1, "acme", "Acme", idUsuarioDePrueba(t, idUsuarioValido1), ahoraDePrueba()), nil
			},
		},
		membresias:  &mocks.RepositorioMembresias{},
		autorizador: &autorizadorFalso{},
		sujetos:     &mocks.VerificadorDeSujetos{},
		auditoria:   &mocks.RegistroAuditoria{},
		eventos:     &mocks.PublicadorEventos{},
		reloj:       &mocks.Reloj{Fija: ahoraDePrueba()},
		ids: &mocks.GeneradorIDs{
			FnNuevoIDMembresia: func() (dominio.IDMembresia, error) { return dominio.IDMembresiaDesde(idMembresiaValido2) },
		},
		uow:      &mocks.UnidadDeTrabajo{},
		politica: politicaDePrueba(t),
	}
}

func (m *mocksMembresias) casoDeUso() *aplicacion.MembresiasCasoDeUso {
	return aplicacion.NuevoMembresiasCasoDeUso(
		m.organizaciones, m.membresias, m.autorizador, m.sujetos,
		m.auditoria, m.eventos, m.reloj, m.ids, m.uow, m.politica,
	)
}

func comandoAgregarMiembroValido(t *testing.T) puertos.ComandoAgregarMiembro {
	t.Helper()
	return puertos.ComandoAgregarMiembro{
		IDOrganizacion: idOrganizacionValido1,
		IDSujeto:       idUsuarioValido1,
		IDUsuario:      idUsuarioValido2,
		Rol:            dominio.RolMiembro.Valor(),
		Origen:         origenDePrueba(t),
	}
}

func TestAgregarMiembroCasoDeUso_FlujoFeliz(t *testing.T) {
	m := nuevosMocksMembresias(t)
	caso := m.casoDeUso()

	vista, err := caso.Agregar(context.Background(), comandoAgregarMiembroValido(t))
	if err != nil {
		t.Fatalf("Agregar() devolvió error inesperado: %v", err)
	}
	if vista.IDUsuario != idUsuarioValido2 {
		t.Errorf("IDUsuario = %q, esperado %q", vista.IDUsuario, idUsuarioValido2)
	}
	if vista.Rol != dominio.RolMiembro.Valor() {
		t.Errorf("Rol = %q, esperado %q", vista.Rol, dominio.RolMiembro.Valor())
	}
	if len(m.organizaciones.LlamadasCargarParaActualizar) != 1 {
		t.Errorf("se esperaba 1 CargarParaActualizar, hubo %d", len(m.organizaciones.LlamadasCargarParaActualizar))
	}
	if len(m.membresias.LlamadasGuardar) != 1 {
		t.Errorf("se esperaba 1 Guardar, hubo %d", len(m.membresias.LlamadasGuardar))
	}
	if nombres := m.auditoria.NombresEventos(); len(nombres) != 1 || nombres[0] != "MiembroAgregado" {
		t.Errorf("eventos auditados = %v, esperado [MiembroAgregado]", nombres)
	}
}

func TestAgregarMiembroCasoDeUso_SujetoNoElegible(t *testing.T) {
	m := nuevosMocksMembresias(t)
	m.sujetos.FnEsElegible = func(ctx context.Context, idUsuario string) (puertos.SujetoElegible, error) {
		return puertos.SujetoElegible{Existe: false}, nil
	}
	caso := m.casoDeUso()

	_, err := caso.Agregar(context.Background(), comandoAgregarMiembroValido(t))
	var errElegible *dominio.ErrSujetoNoElegible
	if !errors.As(err, &errElegible) {
		t.Fatalf("se esperaba *ErrSujetoNoElegible, obtuvo %T: %v", err, err)
	}
	if len(m.organizaciones.LlamadasCargarParaActualizar) != 0 {
		t.Error("no debía tomarse el candado de fila si el sujeto no es elegible")
	}
}

func TestAgregarMiembroCasoDeUso_RolSuperiorAlPropio(t *testing.T) {
	m := nuevosMocksMembresias(t)
	// El administrador SÍ tiene miembro.invitar, así que forzamos una
	// concesión con rol administrador para llegar a la regla de dominancia.
	m.autorizador = &autorizadorFalso{
		FnAutorizar: func(ctx context.Context, q puertos.ConsultaAutorizacion) (puertos.Autorizacion, error) {
			return puertos.Autorizacion{Permitido: true, IDOrganizacion: q.IDOrganizacion, Rol: dominio.RolAdministrador.Valor()}, nil
		},
	}
	caso := m.casoDeUso()

	cmd := comandoAgregarMiembroValido(t)
	cmd.Rol = dominio.RolPropietario.Valor() // un administrador no puede otorgar propietario
	_, err := caso.Agregar(context.Background(), cmd)
	var errDominancia *dominio.ErrRolSuperiorAlPropio
	if !errors.As(err, &errDominancia) {
		t.Fatalf("se esperaba *ErrRolSuperiorAlPropio, obtuvo %T: %v", err, err)
	}
	if len(m.sujetos.LlamadasEsElegible) != 0 {
		t.Error("no debía consultarse elegibilidad si la dominancia ya rechazó")
	}
}

func TestAgregarMiembroCasoDeUso_LimiteMiembrosExcedido(t *testing.T) {
	m := nuevosMocksMembresias(t)
	m.membresias.FnContarActivasDeOrganizacion = func(ctx context.Context, o dominio.IDOrganizacion) (int, error) {
		return m.politica.MaximoMiembrosActivos(), nil
	}
	caso := m.casoDeUso()

	_, err := caso.Agregar(context.Background(), comandoAgregarMiembroValido(t))
	var errLimite *dominio.ErrLimiteMiembrosExcedido
	if !errors.As(err, &errLimite) {
		t.Fatalf("se esperaba *ErrLimiteMiembrosExcedido, obtuvo %T: %v", err, err)
	}
}

func TestAgregarMiembroCasoDeUso_OrganizacionNoOperativa(t *testing.T) {
	m := nuevosMocksMembresias(t)
	m.organizaciones.FnCargarParaActualizar = func(ctx context.Context, id dominio.IDOrganizacion) (*dominio.Organizacion, error) {
		return organizacionSuspendidaDePrueba(t, idOrganizacionValido1, "acme", "Acme", idUsuarioDePrueba(t, idUsuarioValido1), ahoraDePrueba()), nil
	}
	caso := m.casoDeUso()

	_, err := caso.Agregar(context.Background(), comandoAgregarMiembroValido(t))
	var errNoOperativa *dominio.ErrOrganizacionNoOperativa
	if !errors.As(err, &errNoOperativa) {
		t.Fatalf("se esperaba *ErrOrganizacionNoOperativa, obtuvo %T: %v", err, err)
	}
}

func TestAgregarMiembroCasoDeUso_NoAutorizado(t *testing.T) {
	m := nuevosMocksMembresias(t)
	m.autorizador = autorizadorDenegado(dominio.MotivoDenegacionSinMembresia, "")
	caso := m.casoDeUso()

	_, err := caso.Agregar(context.Background(), comandoAgregarMiembroValido(t))
	var errNoMiembro *dominio.ErrNoEsMiembro
	if !errors.As(err, &errNoMiembro) {
		t.Fatalf("se esperaba *ErrNoEsMiembro, obtuvo %T: %v", err, err)
	}
}
