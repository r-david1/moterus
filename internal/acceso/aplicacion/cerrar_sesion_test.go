package aplicacion_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/r-david1/moterus/internal/acceso/aplicacion"
	"github.com/r-david1/moterus/internal/acceso/dominio"
	"github.com/r-david1/moterus/internal/acceso/puertos"
	"github.com/r-david1/moterus/internal/acceso/puertos/mocks"
)

type mocksCerrarSesion struct {
	sesiones        *mocks.RepositorioSesiones
	confianza       *mocks.EvaluadorConfianza
	listaRevocacion *mocks.ListaRevocacion
	auditoria       *mocks.RegistroAuditoria
	reloj           *mocks.Reloj
	uow             *mocks.UnidadDeTrabajo
	politica        dominio.PoliticaSesion
}

func nuevosMocksCerrarSesion(t *testing.T) *mocksCerrarSesion {
	t.Helper()
	return &mocksCerrarSesion{
		sesiones:        &mocks.RepositorioSesiones{},
		confianza:       &mocks.EvaluadorConfianza{},
		listaRevocacion: &mocks.ListaRevocacion{},
		auditoria:       &mocks.RegistroAuditoria{},
		reloj:           &mocks.Reloj{Fija: ahoraDePrueba()},
		uow:             &mocks.UnidadDeTrabajo{},
		politica:        politicaDePrueba(t),
	}
}

func (m *mocksCerrarSesion) casoDeUso() *aplicacion.CerrarSesionCasoDeUso {
	return aplicacion.NuevoCerrarSesionCasoDeUso(m.sesiones, m.confianza, m.listaRevocacion, m.auditoria, m.reloj, m.uow, m.politica)
}

func TestCerrarSesionCasoDeUso_Cerrar_FlujoFeliz(t *testing.T) {
	usuarioID := idUsuarioDePrueba(t, idUsuarioValido1)
	sesion := sesionActivaDePrueba(t, idSesionDePrueba(t, idSesionValido1), usuarioID, 0,
		tokenRefrescoValor1, ahoraDePrueba().Add(-1*time.Hour), ahoraDePrueba().Add(23*time.Hour), ahoraDePrueba().Add(47*time.Hour))

	m := nuevosMocksCerrarSesion(t)
	m.sesiones.FnBuscarPorID = func(ctx context.Context, id dominio.IDSesion) (*dominio.Sesion, error) { return sesion, nil }
	caso := m.casoDeUso()

	err := caso.Cerrar(context.Background(), aplicacion.ComandoCerrarSesion{
		IDSesion: idSesionValido1, IDUsuario: idUsuarioValido1, Origen: origenDePrueba(t),
	})
	if err != nil {
		t.Fatalf("Cerrar() devolvió error inesperado: %v", err)
	}
	if len(m.sesiones.LlamadasGuardar) != 1 {
		t.Fatalf("se esperaba una llamada a Guardar, hubo %d", len(m.sesiones.LlamadasGuardar))
	}
	if !m.sesiones.LlamadasGuardar[0].Estado().EsIgual(dominio.EstadoSesionRevocada) {
		t.Error("la sesión debía quedar revocada")
	}
	if nombres := m.auditoria.NombresEventos(); len(nombres) != 1 || nombres[0] != "SesionCerrada" {
		t.Errorf("eventos auditados = %v, esperado [SesionCerrada]", nombres)
	}
	if len(m.listaRevocacion.LlamadasRevocarSesion) != 1 {
		t.Error("se esperaba propagar la revocación a la lista")
	}
}

func TestCerrarSesionCasoDeUso_Cerrar_SesionAjena(t *testing.T) {
	usuarioID := idUsuarioDePrueba(t, idUsuarioValido1)
	sesion := sesionActivaDePrueba(t, idSesionDePrueba(t, idSesionValido1), usuarioID, 0,
		tokenRefrescoValor1, ahoraDePrueba().Add(-1*time.Hour), ahoraDePrueba().Add(23*time.Hour), ahoraDePrueba().Add(47*time.Hour))

	m := nuevosMocksCerrarSesion(t)
	m.sesiones.FnBuscarPorID = func(ctx context.Context, id dominio.IDSesion) (*dominio.Sesion, error) { return sesion, nil }
	caso := m.casoDeUso()

	err := caso.Cerrar(context.Background(), aplicacion.ComandoCerrarSesion{
		IDSesion: idSesionValido1, IDUsuario: idUsuarioValido2, Origen: origenDePrueba(t),
	})
	var errAjena *dominio.ErrSesionAjena
	if !errors.As(err, &errAjena) {
		t.Fatalf("se esperaba *ErrSesionAjena, obtuvo %T: %v", err, err)
	}
	if len(m.sesiones.LlamadasGuardar) != 0 {
		t.Error("no debe mutarse una sesión ajena")
	}
}

