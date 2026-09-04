package aplicacion_test

import (
	"context"
	"errors"
	"testing"

	"github.com/r-david1/moterus/internal/tenencia/dominio"
	"github.com/r-david1/moterus/internal/tenencia/puertos"
)

func comandoAbandonarOrganizacionValido(t *testing.T) puertos.ComandoAbandonarOrganizacion {
	t.Helper()
	return puertos.ComandoAbandonarOrganizacion{
		IDOrganizacion: idOrganizacionValido1,
		IDSujeto:       idUsuarioValido1,
		Origen:         origenDePrueba(t),
	}
}

func TestAbandonarOrganizacionCasoDeUso_FlujoFeliz(t *testing.T) {
	m := nuevosMocksMembresias(t)
	idOrg := idOrganizacionDePrueba(t, idOrganizacionValido1)
	propia := membresiaActivaDePrueba(t, idMembresiaValido1, idOrg, idUsuarioDePrueba(t, idUsuarioValido1), dominio.RolAdministrador, ahoraDePrueba())
	m.membresias.FnBuscarVigente = func(ctx context.Context, u dominio.IDUsuario, o dominio.IDOrganizacion) (*dominio.Membresia, error) {
		return propia, nil
	}
	m.membresias.FnContarPropietariosActivos = func(ctx context.Context, o dominio.IDOrganizacion) (int, error) { return 1, nil }
	caso := m.casoDeUso()

	err := caso.Abandonar(context.Background(), comandoAbandonarOrganizacionValido(t))
	if err != nil {
		t.Fatalf("Abandonar() devolvió error inesperado: %v", err)
	}
	if !propia.Estado().EsIgual(dominio.EstadoMembresiaRemovida) {
		t.Error("la membresía debía quedar removida")
	}
	// No requiere autorización de Tenencia (INV-TEN-06 aparte): el
	// autorizadorFalso no debe siquiera consultarse.
	if len(m.autorizador.LlamadasAutorizar) != 0 {
		t.Error("Abandonar no debía consultar VerificadorDeAutorizacion")
	}
	if nombres := m.auditoria.NombresEventos(); len(nombres) != 1 || nombres[0] != "MiembroRemovido" {
		t.Errorf("eventos auditados = %v, esperado [MiembroRemovido]", nombres)
	}
}

// TestAbandonarOrganizacionCasoDeUso_UltimoPropietario verifica el
// callejón sin salida más probable del producto: el único propietario no
// puede abandonar.
func TestAbandonarOrganizacionCasoDeUso_UltimoPropietario(t *testing.T) {
	m := nuevosMocksMembresias(t)
	idOrg := idOrganizacionDePrueba(t, idOrganizacionValido1)
	propia := membresiaActivaDePrueba(t, idMembresiaValido1, idOrg, idUsuarioDePrueba(t, idUsuarioValido1), dominio.RolPropietario, ahoraDePrueba())
	m.membresias.FnBuscarVigente = func(ctx context.Context, u dominio.IDUsuario, o dominio.IDOrganizacion) (*dominio.Membresia, error) {
		return propia, nil
	}
	m.membresias.FnContarPropietariosActivos = func(ctx context.Context, o dominio.IDOrganizacion) (int, error) { return 1, nil }
	caso := m.casoDeUso()

	err := caso.Abandonar(context.Background(), comandoAbandonarOrganizacionValido(t))
	var errUltimo *dominio.ErrUltimoPropietario
	if !errors.As(err, &errUltimo) {
		t.Fatalf("se esperaba *ErrUltimoPropietario, obtuvo %T: %v", err, err)
	}
	if len(m.membresias.LlamadasGuardar) != 0 {
		t.Error("no debía persistirse nada si el invariante rechazó la operación")
	}
}

func TestAbandonarOrganizacionCasoDeUso_MembresiaNoEncontrada(t *testing.T) {
	m := nuevosMocksMembresias(t)
	m.membresias.FnBuscarVigente = func(ctx context.Context, u dominio.IDUsuario, o dominio.IDOrganizacion) (*dominio.Membresia, error) {
		return nil, nil
	}
	caso := m.casoDeUso()

	err := caso.Abandonar(context.Background(), comandoAbandonarOrganizacionValido(t))
	var errNoEncontrada *dominio.ErrMembresiaNoEncontrada
	if !errors.As(err, &errNoEncontrada) {
		t.Fatalf("se esperaba *ErrMembresiaNoEncontrada, obtuvo %T: %v", err, err)
	}
}

func TestAbandonarOrganizacionCasoDeUso_OrganizacionNoOperativa(t *testing.T) {
	m := nuevosMocksMembresias(t)
	m.organizaciones.FnCargarParaActualizar = func(ctx context.Context, id dominio.IDOrganizacion) (*dominio.Organizacion, error) {
		return organizacionSuspendidaDePrueba(t, idOrganizacionValido1, "acme", "Acme", idUsuarioDePrueba(t, idUsuarioValido1), ahoraDePrueba()), nil
	}
	caso := m.casoDeUso()

	err := caso.Abandonar(context.Background(), comandoAbandonarOrganizacionValido(t))
	var errNoOperativa *dominio.ErrOrganizacionNoOperativa
	if !errors.As(err, &errNoOperativa) {
		t.Fatalf("se esperaba *ErrOrganizacionNoOperativa, obtuvo %T: %v", err, err)
	}
}
