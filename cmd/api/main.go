// Comando api arranca el servidor HTTP de Auth-as-a-Service. Además del
// health check de infraestructura, monta el bounded context Identidad
// completo (pool pgx, adaptadores concretos, sus cinco casos de uso y sus
// cinco endpoints HTTP) y el bounded context Acceso completo (ADR 0019/
// 0020: sesión autoritativa en Postgres, JWT de acceso EdDSA/Ed25519 de
// vida corta, refresco opaco rotatorio, JWKS público), que es quien cierra
// el hueco de autenticación que Identidad dejó documentado como
// placeholder en GET /identidad/usuarios/{id}.
package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	accesocripto "github.com/r-david1/moterus/internal/acceso/adaptadores/cripto"
	accesodominio "github.com/r-david1/moterus/internal/acceso/dominio"

	accesoauditoria "github.com/r-david1/moterus/internal/acceso/adaptadores/auditoria"
	accesoconfianza "github.com/r-david1/moterus/internal/acceso/adaptadores/confianza"
	accesoeventos "github.com/r-david1/moterus/internal/acceso/adaptadores/eventos"
	accesohttp "github.com/r-david1/moterus/internal/acceso/adaptadores/http"
	accesoidentidad "github.com/r-david1/moterus/internal/acceso/adaptadores/identidad"
	accesojwt "github.com/r-david1/moterus/internal/acceso/adaptadores/jwt"
	accesopostgres "github.com/r-david1/moterus/internal/acceso/adaptadores/postgres"
	accesoredis "github.com/r-david1/moterus/internal/acceso/adaptadores/redis"
	accesoaplicacion "github.com/r-david1/moterus/internal/acceso/aplicacion"
	accesopuertos "github.com/r-david1/moterus/internal/acceso/puertos"

	confianzaauditoria "github.com/r-david1/moterus/internal/confianza/adaptadores/auditoria"
	confianzacripto "github.com/r-david1/moterus/internal/confianza/adaptadores/cripto"
	confianzahttp "github.com/r-david1/moterus/internal/confianza/adaptadores/http"
	"github.com/r-david1/moterus/internal/confianza/adaptadores/porteronoop"
	confianzapostgres "github.com/r-david1/moterus/internal/confianza/adaptadores/postgres"
	confianzaredis "github.com/r-david1/moterus/internal/confianza/adaptadores/redis"
	confianzatenencia "github.com/r-david1/moterus/internal/confianza/adaptadores/tenencia"
	"github.com/r-david1/moterus/internal/confianza/adaptadores/turnstile"
	confianzaaplicacion "github.com/r-david1/moterus/internal/confianza/aplicacion"
	confianzapuertos "github.com/r-david1/moterus/internal/confianza/puertos"

	"github.com/r-david1/moterus/internal/identidad/adaptadores/auditoria"
	identidadconfianza "github.com/r-david1/moterus/internal/identidad/adaptadores/confianza"
	"github.com/r-david1/moterus/internal/identidad/adaptadores/cripto"
	"github.com/r-david1/moterus/internal/identidad/adaptadores/eventos"
	identidadhttp "github.com/r-david1/moterus/internal/identidad/adaptadores/http"
	"github.com/r-david1/moterus/internal/identidad/adaptadores/notificaciones"
	identidadpostgres "github.com/r-david1/moterus/internal/identidad/adaptadores/postgres"
	identidadtenencia "github.com/r-david1/moterus/internal/identidad/adaptadores/tenencia"
	"github.com/r-david1/moterus/internal/identidad/aplicacion"
	identidadpuertos "github.com/r-david1/moterus/internal/identidad/puertos"

	tenenciaauditoria "github.com/r-david1/moterus/internal/tenencia/adaptadores/auditoria"
	tenenciaconfianza "github.com/r-david1/moterus/internal/tenencia/adaptadores/confianza"
	tenenciacripto "github.com/r-david1/moterus/internal/tenencia/adaptadores/cripto"
	tenenciaeventos "github.com/r-david1/moterus/internal/tenencia/adaptadores/eventos"
	tenenciahttp "github.com/r-david1/moterus/internal/tenencia/adaptadores/http"
	tenenciaidentidad "github.com/r-david1/moterus/internal/tenencia/adaptadores/identidad"
	tenencianotificaciones "github.com/r-david1/moterus/internal/tenencia/adaptadores/notificaciones"
	tenenciapostgres "github.com/r-david1/moterus/internal/tenencia/adaptadores/postgres"
	tenenciaaplicacion "github.com/r-david1/moterus/internal/tenencia/aplicacion"
	tenenciadominio "github.com/r-david1/moterus/internal/tenencia/dominio"
	tenenciapuertos "github.com/r-david1/moterus/internal/tenencia/puertos"

	"github.com/r-david1/moterus/internal/plataforma/bd"
	"github.com/r-david1/moterus/internal/plataforma/cache"
	"github.com/r-david1/moterus/internal/plataforma/configuracion"
	"github.com/r-david1/moterus/internal/plataforma/reloj"
)

