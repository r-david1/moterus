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
	"fmt"
	"log"
	"log/slog"
	"strconv"
	"strings"
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

	confianzaredis "github.com/r-david1/moterus/internal/confianza/adaptadores/redis"
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
		montarIdentidadYAcceso(ctx, app, cfg)
	}

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
// ensambla Acceso completo, y por último registra las rutas de ambos.
func montarIdentidadYAcceso(ctx context.Context, app *fiber.App, cfg configuracion.Config) {
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
	riesgo := construirEvaluadorDeRiesgo(cfg)

	// --- Identidad: adaptadores y casos de uso (rutas al final) -----------

	repositorioUsuarios := identidadpostgres.NuevoRepositorioUsuarios(pool)
	repositorioTokensVerificacion := identidadpostgres.NuevoRepositorioTokensVerificacion(pool)
	unidadDeTrabajoIdentidad := identidadpostgres.NuevaUnidadDeTrabajo(pool)
	generadorIDsIdentidad := identidadpostgres.NuevoGeneradorIDs()

	hasher := cripto.NuevoHasherArgon2id()
	verificadorFiltradas := cripto.NuevoVerificadorHIBP()
	generadorTokensIdentidad := cripto.NuevoGeneradorTokens()

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

	// --- Acceso: adaptadores, casos de uso y rutas -------------------------

	validadorAcceso, manejadorAcceso := montarAcceso(cfg, pool, relojReal, autenticadorIdentidad, consultorIdentidad, riesgo)
	accesohttp.RegistrarRutas(app, manejadorAcceso, validadorAcceso)

	// --- Tenencia: adaptadores, casos de uso y rutas -----------------------
	//
	// Se monta DESPUÉS de Acceso (necesita validadorAcceso para su propio
	// middleware de autenticación) y ANTES de ensamblar el manejador HTTP
	// de Identidad: GET /identidad/usuarios/{id} necesita el
	// VerificadorDeAutorizacion y el ConsultorDeMembresias que Tenencia
	// expone (§11.2 del diseño de Tenencia) para construir su propio ACL
	// (identidadtenencia.AutorizadorConsultas) antes de construir
	// manejadorIdentidad.
	autorizadorTenencia, consultorMembresiasTenencia := montarTenencia(cfg, app, pool, relojReal, validadorAcceso, consultorIdentidad, riesgo)
	autorizadorConsultasIdentidad := identidadtenencia.NuevoAutorizadorConsultas(autorizadorTenencia, consultorMembresiasTenencia)

	manejadorIdentidad := identidadhttp.NuevoManejadorIdentidad(
		registrador, autenticadorIdentidad, consultorIdentidad, verificadorCorreo, reenviadorVerificacion,
		evaluadorConfianzaIdentidad, autorizadorConsultasIdentidad,
	)

	// --- Identidad: rutas (ahora sí, con el validador de Acceso) ----------

	identidadhttp.RegistrarRutas(app, manejadorIdentidad, validadorAcceso)

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

	// emisorStepUp: el adaptador real (JWT propio de Acceso, TTL de 5
	// minutos, ADR 0038/docs/design/otp-mfa.md §2.4, reutilizando el
	// Llavero ya construido arriba) todavía no existe en
	// internal/acceso/adaptadores/jwt — es trabajo de infraestructura
	// pendiente, fuera del alcance de este cambio (que solo extiende la
	// capa de aplicación de Identidad/Acceso). Se usa aquí un placeholder
	// que falla explícitamente en vez de fingir emitir un token válido: el
	// login de un usuario que NO requiere segundo factor sigue funcionando
	// de punta a punta sin tocar este puerto; solo el paso "emitir el
	// token de step-up cuando RequiereSegundoFactor==true" queda roto
	// hasta que el adaptador real se implemente.
	emisorStepUp := emisorTokenStepUpPendiente{}

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

	manejador := accesohttp.NuevoManejadorAcceso(iniciador, renovador, cerrador, consultorSesiones, firmador)

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

// emisorTokenStepUpPendiente es un placeholder TEMPORAL de
// puertos.EmisorTokenStepUp (ADR 0038, docs/design/otp-mfa.md §2.4). El
// adaptador real (JWT propio de Acceso firmado con el mismo Llavero de
// FirmadorTokensAcceso, TTL de 5 minutos, `typ` de cabecera distintivo) es
// trabajo de infraestructura pendiente en
// internal/acceso/adaptadores/jwt, fuera del alcance de la capa de
// aplicación. Emitir/Validar fallan explícitamente en vez de fingir emitir
// un token válido: solo afecta al login de un usuario para el que
// Identidad exige un segundo factor (RequiereSegundoFactor==true); el
// resto del flujo de autenticación no toca este puerto.
type emisorTokenStepUpPendiente struct{}

func (emisorTokenStepUpPendiente) Emitir(_ context.Context, _ string, _ string, _ time.Time) (accesopuertos.TokenStepUp, error) {
	return accesopuertos.TokenStepUp{}, fmt.Errorf("EmisorTokenStepUp: adaptador real pendiente de implementar (ADR 0038)")
}

func (emisorTokenStepUpPendiente) Validar(_ context.Context, _ string) (accesopuertos.ClaimsStepUp, error) {
	return accesopuertos.ClaimsStepUp{}, fmt.Errorf("EmisorTokenStepUp: adaptador real pendiente de implementar (ADR 0038)")
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
	tenenciahttp.RegistrarRutas(app, manejador, validadorAcceso, autorizador, alcance)

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
func construirEvaluadorDeRiesgo(cfg configuracion.Config) confianzapuertos.EvaluadorDeRiesgo {
	if cfg.URLRedis == "" {
		return nil
	}
	clienteRedis, err := cache.NuevoClienteRedis(cfg.URLRedis)
	if err != nil {
		log.Fatalf("api: REDIS_URL definido pero inválido: %v", err)
	}
	limitador := confianzaredis.NuevoLimitadorTasa(clienteRedis)
	verificadorCaptcha := turnstile.NuevoVerificadorCaptcha(cfg.TurnstileSecretKey, cfg.TurnstileVerifyURL, cfg.EntornoApp)
	evaluarTrustSignal := confianzaaplicacion.NuevoEvaluarTrustSignalCasoDeUso(limitador, verificadorCaptcha)
	slog.Info("api: motor de Confianza real montado (rate limiting por IP y por cuenta vía Redis + captcha Cloudflare Turnstile)",
		"redis_configurado", true, "turnstile_secret_configurado", cfg.TurnstileSecretKey != "")
	return evaluarTrustSignal
}

// construirEvaluadorConfianzaIdentidad decide entre EvaluadorConfianzaNoOp
// y EvaluadorConfianzaReal de Identidad según si riesgo es nil (ADR 0018).
func construirEvaluadorConfianzaIdentidad(riesgo confianzapuertos.EvaluadorDeRiesgo) identidadpuertos.EvaluadorConfianza {
	if riesgo == nil {
		return identidadconfianza.NuevoEvaluadorConfianzaNoOp(nil)
	}
	return identidadconfianza.NuevoEvaluadorConfianzaReal(riesgo)
}
