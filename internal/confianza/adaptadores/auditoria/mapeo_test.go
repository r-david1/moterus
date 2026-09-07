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
