package aplicacion_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/r-david1/moterus/internal/tenencia/dominio"
	"github.com/r-david1/moterus/internal/tenencia/puertos"
)

func comandoRevocarInvitacionValido(t *testing.T) puertos.ComandoRevocarInvitacion {
	t.Helper()
	return puertos.ComandoRevocarInvitacion{
		IDOrganizacion: idOrganizacionValido1,
		IDSujeto:       idUsuarioValido1,
		IDInvitacion:   idInvitacionValido1,
		Origen:         origenDePrueba(t),
	}
}

func TestRevocarInvitacionCasoDeUso_FlujoFeliz(t *testing.T) {
	m := nuevosMocksInvitaciones(t)
	idOrg := idOrganizacionDePrueba(t, idOrganizacionValido1)
	correo := correoDePrueba(t, "invitado@ejemplo.com")
	pendiente := invitacionPendienteDePrueba(t, idOrg, correo, dominio.RolMiembro, idUsuarioDePrueba(t, idUsuarioValido1),
		tokenInvitacionValor1, ahoraDePrueba().Add(-time.Hour), ahoraDePrueba().Add(6*24*time.Hour))
	m.invitaciones.FnBuscarPorID = func(ctx context.Context, id dominio.IDInvitacion) (*dominio.Invitacion, error) {
		return pendiente, nil
	}
	caso := m.casoDeUso()

	err := caso.Revocar(context.Background(), comandoRevocarInvitacionValido(t))
	if err != nil {
		t.Fatalf("Revocar() devolvió error inesperado: %v", err)
	}
	if !pendiente.Estado().EsIgual(dominio.EstadoInvitacionRevocada) {
		t.Error("la invitación debía quedar revocada")
	}
	if nombres := m.auditoria.NombresEventos(); len(nombres) != 1 || nombres[0] != "InvitacionResuelta" {
		t.Errorf("eventos auditados = %v, esperado [InvitacionResuelta]", nombres)
	}
}

func TestRevocarInvitacionCasoDeUso_NoEncontrada(t *testing.T) {
	m := nuevosMocksInvitaciones(t)
	m.invitaciones.FnBuscarPorID = func(ctx context.Context, id dominio.IDInvitacion) (*dominio.Invitacion, error) {
		return nil, nil
	}
	caso := m.casoDeUso()

	err := caso.Revocar(context.Background(), comandoRevocarInvitacionValido(t))
	var errTransicion *dominio.ErrTransicionEstadoInvitacionInvalida
	if !errors.As(err, &errTransicion) {
		t.Fatalf("se esperaba *ErrTransicionEstadoInvitacionInvalida, obtuvo %T: %v", err, err)
	}
}

func TestRevocarInvitacionCasoDeUso_YaResuelta(t *testing.T) {
	m := nuevosMocksInvitaciones(t)
	idOrg := idOrganizacionDePrueba(t, idOrganizacionValido1)
	correo := correoDePrueba(t, "invitado@ejemplo.com")
	pendiente := invitacionPendienteDePrueba(t, idOrg, correo, dominio.RolMiembro, idUsuarioDePrueba(t, idUsuarioValido1),
		tokenInvitacionValor1, ahoraDePrueba().Add(-time.Hour), ahoraDePrueba().Add(6*24*time.Hour))
	if err := pendiente.Revocar(ahoraDePrueba()); err != nil {
		t.Fatalf("no se pudo preparar la invitación de prueba: %v", err)
	}
	m.invitaciones.FnBuscarPorID = func(ctx context.Context, id dominio.IDInvitacion) (*dominio.Invitacion, error) {
		return pendiente, nil
	}
	caso := m.casoDeUso()

	err := caso.Revocar(context.Background(), comandoRevocarInvitacionValido(t))
	var errTransicion *dominio.ErrTransicionEstadoInvitacionInvalida
	if !errors.As(err, &errTransicion) {
		t.Fatalf("se esperaba *ErrTransicionEstadoInvitacionInvalida, obtuvo %T: %v", err, err)
	}
}

func TestRevocarInvitacionCasoDeUso_NoAutorizado(t *testing.T) {
	m := nuevosMocksInvitaciones(t)
	m.autorizador = autorizadorDenegado(dominio.MotivoDenegacionSinMembresia, "")
	caso := m.casoDeUso()

	err := caso.Revocar(context.Background(), comandoRevocarInvitacionValido(t))
	var errNoMiembro *dominio.ErrNoEsMiembro
	if !errors.As(err, &errNoMiembro) {
		t.Fatalf("se esperaba *ErrNoEsMiembro, obtuvo %T: %v", err, err)
	}
	if len(m.invitaciones.LlamadasBuscarPorID) != 0 {
		t.Error("no debía consultarse la invitación si la autorización ya denegó")
	}
}
