package aplicacion_test

import (
	"context"
	"errors"
	"testing"

	"github.com/r-david1/moterus/internal/tenencia/dominio"
	"github.com/r-david1/moterus/internal/tenencia/puertos"
)

func TestCambiarEstadoOrganizacionCasoDeUso_Suspender(t *testing.T) {
	m := nuevosMocksOrganizaciones(t)
	m.organizaciones.FnBuscarPorID = func(ctx context.Context, id dominio.IDOrganizacion) (*dominio.Organizacion, error) {
		return organizacionActivaDePrueba(t, idOrganizacionValido1, "acme", "Acme", idUsuarioDePrueba(t, idUsuarioValido1), ahoraDePrueba()), nil
	}
	caso := m.casoDeUso()

	cmd := puertos.ComandoCambiarEstadoOrganizacion{
		IDOrganizacion: idOrganizacionValido1,
		IDSujeto:       idUsuarioValido1,
		Destino:        dominio.EstadoOrganizacionSuspendida.String(),
		Motivo:         "investigación de abuso",
		Origen:         origenDePrueba(t),
	}
	vista, err := caso.CambiarEstado(context.Background(), cmd)
	if err != nil {
		t.Fatalf("CambiarEstado() devolvió error inesperado: %v", err)
	}
	if vista.Estado != dominio.EstadoOrganizacionSuspendida.String() {
		t.Errorf("Estado = %q, esperado suspendida", vista.Estado)
	}
	if nombres := m.auditoria.NombresEventos(); len(nombres) != 1 || nombres[0] != "EstadoOrganizacionCambiado" {
		t.Errorf("eventos auditados = %v, esperado [EstadoOrganizacionCambiado]", nombres)
	}
}

func TestCambiarEstadoOrganizacionCasoDeUso_SuspenderSinMotivo(t *testing.T) {
	m := nuevosMocksOrganizaciones(t)
	m.organizaciones.FnBuscarPorID = func(ctx context.Context, id dominio.IDOrganizacion) (*dominio.Organizacion, error) {
		return organizacionActivaDePrueba(t, idOrganizacionValido1, "acme", "Acme", idUsuarioDePrueba(t, idUsuarioValido1), ahoraDePrueba()), nil
	}
	caso := m.casoDeUso()

	cmd := puertos.ComandoCambiarEstadoOrganizacion{
		IDOrganizacion: idOrganizacionValido1,
		IDSujeto:       idUsuarioValido1,
		Destino:        dominio.EstadoOrganizacionSuspendida.String(),
		Origen:         origenDePrueba(t),
	}
	_, err := caso.CambiarEstado(context.Background(), cmd)
	var errMotivo *dominio.ErrMotivoCambioEstadoInvalido
	if !errors.As(err, &errMotivo) {
		t.Fatalf("se esperaba *ErrMotivoCambioEstadoInvalido, obtuvo %T: %v", err, err)
	}
}

func TestCambiarEstadoOrganizacionCasoDeUso_Reactivar(t *testing.T) {
	m := nuevosMocksOrganizaciones(t)
	m.organizaciones.FnBuscarPorID = func(ctx context.Context, id dominio.IDOrganizacion) (*dominio.Organizacion, error) {
		return organizacionSuspendidaDePrueba(t, idOrganizacionValido1, "acme", "Acme", idUsuarioDePrueba(t, idUsuarioValido1), ahoraDePrueba()), nil
	}
	caso := m.casoDeUso()

	cmd := puertos.ComandoCambiarEstadoOrganizacion{
		IDOrganizacion: idOrganizacionValido1,
		IDSujeto:       idUsuarioValido1,
		Destino:        dominio.EstadoOrganizacionActiva.String(),
		Origen:         origenDePrueba(t),
	}
	vista, err := caso.CambiarEstado(context.Background(), cmd)
	if err != nil {
		t.Fatalf("CambiarEstado() devolvió error inesperado: %v", err)
	}
	if vista.Estado != dominio.EstadoOrganizacionActiva.String() {
		t.Errorf("Estado = %q, esperado activa", vista.Estado)
	}
}

