package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"

	"github.com/r-david1/moterus/internal/confianza/dominio"
	"github.com/r-david1/moterus/internal/confianza/puertos"
	"github.com/r-david1/moterus/internal/confianza/puertos/mocks"
)

// nuevoContextoPrueba construye un huma.API real (humatest.New, con
// negociación de contenido JSON por defecto) y un huma.Context sobre un
// http.Request/httptest.ResponseRecorder — el mecanismo de pruebas que el
// propio paquete huma/v2 provee, sin necesitar Fiber (no hay un patrón de
// httptest ya establecido en tenencia/identidad/acceso para probar
// middlewares Huma; este es el más cercano al que usa Huma internamente).
func nuevoContextoPrueba(t *testing.T, cabeceras map[string]string) (huma.API, huma.Context, *httptest.ResponseRecorder) {
	t.Helper()
	_, api := humatest.New(t)

	req := httptest.NewRequest(http.MethodPost, "/acceso/sesiones", nil)
	for k, v := range cabeceras {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	op := &huma.Operation{OperationID: "acceso-iniciar-sesion", Method: http.MethodPost, Path: "/acceso/sesiones"}
	ctx := humatest.NewContext(op, req, w)
	return api, ctx, w
}

func TestMiddlewareSalaDeEspera_SinSalaVigente_DejaPasar(t *testing.T) {
	api, ctx, w := nuevoContextoPrueba(t, nil)
	portero := &mocks.PorteroDeSala{
		FnSalaVigentePara: func(_ context.Context, _ puertos.ConsultaSalaVigente) (puertos.VistaSalaVigente, bool) {
			return puertos.VistaSalaVigente{}, false
		},
	}
	llamadoNext := false

	mw := MiddlewareSalaDeEspera(api, portero, dominio.RutaAccesoIniciarSesion)
	mw(ctx, func(huma.Context) { llamadoNext = true })

	if !llamadoNext {
		t.Fatal("se esperaba que next() se invocara: sin sala vigente, el middleware no debe hacer nada más")
	}
	if w.Code != 0 && w.Code != http.StatusOK {
		t.Fatalf("no se esperaba que el middleware escribiera una respuesta, status = %d", w.Code)
	}
}

func salaVigenteDePrueba(modoDegradado string) puertos.VistaSalaVigente {
	return puertos.VistaSalaVigente{
		Alias:         "inscripciones-2026",
		Clave:         "sistema:acceso.iniciar_sesion",
		Estado:        "abierta",
		ModoDegradado: modoDegradado,
	}
}

func TestMiddlewareSalaDeEspera_SalaVigenteSinTicket_Bloquea503(t *testing.T) {
	api, ctx, w := nuevoContextoPrueba(t, nil)
	portero := &mocks.PorteroDeSala{
		FnSalaVigentePara: func(_ context.Context, _ puertos.ConsultaSalaVigente) (puertos.VistaSalaVigente, bool) {
			return salaVigenteDePrueba("permitir"), true
		},
	}
	llamadoNext := false

	mw := MiddlewareSalaDeEspera(api, portero, dominio.RutaAccesoIniciarSesion)
	mw(ctx, func(huma.Context) { llamadoNext = true })

	if llamadoNext {
		t.Fatal("no se esperaba que next() se invocara: falta la cabecera X-Ticket-Cola")
	}
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, esperado 503", w.Code)
	}
	if got := w.Header().Get("Retry-After"); got != "5" {
		t.Fatalf("Retry-After = %q, esperado \"5\"", got)
	}

	var cuerpo cuerpoBloqueoSala
	if err := json.Unmarshal(w.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("no se pudo parsear el cuerpo: %v", err)
	}
	if cuerpo.Desenlace != "ticket_requerido" {
		t.Fatalf("desenlace = %q, esperado \"ticket_requerido\"", cuerpo.Desenlace)
	}
	if cuerpo.AliasSala != "inscripciones-2026" {
		t.Fatalf("alias_sala = %q, esperado \"inscripciones-2026\"", cuerpo.AliasSala)
	}
	if cuerpo.Ingreso != "/confianza/salas-espera/inscripciones-2026/tickets" {
		t.Fatalf("ingreso = %q, no coincide con la ruta pública de ingreso", cuerpo.Ingreso)
	}
}

func TestMiddlewareSalaDeEspera_TicketAdmitido_DejaPasar(t *testing.T) {
	api, ctx, w := nuevoContextoPrueba(t, map[string]string{"X-Ticket-Cola": "mot_cola_valido"})
	portero := &mocks.PorteroDeSala{
		FnSalaVigentePara: func(_ context.Context, _ puertos.ConsultaSalaVigente) (puertos.VistaSalaVigente, bool) {
			return salaVigenteDePrueba("permitir"), true
		},
		FnReclamar: func(_ context.Context, cmd puertos.ComandoReclamarTurno) (puertos.ResultadoTurno, error) {
			if cmd.Clave != "sistema:acceso.iniciar_sesion" || cmd.TicketPlano != "mot_cola_valido" {
				t.Fatalf("Reclamar recibió argumentos inesperados: %+v", cmd)
			}
			return puertos.ResultadoTurno{Desenlace: "admitido"}, nil
		},
	}
	llamadoNext := false

	mw := MiddlewareSalaDeEspera(api, portero, dominio.RutaAccesoIniciarSesion)
	mw(ctx, func(huma.Context) { llamadoNext = true })

	if !llamadoNext {
		t.Fatal("se esperaba que next() se invocara: el ticket fue admitido")
	}
	if w.Code != 0 && w.Code != http.StatusOK {
		t.Fatalf("no se esperaba que el middleware escribiera una respuesta, status = %d", w.Code)
	}
}

