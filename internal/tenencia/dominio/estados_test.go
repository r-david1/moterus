package dominio

import (
	"errors"
	"strings"
	"testing"
)

// --- EstadoOrganizacion ------------------------------------------------------

func TestEstadoOrganizacionDesde(t *testing.T) {
	for _, v := range []string{"activa", "suspendida", "archivada"} {
		if _, err := EstadoOrganizacionDesde(v); err != nil {
			t.Errorf("%q debería ser válido: %v", v, err)
		}
	}
	if _, err := EstadoOrganizacionDesde("inexistente"); err == nil {
		t.Fatal("se esperaba error para un estado fuera del catálogo")
	} else {
		var errEstado *ErrEstadoOrganizacionInvalido
		if !errors.As(err, &errEstado) {
			t.Errorf("se esperaba *ErrEstadoOrganizacionInvalido, obtuvo %T", err)
		}
	}
}

// TestINV_TEN_04_MaquinaEstadosOrganizacion verifica la máquina de estados
// completa de §1.4 del diseño:
//
//	activa      --suspender--> suspendida
//	activa      --archivar---> archivada     (terminal)
//	suspendida  --reactivar--> activa
//	suspendida  --archivar---> archivada     (terminal)
//	archivada   --*----------> ✗ (ninguna)
func TestINV_TEN_04_MaquinaEstadosOrganizacion(t *testing.T) {
	casos := []struct {
		origen, destino EstadoOrganizacion
		permitida       bool
	}{
		{EstadoOrganizacionActiva, EstadoOrganizacionSuspendida, true},
		{EstadoOrganizacionActiva, EstadoOrganizacionArchivada, true},
		{EstadoOrganizacionActiva, EstadoOrganizacionActiva, false},
		{EstadoOrganizacionSuspendida, EstadoOrganizacionActiva, true},
		{EstadoOrganizacionSuspendida, EstadoOrganizacionArchivada, true},
		{EstadoOrganizacionSuspendida, EstadoOrganizacionSuspendida, false},
		{EstadoOrganizacionArchivada, EstadoOrganizacionActiva, false},
		{EstadoOrganizacionArchivada, EstadoOrganizacionSuspendida, false},
		{EstadoOrganizacionArchivada, EstadoOrganizacionArchivada, false},
	}
	for _, c := range casos {
		t.Run(c.origen.String()+"->"+c.destino.String(), func(t *testing.T) {
			got := c.origen.PuedeTransicionarA(c.destino)
			if got != c.permitida {
				t.Errorf("PuedeTransicionarA() = %v, esperado %v", got, c.permitida)
			}
		})
	}
}

func TestEstadoOrganizacion_EsTerminal(t *testing.T) {
	if !EstadoOrganizacionArchivada.EsTerminal() {
		t.Error("archivada debe ser terminal (INV-TEN-04)")
	}
	if EstadoOrganizacionActiva.EsTerminal() || EstadoOrganizacionSuspendida.EsTerminal() {
		t.Error("activa y suspendida no deben ser terminales")
	}
}

// --- EstadoMembresia ----------------------------------------------------------

func TestEstadoMembresiaDesde(t *testing.T) {
	for _, v := range []string{"activa", "suspendida", "removida"} {
		if _, err := EstadoMembresiaDesde(v); err != nil {
			t.Errorf("%q debería ser válido: %v", v, err)
		}
	}
	if _, err := EstadoMembresiaDesde("inexistente"); err == nil {
		t.Fatal("se esperaba error para un estado fuera del catálogo")
	}
}

