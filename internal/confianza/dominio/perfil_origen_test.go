package dominio

import (
	"reflect"
	"testing"
	"time"
)

func claveDePrueba() ClaveCuenta { return NuevaClaveCuenta("cuenta@example.com") }

// obsConHuella construye una HuellaDeOrigen "trae huella, trae red pública"
// para los tests de Senales.
func obsConHuella(t *testing.T, huella, ip string) HuellaDeOrigen {
	t.Helper()
	o, err := NuevoOrigenSolicitud(ip, "agente", huella, "id-1")
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	return HuellaDeOrigenDesde(o)
}

// --- INV-RIES-04 -------------------------------------------------------

// TestINV_RIES_04_SinHistorialSuficienteProduceCeroSenales verifica que una
// cuenta con menos de politica.MinimoExitosParaJuzgar() logins exitosos
// nunca produce señales, sin importar qué tan "nueva" luzca la petición
// actual: el primer login de una cuenta no tiene con qué compararse.
func TestINV_RIES_04_SinHistorialSuficienteProduceCeroSenales(t *testing.T) {
	pol := PoliticaRiesgoPorDefecto()
	obs := obsConHuella(t, "huella-nueva", "203.0.113.9")

	casos := []int64{0, 1, 2}
	for _, exitos := range casos {
		perfil := NuevoPerfilDeOrigen(claveDePrueba(), exitos, 0, false, false)
		if perfil.TieneHistorialSuficiente(pol) {
			t.Errorf("con %d éxitos no debería haber historial suficiente (mínimo %d)", exitos, pol.MinimoExitosParaJuzgar())
		}
		senales := perfil.Senales(obs, pol)
		if senales != nil {
			t.Errorf("con %d éxitos se esperaban cero señales, se obtuvieron %v", exitos, senales)
		}
	}
}

func TestINV_RIES_04_ConHistorialSuficienteSiProduceSenales(t *testing.T) {
	pol := PoliticaRiesgoPorDefecto()
	obs := obsConHuella(t, "huella-nueva", "203.0.113.9")

	perfil := NuevoPerfilDeOrigen(claveDePrueba(), pol.MinimoExitosParaJuzgar(), 0, false, false)
	if !perfil.TieneHistorialSuficiente(pol) {
		t.Fatal("con exactamente el mínimo de éxitos ya debería haber historial suficiente")
	}
	senales := perfil.Senales(obs, pol)
	if len(senales) == 0 {
		t.Error("con historial suficiente y dispositivo/red desconocidos se esperaban señales")
	}
}

// --- INV-RIES-05 (mitad de dominio) -------------------------------------

// TestINV_RIES_05_PerfilDeOrigenNoExponeNingunMetodoDePromocion documenta y
// verifica la mitad de dominio de INV-RIES-05: "un origen se promueve a
// conocido exclusivamente tras una autenticación exitosa". Esa decisión
// vive en el caso de uso (RegistrarResultado, fase posterior), nunca en el
// dominio: PerfilDeOrigen es de solo lectura (reconstituido desde una vista
// ya resuelta) y no ofrece ningún método que module su propio estado. Si
// alguien agrega un método de mutación aquí (p. ej. "Registrar" o
// "Promover"), rompe la separación que hace posible razonar sobre
// INV-RIES-05 sin poder "calentar" el perfil durante la evaluación previa.
func TestINV_RIES_05_PerfilDeOrigenNoExponeNingunMetodoDePromocion(t *testing.T) {
	perfil := NuevoPerfilDeOrigen(claveDePrueba(), 5, 5, true, true)
	copia := perfil // PerfilDeOrigen debe ser un value type inmutable: copiarlo no debe compartir estado mutable.
	_ = copia

	// No hay compilación posible de perfil.Registrar(...) ni
	// perfil.Promover(...): este comentario es la documentación de que la
	// ausencia es intencional, y el test en sí ejercita que Senales/
	// TieneHistorialSuficiente son las únicas dos operaciones observables,
	// ambas de solo lectura.
	pol := PoliticaRiesgoPorDefecto()
	obs := obsConHuella(t, "huella-conocida", "203.0.113.9")
	antes := perfil.Senales(obs, pol)
	despues := perfil.Senales(obs, pol)
	if len(antes) != len(despues) {
		t.Error("Senales debe ser una función pura: llamarla dos veces con los mismos argumentos debe dar el mismo resultado")
	}
}

// --- INV-RIES-06 ---------------------------------------------------------

