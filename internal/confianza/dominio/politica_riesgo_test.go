package dominio

import (
	"errors"
	"testing"
	"time"
)

func TestPoliticaRiesgoPorDefecto_EsValida(t *testing.T) {
	// PoliticaRiesgoPorDefecto hace panic si sus propios valores no pasan
	// NuevaPoliticaRiesgo; este test es la red de seguridad que el
	// comentario de PoliticaRiesgoPorDefecto promete.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("PoliticaRiesgoPorDefecto no debería entrar en pánico: %v", r)
		}
	}()
	pol := PoliticaRiesgoPorDefecto()
	if pol.EsVacia() {
		t.Fatal("PoliticaRiesgoPorDefecto no debe devolver el cero value")
	}
}

func TestPoliticaRiesgoPorDefecto_ValoresExactosDeLaTablaDelDiseno(t *testing.T) {
	pol := PoliticaRiesgoPorDefecto()

	if pol.MinimoExitosParaJuzgar() != 3 {
		t.Errorf("MinimoExitosParaJuzgar = %d, esperado 3", pol.MinimoExitosParaJuzgar())
	}
	if !pol.Modo().EsIgual(ModoRiesgoObservar) {
		t.Errorf("Modo = %q, esperado observar", pol.Modo().String())
	}
	if pol.VidaPerfil() != 180*24*time.Hour {
		t.Errorf("VidaPerfil = %v, esperado 180 días", pol.VidaPerfil())
	}
	if pol.MaximoOrigenesRecordados() != 20 {
		t.Errorf("MaximoOrigenesRecordados = %d, esperado 20", pol.MaximoOrigenesRecordados())
	}
	if pol.UmbralElevado() != 0.50 {
		t.Errorf("UmbralElevado = %v, esperado 0.50", pol.UmbralElevado())
	}
	if pol.UmbralAlto() != 0.80 {
		t.Errorf("UmbralAlto = %v, esperado 0.80", pol.UmbralAlto())
	}

	// Los pesos no tienen getter público (no hacen falta fuera de Evaluar),
	// así que se verifican indirectamente a través de Evaluar: es la forma
	// correcta de probarlos, porque es la única forma en que el resto del
	// sistema los observa.
	dispositivo := pol.Evaluar([]SenalRiesgo{SenalDispositivoDesconocido})
	if dispositivo.Valor() != 0.40 {
		t.Errorf("peso de dispositivo_desconocido = %v, esperado 0.40", dispositivo.Valor())
	}
	huella := pol.Evaluar([]SenalRiesgo{SenalHuellaAusente})
	if huella.Valor() != 0.40 {
		t.Errorf("peso de huella_ausente = %v, esperado 0.40", huella.Valor())
	}
	red := pol.Evaluar([]SenalRiesgo{SenalRedDesconocida})
	if red.Valor() != 0.25 {
		t.Errorf("peso de red_desconocida = %v, esperado 0.25", red.Valor())
	}
}

func TestNuevaPoliticaRiesgo_RechazaPesosFueraDeRango(t *testing.T) {
	_, err := NuevaPoliticaRiesgo(-0.1, 0.40, 0.25, 0.50, 0.80, 3, ModoRiesgoObservar, 180*24*time.Hour, 20)
	if err == nil {
		t.Fatal("se esperaba error con un peso negativo")
	}
	var errEsperado *ErrPoliticaRiesgoInvalida
	if !errors.As(err, &errEsperado) {
		t.Errorf("tipo de error inesperado: %T", err)
	}

	if _, err := NuevaPoliticaRiesgo(1.1, 0.40, 0.25, 0.50, 0.80, 3, ModoRiesgoObservar, 180*24*time.Hour, 20); err == nil {
		t.Error("se esperaba error con un peso mayor a 1.0")
	}
}

