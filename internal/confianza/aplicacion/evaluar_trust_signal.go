package aplicacion

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/r-david1/moterus/internal/confianza/dominio"
	"github.com/r-david1/moterus/internal/confianza/puertos"
)

// reintentarEnCaptchaPorRiesgo es el Retry-After que acompaña un 429
// riesgo_de_origen_requiere_captcha (§1/§10 del diseño
// fingerprinting-comportamiento.md): no es un cooldown real (resolver un
// captcha es inmediato), solo el margen para que el cliente cargue el
// widget — mismo valor que retryTicketRequeridoSegundos en las colas de
// acceso virtual.
const reintentarEnCaptchaPorRiesgo = 5 * time.Second

// EvaluarTrustSignalCasoDeUso implementa puertos.EvaluadorDeRiesgo: dos
// niveles de rate limiting simultáneos (IP y cuenta, ADR 0018) más
// verificación de captcha invisible, y —desde la extensión de
// reconocimiento de origen, docs/design/fingerprinting-comportamiento.md—
// un paso adicional de heurística de origen conocido. El orden es
// normativo:
//
//  1. Si viene TokenCaptcha, se verifica primero (se necesita el puntaje
//     tanto para decidir un posible bypass del límite por cuenta como
//     para el gate final de puntaje).
//  2. Límite por IP: bloqueo duro, sin bypass por captcha — protege
//     contra floods puramente volumétricos desde un origen, algo que
//     "resolver un captcha" no mitiga (un script puede resolver un
//     captcha una vez y seguir).
//     2.5. Reconocimiento de origen (§3.1 del diseño), SOLO para
//     AccionLogin y solo si c.perfiles != nil: compara el origen de esta
//     solicitud contra el historial de la cuenta y, según c.politicaRiesgo,
//     puede exigir un captcha (nunca un step-up: INV-RIES-02/ADR 0051).
//  3. Límite por cuenta: bloqueo blando — si no hay captcha válido con
//     puntaje aceptable, se deniega pidiendo uno (RequiereCaptcha=true);
//     si sí lo hay, se deja pasar pese al límite (una persona real
//     reintentando su propia cuenta no debe quedar atascada 15 minutos).
//  4. Gate final de puntaje: si se verificó un captcha y el puntaje no es
//     aceptable, se deniega aunque ningún límite se haya superado
//     (defensa en profundidad).
type EvaluarTrustSignalCasoDeUso struct {
	limitador puertos.LimitadorTasa
	captcha   puertos.VerificadorCaptcha
	politica  dominio.PoliticaLimites

	// perfiles, politicaRiesgo, auditoria y reloj son las dependencias de
	// la extensión de reconocimiento de origen (§8 del diseño:
	// "evaluar_trust_signal.go: dos dependencias nuevas nileables
	// (perfiles, auditoria) + politicaRiesgo"). Todas nileables/con
	// zero-value seguro salvo politicaRiesgo, que arranca en
	// dominio.PoliticaRiesgoPorDefecto() en el constructor: con
	// perfiles == nil (el estado por defecto sin REDIS_URL) ninguna de las
	// tres se llega a usar, así que el caso de uso se comporta byte por
	// byte igual que antes de esta extensión.
	perfiles       puertos.PerfilDeOrigenes
	politicaRiesgo dominio.PoliticaRiesgo
	auditoria      puertos.RegistroAuditoria
	reloj          puertos.Reloj
}

var _ puertos.EvaluadorDeRiesgo = (*EvaluarTrustSignalCasoDeUso)(nil)

// OpcionEvaluarTrustSignal configura una dependencia opcional de
// EvaluarTrustSignalCasoDeUso. Se usa el patrón de opciones funcionales,
// variádico y al final de la firma de NuevoEvaluarTrustSignalCasoDeUso,
// deliberadamente para NO romper los call sites existentes (§10 paso 5 del
// diseño: "primero el test de no-regresión... ese orden no es estético: es
// la única forma de saber que no se rompió ADR 0018"). Sin ninguna opción,
// el constructor produce exactamente el mismo caso de uso que producía
// antes de esta extensión.
type OpcionEvaluarTrustSignal func(*EvaluarTrustSignalCasoDeUso)