func main() {
	cfg, err := configuracion.CargarDesdeEntorno()
	if err != nil {
		log.Fatalf("configuracion: %v", err)
	}

	ctx := context.Background()

	// ctxFondo/cancelarFondo es el context.Context de cierre ordenado que
	// comparten las goroutines de vida larga del proceso (hoy, solo el
	// reconciliador de colas de acceso virtual, §3.7/§12 de
	// docs/design/colas-virtuales.md: no había ninguna goroutine de fondo
	// antes de esta extensión). Se cancela al recibir SIGINT/SIGTERM, antes
	// de apagar el servidor HTTP.
	ctxFondo, cancelarFondo := context.WithCancel(context.Background())
	defer cancelarFondo()

	app := fiber.New(fiber.Config{
		AppName: "auth-service",
	})

	// Health check de infraestructura. No es un endpoint de negocio de
	// ningún bounded context: se registra aquí, en el bootstrap, sin
	// rate limit ni auth (uso interno/orquestador).
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status": "ok",
			"env":    cfg.EntornoApp,
		})
	})

	if cfg.URLBaseDeDatos == "" && cfg.URLBaseDeDatosAplicacion == "" {
		log.Println("api: ni DATABASE_URL ni DATABASE_URL_APLICACION están definidos — solo se expone /health, los contextos Identidad y Acceso no se montan")
	} else {
		montarIdentidadYAcceso(ctx, ctxFondo, app, cfg)
	}

	// Apagado ordenado: al recibir SIGINT/SIGTERM, cancela ctxFondo (detiene
	// el reconciliador en su próxima vuelta del select) y cierra el
	// servidor HTTP dejando terminar las conexiones activas.
	go func() {
		senales := make(chan os.Signal, 1)
		signal.Notify(senales, os.Interrupt, syscall.SIGTERM)
		<-senales
		log.Println("api: señal de apagado recibida, cerrando de forma ordenada")
		cancelarFondo()
		if err := app.ShutdownWithTimeout(10 * time.Second); err != nil {
			log.Printf("api: error cerrando el servidor HTTP: %v", err)
		}
	}()

	addr := ":" + strconv.Itoa(cfg.Puerto)
	log.Printf("auth-service escuchando en %s (env=%s)", addr, cfg.EntornoApp)
	if err := app.Listen(addr); err != nil {
		log.Fatalf("servidor: %v", err)
	}
}