func TestNuevaPoliticaRiesgo_RechazaUmbralElevadoMayorOIgualQueUmbralAlto(t *testing.T) {
	if _, err := NuevaPoliticaRiesgo(0.40, 0.40, 0.25, 0.80, 0.80, 3, ModoRiesgoObservar, 180*24*time.Hour, 20); err == nil {
		t.Error("se esperaba error cuando umbralElevado == umbralAlto")
	}
	if _, err := NuevaPoliticaRiesgo(0.40, 0.40, 0.25, 0.90, 0.80, 3, ModoRiesgoObservar, 180*24*time.Hour, 20); err == nil {
		t.Error("se esperaba error cuando umbralElevado > umbralAlto")
	}
}

func TestNuevaPoliticaRiesgo_RechazaMinimoExitosMenorAUno(t *testing.T) {
	if _, err := NuevaPoliticaRiesgo(0.40, 0.40, 0.25, 0.50, 0.80, 0, ModoRiesgoObservar, 180*24*time.Hour, 20); err == nil {
		t.Error("se esperaba error con minimoExitosParaJuzgar = 0")
	}
}

func TestNuevaPoliticaRiesgo_RechazaModoVacio(t *testing.T) {
	if _, err := NuevaPoliticaRiesgo(0.40, 0.40, 0.25, 0.50, 0.80, 3, ModoRiesgo{}, 180*24*time.Hour, 20); err == nil {
		t.Error("se esperaba error con un ModoRiesgo vacío")
	}
}

func TestNuevaPoliticaRiesgo_RechazaVidaPerfilFueraDeRango(t *testing.T) {
	if _, err := NuevaPoliticaRiesgo(0.40, 0.40, 0.25, 0.50, 0.80, 3, ModoRiesgoObservar, 6*24*time.Hour, 20); err == nil {
		t.Error("se esperaba error con vidaPerfil menor a 7 días")
	}
	if _, err := NuevaPoliticaRiesgo(0.40, 0.40, 0.25, 0.50, 0.80, 3, ModoRiesgoObservar, 3*365*24*time.Hour, 20); err == nil {
		t.Error("se esperaba error con vidaPerfil mayor a 2 años")
	}
}

func TestNuevaPoliticaRiesgo_RechazaMaximoOrigenesRecordadosFueraDeRango(t *testing.T) {
	if _, err := NuevaPoliticaRiesgo(0.40, 0.40, 0.25, 0.50, 0.80, 3, ModoRiesgoObservar, 180*24*time.Hour, 0); err == nil {
		t.Error("se esperaba error con maximoOrigenesRecordados = 0")
	}
	if _, err := NuevaPoliticaRiesgo(0.40, 0.40, 0.25, 0.50, 0.80, 3, ModoRiesgoObservar, 180*24*time.Hour, 101); err == nil {
		t.Error("se esperaba error con maximoOrigenesRecordados = 101")
	}
}

func TestNuevaPoliticaRiesgo_AceptaLosLimitesDeLosRangos(t *testing.T) {
	if _, err := NuevaPoliticaRiesgo(0.0, 1.0, 0.5, 0.0, 1.0, 1, ModoRiesgoObservar, 7*24*time.Hour, 1); err != nil {
		t.Errorf("no se esperaba error en el límite inferior de los rangos: %v", err)
	}
	if _, err := NuevaPoliticaRiesgo(1.0, 0.0, 0.5, 0.99, 1.0, 1, ModoRiesgoExigirCaptcha, 2*365*24*time.Hour, 100); err != nil {
		t.Errorf("no se esperaba error en el límite superior de los rangos: %v", err)
	}
}

// --- ModoRiesgo --------------------------------------------------------------

func TestModoRiesgoDesde_AceptaElCatalogoCerrado(t *testing.T) {
	if m, err := ModoRiesgoDesde("observar"); err != nil || !m.EsIgual(ModoRiesgoObservar) {
		t.Errorf("ModoRiesgoDesde(observar) = %v, %v", m, err)
	}
	if m, err := ModoRiesgoDesde("exigir_captcha"); err != nil || !m.EsIgual(ModoRiesgoExigirCaptcha) {
		t.Errorf("ModoRiesgoDesde(exigir_captcha) = %v, %v", m, err)
	}
}

