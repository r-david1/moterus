package aplicacion_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/r-david1/moterus/internal/tenencia/dominio"
	"github.com/r-david1/moterus/internal/tenencia/puertos"
)

func comandoTransferirPropiedadValido(t *testing.T) puertos.ComandoTransferirPropiedad {
	t.Helper()
	return puertos.ComandoTransferirPropiedad{
		IDOrganizacion:          idOrganizacionValido1,
		IDSujeto:                idUsuarioValido1,
		IDNuevoPropietario:      idUsuarioValido2,
		RolResultanteDelCedente: dominio.RolAdministrador.Valor(),
		Origen:                  origenDePrueba(t),
	}
}

// setUpMembresiasTransferencia configura cedente (propietario) y
// destinatario (administrador activo) en el mock de RepositorioMembresias.
func setUpMembresiasTransferencia(t *testing.T, m *mocksMembresias, rolDestinatario dominio.Rol, estadoDestinatario func(t *testing.T, idMembresia string, idOrganizacion dominio.IDOrganizacion, idUsuario dominio.IDUsuario, rol dominio.Rol, ahora time.Time) *dominio.Membresia) (*dominio.Membresia, *dominio.Membresia) {
	t.Helper()
	idOrg := idOrganizacionDePrueba(t, idOrganizacionValido1)
	cedente := membresiaActivaDePrueba(t, idMembresiaValido1, idOrg, idUsuarioDePrueba(t, idUsuarioValido1), dominio.RolPropietario, ahoraDePrueba())
	destinatario := estadoDestinatario(t, idMembresiaValido2, idOrg, idUsuarioDePrueba(t, idUsuarioValido2), rolDestinatario, ahoraDePrueba())
	m.membresias.FnBuscarVigente = func(ctx context.Context, u dominio.IDUsuario, o dominio.IDOrganizacion) (*dominio.Membresia, error) {
		if u.EsIgual(idUsuarioDePrueba(t, idUsuarioValido1)) {
			return cedente, nil
		}
		if u.EsIgual(idUsuarioDePrueba(t, idUsuarioValido2)) {
			return destinatario, nil
		}
		return nil, nil
	}
	m.membresias.FnContarPropietariosActivos = func(ctx context.Context, o dominio.IDOrganizacion) (int, error) { return 1, nil }
	return cedente, destinatario
}

func TestTransferirPropiedadCasoDeUso_FlujoFeliz(t *testing.T) {
	m := nuevosMocksMembresias(t)
	cedente, destinatario := setUpMembresiasTransferencia(t, m, dominio.RolAdministrador, membresiaActivaDePrueba)
	caso := m.casoDeUso()

	err := caso.TransferirPropiedad(context.Background(), comandoTransferirPropiedadValido(t))
	if err != nil {
		t.Fatalf("TransferirPropiedad() devolvió error inesperado: %v", err)
	}
	if !destinatario.Rol().EsIgual(dominio.RolPropietario) {
		t.Errorf("rol del destinatario = %q, esperado propietario", destinatario.Rol().Valor())
	}
	if !cedente.Rol().EsIgual(dominio.RolAdministrador) {
		t.Errorf("rol del cedente = %q, esperado administrador", cedente.Rol().Valor())
	}
	if len(m.membresias.LlamadasGuardar) != 2 {
		t.Errorf("se esperaban 2 Guardar (destinatario + cedente), hubo %d", len(m.membresias.LlamadasGuardar))
	}
	nombres := m.auditoria.NombresEventos()
	if len(nombres) != 2 || nombres[0] != "RolDeMiembroCambiado" || nombres[1] != "RolDeMiembroCambiado" {
		t.Errorf("eventos auditados = %v, esperado 2x RolDeMiembroCambiado", nombres)
	}
}

// TestTransferirPropiedadCasoDeUso_ConservarPropiedad verifica que
// RolResultanteDelCedente == "" mantiene al cedente como propietario
// (co-propiedad explícita): solo se muta el destinatario.
func TestTransferirPropiedadCasoDeUso_ConservarPropiedad(t *testing.T) {
	m := nuevosMocksMembresias(t)
	cedente, destinatario := setUpMembresiasTransferencia(t, m, dominio.RolMiembro, membresiaActivaDePrueba)
	caso := m.casoDeUso()

	cmd := comandoTransferirPropiedadValido(t)
	cmd.RolResultanteDelCedente = ""
	err := caso.TransferirPropiedad(context.Background(), cmd)
	if err != nil {
		t.Fatalf("TransferirPropiedad() devolvió error inesperado: %v", err)
	}
	if !destinatario.Rol().EsIgual(dominio.RolPropietario) {
		t.Errorf("rol del destinatario = %q, esperado propietario", destinatario.Rol().Valor())
	}
	if !cedente.Rol().EsIgual(dominio.RolPropietario) {
		t.Errorf("rol del cedente = %q, esperado propietario (conservado)", cedente.Rol().Valor())
	}
	if len(m.membresias.LlamadasGuardar) != 1 {
		t.Errorf("se esperaba 1 Guardar (solo destinatario), hubo %d", len(m.membresias.LlamadasGuardar))
	}
	if nombres := m.auditoria.NombresEventos(); len(nombres) != 1 {
		t.Errorf("eventos auditados = %v, esperado 1 solo (destinatario)", nombres)
	}
}