func TestCambiarEstadoOrganizacionCasoDeUso_Archivar(t *testing.T) {
	m := nuevosMocksOrganizaciones(t)
	m.organizaciones.FnBuscarPorID = func(ctx context.Context, id dominio.IDOrganizacion) (*dominio.Organizacion, error) {
		return organizacionActivaDePrueba(t, idOrganizacionValido1, "acme", "Acme", idUsuarioDePrueba(t, idUsuarioValido1), ahoraDePrueba()), nil
	}
	caso := m.casoDeUso()

	cmd := puertos.ComandoCambiarEstadoOrganizacion{
		IDOrganizacion: idOrganizacionValido1,
		IDSujeto:       idUsuarioValido1,
		Destino:        dominio.EstadoOrganizacionArchivada.String(),
		Motivo:         "cierre de cuenta a pedido del cliente",
		Origen:         origenDePrueba(t),
	}
	vista, err := caso.CambiarEstado(context.Background(), cmd)
	if err != nil {
		t.Fatalf("CambiarEstado() devolvió error inesperado: %v", err)
	}
	if vista.Estado != dominio.EstadoOrganizacionArchivada.String() {
		t.Errorf("Estado = %q, esperado archivada", vista.Estado)
	}
}

func TestCambiarEstadoOrganizacionCasoDeUso_TransicionInvalida(t *testing.T) {
	m := nuevosMocksOrganizaciones(t)
	m.organizaciones.FnBuscarPorID = func(ctx context.Context, id dominio.IDOrganizacion) (*dominio.Organizacion, error) {
		return organizacionSuspendidaDePrueba(t, idOrganizacionValido1, "acme", "Acme", idUsuarioDePrueba(t, idUsuarioValido1), ahoraDePrueba()), nil
	}
	caso := m.casoDeUso()

	// archivar directamente desde suspendida SÍ es válido según la máquina
	// de estados; usamos "activa -> archivada -> activa" para forzar una
	// transición prohibida: reactivar una organización archivada. Aquí
	// simulamos reactivar desde archivada usando el estado directamente.
	motivo, err := dominio.NuevoMotivo("cierre")
	if err != nil {
		t.Fatalf("motivo de prueba inválido: %v", err)
	}
	orgArchivada := organizacionActivaDePrueba(t, idOrganizacionValido1, "acme", "Acme", idUsuarioDePrueba(t, idUsuarioValido1), ahoraDePrueba())
	if err := orgArchivada.Archivar(motivo, ahoraDePrueba()); err != nil {
		t.Fatalf("no se pudo archivar la organización de prueba: %v", err)
	}
	m.organizaciones.FnBuscarPorID = func(ctx context.Context, id dominio.IDOrganizacion) (*dominio.Organizacion, error) {
		return orgArchivada, nil
	}

	cmd := puertos.ComandoCambiarEstadoOrganizacion{
		IDOrganizacion: idOrganizacionValido1,
		IDSujeto:       idUsuarioValido1,
		Destino:        dominio.EstadoOrganizacionActiva.String(),
		Origen:         origenDePrueba(t),
	}
	_, err = caso.CambiarEstado(context.Background(), cmd)
	var errTransicion *dominio.ErrTransicionEstadoOrganizacionInvalida
	if !errors.As(err, &errTransicion) {
		t.Fatalf("se esperaba *ErrTransicionEstadoOrganizacionInvalida, obtuvo %T: %v", err, err)
	}
}

func TestCambiarEstadoOrganizacionCasoDeUso_NoAutorizado(t *testing.T) {
	m := nuevosMocksOrganizaciones(t)
	m.autorizador = autorizadorDenegado(dominio.MotivoDenegacionRolInsuficiente, dominio.RolAdministrador.Valor())
	caso := m.casoDeUso()

	cmd := puertos.ComandoCambiarEstadoOrganizacion{
		IDOrganizacion: idOrganizacionValido1,
		IDSujeto:       idUsuarioValido1,
		Destino:        dominio.EstadoOrganizacionSuspendida.String(),
		Motivo:         "investigación",
		Origen:         origenDePrueba(t),
	}
	_, err := caso.CambiarEstado(context.Background(), cmd)
	var errNoAutorizado *dominio.ErrNoAutorizado
	if !errors.As(err, &errNoAutorizado) {
		t.Fatalf("se esperaba *ErrNoAutorizado, obtuvo %T: %v", err, err)
	}
}
