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

type mocksAutorizar struct {
	organizaciones *mocks.RepositorioOrganizaciones
	membresias     *mocks.RepositorioMembresias
	auditoria      *mocks.RegistroAuditoria
	reloj          *mocks.Reloj
}

func nuevosMocksAutorizar(t *testing.T) *mocksAutorizar {
	t.Helper()
	return &mocksAutorizar{
		organizaciones: &mocks.RepositorioOrganizaciones{},
		membresias:     &mocks.RepositorioMembresias{},
		auditoria:      &mocks.RegistroAuditoria{},
		reloj:          &mocks.Reloj{Fija: ahoraDePrueba()},
	}
}

func (m *mocksAutorizar) casoDeUso() *aplicacion.AutorizarCasoDeUso {
	return aplicacion.NuevoAutorizarCasoDeUso(m.organizaciones, m.membresias, m.auditoria, m.reloj)
}

func consultaAutorizacionValida() puertos.ConsultaAutorizacion {
	origen, _ := dominio.NuevoOrigenSolicitud("203.0.113.9", "agente-de-prueba", "", "")
	return puertos.ConsultaAutorizacion{
		IDUsuario:      idUsuarioValido1,
		IDOrganizacion: idOrganizacionValido1,
		Permiso:        dominio.PermisoMiembroVer.Valor(),
		Origen:         origen,
	}
}

// TestAutorizarCasoDeUso_SinMembresia cubre el primer caso del orden
// normativo: sin membresía -> denegado(sin_membresia), auditado, 404 aguas
// arriba.
func TestAutorizarCasoDeUso_SinMembresia(t *testing.T) {
	m := nuevosMocksAutorizar(t)
	m.membresias.FnBuscarVigente = func(ctx context.Context, u dominio.IDUsuario, o dominio.IDOrganizacion) (*dominio.Membresia, error) {
		return nil, nil
	}
	caso := m.casoDeUso()

	consulta := consultaAutorizacionValida()
	resultado, err := caso.Autorizar(context.Background(), consulta)
	if err != nil {
		t.Fatalf("Autorizar() devolvió error inesperado: %v", err)
	}
	if resultado.Permitido {
		t.Error("se esperaba Permitido=false")
	}
	if resultado.Motivo != dominio.MotivoDenegacionSinMembresia.Valor() {
		t.Errorf("Motivo = %q, esperado %q", resultado.Motivo, dominio.MotivoDenegacionSinMembresia.Valor())
	}
	if resultado.Rol != "" {
		t.Errorf("Rol = %q, esperado vacío", resultado.Rol)
	}
	if len(m.auditoria.LlamadasRegistrar) != 1 {
		t.Fatalf("se esperaba 1 auditoría, hubo %d", len(m.auditoria.LlamadasRegistrar))
	}
	if nombres := m.auditoria.NombresEventos(); nombres[0] != "AutorizacionDenegada" {
		t.Errorf("evento auditado = %q, esperado AutorizacionDenegada", nombres[0])
	}
	// El Origen de la consulta debe llegar intacto a la auditoría: es la
	// señal forense de un intento de escalada, no debe registrarse vacía.
	if got := m.auditoria.LlamadasRegistrar[0].Origen; got.IP().String() != consulta.Origen.IP().String() {
		t.Errorf("IP auditada = %q, esperada %q", got.IP().String(), consulta.Origen.IP().String())
	}
	// No debe pagarse la segunda lectura (RepositorioOrganizaciones) cuando
	// ni siquiera hay membresía.
	if len(m.organizaciones.LlamadasBuscarPorID) != 0 {
		t.Error("no debía consultarse RepositorioOrganizaciones sin membresía")
	}
}