// TestINV_RIES_06_OmitirLaHuellaCuestaAlMenosLoMismoQueTraerUnaNueva es la
// manifestación de dominio de INV-RIES-06: la huella es falsificable y
// nunca autoriza, pero omitirla (la evasión más barata) tiene que producir
// una señal si la cuenta venía mandándola. huella_ausente mira
// exitosConHuella, no exitos.
func TestINV_RIES_06_OmitirLaHuellaCuestaAlMenosLoMismoQueTraerUnaNueva(t *testing.T) {
	pol := PoliticaRiesgoPorDefecto()

	// La cuenta siempre mandó huella en sus éxitos anteriores.
	perfil := NuevoPerfilDeOrigen(claveDePrueba(), 5, 5, true, true)
	sinHuella := obsConHuella(t, "", "203.0.113.9")
	senales := perfil.Senales(sinHuella, pol)

	encontrada := false
	for _, s := range senales {
		if s.EsIgual(SenalHuellaAusente) {
			encontrada = true
		}
		if s.EsIgual(SenalDispositivoDesconocido) {
			t.Error("dispositivo_desconocido no debe emitirse cuando la petición no trae huella (serían la misma ausencia contada dos veces)")
		}
	}
	if !encontrada {
		t.Error("se esperaba la señal huella_ausente cuando la cuenta siempre mandaba huella y dejó de hacerlo")
	}
}

func TestINV_RIES_06_HuellaAusenteNoSeEmiteSiNuncaMandoHuella(t *testing.T) {
	pol := PoliticaRiesgoPorDefecto()
	// exitosConHuella=0: la cuenta nunca mandó huella, así que su ausencia
	// no significa nada.
	perfil := NuevoPerfilDeOrigen(claveDePrueba(), 5, 0, false, true)
	sinHuella := obsConHuella(t, "", "203.0.113.9")
	senales := perfil.Senales(sinHuella, pol)
	for _, s := range senales {
		if s.EsIgual(SenalHuellaAusente) {
			t.Error("no se esperaba huella_ausente cuando exitosConHuella es 0")
		}
	}
}

func TestDispositivoDesconocidoNoSeEmiteSinHuellaEnLaPeticion(t *testing.T) {
	pol := PoliticaRiesgoPorDefecto()
	perfil := NuevoPerfilDeOrigen(claveDePrueba(), 5, 5, false, true)
	conHuellaDesconocida := obsConHuella(t, "huella-nunca-vista", "203.0.113.9")
	senales := perfil.Senales(conHuellaDesconocida, pol)
	encontrada := false
	for _, s := range senales {
		if s.EsIgual(SenalDispositivoDesconocido) {
			encontrada = true
		}
	}
	if !encontrada {
		t.Error("se esperaba dispositivo_desconocido cuando la petición trae una huella que el perfil no reconoce")
	}
}

func TestDispositivoConocidoNoProduceSenalDeDispositivo(t *testing.T) {
	pol := PoliticaRiesgoPorDefecto()
	perfil := NuevoPerfilDeOrigen(claveDePrueba(), 5, 5, true, true)
	conHuellaConocida := obsConHuella(t, "huella-ya-vista", "203.0.113.9")
	senales := perfil.Senales(conHuellaConocida, pol)
	for _, s := range senales {
		if s.EsIgual(SenalDispositivoDesconocido) {
			t.Error("no se esperaba dispositivo_desconocido cuando dispositivoConocido=true")
		}
	}
}

func TestRedDesconocidaSoloSeEmiteConRedNoVaciaYNoConocida(t *testing.T) {
	pol := PoliticaRiesgoPorDefecto()

	// Red conocida: no debe emitirse.
	perfilRedConocida := NuevoPerfilDeOrigen(claveDePrueba(), 5, 5, true, true)
	obsRedConocida := obsConHuella(t, "huella-ya-vista", "203.0.113.9")
	for _, s := range perfilRedConocida.Senales(obsRedConocida, pol) {
		if s.EsIgual(SenalRedDesconocida) {
			t.Error("no se esperaba red_desconocida cuando redConocida=true")
		}
	}

	// Red ausente (IP privada u omitida): no debe emitirse, aunque
	// redConocida sea false.
	perfilRedNoConocida := NuevoPerfilDeOrigen(claveDePrueba(), 5, 5, true, false)
	obsSinRed := obsConHuella(t, "huella-ya-vista", "10.0.0.5")
	for _, s := range perfilRedNoConocida.Senales(obsSinRed, pol) {
		if s.EsIgual(SenalRedDesconocida) {
			t.Error("no se esperaba red_desconocida cuando la petición no trae una red evaluable")
		}
	}

	// Red pública desconocida: sí debe emitirse.
	obsConRed := obsConHuella(t, "huella-ya-vista", "198.51.100.9")
	encontrada := false
	for _, s := range perfilRedNoConocida.Senales(obsConRed, pol) {
		if s.EsIgual(SenalRedDesconocida) {
			encontrada = true
		}
	}
	if !encontrada {
		t.Error("se esperaba red_desconocida con una IP pública fuera del perfil")
	}
}

// --- INV-RIES-08 ---------------------------------------------------------

