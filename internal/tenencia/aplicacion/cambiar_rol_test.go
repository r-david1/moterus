package aplicacion_test

import (
	"context"
	"errors"
	"testing"

	"github.com/r-david1/moterus/internal/tenencia/dominio"
	"github.com/r-david1/moterus/internal/tenencia/puertos"
)

func comandoCambiarRolValido(t *testing.T) puertos.ComandoCambiarRol {
	t.Helper()
	return puertos.ComandoCambiarRol{
		IDOrganizacion: idOrganizacionValido1,
		IDSujeto:       idUsuarioValido1,
		IDUsuario:      idUsuarioValido2,
		NuevoRol:       dominio.RolAdministrador.Valor(),
		Origen:         origenDePrueba(t),
	}
}

func TestCambiarRolCasoDeUso_FlujoFeliz(t *testing.T) {
	m := nuevosMocksMembresias(t)
	idOrg := idOrganizacionDePrueba(t, idOrganizacionValido1)
	objetivo := membresiaActivaDePrueba(t, idMembresiaValido2, idOrg, idUsuarioDePrueba(t, idUsuarioValido2), dominio.RolMiembro, ahoraDePrueba())
	m.membresias.FnBuscarVigente = func(ctx context.Context, u dominio.IDUsuario, o dominio.IDOrganizacion) (*dominio.Membresia, error) {
		return objetivo, nil
	}
	m.membresias.FnContarPropietariosActivos = func(ctx context.Context, o dominio.IDOrganizacion) (int, error) { return 1, nil }
	caso := m.casoDeUso()

	vista, err := caso.CambiarRol(context.Background(), comandoCambiarRolValido(t))
	if err != nil {
		t.Fatalf("CambiarRol() devolvió error inesperado: %v", err)
	}
	if vista.Rol != dominio.RolAdministrador.Valor() {
		t.Errorf("Rol = %q, esperado %q", vista.Rol, dominio.RolAdministrador.Valor())
	}
	// El candado de fila se toma ANTES de contar propietarios.
	if len(m.organizaciones.LlamadasCargarParaActualizar) != 1 {
		t.Errorf("se esperaba 1 CargarParaActualizar, hubo %d", len(m.organizaciones.LlamadasCargarParaActualizar))
	}
	if len(m.membresias.LlamadasContarPropietariosActivos) != 1 {
		t.Errorf("se esperaba 1 ContarPropietariosActivos, hubo %d", len(m.membresias.LlamadasContarPropietariosActivos))
	}
	if nombres := m.auditoria.NombresEventos(); len(nombres) != 1 || nombres[0] != "RolDeMiembroCambiado" {
		t.Errorf("eventos auditados = %v, esperado [RolDeMiembroCambiado]", nombres)
	}
}

// TestCambiarRolCasoDeUso_NoOpIdempotente verifica que cambiar al mismo rol
// no muta ni audita (mismo criterio que el logout idempotente de Acceso).
func TestCambiarRolCasoDeUso_NoOpIdempotente(t *testing.T) {
	m := nuevosMocksMembresias(t)
	idOrg := idOrganizacionDePrueba(t, idOrganizacionValido1)
	objetivo := membresiaActivaDePrueba(t, idMembresiaValido2, idOrg, idUsuarioDePrueba(t, idUsuarioValido2), dominio.RolAdministrador, ahoraDePrueba())
	m.membresias.FnBuscarVigente = func(ctx context.Context, u dominio.IDUsuario, o dominio.IDOrganizacion) (*dominio.Membresia, error) {
		return objetivo, nil
	}
	caso := m.casoDeUso()

	_, err := caso.CambiarRol(context.Background(), comandoCambiarRolValido(t)) // ya es administrador
	if err != nil {
		t.Fatalf("CambiarRol() devolvió error inesperado: %v", err)
	}
	if len(m.membresias.LlamadasGuardar) != 0 {
		t.Error("un cambio de rol no-op no debía persistirse")
	}
	if len(m.auditoria.LlamadasRegistrar) != 0 {
		t.Error("un cambio de rol no-op no debía auditarse")
	}
}

