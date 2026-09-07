package aplicacion_test

// Tests del paso 2.5 de EvaluarTrustSignalCasoDeUso.Evaluar, de la
// promoción dentro de RegistrarResultado y de OlvidarPerfilDeOrigenCasoDeUso
// (extensión de reconocimiento de origen,
// docs/design/fingerprinting-comportamiento.md). Los 8 tests preexistentes
// de evaluar_trust_signal_test.go NO se tocan: son el propio test de
// no-regresión de esta extensión (§10 paso 5 del diseño) y siguen pasando
// exactamente igual porque perfiles/auditoria/reloj quedan nil en ellos.

import (
	"context"
	"testing"
	"time"

	"github.com/r-david1/moterus/internal/confianza/aplicacion"
	"github.com/r-david1/moterus/internal/confianza/dominio"
	"github.com/r-david1/moterus/internal/confianza/puertos"
	"github.com/r-david1/moterus/internal/confianza/puertos/mocks"
)

// politicaRiesgoDePrueba construye una PoliticaRiesgo válida con los
// parámetros dados y el resto en los valores por defecto de §1.5 del
// diseño, para que cada test solo tenga que declarar lo que le importa.
func politicaRiesgoDePrueba(t *testing.T, pesoDispositivo, pesoHuella, pesoRed, umbralElevado, umbralAlto float64, minimoExitos int64, modo dominio.ModoRiesgo) dominio.PoliticaRiesgo {
	t.Helper()
	p, err := dominio.NuevaPoliticaRiesgo(
		pesoDispositivo, pesoHuella, pesoRed,
		umbralElevado, umbralAlto,
		minimoExitos,
		modo,
		180*24*time.Hour,
		20,
	)
	if err != nil {
		t.Fatalf("política de riesgo de prueba inválida: %v", err)
	}
	return p
}

// TestEvaluarRiesgoDeOrigen_INV_RIES_02_NuncaProduceRequiereStepUp es el
// test más importante de todos: ni siquiera con un nivel de riesgo "alto"
// forzado por la política, el paso 2.5 puede fijar Decision.RequiereStepUp
// (INV-RIES-02, ADR 0051). Su única forma de agregar fricción es
// RequiereCaptcha.
func TestEvaluarRiesgoDeOrigen_INV_RIES_02_NuncaProduceRequiereStepUp(t *testing.T) {
	// Un solo peso ya alcanza "alto": pesoDispositivoDesconocido=1.0 con
	// umbralAlto=0.2 fuerza NivelRiesgoAlto con una sola señal.
	politica := politicaRiesgoDePrueba(t, 1.0, 0.0, 0.0, 0.1, 0.2, 1, dominio.ModoRiesgoExigirCaptcha)

	perfiles := &mocks.PerfilDeOrigenes{
		FnConsultar: func(ctx context.Context, q puertos.ConsultaPerfilOrigen) (puertos.VistaPerfilOrigen, error) {
			return puertos.VistaPerfilOrigen{
				Exitos:              5,
				ExitosConHuella:     5,
				DispositivoConocido: false,
				RedConocida:         true,
			}, nil
		},
	}

	caso := aplicacion.NuevoEvaluarTrustSignalCasoDeUso(
		&mocks.LimitadorTasa{},
		&mocks.VerificadorCaptcha{},
		aplicacion.ConPerfilesDeOrigen(perfiles),
		aplicacion.ConPoliticaRiesgo(politica),
	)

	decision, err := caso.Evaluar(context.Background(), aplicacion.Solicitud{
		Accion:            dominio.AccionLogin,
		IPOrigen:          "203.0.113.1",
		CorreoNormalizado: "ana@ejemplo.com",
		HuellaDispositivo: "huella-nueva",
	})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	if decision.RequiereStepUp {
		t.Fatalf("INV-RIES-02: el reconocimiento de origen JAMÁS debe producir RequiereStepUp, obtuvo %+v", decision)
	}
	if !decision.NivelRiesgo.EsIgual(dominio.NivelRiesgoAlto) {
		t.Fatalf("se esperaba NivelRiesgo=alto para que el test sea significativo, obtuvo %q", decision.NivelRiesgo.String())
	}
	if decision.Permitido {
		t.Fatalf("se esperaba Permitido=false (exigir_captcha + nivel alto + sin captcha), obtuvo %+v", decision)
	}
	if !decision.RequiereCaptcha {
		t.Fatalf("se esperaba RequiereCaptcha=true, obtuvo %+v", decision)
	}
	if decision.Motivo != "riesgo_de_origen_requiere_captcha" {
		t.Fatalf("motivo = %q, esperado riesgo_de_origen_requiere_captcha", decision.Motivo)
	}
}