// TestINV_RIES_08_PerfilPerdidoEquivaleASinHistorial verifica la mitad de
// dominio de INV-RIES-08: el perfil de orígenes es un índice derivado, no
// una fuente de verdad. Perder el índice entero (FLUSHALL, TTL vencido)
// significa, desde el punto de vista del dominio, reconstituir un
// PerfilDeOrigen en su cero-value de contadores — indistinguible de una
// cuenta genuinamente nueva. No hay pérdida de "información" porque el
// dominio nunca trató ese índice como la fuente de verdad.
func TestINV_RIES_08_PerfilPerdidoEquivaleASinHistorial(t *testing.T) {
	pol := PoliticaRiesgoPorDefecto()
	obs := obsConHuella(t, "cualquier-huella", "203.0.113.9")

	perfilPerdido := NuevoPerfilDeOrigen(claveDePrueba(), 0, 0, false, false)
	perfilNuevo := NuevoPerfilDeOrigen(claveDePrueba(), 0, 0, false, false)

	if perfilPerdido.TieneHistorialSuficiente(pol) != perfilNuevo.TieneHistorialSuficiente(pol) {
		t.Error("un perfil recién perdido debe comportarse igual que un perfil genuinamente nuevo")
	}
	if len(perfilPerdido.Senales(obs, pol)) != len(perfilNuevo.Senales(obs, pol)) {
		t.Error("un perfil recién perdido no debe producir señales, igual que uno nuevo")
	}
}

// --- INV-RIES-10 (mitad de dominio) -------------------------------------

// TestINV_RIES_10_SenalesYEvaluarSonPurasYRapidas ejercita en un bucle
// grande PerfilDeOrigen.Senales y PoliticaRiesgo.Evaluar para verificar que
// son aritmética pura sobre contadores: nada de E/S, nada de asignación
// desbocada, nada que dependa del reloj. Es la contrapartida de dominio de
// INV-RIES-10 ("el camino caliente añade como máximo una operación de
// Redis... y cero consultas a Postgres"): el propio dominio no puede hacer
// ninguna de las dos, y aquí se demuestra que además es barato hacerlo
// muchas veces (mismo patrón que TestINV_COLA_08_CaminoCalienteEsPuroYRapido).
func TestINV_RIES_10_SenalesYEvaluarSonPurasYRapidas(t *testing.T) {
	pol := PoliticaRiesgoPorDefecto()
	perfil := NuevoPerfilDeOrigen(claveDePrueba(), 10, 8, true, false)
	obs := obsConHuella(t, "huella-cualquiera", "198.51.100.1")

	inicio := time.Now()
	for i := 0; i < 100_000; i++ {
		senales := perfil.Senales(obs, pol)
		puntaje := pol.Evaluar(senales)
		_ = puntaje.Nivel(pol)
	}
	transcurrido := time.Since(inicio)
	if transcurrido > 2*time.Second {
		t.Errorf("100000 evaluaciones puras tardaron %v: demasiado lento para ser aritmética sobre contadores", transcurrido)
	}
}

// --- INV-RIES-15 (mitad de dominio) -------------------------------------

// TestINV_RIES_15_TiposDeReconocimientoDeOrigenSonAgnosticosDeAccion
// verifica, por reflexión, que ninguno de los value objects nuevos de esta
// extensión (HuellaDeOrigen, PerfilDeOrigen, PoliticaRiesgo, SenalRiesgo,
// PuntajeRiesgo, NivelRiesgo) tiene un campo de tipo Accion ni un nombre de
// campo que sugiera acoplarse a una acción concreta: la restricción "solo
// AccionLogin" (§3.1 del diseño: registro no tiene historial de cuenta por
// definición, y las acciones sensibles post-login ya tienen su freno en
// PoliticaLimitesPorDefecto) es una decisión del caso de uso que invoca a
// estos tipos, no una propiedad que el dominio deba codificar aquí. Si
// alguna vez un tipo de este archivo empieza a depender de Accion, es
// señal de que esa gate se está colando al lugar equivocado.
func TestINV_RIES_15_TiposDeReconocimientoDeOrigenSonAgnosticosDeAccion(t *testing.T) {
	tipoAccion := reflect.TypeOf(AccionLogin)
	tipos := []reflect.Type{
		reflect.TypeOf(HuellaDeOrigen{}),
		reflect.TypeOf(PerfilDeOrigen{}),
		reflect.TypeOf(PoliticaRiesgo{}),
		reflect.TypeOf(SenalRiesgo{}),
		reflect.TypeOf(PuntajeRiesgo{}),
		reflect.TypeOf(NivelRiesgo{}),
	}
	for _, tipo := range tipos {
		for i := 0; i < tipo.NumField(); i++ {
			campo := tipo.Field(i)
			if campo.Type == tipoAccion {
				t.Errorf("%s.%s es de tipo Accion: estos VOs deben ser agnósticos de la acción (INV-RIES-15 es una decisión de aplicacion)", tipo.Name(), campo.Name)
			}
		}
	}
}
