package http

import (
	"context"
	"testing"
	"time"

	accesopuertos "github.com/r-david1/moterus/internal/acceso/puertos"
	"github.com/r-david1/moterus/internal/confianza/dominio"
	"github.com/r-david1/moterus/internal/confianza/puertos"
	"github.com/r-david1/moterus/internal/confianza/puertos/mocks"
)

// accesoDePrueba construye un acceso/puertos.Acceso mínimo para publicarlo
// en el contexto bajo claveAcceso{}, simulando lo que
// middlewareAutenticacion ya validó (§7.2 del diseño).
func accesoDePrueba(idUsuario string) accesopuertos.Acceso {
	return accesopuertos.Acceso{IDUsuario: idUsuario}
}

func TestManejadorConfianza_IngresarASala_IncluyeElTicketEnLaRespuesta(t *testing.T) {
	portero := &mocks.PorteroDeSala{
		FnIngresar: func(_ context.Context, cmd puertos.ComandoIngresarASala) (puertos.ResultadoTurno, error) {
			if cmd.Alias != "inscripciones-2026" {
				t.Fatalf("Alias = %q, esperado \"inscripciones-2026\"", cmd.Alias)
			}
			return puertos.ResultadoTurno{
				TicketPlano:     "mot_cola_secreto",
				Desenlace:       "esperando",
				Posicion:        12843,
				LongitudCola:    41902,
				EsperaEstimada:  257 * time.Second,
				TurnoEstimadoEn: time.Date(2026, 9, 7, 12, 4, 17, 0, time.UTC),
				ReconsultarEn:   15 * time.Second,
			}, nil
		},
	}
	m := NuevoManejadorConfianza(portero, &mocks.GestorDeSalasDeEspera{}, &mocks.ConsultorDeSalas{})

	out, err := m.IngresarASala(context.Background(), &IngresarASalaInput{AliasSala: "inscripciones-2026"})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if out.Body.Ticket != "mot_cola_secreto" {
		t.Fatalf("Ticket = %q, se esperaba que IngresarASala SÍ lo incluya (única vez que sale del proceso)", out.Body.Ticket)
	}
	if out.Body.Desenlace != "esperando" || out.Body.Posicion != 12843 || out.Body.LongitudCola != 41902 {
		t.Fatalf("proyección incorrecta: %+v", out.Body)
	}
	if out.Body.EsperaEstimadaSegundos != 257 {
		t.Fatalf("espera_estimada_segundos = %d, esperado 257", out.Body.EsperaEstimadaSegundos)
	}
	if out.Body.ReconsultarEnMs != 15000 {
		t.Fatalf("reconsultar_en_ms = %d, esperado 15000", out.Body.ReconsultarEnMs)
	}
}

func TestManejadorConfianza_IngresarASala_MapeaErrorDeDominio(t *testing.T) {
	portero := &mocks.PorteroDeSala{
		FnIngresar: func(_ context.Context, _ puertos.ComandoIngresarASala) (puertos.ResultadoTurno, error) {
			return puertos.ResultadoTurno{}, &dominio.ErrSalaNoEncontrada{Referencia: "no-existe"}
		},
	}
	m := NuevoManejadorConfianza(portero, &mocks.GestorDeSalasDeEspera{}, &mocks.ConsultorDeSalas{})

	_, err := m.IngresarASala(context.Background(), &IngresarASalaInput{AliasSala: "no-existe"})
	if err == nil {
		t.Fatal("se esperaba un error")
	}
}