// montarIdentidadYAcceso conecta el pool pgx (compartido por ambos
// contextos: cada uno con sus propios adaptadores de repositorio y su
// propia UnidadDeTrabajo, ADR 0017), ensambla Identidad (sin registrar
// todavía sus rutas: el endpoint GET /identidad/usuarios/{id} necesita el
// puertos.ValidadorDeAccesos que solo existe una vez montado Acceso),
// ensambla Acceso completo, monta Confianza (colas de acceso virtual, §12
// de docs/design/colas-virtuales.md) y por último registra las rutas de
// los tres. ctxFondo es el context.Context de cierre ordenado para la
// goroutine del reconciliador de salas de espera (ver main()).
func montarIdentidadYAcceso(ctx, ctxFondo context.Context, app *fiber.App, cfg configuracion.Config) {
	dsn := cfg.URLBaseDeDatosAplicacion
	if dsn == "" {
		log.Println("api: ALERTA — DATABASE_URL_APLICACION no definido, usando DATABASE_URL (rol dueño/superusuario). " +
			"El REVOKE UPDATE/DELETE de ADR 0005 queda sin efecto: no usar así en producción. " +
			"Definir DATABASE_URL_APLICACION apuntando a rol_login_identidad (migración 000003).")
		dsn = cfg.URLBaseDeDatos
	}

	pool, err := bd.NuevoPool(ctx, dsn)
	if err != nil {
		log.Fatalf("api: no se pudo conectar a la base de datos: %v", err)
	}

	relojReal := reloj.NuevoReal()

	// riesgo es el motor de Confianza (ADR 0018) compartido por los ACL de
	// Identidad y de Acceso: un único LimitadorTasa/VerificadorCaptcha
	// contra el mismo Redis, cada contexto lo envuelve detrás de su propio
	// puerto EvaluadorConfianza. nil si REDIS_URL no está definido (ambos
	// contextos caen a su propio no-op, cada uno con su WARN de arranque).
	riesgo := construirEvaluadorDeRiesgo(cfg, pool, relojReal)

	// portero es el confianza/puertos.PorteroDeSala que las rutas de
	// Acceso, Identidad y Tenencia montan como su primer/único middleware
	// de sala de espera (§12 del diseño colas-virtuales.md). gestorSalas y
	// consultorSalas alimentan los propios endpoints HTTP de Confianza (§7
	// del diseño) más abajo, después de montar Tenencia (necesitan el
	// VerificadorDeAutorizacion que Tenencia expone). Los tres son nil-safe:
	// sin REDIS_URL, portero es un no-op que nunca encuentra sala vigente
	// (construirConfianzaColas) y gestorSalas/consultorSalas quedan nil —
	// en ese caso NO se registran las rutas propias de Confianza (más abajo).
	portero, gestorSalas, consultorSalas := construirConfianzaColas(ctxFondo, cfg, pool, relojReal, riesgo)

	// --- Identidad: adaptadores y casos de uso (rutas al final) -----------

	repositorioUsuarios := identidadpostgres.NuevoRepositorioUsuarios(pool)
	repositorioTokensVerificacion := identidadpostgres.NuevoRepositorioTokensVerificacion(pool)
	repositorioFactoresMFA := identidadpostgres.NuevoRepositorioFactoresMFA(pool)
	unidadDeTrabajoIdentidad := identidadpostgres.NuevaUnidadDeTrabajo(pool)
	generadorIDsIdentidad := identidadpostgres.NuevoGeneradorIDs()

	hasher := cripto.NuevoHasherArgon2id()
	verificadorFiltradas := cripto.NuevoVerificadorHIBP()
	generadorTokensIdentidad := cripto.NuevoGeneradorTokens()
	generadorTOTP := cripto.NuevoGeneradorTOTP()
	cifradorSecretosMFA := construirCifradorSecretosMFA(cfg)

	registroAuditoriaIdentidad := auditoria.NuevoRegistroAuditoria(pool)
	publicadorEventosIdentidad := eventos.NuevoPublicadorLog(nil)
	evaluadorConfianzaIdentidad := construirEvaluadorConfianzaIdentidad(riesgo)
	notificadorCorreo := notificaciones.NuevoNotificadorCorreoLog(nil)

	registrador := aplicacion.NuevoRegistrarUsuarioCasoDeUso(
		repositorioUsuarios, hasher, verificadorFiltradas, evaluadorConfianzaIdentidad,
		registroAuditoriaIdentidad, publicadorEventosIdentidad, relojReal, generadorIDsIdentidad, unidadDeTrabajoIdentidad,
		generadorTokensIdentidad, repositorioTokensVerificacion, notificadorCorreo,
	)
	autenticadorIdentidad := aplicacion.NuevoAutenticarUsuarioCasoDeUso(
		repositorioUsuarios, hasher, evaluadorConfianzaIdentidad,
		registroAuditoriaIdentidad, publicadorEventosIdentidad, relojReal, unidadDeTrabajoIdentidad,
	)
	consultorIdentidad := aplicacion.NuevoObtenerUsuarioCasoDeUso(repositorioUsuarios, registroAuditoriaIdentidad, relojReal)
	verificadorCorreo := aplicacion.NuevoVerificarCorreoCasoDeUso(
		repositorioUsuarios, repositorioTokensVerificacion, registroAuditoriaIdentidad, publicadorEventosIdentidad, relojReal, unidadDeTrabajoIdentidad,
	)
	reenviadorVerificacion := aplicacion.NuevoReenviarVerificacionCasoDeUso(
		repositorioUsuarios, generadorTokensIdentidad, repositorioTokensVerificacion, notificadorCorreo, relojReal,
	)

	// --- Identidad: OTP/MFA (docs/design/otp-mfa.md) -----------------------
	//
	// Cuatro casos de uso nuevos sobre el mismo repositorioUsuarios/
	// auditoria/eventos/reloj/uow ya ensamblados arriba. gestorMFACompuesto
	// (definido más abajo en este archivo) es la única pieza de wiring no
	// trivial: puertos.GestorDeMFA (identidad/puertos/entrada.go, cerrado
	// para este encargo) agrupa Habilitar/ConfirmarFactor/Deshabilitar en UNA
	// interfaz, pero cada caso de uso de aplicacion implementa solo UNO de
	// esos tres métodos (HabilitarMFACasoDeUso.Habilitar,
	// ConfirmarFactorMFACasoDeUso.ConfirmarFactor,
	// DeshabilitarMFACasoDeUso.Deshabilitar) — ningún struct de aplicacion
	// satisface GestorDeMFA por sí solo. gestorMFACompuesto es el adaptador
	// de composición que cierra ese hueco delegando cada método al caso de
	// uso correspondiente, sin tocar aplicacion/puertos.
	habilitarMFA := aplicacion.NuevoHabilitarMFACasoDeUso(
		repositorioUsuarios, repositorioFactoresMFA, generadorTOTP, cifradorSecretosMFA,
		registroAuditoriaIdentidad, publicadorEventosIdentidad, relojReal, generadorIDsIdentidad, unidadDeTrabajoIdentidad,
	)
	confirmarFactorMFA := aplicacion.NuevoConfirmarFactorMFACasoDeUso(
		repositorioUsuarios, repositorioFactoresMFA, generadorTOTP, cifradorSecretosMFA,
		registroAuditoriaIdentidad, publicadorEventosIdentidad, relojReal, unidadDeTrabajoIdentidad,
	)
	deshabilitarMFA := aplicacion.NuevoDeshabilitarMFACasoDeUso(
		repositorioUsuarios, repositorioFactoresMFA, cifradorSecretosMFA,
		registroAuditoriaIdentidad, publicadorEventosIdentidad, relojReal, unidadDeTrabajoIdentidad,
	)
	verificarOTP := aplicacion.NuevoVerificarOTPCasoDeUso(
		repositorioFactoresMFA, cifradorSecretosMFA, registroAuditoriaIdentidad, relojReal, unidadDeTrabajoIdentidad,
	)
	gestorMFA := gestorMFACompuesto{habilitar: habilitarMFA, confirmar: confirmarFactorMFA, deshabilitar: deshabilitarMFA}

	// --- Acceso: adaptadores, casos de uso y rutas -------------------------

	validadorAcceso, manejadorAcceso := montarAcceso(cfg, pool, relojReal, autenticadorIdentidad, consultorIdentidad, verificarOTP, riesgo)
	accesohttp.RegistrarRutas(app, manejadorAcceso, validadorAcceso, portero)

	// --- Tenencia: adaptadores, casos de uso y rutas -----------------------
	//
	// Se monta DESPUÉS de Acceso (necesita validadorAcceso para su propio
	// middleware de autenticación) y ANTES de ensamblar el manejador HTTP
	// de Identidad: GET /identidad/usuarios/{id} necesita el
	// VerificadorDeAutorizacion y el ConsultorDeMembresias que Tenencia
	// expone (§11.2 del diseño de Tenencia) para construir su propio ACL
	// (identidadtenencia.AutorizadorConsultas) antes de construir
	// manejadorIdentidad.
	autorizadorTenencia, consultorMembresiasTenencia := montarTenencia(cfg, app, pool, relojReal, validadorAcceso, consultorIdentidad, riesgo, portero)
	autorizadorConsultasIdentidad := identidadtenencia.NuevoAutorizadorConsultas(autorizadorTenencia, consultorMembresiasTenencia)

	manejadorIdentidad := identidadhttp.NuevoManejadorIdentidad(
		registrador, autenticadorIdentidad, consultorIdentidad, verificadorCorreo, reenviadorVerificacion,
		evaluadorConfianzaIdentidad, autorizadorConsultasIdentidad, gestorMFA,
	)

	// --- Identidad: rutas (ahora sí, con el validador de Acceso) ----------

	identidadhttp.RegistrarRutas(app, manejadorIdentidad, validadorAcceso, portero)

	// --- Confianza: rutas propias (§7 del diseño colas-virtuales.md) ------
	//
	// Se montan al final porque los dos endpoints de administración
	// org-scoped (§7.2) necesitan el VerificadorDeAutorizacion que Tenencia
	// recién terminó de exponer. Sin REDIS_URL, gestorSalas/consultorSalas
	// son nil (construirConfianzaColas): no hay nada real que administrar
	// ni consultar, así que las rutas de Confianza NO se registran en
	// absoluto (a diferencia de Acceso/Identidad/Tenencia, que siempre
	// registran las suyas con el portero no-op montado).
	if gestorSalas != nil && consultorSalas != nil {
		autorizadorConfianza := confianzatenencia.NuevoVerificadorAutorizacion(autorizadorTenencia)
		manejadorConfianza := confianzahttp.NuevoManejadorConfianza(portero, gestorSalas, consultorSalas)
		confianzahttp.RegistrarRutas(app, manejadorConfianza, validadorAcceso, autorizadorConfianza)
		log.Println("api: contexto Confianza (colas de acceso virtual) montado: rutas propias + middleware en acceso.iniciar_sesion/identidad.registrar_usuario/tenencia.aceptar_invitacion")
	} else {
		log.Println("api: colas de acceso virtual en modo no-op (REDIS_URL no configurado) — Acceso/Identidad/Tenencia montan el middleware, pero nunca hay sala vigente; sin rutas propias de Confianza")
	}

	estadoConfianza := "confianza-noop"
	if riesgo != nil {
		estadoConfianza = "confianza-real(redis+turnstile)"
	}
	log.Printf("api: contexto Identidad montado (postgres, argon2id, hibp, auditoria, eventos-log, %s, notificador-correo-log)", estadoConfianza)
}

