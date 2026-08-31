// Comando api arranca el servidor HTTP de Auth-as-a-Service. Además del
// health check de infraestructura, monta el bounded context Identidad
// completo: pool pgx, adaptadores concretos (postgres, cripto, auditoria,
// eventos, confianza, notificaciones), los cinco casos de uso de
// aplicacion (registro, autenticación, consulta, verificación de correo y
// reenvío de verificación), y sus cinco endpoints HTTP (Huma v2 sobre
// Fiber v2 vía humafiber.NewV2).
package main

import (
	"context"
	"log"
	"strconv"

	"github.com/gofiber/fiber/v2"

	"github.com/r-david1/moterus/internal/identidad/adaptadores/auditoria"
	"github.com/r-david1/moterus/internal/identidad/adaptadores/confianza"
	"github.com/r-david1/moterus/internal/identidad/adaptadores/cripto"
	"github.com/r-david1/moterus/internal/identidad/adaptadores/eventos"
	identidadhttp "github.com/r-david1/moterus/internal/identidad/adaptadores/http"
	"github.com/r-david1/moterus/internal/identidad/adaptadores/notificaciones"
	identidadpostgres "github.com/r-david1/moterus/internal/identidad/adaptadores/postgres"
	"github.com/r-david1/moterus/internal/identidad/aplicacion"
	"github.com/r-david1/moterus/internal/plataforma/bd"
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

	// Bug encontrado por el agente de calidad/testing (ver internal/identidad,
	// tarea de tests de integración): esta condición solo miraba
	// cfg.URLBaseDeDatos (DATABASE_URL), el DSN documentado como "solo para
	// herramientas administrativas (el migrador)" (ver configuracion.go). Un
	// despliegue que siguiera al pie de la letra ADR 0017 y solo definiera
	// DATABASE_URL_APLICACION (el DSN de runtime recomendado, rol_login_identidad)
	// sin definir también DATABASE_URL dejaba el contexto Identidad sin montar
	// silenciosamente — con el proceso igualmente arriba y /health respondiendo
	// 200, lo que hacía el fallo difícil de detectar en un despliegue real.
	if cfg.URLBaseDeDatos == "" && cfg.URLBaseDeDatosAplicacion == "" {
		log.Println("api: ni DATABASE_URL ni DATABASE_URL_APLICACION están definidos — solo se expone /health, el contexto Identidad no se monta")
	} else {
		montarIdentidad(ctx, app, cfg)
	}

	addr := ":" + strconv.Itoa(cfg.Puerto)
	log.Printf("auth-service escuchando en %s (env=%s)", addr, cfg.EntornoApp)
	if err := app.Listen(addr); err != nil {
		log.Fatalf("servidor: %v", err)
	}
}

// montarIdentidad conecta el pool pgx y ensambla los adaptadores concretos
// de Identidad detrás de sus puertos, construye los tres casos de uso de
// aplicacion (que sigue cerrada: aquí solo se inyectan sus dependencias por
// interfaz, nunca se modifica su código) y registra las rutas HTTP.
func montarIdentidad(ctx context.Context, app *fiber.App, cfg configuracion.Config) {
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

	repositorioUsuarios := identidadpostgres.NuevoRepositorioUsuarios(pool)
	repositorioTokensVerificacion := identidadpostgres.NuevoRepositorioTokensVerificacion(pool)
	unidadDeTrabajo := identidadpostgres.NuevaUnidadDeTrabajo(pool)
	generadorIDs := identidadpostgres.NuevoGeneradorIDs()

	hasher := cripto.NuevoHasherArgon2id()
	verificadorFiltradas := cripto.NuevoVerificadorHIBP()
	generadorTokens := cripto.NuevoGeneradorTokens()

	registroAuditoria := auditoria.NuevoRegistroAuditoria(pool)
	publicadorEventos := eventos.NuevoPublicadorLog(nil)
	evaluadorConfianza := confianza.NuevoEvaluadorConfianzaNoOp(nil)
	notificadorCorreo := notificaciones.NuevoNotificadorCorreoLog(nil)

	registrador := aplicacion.NuevoRegistrarUsuarioCasoDeUso(
		repositorioUsuarios,
		hasher,
		verificadorFiltradas,
		evaluadorConfianza,
		registroAuditoria,
		publicadorEventos,
		relojReal,
		generadorIDs,
		unidadDeTrabajo,
		generadorTokens,
		repositorioTokensVerificacion,
		notificadorCorreo,
	)
	autenticador := aplicacion.NuevoAutenticarUsuarioCasoDeUso(
		repositorioUsuarios,
		hasher,
		evaluadorConfianza,
		registroAuditoria,
		publicadorEventos,
		relojReal,
		unidadDeTrabajo,
	)
	consultor := aplicacion.NuevoObtenerUsuarioCasoDeUso(
		repositorioUsuarios,
		registroAuditoria,
		relojReal,
	)
	verificadorCorreo := aplicacion.NuevoVerificarCorreoCasoDeUso(
		repositorioUsuarios,
		repositorioTokensVerificacion,
		registroAuditoria,
		publicadorEventos,
		relojReal,
		unidadDeTrabajo,
	)
	reenviadorVerificacion := aplicacion.NuevoReenviarVerificacionCasoDeUso(
		repositorioUsuarios,
		generadorTokens,
		repositorioTokensVerificacion,
		notificadorCorreo,
		relojReal,
	)

	manejador := identidadhttp.NuevoManejadorIdentidad(registrador, autenticador, consultor, verificadorCorreo, reenviadorVerificacion)
	identidadhttp.RegistrarRutas(app, manejador)

	log.Println("api: contexto Identidad montado (postgres, argon2id, hibp, auditoria, eventos-log, confianza-noop, notificador-correo-log)")
}