func TestTransferirPropiedadCasoDeUso_DestinatarioNoEncontrado(t *testing.T) {
	m := nuevosMocksMembresias(t)
	idOrg := idOrganizacionDePrueba(t, idOrganizacionValido1)
	cedente := membresiaActivaDePrueba(t, idMembresiaValido1, idOrg, idUsuarioDePrueba(t, idUsuarioValido1), dominio.RolPropietario, ahoraDePrueba())
	m.membresias.FnBuscarVigente = func(ctx context.Context, u dominio.IDUsuario, o dominio.IDOrganizacion) (*dominio.Membresia, error) {
		if u.EsIgual(idUsuarioDePrueba(t, idUsuarioValido1)) {
			return cedente, nil
		}
		return nil, nil // destinatario no es miembro
	}
	caso := m.casoDeUso()

	err := caso.TransferirPropiedad(context.Background(), comandoTransferirPropiedadValido(t))
	var errNoEncontrada *dominio.ErrMembresiaNoEncontrada
	if !errors.As(err, &errNoEncontrada) {
		t.Fatalf("se esperaba *ErrMembresiaNoEncontrada, obtuvo %T: %v", err, err)
	}
}

// TestTransferirPropiedadCasoDeUso_DestinatarioSuspendido verifica que un
// miembro suspendido no es un destinatario válido (debe ser "miembro
// activo previo", §3.2 del diseño).
func TestTransferirPropiedadCasoDeUso_DestinatarioSuspendido(t *testing.T) {
	m := nuevosMocksMembresias(t)
	setUpMembresiasTransferencia(t, m, dominio.RolMiembro, membresiaSuspendidaDePrueba)
	caso := m.casoDeUso()

	err := caso.TransferirPropiedad(context.Background(), comandoTransferirPropiedadValido(t))
	var errNoEncontrada *dominio.ErrMembresiaNoEncontrada
	if !errors.As(err, &errNoEncontrada) {
		t.Fatalf("se esperaba *ErrMembresiaNoEncontrada, obtuvo %T: %v", err, err)
	}
}

func TestTransferirPropiedadCasoDeUso_NoAutorizado(t *testing.T) {
	m := nuevosMocksMembresias(t)
	m.autorizador = autorizadorDenegado(dominio.MotivoDenegacionRolInsuficiente, dominio.RolAdministrador.Valor())
	caso := m.casoDeUso()

	err := caso.TransferirPropiedad(context.Background(), comandoTransferirPropiedadValido(t))
	var errNoAutorizado *dominio.ErrNoAutorizado
	if !errors.As(err, &errNoAutorizado) {
		t.Fatalf("se esperaba *ErrNoAutorizado, obtuvo %T: %v", err, err)
	}
	if len(m.organizaciones.LlamadasCargarParaActualizar) != 0 {
		t.Error("no debía tomarse el candado de fila si la autorización ya denegó")
	}
}

func TestTransferirPropiedadCasoDeUso_OrganizacionNoOperativa(t *testing.T) {
	m := nuevosMocksMembresias(t)
	m.organizaciones.FnCargarParaActualizar = func(ctx context.Context, id dominio.IDOrganizacion) (*dominio.Organizacion, error) {
		return organizacionSuspendidaDePrueba(t, idOrganizacionValido1, "acme", "Acme", idUsuarioDePrueba(t, idUsuarioValido1), ahoraDePrueba()), nil
	}
	caso := m.casoDeUso()

	err := caso.TransferirPropiedad(context.Background(), comandoTransferirPropiedadValido(t))
	var errNoOperativa *dominio.ErrOrganizacionNoOperativa
	if !errors.As(err, &errNoOperativa) {
		t.Fatalf("se esperaba *ErrOrganizacionNoOperativa, obtuvo %T: %v", err, err)
	}
}
