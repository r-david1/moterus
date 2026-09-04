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

type mocksConsultas struct {
	organizaciones *mocks.RepositorioOrganizaciones
	membresias     *mocks.RepositorioMembresias
	autorizador    *autorizadorFalso
}

func nuevosMocksConsultas(t *testing.T) *mocksConsultas {
	t.Helper()
	return &mocksConsultas{
		organizaciones: &mocks.RepositorioOrganizaciones{
			FnBuscarPorID: func(ctx context.Context, id dominio.IDOrganizacion) (*dominio.Organizacion, error) {
				return organizacionActivaDePrueba(t, idOrganizacionValido1, "acme", "Acme", idUsuarioDePrueba(t, idUsuarioValido1), ahoraDePrueba()), nil
			},
		},
		membresias:  &mocks.RepositorioMembresias{},
		autorizador: &autorizadorFalso{},
	}
}

func (m *mocksConsultas) casoDeUso() *aplicacion.ConsultasCasoDeUso {
	return aplicacion.NuevoConsultasCasoDeUso(m.organizaciones, m.membresias, m.autorizador)
}

func TestConsultasCasoDeUso_ObtenerPorID_FlujoFeliz(t *testing.T) {
	m := nuevosMocksConsultas(t)
	m.membresias.FnContarActivasDeOrganizacion = func(ctx context.Context, o dominio.IDOrganizacion) (int, error) { return 5, nil }
	caso := m.casoDeUso()

	vista, err := caso.ObtenerPorID(context.Background(), puertos.ConsultaOrganizacionPorID{
		IDOrganizacion: idOrganizacionValido1,
		IDSujeto:       idUsuarioValido1,
		Origen:         origenDePrueba(t),
	})
	if err != nil {
		t.Fatalf("ObtenerPorID() devolvió error inesperado: %v", err)
	}
	if vista.ID != idOrganizacionValido1 {
		t.Errorf("ID = %q, esperado %q", vista.ID, idOrganizacionValido1)
	}
	if vista.MiembrosActivos != 5 {
		t.Errorf("MiembrosActivos = %d, esperado 5", vista.MiembrosActivos)
	}
}

func TestConsultasCasoDeUso_ObtenerPorID_NoEncontrada(t *testing.T) {
	m := nuevosMocksConsultas(t)
	m.organizaciones.FnBuscarPorID = func(ctx context.Context, id dominio.IDOrganizacion) (*dominio.Organizacion, error) {
		return nil, nil
	}
	caso := m.casoDeUso()

	_, err := caso.ObtenerPorID(context.Background(), puertos.ConsultaOrganizacionPorID{
		IDOrganizacion: idOrganizacionValido1,
		IDSujeto:       idUsuarioValido1,
		Origen:         origenDePrueba(t),
	})
	var errNoEncontrada *dominio.ErrOrganizacionNoEncontrada
	if !errors.As(err, &errNoEncontrada) {
		t.Fatalf("se esperaba *ErrOrganizacionNoEncontrada, obtuvo %T: %v", err, err)
	}
}

func TestConsultasCasoDeUso_ObtenerPorID_NoAutorizado(t *testing.T) {
	m := nuevosMocksConsultas(t)
	m.autorizador = autorizadorDenegado(dominio.MotivoDenegacionSinMembresia, "")
	caso := m.casoDeUso()

	_, err := caso.ObtenerPorID(context.Background(), puertos.ConsultaOrganizacionPorID{
		IDOrganizacion: idOrganizacionValido1,
		IDSujeto:       idUsuarioValido1,
		Origen:         origenDePrueba(t),
	})
	var errNoMiembro *dominio.ErrNoEsMiembro
	if !errors.As(err, &errNoMiembro) {
		t.Fatalf("se esperaba *ErrNoEsMiembro, obtuvo %T: %v", err, err)
	}
	if len(m.organizaciones.LlamadasBuscarPorID) != 0 {
		t.Error("no debía consultarse la organización si la autorización ya denegó")
	}
}

