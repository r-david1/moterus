package dominio

import (
	"errors"
	"testing"
	"time"
)

func TestNuevaPoliticaOrganizacion(t *testing.T) {
	casos := []struct {
		nombre                         string
		vigenciaInvitacion             time.Duration
		maximoMiembrosActivos          int
		maximoInvitacionesPendientes   int
		maximoOrganizacionesPorUsuario int
		esValida                       bool
	}{
		{"válida por defecto", 7 * 24 * time.Hour, 200, 50, 20, true},
		{"vigencia mínima exacta", time.Hour, 1, 1, 1, true},
		{"vigencia máxima exacta", 30 * 24 * time.Hour, 1, 1, 1, true},
		{"vigencia por debajo del mínimo", 59 * time.Minute, 1, 1, 1, false},
		{"vigencia por encima del máximo", 31 * 24 * time.Hour, 1, 1, 1, false},
		{"maximoMiembrosActivos cero", time.Hour, 0, 1, 1, false},
		{"maximoInvitacionesPendientes cero", time.Hour, 1, 0, 1, false},
		{"maximoOrganizacionesPorUsuario cero", time.Hour, 1, 1, 0, false},
		{"varias violaciones a la vez", 0, 0, 0, 0, false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			p, err := NuevaPoliticaOrganizacion(c.vigenciaInvitacion, c.maximoMiembrosActivos, c.maximoInvitacionesPendientes, c.maximoOrganizacionesPorUsuario)
			if c.esValida {
				if err != nil {
					t.Fatalf("no se esperaba error: %v", err)
				}
				if p.VigenciaInvitacion() != c.vigenciaInvitacion {
					t.Errorf("VigenciaInvitacion() = %v", p.VigenciaInvitacion())
				}
				return
			}
			if err == nil {
				t.Fatal("se esperaba error")
			}
			var errPolitica *ErrPoliticaOrganizacionInvalida
			if !errors.As(err, &errPolitica) {
				t.Errorf("se esperaba *ErrPoliticaOrganizacionInvalida, obtuvo %T", err)
			}
		})
	}
}

func TestPoliticaOrganizacionPorDefecto(t *testing.T) {
	p := PoliticaOrganizacionPorDefecto()
	if p.VigenciaInvitacion() != 7*24*time.Hour {
		t.Errorf("VigenciaInvitacion() = %v, esperado 7 días", p.VigenciaInvitacion())
	}
	if p.MaximoMiembrosActivos() != 200 {
		t.Errorf("MaximoMiembrosActivos() = %d, esperado 200", p.MaximoMiembrosActivos())
	}
	if p.MaximoInvitacionesPendientes() != 50 {
		t.Errorf("MaximoInvitacionesPendientes() = %d, esperado 50", p.MaximoInvitacionesPendientes())
	}
	if p.MaximoOrganizacionesPorUsuario() != 20 {
		t.Errorf("MaximoOrganizacionesPorUsuario() = %d, esperado 20", p.MaximoOrganizacionesPorUsuario())
	}
}

func TestPoliticaOrganizacionPorDefecto_NoEntraEnPanico(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("PoliticaOrganizacionPorDefecto() no debe entrar en pánico: %v", r)
		}
	}()
	PoliticaOrganizacionPorDefecto()
}