// montarAcceso ensambla el bounded context Acceso completo (ADR 0019/0020)
// y devuelve el puertos.ValidadorDeAccesos (para que Identidad cierre su
// hueco de autenticación) y el manejador HTTP ya listo para registrar sus
// propias rutas. autenticadorIdentidad/consultorIdentidad son los casos de
// uso de Identidad detrás de los ACL de acceso/adaptadores/identidad — el
// único paquete de Acceso autorizado a importar identidad/puertos
// (INV-ACC-19).
func montarAcceso(
	cfg configuracion.Config,
	pool *pgxpool.Pool,
	relojReal reloj.Real,
	autenticadorIdentidad identidadpuertos.AutenticadorDeCredenciales,
	consultorIdentidad identidadpuertos.ConsultorDeUsuarios,
	verificadorOTPIdentidad identidadpuertos.VerificadorOTP,
	riesgo confianzapuertos.EvaluadorDeRiesgo,
) (accesopuertos.ValidadorDeAccesos, *accesohttp.ManejadorAcceso) {
	politica := accesodominio.PoliticaSesionPorDefecto()

	sesiones := accesopostgres.NuevoRepositorioSesiones(pool)
	uow := accesopostgres.NuevaUnidadDeTrabajo(pool)
	generadorIDs := accesopostgres.NuevoGeneradorIDs()
	generadorRefrescos := accesocripto.NuevoGeneradorTokensRefresco()
	registroAuditoria := accesoauditoria.NuevoRegistroAuditoria(pool)
	publicadorEventos := accesoeventos.NuevoPublicadorLog(nil)

	emisor := exigirEnProduccionOAdvertir(cfg, "ACCESO_EMISOR", cfg.AccesoEmisor, "https://accesos.moterus.local")
	audiencia := exigirEnProduccionOAdvertir(cfg, "ACCESO_AUDIENCIA", cfg.AccesoAudiencia, "moterus")

	llaveFirma := cfg.AccesoLlaveFirma
	if llaveFirma == "" {
		if cfg.EntornoApp == "production" {
			log.Fatalf("api: ACCESO_LLAVE_FIRMA no está definida en APP_ENV=production — un servicio de " +
				"autenticación que no puede firmar tokens no tiene nada que hacer sirviendo tráfico (ADR 0020 §4).")
		}
		efimera, errLlave := accesojwt.GenerarLlaveEfimera()
		if errLlave != nil {
			log.Fatalf("api: no se pudo generar la llave de firma efímera de Acceso: %v", errLlave)
		}
		llaveFirma = efimera
		log.Println("api: ALERTA — ACCESO_LLAVE_FIRMA no está definida, usando una llave Ed25519 efímera " +
			"generada en memoria. Todos los tokens de acceso firmados mueren al reiniciar el proceso. " +
			"No usar así en producción (ADR 0020 §4).")
	}
	var llavesPrevias []string
	if cfg.AccesoLlavesVerificacionPrevias != "" {
		llavesPrevias = strings.Split(cfg.AccesoLlavesVerificacionPrevias, ",")
	}
	llavero, err := accesojwt.NuevoLlavero(llaveFirma, llavesPrevias)
	if err != nil {
		log.Fatalf("api: no se pudo construir el llavero de firma de Acceso (ACCESO_LLAVE_FIRMA/ACCESO_LLAVES_VERIFICACION_PREVIAS): %v", err)
	}
	firmador := accesojwt.NuevoFirmador(llavero, emisor, audiencia, politica.ToleranciaReloj())

	var listaRevocacion accesopuertos.ListaRevocacion
	if cfg.URLRedis != "" {
		clienteRedis, errRedis := cache.NuevoClienteRedis(cfg.URLRedis)
		if errRedis != nil {
			log.Fatalf("api: REDIS_URL definido pero inválido (Acceso): %v", errRedis)
		}
		listaRevocacion = accesoredis.NuevaListaRevocacion(clienteRedis)
	} else {
		listaRevocacion = accesoredis.NuevaListaRevocacionNoOp()
	}

	var evaluadorConfianzaAcceso accesopuertos.EvaluadorConfianza
	if riesgo != nil {
		evaluadorConfianzaAcceso = accesoconfianza.NuevoEvaluadorConfianzaReal(riesgo)
	} else {
		evaluadorConfianzaAcceso = accesoconfianza.NuevoEvaluadorConfianzaNoOp(nil)
	}

	autenticadorACL := accesoidentidad.NuevoAutenticadorIdentidad(autenticadorIdentidad)
	consultorEstadoSujeto := accesoidentidad.NuevoConsultorEstadoSujeto(consultorIdentidad)
	verificadorSegundoFactorACL := accesoidentidad.NuevoVerificadorOTP(verificadorOTPIdentidad)

	// emisorStepUp: JWT propio de Acceso (ADR 0038/docs/design/otp-mfa.md
	// §2.4), reutilizando el MISMO Llavero que Firmador ya construyó arriba
	// — sin llave nueva ni JWKS nuevo, solo un `typ` de cabecera distinto
	// (INV-MFA-03).
	emisorStepUp := accesojwt.NuevoEmisorTokenStepUp(llavero, politica.ToleranciaReloj())

	iniciador := accesoaplicacion.NuevoIniciarSesionCasoDeUso(
		autenticadorACL, sesiones, generadorRefrescos, firmador, listaRevocacion,
		registroAuditoria, publicadorEventos, relojReal, generadorIDs, uow, politica, emisor, audiencia,
		emisorStepUp,
	)
	renovador := accesoaplicacion.NuevoRenovarSesionCasoDeUso(
		evaluadorConfianzaAcceso, sesiones, generadorRefrescos, firmador, consultorEstadoSujeto, listaRevocacion,
		registroAuditoria, relojReal, generadorIDs, uow, politica, emisor, audiencia,
	)
	validador := accesoaplicacion.NuevoValidarAccesoCasoDeUso(firmador, listaRevocacion, sesiones, registroAuditoria, relojReal)
	cerrador := accesoaplicacion.NuevoCerrarSesionCasoDeUso(sesiones, evaluadorConfianzaAcceso, listaRevocacion, registroAuditoria, relojReal, uow, politica)
	consultorSesiones := accesoaplicacion.NuevoListarSesionesCasoDeUso(sesiones)
	completadorSegundoFactor := accesoaplicacion.NuevoCompletarSegundoFactorCasoDeUso(
		emisorStepUp, evaluadorConfianzaAcceso, verificadorSegundoFactorACL, iniciador,
	)

	manejador := accesohttp.NuevoManejadorAcceso(iniciador, renovador, cerrador, consultorSesiones, firmador, completadorSegundoFactor)

	estadoRedis := "sin-redis(lista-revocacion-degradada-hasta-vida-token-acceso)"
	if cfg.URLRedis != "" {
		estadoRedis = "redis(lista-revocacion-inmediata)"
	}
	estadoConfianza := "confianza-noop"
	if riesgo != nil {
		estadoConfianza = "confianza-real(redis+turnstile)"
	}
	log.Printf("api: contexto Acceso montado (postgres, jwx/ed25519 kid=%s, %s, %s, auditoria, eventos-log)",
		llavero.KIDActivo(), estadoRedis, estadoConfianza)

	return validador, manejador
}

