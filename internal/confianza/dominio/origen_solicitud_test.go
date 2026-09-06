package dominio

import (
	"strings"
	"testing"
)

func TestNuevaDireccionIP_ValidaFormato(t *testing.T) {
	if _, err := NuevaDireccionIP(""); err == nil {
		t.Error("se esperaba error para IP vacía")
	}
	if _, err := NuevaDireccionIP("no-es-una-ip"); err == nil {
		t.Error("se esperaba error para formato irreconocible")
	}
	ip, err := NuevaDireccionIP("203.0.113.7")
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if ip.String() != "203.0.113.7" {
		t.Errorf("String() = %q", ip.String())
	}
	if ip.EsPrivada() {
		t.Error("203.0.113.7 es una IP pública (TEST-NET-3)")
	}
}

func TestDireccionIP_EsPrivada(t *testing.T) {
	ip, _ := NuevaDireccionIP("10.0.0.5")
	if !ip.EsPrivada() {
		t.Error("10.0.0.5 debería considerarse privada")
	}
}

func TestDireccionIP_ZeroValue(t *testing.T) {
	var ip DireccionIP
	if !ip.EsVacia() {
		t.Error("el zero value debería estar vacío")
	}
	if ip.String() != "" {
		t.Errorf("String() = %q, esperado vacío", ip.String())
	}
	if ip.EsPrivada() {
		t.Error("una IP vacía no debería considerarse privada")
	}
}

func TestNuevoOrigenSolicitud_IPVaciaParaLlamadaInterna(t *testing.T) {
	o, err := NuevoOrigenSolicitud("", "cli/1.0", "", "req-1")
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if !o.IP().EsVacia() {
		t.Error("la IP debería quedar vacía cuando no se provee")
	}
	if o.AgenteUsuario() != "cli/1.0" {
		t.Errorf("AgenteUsuario() = %q", o.AgenteUsuario())
	}
	if o.IDSolicitud() != "req-1" {
		t.Errorf("IDSolicitud() = %q", o.IDSolicitud())
	}
}

func TestNuevoOrigenSolicitud_HuellaDispositivo(t *testing.T) {
	o, err := NuevoOrigenSolicitud("", "", "huella-abc", "")
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if o.HuellaDispositivo() != "huella-abc" {
		t.Errorf("HuellaDispositivo() = %q", o.HuellaDispositivo())
	}
}

func TestNuevoOrigenSolicitud_TruncaAgenteUsuarioLargo(t *testing.T) {
	agente := strings.Repeat("a", 1000)
	o, err := NuevoOrigenSolicitud("", agente, "", "")
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if len([]rune(o.AgenteUsuario())) != longitudMaximaAgenteUsuario {
		t.Errorf("longitud del agente = %d, esperado %d", len([]rune(o.AgenteUsuario())), longitudMaximaAgenteUsuario)
	}
}

func TestNuevoOrigenSolicitud_PropagaErrorDeIPInvalida(t *testing.T) {
	if _, err := NuevoOrigenSolicitud("no-es-una-ip", "", "", ""); err == nil {
		t.Error("se esperaba error al propagar una IP inválida")
	}
}
