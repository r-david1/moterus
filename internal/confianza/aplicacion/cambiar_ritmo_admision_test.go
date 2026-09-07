package aplicacion_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/r-david1/moterus/internal/confianza/aplicacion"
	"github.com/r-david1/moterus/internal/confianza/dominio"
	"github.com/r-david1/moterus/internal/confianza/puertos"
	"github.com/r-david1/moterus/internal/confianza/puertos/mocks"
)

type mocksCambiarRitmo struct {
	salas      *mocks.RepositorioSalasDeEspera
	estadoCola *mocks.EstadoDeCola
	auditoria  *mocks.RegistroAuditoria
	reloj      *mocks.Reloj
	uow        *mocks.UnidadDeTrabajo
}

func nuevosMocksCambiarRitmo(t *testing.T, sala *dominio.SalaDeEspera) *mocksCambiarRitmo {
	t.Helper()
	return &mocksCambiarRitmo{
		salas: &mocks.RepositorioSalasDeEspera{
			FnBuscarPorID: func(ctx context.Context, id dominio.IDSalaDeEspera) (*dominio.SalaDeEspera, error) {
				return sala, nil
			},
		},
		estadoCola: &mocks.EstadoDeCola{},
		auditoria:  &mocks.RegistroAuditoria{},
		reloj:      &mocks.Reloj{Fija: ahoraDePrueba()},
		uow:        &mocks.UnidadDeTrabajo{},
	}
}

func (m *mocksCambiarRitmo) casoDeUso() *aplicacion.CambiarRitmoDeAdmisionCasoDeUso {
	return aplicacion.NuevoCambiarRitmoDeAdmisionCasoDeUso(m.salas, m.estadoCola, m.auditoria, m.reloj, m.uow)
}

// salaAbiertaDePrueba construye, vía dominio.NuevaSalaDeEspera + Abrir, una
// SalaDeEspera de alcance sistema ya abierta, sin eventos pendientes (los
// drena antes de devolverla).
func salaAbiertaDePrueba(t *testing.T, id string, ritmo int, ahora time.Time) *dominio.SalaDeEspera {
	t.Helper()
	idSala, err := dominio.IDSalaDeEsperaDesde(id)
	if err != nil {
		t.Fatalf("id de sala inválido: %v", err)
	}
	r, err := dominio.NuevoRitmoAdmision(ritmo)
	if err != nil {
		t.Fatalf("ritmo inválido: %v", err)
	}
	politica, err := dominio.NuevaPoliticaSala(r, 500_000, 2*time.Minute, dominio.ModoDegradadoPermitir)
	if err != nil {
		t.Fatalf("política inválida: %v", err)
	}
	sala, err := dominio.NuevaSalaDeEspera(idSala, aliasSalaDePrueba(t, "evento-"+id), dominio.AlcanceSistema(), dominio.RutaAccesoIniciarSesion, politica, nil, ahora)
	if err != nil {
		t.Fatalf("no se pudo construir la sala de prueba: %v", err)
	}
	if err := sala.Abrir(ahora); err != nil {
		t.Fatalf("no se pudo abrir la sala de prueba: %v", err)
	}
	sala.EventosPendientes() // drenar
	return sala
}

func aliasSalaDePrueba(t *testing.T, valor string) dominio.AliasSala {
	t.Helper()
	a, err := dominio.NuevoAliasSala(valor)
	if err != nil {
		t.Fatalf("alias de sala inválido %q: %v", valor, err)
	}
	return a
}

func TestCambiarRitmoDeAdmisionCasoDeUso_FlujoFeliz(t *testing.T) {
	ahora := ahoraDePrueba()
	sala := salaAbiertaDePrueba(t, idSalaValido1, 50, ahora)
	m := nuevosMocksCambiarRitmo(t, sala)
	m.estadoCola.FnInstantanea = func(ctx context.Context, clave string) (puertos.InstantaneaCola, error) {
		return puertos.InstantaneaCola{LongitudAproximada: 4321, Cursor: 10, Ingresos: 100}, nil
	}
	m.reloj.Fija = ahora.Add(5 * time.Second)
	caso := m.casoDeUso()

	vista, err := caso.CambiarRitmo(context.Background(), puertos.ComandoCambiarRitmoAdmision{
		IDSala:        idSalaValido1,
		RitmoAdmision: 120,
		Origen:        origenDePrueba(t),
	})
	if err != nil {
		t.Fatalf("CambiarRitmo() devolvió error inesperado: %v", err)
	}
	if vista.RitmoAdmision != 120 {
		t.Errorf("RitmoAdmision = %d, esperado 120", vista.RitmoAdmision)
	}

	if len(m.estadoCola.LlamadasProyectar) != 1 {
		t.Fatalf("se esperaba 1 llamada a Proyectar, hubo %d", len(m.estadoCola.LlamadasProyectar))
	}
	if m.estadoCola.LlamadasProyectar[0].RitmoAdmision != 120 {
		t.Errorf("ProyeccionSala.RitmoAdmision = %d, esperado 120", m.estadoCola.LlamadasProyectar[0].RitmoAdmision)
	}

	nombres := m.auditoria.NombresEventos()
	if len(nombres) != 1 || nombres[0] != "RitmoDeAdmisionCambiado" {
		t.Fatalf("eventos auditados = %v, esperado [RitmoDeAdmisionCambiado]", nombres)
	}
	evento, ok := m.auditoria.LlamadasRegistrar[0].Evento.(dominio.RitmoDeAdmisionCambiado)
	if !ok {
		t.Fatalf("el evento auditado no es RitmoDeAdmisionCambiado: %T", m.auditoria.LlamadasRegistrar[0].Evento)
	}
	if evento.LongitudCola != 4321 {
		t.Errorf("LongitudCola del evento = %d, esperado 4321 (viene de EstadoDeCola.Instantanea, INV-COLA-08)", evento.LongitudCola)
	}
	if evento.RitmoAnterior != 50 || evento.RitmoNuevo != 120 {
		t.Errorf("RitmoAnterior/RitmoNuevo = %d/%d, esperado 50/120", evento.RitmoAnterior, evento.RitmoNuevo)
	}

	if len(m.salas.LlamadasGuardar) != 1 {
		t.Fatalf("se esperaba 1 llamada a Guardar, hubo %d", len(m.salas.LlamadasGuardar))
	}
}