func TestCerrarSesionCasoDeUso_Cerrar_SesionNoEncontrada(t *testing.T) {
	m := nuevosMocksCerrarSesion(t)
	caso := m.casoDeUso()

	err := caso.Cerrar(context.Background(), aplicacion.ComandoCerrarSesion{
		IDSesion: idSesionValido1, IDUsuario: idUsuarioValido1, Origen: origenDePrueba(t),
	})
	var errNoEncontrada *dominio.ErrSesionNoEncontrada
	if !errors.As(err, &errNoEncontrada) {
		t.Fatalf("se esperaba *ErrSesionNoEncontrada, obtuvo %T: %v", err, err)
	}
}

func TestCerrarSesionCasoDeUso_Cerrar_EsIdempotente(t *testing.T) {
	usuarioID := idUsuarioDePrueba(t, idUsuarioValido1)
	sesion := sesionRevocadaDePrueba(t, idSesionDePrueba(t, idSesionValido1), usuarioID,
		tokenRefrescoValor1, dominio.MotivoCierreUsuario,
		ahoraDePrueba().Add(-2*time.Hour), ahoraDePrueba().Add(-1*time.Hour), ahoraDePrueba().Add(46*time.Hour))

	m := nuevosMocksCerrarSesion(t)
	m.sesiones.FnBuscarPorID = func(ctx context.Context, id dominio.IDSesion) (*dominio.Sesion, error) { return sesion, nil }
	caso := m.casoDeUso()

	err := caso.Cerrar(context.Background(), aplicacion.ComandoCerrarSesion{
		IDSesion: idSesionValido1, IDUsuario: idUsuarioValido1, Origen: origenDePrueba(t),
	})
	if err != nil {
		t.Fatalf("cerrar una sesión ya revocada debe ser idempotente (nil), obtuvo: %v", err)
	}
	if len(m.sesiones.LlamadasGuardar) != 0 {
		t.Error("una sesión ya revocada no debe volver a persistirse")
	}
	if len(m.auditoria.LlamadasRegistrar) != 0 {
		t.Error("un cierre idempotente no debe auditar un hecho nuevo")
	}
}

func TestCerrarSesionCasoDeUso_CerrarTodas_FlujoFeliz(t *testing.T) {
	m := nuevosMocksCerrarSesion(t)
	id1 := idSesionDePrueba(t, idSesionValido1)
	id2 := idSesionDePrueba(t, idSesionValido2)
	m.sesiones.FnRevocarActivasDeUsuario = func(ctx context.Context, u dominio.IDUsuario, excepto dominio.IDSesion,
		motivo dominio.MotivoRevocacion, ahora time.Time) ([]dominio.IDSesion, error) {
		return []dominio.IDSesion{id1, id2}, nil
	}
	caso := m.casoDeUso()

	resultado, err := caso.CerrarTodas(context.Background(), aplicacion.ComandoCerrarTodasLasSesiones{
		IDUsuario: idUsuarioValido1, Origen: origenDePrueba(t),
	})
	if err != nil {
		t.Fatalf("CerrarTodas() devolvió error inesperado: %v", err)
	}
	if resultado.SesionesRevocadas != 2 {
		t.Errorf("SesionesRevocadas = %d, esperado 2", resultado.SesionesRevocadas)
	}
	if nombres := m.auditoria.NombresEventos(); len(nombres) != 2 || nombres[0] != "SesionCerrada" || nombres[1] != "SesionCerrada" {
		t.Errorf("eventos auditados = %v, esperado [SesionCerrada SesionCerrada]", nombres)
	}
	if len(m.listaRevocacion.LlamadasRevocarSesion) != 2 {
		t.Errorf("se esperaban 2 propagaciones a la lista de revocación, hubo %d", len(m.listaRevocacion.LlamadasRevocarSesion))
	}
}

func TestCerrarSesionCasoDeUso_CerrarTodas_ConfianzaDeniega(t *testing.T) {
	m := nuevosMocksCerrarSesion(t)
	m.confianza.FnEvaluar = func(ctx context.Context, s puertos.SolicitudEvaluacion) (puertos.DecisionConfianza, error) {
		return puertos.DecisionConfianza{Permitido: false, Motivo: "riesgo_alto", ReintentarEn: time.Minute}, nil
	}
	caso := m.casoDeUso()

	_, err := caso.CerrarTodas(context.Background(), aplicacion.ComandoCerrarTodasLasSesiones{
		IDUsuario: idUsuarioValido1, Origen: origenDePrueba(t),
	})
	var errConfianza *dominio.ErrAccesoDenegadoPorConfianza
	if !errors.As(err, &errConfianza) {
		t.Fatalf("se esperaba *ErrAccesoDenegadoPorConfianza, obtuvo %T: %v", err, err)
	}
	if len(m.sesiones.LlamadasRevocarActivasDeUsuario) != 0 {
		t.Error("una denegación de Confianza no debe llegar a revocar sesiones")
	}
}