// ConPerfilesDeOrigen inyecta el puerto de reconocimiento de origen (§2.2
// del diseño). Sin esta opción, c.perfiles queda nil: el paso 2.5 de
// Evaluar y la promoción dentro de RegistrarResultado quedan
// completamente inertes (una comparación de puntero, según el diseño). Un
// adaptador nil es legítimo y significa "la extensión está apagada".
func ConPerfilesDeOrigen(perfiles puertos.PerfilDeOrigenes) OpcionEvaluarTrustSignal {
	return func(c *EvaluarTrustSignalCasoDeUso) { c.perfiles = perfiles }
}

// ConPoliticaRiesgo sustituye dominio.PoliticaRiesgoPorDefecto() (el valor
// con el que arranca el constructor) por la política dada: pesos,
// umbrales, minimoExitosParaJuzgar, modo, vidaPerfil y
// maximoOrigenesRecordados (§1.5 del diseño).
func ConPoliticaRiesgo(politica dominio.PoliticaRiesgo) OpcionEvaluarTrustSignal {
	return func(c *EvaluarTrustSignalCasoDeUso) { c.politicaRiesgo = politica }
}

// ConAuditoriaDeRiesgo inyecta el puerto de auditoría usado
// exclusivamente para el evento OrigenNuevoObservado (§1.6 del diseño).
// Sin esta opción, la promoción de RegistrarResultado sigue reconociendo
// el origen (Consultar/Registrar) pero nunca audita: best-effort, nunca
// aborta el login.
func ConAuditoriaDeRiesgo(auditoria puertos.RegistroAuditoria) OpcionEvaluarTrustSignal {
	return func(c *EvaluarTrustSignalCasoDeUso) { c.auditoria = auditoria }
}

// ConRelojDeRiesgo inyecta el reloj usado para fechar el evento
// OrigenNuevoObservado. El dominio nunca llama a time.Now(); sin esta
// opción, la promoción no audita (no hay con qué fechar el evento).
func ConRelojDeRiesgo(reloj puertos.Reloj) OpcionEvaluarTrustSignal {
	return func(c *EvaluarTrustSignalCasoDeUso) { c.reloj = reloj }
}

// NuevoEvaluarTrustSignalCasoDeUso construye el caso de uso con sus
// dependencias inyectadas por puerto, la política de umbrales por defecto
// (dominio.PoliticaLimitesPorDefecto) y, para la extensión de
// reconocimiento de origen, dominio.PoliticaRiesgoPorDefecto() —
// irrelevante mientras no se pase ConPerfilesDeOrigen, porque el paso 2.5
// y la promoción están guardados por c.perfiles != nil.
func NuevoEvaluarTrustSignalCasoDeUso(limitador puertos.LimitadorTasa, captcha puertos.VerificadorCaptcha, opciones ...OpcionEvaluarTrustSignal) *EvaluarTrustSignalCasoDeUso {
	c := &EvaluarTrustSignalCasoDeUso{
		limitador:      limitador,
		captcha:        captcha,
		politica:       dominio.PoliticaLimitesPorDefecto(),
		politicaRiesgo: dominio.PoliticaRiesgoPorDefecto(),
	}
	for _, opcion := range opciones {
		opcion(c)
	}
	return c
}

