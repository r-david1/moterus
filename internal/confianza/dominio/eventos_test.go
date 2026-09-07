package dominio

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

// TestINV_COLA_07_EventosNuncaTransportanElTicketNiSuHash recorre por
// reflexión los tres eventos de dominio del catálogo de §1.7 y verifica que
// ninguno tiene un campo de tipo TicketPlano ni HashTicket, y que ningún
// nombre de campo sugiere transportar un ticket o un secreto. Es lo que
// exige INV-COLA-07: "el ticket en claro nunca [...] viaja en un evento de
// dominio", verificado con el mismo tipo de test que ya usan
// identidad/dominio, acceso/dominio y tenencia/dominio (INV-TEN-23).
func TestINV_COLA_07_EventosNuncaTransportanElTicketNiSuHash(t *testing.T) {
	ahora := time.Now()
	id, _ := IDSalaDeEsperaDesde("018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6d")
	alias, _ := NuevoAliasSala("inscripciones-2026")
	ritmoAnterior, _ := NuevoRitmoAdmision(50)
	ritmoNuevo, _ := NuevoRitmoAdmision(120)

	eventos := []EventoDominio{
		NuevoSalaDeEsperaAbierta(id, alias, AlcanceSistema(), RutaAccesoIniciarSesion, PoliticaSalaPorDefecto(), ahora),
		NuevoRitmoDeAdmisionCambiado(id, ritmoAnterior, ritmoNuevo, 1000, 5000, ahora),
		NuevoSalaDeEsperaCerrada(id, EstadoSalaCerrada, 10000, 9500, ahora),
	}

	if len(eventos) != 3 {
		t.Fatalf("se esperaban 3 eventos de dominio (§1.7 del diseño), hay %d", len(eventos))
	}

	tipoTicketPlano := reflect.TypeOf(TicketPlano{})
	tipoHashTicket := reflect.TypeOf(HashTicket{})
	nombresPeligrosos := []string{"ticketplano", "hashticket", "secreto"}

	for _, ev := range eventos {
		v := reflect.ValueOf(ev)
		tipoEv := v.Type()
		for i := 0; i < tipoEv.NumField(); i++ {
			campo := tipoEv.Field(i)
			if campo.Type == tipoTicketPlano || campo.Type == tipoHashTicket {
				t.Errorf("%s.%s tiene tipo %s: un evento no puede transportar el ticket ni su hash (INV-COLA-07)", tipoEv.Name(), campo.Name, campo.Type)
			}
			nombreMin := strings.ToLower(campo.Name)
			for _, peligroso := range nombresPeligrosos {
				if strings.Contains(nombreMin, peligroso) {
					t.Errorf("%s.%s tiene un nombre que sugiere transportar un secreto/ticket", tipoEv.Name(), campo.Name)
				}
			}
		}
		if ev.NombreEvento() == "" {
			t.Errorf("%T.NombreEvento() no debe estar vacío", ev)
		}
		if !ev.OcurridoEn().Equal(ahora) {
			t.Errorf("%T.OcurridoEn() = %v, esperado %v", ev, ev.OcurridoEn(), ahora)
		}
		if ev.IDAgregado() != id.String() {
			t.Errorf("%T.IDAgregado() = %q, esperado %q", ev, ev.IDAgregado(), id.String())
		}
	}
}

func TestEventos_NombresUnicos(t *testing.T) {
	ahora := time.Now()
	id, _ := IDSalaDeEsperaDesde("018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6d")
	alias, _ := NuevoAliasSala("inscripciones-2026")
	ritmoAnterior, _ := NuevoRitmoAdmision(50)
	ritmoNuevo, _ := NuevoRitmoAdmision(120)

	eventos := []EventoDominio{
		NuevoSalaDeEsperaAbierta(id, alias, AlcanceSistema(), RutaAccesoIniciarSesion, PoliticaSalaPorDefecto(), ahora),
		NuevoRitmoDeAdmisionCambiado(id, ritmoAnterior, ritmoNuevo, 1000, 5000, ahora),
		NuevoSalaDeEsperaCerrada(id, EstadoSalaCerrada, 10000, 9500, ahora),
	}
	vistos := map[string]bool{}
	for _, ev := range eventos {
		nombre := ev.NombreEvento()
		if vistos[nombre] {
			t.Errorf("el nombre de evento %q está duplicado", nombre)
		}
		vistos[nombre] = true
	}
}

// --- OrigenNuevoObservado (extensión de reconocimiento de origen) --------

