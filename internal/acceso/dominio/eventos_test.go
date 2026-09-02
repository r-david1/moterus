package dominio

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

// TestINV_ACC_11_EventosNuncaTransportanSecretos recorre por reflexión los 7
// eventos de dominio del §1.6 del diseño y verifica que ningún campo es de
// tipo TokenRefrescoPlano ni HashTokenRefresco, y que ningún nombre de campo
// sugiere transportar un secreto, el JWT compacto o la llave de firma
// (INV-ACC-11).
func TestINV_ACC_11_EventosNuncaTransportanSecretos(t *testing.T) {
	ahora := time.Now()
	idSesion := idSesionDePrueba(t)
	idUsuario := idUsuarioDePrueba(t)

	eventos := []EventoDominio{
		NuevoSesionIniciada(idSesion, idUsuario, ahora, ahora),
		NuevoSesionRenovada(idSesion, idUsuario, 1, ahora),
		NuevoRenovacionRechazada(idSesion.String(), "motivo", ahora),
		NuevoReusoRefrescoDetectado(idSesion, idUsuario, 0, 1, ahora),
		NuevoSesionCerrada(idSesion, idUsuario, AlcanceCierreIndividual, 1, ahora),
		NuevoSesionRevocada(idSesion, idUsuario, MotivoCuentaNoOperativa, ahora),
		NuevoTokenAccesoRechazado(idSesion.String(), "kid_desconocido", ahora),
	}
	if len(eventos) != 7 {
		t.Fatalf("se esperaban 7 eventos de dominio (tabla 1.6 del diseño), hay %d", len(eventos))
	}

	tipoTokenPlano := reflect.TypeOf(TokenRefrescoPlano{})
	tipoHashRefresco := reflect.TypeOf(HashTokenRefresco{})
	nombresPeligrosos := []string{"token", "hash", "jwt", "llave", "secreto"}

	for _, ev := range eventos {
		v := reflect.ValueOf(ev)
		tipoEv := v.Type()
		for i := 0; i < tipoEv.NumField(); i++ {
			campo := tipoEv.Field(i)
			if campo.Type == tipoTokenPlano || campo.Type == tipoHashRefresco {
				t.Errorf("%s.%s tiene tipo %s: un evento no puede transportar tokens ni hashes de refresco", tipoEv.Name(), campo.Name, campo.Type)
			}
			nombreMin := strings.ToLower(campo.Name)
			for _, peligroso := range nombresPeligrosos {
				if strings.Contains(nombreMin, peligroso) {
					t.Errorf("%s.%s tiene un nombre que sugiere transportar un secreto/token/hash", tipoEv.Name(), campo.Name)
				}
			}
		}
		if ev.NombreEvento() == "" {
			t.Errorf("%T.NombreEvento() no debe estar vacío", ev)
		}
		if !ev.OcurridoEn().Equal(ahora) {
			t.Errorf("%T.OcurridoEn() = %v, esperado %v", ev, ev.OcurridoEn(), ahora)
		}
	}
}

func TestEventosAcceso_NombresUnicos(t *testing.T) {
	ahora := time.Now()
	idSesion := idSesionDePrueba(t)
	idUsuario := idUsuarioDePrueba(t)

	eventos := []EventoDominio{
		NuevoSesionIniciada(idSesion, idUsuario, ahora, ahora),
		NuevoSesionRenovada(idSesion, idUsuario, 1, ahora),
		NuevoRenovacionRechazada(idSesion.String(), "motivo", ahora),
		NuevoReusoRefrescoDetectado(idSesion, idUsuario, 0, 1, ahora),
		NuevoSesionCerrada(idSesion, idUsuario, AlcanceCierreIndividual, 1, ahora),
		NuevoSesionRevocada(idSesion, idUsuario, MotivoCuentaNoOperativa, ahora),
		NuevoTokenAccesoRechazado(idSesion.String(), "kid_desconocido", ahora),
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

func TestEventosAcceso_IDAgregado_ApuntaALaSesion(t *testing.T) {
	ahora := time.Now()
	idSesion := idSesionDePrueba(t)
	idUsuario := idUsuarioDePrueba(t)

	casos := []EventoDominio{
		NuevoSesionIniciada(idSesion, idUsuario, ahora, ahora),
		NuevoSesionRenovada(idSesion, idUsuario, 1, ahora),
		NuevoReusoRefrescoDetectado(idSesion, idUsuario, 0, 1, ahora),
		NuevoSesionCerrada(idSesion, idUsuario, AlcanceCierreIndividual, 1, ahora),
		NuevoSesionRevocada(idSesion, idUsuario, MotivoCuentaNoOperativa, ahora),
	}
	for _, ev := range casos {
		if ev.IDAgregado() != idSesion.String() {
			t.Errorf("%s.IDAgregado() = %q, esperado %q", ev.NombreEvento(), ev.IDAgregado(), idSesion.String())
		}
	}
}

func TestTokenAccesoRechazado_IDAgregado(t *testing.T) {
	idSesion := idSesionDePrueba(t)
	ev := NuevoTokenAccesoRechazado(idSesion.String(), "kid_desconocido", time.Now())
	if ev.IDAgregado() != idSesion.String() {
		t.Errorf("IDAgregado() = %q, esperado %q", ev.IDAgregado(), idSesion.String())
	}
	sinSesion := NuevoTokenAccesoRechazado("", "estructura_corrupta", time.Now())
	if sinSesion.IDAgregado() != "" {
		t.Errorf("IDAgregado() = %q, esperado vacío cuando no se pudo resolver el sid", sinSesion.IDAgregado())
	}
}

// TestRenovacionRechazada_IDAgregadoPuedeIrVacio verifica que
// RenovacionRechazada admite IDSesion vacío cuando el refresco presentado
// era desconocido (§1.6 del diseño: "IDAgregado puede ir vacío").
func TestRenovacionRechazada_IDAgregadoPuedeIrVacio(t *testing.T) {
	ev := NuevoRenovacionRechazada("", "refresco_desconocido", time.Now())
	if ev.IDAgregado() != "" {
		t.Errorf("IDAgregado() = %q, esperado vacío", ev.IDAgregado())
	}
	if ev.Motivo != "refresco_desconocido" {
		t.Errorf("Motivo = %q", ev.Motivo)
	}
}

func TestSesionRevocada_LlevaMotivoDelCatalogoCerrado(t *testing.T) {
	ev := NuevoSesionRevocada(idSesionDePrueba(t), idUsuarioDePrueba(t), MotivoLimiteSesionesExcedido, time.Now())
	if ev.Motivo != "limite_sesiones_excedido" {
		t.Errorf("Motivo = %q, esperado limite_sesiones_excedido", ev.Motivo)
	}
}