// TestINV_TEN_08_MaquinaEstadosMembresia verifica la máquina de estados
// completa de §1.4 del diseño: removida es terminal (nunca se borra
// físicamente, INV-TEN-08).
func TestINV_TEN_08_MaquinaEstadosMembresia(t *testing.T) {
	casos := []struct {
		origen, destino EstadoMembresia
		permitida       bool
	}{
		{EstadoMembresiaActiva, EstadoMembresiaSuspendida, true},
		{EstadoMembresiaActiva, EstadoMembresiaRemovida, true},
		{EstadoMembresiaActiva, EstadoMembresiaActiva, false},
		{EstadoMembresiaSuspendida, EstadoMembresiaActiva, true},
		{EstadoMembresiaSuspendida, EstadoMembresiaRemovida, true},
		{EstadoMembresiaRemovida, EstadoMembresiaActiva, false},
		{EstadoMembresiaRemovida, EstadoMembresiaSuspendida, false},
		{EstadoMembresiaRemovida, EstadoMembresiaRemovida, false},
	}
	for _, c := range casos {
		t.Run(c.origen.String()+"->"+c.destino.String(), func(t *testing.T) {
			got := c.origen.PuedeTransicionarA(c.destino)
			if got != c.permitida {
				t.Errorf("PuedeTransicionarA() = %v, esperado %v", got, c.permitida)
			}
		})
	}
}

// --- EstadoInvitacion ---------------------------------------------------------

func TestEstadoInvitacionDesde(t *testing.T) {
	for _, v := range []string{"pendiente", "aceptada", "revocada", "expirada"} {
		if _, err := EstadoInvitacionDesde(v); err != nil {
			t.Errorf("%q debería ser válido: %v", v, err)
		}
	}
	if _, err := EstadoInvitacionDesde("inexistente"); err == nil {
		t.Fatal("se esperaba error para un estado fuera del catálogo")
	}
}

func TestMaquinaEstadosInvitacion(t *testing.T) {
	casos := []struct {
		origen, destino EstadoInvitacion
		permitida       bool
	}{
		{EstadoInvitacionPendiente, EstadoInvitacionAceptada, true},
		{EstadoInvitacionPendiente, EstadoInvitacionRevocada, true},
		{EstadoInvitacionPendiente, EstadoInvitacionExpirada, true},
		{EstadoInvitacionPendiente, EstadoInvitacionPendiente, false},
		{EstadoInvitacionAceptada, EstadoInvitacionRevocada, false},
		{EstadoInvitacionRevocada, EstadoInvitacionAceptada, false},
		{EstadoInvitacionExpirada, EstadoInvitacionAceptada, false},
	}
	for _, c := range casos {
		t.Run(c.origen.String()+"->"+c.destino.String(), func(t *testing.T) {
			got := c.origen.PuedeTransicionarA(c.destino)
			if got != c.permitida {
				t.Errorf("PuedeTransicionarA() = %v, esperado %v", got, c.permitida)
			}
		})
	}
}

func TestEstadoInvitacion_EsTerminal(t *testing.T) {
	if EstadoInvitacionPendiente.EsTerminal() {
		t.Error("pendiente no debe ser terminal")
	}
	for _, e := range []EstadoInvitacion{EstadoInvitacionAceptada, EstadoInvitacionRevocada, EstadoInvitacionExpirada} {
		if !e.EsTerminal() {
			t.Errorf("%s debe ser terminal", e.String())
		}
	}
}

// --- MotivoCambioEstado -------------------------------------------------------

func TestNuevoMotivo(t *testing.T) {
	if _, err := NuevoMotivo(""); err == nil {
		t.Fatal("se esperaba error con motivo vacío")
	}
	if _, err := NuevoMotivo("   "); err == nil {
		t.Fatal("se esperaba error con motivo de solo espacios")
	}
	if _, err := NuevoMotivo(strings.Repeat("a", 281)); err == nil {
		t.Fatal("se esperaba error al superar la longitud máxima")
	}
	m, err := NuevoMotivo("incumplimiento de términos de servicio")
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if m.Valor() != "incumplimiento de términos de servicio" {
		t.Errorf("Valor() = %q", m.Valor())
	}
	if m.EsVacio() {
		t.Error("un motivo construido no debe estar vacío")
	}
}
