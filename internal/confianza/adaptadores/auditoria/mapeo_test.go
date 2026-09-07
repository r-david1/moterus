package auditoria

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/r-david1/moterus/internal/confianza/dominio"
)

// TestMapearEvento_LosTresEventosSonReconocidos cubre el catálogo cerrado
// completo del §1.7 del diseño docs/design/colas-virtuales.md.
func TestMapearEvento_LosTresEventosSonReconocidos(t *testing.T) {
	ahora := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	id, err := dominio.IDSalaDeEsperaDesde("018e7e6a-0000-7000-8000-000000000001")
	if err != nil {
		t.Fatalf("no se pudo construir el ID de prueba: %v", err)
	}
	alias, err := dominio.NuevoAliasSala("inscripciones-2026")
	if err != nil {
		t.Fatalf("no se pudo construir el alias de prueba: %v", err)
	}
	ruta, err := dominio.RutaProtegidaDesde("acceso.iniciar_sesion")
	if err != nil {
		t.Fatalf("no se pudo construir la ruta de prueba: %v", err)
	}
	ritmo, err := dominio.NuevoRitmoAdmision(50)
	if err != nil {
		t.Fatalf("no se pudo construir el ritmo de prueba: %v", err)
	}
	politica, err := dominio.NuevaPoliticaSala(ritmo, 500000, 2*time.Minute, dominio.ModoDegradadoPermitir)
	if err != nil {
		t.Fatalf("no se pudo construir la política de prueba: %v", err)
	}
	ritmoNuevo, err := dominio.NuevoRitmoAdmision(120)
	if err != nil {
		t.Fatalf("no se pudo construir el ritmo nuevo de prueba: %v", err)
	}

	eventos := []dominio.EventoDominio{
		dominio.NuevoSalaDeEsperaAbierta(id, alias, dominio.AlcanceSistema(), ruta, politica, ahora),
		dominio.NuevoRitmoDeAdmisionCambiado(id, ritmo, ritmoNuevo, 100, 5000, ahora),
		dominio.NuevoSalaDeEsperaCerrada(id, dominio.EstadoSalaCerrada, 41902, 40000, ahora),
	}

	for _, ev := range eventos {
		t.Run(ev.NombreEvento(), func(t *testing.T) {
			fila, reconocido := mapearEvento(ev)
			if !reconocido {
				t.Fatalf("el evento %s no fue reconocido por el catálogo cerrado", ev.NombreEvento())
			}
			if fila.recurso != recursoSalaEspera {
				t.Fatalf("recurso = %q, esperado %q", fila.recurso, recursoSalaEspera)
			}
			if fila.recursoID != id.String() {
				t.Fatalf("recursoID = %q, esperado %q", fila.recursoID, id.String())
			}
			if fila.resultado != resultadoExito {
				t.Fatalf("resultado = %q, esperado %q", fila.resultado, resultadoExito)
			}

			// Bug real ya encontrado en este mismo repo durante OTP/MFA
			// (identidad/adaptadores/auditoria/mapeo.go): un `detalles` que
			// quede como el mapa nil por defecto serializa a "null", no
			// "{}", y viola el CHECK jsonb_typeof(detalles)='object' de la
			// tabla auditoria. Los tres eventos de Confianza siempre
			// llevan campos propios en detalles, pero se verifica el
			// invariante explícitamente para que un futuro evento sin
			// campos no reintroduzca el bug en silencio.
			if fila.detalles == nil {
				t.Fatal("detalles no puede ser nil: json.Marshal(nil) produce \"null\", que viola el CHECK de la tabla auditoria")
			}
			serializado, err := json.Marshal(fila.detalles)
			if err != nil {
				t.Fatalf("no se pudo serializar detalles: %v", err)
			}
			if string(serializado) == "null" {
				t.Fatal("detalles serializó a \"null\": violaría el CHECK jsonb_typeof(detalles)='object'")
			}
		})
	}
}

func TestMapearEvento_SalaDeEsperaAbierta_DetallesCompletos(t *testing.T) {
	ahora := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	id, _ := dominio.IDSalaDeEsperaDesde("018e7e6a-0000-7000-8000-000000000001")
	alias, _ := dominio.NuevoAliasSala("inscripciones-2026")
	ruta, _ := dominio.RutaProtegidaDesde("acceso.iniciar_sesion")
	ritmo, _ := dominio.NuevoRitmoAdmision(50)
	politica, _ := dominio.NuevaPoliticaSala(ritmo, 500000, 2*time.Minute, dominio.ModoDegradadoPermitir)

	ev := dominio.NuevoSalaDeEsperaAbierta(id, alias, dominio.AlcanceSistema(), ruta, politica, ahora)
	fila, reconocido := mapearEvento(ev)
	if !reconocido {
		t.Fatal("SalaDeEsperaAbierta no fue reconocido")
	}
	if fila.accion != "sala_espera.abierta" {
		t.Fatalf("accion = %q, esperado \"sala_espera.abierta\"", fila.accion)
	}

	esperados := map[string]any{
		"alias":            "inscripciones-2026",
		"alcance":          "sistema",
		"ruta":             "acceso.iniciar_sesion",
		"ritmo":            50,
		"capacidad_maxima": int64(500000),
		"modo_degradado":   "permitir",
	}
	for clave, valorEsperado := range esperados {
		if fila.detalles[clave] != valorEsperado {
			t.Fatalf("detalles[%q] = %v (%T), esperado %v (%T)", clave, fila.detalles[clave], fila.detalles[clave], valorEsperado, valorEsperado)
		}
	}
}