// TestAutorizarCasoDeUso_MembresiaSuspendida cubre el segundo caso del
// orden normativo.
func TestAutorizarCasoDeUso_MembresiaSuspendida(t *testing.T) {
	m := nuevosMocksAutorizar(t)
	idOrg := idOrganizacionDePrueba(t, idOrganizacionValido1)
	idUsuario := idUsuarioDePrueba(t, idUsuarioValido1)
	membresia := membresiaSuspendidaDePrueba(t, idMembresiaValido1, idOrg, idUsuario, dominio.RolMiembro, ahoraDePrueba())
	m.membresias.FnBuscarVigente = func(ctx context.Context, u dominio.IDUsuario, o dominio.IDOrganizacion) (*dominio.Membresia, error) {
		return membresia, nil
	}
	caso := m.casoDeUso()

	resultado, err := caso.Autorizar(context.Background(), consultaAutorizacionValida())
	if err != nil {
		t.Fatalf("Autorizar() devolvió error inesperado: %v", err)
	}
	if resultado.Permitido {
		t.Error("se esperaba Permitido=false")
	}
	if resultado.Motivo != dominio.MotivoDenegacionMembresiaSuspendida.Valor() {
		t.Errorf("Motivo = %q, esperado %q", resultado.Motivo, dominio.MotivoDenegacionMembresiaSuspendida.Valor())
	}
	if resultado.Rol != dominio.RolMiembro.Valor() {
		t.Errorf("Rol = %q, esperado %q", resultado.Rol, dominio.RolMiembro.Valor())
	}
	if len(m.auditoria.LlamadasRegistrar) != 1 {
		t.Fatalf("se esperaba 1 auditoría, hubo %d", len(m.auditoria.LlamadasRegistrar))
	}
	// Membresía suspendida: el estado de la organización es irrelevante
	// para dominio.Autorizar, tampoco debe pagarse la segunda lectura.
	if len(m.organizaciones.LlamadasBuscarPorID) != 0 {
		t.Error("no debía consultarse RepositorioOrganizaciones con membresía suspendida")
	}
}

// TestAutorizarCasoDeUso_OrganizacionNoOperativa cubre el tercer caso del
// orden normativo: membresía activa, pero la organización no lo está.
func TestAutorizarCasoDeUso_OrganizacionNoOperativa(t *testing.T) {
	m := nuevosMocksAutorizar(t)
	idOrg := idOrganizacionDePrueba(t, idOrganizacionValido1)
	idUsuario := idUsuarioDePrueba(t, idUsuarioValido1)
	membresia := membresiaActivaDePrueba(t, idMembresiaValido1, idOrg, idUsuario, dominio.RolAdministrador, ahoraDePrueba())
	org := organizacionSuspendidaDePrueba(t, idOrganizacionValido1, "acme", "Acme", idUsuario, ahoraDePrueba())
	m.membresias.FnBuscarVigente = func(ctx context.Context, u dominio.IDUsuario, o dominio.IDOrganizacion) (*dominio.Membresia, error) {
		return membresia, nil
	}
	m.organizaciones.FnBuscarPorID = func(ctx context.Context, id dominio.IDOrganizacion) (*dominio.Organizacion, error) {
		return org, nil
	}
	caso := m.casoDeUso()

	resultado, err := caso.Autorizar(context.Background(), consultaAutorizacionValida())
	if err != nil {
		t.Fatalf("Autorizar() devolvió error inesperado: %v", err)
	}
	if resultado.Permitido {
		t.Error("se esperaba Permitido=false")
	}
	if resultado.Motivo != dominio.MotivoDenegacionOrganizacionNoOperativa.Valor() {
		t.Errorf("Motivo = %q, esperado %q", resultado.Motivo, dominio.MotivoDenegacionOrganizacionNoOperativa.Valor())
	}
	if len(m.organizaciones.LlamadasBuscarPorID) != 1 {
		t.Errorf("se esperaba 1 lectura de RepositorioOrganizaciones, hubo %d", len(m.organizaciones.LlamadasBuscarPorID))
	}
	if len(m.auditoria.LlamadasRegistrar) != 1 {
		t.Fatalf("se esperaba 1 auditoría, hubo %d", len(m.auditoria.LlamadasRegistrar))
	}
}