func TestModoRiesgoDesde_RechazaValoresFueraDelCatalogo(t *testing.T) {
	if _, err := ModoRiesgoDesde("bloquear"); err == nil {
		t.Error("se esperaba error para un modo desconocido")
	}
	if _, err := ModoRiesgoDesde(""); err == nil {
		t.Error("se esperaba error para un modo vacío")
	}
}

func TestModoRiesgo_EsObservar(t *testing.T) {
	if !ModoRiesgoObservar.EsObservar() {
		t.Error("ModoRiesgoObservar.EsObservar() debe ser true")
	}
	if ModoRiesgoExigirCaptcha.EsObservar() {
		t.Error("ModoRiesgoExigirCaptcha.EsObservar() debe ser false")
	}
}

// --- Evaluar -------------------------------------------------------------

func TestPoliticaRiesgo_Evaluar_SumaLosPesosDeLasSenalesPresentes(t *testing.T) {
	pol := PoliticaRiesgoPorDefecto()

	sinSenales := pol.Evaluar(nil)
	if sinSenales.Valor() != 0.0 {
		t.Errorf("Evaluar(nil) = %v, esperado 0.0", sinSenales.Valor())
	}

	dosSenales := pol.Evaluar([]SenalRiesgo{SenalDispositivoDesconocido, SenalRedDesconocida})
	if dosSenales.Valor() != 0.65 {
		t.Errorf("Evaluar(dispositivo+red) = %v, esperado 0.65", dosSenales.Valor())
	}
}

// --- INV-RIES-03 (mitad de dominio) -------------------------------------

// TestINV_RIES_03_CeroSenalesProducePuntajeCeroYNivelNormal verifica la
// mitad de dominio de INV-RIES-03: cualquier error del puerto
// PerfilDeOrigenes (Redis caído, timeout) se resuelve en aplicación
// pasando una lista de señales vacía; el dominio garantiza que, dada esa
// lista vacía, el resultado es siempre PuntajeRiesgo=0.0 y
// NivelRiesgo=normal, nunca fricción. Fail-open incondicional.
func TestINV_RIES_03_CeroSenalesProducePuntajeCeroYNivelNormal(t *testing.T) {
	pol := PoliticaRiesgoPorDefecto()
	puntaje := pol.Evaluar(nil)
	if puntaje.Valor() != 0.0 {
		t.Errorf("Evaluar(nil) = %v, esperado 0.0", puntaje.Valor())
	}
	if !puntaje.Nivel(pol).EsIgual(NivelRiesgoNormal) {
		t.Errorf("Nivel de un puntaje 0.0 = %q, esperado normal", puntaje.Nivel(pol).String())
	}
}

// --- INV-RIES-11 ---------------------------------------------------------

// TestINV_RIES_11_MaximoOrigenesRecordados_EsUnTechoAcotado verifica que
// maximoOrigenesRecordados es una configuración validada y acotada
// ([1, 100], default 20 según §1.5 del diseño): el techo es de memoria y
// retención, no de seguridad, y por eso vive en PoliticaRiesgo como un
// entero simple, no como una decisión de negocio con su propio catálogo.
// La poda real (descartar el de uso más antiguo) ocurre en el script Lua
// del adaptador Redis (fase posterior); aquí solo se garantiza que el
// número que ese script recibe siempre es válido.
func TestINV_RIES_11_MaximoOrigenesRecordados_EsUnTechoAcotado(t *testing.T) {
	pol := PoliticaRiesgoPorDefecto()
	if pol.MaximoOrigenesRecordados() != 20 {
		t.Errorf("MaximoOrigenesRecordados por defecto = %d, esperado 20", pol.MaximoOrigenesRecordados())
	}
	if _, err := NuevaPoliticaRiesgo(0.40, 0.40, 0.25, 0.50, 0.80, 3, ModoRiesgoObservar, 180*24*time.Hour, -1); err == nil {
		t.Error("se esperaba error con maximoOrigenesRecordados negativo")
	}
}