// TestMapearEvento_OrigenNuevoObservado_DetallesCompletos cubre el `case`
// agregado por la extensión de reconocimiento de origen (§1.6 de
// fingerprinting-comportamiento.md).
func TestMapearEvento_OrigenNuevoObservado_DetallesCompletos(t *testing.T) {
	ahora := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	senales := []dominio.SenalRiesgo{dominio.SenalDispositivoDesconocido, dominio.SenalRedDesconocida}
	puntaje := dominio.NuevoPuntajeRiesgo(0.65)
	pol := dominio.PoliticaRiesgoPorDefecto()
	nivel := puntaje.Nivel(pol)

	ev := dominio.NuevoOrigenNuevoObservado("usuario-123", senales, puntaje, nivel, dominio.ModoRiesgoObservar, 3, ahora)

	fila, reconocido := mapearEvento(ev)
	if !reconocido {
		t.Fatal("OrigenNuevoObservado no fue reconocido")
	}
	if fila.accion != "origen.nuevo" {
		t.Fatalf("accion = %q, esperado \"origen.nuevo\"", fila.accion)
	}
	if fila.recurso != recursoOrigen {
		t.Fatalf("recurso = %q, esperado %q", fila.recurso, recursoOrigen)
	}
	if fila.usuarioID != "usuario-123" {
		t.Fatalf("usuarioID = %q, esperado \"usuario-123\"", fila.usuarioID)
	}
	if fila.resultado != resultadoExito {
		t.Fatalf("resultado = %q, esperado %q", fila.resultado, resultadoExito)
	}
	if fila.detalles == nil {
		t.Fatal("detalles no puede ser nil: json.Marshal(nil) produce \"null\", que viola el CHECK de la tabla auditoria")
	}

	serializado, err := json.Marshal(fila.detalles)
	if err != nil {
		t.Fatalf("no se pudo serializar detalles: %v", err)
	}
	if string(serializado) == "null" {
		t.Fatal("detalles serializó a \"null\": violaría el CHECK jsonb_typeof(detalles)='object'")
	}
	if fila.detalles["origenes_conocidos"] != 3 {
		t.Fatalf("detalles[origenes_conocidos] = %v, esperado 3", fila.detalles["origenes_conocidos"])
	}
	if fila.detalles["modo"] != "observar" {
		t.Fatalf("detalles[modo] = %v, esperado \"observar\"", fila.detalles["modo"])
	}
}

// TestMapearEvento_OrigenNuevoObservado_SenalesVaciasNoProducenDetallesNulos
// cubre explícitamente el caso límite que el bug de OTP/MFA hizo evidente
// (identidad/adaptadores/auditoria/mapeo.go, caso ContrasenaCambiada): aunque
// Senales llegara vacío o nil, detalles nunca debe degradar a un mapa nil ni
// a un slice serializado como "null".
func TestMapearEvento_OrigenNuevoObservado_SenalesVaciasNoProducenDetallesNulos(t *testing.T) {
	ahora := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	pol := dominio.PoliticaRiesgoPorDefecto()
	puntaje := dominio.NuevoPuntajeRiesgo(0)
	nivel := puntaje.Nivel(pol)

	ev := dominio.NuevoOrigenNuevoObservado("usuario-456", nil, puntaje, nivel, dominio.ModoRiesgoObservar, 1, ahora)

	fila, reconocido := mapearEvento(ev)
	if !reconocido {
		t.Fatal("OrigenNuevoObservado no fue reconocido")
	}
	if fila.detalles == nil {
		t.Fatal("detalles no puede ser nil")
	}
	senales, ok := fila.detalles["senales"].([]string)
	if !ok {
		t.Fatalf("detalles[senales] tiene tipo inesperado: %T", fila.detalles["senales"])
	}
	if senales == nil {
		t.Fatal("detalles[senales] no puede ser un slice nil: serializaría a \"null\" en vez de \"[]\"")
	}

	serializado, err := json.Marshal(fila.detalles)
	if err != nil {
		t.Fatalf("no se pudo serializar detalles: %v", err)
	}
	if string(serializado) == "null" {
		t.Fatal("detalles serializó a \"null\"")
	}
}

func TestMapearEvento_EventoNoReconocido(t *testing.T) {
	_, reconocido := mapearEvento(eventoDeMentira{})
	if reconocido {
		t.Fatal("un evento fuera del catálogo cerrado no debe reconocerse")
	}
}

type eventoDeMentira struct{}

func (eventoDeMentira) NombreEvento() string  { return "EventoDeMentira" }
func (eventoDeMentira) OcurridoEn() time.Time { return time.Time{} }
func (eventoDeMentira) IDAgregado() string    { return "" }
