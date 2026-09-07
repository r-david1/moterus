package aplicacion_test

import (
	"context"
	"errors"
	"testing"

	"github.com/r-david1/moterus/internal/confianza/aplicacion"
	"github.com/r-david1/moterus/internal/confianza/dominio"
	"github.com/r-david1/moterus/internal/confianza/puertos"
	"github.com/r-david1/moterus/internal/confianza/puertos/mocks"
)

type mocksCambiarEstadoSala struct {
	salas      *mocks.RepositorioSalasDeEspera
	estadoCola *mocks.EstadoDeCola
	auditoria  *mocks.RegistroAuditoria
	reloj      *mocks.Reloj
	uow        *mocks.UnidadDeTrabajo
}

func nuevosMocksCambiarEstadoSala(t *testing.T, sala *dominio.SalaDeEspera) *mocksCambiarEstadoSala {
	t.Helper()
	return &mocksCambiarEstadoSala{
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

func (m *mocksCambiarEstadoSala) casoDeUso() *aplicacion.CambiarEstadoSalaCasoDeUso {
	return aplicacion.NuevoCambiarEstadoSalaCasoDeUso(m.salas, m.estadoCola, m.auditoria, m.reloj, m.uow)
}

func TestCambiarEstadoSalaCasoDeUso_Drenar(t *testing.T) {
	ahora := ahoraDePrueba()
	sala := salaAbiertaDePrueba(t, idSalaValido1, 50, ahora)
	m := nuevosMocksCambiarEstadoSala(t, sala)
	m.estadoCola.FnInstantanea = func(ctx context.Context, clave string) (puertos.InstantaneaCola, error) {
		return puertos.InstantaneaCola{LongitudAproximada: 10, Cursor: 900, Ingresos: 1000}, nil
	}
	caso := m.casoDeUso()

	vista, err := caso.CambiarEstado(context.Background(), puertos.ComandoCambiarEstadoSala{
		IDSala:  idSalaValido1,
		Destino: dominio.EstadoSalaDrenando.String(),
		Origen:  origenDePrueba(t),
	})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if vista.Estado != dominio.EstadoSalaDrenando.String() {
		t.Errorf("Estado = %q, esperado drenando", vista.Estado)
	}
	if len(m.estadoCola.LlamadasProyectar) != 1 {
		t.Fatalf("drenar debía reproyectar (la sala sigue viva en Redis), hubo %d Proyectar", len(m.estadoCola.LlamadasProyectar))
	}
	if len(m.estadoCola.LlamadasRetirar) != 0 {
		t.Error("drenar no debía retirar la proyección de Redis")
	}
	evento, ok := m.auditoria.LlamadasRegistrar[0].Evento.(dominio.SalaDeEsperaCerrada)
	if !ok {
		t.Fatalf("el evento auditado no es SalaDeEsperaCerrada: %T", m.auditoria.LlamadasRegistrar[0].Evento)
	}
	if evento.Destino != dominio.EstadoSalaDrenando.String() {
		t.Errorf("Destino del evento = %q, esperado drenando", evento.Destino)
	}
	if evento.IngresosTotales != 1000 || evento.AdmitidosTotales != 900 {
		t.Errorf("IngresosTotales/AdmitidosTotales = %d/%d, esperado 1000/900 (de EstadoDeCola.Instantanea)", evento.IngresosTotales, evento.AdmitidosTotales)
	}
}

func TestCambiarEstadoSalaCasoDeUso_Cerrar(t *testing.T) {
	ahora := ahoraDePrueba()
	sala := salaAbiertaDePrueba(t, idSalaValido1, 50, ahora)
	m := nuevosMocksCambiarEstadoSala(t, sala)
	m.estadoCola.FnInstantanea = func(ctx context.Context, clave string) (puertos.InstantaneaCola, error) {
		return puertos.InstantaneaCola{LongitudAproximada: 0, Cursor: 5000, Ingresos: 5000}, nil
	}
	caso := m.casoDeUso()

	vista, err := caso.CambiarEstado(context.Background(), puertos.ComandoCambiarEstadoSala{
		IDSala:  idSalaValido1,
		Destino: dominio.EstadoSalaCerrada.String(),
		Origen:  origenDePrueba(t),
	})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if vista.Estado != dominio.EstadoSalaCerrada.String() {
		t.Errorf("Estado = %q, esperado cerrada", vista.Estado)
	}
	if len(m.estadoCola.LlamadasRetirar) != 1 {
		t.Fatalf("cerrar debía retirar la proyección de Redis, hubo %d Retirar", len(m.estadoCola.LlamadasRetirar))
	}
	if len(m.estadoCola.LlamadasProyectar) != 0 {
		t.Error("cerrar no debía reproyectar (se retira, no se actualiza)")
	}
	nombres := m.auditoria.NombresEventos()
	if len(nombres) != 1 || nombres[0] != "SalaDeEsperaCerrada" {
		t.Fatalf("eventos auditados = %v, esperado [SalaDeEsperaCerrada]", nombres)
	}
}

func TestCambiarEstadoSalaCasoDeUso_Reabrir(t *testing.T) {
	ahora := ahoraDePrueba()
	sala := salaAbiertaDePrueba(t, idSalaValido1, 50, ahora)
	if err := sala.Drenar(10, 5, ahora); err != nil {
		t.Fatalf("no se pudo drenar la sala de prueba: %v", err)
	}
	sala.EventosPendientes() // drenar buffer de eventos previos al caso de uso bajo prueba
	m := nuevosMocksCambiarEstadoSala(t, sala)
	caso := m.casoDeUso()

	vista, err := caso.CambiarEstado(context.Background(), puertos.ComandoCambiarEstadoSala{
		IDSala:  idSalaValido1,
		Destino: dominio.EstadoSalaAbierta.String(),
		Origen:  origenDePrueba(t),
	})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if vista.Estado != dominio.EstadoSalaAbierta.String() {
		t.Errorf("Estado = %q, esperado abierta", vista.Estado)
	}
	if len(m.estadoCola.LlamadasInstantanea) != 0 {
		t.Error("reabrir no necesita EstadoDeCola.Instantanea: dominio.Abrir no toma longitud ni totales")
	}
	if len(m.estadoCola.LlamadasProyectar) != 1 {
		t.Errorf("reabrir debía reproyectar, hubo %d Proyectar", len(m.estadoCola.LlamadasProyectar))
	}
	nombres := m.auditoria.NombresEventos()
	if len(nombres) != 1 || nombres[0] != "SalaDeEsperaAbierta" {
		t.Fatalf("eventos auditados = %v, esperado [SalaDeEsperaAbierta]", nombres)
	}
}

func TestCambiarEstadoSalaCasoDeUso_DestinoInvalido(t *testing.T) {
	sala := salaAbiertaDePrueba(t, idSalaValido1, 50, ahoraDePrueba())
	m := nuevosMocksCambiarEstadoSala(t, sala)
	caso := m.casoDeUso()

	_, err := caso.CambiarEstado(context.Background(), puertos.ComandoCambiarEstadoSala{
		IDSala:  idSalaValido1,
		Destino: "no-es-un-estado",
		Origen:  origenDePrueba(t),
	})
	var errEstado *dominio.ErrEstadoSalaInvalido
	if !errors.As(err, &errEstado) {
		t.Fatalf("se esperaba *ErrEstadoSalaInvalido, obtuvo %T: %v", err, err)
	}
}

func TestCambiarEstadoSalaCasoDeUso_TransicionInvalida(t *testing.T) {
	// "programada" es un valor válido del catálogo EstadoSala, pero nunca es
	// un destino administrable por este comando: debe caer en el default
	// del switch (ErrTransicionEstadoSalaInvalida), no confundirse con un
	// ErrEstadoSalaInvalido.
	sala := salaAbiertaDePrueba(t, idSalaValido1, 50, ahoraDePrueba())
	m := nuevosMocksCambiarEstadoSala(t, sala)
	caso := m.casoDeUso()

	_, err := caso.CambiarEstado(context.Background(), puertos.ComandoCambiarEstadoSala{
		IDSala:  idSalaValido1,
		Destino: dominio.EstadoSalaProgramada.String(),
		Origen:  origenDePrueba(t),
	})
	var errTransicion *dominio.ErrTransicionEstadoSalaInvalida
	if !errors.As(err, &errTransicion) {
		t.Fatalf("se esperaba *ErrTransicionEstadoSalaInvalida, obtuvo %T: %v", err, err)
	}
}

func TestCambiarEstadoSalaCasoDeUso_CerrarDesdeCerradaEsInvalido(t *testing.T) {
	sala := salaAbiertaDePrueba(t, idSalaValido1, 50, ahoraDePrueba())
	if err := sala.Cerrar(0, 0, ahoraDePrueba()); err != nil {
		t.Fatalf("no se pudo cerrar la sala de prueba: %v", err)
	}
	sala.EventosPendientes()
	m := nuevosMocksCambiarEstadoSala(t, sala)
	caso := m.casoDeUso()

	_, err := caso.CambiarEstado(context.Background(), puertos.ComandoCambiarEstadoSala{
		IDSala:  idSalaValido1,
		Destino: dominio.EstadoSalaCerrada.String(),
		Origen:  origenDePrueba(t),
	})
	var errTransicion *dominio.ErrTransicionEstadoSalaInvalida
	if !errors.As(err, &errTransicion) {
		t.Fatalf("se esperaba *ErrTransicionEstadoSalaInvalida (cerrada es terminal), obtuvo %T: %v", err, err)
	}
	if len(m.estadoCola.LlamadasRetirar) != 0 {
		t.Error("no debía retirarse nada si la transición es inválida")
	}
}

func TestCambiarEstadoSalaCasoDeUso_SalaNoEncontrada(t *testing.T) {
	m := &mocksCambiarEstadoSala{
		salas:      &mocks.RepositorioSalasDeEspera{},
		estadoCola: &mocks.EstadoDeCola{},
		auditoria:  &mocks.RegistroAuditoria{},
		reloj:      &mocks.Reloj{Fija: ahoraDePrueba()},
		uow:        &mocks.UnidadDeTrabajo{},
	}
	caso := m.casoDeUso()

	_, err := caso.CambiarEstado(context.Background(), puertos.ComandoCambiarEstadoSala{
		IDSala:  idSalaValido1,
		Destino: dominio.EstadoSalaCerrada.String(),
		Origen:  origenDePrueba(t),
	})
	var errNoEncontrada *dominio.ErrSalaNoEncontrada
	if !errors.As(err, &errNoEncontrada) {
		t.Fatalf("se esperaba *ErrSalaNoEncontrada, obtuvo %T: %v", err, err)
	}
}

func TestCambiarEstadoSalaCasoDeUso_RetirarFallidoNoAuditaNiPersiste(t *testing.T) {
	sala := salaAbiertaDePrueba(t, idSalaValido1, 50, ahoraDePrueba())
	m := nuevosMocksCambiarEstadoSala(t, sala)
	m.estadoCola.FnInstantanea = func(ctx context.Context, clave string) (puertos.InstantaneaCola, error) {
		return puertos.InstantaneaCola{}, nil
	}
	falloRetirar := errors.New("redis no disponible")
	m.estadoCola.FnRetirar = func(ctx context.Context, clave string) error { return falloRetirar }
	caso := m.casoDeUso()

	_, err := caso.CambiarEstado(context.Background(), puertos.ComandoCambiarEstadoSala{
		IDSala:  idSalaValido1,
		Destino: dominio.EstadoSalaCerrada.String(),
		Origen:  origenDePrueba(t),
	})
	if !errors.Is(err, falloRetirar) {
		t.Fatalf("se esperaba el error de Retirar, obtuvo %v", err)
	}
	if len(m.salas.LlamadasGuardar) != 0 {
		t.Error("no debía persistirse el cierre si Retirar falló")
	}
	if len(m.auditoria.LlamadasRegistrar) != 0 {
		t.Error("no debía auditarse nada si Retirar falló")
	}
}