// Evaluar ejecuta el flujo descrito en el comentario del tipo.
func (c *EvaluarTrustSignalCasoDeUso) Evaluar(ctx context.Context, s Solicitud) (dominio.Decision, error) {
	limites := c.politica.Para(s.Accion)

	huboToken := s.TokenCaptcha != ""
	puntaje := 1.0 // sin captcha evaluado, no se penaliza el puntaje reportado.
	if huboToken {
		p, err := c.captcha.Verificar(ctx, s.TokenCaptcha, string(s.Accion), s.IPOrigen)
		if err != nil {
			// Fail-closed deliberado (a diferencia de HIBP en Identidad):
			// un captcha que no se pudo verificar se trata como "no
			// verificado" (puntaje 0), no como "ausente" — un atacante no
			// debe poder anular la verificación provocando errores de red
			// contra el proveedor.
			slog.WarnContext(ctx, "confianza: verificación de captcha falló; se trata como puntaje 0 (fail-closed)",
				"error", err, "accion", s.Accion)
			puntaje = 0
		} else {
			puntaje = p
		}
	}

	// 2. Límite por IP: bloqueo duro.
	if s.IPOrigen != "" {
		permitido, _, reintentarEn, err := c.limitador.Permitir(ctx, claveLimite("ip", s.Accion, s.IPOrigen), limites.IP)
		if err != nil {
			// Fail-open: una caída de Redis no puede tumbar login/registro
			// por completo (mismo criterio que VerificadorContrasenasFiltradas
			// en Identidad). Se deja constancia en logs para que sea
			// observable, no silenciosa.
			slog.WarnContext(ctx, "confianza: limitador de tasa por IP no disponible; continuando fail-open",
				"error", err, "accion", s.Accion)
		} else if !permitido {
			return dominio.Decision{
				Permitido:    false,
				Motivo:       "limite_ip_excedido",
				ReintentarEn: reintentarEn,
				Puntaje:      puntaje,
			}, nil
		}
	}

	// 2.5. Reconocimiento de origen (§3.1 del diseño). Se poblan
	// PuntajeRiesgo/NivelRiesgo/SenalesDeRiesgo en TODAS las Decision que
	// Evaluar devuelva desde aquí en adelante, incluso cuando al final se
	// permite: son el insumo del modo observación (§3.3) y nunca cruzan la
	// frontera de contexto (INV-RIES-09). Este paso solo puede agregar un
	// RequiereCaptcha; JAMÁS un RequiereStepUp (INV-RIES-02, ADR 0051).
	senalesDeRiesgo, puntajeRiesgo, nivelRiesgo, corte := c.evaluarRiesgoDeOrigen(ctx, s, huboToken, puntaje)
	if corte != nil {
		return *corte, nil
	}

	// 3. Límite por cuenta: bloqueo blando, con bypass por captcha válido.
	if s.CorreoNormalizado != "" {
		permitido, _, reintentarEn, err := c.limitador.Permitir(ctx, claveLimite("cuenta", s.Accion, s.CorreoNormalizado), limites.Cuenta)
		if err != nil {
			slog.WarnContext(ctx, "confianza: limitador de tasa por cuenta no disponible; continuando fail-open",
				"error", err, "accion", s.Accion)
		} else if !permitido {
			aceptable, _ := dominio.EvaluarPuntajeCaptcha(puntaje)
			if !huboToken || !aceptable {
				motivo := "limite_cuenta_excedido_requiere_captcha"
				if huboToken {
					motivo = "limite_cuenta_excedido_captcha_insuficiente"
				}
				return dominio.Decision{
					Permitido:       false,
					RequiereCaptcha: true,
					Motivo:          motivo,
					ReintentarEn:    reintentarEn,
					Puntaje:         puntaje,
					PuntajeRiesgo:   puntajeRiesgo,
					NivelRiesgo:     nivelRiesgo,
					SenalesDeRiesgo: senalesDeRiesgo,
				}, nil
			}
			// Bypass: humano verificado con buen puntaje pese al cooldown
			// de la cuenta. Cae al gate final de puntaje (paso 4), que ya
			// va a aprobar porque aceptable==true.
		}
	}

	// 4. Gate final de puntaje (defensa en profundidad, independiente de
	// si algún límite se superó).
	if huboToken {
		aceptable, sospechoso := dominio.EvaluarPuntajeCaptcha(puntaje)
		if !aceptable {
			motivo := "captcha_puntaje_bajo"
			if sospechoso {
				motivo = "captcha_puntaje_sospechoso"
			}
			return dominio.Decision{
				Permitido:       false,
				Motivo:          motivo,
				Puntaje:         puntaje,
				PuntajeRiesgo:   puntajeRiesgo,
				NivelRiesgo:     nivelRiesgo,
				SenalesDeRiesgo: senalesDeRiesgo,
			}, nil
		}
	}

	return dominio.Decision{
		Permitido:       true,
		Puntaje:         puntaje,
		PuntajeRiesgo:   puntajeRiesgo,
		NivelRiesgo:     nivelRiesgo,
		SenalesDeRiesgo: senalesDeRiesgo,
	}, nil
}

