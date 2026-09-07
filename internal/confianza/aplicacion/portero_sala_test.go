package aplicacion_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/r-david1/moterus/internal/confianza/aplicacion"
	"github.com/r-david1/moterus/internal/confianza/dominio"
	"github.com/r-david1/moterus/internal/confianza/puertos"
	"github.com/r-david1/moterus/internal/confianza/puertos/mocks"
)

// instantaneaConSala construye una InstantaneaSalasVigentes ya poblada con
// una sola sala, reutilizando ReconciliarSalasCasoDeUso.Reconciliar para no
// duplicar la lógica de reemplazo (que es privada al paquete aplicacion).
func instantaneaConSala(t *testing.T, sala *dominio.SalaDeEspera) *aplicacion.InstantaneaSalasVigentes {
	t.Helper()
	instantanea := aplicacion.NuevaInstantaneaSalasVigentes()
	repo := &mocks.RepositorioSalasDeEspera{
		FnListarVigentes: func(ctx context.Context) ([]*dominio.SalaDeEspera, error) {
			return []*dominio.SalaDeEspera{sala}, nil
		},
	}
	estadoCola := &mocks.EstadoDeCola{}
	reconciliador := aplicacion.NuevoReconciliarSalasCasoDeUso(repo, estadoCola, &mocks.Reloj{Fija: ahoraDePrueba()}, instantanea)
	if err := reconciliador.Reconciliar(context.Background()); err != nil {
		t.Fatalf("no se pudo poblar la instantánea de prueba: %v", err)
	}
	return instantanea
}

func nuevoPorteroDeSala(instantanea *aplicacion.InstantaneaSalasVigentes, estadoCola *mocks.EstadoDeCola, tickets *mocks.GeneradorTickets, reloj *mocks.Reloj) *aplicacion.PorteroDeSalaCasoDeUso {
	return aplicacion.NuevoPorteroDeSalaCasoDeUso(instantanea, estadoCola, tickets, reloj)
}

// --- INV-COLA-08: el camino caliente nunca toca Postgres --------------------

// TestPorteroDeSalaCasoDeUso_NuncaDependeDeRepositorioSalasDeEspera verifica
// INV-COLA-08 de forma estructural: PorteroDeSalaCasoDeUso ni siquiera
// recibe un puertos.RepositorioSalasDeEspera en su constructor (a diferencia
// de los tres casos de uso de administración), así que Ingresar/
// ConsultarTurno/Reclamar no tienen forma de invocarlo, ni por accidente. La
// firma del constructor es la prueba: si algún día alguien le agrega esa
// dependencia, esta prueba de tipos deja de compilar y el problema se
// detecta en build, no en runtime.
func TestPorteroDeSalaCasoDeUso_NuncaDependeDeRepositorioSalasDeEspera(t *testing.T) {
	var _ func(
		*aplicacion.InstantaneaSalasVigentes,
		puertos.EstadoDeCola,
		puertos.GeneradorTickets,
		puertos.Reloj,
	) *aplicacion.PorteroDeSalaCasoDeUso = aplicacion.NuevoPorteroDeSalaCasoDeUso
}

func TestPorteroDeSalaCasoDeUso_SalaVigentePara(t *testing.T) {
	ahora := ahoraDePrueba()
	sala := salaAbiertaDePrueba(t, idSalaValido1, 50, ahora)
	instantanea := instantaneaConSala(t, sala)
	portero := nuevoPorteroDeSala(instantanea, &mocks.EstadoDeCola{}, &mocks.GeneradorTickets{}, &mocks.Reloj{Fija: ahora})

	vista, ok := portero.SalaVigentePara(context.Background(), puertos.ConsultaSalaVigente{
		Ruta: dominio.RutaAccesoIniciarSesion.String(),
	})
	if !ok {
		t.Fatal("se esperaba que hubiera sala vigente para acceso.iniciar_sesion")
	}
	if vista.Estado != dominio.EstadoSalaAbierta.String() {
		t.Errorf("Estado = %q, esperado abierta", vista.Estado)
	}

	_, ok = portero.SalaVigentePara(context.Background(), puertos.ConsultaSalaVigente{
		Ruta: dominio.RutaIdentidadRegistrarUsuario.String(),
	})
	if ok {
		t.Error("no debía haber sala vigente para una ruta sin sala abierta")
	}
}