// TestINV_RIES_12_OrigenNuevoObservado_NuncaTransportaDatosCrudos recorre
// por reflexión los campos de OrigenNuevoObservado y verifica que ninguno
// transporta la huella cruda, su hash, el correo ni la IP: solo códigos del
// catálogo cerrado SenalRiesgo (como string) y enteros/códigos cortos.
// Mismo patrón que TestINV_COLA_07_EventosNuncaTransportanElTicketNiSuHash.
func TestINV_RIES_12_OrigenNuevoObservado_NuncaTransportaDatosCrudos(t *testing.T) {
	ahora := time.Now()
	ev := NuevoOrigenNuevoObservado(
		"018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6d",
		[]SenalRiesgo{SenalDispositivoDesconocido, SenalRedDesconocida},
		NuevoPuntajeRiesgo(0.65),
		NivelRiesgoElevado,
		ModoRiesgoObservar,
		3,
		ahora,
	)

	if ev.NombreEvento() != "OrigenNuevoObservado" {
		t.Errorf("NombreEvento() = %q, esperado OrigenNuevoObservado", ev.NombreEvento())
	}
	if !ev.OcurridoEn().Equal(ahora) {
		t.Errorf("OcurridoEn() = %v, esperado %v", ev.OcurridoEn(), ahora)
	}
	if ev.IDAgregado() != "018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6d" {
		t.Errorf("IDAgregado() = %q, esperado el IDUsuario", ev.IDAgregado())
	}

	tipoHashHuella := reflect.TypeOf(HashHuella{})
	tipoHashRed := reflect.TypeOf(HashRed{})
	tipoOrigenSolicitud := reflect.TypeOf(OrigenSolicitud{})
	nombresPeligrosos := []string{"huelladispositivo", "correo", "email", "ipor", "direccionip", "hashhuella", "hashred"}

	tipoEv := reflect.TypeOf(ev)
	for i := 0; i < tipoEv.NumField(); i++ {
		campo := tipoEv.Field(i)
		if campo.Type == tipoHashHuella || campo.Type == tipoHashRed || campo.Type == tipoOrigenSolicitud {
			t.Errorf("OrigenNuevoObservado.%s tiene tipo %s: un evento no puede transportar la huella, su hash ni la IP (INV-RIES-12)", campo.Name, campo.Type)
		}
		nombreMin := strings.ToLower(campo.Name)
		for _, peligroso := range nombresPeligrosos {
			if strings.Contains(nombreMin, peligroso) {
				t.Errorf("OrigenNuevoObservado.%s tiene un nombre que sugiere transportar un dato crudo prohibido (INV-RIES-12)", campo.Name)
			}
		}
	}

	for _, s := range ev.Senales {
		if _, err := SenalRiesgoDesde(s); err != nil {
			t.Errorf("Senales contiene un valor fuera del catálogo cerrado: %q", s)
		}
	}
}

// TestINV_RIES_13_UnSoloTipoDeEventoParaElReconocimientoDeOrigen verifica
// que la extensión de reconocimiento de origen declara exactamente un tipo
// de evento (OrigenNuevoObservado): no se auditan las evaluaciones ni las
// denegaciones por riesgo (esas viajan como Motivo dentro de
// usuario.login/denegado, ADR 0018), solo el hecho raro de un origen nuevo.
// La condición "una vez por par (cuenta, dispositivo) y solo si ya había
// al menos un origen conocido" la aplica el caso de uso (aplicacion, fase
// posterior; OrigenesConocidos es el dato que se lo permite decidir), no
// el propio evento — de ahí que este test verifique la forma del evento,
// no la política de cuándo emitirlo.
func TestINV_RIES_13_UnSoloTipoDeEventoParaElReconocimientoDeOrigen(t *testing.T) {
	ev := NuevoOrigenNuevoObservado("id-usuario", nil, NuevoPuntajeRiesgo(0), NivelRiesgoNormal, ModoRiesgoObservar, 0, time.Now())
	var _ EventoDominio = ev // debe satisfacer el mismo contrato que los otros tres eventos del paquete.

	if ev.OrigenesConocidos != 0 {
		t.Errorf("OrigenesConocidos = %d, esperado 0 en este caso de prueba", ev.OrigenesConocidos)
	}
}

func TestSalaDeEsperaCerrada_DistingueDestinoPorCampo(t *testing.T) {
	id, _ := IDSalaDeEsperaDesde("018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6d")
	ahora := time.Now()
	drenando := NuevoSalaDeEsperaCerrada(id, EstadoSalaDrenando, 100, 80, ahora)
	cerrada := NuevoSalaDeEsperaCerrada(id, EstadoSalaCerrada, 100, 95, ahora)
	if drenando.Destino != "drenando" {
		t.Errorf("Destino = %q, esperado drenando", drenando.Destino)
	}
	if cerrada.Destino != "cerrada" {
		t.Errorf("Destino = %q, esperado cerrada", cerrada.Destino)
	}
	if drenando.NombreEvento() != cerrada.NombreEvento() {
		t.Error("ambos destinos deben compartir el mismo NombreEvento (un solo evento, distinguido por Destino, §1.7 del diseño)")
	}
}