// evaluarRiesgoDeOrigen implementa el paso 2.5 de Evaluar (§3.1 del diseño
// fingerprinting-comportamiento.md): se inserta entre el límite por IP y
// el límite por cuenta, y solo corre para AccionLogin.
//
// Devuelve las señales/puntaje/nivel calculados (para que Evaluar los
// pueda poblar en cualquier Decision que termine devolviendo, incluso al
// permitir) y, si el modo de la política exige cortar el flujo aquí mismo
// (captcha), una Decision de corte no nil que Evaluar debe devolver de
// inmediato sin seguir a los pasos 3/4.
//
// INV-RIES-02 (ADR 0051), la restricción más importante de este método:
// JAMÁS fija Decision.RequiereStepUp. Su única forma de agregar fricción
// es RequiereCaptcha — un usuario sin MFA que recibiera RequiereStepUp
// quedaría bloqueado sin salida (ver el hallazgo documentado en §3.2 del
// diseño).
func (c *EvaluarTrustSignalCasoDeUso) evaluarRiesgoDeOrigen(ctx context.Context, s Solicitud, huboToken bool, puntajeCaptcha float64) (senales []dominio.SenalRiesgo, riesgo dominio.PuntajeRiesgo, nivel dominio.NivelRiesgo, corte *dominio.Decision) {
	if s.Accion != dominio.AccionLogin || c.perfiles == nil || s.CorreoNormalizado == "" {
		return nil, dominio.PuntajeRiesgo{}, dominio.NivelRiesgo{}, nil
	}

	clave := dominio.NuevaClaveCuenta(s.CorreoNormalizado)

	// Solicitud (puertos) transporta IP y huella como primitivos propios
	// (§2.1 del diseño); se reconstruye aquí un OrigenSolicitud mínimo
	// para poder usar dominio.HuellaDeOrigenDesde. Sin IDSolicitud: este
	// paso no audita nada (solo la promoción en RegistrarResultado lo
	// hace), así que no hace falta correlación forense todavía.
	origen, err := dominio.NuevoOrigenSolicitud(s.IPOrigen, "", s.HuellaDispositivo, "")
	if err != nil {
		// La IP no llegó en un formato reconocible. No es una falla del
		// puerto PerfilDeOrigenes (INV-RIES-03 propiamente dicha), pero se
		// trata con el mismo criterio fail-open: nunca agregar fricción
		// por un dato de entrada que este paso no pudo interpretar.
		slog.WarnContext(ctx, "confianza: origen de la solicitud inválido al evaluar riesgo de origen; se omite esta señal (fail-open)",
			"error", err, "accion", s.Accion)
		riesgo = c.politicaRiesgo.Evaluar(nil)
		nivel = riesgo.Nivel(c.politicaRiesgo)
		return nil, riesgo, nivel, nil
	}
	obs := dominio.HuellaDeOrigenDesde(origen)

	vista, err := c.perfiles.Consultar(ctx, puertos.ConsultaPerfilOrigen{
		Clave:      clave.String(),
		HashHuella: obs.Huella().String(),
		HashRed:    obs.Red().String(),
	})
	if err != nil {
		// INV-RIES-03: fail-open incondicional y NO conmutable. Un fallo
		// de Redis nunca puede agregar fricción a un login legítimo.
		slog.WarnContext(ctx, "confianza: perfil de orígenes no disponible; continuando fail-open sin señales de riesgo",
			"error", err, "accion", s.Accion)
		riesgo = c.politicaRiesgo.Evaluar(nil)
		nivel = riesgo.Nivel(c.politicaRiesgo)
		return nil, riesgo, nivel, nil
	}

	perfil := dominio.NuevoPerfilDeOrigen(clave, vista.Exitos, vista.ExitosConHuella, vista.DispositivoConocido, vista.RedConocida)
	senales = perfil.Senales(obs, c.politicaRiesgo)
	riesgo = c.politicaRiesgo.Evaluar(senales)
	nivel = riesgo.Nivel(c.politicaRiesgo)

	modo := c.politicaRiesgo.Modo()
	switch {
	case modo.EsIgual(dominio.ModoRiesgoExigirCaptcha) && nivel.AlMenos(dominio.NivelRiesgoElevado):
		// Mismo criterio de bypass que el límite por cuenta (paso 3): un
		// captcha ya aceptable en este mismo intento acredita al humano y
		// no hace falta seguir exigiendo.
		aceptable, _ := dominio.EvaluarPuntajeCaptcha(puntajeCaptcha)
		if !huboToken || !aceptable {
			d := dominio.Decision{
				Permitido:       false,
				RequiereCaptcha: true,
				Motivo:          "riesgo_de_origen_requiere_captcha",
				// ReintentarEn no representa un cooldown que haya que
				// esperar (a diferencia de limite_ip_excedido/
				// limite_cuenta_excedido_*, que sí tienen una ventana
				// real): resolver un captcha es inmediato. Se fija un valor
				// corto igual — mismo criterio que
				// retryTicketRequeridoSegundos en las colas de acceso
				// virtual (5s: alcanza para que el cliente cargue el
				// widget) — porque §1/§10 del diseño describe este 429
				// reutilizando la MISMA forma de cuerpo que
				// ErrAccesoDenegadoPorConfianza, "con su Retry-After"; sin
				// este campo, mapearErrorDominio (que solo agrega la
				// cabecera si ReintentarEn > 0) lo omitía, dejando el 429
				// sin Retry-After pese a que el diseño lo pedía.
				ReintentarEn:    reintentarEnCaptchaPorRiesgo,
				Puntaje:         puntajeCaptcha,
				PuntajeRiesgo:   riesgo,
				NivelRiesgo:     nivel,
				SenalesDeRiesgo: senales,
			}
			return senales, riesgo, nivel, &d
		}
		slog.InfoContext(ctx, "confianza: riesgo de origen elevado pero con captcha ya aceptable en este intento; se permite (bypass)",
			"accion", s.Accion, "nivel", nivel.String(), "puntaje", riesgo.Valor(), "senales", codigosDeSenales(senales))
	case len(senales) > 0:
		// modo == observar (o nivel por debajo de elevado con
		// exigir_captcha): nunca cambia el desenlace (§3.3, INV-RIES-14).
		slog.InfoContext(ctx, "confianza: reconocimiento de origen en modo observación; no cambia el desenlace",
			"accion", s.Accion, "nivel", nivel.String(), "puntaje", riesgo.Valor(), "senales", codigosDeSenales(senales), "modo", modo.String())
	}

	return senales, riesgo, nivel, nil
}