func TestManejadorConfianza_ConsultarTurno_NuncaIncluyeElTicket(t *testing.T) {
	portero := &mocks.PorteroDeSala{
		FnConsultarTurno: func(_ context.Context, q puertos.ConsultaTurno) (puertos.ResultadoTurno, error) {
			if q.TicketPlano != "mot_cola_secreto" {
				t.Fatalf("TicketPlano = %q, esperado \"mot_cola_secreto\"", q.TicketPlano)
			}
			return puertos.ResultadoTurno{Desenlace: "ticket_consumido"}, nil
		},
	}
	m := NuevoManejadorConfianza(portero, &mocks.GestorDeSalasDeEspera{}, &mocks.ConsultorDeSalas{})

	out, err := m.ConsultarTurno(context.Background(), &ConsultarTurnoInput{
		AliasSala:  "inscripciones-2026",
		TicketCola: "mot_cola_secreto",
	})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if out.Body.Ticket != "" {
		t.Fatalf("Ticket = %q, se esperaba vacío (INV-COLA-07)", out.Body.Ticket)
	}
	if out.Body.Desenlace != "ticket_consumido" {
		t.Fatalf("desenlace = %q, esperado \"ticket_consumido\" — un ticket inválido es un 200, no un error (§7.1)", out.Body.Desenlace)
	}
	if out.CacheControl != "no-store" {
		t.Fatalf("Cache-Control = %q, esperado \"no-store\"", out.CacheControl)
	}
}

func TestManejadorConfianza_ObtenerSalaPublica_EsCacheable(t *testing.T) {
	consultor := &mocks.ConsultorDeSalas{
		FnObtenerPorAlias: func(_ context.Context, q puertos.ConsultaSalaPorAlias) (puertos.VistaSalaPublica, error) {
			return puertos.VistaSalaPublica{
				Alias:              q.Alias,
				Estado:             "abierta",
				LongitudAproximada: 38110,
				EsperaEstimada:     762 * time.Second,
				ReconsultarEn:      15 * time.Second,
			}, nil
		},
	}
	m := NuevoManejadorConfianza(&mocks.PorteroDeSala{}, &mocks.GestorDeSalasDeEspera{}, consultor)

	out, err := m.ObtenerSalaPublica(context.Background(), &ObtenerSalaPublicaInput{AliasSala: "inscripciones-2026"})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if out.CacheControl != "public, max-age=5" {
		t.Fatalf("Cache-Control = %q, esperado \"public, max-age=5\" (§7.1 del diseño)", out.CacheControl)
	}
	if out.Body.Alias != "inscripciones-2026" || out.Body.LongitudAproximada != 38110 {
		t.Fatalf("proyección incorrecta: %+v", out.Body)
	}
}

func TestManejadorConfianza_AbrirSala_PropagaIDSujetoDelContexto(t *testing.T) {
	var comandoCapturado puertos.ComandoAbrirSala
	gestor := &mocks.GestorDeSalasDeEspera{
		FnAbrir: func(_ context.Context, cmd puertos.ComandoAbrirSala) (puertos.VistaSala, error) {
			comandoCapturado = cmd
			return puertos.VistaSala{ID: "sala-1", Alias: cmd.Alias}, nil
		},
	}
	m := NuevoManejadorConfianza(&mocks.PorteroDeSala{}, gestor, &mocks.ConsultorDeSalas{})

	ctx := context.WithValue(context.Background(), claveAcceso{}, accesoDePrueba("018e7e6a-0000-7000-8000-000000000001"))
	out, err := m.AbrirSala(ctx, &AbrirSalaInput{
		IDOrganizacion: "018e7e6a-0000-7000-8000-000000000002",
		Body: abrirSalaPeticion{
			Alias:                  "inscripciones-2026",
			Ruta:                   "tenencia.aceptar_invitacion",
			RitmoAdmision:          50,
			CapacidadMaximaCola:    500000,
			VentanaReclamoSegundos: 120,
			ModoDegradado:          "permitir",
		},
	})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if comandoCapturado.IDSujeto != "018e7e6a-0000-7000-8000-000000000001" {
		t.Fatalf("IDSujeto = %q, se esperaba que saliera del contexto de autenticación (INV-TEN-12)", comandoCapturado.IDSujeto)
	}
	if comandoCapturado.IDOrganizacion != "018e7e6a-0000-7000-8000-000000000002" {
		t.Fatalf("IDOrganizacion = %q, esperado el de la ruta", comandoCapturado.IDOrganizacion)
	}
	if comandoCapturado.VentanaReclamo != 120*time.Second {
		t.Fatalf("VentanaReclamo = %v, esperado 120s", comandoCapturado.VentanaReclamo)
	}
	if out.Body.ID != "sala-1" {
		t.Fatalf("ID = %q, esperado \"sala-1\"", out.Body.ID)
	}
}