func TestConsultasCasoDeUso_ListarMiembros_FlujoFeliz(t *testing.T) {
	m := nuevosMocksConsultas(t)
	idOrg := idOrganizacionDePrueba(t, idOrganizacionValido1)
	m.membresias.FnListarDeOrganizacion = func(ctx context.Context, o dominio.IDOrganizacion) ([]*dominio.Membresia, error) {
		return []*dominio.Membresia{
			membresiaActivaDePrueba(t, idMembresiaValido1, idOrg, idUsuarioDePrueba(t, idUsuarioValido1), dominio.RolPropietario, ahoraDePrueba()),
			membresiaActivaDePrueba(t, idMembresiaValido2, idOrg, idUsuarioDePrueba(t, idUsuarioValido2), dominio.RolMiembro, ahoraDePrueba()),
		}, nil
	}
	caso := m.casoDeUso()

	vistas, err := caso.ListarMiembros(context.Background(), puertos.ConsultaMiembrosDeOrganizacion{
		IDOrganizacion: idOrganizacionValido1,
		IDSujeto:       idUsuarioValido1,
		Origen:         origenDePrueba(t),
	})
	if err != nil {
		t.Fatalf("ListarMiembros() devolvió error inesperado: %v", err)
	}
	if len(vistas) != 2 {
		t.Fatalf("se esperaban 2 miembros, hubo %d", len(vistas))
	}
	// INV-TEN-29: sin correo, sin nombre de usuario — VistaMiembro no tiene
	// esos campos, así que basta con verificar que compila y trae Rol/Estado.
	if vistas[0].Rol != dominio.RolPropietario.Valor() {
		t.Errorf("Rol[0] = %q, esperado propietario", vistas[0].Rol)
	}
}

func TestConsultasCasoDeUso_ListarMiembros_NoAutorizado(t *testing.T) {
	m := nuevosMocksConsultas(t)
	m.autorizador = autorizadorDenegado(dominio.MotivoDenegacionRolInsuficiente, dominio.RolMiembro.Valor())
	caso := m.casoDeUso()

	_, err := caso.ListarMiembros(context.Background(), puertos.ConsultaMiembrosDeOrganizacion{
		IDOrganizacion: idOrganizacionValido1,
		IDSujeto:       idUsuarioValido1,
		Origen:         origenDePrueba(t),
	})
	var errNoAutorizado *dominio.ErrNoAutorizado
	if !errors.As(err, &errNoAutorizado) {
		t.Fatalf("se esperaba *ErrNoAutorizado, obtuvo %T: %v", err, err)
	}
}

func TestConsultasCasoDeUso_ListarOrganizacionesDeUsuario_FiltraNoActivas(t *testing.T) {
	m := nuevosMocksConsultas(t)
	idUsuario := idUsuarioDePrueba(t, idUsuarioValido1)
	idOrgActiva := idOrganizacionDePrueba(t, idOrganizacionValido1)
	idOrgSuspendida := idOrganizacionDePrueba(t, idOrganizacionValido2)

	m.membresias.FnListarDeUsuario = func(ctx context.Context, u dominio.IDUsuario) ([]*dominio.Membresia, error) {
		return []*dominio.Membresia{
			membresiaActivaDePrueba(t, idMembresiaValido1, idOrgActiva, idUsuario, dominio.RolPropietario, ahoraDePrueba()),
			membresiaActivaDePrueba(t, idMembresiaValido2, idOrgSuspendida, idUsuario, dominio.RolMiembro, ahoraDePrueba()),
		}, nil
	}
	m.organizaciones.FnBuscarPorID = func(ctx context.Context, id dominio.IDOrganizacion) (*dominio.Organizacion, error) {
		if id.EsIgual(idOrgActiva) {
			return organizacionActivaDePrueba(t, idOrganizacionValido1, "acme", "Acme", idUsuario, ahoraDePrueba()), nil
		}
		return organizacionSuspendidaDePrueba(t, idOrganizacionValido2, "beta", "Beta", idUsuario, ahoraDePrueba()), nil
	}
	caso := m.casoDeUso()

	vistas, err := caso.ListarOrganizacionesDeUsuario(context.Background(), puertos.ConsultaOrganizacionesDeUsuario{
		IDUsuario:        idUsuarioValido1,
		IncluirNoActivas: false,
	})
	if err != nil {
		t.Fatalf("ListarOrganizacionesDeUsuario() devolvió error inesperado: %v", err)
	}
	if len(vistas) != 1 {
		t.Fatalf("se esperaba 1 organización (la activa), hubo %d", len(vistas))
	}
	if vistas[0].IDOrganizacion != idOrganizacionValido1 {
		t.Errorf("IDOrganizacion = %q, esperado %q", vistas[0].IDOrganizacion, idOrganizacionValido1)
	}

	// No requiere autorización de Tenencia (§3.6 del diseño).
	if len(m.autorizador.LlamadasAutorizar) != 0 {
		t.Error("ListarMisOrganizaciones no debía consultar VerificadorDeAutorizacion")
	}
}

