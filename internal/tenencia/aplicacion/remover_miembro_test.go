package aplicacion_test

import (
	"context"
	"errors"
	"testing"

	"github.com/r-david1/moterus/internal/tenencia/dominio"
	"github.com/r-david1/moterus/internal/tenencia/puertos"
)

func comandoRemoverMiembroValido(t *testing.T) puertos.ComandoRemoverMiembro {
	t.Helper()
	return puertos.ComandoRemoverMiembro{
		IDOrganizacion: idOrganizacionValido1,
		IDSujeto:       idUsuarioValido1,
		IDUsuario:      idUsuarioValido2,
		Origen:         origenDePrueba(t),
	}
}

func TestRemoverMiembroCasoDeUso_FlujoFeliz(t *testing.T) {
	m := nuevosMocksMembresias(t)
	idOrg := idOrganizacionDePrueba(t, idOrganizacionValido1)
	objetivo := membresiaActivaDePrueba(t, idMembresiaValido2, idOrg, idUsuarioDePrueba(t, idUsuarioValido2), dominio.RolMiembro, ahoraDePrueba())
	m.membresias.FnBuscarVigente = func(ctx context.Context, u dominio.IDUsuario, o dominio.IDOrganizacion) (*dominio.Membresia, error) {
		return objetivo, nil
	}
	m.membresias.FnContarPropietariosActivos = func(ctx context.Context, o dominio.IDOrganizacion) (int, error) { return 1, nil }
	caso := m.casoDeUso()

	err := caso.Remover(context.Background(), comandoRemoverMiembroValido(t))
	if err != nil {
		t.Fatalf("Remover() devolvió error inesperado: %v", err)
	}
	if !objetivo.Estado().EsIgual(dominio.EstadoMembresiaRemovida) {
		t.Error("la membresía debía quedar removida")
	}
	if len(m.organizaciones.LlamadasCargarParaActualizar) != 1 {
		t.Errorf("se esperaba 1 CargarParaActualizar, hubo %d", len(m.organizaciones.LlamadasCargarParaActualizar))
	}
	if nombres := m.auditoria.NombresEventos(); len(nombres) != 1 || nombres[0] != "MiembroRemovido" {
		t.Errorf("eventos auditados = %v, esperado [MiembroRemovido]", nombres)
	}
}

func TestRemoverMiembroCasoDeUso_UltimoPropietario(t *testing.T) {
	m := nuevosMocksMembresias(t)
	idOrg := idOrganizacionDePrueba(t, idOrganizacionValido1)
	objetivo := membresiaActivaDePrueba(t, idMembresiaValido2, idOrg, idUsuarioDePrueba(t, idUsuarioValido2), dominio.RolPropietario, ahoraDePrueba())
	m.membresias.FnBuscarVigente = func(ctx context.Context, u dominio.IDUsuario, o dominio.IDOrganizacion) (*dominio.Membresia, error) {
		return objetivo, nil
	}
	m.membresias.FnContarPropietariosActivos = func(ctx context.Context, o dominio.IDOrganizacion) (int, error) { return 1, nil }
	caso := m.casoDeUso()

	err := caso.Remover(context.Background(), comandoRemoverMiembroValido(t))
	var errUltimo *dominio.ErrUltimoPropietario
	if !errors.As(err, &errUltimo) {
		t.Fatalf("se esperaba *ErrUltimoPropietario, obtuvo %T: %v", err, err)
	}
}

func TestRemoverMiembroCasoDeUso_NoAutorizado(t *testing.T) {
	m := nuevosMocksMembresias(t)
	m.autorizador = autorizadorDenegado(dominio.MotivoDenegacionRolInsuficiente, dominio.RolMiembro.Valor())
	caso := m.casoDeUso()

	err := caso.Remover(context.Background(), comandoRemoverMiembroValido(t))
	var errNoAutorizado *dominio.ErrNoAutorizado
	if !errors.As(err, &errNoAutorizado) {
		t.Fatalf("se esperaba *ErrNoAutorizado, obtuvo %T: %v", err, err)
	}
	if len(m.organizaciones.LlamadasCargarParaActualizar) != 0 {
		t.Error("no debía tomarse el candado de fila si la autorización ya denegó")
	}
}

func TestRemoverMiembroCasoDeUso_MembresiaNoEncontrada(t *testing.T) {
	m := nuevosMocksMembresias(t)
	m.membresias.FnBuscarVigente = func(ctx context.Context, u dominio.IDUsuario, o dominio.IDOrganizacion) (*dominio.Membresia, error) {
		return nil, nil
	}
	caso := m.casoDeUso()

	err := caso.Remover(context.Background(), comandoRemoverMiembroValido(t))
	var errNoEncontrada *dominio.ErrMembresiaNoEncontrada
	if !errors.As(err, &errNoEncontrada) {
		t.Fatalf("se esperaba *ErrMembresiaNoEncontrada, obtuvo %T: %v", err, err)
	}
}
