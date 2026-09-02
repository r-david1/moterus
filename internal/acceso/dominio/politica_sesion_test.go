package dominio

import (
	"errors"
	"testing"
	"time"
)

func TestPoliticaSesionPorDefecto_EsValida(t *testing.T) {
	p := PoliticaSesionPorDefecto()
	if p.VidaTokenAcceso() != 10*time.Minute {
		t.Errorf("VidaTokenAcceso() = %v, esperado 10 minutos", p.VidaTokenAcceso())
	}
	if p.ToleranciaReloj() != 60*time.Second {
		t.Errorf("ToleranciaReloj() = %v, esperado 60 segundos", p.ToleranciaReloj())
	}
	if p.VidaTokenRefresco() != 30*24*time.Hour {
		t.Errorf("VidaTokenRefresco() = %v, esperado 30 días", p.VidaTokenRefresco())
	}
	if p.InactividadMaxima() != 30*24*time.Hour {
		t.Errorf("InactividadMaxima() = %v, esperado 30 días", p.InactividadMaxima())
	}
	if p.VidaAbsolutaSesion() != 90*24*time.Hour {
		t.Errorf("VidaAbsolutaSesion() = %v, esperado 90 días", p.VidaAbsolutaSesion())
	}
	if p.MaximoSesionesActivas() != 10 {
		t.Errorf("MaximoSesionesActivas() = %d, esperado 10", p.MaximoSesionesActivas())
	}
}

// TestNuevaPoliticaSesion_ValidaVidaTokenAcceso verifica el límite [1min,
// 60min] de la tabla 1.3 del diseño.
func TestNuevaPoliticaSesion_ValidaVidaTokenAcceso(t *testing.T) {
	casos := []struct {
		nombre   string
		vida     time.Duration
		esValida bool
	}{
		{"menos de un minuto", 30 * time.Second, false},
		{"exactamente un minuto", time.Minute, true},
		{"quince minutos", 15 * time.Minute, true},
		{"exactamente 60 minutos", 60 * time.Minute, true},
		{"más de 60 minutos", 61 * time.Minute, false},
	}
	for _, c := range casos {
		_, err := NuevaPoliticaSesion(c.vida, 24*time.Hour, 24*time.Hour, 48*time.Hour, time.Minute, 10)
		if c.esValida && err != nil {
			t.Errorf("%s: no se esperaba error, obtuvo %v", c.nombre, err)
		}
		if !c.esValida && err == nil {
			t.Errorf("%s: se esperaba error", c.nombre)
		}
	}
}

func TestNuevaPoliticaSesion_InactividadNoPuedeSuperarVidaAbsoluta(t *testing.T) {
	_, err := NuevaPoliticaSesion(10*time.Minute, 24*time.Hour, 48*time.Hour, 24*time.Hour, time.Minute, 10)
	if err == nil {
		t.Fatal("se esperaba error: inactividadMaxima > vidaAbsolutaSesion")
	}
	var errPolitica *ErrPoliticaSesionInvalida
	if !errors.As(err, &errPolitica) {
		t.Errorf("se esperaba *ErrPoliticaSesionInvalida, obtuvo %T", err)
	}
}

func TestNuevaPoliticaSesion_RefrescoNoPuedeSuperarInactividad(t *testing.T) {
	_, err := NuevaPoliticaSesion(10*time.Minute, 48*time.Hour, 24*time.Hour, 72*time.Hour, time.Minute, 10)
	if err == nil {
		t.Fatal("se esperaba error: vidaTokenRefresco > inactividadMaxima")
	}
}

func TestNuevaPoliticaSesion_RequiereAlMenosUnaSesionActiva(t *testing.T) {
	_, err := NuevaPoliticaSesion(10*time.Minute, 24*time.Hour, 24*time.Hour, 48*time.Hour, time.Minute, 0)
	if err == nil {
		t.Fatal("se esperaba error: maximoSesionesActivas debe ser al menos 1")
	}
}

func TestNuevaPoliticaSesion_ToleranciaRelojNoPuedeSerNegativa(t *testing.T) {
	_, err := NuevaPoliticaSesion(10*time.Minute, 24*time.Hour, 24*time.Hour, 48*time.Hour, -1*time.Second, 10)
	if err == nil {
		t.Fatal("se esperaba error: toleranciaReloj negativa")
	}
}

func TestNuevaPoliticaSesion_VentanasDebenSerPositivas(t *testing.T) {
	if _, err := NuevaPoliticaSesion(10*time.Minute, 24*time.Hour, 0, 48*time.Hour, time.Minute, 10); err == nil {
		t.Error("se esperaba error: inactividadMaxima debe ser positiva")
	}
	if _, err := NuevaPoliticaSesion(10*time.Minute, 24*time.Hour, 24*time.Hour, 0, time.Minute, 10); err == nil {
		t.Error("se esperaba error: vidaAbsolutaSesion debe ser positiva")
	}
	if _, err := NuevaPoliticaSesion(10*time.Minute, 0, 24*time.Hour, 48*time.Hour, time.Minute, 10); err == nil {
		t.Error("se esperaba error: vidaTokenRefresco debe ser positiva")
	}
}

// TestINV_ACC_16_CalcularExpiraInactividad_NuncaSuperaVidaAbsoluta verifica
// el servicio de dominio PoliticaRotacion (§1.4 del diseño): la nueva
// ventana de inactividad nunca empuja la sesión más allá de su vida
// absoluta ya fijada.
func TestINV_ACC_16_CalcularExpiraInactividad_NuncaSuperaVidaAbsoluta(t *testing.T) {
	politica, _ := NuevaPoliticaSesion(10*time.Minute, 24*time.Hour, 30*24*time.Hour, 90*24*time.Hour, time.Minute, 10)
	inicio := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	expiraAbsolutoEn := inicio.Add(90 * 24 * time.Hour)

	// Caso normal: ahora + inactividadMaxima queda muy por debajo de la vida
	// absoluta.
	ahora := inicio.Add(time.Hour)
	got := CalcularExpiraInactividad(ahora, expiraAbsolutoEn, politica)
	esperado := ahora.Add(30 * 24 * time.Hour)
	if !got.Equal(esperado) {
		t.Errorf("caso normal: got = %v, esperado %v", got, esperado)
	}

	// Caso límite: ahora está cerca de expiraAbsolutoEn, así que
	// ahora+inactividadMaxima la superaría; debe acotarse a expiraAbsolutoEn.
	ahoraCercaDelLimite := expiraAbsolutoEn.Add(-time.Hour)
	got = CalcularExpiraInactividad(ahoraCercaDelLimite, expiraAbsolutoEn, politica)
	if !got.Equal(expiraAbsolutoEn) {
		t.Errorf("caso límite: got = %v, esperado expiraAbsolutoEn = %v (INV-ACC-16)", got, expiraAbsolutoEn)
	}
	if got.After(expiraAbsolutoEn) {
		t.Error("CalcularExpiraInactividad nunca debe superar expiraAbsolutoEn (INV-ACC-16)")
	}
}