// gestorMFACompuesto implementa identidad/puertos.GestorDeMFA delegando
// cada método al caso de uso de aplicacion que efectivamente lo implementa
// (ver el comentario en montarIdentidadYAcceso sobre por qué hace falta
// este adaptador de composición: GestorDeMFA agrupa tres operaciones en una
// sola interfaz de puerto, pero cada caso de uso de aplicacion solo
// implementa una).
type gestorMFACompuesto struct {
	habilitar    *aplicacion.HabilitarMFACasoDeUso
	confirmar    *aplicacion.ConfirmarFactorMFACasoDeUso
	deshabilitar *aplicacion.DeshabilitarMFACasoDeUso
}

var _ identidadpuertos.GestorDeMFA = gestorMFACompuesto{}

func (g gestorMFACompuesto) Habilitar(ctx context.Context, cmd identidadpuertos.ComandoHabilitarMFA) (identidadpuertos.ResultadoHabilitarMFA, error) {
	return g.habilitar.Habilitar(ctx, cmd)
}

func (g gestorMFACompuesto) ConfirmarFactor(ctx context.Context, cmd identidadpuertos.ComandoConfirmarFactorMFA) (identidadpuertos.ResultadoConfirmarMFA, error) {
	return g.confirmar.ConfirmarFactor(ctx, cmd)
}

func (g gestorMFACompuesto) Deshabilitar(ctx context.Context, cmd identidadpuertos.ComandoDeshabilitarMFA) error {
	return g.deshabilitar.Deshabilitar(ctx, cmd)
}