func TestConsultasCasoDeUso_ListarOrganizacionesDeUsuario_IncluyeNoActivas(t *testing.T) {
	m := nuevosMocksConsultas(t)
	idUsuario := idUsuarioDePrueba(t, idUsuarioValido1)
	idOrgSuspendida := idOrganizacionDePrueba(t, idOrganizacionValido2)

	m.membresias.FnListarDeUsuario = func(ctx context.Context, u dominio.IDUsuario) ([]*dominio.Membresia, error) {
		return []*dominio.Membresia{
			membresiaActivaDePrueba(t, idMembresiaValido2, idOrgSuspendida, idUsuario, dominio.RolMiembro, ahoraDePrueba()),
		}, nil
	}
	m.organizaciones.FnBuscarPorID = func(ctx context.Context, id dominio.IDOrganizacion) (*dominio.Organizacion, error) {
		return organizacionSuspendidaDePrueba(t, idOrganizacionValido2, "beta", "Beta", idUsuario, ahoraDePrueba()), nil
	}
	caso := m.casoDeUso()

	vistas, err := caso.ListarOrganizacionesDeUsuario(context.Background(), puertos.ConsultaOrganizacionesDeUsuario{
		IDUsuario:        idUsuarioValido1,
		IncluirNoActivas: true,
	})
	if err != nil {
		t.Fatalf("ListarOrganizacionesDeUsuario() devolvió error inesperado: %v", err)
	}
	if len(vistas) != 1 {
		t.Fatalf("se esperaba 1 organización (incluida a pesar de estar suspendida), hubo %d", len(vistas))
	}
}

func TestConsultasCasoDeUso_RolEnOrganizacion_EsMiembro(t *testing.T) {
	m := nuevosMocksConsultas(t)
	idOrg := idOrganizacionDePrueba(t, idOrganizacionValido1)
	membresia := membresiaActivaDePrueba(t, idMembresiaValido1, idOrg, idUsuarioDePrueba(t, idUsuarioValido1), dominio.RolAdministrador, ahoraDePrueba())
	m.membresias.FnBuscarVigente = func(ctx context.Context, u dominio.IDUsuario, o dominio.IDOrganizacion) (*dominio.Membresia, error) {
		return membresia, nil
	}
	caso := m.casoDeUso()

	vista, err := caso.RolEnOrganizacion(context.Background(), puertos.ConsultaRolEnOrganizacion{
		IDUsuario:      idUsuarioValido1,
		IDOrganizacion: idOrganizacionValido1,
	})
	if err != nil {
		t.Fatalf("RolEnOrganizacion() devolvió error inesperado: %v", err)
	}
	if !vista.EsMiembro {
		t.Error("se esperaba EsMiembro=true")
	}
	if vista.Rol != dominio.RolAdministrador.Valor() {
		t.Errorf("Rol = %q, esperado %q", vista.Rol, dominio.RolAdministrador.Valor())
	}
	if vista.EstadoOrganizacion != dominio.EstadoOrganizacionActiva.String() {
		t.Errorf("EstadoOrganizacion = %q, esperado activa", vista.EstadoOrganizacion)
	}
	// No aplica la matriz de permisos ni exige autorización.
	if len(m.autorizador.LlamadasAutorizar) != 0 {
		t.Error("RolEnOrganizacion no debía consultar VerificadorDeAutorizacion")
	}
}

func TestConsultasCasoDeUso_RolEnOrganizacion_NoEsMiembro(t *testing.T) {
	m := nuevosMocksConsultas(t)
	m.membresias.FnBuscarVigente = func(ctx context.Context, u dominio.IDUsuario, o dominio.IDOrganizacion) (*dominio.Membresia, error) {
		return nil, nil
	}
	caso := m.casoDeUso()

	vista, err := caso.RolEnOrganizacion(context.Background(), puertos.ConsultaRolEnOrganizacion{
		IDUsuario:      idUsuarioValido2,
		IDOrganizacion: idOrganizacionValido1,
	})
	if err != nil {
		t.Fatalf("RolEnOrganizacion() devolvió error inesperado: %v", err)
	}
	if vista.EsMiembro {
		t.Error("se esperaba EsMiembro=false")
	}
	if vista.Rol != "" {
		t.Errorf("Rol = %q, esperado vacío", vista.Rol)
	}
}
