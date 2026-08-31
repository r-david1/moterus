package dominio

import (
	"testing"
	"time"
)

func TestPoliticaLimitesPorDefecto_Para_devuelveUmbralesDocumentados(t *testing.T) {
	politica := PoliticaLimitesPorDefecto()

	casos := []struct {
		accion        Accion
		ipLimite      int
		ipVentana     time.Duration
		cuentaLimite  int
		cuentaVentana time.Duration
	}{
		{AccionLogin, 5, time.Minute, 5, 15 * time.Minute},
		{AccionRegistro, 10, time.Minute, 3, 15 * time.Minute},
		{AccionReenvioVerificacion, 5, time.Minute, 3, 15 * time.Minute},
	}

	for _, c := range casos {
		limites := politica.Para(c.accion)
		if limites.IP.Limite != c.ipLimite || limites.IP.Ventana != c.ipVentana {
			t.Errorf("%s: IP = %+v, esperado {%d %s}", c.accion, limites.IP, c.ipLimite, c.ipVentana)
		}
		if limites.Cuenta.Limite != c.cuentaLimite || limites.Cuenta.Ventana != c.cuentaVentana {
			t.Errorf("%s: Cuenta = %+v, esperado {%d %s}", c.accion, limites.Cuenta, c.cuentaLimite, c.cuentaVentana)
		}
	}
}

func TestPoliticaLimitesPorDefecto_Para_accionDesconocidaUsaFailSafe(t *testing.T) {
	politica := PoliticaLimitesPorDefecto()

	limites := politica.Para(Accion("accion_nunca_registrada"))

	if limites.IP.Limite == 0 || limites.Cuenta.Limite == 0 {
		t.Fatalf("una acción no registrada debe seguir teniendo un umbral fail-safe > 0, obtuvo %+v", limites)
	}
}

func TestEvaluarPuntajeCaptcha(t *testing.T) {
	casos := []struct {
		puntaje          float64
		esperaAceptable  bool
		esperaSospechoso bool
	}{
		{1.0, true, false},
		{0.5, true, false},
		{0.49, false, true},
		{0.3, false, true},
		{0.29, false, false},
		{0.0, false, false},
	}

	for _, c := range casos {
		aceptable, sospechoso := EvaluarPuntajeCaptcha(c.puntaje)
		if aceptable != c.esperaAceptable || sospechoso != c.esperaSospechoso {
			t.Errorf("puntaje=%v: aceptable=%v sospechoso=%v, esperado aceptable=%v sospechoso=%v",
				c.puntaje, aceptable, sospechoso, c.esperaAceptable, c.esperaSospechoso)
		}
	}
}