// montarTenencia ensambla el bounded context Tenencia completo
// (docs/design/tenencia-bounded-context.md): los tres repositorios Postgres
// con RLS (§6.3), los cinco ACL de cruce (Identidad, Confianza, Auditoría,
// eventos, notificaciones), los casos de uso y las rutas HTTP con sus dos
// middlewares (autenticación + autorización). Se monta DESPUÉS de Acceso
// (necesita validadorAcceso) y registra sus propias rutas directamente
// (mismo criterio que montarAcceso, salvo que aquí RegistrarRutas se llama
// dentro de esta función porque el autorizador y el alcance que necesita
// son internos a este ensamblado, no algo que otro contexto vaya a
// reutilizar). Devuelve el VerificadorDeAutorizacion y el
// ConsultorDeMembresias — los dos puertos que Tenencia EXPONE a otros
// contextos (§2.4 del diseño) — para que montarIdentidadYAcceso construya
// el ACL identidad/adaptadores/tenencia.AutorizadorConsultas antes de
// ensamblar el manejador HTTP de Identidad.
func montarTenencia(
	cfg configuracion.Config,
	app *fiber.App,
	pool *pgxpool.Pool,
	relojReal reloj.Real,
	validadorAcceso accesopuertos.ValidadorDeAccesos,
	consultorIdentidad identidadpuertos.ConsultorDeUsuarios,
	riesgo confianzapuertos.EvaluadorDeRiesgo,
	portero confianzapuertos.PorteroDeSala,
) (tenenciapuertos.VerificadorDeAutorizacion, tenenciapuertos.ConsultorDeMembresias) {
	organizaciones := tenenciapostgres.NuevoRepositorioOrganizaciones(pool)
	membresias := tenenciapostgres.NuevoRepositorioMembresias(pool)
	invitacionesRepo := tenenciapostgres.NuevoRepositorioInvitaciones(pool)
	uow := tenenciapostgres.NuevaUnidadDeTrabajo(pool)
	generadorIDs := tenenciapostgres.NuevoGeneradorIDs()
	alcance := tenenciapostgres.NuevaAlcanceTenencia()
	generadorTokens := tenenciacripto.NuevoGeneradorTokens()

	registroAuditoriaTenencia := tenenciaauditoria.NuevoRegistroAuditoria(pool)
	publicadorEventosTenencia := tenenciaeventos.NuevoPublicadorLog(nil)
	notificadorInvitaciones := tenencianotificaciones.NuevoNotificadorInvitacionesLog(nil)
	sujetos := tenenciaidentidad.NuevoVerificadorSujetos(consultorIdentidad)

	var evaluadorConfianzaTenencia tenenciapuertos.EvaluadorConfianza
	if riesgo != nil {
		evaluadorConfianzaTenencia = tenenciaconfianza.NuevoEvaluadorConfianzaReal(riesgo)
	} else {
		evaluadorConfianzaTenencia = tenenciaconfianza.NuevoEvaluadorConfianzaNoOp(nil)
	}

	politica := tenenciadominio.PoliticaOrganizacionPorDefecto()

	autorizador := tenenciaaplicacion.NuevoAutorizarCasoDeUso(organizaciones, membresias, registroAuditoriaTenencia, relojReal)
	gestorOrganizaciones := tenenciaaplicacion.NuevoOrganizacionesCasoDeUso(
		organizaciones, membresias, autorizador, sujetos, evaluadorConfianzaTenencia,
		registroAuditoriaTenencia, publicadorEventosTenencia, relojReal, generadorIDs, uow, politica,
	)
	gestorMembresias := tenenciaaplicacion.NuevoMembresiasCasoDeUso(
		organizaciones, membresias, autorizador, sujetos,
		registroAuditoriaTenencia, publicadorEventosTenencia, relojReal, generadorIDs, uow, politica,
	)
	gestorInvitaciones := tenenciaaplicacion.NuevoInvitacionesCasoDeUso(
		organizaciones, membresias, invitacionesRepo, autorizador, sujetos, evaluadorConfianzaTenencia, notificadorInvitaciones,
		registroAuditoriaTenencia, publicadorEventosTenencia, relojReal, generadorIDs, generadorTokens, uow, politica,
	)
	consultas := tenenciaaplicacion.NuevoConsultasCasoDeUso(organizaciones, membresias, autorizador)

	manejador := tenenciahttp.NuevoManejadorTenencia(gestorOrganizaciones, consultas, gestorMembresias, consultas, gestorInvitaciones)
	tenenciahttp.RegistrarRutas(app, manejador, validadorAcceso, autorizador, alcance, portero)

	estadoConfianza := "confianza-noop"
	if riesgo != nil {
		estadoConfianza = "confianza-real(redis+turnstile)"
	}
	log.Printf("api: contexto Tenencia montado (postgres+rls, auditoria, eventos-log, notificador-invitaciones-log, %s)", estadoConfianza)

	return autorizador, consultas
}

// exigirEnProduccionOAdvertir implementa el fail-closed de ADR 0020
// (Consecuencias: "Configuración nueva obligatoria en producción:
// ACCESO_LLAVE_FIRMA, ACCESO_EMISOR, ACCESO_AUDIENCIA") para los dos
// valores que sí admiten un default operable fuera de producción.
func exigirEnProduccionOAdvertir(cfg configuracion.Config, nombreVar, valor, defectoDesarrollo string) string {
	if valor != "" {
		return valor
	}
	if cfg.EntornoApp == "production" {
		log.Fatalf("api: %s no está definida en APP_ENV=production (ADR 0020).", nombreVar)
	}
	log.Printf("api: %s no está definida; usando el valor de desarrollo %q", nombreVar, defectoDesarrollo)
	return defectoDesarrollo
}