// RegistrarResultado resetea el contador de cuenta tras un intento
// exitoso (no penaliza a quien se equivocó una vez y luego acertó). Un
// intento fallido no hace nada adicional aquí: el propio Evaluar ya
// consumió presupuesto de la ventana en la llamada previa (INV-ID-12: se
// evalúa antes de cada intento, éxito o fracaso), así que no hace falta
// una segunda escritura de contador para "fallos" — evita el problema
// clásico de bucket duplicado que se corrige en direcciones opuestas.
//
// Desde la extensión de reconocimiento de origen (§3.4 del diseño),
// también intenta promover el origen de este intento a "conocido" —
// exclusivamente en éxito (INV-RIES-05: un atacante no debe poder
// "calentar" su dispositivo con intentos fallidos).
func (c *EvaluarTrustSignalCasoDeUso) RegistrarResultado(ctx context.Context, r ResultadoIntento) error {
	if !r.Exitoso || r.CorreoNormalizado == "" {
		return nil
	}
	if err := c.limitador.Reiniciar(ctx, claveLimite("cuenta", r.Accion, r.CorreoNormalizado)); err != nil {
		return fmt.Errorf("confianza: no se pudo reiniciar el contador de cuenta tras un intento exitoso: %w", err)
	}

	c.promoverOrigenSiCorresponde(ctx, r)

	return nil
}