func TestCambiarRolCasoDeUso_UltimoPropietario(t *testing.T) {
	m := nuevosMocksMembresias(t)
	idOrg := idOrganizacionDePrueba(t, idOrganizacionValido1)
	objetivo := membresiaActivaDePrueba(t, idMembresiaValido2, idOrg, idUsuarioDePrueba(t, idUsuarioValido2), dominio.RolPropietario, ahoraDePrueba())
	m.membresias.FnBuscarVigente = func(ctx context.Context, u dominio.IDUsuario, o dominio.IDOrganizacion) (*dominio.Membresia, error) {
		return objetivo, nil
	}
	m.membresias.FnContarPropietariosActivos = func(ctx context.Context, o dominio.IDOrganizacion) (int, error) { return 1, nil }
	// El ejecutor es propietario (default del autorizadorFalso) degradando
	// al único otro propietario.
	caso := m.casoDeUso()

	cmd := comandoCambiarRolValido(t)
	cmd.NuevoRol = dominio.RolMiembro.Valor()
	_, err := caso.CambiarRol(context.Background(), cmd)
	var errUltimo *dominio.ErrUltimoPropietario
	if !errors.As(err, &errUltimo) {
		t.Fatalf("se esperaba *ErrUltimoPropietario, obtuvo %T: %v", err, err)
	}
	if len(m.membresias.LlamadasGuardar) != 0 {
		t.Error("no debía persistirse una mutación rechazada por el invariante")
	}
}

func TestCambiarRolCasoDeUso_MembresiaNoEncontrada(t *testing.T) {
	m := nuevosMocksMembresias(t)
	m.membresias.FnBuscarVigente = func(ctx context.Context, u dominio.IDUsuario, o dominio.IDOrganizacion) (*dominio.Membresia, error) {
		return nil, nil
	}
	caso := m.casoDeUso()

	_, err := caso.CambiarRol(context.Background(), comandoCambiarRolValido(t))
	var errNoEncontrada *dominio.ErrMembresiaNoEncontrada
	if !errors.As(err, &errNoEncontrada) {
		t.Fatalf("se esperaba *ErrMembresiaNoEncontrada, obtuvo %T: %v", err, err)
	}
}

func TestCambiarRolCasoDeUso_MembresiaDominante(t *testing.T) {
	m := nuevosMocksMembresias(t)
	idOrg := idOrganizacionDePrueba(t, idOrganizacionValido1)
	objetivo := membresiaActivaDePrueba(t, idMembresiaValido2, idOrg, idUsuarioDePrueba(t, idUsuarioValido2), dominio.RolPropietario, ahoraDePrueba())
	m.membresias.FnBuscarVigente = func(ctx context.Context, u dominio.IDUsuario, o dominio.IDOrganizacion) (*dominio.Membresia, error) {
		return objetivo, nil
	}
	// El ejecutor solo llega con rol administrador (autorizado igual porque
	// administrador SÍ tiene miembro.cambiar_rol en la matriz), pero no
	// puede tocar a un propietario.
	m.autorizador.FnAutorizar = func(ctx context.Context, q puertos.ConsultaAutorizacion) (puertos.Autorizacion, error) {
		return puertos.Autorizacion{Permitido: true, IDOrganizacion: q.IDOrganizacion, Rol: dominio.RolAdministrador.Valor()}, nil
	}
	caso := m.casoDeUso()

	cmd := comandoCambiarRolValido(t)
	cmd.NuevoRol = dominio.RolMiembro.Valor()
	_, err := caso.CambiarRol(context.Background(), cmd)
	var errDominante *dominio.ErrMembresiaDominante
	if !errors.As(err, &errDominante) {
		t.Fatalf("se esperaba *ErrMembresiaDominante, obtuvo %T: %v", err, err)
	}
}

func TestCambiarRolCasoDeUso_OrganizacionNoOperativa(t *testing.T) {
	m := nuevosMocksMembresias(t)
	m.organizaciones.FnCargarParaActualizar = func(ctx context.Context, id dominio.IDOrganizacion) (*dominio.Organizacion, error) {
		return organizacionSuspendidaDePrueba(t, idOrganizacionValido1, "acme", "Acme", idUsuarioDePrueba(t, idUsuarioValido1), ahoraDePrueba()), nil
	}
	caso := m.casoDeUso()

	_, err := caso.CambiarRol(context.Background(), comandoCambiarRolValido(t))
	var errNoOperativa *dominio.ErrOrganizacionNoOperativa
	if !errors.As(err, &errNoOperativa) {
		t.Fatalf("se esperaba *ErrOrganizacionNoOperativa, obtuvo %T: %v", err, err)
	}
}