// TestEvaluarRiesgoDeOrigen_INV_RIES_03_FailOpenSiConsultarFalla verifica
// que un error del puerto PerfilDeOrigenes nunca deniega ni agrega
// fricción: cero señales, PuntajeRiesgo=0, NivelRiesgo=normal.
func TestEvaluarRiesgoDeOrigen_INV_RIES_03_FailOpenSiConsultarFalla(t *testing.T) {
	politica := politicaRiesgoDePrueba(t, 0.40, 0.40, 0.25, 0.50, 0.80, 3, dominio.ModoRiesgoExigirCaptcha)

	perfiles := &mocks.PerfilDeOrigenes{
		FnConsultar: func(ctx context.Context, q puertos.ConsultaPerfilOrigen) (puertos.VistaPerfilOrigen, error) {
			return puertos.VistaPerfilOrigen{}, context.DeadlineExceeded
		},
	}

	caso := aplicacion.NuevoEvaluarTrustSignalCasoDeUso(
		&mocks.LimitadorTasa{},
		&mocks.VerificadorCaptcha{},
		aplicacion.ConPerfilesDeOrigen(perfiles),
		aplicacion.ConPoliticaRiesgo(politica),
	)

	decision, err := caso.Evaluar(context.Background(), aplicacion.Solicitud{
		Accion:            dominio.AccionLogin,
		IPOrigen:          "203.0.113.1",
		CorreoNormalizado: "ana@ejemplo.com",
		HuellaDispositivo: "huella-cualquiera",
	})
	if err != nil {
		t.Fatalf("INV-RIES-03: un error del puerto nunca debe propagarse, obtuvo %v", err)
	}
	if !decision.Permitido {
		t.Fatalf("INV-RIES-03: un fallo de Redis nunca puede agregar fricción, obtuvo %+v", decision)
	}
	if decision.RequiereCaptcha {
		t.Fatalf("INV-RIES-03: no debe exigirse captcha por un fallo del puerto, obtuvo %+v", decision)
	}
	if decision.PuntajeRiesgo.Valor() != 0 {
		t.Fatalf("INV-RIES-03: PuntajeRiesgo debe ser 0, obtuvo %v", decision.PuntajeRiesgo.Valor())
	}
	if !decision.NivelRiesgo.EsIgual(dominio.NivelRiesgoNormal) {
		t.Fatalf("INV-RIES-03: NivelRiesgo debe ser normal, obtuvo %q", decision.NivelRiesgo.String())
	}
}

// TestEvaluarRiesgoDeOrigen_INV_RIES_04_SinHistorialSuficiente_NoCambiaNada
// verifica que una cuenta por debajo de minimoExitosParaJuzgar produce cero
// señales y no cambia el desenlace, incluso en modo exigir_captcha con un
// dispositivo nunca visto.
func TestEvaluarRiesgoDeOrigen_INV_RIES_04_SinHistorialSuficiente_NoCambiaNada(t *testing.T) {
	politica := politicaRiesgoDePrueba(t, 0.40, 0.40, 0.25, 0.50, 0.80, 3, dominio.ModoRiesgoExigirCaptcha)

	perfiles := &mocks.PerfilDeOrigenes{
		FnConsultar: func(ctx context.Context, q puertos.ConsultaPerfilOrigen) (puertos.VistaPerfilOrigen, error) {
			return puertos.VistaPerfilOrigen{
				Exitos:              2, // < minimoExitosParaJuzgar (3)
				ExitosConHuella:     2,
				DispositivoConocido: false,
				RedConocida:         false,
			}, nil
		},
	}

	caso := aplicacion.NuevoEvaluarTrustSignalCasoDeUso(
		&mocks.LimitadorTasa{},
		&mocks.VerificadorCaptcha{},
		aplicacion.ConPerfilesDeOrigen(perfiles),
		aplicacion.ConPoliticaRiesgo(politica),
	)

	decision, err := caso.Evaluar(context.Background(), aplicacion.Solicitud{
		Accion:            dominio.AccionLogin,
		IPOrigen:          "203.0.113.1",
		CorreoNormalizado: "cuenta-nueva@ejemplo.com",
		HuellaDispositivo: "huella-nunca-vista",
	})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if !decision.Permitido {
		t.Fatalf("INV-RIES-04: una cuenta sin historial suficiente nunca debe verse penalizada, obtuvo %+v", decision)
	}
	if decision.RequiereCaptcha {
		t.Fatalf("INV-RIES-04: no debe exigirse captcha sin historial suficiente, obtuvo %+v", decision)
	}
	if len(decision.SenalesDeRiesgo) != 0 {
		t.Fatalf("INV-RIES-04: se esperaban cero señales, obtuvo %v", decision.SenalesDeRiesgo)
	}
}