func TestPorteroDeSalaCasoDeUso_Ingresar_FlujoFeliz(t *testing.T) {
	ahora := ahoraDePrueba()
	sala := salaAbiertaDePrueba(t, idSalaValido1, 50, ahora)
	instantanea := instantaneaConSala(t, sala)
	estadoCola := &mocks.EstadoDeCola{
		FnIngresar: func(ctx context.Context, clave, hash string) (puertos.EstadoTicket, error) {
			return puertos.EstadoTicket{Desenlace: "esperando", Rango: 120, Cursor: 20, LongitudCola: 120, TurnoEstimadoEn: ahora.Add(2 * time.Second)}, nil
		},
	}
	tickets := &mocks.GeneradorTickets{FnGenerarTicket: func() (dominio.TicketPlano, error) {
		return ticketPlanoDePrueba(t, ticketPlanoValido1), nil
	}}
	portero := nuevoPorteroDeSala(instantanea, estadoCola, tickets, &mocks.Reloj{Fija: ahora})

	resultado, err := portero.Ingresar(context.Background(), puertos.ComandoIngresarASala{
		Alias: "evento-" + idSalaValido1,
	})
	if err != nil {
		t.Fatalf("Ingresar() devolvió error inesperado: %v", err)
	}
	if resultado.TicketPlano != ticketPlanoValido1 {
		t.Errorf("TicketPlano = %q, esperado %q (única vez que sale del proceso, INV-COLA-07)", resultado.TicketPlano, ticketPlanoValido1)
	}
	if resultado.Desenlace != dominio.DesenlaceEsperando.String() {
		t.Errorf("Desenlace = %q, esperado esperando", resultado.Desenlace)
	}
	if resultado.Posicion != 100 {
		t.Errorf("Posicion = %d, esperado 100 (rango 120 - cursor 20)", resultado.Posicion)
	}
	if len(estadoCola.LlamadasIngresar) != 1 || estadoCola.LlamadasIngresar[0].Clave != sala.Clave().String() {
		t.Errorf("EstadoDeCola.Ingresar debía invocarse con la clave de la sala vigente, llamadas = %v", estadoCola.LlamadasIngresar)
	}
}

func TestPorteroDeSalaCasoDeUso_Ingresar_SalaNoEncontrada(t *testing.T) {
	instantanea := aplicacion.NuevaInstantaneaSalasVigentes()
	portero := nuevoPorteroDeSala(instantanea, &mocks.EstadoDeCola{}, &mocks.GeneradorTickets{}, &mocks.Reloj{Fija: ahoraDePrueba()})

	_, err := portero.Ingresar(context.Background(), puertos.ComandoIngresarASala{Alias: "no-existe"})
	var errNoEncontrada *dominio.ErrSalaNoEncontrada
	if !errors.As(err, &errNoEncontrada) {
		t.Fatalf("se esperaba *ErrSalaNoEncontrada, obtuvo %T: %v", err, err)
	}
}

func TestPorteroDeSalaCasoDeUso_Ingresar_ColaLlena(t *testing.T) {
	ahora := ahoraDePrueba()
	sala := salaAbiertaDePrueba(t, idSalaValido1, 50, ahora)
	instantanea := instantaneaConSala(t, sala)
	estadoCola := &mocks.EstadoDeCola{
		FnIngresar: func(ctx context.Context, clave, hash string) (puertos.EstadoTicket, error) {
			return puertos.EstadoTicket{Desenlace: "cola_llena"}, nil
		},
	}
	portero := nuevoPorteroDeSala(instantanea, estadoCola, &mocks.GeneradorTickets{}, &mocks.Reloj{Fija: ahora})

	_, err := portero.Ingresar(context.Background(), puertos.ComandoIngresarASala{Alias: "evento-" + idSalaValido1})
	var errColaLlena *dominio.ErrColaLlena
	if !errors.As(err, &errColaLlena) {
		t.Fatalf("se esperaba *ErrColaLlena, obtuvo %T: %v", err, err)
	}
}

// --- ConsultarTurno ----------------------------------------------------------

func TestPorteroDeSalaCasoDeUso_ConsultarTurno_NuncaEsErrorDeGoPorDesenlaceNegativo(t *testing.T) {
	ahora := ahoraDePrueba()
	sala := salaAbiertaDePrueba(t, idSalaValido1, 50, ahora)
	instantanea := instantaneaConSala(t, sala)

	casos := []string{"esperando", "admitido", "turno_caducado", "ticket_desconocido", "ticket_consumido", "sala_cerrada"}
	for _, desenlace := range casos {
		desenlace := desenlace
		t.Run(desenlace, func(t *testing.T) {
			estadoCola := &mocks.EstadoDeCola{
				FnConsultar: func(ctx context.Context, clave, hash string) (puertos.EstadoTicket, error) {
					return puertos.EstadoTicket{Desenlace: desenlace, Rango: 5, Cursor: 5}, nil
				},
			}
			portero := nuevoPorteroDeSala(instantanea, estadoCola, &mocks.GeneradorTickets{}, &mocks.Reloj{Fija: ahora})

			resultado, err := portero.ConsultarTurno(context.Background(), puertos.ConsultaTurno{
				Alias:       "evento-" + idSalaValido1,
				TicketPlano: ticketPlanoValido1,
			})
			if err != nil {
				t.Fatalf("ConsultarTurno() no debía devolver error de Go para el desenlace %q (§7.1: 200 siempre), obtuvo: %v", desenlace, err)
			}
			if resultado.Desenlace != desenlace {
				t.Errorf("Desenlace = %q, esperado %q", resultado.Desenlace, desenlace)
			}
			if resultado.TicketPlano != "" {
				t.Error("ConsultarTurno nunca debe revelar el ticket en claro (INV-COLA-07)")
			}
		})
	}
}