// TestAutorizarCasoDeUso_RolInsuficiente cubre el cuarto caso del orden
// normativo: membresía y organización activas, pero el rol no alcanza.
func TestAutorizarCasoDeUso_RolInsuficiente(t *testing.T) {
	m := nuevosMocksAutorizar(t)
	idOrg := idOrganizacionDePrueba(t, idOrganizacionValido1)
	idUsuario := idUsuarioDePrueba(t, idUsuarioValido1)
	membresia := membresiaActivaDePrueba(t, idMembresiaValido1, idOrg, idUsuario, dominio.RolMiembro, ahoraDePrueba())
	org := organizacionActivaDePrueba(t, idOrganizacionValido1, "acme", "Acme", idUsuario, ahoraDePrueba())
	m.membresias.FnBuscarVigente = func(ctx context.Context, u dominio.IDUsuario, o dominio.IDOrganizacion) (*dominio.Membresia, error) {
		return membresia, nil
	}
	m.organizaciones.FnBuscarPorID = func(ctx context.Context, id dominio.IDOrganizacion) (*dominio.Organizacion, error) {
		return org, nil
	}
	caso := m.casoDeUso()

	q := consultaAutorizacionValida()
	q.Permiso = dominio.PermisoMiembroRemover.Valor() // miembro no tiene este permiso
	resultado, err := caso.Autorizar(context.Background(), q)
	if err != nil {
		t.Fatalf("Autorizar() devolvió error inesperado: %v", err)
	}
	if resultado.Permitido {
		t.Error("se esperaba Permitido=false")
	}
	if resultado.Motivo != dominio.MotivoDenegacionRolInsuficiente.Valor() {
		t.Errorf("Motivo = %q, esperado %q", resultado.Motivo, dominio.MotivoDenegacionRolInsuficiente.Valor())
	}
	if len(m.auditoria.LlamadasRegistrar) != 1 {
		t.Fatalf("se esperaba 1 auditoría, hubo %d", len(m.auditoria.LlamadasRegistrar))
	}
}

// TestAutorizarCasoDeUso_Permitido cubre el quinto caso: todo en regla, se
// devuelve la concesión SIN auditar (INV-TEN-25).
func TestAutorizarCasoDeUso_Permitido(t *testing.T) {
	m := nuevosMocksAutorizar(t)
	idOrg := idOrganizacionDePrueba(t, idOrganizacionValido1)
	idUsuario := idUsuarioDePrueba(t, idUsuarioValido1)
	membresia := membresiaActivaDePrueba(t, idMembresiaValido1, idOrg, idUsuario, dominio.RolPropietario, ahoraDePrueba())
	org := organizacionActivaDePrueba(t, idOrganizacionValido1, "acme", "Acme", idUsuario, ahoraDePrueba())
	m.membresias.FnBuscarVigente = func(ctx context.Context, u dominio.IDUsuario, o dominio.IDOrganizacion) (*dominio.Membresia, error) {
		return membresia, nil
	}
	m.organizaciones.FnBuscarPorID = func(ctx context.Context, id dominio.IDOrganizacion) (*dominio.Organizacion, error) {
		return org, nil
	}
	caso := m.casoDeUso()

	resultado, err := caso.Autorizar(context.Background(), consultaAutorizacionValida())
	if err != nil {
		t.Fatalf("Autorizar() devolvió error inesperado: %v", err)
	}
	if !resultado.Permitido {
		t.Fatal("se esperaba Permitido=true")
	}
	if resultado.Rol != dominio.RolPropietario.Valor() {
		t.Errorf("Rol = %q, esperado %q", resultado.Rol, dominio.RolPropietario.Valor())
	}
	if resultado.Motivo != "" {
		t.Errorf("Motivo = %q, esperado vacío en una concesión", resultado.Motivo)
	}
	if len(m.auditoria.LlamadasRegistrar) != 0 {
		t.Error("INV-TEN-25: una concesión de autorización nunca se audita")
	}
}

// TestAutorizarCasoDeUso_PermisoDesconocido verifica que un permiso fuera
// del catálogo cerrado es un error del llamador (ErrPermisoDesconocido), no
// una denegación.
func TestAutorizarCasoDeUso_PermisoDesconocido(t *testing.T) {
	m := nuevosMocksAutorizar(t)
	caso := m.casoDeUso()

	q := consultaAutorizacionValida()
	q.Permiso = "recurso.accion_inventada"
	_, err := caso.Autorizar(context.Background(), q)
	var errPermiso *dominio.ErrPermisoDesconocido
	if !errors.As(err, &errPermiso) {
		t.Fatalf("se esperaba *ErrPermisoDesconocido, obtuvo %T: %v", err, err)
	}
	if len(m.membresias.LlamadasBuscarVigente) != 0 {
		t.Error("no debía consultarse la membresía con un permiso inválido")
	}
	if len(m.auditoria.LlamadasRegistrar) != 0 {
		t.Error("un permiso desconocido no es una denegación auditable")
	}
}