func TestManejadorConfianza_ActualizarSala_ExigeAlMenosUnCampo(t *testing.T) {
	m := NuevoManejadorConfianza(&mocks.PorteroDeSala{}, &mocks.GestorDeSalasDeEspera{}, &mocks.ConsultorDeSalas{})

	_, err := m.ActualizarSala(context.Background(), &ActualizarSalaInput{
		IDOrganizacion: "018e7e6a-0000-7000-8000-000000000002",
		IDSala:         "018e7e6a-0000-7000-8000-000000000003",
		Body:           actualizarSalaPeticion{},
	})
	if err == nil {
		t.Fatal("se esperaba un error 422 cuando ni ritmo_admision ni estado están presentes")
	}
}

func TestManejadorConfianza_ActualizarSala_CambiaRitmo(t *testing.T) {
	var comandoCapturado puertos.ComandoCambiarRitmoAdmision
	gestor := &mocks.GestorDeSalasDeEspera{
		FnCambiarRitmo: func(_ context.Context, cmd puertos.ComandoCambiarRitmoAdmision) (puertos.VistaSala, error) {
			comandoCapturado = cmd
			return puertos.VistaSala{ID: cmd.IDSala, RitmoAdmision: cmd.RitmoAdmision}, nil
		},
	}
	m := NuevoManejadorConfianza(&mocks.PorteroDeSala{}, gestor, &mocks.ConsultorDeSalas{})

	nuevoRitmo := 120
	out, err := m.ActualizarSala(context.Background(), &ActualizarSalaInput{
		IDOrganizacion: "018e7e6a-0000-7000-8000-000000000002",
		IDSala:         "018e7e6a-0000-7000-8000-000000000003",
		Body:           actualizarSalaPeticion{RitmoAdmision: &nuevoRitmo},
	})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if comandoCapturado.RitmoAdmision != 120 {
		t.Fatalf("RitmoAdmision = %d, esperado 120", comandoCapturado.RitmoAdmision)
	}
	if out.Body.RitmoAdmision != 120 {
		t.Fatalf("respuesta.RitmoAdmision = %d, esperado 120", out.Body.RitmoAdmision)
	}
}

func TestManejadorConfianza_ActualizarSala_CambiaEstado(t *testing.T) {
	var comandoCapturado puertos.ComandoCambiarEstadoSala
	gestor := &mocks.GestorDeSalasDeEspera{
		FnCambiarEstado: func(_ context.Context, cmd puertos.ComandoCambiarEstadoSala) (puertos.VistaSala, error) {
			comandoCapturado = cmd
			return puertos.VistaSala{ID: cmd.IDSala, Estado: cmd.Destino}, nil
		},
	}
	m := NuevoManejadorConfianza(&mocks.PorteroDeSala{}, gestor, &mocks.ConsultorDeSalas{})

	destino := "drenando"
	out, err := m.ActualizarSala(context.Background(), &ActualizarSalaInput{
		IDOrganizacion: "018e7e6a-0000-7000-8000-000000000002",
		IDSala:         "018e7e6a-0000-7000-8000-000000000003",
		Body:           actualizarSalaPeticion{Estado: &destino},
	})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if comandoCapturado.Destino != "drenando" {
		t.Fatalf("Destino = %q, esperado \"drenando\"", comandoCapturado.Destino)
	}
	if out.Body.Estado != "drenando" {
		t.Fatalf("respuesta.Estado = %q, esperado \"drenando\"", out.Body.Estado)
	}
}