func TestPorteroDeSalaCasoDeUso_ConsultarTurno_TicketMalformado(t *testing.T) {
	sala := salaAbiertaDePrueba(t, idSalaValido1, 50, ahoraDePrueba())
	instantanea := instantaneaConSala(t, sala)
	portero := nuevoPorteroDeSala(instantanea, &mocks.EstadoDeCola{}, &mocks.GeneradorTickets{}, &mocks.Reloj{Fija: ahoraDePrueba()})

	_, err := portero.ConsultarTurno(context.Background(), puertos.ConsultaTurno{
		Alias:       "evento-" + idSalaValido1,
		TicketPlano: "no-tiene-el-prefijo-correcto",
	})
	var errTicket *dominio.ErrTicketPlanoInvalido
	if !errors.As(err, &errTicket) {
		t.Fatalf("se esperaba *ErrTicketPlanoInvalido, obtuvo %T: %v", err, err)
	}
}

// --- Reclamar ----------------------------------------------------------------

func TestPorteroDeSalaCasoDeUso_Reclamar_Admitido(t *testing.T) {
	ahora := ahoraDePrueba()
	estadoCola := &mocks.EstadoDeCola{
		FnReclamar: func(ctx context.Context, clave, hash string) (puertos.EstadoTicket, error) {
			return puertos.EstadoTicket{Desenlace: "admitido", Rango: 10, Cursor: 10}, nil
		},
	}
	portero := nuevoPorteroDeSala(aplicacion.NuevaInstantaneaSalasVigentes(), estadoCola, &mocks.GeneradorTickets{}, &mocks.Reloj{Fija: ahora})

	resultado, err := portero.Reclamar(context.Background(), puertos.ComandoReclamarTurno{
		Clave:       "sistema:" + dominio.RutaAccesoIniciarSesion.String(),
		TicketPlano: ticketPlanoValido1,
	})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if resultado.Desenlace != dominio.DesenlaceAdmitido.String() {
		t.Errorf("Desenlace = %q, esperado admitido", resultado.Desenlace)
	}
}

func TestPorteroDeSalaCasoDeUso_Reclamar_DesenlacesNoAdmitidosSonError(t *testing.T) {
	casos := map[string]error{
		"esperando":          &dominio.ErrTurnoNoAlcanzado{},
		"turno_caducado":     &dominio.ErrTurnoCaducado{},
		"ticket_desconocido": &dominio.ErrTicketDesconocido{},
		"ticket_consumido":   &dominio.ErrTicketConsumido{},
		"sala_cerrada":       &dominio.ErrSalaNoAbierta{},
	}
	for desenlace, errEsperado := range casos {
		desenlace, errEsperado := desenlace, errEsperado
		t.Run(desenlace, func(t *testing.T) {
			estadoCola := &mocks.EstadoDeCola{
				FnReclamar: func(ctx context.Context, clave, hash string) (puertos.EstadoTicket, error) {
					return puertos.EstadoTicket{Desenlace: desenlace, Rango: 50, Cursor: 10}, nil
				},
			}
			portero := nuevoPorteroDeSala(aplicacion.NuevaInstantaneaSalasVigentes(), estadoCola, &mocks.GeneradorTickets{}, &mocks.Reloj{Fija: ahoraDePrueba()})

			resultado, err := portero.Reclamar(context.Background(), puertos.ComandoReclamarTurno{
				Clave:       "sistema:" + dominio.RutaAccesoIniciarSesion.String(),
				TicketPlano: ticketPlanoValido1,
			})
			if err == nil {
				t.Fatalf("se esperaba un error para el desenlace %q", desenlace)
			}
			if got, want := fmt.Sprintf("%T", err), fmt.Sprintf("%T", errEsperado); got != want {
				t.Fatalf("tipo de error = %s, esperado %s", got, want)
			}
			// El resultado con datos (posición, etc.) igual se devuelve: el
			// middleware lo necesita para construir el 503 de §7.4.
			if resultado.Desenlace != desenlace {
				t.Errorf("Desenlace en el resultado = %q, esperado %q incluso en el camino de error", resultado.Desenlace, desenlace)
			}
		})
	}
}