// construirEvaluadorDeRiesgo monta el motor real de Confianza (ADR 0018:
// LimitadorTasa sobre Redis + VerificadorCaptcha Turnstile) si REDIS_URL
// está definido, o nil si no — cada contexto (Identidad, Acceso) decide
// por su cuenta qué adaptador no-op montar detrás de su propio puerto
// EvaluadorConfianza cuando esto es nil.
//
// Con REDIS_URL definido, también cablea la tercera extensión de Confianza
// —reconocimiento de origen, docs/design/fingerprinting-comportamiento.md—:
// el puerto PerfilDeOrigenes (§2.2) sobre el mismo cliente Redis compartido
// (nunca una conexión nueva), la auditoría de Confianza ya usada por las
// colas virtuales (§1.6) y el reloj real. ConPoliticaRiesgo se omite a
// propósito: el propio constructor usa dominio.PoliticaRiesgoPorDefecto(),
// que arranca en modo `observar` (INV-RIES-14) — el comportamiento correcto
// para un primer despliegue, sin calibrar contra tráfico real.
func construirEvaluadorDeRiesgo(cfg configuracion.Config, pool *pgxpool.Pool, relojReal reloj.Real) confianzapuertos.EvaluadorDeRiesgo {
	if cfg.URLRedis == "" {
		slog.Warn("api: reconocimiento de origen desactivado — REDIS_URL no está definido, EvaluarTrustSignalCasoDeUso ni siquiera se monta")
		return nil
	}
	clienteRedis, err := cache.NuevoClienteRedis(cfg.URLRedis)
	if err != nil {
		log.Fatalf("api: REDIS_URL definido pero inválido: %v", err)
	}
	limitador := confianzaredis.NuevoLimitadorTasa(clienteRedis)
	verificadorCaptcha := turnstile.NuevoVerificadorCaptcha(cfg.TurnstileSecretKey, cfg.TurnstileVerifyURL, cfg.EntornoApp)
	perfilOrigenes := confianzaredis.NuevoPerfilDeOrigenes(clienteRedis)
	registroAuditoriaRiesgo := confianzaauditoria.NuevoRegistroAuditoria(pool)
	evaluarTrustSignal := confianzaaplicacion.NuevoEvaluarTrustSignalCasoDeUso(
		limitador, verificadorCaptcha,
		confianzaaplicacion.ConPerfilesDeOrigen(perfilOrigenes),
		confianzaaplicacion.ConAuditoriaDeRiesgo(registroAuditoriaRiesgo),
		confianzaaplicacion.ConRelojDeRiesgo(relojReal),
	)
	slog.Info("api: motor de Confianza real montado (rate limiting por IP y por cuenta vía Redis + captcha Cloudflare Turnstile)",
		"redis_configurado", true, "turnstile_secret_configurado", cfg.TurnstileSecretKey != "")
	slog.Info("api: reconocimiento de origen activo (modo observar por defecto — no cambia el desenlace de ningún login, solo lo audita/loguea)",
		"redis_configurado", true)
	return evaluarTrustSignal
}

// construirConfianzaColas ensambla el motor de colas de acceso virtual
// (Confianza, docs/design/colas-virtuales.md §12): el PorteroDeSala de
// camino caliente que consumen los tres middlewares de Acceso/Identidad/
// Tenencia, y —solo si REDIS_URL está configurado— el GestorDeSalasDeEspera
// y el ConsultorDeSalas que alimentan las rutas propias de administración/
// consulta de Confianza (registradas más abajo, después de montar
// Tenencia). Arranca además el reconciliador (§3.7) como goroutine con
// time.Ticker(15s), atada a ctxFondo para un cierre ordenado (ver main()).
//
// Sin REDIS_URL: devuelve el portero no-op de
// confianza/adaptadores/porteronoop (nunca hay sala vigente, con su WARN de
// arranque — mismo criterio que EvaluadorConfianzaNoOp) y (nil, nil) para
// gestorSalas/consultorSalas: INV-COLA-08 hace que un motor sin Redis no
// tenga nada real que administrar.
func construirConfianzaColas(
	ctxFondo context.Context,
	cfg configuracion.Config,
	pool *pgxpool.Pool,
	relojReal reloj.Real,
	riesgo confianzapuertos.EvaluadorDeRiesgo,
) (confianzapuertos.PorteroDeSala, confianzapuertos.GestorDeSalasDeEspera, confianzapuertos.ConsultorDeSalas) {
	if cfg.URLRedis == "" {
		return porteronoop.NuevoPorteroDeSala(nil), nil, nil
	}

	clienteRedis, err := cache.NuevoClienteRedis(cfg.URLRedis)
	if err != nil {
		log.Fatalf("api: REDIS_URL definido pero inválido (Confianza/colas de acceso virtual): %v", err)
	}
	estadoCola := confianzaredis.NuevoEstadoCola(clienteRedis)
	repoSalas := confianzapostgres.NuevoRepositorioSalasDeEspera(pool)
	uow := confianzapostgres.NuevaUnidadDeTrabajo(pool)
	generadorIDs := confianzapostgres.NuevoGeneradorIDs()
	generadorTickets := confianzacripto.NuevoGeneradorTickets()
	registroAuditoria := confianzaauditoria.NuevoRegistroAuditoria(pool)

	instantanea := confianzaaplicacion.NuevaInstantaneaSalasVigentes()
	reconciliador := confianzaaplicacion.NuevoReconciliarSalasCasoDeUso(repoSalas, estadoCola, relojReal, instantanea)
	portero := confianzaaplicacion.NuevoPorteroDeSalaCasoDeUso(instantanea, estadoCola, generadorTickets, relojReal, riesgo)

	abrirSala := confianzaaplicacion.NuevoAbrirSalaCasoDeUso(repoSalas, estadoCola, registroAuditoria, relojReal, generadorIDs, uow)
	cambiarRitmo := confianzaaplicacion.NuevoCambiarRitmoDeAdmisionCasoDeUso(repoSalas, estadoCola, registroAuditoria, relojReal, uow)
	cambiarEstado := confianzaaplicacion.NuevoCambiarEstadoSalaCasoDeUso(repoSalas, estadoCola, registroAuditoria, relojReal, uow)
	gestorSalas := gestorSalasDeEsperaCompuesto{abrir: abrirSala, cambiarRitmo: cambiarRitmo, cambiarEstado: cambiarEstado}
	consultorSalas := confianzaaplicacion.NuevoConsultarSalaCasoDeUso(instantanea, estadoCola)

	// Ciclo inicial síncrono: una sala que ya estaba abierta en Postgres
	// antes de que este proceso arrancara queda protegida desde la primera
	// petición, no recién a los 15s del primer tick.
	if errRecon := reconciliador.Reconciliar(context.Background()); errRecon != nil {
		slog.Error("api: el primer ciclo del reconciliador de colas de acceso virtual falló; se reintentará en el próximo tick (15s)",
			"error", errRecon)
	}

	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctxFondo.Done():
				slog.Info("api: reconciliador de colas de acceso virtual detenido (apagado ordenado)")
				return
			case <-ticker.C:
				if errRecon := reconciliador.Reconciliar(ctxFondo); errRecon != nil {
					slog.Error("api: ciclo del reconciliador de colas de acceso virtual falló", "error", errRecon)
				}
			}
		}
	}()

	slog.Info("api: motor de colas de acceso virtual (Confianza) montado (postgres+redis, reconciliador cada 15s)")
	return portero, gestorSalas, consultorSalas
}