// TestEvaluarRiesgoDeOrigen_ModoObservar_NuncaCambiaElDesenlace verifica
// INV-RIES-14: en modo observar, aunque el nivel de riesgo sea alto, la
// Decision se sigue permitiendo (las señales solo quedan pobladas para
// observación).
func TestEvaluarRiesgoDeOrigen_ModoObservar_NuncaCambiaElDesenlace(t *testing.T) {
	politica := politicaRiesgoDePrueba(t, 1.0, 0.0, 0.0, 0.1, 0.2, 1, dominio.ModoRiesgoObservar)

	perfiles := &mocks.PerfilDeOrigenes{
		FnConsultar: func(ctx context.Context, q puertos.ConsultaPerfilOrigen) (puertos.VistaPerfilOrigen, error) {
			return puertos.VistaPerfilOrigen{
				Exitos:              5,
				ExitosConHuella:     5,
				DispositivoConocido: false,
				RedConocida:         true,
			}, nil
		},
	}

	caso := aplicacion.NuevoEvaluarTrustSignalCasoDeUso(
		&mocks.LimitadorTasa{},
		&mocks.VerificadorCaptcha{},
		aplicacion.ConPerfilesDeOrigen(perfiles),
		aplicacion.ConPoliticaRiesgo(politica),
	)

	decision, err := caso.Evaluar(context.Background(), aplicacion.Solicitud{
		Accion:            dominio.AccionLogin,
		IPOrigen:          "203.0.113.1",
		CorreoNormalizado: "ana@ejemplo.com",
		HuellaDispositivo: "huella-nueva",
	})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if !decision.Permitido {
		t.Fatalf("INV-RIES-14: el modo observar nunca debe cambiar el desenlace, obtuvo %+v", decision)
	}
	if decision.RequiereCaptcha {
		t.Fatalf("INV-RIES-14: el modo observar nunca debe exigir captcha, obtuvo %+v", decision)
	}
	if !decision.NivelRiesgo.EsIgual(dominio.NivelRiesgoAlto) {
		t.Fatalf("se esperaba NivelRiesgo=alto (poblado igual, para observación), obtuvo %q", decision.NivelRiesgo.String())
	}
}

// TestEvaluarRiesgoDeOrigen_BypassConCaptchaYaAceptable verifica el bypass
// documentado en §3.1.e: con modo exigir_captcha y nivel >= elevado, si la
// solicitud ya trae un token de captcha con puntaje aceptable, no se
// deniega (el humano ya se acreditó en este mismo intento).
func TestEvaluarRiesgoDeOrigen_BypassConCaptchaYaAceptable(t *testing.T) {
	politica := politicaRiesgoDePrueba(t, 1.0, 0.0, 0.0, 0.1, 0.9, 1, dominio.ModoRiesgoExigirCaptcha)

	perfiles := &mocks.PerfilDeOrigenes{
		FnConsultar: func(ctx context.Context, q puertos.ConsultaPerfilOrigen) (puertos.VistaPerfilOrigen, error) {
			return puertos.VistaPerfilOrigen{
				Exitos:              5,
				ExitosConHuella:     5,
				DispositivoConocido: false,
				RedConocida:         true,
			}, nil
		},
	}
	captcha := &mocks.VerificadorCaptcha{FnVerificar: func(ctx context.Context, token, accion, ip string) (float64, error) {
		return 1.0, nil // puntaje aceptable
	}}

	caso := aplicacion.NuevoEvaluarTrustSignalCasoDeUso(
		&mocks.LimitadorTasa{},
		captcha,
		aplicacion.ConPerfilesDeOrigen(perfiles),
		aplicacion.ConPoliticaRiesgo(politica),
	)

	decision, err := caso.Evaluar(context.Background(), aplicacion.Solicitud{
		Accion:            dominio.AccionLogin,
		IPOrigen:          "203.0.113.1",
		CorreoNormalizado: "ana@ejemplo.com",
		HuellaDispositivo: "huella-nueva",
		TokenCaptcha:      "token-valido",
	})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if !decision.Permitido {
		t.Fatalf("un captcha ya aceptable en este intento debe levantar la exigencia de riesgo de origen, obtuvo %+v", decision)
	}
	if decision.RequiereCaptcha {
		t.Fatalf("no debe volver a exigirse captcha si ya se presentó uno aceptable, obtuvo %+v", decision)
	}
}