func TestCambiarRitmoDeAdmisionCasoDeUso_SalaNoEncontrada(t *testing.T) {
	m := &mocksCambiarRitmo{
		salas:      &mocks.RepositorioSalasDeEspera{}, // FnBuscarPorID sin configurar -> nil, nil
		estadoCola: &mocks.EstadoDeCola{},
		auditoria:  &mocks.RegistroAuditoria{},
		reloj:      &mocks.Reloj{Fija: ahoraDePrueba()},
		uow:        &mocks.UnidadDeTrabajo{},
	}
	caso := m.casoDeUso()

	_, err := caso.CambiarRitmo(context.Background(), puertos.ComandoCambiarRitmoAdmision{
		IDSala:        idSalaValido1,
		RitmoAdmision: 100,
		Origen:        origenDePrueba(t),
	})
	var errNoEncontrada *dominio.ErrSalaNoEncontrada
	if !errors.As(err, &errNoEncontrada) {
		t.Fatalf("se esperaba *ErrSalaNoEncontrada, obtuvo %T: %v", err, err)
	}
}

func TestCambiarRitmoDeAdmisionCasoDeUso_ProyeccionFallidaNoAuditaNiPersiste(t *testing.T) {
	ahora := ahoraDePrueba()
	sala := salaAbiertaDePrueba(t, idSalaValido1, 50, ahora)
	m := nuevosMocksCambiarRitmo(t, sala)
	falloProyeccion := errors.New("redis no disponible")
	m.estadoCola.FnProyectar = func(ctx context.Context, p puertos.ProyeccionSala) error {
		return falloProyeccion
	}
	caso := m.casoDeUso()

	_, err := caso.CambiarRitmo(context.Background(), puertos.ComandoCambiarRitmoAdmision{
		IDSala:        idSalaValido1,
		RitmoAdmision: 200,
		Origen:        origenDePrueba(t),
	})
	if !errors.Is(err, falloProyeccion) {
		t.Fatalf("se esperaba el error de proyección, obtuvo %v", err)
	}
	if len(m.salas.LlamadasGuardar) != 0 {
		t.Error("no debía persistirse el cambio de ritmo si la proyección a Redis falló")
	}
	if len(m.auditoria.LlamadasRegistrar) != 0 {
		t.Error("no debía auditarse nada si la proyección a Redis falló")
	}
}

func TestCambiarRitmoDeAdmisionCasoDeUso_RitmoIgualEsNoOpSinAuditar(t *testing.T) {
	ahora := ahoraDePrueba()
	sala := salaAbiertaDePrueba(t, idSalaValido1, 50, ahora)
	m := nuevosMocksCambiarRitmo(t, sala)
	caso := m.casoDeUso()

	_, err := caso.CambiarRitmo(context.Background(), puertos.ComandoCambiarRitmoAdmision{
		IDSala:        idSalaValido1,
		RitmoAdmision: 50,
		Origen:        origenDePrueba(t),
	})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if len(m.auditoria.LlamadasRegistrar) != 0 {
		t.Error("un ritmo igual al vigente es un no-op de dominio: no debía auditarse nada")
	}
}

func TestCambiarRitmoDeAdmisionCasoDeUso_RitmoInvalido(t *testing.T) {
	sala := salaAbiertaDePrueba(t, idSalaValido1, 50, ahoraDePrueba())
	m := nuevosMocksCambiarRitmo(t, sala)
	caso := m.casoDeUso()

	_, err := caso.CambiarRitmo(context.Background(), puertos.ComandoCambiarRitmoAdmision{
		IDSala:        idSalaValido1,
		RitmoAdmision: 0,
		Origen:        origenDePrueba(t),
	})
	var errRitmo *dominio.ErrRitmoAdmisionInvalido
	if !errors.As(err, &errRitmo) {
		t.Fatalf("se esperaba *ErrRitmoAdmisionInvalido, obtuvo %T: %v", err, err)
	}
	if len(m.salas.LlamadasBuscarPorID) != 0 {
		t.Error("no debía consultarse la sala si el ritmo pedido ya era inválido")
	}
}