// gestorSalasDeEsperaCompuesto implementa confianza/puertos.
// GestorDeSalasDeEspera delegando cada método al caso de uso de aplicacion
// que efectivamente lo implementa (§3.1-§3.3 del diseño colas-virtuales.md:
// Abrir/CambiarRitmo/CambiarEstado son tres casos de uso separados que,
// juntos, satisfacen el puerto de administración) — mismo criterio de
// composición exacto que gestorMFACompuesto para
// identidad/puertos.GestorDeMFA, más arriba en este archivo.
type gestorSalasDeEsperaCompuesto struct {
	abrir         *confianzaaplicacion.AbrirSalaCasoDeUso
	cambiarRitmo  *confianzaaplicacion.CambiarRitmoDeAdmisionCasoDeUso
	cambiarEstado *confianzaaplicacion.CambiarEstadoSalaCasoDeUso
}

var _ confianzapuertos.GestorDeSalasDeEspera = gestorSalasDeEsperaCompuesto{}

func (g gestorSalasDeEsperaCompuesto) Abrir(ctx context.Context, cmd confianzapuertos.ComandoAbrirSala) (confianzapuertos.VistaSala, error) {
	return g.abrir.Abrir(ctx, cmd)
}

func (g gestorSalasDeEsperaCompuesto) CambiarRitmo(ctx context.Context, cmd confianzapuertos.ComandoCambiarRitmoAdmision) (confianzapuertos.VistaSala, error) {
	return g.cambiarRitmo.CambiarRitmo(ctx, cmd)
}

func (g gestorSalasDeEsperaCompuesto) CambiarEstado(ctx context.Context, cmd confianzapuertos.ComandoCambiarEstadoSala) (confianzapuertos.VistaSala, error) {
	return g.cambiarEstado.CambiarEstado(ctx, cmd)
}

// construirCifradorSecretosMFA implementa el mismo criterio de gestión de
// llaves que ACCESO_LLAVE_FIRMA (ADR 0020 §4) para
// IDENTIDAD_LLAVE_CIFRADO_MFA (docs/design/otp-mfa.md §2.2, AES-256-GCM):
// sin ella en APP_ENV=production, el proceso no arranca; fuera de
// producción, si falta, genera una llave efímera en memoria con un WARN
// explícito de que los secretos TOTP cifrados con ella quedan
// indescifrables al reiniciar el proceso (un usuario con MFA ya confirmado
// perdería la capacidad de completar el login hasta deshabilitar y volver
// a habilitar MFA).
func construirCifradorSecretosMFA(cfg configuracion.Config) *cripto.CifradorSecretosAESGCM {
	llaveCruda := cfg.IdentidadLlaveCifradoMFA
	if llaveCruda == "" {
		if cfg.EntornoApp == "production" {
			log.Fatalf("api: IDENTIDAD_LLAVE_CIFRADO_MFA no está definida en APP_ENV=production — un servicio " +
				"que no puede cifrar/descifrar secretos TOTP no tiene nada que hacer sirviendo tráfico (docs/design/otp-mfa.md §2.2).")
		}
		efimera, err := cripto.GenerarLlaveCifradoMFAEfimera()
		if err != nil {
			log.Fatalf("api: no se pudo generar la llave de cifrado de MFA efímera: %v", err)
		}
		llaveCruda = efimera
		log.Println("api: ALERTA — IDENTIDAD_LLAVE_CIFRADO_MFA no está definida, usando una llave AES-256 efímera " +
			"generada en memoria. Todo secreto TOTP cifrado con ella queda indescifrable al reiniciar el proceso. " +
			"No usar así en producción (docs/design/otp-mfa.md §2.2).")
	}
	llave, err := cripto.DecodificarLlaveCifradoMFA(llaveCruda)
	if err != nil {
		log.Fatalf("api: IDENTIDAD_LLAVE_CIFRADO_MFA inválida: %v", err)
	}
	cifrador, err := cripto.NuevoCifradorSecretosAESGCM(llave)
	if err != nil {
		log.Fatalf("api: no se pudo construir el cifrador de secretos MFA: %v", err)
	}
	return cifrador
}

// construirEvaluadorConfianzaIdentidad decide entre EvaluadorConfianzaNoOp
// y EvaluadorConfianzaReal de Identidad según si riesgo es nil (ADR 0018).
func construirEvaluadorConfianzaIdentidad(riesgo confianzapuertos.EvaluadorDeRiesgo) identidadpuertos.EvaluadorConfianza {
	if riesgo == nil {
		return identidadconfianza.NuevoEvaluadorConfianzaNoOp(nil)
	}
	return identidadconfianza.NuevoEvaluadorConfianzaReal(riesgo)
}
