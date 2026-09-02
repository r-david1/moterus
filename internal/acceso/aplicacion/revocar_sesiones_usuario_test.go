package aplicacion_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/r-david1/moterus/internal/acceso/aplicacion"
	"github.com/r-david1/moterus/internal/acceso/dominio"
	"github.com/r-david1/moterus/internal/acceso/puertos/mocks"
)

type mocksRevocarSesionesUsuario struct {
	sesiones        *mocks.RepositorioSesiones
	listaRevocacion *mocks.ListaRevocacion
	auditoria       *mocks.RegistroAuditoria
	reloj           *mocks.Reloj
	uow             *mocks.UnidadDeTrabajo
	politica        dominio.PoliticaSesion
}

func nuevosMocksRevocarSesionesUsuario(t *testing.T) *mocksRevocarSesionesUsuario {
	t.Helper()
	return &mocksRevocarSesionesUsuario{
		sesiones:        &mocks.RepositorioSesiones{},
		listaRevocacion: &mocks.ListaRevocacion{},
		auditoria:       &mocks.RegistroAuditoria{},
		reloj:           &mocks.Reloj{Fija: ahoraDePrueba()},
		uow:             &mocks.UnidadDeTrabajo{},
		politica:        politicaDePrueba(t),
	}
}

func (m *mocksRevocarSesionesUsuario) casoDeUso() *aplicacion.RevocarSesionesUsuarioCasoDeUso {
	return aplicacion.NuevoRevocarSesionesUsuarioCasoDeUso(m.sesiones, m.listaRevocacion, m.auditoria, m.reloj, m.uow, m.politica)
}

func TestRevocarSesionesUsuarioCasoDeUso_FlujoFeliz(t *testing.T) {
	m := nuevosMocksRevocarSesionesUsuario(t)
	id1 := idSesionDePrueba(t, idSesionValido1)
	m.sesiones.FnRevocarActivasDeUsuario = func(ctx context.Context, u dominio.IDUsuario, excepto dominio.IDSesion,
		motivo dominio.MotivoRevocacion, ahora time.Time) ([]dominio.IDSesion, error) {
		if !motivo.EsIgual(dominio.MotivoRevocacionAdministrativa) {
			t.Errorf("motivo pasado al repositorio = %v, esperado revocacion_administrativa", motivo)
		}
		return []dominio.IDSesion{id1}, nil
	}
	caso := m.casoDeUso()

	resultado, err := caso.RevocarPorUsuario(context.Background(), aplicacion.ComandoRevocarSesionesDeUsuario{
		IDUsuario: idUsuarioValido1,
		Motivo:    dominio.MotivoRevocacionAdministrativa.Valor(),
		Origen:    origenDePrueba(t),
	})
	if err != nil {
		t.Fatalf("RevocarPorUsuario() devolvió error inesperado: %v", err)
	}
	if resultado.SesionesRevocadas != 1 {
		t.Errorf("SesionesRevocadas = %d, esperado 1", resultado.SesionesRevocadas)
	}
	if nombres := m.auditoria.NombresEventos(); len(nombres) != 1 || nombres[0] != "SesionRevocada" {
		t.Errorf("eventos auditados = %v, esperado [SesionRevocada]", nombres)
	}
	if len(m.listaRevocacion.LlamadasRevocarSesion) != 1 {
		t.Error("se esperaba propagar la revocación a la lista")
	}
}

func TestRevocarSesionesUsuarioCasoDeUso_MotivoInvalido(t *testing.T) {
	m := nuevosMocksRevocarSesionesUsuario(t)
	caso := m.casoDeUso()

	_, err := caso.RevocarPorUsuario(context.Background(), aplicacion.ComandoRevocarSesionesDeUsuario{
		IDUsuario: idUsuarioValido1,
		Motivo:    "no_existe_en_el_catalogo",
		Origen:    origenDePrueba(t),
	})
	var errMotivo *dominio.ErrMotivoRevocacionInvalido
	if !errors.As(err, &errMotivo) {
		t.Fatalf("se esperaba *ErrMotivoRevocacionInvalido, obtuvo %T: %v", err, err)
	}
	if len(m.sesiones.LlamadasRevocarActivasDeUsuario) != 0 {
		t.Error("un motivo inválido no debe llegar a revocar sesiones")
	}
}
