package dominio

import (
	"strings"
	"testing"
)

func TestNuevaDireccionIP_Valida(t *testing.T) {
	casos := []string{"192.168.1.10", "8.8.8.8", "::1", "2001:db8::1"}
	for _, c := range casos {
		ip, err := NuevaDireccionIP(c)
		if err != nil {
			t.Errorf("NuevaDireccionIP(%q) devolvió error inesperado: %v", c, err)
			continue
		}
		if ip.EsVacia() {
			t.Errorf("NuevaDireccionIP(%q) no debería resultar vacía", c)
		}
	}
}

func TestNuevaDireccionIP_Invalida(t *testing.T) {
	casos := []string{"", "no-es-una-ip", "999.999.999.999", "1.2.3"}
	for _, c := range casos {
		if _, err := NuevaDireccionIP(c); err == nil {
			t.Errorf("NuevaDireccionIP(%q) debía fallar", c)
		}
	}
}

func TestDireccionIP_EsPrivada(t *testing.T) {
	privada, _ := NuevaDireccionIP("10.0.0.5")
	if !privada.EsPrivada() {
		t.Error("10.0.0.5 debe reportarse como privada")
	}
	loopback, _ := NuevaDireccionIP("127.0.0.1")
	if !loopback.EsPrivada() {
		t.Error("127.0.0.1 (loopback) debe reportarse como privada")
	}
	publica, _ := NuevaDireccionIP("8.8.8.8")
	if publica.EsPrivada() {
		t.Error("8.8.8.8 no debe reportarse como privada")
	}
}

func TestDireccionIP_ZeroValue(t *testing.T) {
	var vacia DireccionIP
	if !vacia.EsVacia() {
		t.Error("el zero value de DireccionIP debe reportarse como vacío")
	}
	if vacia.String() != "" {
		t.Errorf("String() sobre una DireccionIP vacía = %q, esperado \"\"", vacia.String())
	}
	if vacia.EsPrivada() {
		t.Error("una DireccionIP vacía no debe reportarse como privada")
	}
}

func TestDireccionIP_EnlaceLocal(t *testing.T) {
	enlaceLocal, err := NuevaDireccionIP("169.254.1.1")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if !enlaceLocal.EsPrivada() {
		t.Error("169.254.1.1 (enlace local) debe reportarse como privada")
	}
}

func TestDireccionIP_String(t *testing.T) {
	ip, _ := NuevaDireccionIP("192.168.1.1")
	if ip.String() != "192.168.1.1" {
		t.Errorf("String() = %q, esperado 192.168.1.1", ip.String())
	}
}

func TestErrDireccionIPInvalida_Error(t *testing.T) {
	err := &ErrDireccionIPInvalida{Motivo: "x"}
	if err.Error() == "" {
		t.Error("ErrDireccionIPInvalida.Error() no debe estar vacío")
	}
}

func TestNuevoOrigenSolicitud_PermiteIPVacia(t *testing.T) {
	origen, err := NuevoOrigenSolicitud("", "sistema-interno", "", "req-123")
	if err != nil {
		t.Fatalf("una llamada interna sin IP no debe fallar: %v", err)
	}
	if !origen.IP().EsVacia() {
		t.Error("la IP debe quedar vacía para llamadas internas")
	}
}

func TestNuevoOrigenSolicitud_PropagaErrorDeIPInvalida(t *testing.T) {
	if _, err := NuevoOrigenSolicitud("ip-invalida", "agente", "huella", "req-1"); err == nil {
		t.Error("se esperaba que el error de DireccionIP se propague")
	}
}

func TestNuevoOrigenSolicitud_TruncaAgenteUsuario(t *testing.T) {
	agenteLargo := strings.Repeat("a", 1000)
	origen, err := NuevoOrigenSolicitud("", agenteLargo, "", "")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if len([]rune(origen.AgenteUsuario())) != 512 {
		t.Errorf("AgenteUsuario() debe truncarse a 512 caracteres, longitud obtenida %d", len([]rune(origen.AgenteUsuario())))
	}
}

func TestNuevoOrigenSolicitud_CamposCompletos(t *testing.T) {
	origen, err := NuevoOrigenSolicitud("192.168.0.1", "  Mozilla/5.0  ", "huella-abc", "req-xyz")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if origen.IP().EsVacia() {
		t.Error("la IP no debería quedar vacía")
	}
	if origen.AgenteUsuario() != "Mozilla/5.0" {
		t.Errorf("AgenteUsuario() = %q, esperado Mozilla/5.0", origen.AgenteUsuario())
	}
	if origen.HuellaDispositivo() != "huella-abc" {
		t.Errorf("HuellaDispositivo() = %q, esperado huella-abc", origen.HuellaDispositivo())
	}
	if origen.IDSolicitud() != "req-xyz" {
		t.Errorf("IDSolicitud() = %q, esperado req-xyz", origen.IDSolicitud())
	}
}