// TestEvaluarRiesgoDeOrigen_SoloAplicaAAccionLogin verifica INV-RIES-15: la
// evaluación de riesgo de origen no corre para otras acciones (p. ej.
// registro), aunque perfiles esté configurado.
func TestEvaluarRiesgoDeOrigen_SoloAplicaAAccionLogin(t *testing.T) {
	politica := politicaRiesgoDePrueba(t, 1.0, 0.0, 0.0, 0.1, 0.2, 1, dominio.ModoRiesgoExigirCaptcha)
	perfiles := &mocks.PerfilDeOrigenes{}

	caso := aplicacion.NuevoEvaluarTrustSignalCasoDeUso(
		&mocks.LimitadorTasa{},
		&mocks.VerificadorCaptcha{},
		aplicacion.ConPerfilesDeOrigen(perfiles),
		aplicacion.ConPoliticaRiesgo(politica),
	)

	decision, err := caso.Evaluar(context.Background(), aplicacion.Solicitud{
		Accion:            dominio.AccionRegistro,
		IPOrigen:          "203.0.113.1",
		CorreoNormalizado: "ana@ejemplo.com",
		HuellaDispositivo: "huella-nueva",
	})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if !decision.Permitido {
		t.Fatalf("INV-RIES-15: el registro no debe evaluarse por riesgo de origen, obtuvo %+v", decision)
	}
	if len(perfiles.LlamadasConsultar) != 0 {
		t.Fatalf("INV-RIES-15: no debe consultarse el perfil de orígenes fuera de AccionLogin, hubo %d llamadas", len(perfiles.LlamadasConsultar))
	}
}

// TestRegistrarResultado_INV_RIES_05_IntentoFallidoNuncaPromueve verifica
// que un intento fallido jamás llega a promover un origen, ni siquiera si
// perfiles está configurado (el guard de RegistrarResultado corta antes).
func TestRegistrarResultado_INV_RIES_05_IntentoFallidoNuncaPromueve(t *testing.T) {
	perfiles := &mocks.PerfilDeOrigenes{}
	caso := aplicacion.NuevoEvaluarTrustSignalCasoDeUso(
		&mocks.LimitadorTasa{},
		&mocks.VerificadorCaptcha{},
		aplicacion.ConPerfilesDeOrigen(perfiles),
	)

	if err := caso.RegistrarResultado(context.Background(), aplicacion.ResultadoIntento{
		Accion:            dominio.AccionLogin,
		CorreoNormalizado: "ana@ejemplo.com",
		HuellaDispositivo: "huella-atacante",
		Exitoso:           false,
	}); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if len(perfiles.LlamadasRegistrar) != 0 {
		t.Fatalf("INV-RIES-05: un intento fallido NUNCA debe promover el origen, hubo %v", perfiles.LlamadasRegistrar)
	}
	if len(perfiles.LlamadasConsultar) != 0 {
		t.Fatalf("INV-RIES-05: un intento fallido no debe ni siquiera consultar el perfil, hubo %v", perfiles.LlamadasConsultar)
	}
}