// promoverOrigenSiCorresponde implementa el paso nuevo de RegistrarResultado
// (§3.4 del diseño): promueve el origen de un login exitoso a "conocido" y,
// solo si la cuenta ya tenía al menos un origen conocido y el dispositivo
// era nuevo, audita OrigenNuevoObservado (INV-RIES-13). Quien llama ya
// garantizó r.Exitoso && r.CorreoNormalizado != "" (la única condición que
// falta comprobar acá es r.Accion == login && c.perfiles != nil).
//
// Todo lo que hace este método es best-effort: ya corre fuera de la
// transacción de negocio del login (identidad/aplicacion/
// autenticar_usuario.go invoca RegistrarResultado después de la unidad de
// trabajo), así que ningún error de este método puede tumbar un login que
// ya tuvo éxito — se loguea y se sigue.
func (c *EvaluarTrustSignalCasoDeUso) promoverOrigenSiCorresponde(ctx context.Context, r ResultadoIntento) {
	if r.Accion != dominio.AccionLogin || c.perfiles == nil {
		return
	}

	clave := dominio.NuevaClaveCuenta(r.CorreoNormalizado)
	origen, err := dominio.NuevoOrigenSolicitud(r.IPOrigen, "", r.HuellaDispositivo, r.IDSolicitud)
	if err != nil {
		slog.WarnContext(ctx, "confianza: origen inválido al promover el origen observado; se omite (best-effort)",
			"error", err, "accion", r.Accion)
		return
	}
	obs := dominio.HuellaDeOrigenDesde(origen)

	// Consulta previa, best-effort: solo para saber si el dispositivo ya
	// era conocido (y para los detalles de auditoría). Un error acá no
	// impide la promoción, solo impide saber si corresponde auditar.
	vista, err := c.perfiles.Consultar(ctx, puertos.ConsultaPerfilOrigen{
		Clave:      clave.String(),
		HashHuella: obs.Huella().String(),
		HashRed:    obs.Red().String(),
	})
	if err != nil {
		slog.WarnContext(ctx, "confianza: no se pudo consultar el perfil de orígenes antes de promover; se continúa sin auditar (best-effort)",
			"error", err, "accion", r.Accion)
	}

	if err := c.perfiles.Registrar(ctx, puertos.RegistrarOrigenObservado{
		Clave:                    clave.String(),
		HashHuella:               obs.Huella().String(),
		HashRed:                  obs.Red().String(),
		VidaPerfil:               c.politicaRiesgo.VidaPerfil(),
		MaximoOrigenesRecordados: c.politicaRiesgo.MaximoOrigenesRecordados(),
	}); err != nil {
		slog.WarnContext(ctx, "confianza: no se pudo registrar el origen observado tras un login exitoso (best-effort, no aborta el login)",
			"error", err, "accion", r.Accion)
	}

	// INV-RIES-13: se audita un solo hecho, una vez por par (cuenta,
	// dispositivo), y solo si la cuenta ya tenía al menos un origen
	// conocido (así no hay una fila inútil en el primer login de cada
	// usuario).
	if vista.OrigenesConocidos == 0 || vista.DispositivoConocido || !obs.TraeHuella() {
		return
	}
	if c.auditoria == nil || c.reloj == nil {
		return
	}

	perfilPrevio := dominio.NuevoPerfilDeOrigen(clave, vista.Exitos, vista.ExitosConHuella, vista.DispositivoConocido, vista.RedConocida)
	senales := perfilPrevio.Senales(obs, c.politicaRiesgo)
	puntajeRiesgo := c.politicaRiesgo.Evaluar(senales)
	nivelRiesgo := puntajeRiesgo.Nivel(c.politicaRiesgo)

	evento := dominio.NuevoOrigenNuevoObservado(
		r.IDUsuario,
		senales,
		puntajeRiesgo,
		nivelRiesgo,
		c.politicaRiesgo.Modo(),
		vista.OrigenesConocidos,
		c.reloj.Ahora(),
	)
	if err := c.auditoria.Registrar(ctx, evento, origen); err != nil {
		slog.WarnContext(ctx, "confianza: no se pudo auditar origen.nuevo (best-effort, no aborta el login)",
			"error", err, "accion", r.Accion)
	}
}

// codigosDeSenales convierte []dominio.SenalRiesgo a []string para los
// campos estructurados de slog (INV-RIES-12: los logs, igual que la
// auditoría, solo transportan códigos del catálogo cerrado, nunca datos
// personales).
func codigosDeSenales(senales []dominio.SenalRiesgo) []string {
	codigos := make([]string, len(senales))
	for i, s := range senales {
		codigos[i] = s.String()
	}
	return codigos
}

// claveLimite arma la clave de Redis (o de cualquier LimitadorTasa) para
// un nivel (ip|cuenta) + acción + identificador. Con un separador estable
// para que test/carga pueda predecir las claves sin acoplarse a un string
// interno distinto en cada sitio.
func claveLimite(nivel string, accion dominio.Accion, identificador string) string {
	return fmt.Sprintf("confianza:rl:%s:%s:%s", nivel, accion, identificador)
}