func TestMiddlewareSalaDeEspera_TurnoNoAlcanzado_Bloquea503ConRetryDeLaEsperaEstimada(t *testing.T) {
	api, ctx, w := nuevoContextoPrueba(t, map[string]string{"X-Ticket-Cola": "mot_cola_valido"})
	portero := &mocks.PorteroDeSala{
		FnSalaVigentePara: func(_ context.Context, _ puertos.ConsultaSalaVigente) (puertos.VistaSalaVigente, bool) {
			return salaVigenteDePrueba("permitir"), true
		},
		FnReclamar: func(_ context.Context, _ puertos.ComandoReclamarTurno) (puertos.ResultadoTurno, error) {
			return puertos.ResultadoTurno{
				Desenlace:      "esperando",
				Posicion:       12843,
				EsperaEstimada: 257 * time.Second,
			}, &dominio.ErrTurnoNoAlcanzado{Posicion: 12843}
		},
	}
	llamadoNext := false

	mw := MiddlewareSalaDeEspera(api, portero, dominio.RutaAccesoIniciarSesion)
	mw(ctx, func(huma.Context) { llamadoNext = true })

	if llamadoNext {
		t.Fatal("no se esperaba que next() se invocara: el turno todavía no llegó")
	}
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, esperado 503", w.Code)
	}
	// El ejemplo de §7.4 del diseño fija Retry-After == espera_estimada_segundos
	// para el desenlace "esperando".
	if got := w.Header().Get("Retry-After"); got != "257" {
		t.Fatalf("Retry-After = %q, esperado \"257\"", got)
	}

	var cuerpo cuerpoBloqueoSala
	if err := json.Unmarshal(w.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("no se pudo parsear el cuerpo: %v", err)
	}
	if cuerpo.Desenlace != "esperando" {
		t.Fatalf("desenlace = %q, esperado \"esperando\"", cuerpo.Desenlace)
	}
	if cuerpo.Posicion != 12843 {
		t.Fatalf("posicion = %d, esperado 12843", cuerpo.Posicion)
	}
	if cuerpo.EsperaEstimadaSegundos != 257 {
		t.Fatalf("espera_estimada_segundos = %d, esperado 257", cuerpo.EsperaEstimadaSegundos)
	}
}

func TestMiddlewareSalaDeEspera_ErrorDeInfraestructura_ModoPermitir_DejaPasar(t *testing.T) {
	api, ctx, _ := nuevoContextoPrueba(t, map[string]string{"X-Ticket-Cola": "mot_cola_valido"})
	portero := &mocks.PorteroDeSala{
		FnSalaVigentePara: func(_ context.Context, _ puertos.ConsultaSalaVigente) (puertos.VistaSalaVigente, bool) {
			return salaVigenteDePrueba("permitir"), true
		},
		FnReclamar: func(_ context.Context, _ puertos.ComandoReclamarTurno) (puertos.ResultadoTurno, error) {
			return puertos.ResultadoTurno{}, errors.New("confianza/redis: fallo al reclamar el turno")
		},
	}
	llamadoNext := false

	mw := MiddlewareSalaDeEspera(api, portero, dominio.RutaAccesoIniciarSesion)
	mw(ctx, func(huma.Context) { llamadoNext = true })

	if !llamadoNext {
		t.Fatal("se esperaba fail-open: modoDegradado=permitir debe dejar pasar ante una falla de infraestructura")
	}
}

func TestMiddlewareSalaDeEspera_ErrorDeInfraestructura_ModoRechazar_Bloquea503(t *testing.T) {
	api, ctx, w := nuevoContextoPrueba(t, map[string]string{"X-Ticket-Cola": "mot_cola_valido"})
	portero := &mocks.PorteroDeSala{
		FnSalaVigentePara: func(_ context.Context, _ puertos.ConsultaSalaVigente) (puertos.VistaSalaVigente, bool) {
			return salaVigenteDePrueba("rechazar"), true
		},
		FnReclamar: func(_ context.Context, _ puertos.ComandoReclamarTurno) (puertos.ResultadoTurno, error) {
			return puertos.ResultadoTurno{}, errors.New("confianza/redis: fallo al reclamar el turno")
		},
	}
	llamadoNext := false

	mw := MiddlewareSalaDeEspera(api, portero, dominio.RutaAccesoIniciarSesion)
	mw(ctx, func(huma.Context) { llamadoNext = true })

	if llamadoNext {
		t.Fatal("se esperaba fail-closed: modoDegradado=rechazar no debe dejar pasar ante una falla de infraestructura")
	}
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, esperado 503", w.Code)
	}
	if got := w.Header().Get("Retry-After"); got != "30" {
		t.Fatalf("Retry-After = %q, esperado \"30\" (§8 del diseño)", got)
	}

	var cuerpo cuerpoBloqueoSala
	if err := json.Unmarshal(w.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("no se pudo parsear el cuerpo: %v", err)
	}
	if cuerpo.Desenlace != "sala_no_disponible" {
		t.Fatalf("desenlace = %q, esperado \"sala_no_disponible\"", cuerpo.Desenlace)
	}
}

func TestSegundosCeil(t *testing.T) {
	casos := []struct {
		d        time.Duration
		esperado int64
	}{
		{0, 0},
		{-5 * time.Second, 0},
		{500 * time.Millisecond, 1},
		{90 * time.Second, 90},
		{90*time.Second + 100*time.Millisecond, 91},
	}
	for _, c := range casos {
		if got := segundosCeil(c.d); got != c.esperado {
			t.Errorf("segundosCeil(%v) = %d, esperado %d", c.d, got, c.esperado)
		}
	}
}
