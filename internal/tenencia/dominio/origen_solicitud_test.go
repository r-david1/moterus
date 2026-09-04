package dominio

import (
	"errors"
	"strings"
	"testing"
)

func TestNuevaDireccionIP(t *testing.T) {
	casos := []struct {
		nombre   string
		valor    string
		esValido bool
	}{
		{"IPv4 válida", "203.0.113.10", true},
		{"IPv6 válida", "2001:db8::1", true},
		{"vacía", "", false},
		{"formato irreconocible", "no-es-una-ip", false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			ip, err := NuevaDireccionIP(c.valor)
			if c.esValido {
				if err != nil {
					t.Fatalf("no se esperaba error: %v", err)
				}
				if ip.EsVacia() {
					t.Error("no debería estar vacía")
				}
				return
			}
			if err == nil {
				t.Fatal("se esperaba error")
			}
			var errIP *ErrDireccionIPInvalida
			if !errors.As(err, &errIP) {
				t.Errorf("se esperaba *ErrDireccionIPInvalida, obtuvo %T", err)
			}
		})
	}
}

func TestDireccionIP_EsPrivada(t *testing.T) {
	privada, _ := NuevaDireccionIP("10.0.0.5")
	if !privada.EsPrivada() {
		t.Error("10.0.0.5 debería considerarse privada")
	}
	publica, _ := NuevaDireccionIP("203.0.113.10")
	if publica.EsPrivada() {
		t.Error("203.0.113.10 no debería considerarse privada")
	}
	var vacia DireccionIP
	if vacia.EsPrivada() {
		t.Error("una IP vacía no debe considerarse privada")
	}
}

func TestNuevoOrigenSolicitud(t *testing.T) {
	o, err := NuevoOrigenSolicitud("203.0.113.10", "agente/1.0", "huella-abc", "req-123")
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if o.IP().EsVacia() {
		t.Error("IP() no debería estar vacía")
	}
	if o.AgenteUsuario() != "agente/1.0" {
		t.Errorf("AgenteUsuario() = %q", o.AgenteUsuario())
	}
	if o.HuellaDispositivo() != "huella-abc" {
		t.Errorf("HuellaDispositivo() = %q", o.HuellaDispositivo())
	}
	if o.IDSolicitud() != "req-123" {
		t.Errorf("IDSolicitud() = %q", o.IDSolicitud())
	}
}

func TestNuevoOrigenSolicitud_IPVaciaParaLlamadasInternas(t *testing.T) {
	o, err := NuevoOrigenSolicitud("", "", "", "")
	if err != nil {
		t.Fatalf("no se esperaba error con campos vacíos (llamada interna del sistema): %v", err)
	}
	if !o.IP().EsVacia() {
		t.Error("IP() debería estar vacía cuando no se provee")
	}
}

func TestNuevoOrigenSolicitud_TruncaAgenteUsuario(t *testing.T) {
	agenteLargo := strings.Repeat("a", 1000)
	o, err := NuevoOrigenSolicitud("", agenteLargo, "", "")
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if len(o.AgenteUsuario()) != 512 {
		t.Errorf("longitud de AgenteUsuario() = %d, esperado 512", len(o.AgenteUsuario()))
	}
}

func TestNuevoOrigenSolicitud_RechazaIPInvalida(t *testing.T) {
	if _, err := NuevoOrigenSolicitud("no-es-una-ip", "", "", ""); err == nil {
		t.Fatal("se esperaba error con IP inválida")
	}
}