// --- INV-RIES-14 ---------------------------------------------------------

// TestINV_RIES_14_ModoPorDefectoEsObservar verifica que
// PoliticaRiesgoPorDefecto() usa ModoRiesgoObservar: desplegar esta
// extensión no cambia el desenlace de ningún login hasta que alguien
// decida explícitamente pasar a exigir_captcha.
func TestINV_RIES_14_ModoPorDefectoEsObservar(t *testing.T) {
	pol := PoliticaRiesgoPorDefecto()
	if !pol.Modo().EsObservar() {
		t.Errorf("Modo por defecto = %q, esperado observar", pol.Modo().String())
	}
}

func TestPoliticaRiesgo_Evaluar_SaturaAUno(t *testing.T) {
	pol, err := NuevaPoliticaRiesgo(0.9, 0.9, 0.9, 0.50, 0.80, 3, ModoRiesgoObservar, 180*24*time.Hour, 20)
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	p := pol.Evaluar([]SenalRiesgo{SenalDispositivoDesconocido, SenalHuellaAusente, SenalRedDesconocida})
	if p.Valor() != 1.0 {
		t.Errorf("Evaluar con pesos que suman más de 1.0 debe saturar a 1.0, obtuvo %v", p.Valor())
	}
}

// TestINV_RIES_?? este test no representa una invariante numerada por sí
// mismo, pero es requisito explícito de §10 paso 2 del diseño: con los
// pesos por defecto, el máximo alcanzable combinando únicamente las tres
// señales del MVP es 0.65 (dispositivo_desconocido + red_desconocida),
// porque huella_ausente y dispositivo_desconocido son mutuamente
// excluyentes por construcción de PerfilDeOrigen.Senales (nunca se emiten
// juntas: una depende de traeHuella=true y la otra de traeHuella=false). Si
// este test falla es porque alguien cambió los pesos de forma que sí se
// alcanza "alto" sin haber leído la justificación de §1.5.
func TestPoliticaRiesgoPorDefecto_NoAlcanzaNivelAltoConLasTresSenalesDelMVP(t *testing.T) {
	pol := PoliticaRiesgoPorDefecto()

	// Las combinaciones alcanzables de Senales() son subconjuntos de
	// {dispositivo_desconocido XOR huella_ausente} ∪ {red_desconocida}: se
	// enumeran las cuatro combinaciones posibles de "una de las dos
	// mutuamente excluyentes" (o ninguna) cruzadas con "red o no".
	combinaciones := [][]SenalRiesgo{
		{},
		{SenalDispositivoDesconocido},
		{SenalHuellaAusente},
		{SenalRedDesconocida},
		{SenalDispositivoDesconocido, SenalRedDesconocida},
		{SenalHuellaAusente, SenalRedDesconocida},
	}

	var maximo float64
	for _, c := range combinaciones {
		p := pol.Evaluar(c)
		if p.Valor() > maximo {
			maximo = p.Valor()
		}
		if p.Nivel(pol).EsIgual(NivelRiesgoAlto) {
			t.Fatalf("la combinación %v alcanzó el nivel 'alto' (puntaje %v) con las señales del MVP; "+
				"eso solo debería ser posible si se agrega geovelocidad_imposible u otra señal nueva — "+
				"leer §1.5 del diseño antes de tocar los pesos", c, p.Valor())
		}
	}

	if maximo != 0.65 {
		t.Errorf("el puntaje máximo alcanzable con las tres señales del MVP y los pesos por defecto = %v, esperado 0.65", maximo)
	}
	if maximo >= pol.UmbralAlto() {
		t.Errorf("el puntaje máximo alcanzable (%v) no debería llegar a umbralAlto (%v)", maximo, pol.UmbralAlto())
	}
}