// TestRegistrarResultado_PromueveOrigenTrasExito verifica el camino feliz
// de §3.4: un login exitoso promueve el origen con los hashes y el TTL de
// la política, best-effort.
func TestRegistrarResultado_PromueveOrigenTrasExito(t *testing.T) {
	politica := politicaRiesgoDePrueba(t, 0.40, 0.40, 0.25, 0.50, 0.80, 3, dominio.ModoRiesgoObservar)
	perfiles := &mocks.PerfilDeOrigenes{
		FnConsultar: func(ctx context.Context, q puertos.ConsultaPerfilOrigen) (puertos.VistaPerfilOrigen, error) {
			return puertos.VistaPerfilOrigen{OrigenesConocidos: 0}, nil
		},
	}

	caso := aplicacion.NuevoEvaluarTrustSignalCasoDeUso(
		&mocks.LimitadorTasa{},
		&mocks.VerificadorCaptcha{},
		aplicacion.ConPerfilesDeOrigen(perfiles),
		aplicacion.ConPoliticaRiesgo(politica),
	)

	if err := caso.RegistrarResultado(context.Background(), aplicacion.ResultadoIntento{
		Accion:            dominio.AccionLogin,
		CorreoNormalizado: "ana@ejemplo.com",
		IPOrigen:          "203.0.113.1",
		HuellaDispositivo: "huella-de-ana",
		Exitoso:           true,
	}); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	if len(perfiles.LlamadasRegistrar) != 1 {
		t.Fatalf("se esperaba una promoción, hubo %d", len(perfiles.LlamadasRegistrar))
	}
	cmd := perfiles.LlamadasRegistrar[0]
	if cmd.HashHuella == "" {
		t.Fatalf("se esperaba HashHuella poblado, obtuvo %+v", cmd)
	}
	if cmd.VidaPerfil != politica.VidaPerfil() || cmd.MaximoOrigenesRecordados != politica.MaximoOrigenesRecordados() {
		t.Fatalf("VidaPerfil/MaximoOrigenesRecordados no coinciden con la política, obtuvo %+v", cmd)
	}
}

// TestRegistrarResultado_AuditaOrigenNuevoSoloSiYaHabiaOrigenConocido
// verifica INV-RIES-13: se audita OrigenNuevoObservado únicamente cuando
// la cuenta ya tenía al menos un origen conocido y el dispositivo de este
// intento no era uno de ellos.
func TestRegistrarResultado_AuditaOrigenNuevoSoloSiYaHabiaOrigenConocido(t *testing.T) {
	politica := politicaRiesgoDePrueba(t, 0.40, 0.40, 0.25, 0.50, 0.80, 3, dominio.ModoRiesgoObservar)
	perfiles := &mocks.PerfilDeOrigenes{
		FnConsultar: func(ctx context.Context, q puertos.ConsultaPerfilOrigen) (puertos.VistaPerfilOrigen, error) {
			return puertos.VistaPerfilOrigen{
				Exitos:              4,
				ExitosConHuella:     4,
				DispositivoConocido: false,
				RedConocida:         true,
				OrigenesConocidos:   2,
			}, nil
		},
	}
	auditoria := &mocks.RegistroAuditoria{}
	reloj := &mocks.Reloj{Fija: ahoraDePrueba()}

	caso := aplicacion.NuevoEvaluarTrustSignalCasoDeUso(
		&mocks.LimitadorTasa{},
		&mocks.VerificadorCaptcha{},
		aplicacion.ConPerfilesDeOrigen(perfiles),
		aplicacion.ConPoliticaRiesgo(politica),
		aplicacion.ConAuditoriaDeRiesgo(auditoria),
		aplicacion.ConRelojDeRiesgo(reloj),
	)

	if err := caso.RegistrarResultado(context.Background(), aplicacion.ResultadoIntento{
		Accion:            dominio.AccionLogin,
		CorreoNormalizado: "ana@ejemplo.com",
		IPOrigen:          "203.0.113.1",
		HuellaDispositivo: "huella-nueva-de-ana",
		IDUsuario:         idUsuarioValido1,
		IDSolicitud:       "req-1",
		Exitoso:           true,
	}); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	if len(auditoria.LlamadasRegistrar) != 1 {
		t.Fatalf("se esperaba una fila de auditoría origen.nuevo, hubo %d", len(auditoria.LlamadasRegistrar))
	}
	evento, ok := auditoria.LlamadasRegistrar[0].Evento.(dominio.OrigenNuevoObservado)
	if !ok {
		t.Fatalf("se esperaba un evento OrigenNuevoObservado, obtuvo %T", auditoria.LlamadasRegistrar[0].Evento)
	}
	if evento.IDUsuario != idUsuarioValido1 {
		t.Fatalf("IDUsuario = %q, esperado %q", evento.IDUsuario, idUsuarioValido1)
	}
	if evento.OrigenesConocidos != 2 {
		t.Fatalf("OrigenesConocidos = %d, esperado 2", evento.OrigenesConocidos)
	}
}

