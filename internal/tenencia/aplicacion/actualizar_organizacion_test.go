package aplicacion_test

import (
	"context"
	"errors"
	"testing"

	"github.com/r-david1/moterus/internal/tenencia/dominio"
	"github.com/r-david1/moterus/internal/tenencia/puertos"
)

func TestActualizarOrganizacionCasoDeUso_FlujoFeliz(t *testing.T) {
	m := nuevosMocksOrganizaciones(t)
	nuevoNombre := "Acme Global"
	cmd := puertos.ComandoActualizarOrganizacion{
		IDOrganizacion: idOrganizacionValido1,
		IDSujeto:       idUsuarioValido1,
		Nombre:         &nuevoNombre,
		Origen:         origenDePrueba(t),
	}
	m.organizaciones.FnBuscarPorID = func(ctx context.Context, id dominio.IDOrganizacion) (*dominio.Organizacion, error) {
		return organizacionActivaDePrueba(t, idOrganizacionValido1, "acme", "Acme", idUsuarioDePrueba(t, idUsuarioValido1), ahoraDePrueba()), nil
	}
	caso := m.casoDeUso()

	vista, err := caso.Actualizar(context.Background(), cmd)
	if err != nil {
		t.Fatalf("Actualizar() devolvió error inesperado: %v", err)
	}
	if vista.Nombre != nuevoNombre {
		t.Errorf("Nombre = %q, esperado %q", vista.Nombre, nuevoNombre)
	}
	if len(m.organizaciones.LlamadasGuardar) != 1 {
		t.Errorf("se esperaba 1 Guardar, hubo %d", len(m.organizaciones.LlamadasGuardar))
	}
	if nombres := m.auditoria.NombresEventos(); len(nombres) != 1 || nombres[0] != "OrganizacionActualizada" {
		t.Errorf("eventos auditados = %v, esperado [OrganizacionActualizada]", nombres)
	}
}

func TestActualizarOrganizacionCasoDeUso_NoOpIdempotente(t *testing.T) {
	m := nuevosMocksOrganizaciones(t)
	mismoNombre := "Acme"
	cmd := puertos.ComandoActualizarOrganizacion{
		IDOrganizacion: idOrganizacionValido1,
		IDSujeto:       idUsuarioValido1,
		Nombre:         &mismoNombre,
		Origen:         origenDePrueba(t),
	}
	m.organizaciones.FnBuscarPorID = func(ctx context.Context, id dominio.IDOrganizacion) (*dominio.Organizacion, error) {
		return organizacionActivaDePrueba(t, idOrganizacionValido1, "acme", "Acme", idUsuarioDePrueba(t, idUsuarioValido1), ahoraDePrueba()), nil
	}
	caso := m.casoDeUso()

	_, err := caso.Actualizar(context.Background(), cmd)
	if err != nil {
		t.Fatalf("Actualizar() devolvió error inesperado: %v", err)
	}
	if len(m.organizaciones.LlamadasGuardar) != 0 {
		t.Error("un no-op no debía persistirse")
	}
	if len(m.auditoria.LlamadasRegistrar) != 0 {
		t.Error("un no-op no debía auditarse")
	}
}

func TestActualizarOrganizacionCasoDeUso_NoAutorizado(t *testing.T) {
	m := nuevosMocksOrganizaciones(t)
	m.autorizador = autorizadorDenegado(dominio.MotivoDenegacionSinMembresia, "")
	nuevoNombre := "Acme Global"
	cmd := puertos.ComandoActualizarOrganizacion{
		IDOrganizacion: idOrganizacionValido1,
		IDSujeto:       idUsuarioValido1,
		Nombre:         &nuevoNombre,
		Origen:         origenDePrueba(t),
	}
	caso := m.casoDeUso()

	_, err := caso.Actualizar(context.Background(), cmd)
	var errNoMiembro *dominio.ErrNoEsMiembro
	if !errors.As(err, &errNoMiembro) {
		t.Fatalf("se esperaba *ErrNoEsMiembro, obtuvo %T: %v", err, err)
	}
}

func TestActualizarOrganizacionCasoDeUso_OrganizacionNoOperativa(t *testing.T) {
	m := nuevosMocksOrganizaciones(t)
	m.organizaciones.FnBuscarPorID = func(ctx context.Context, id dominio.IDOrganizacion) (*dominio.Organizacion, error) {
		return organizacionSuspendidaDePrueba(t, idOrganizacionValido1, "acme", "Acme", idUsuarioDePrueba(t, idUsuarioValido1), ahoraDePrueba()), nil
	}
	nuevoNombre := "Acme Global"
	cmd := puertos.ComandoActualizarOrganizacion{
		IDOrganizacion: idOrganizacionValido1,
		IDSujeto:       idUsuarioValido1,
		Nombre:         &nuevoNombre,
		Origen:         origenDePrueba(t),
	}
	caso := m.casoDeUso()

	_, err := caso.Actualizar(context.Background(), cmd)
	var errNoOperativa *dominio.ErrOrganizacionNoOperativa
	if !errors.As(err, &errNoOperativa) {
		t.Fatalf("se esperaba *ErrOrganizacionNoOperativa, obtuvo %T: %v", err, err)
	}
}
