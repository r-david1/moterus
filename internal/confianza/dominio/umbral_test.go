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

// TestINV_BLQ_05_NingunaAccionProduceUnUmbralDeCuentaConEscalado verifica
// ADR 0052: ninguna acción del catálogo cerrado, ni siquiera la rama
// fail-safe de una acción que alguien olvide registrar, puede producir un
// Umbral de Cuenta con BackoffMaximo distinto de su propia Ventana — el
// escalado exponencial solo tiene sentido en la clave de IP, nunca en la
// de cuenta (la posee la potencial víctima, no quien la ataca). El valor
// de esta invariante está en que no se pueda olvidar al agregar una
// acción nueva, así que el test recorre TODAS las acciones reales del
// catálogo más una inexistente, no una muestra.
func TestINV_BLQ_05_NingunaAccionProduceUnUmbralDeCuentaConEscalado(t *testing.T) {
	politica := PoliticaLimitesPorDefecto()
	acciones := []Accion{
		AccionLogin, AccionRegistro, AccionReenvioVerificacion,
		AccionRenovacionSesion, AccionCierreMasivoSesiones,
		AccionCrearOrganizacion, AccionInvitarMiembro, AccionAceptarInvitacion,
		AccionVerificarOTP, AccionIngresoASala,
		Accion("accion_nunca_registrada"), // rama fail-safe de Para()
	}

	for _, accion := range acciones {
		limites := politica.Para(accion)
		if limites.Cuenta.BackoffMaximo != limites.Cuenta.Ventana {
			t.Errorf("%s: Cuenta.BackoffMaximo = %v, esperado exactamente Cuenta.Ventana (%v) — INV-BLQ-05 violada",
				accion, limites.Cuenta.BackoffMaximo, limites.Cuenta.Ventana)
		}
	}
}

// TestINV_BLQ_05_ElNivelIPNoSeNormaliza confirma el otro lado de la misma
// invariante: Para() nunca toca IP.BackoffMaximo, así que el nivel IP
// sigue usando el tope por defecto del adaptador (2h) cuando no se fija
// explícitamente — el escalado ahí está bien dirigido (ADR 0052) y no
// debe acotarse.
func TestINV_BLQ_05_ElNivelIPNoSeNormaliza(t *testing.T) {
	politica := PoliticaLimitesPorDefecto()

	limites := politica.Para(AccionLogin)
	if limites.IP.BackoffMaximo != 0 {
		t.Errorf("IP.BackoffMaximo = %v, esperado 0 (usa el tope por defecto del adaptador, no se acota)", limites.IP.BackoffMaximo)
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
