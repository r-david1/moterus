package aplicacion_test

// Fixtures compartidos entre los tests de los casos de uso de aplicacion.
// Viven en package aplicacion_test (caja negra): los tests solo ejercen la
// API pública de aplicacion/puertos/dominio. Mismo patrón que
// tenencia/aplicacion/helpers_aplicacion_test.go y
// acceso/aplicacion/helpers_aplicacion_test.go.

import (
	"testing"
	"time"

	"github.com/r-david1/moterus/internal/confianza/dominio"
)

// idSalaValido1/2 son UUIDs sintácticamente válidos y distintos.
const (
	idSalaValido1 = "018e7e6a-0000-7000-8000-000000000401"
	idSalaValido2 = "018e7e6a-0000-7000-8000-000000000402"
)

// idOrganizacionValido1 es un UUID sintácticamente válido.
const idOrganizacionValido1 = "018e7e6a-0000-7000-8000-000000000101"

// idUsuarioValido1 es un UUID sintácticamente válido.
const idUsuarioValido1 = "123e4567-e89b-12d3-a456-426614174000"

// ticketPlanoValido1/2 son dos tickets de cola en claro con forma válida
// (prefijo mot_cola_ + 43 caracteres base64url) y distintos.
const (
	ticketPlanoValido1 = "mot_cola_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	ticketPlanoValido2 = "mot_cola_BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"
)

func ahoraDePrueba() time.Time {
	return time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
}

func origenDePrueba(t *testing.T) dominio.OrigenSolicitud {
	t.Helper()
	o, err := dominio.NuevoOrigenSolicitud("203.0.113.7", "agente-test/1.0", "huella-test", "req-confianza-123")
	if err != nil {
		t.Fatalf("no se pudo construir el origen de prueba: %v", err)
	}
	return o
}

func ticketPlanoDePrueba(t *testing.T, valor string) dominio.TicketPlano {
	t.Helper()
	p, err := dominio.NuevoTicketPlano(valor)
	if err != nil {
		t.Fatalf("no se pudo construir el ticket de prueba %q: %v", valor, err)
	}
	return p
}

func idSalaDePrueba(t *testing.T, uuid string) dominio.IDSalaDeEspera {
	t.Helper()
	id, err := dominio.IDSalaDeEsperaDesde(uuid)
	if err != nil {
		t.Fatalf("no se pudo construir el IDSalaDeEspera de prueba %q: %v", uuid, err)
	}
	return id
}

// politicaSalaDePrueba construye una PoliticaSala válida con el ritmo dado y
// el resto de los parámetros en sus valores por defecto.
func politicaSalaDePrueba(t *testing.T, ritmoPorSegundo int) dominio.PoliticaSala {
	t.Helper()
	r, err := dominio.NuevoRitmoAdmision(ritmoPorSegundo)
	if err != nil {
		t.Fatalf("ritmo de prueba inválido: %v", err)
	}
	p, err := dominio.NuevaPoliticaSala(r, 500_000, 2*time.Minute, dominio.ModoDegradadoPermitir)
	if err != nil {
		t.Fatalf("política de prueba inválida: %v", err)
	}
	return p
}