// TestRegistrarResultado_NoAuditaElPrimerOrigenDeUnaCuenta verifica la
// segunda mitad de INV-RIES-13: si la cuenta todavía no tenía ningún
// origen conocido (primer login exitoso), no se audita nada.
func TestRegistrarResultado_NoAuditaElPrimerOrigenDeUnaCuenta(t *testing.T) {
	perfiles := &mocks.PerfilDeOrigenes{
		FnConsultar: func(ctx context.Context, q puertos.ConsultaPerfilOrigen) (puertos.VistaPerfilOrigen, error) {
			return puertos.VistaPerfilOrigen{OrigenesConocidos: 0}, nil
		},
	}
	auditoria := &mocks.RegistroAuditoria{}
	reloj := &mocks.Reloj{Fija: ahoraDePrueba()}

	caso := aplicacion.NuevoEvaluarTrustSignalCasoDeUso(
		&mocks.LimitadorTasa{},
		&mocks.VerificadorCaptcha{},
		aplicacion.ConPerfilesDeOrigen(perfiles),
		aplicacion.ConAuditoriaDeRiesgo(auditoria),
		aplicacion.ConRelojDeRiesgo(reloj),
	)

	if err := caso.RegistrarResultado(context.Background(), aplicacion.ResultadoIntento{
		Accion:            dominio.AccionLogin,
		CorreoNormalizado: "usuario-nuevo@ejemplo.com",
		HuellaDispositivo: "primera-huella",
		Exitoso:           true,
	}); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if len(auditoria.LlamadasRegistrar) != 0 {
		t.Fatalf("INV-RIES-13: no debe auditarse el primer origen de una cuenta, hubo %v", auditoria.LlamadasRegistrar)
	}
}

// TestOlvidarPerfilDeOrigen_DelegaEnElPuertoConLaClaveDerivada verifica
// que el caso de uso deriva la ClaveCuenta del correo y delega en
// PerfilDeOrigenes.Olvidar (§3.5 del diseño); nunca pasa el correo en
// claro (INV-RIES-07).
func TestOlvidarPerfilDeOrigen_DelegaEnElPuertoConLaClaveDerivada(t *testing.T) {
	perfiles := &mocks.PerfilDeOrigenes{}
	caso := aplicacion.NuevoOlvidarPerfilDeOrigenCasoDeUso(perfiles)

	correo := "ana@ejemplo.com"
	if err := caso.Olvidar(context.Background(), aplicacion.ComandoOlvidarPerfilDeOrigen{CorreoNormalizado: correo}); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	if len(perfiles.LlamadasOlvidar) != 1 {
		t.Fatalf("se esperaba una llamada a Olvidar, hubo %d", len(perfiles.LlamadasOlvidar))
	}
	claveEsperada := dominio.NuevaClaveCuenta(correo).String()
	if perfiles.LlamadasOlvidar[0] != claveEsperada {
		t.Fatalf("clave = %q, esperado %q", perfiles.LlamadasOlvidar[0], claveEsperada)
	}
	if perfiles.LlamadasOlvidar[0] == correo {
		t.Fatalf("INV-RIES-07: el correo en claro nunca debe llegar al puerto")
	}
}

// TestOlvidarPerfilDeOrigen_PropagaElErrorDelPuerto verifica que un error
// del puerto se propaga tal cual (no hay lógica de negocio adicional que
// enmascararlo).
func TestOlvidarPerfilDeOrigen_PropagaElErrorDelPuerto(t *testing.T) {
	perfiles := &mocks.PerfilDeOrigenes{
		FnOlvidar: func(ctx context.Context, clave string) error {
			return context.DeadlineExceeded
		},
	}
	caso := aplicacion.NuevoOlvidarPerfilDeOrigenCasoDeUso(perfiles)

	err := caso.Olvidar(context.Background(), aplicacion.ComandoOlvidarPerfilDeOrigen{CorreoNormalizado: "ana@ejemplo.com"})
	if err == nil {
		t.Fatalf("se esperaba que el error del puerto se propagara")
	}
}
