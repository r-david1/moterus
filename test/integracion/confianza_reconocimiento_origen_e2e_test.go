// Este archivo cubre la batería de tests de integración end-to-end que
// exige §10 paso 10 del diseño docs/design/fingerprinting-comportamiento.md
// (reconocimiento de origen, tercera extensión de Confianza), contra
// Postgres y Redis REALES y el servidor HTTP completo (Fiber+Huma), sin
// mocks:
//
//  1. Login exitoso repetido desde la misma huella no produce ningún
//     cambio observable (INV-RIES-09) y el perfil de Redis acumula el
//     historial esperado (verificación de caja blanca explícita).
//  2. Una huella distinta produce dispositivo_desconocido y una fila
//     origen.nuevo en auditoria, con la cadena de hashes íntegra.
//  3. Tres intentos fallidos desde una huella nueva NUNCA la promueven
//     (INV-RIES-05, "el test que más importa de todos" según el propio
//     diseño), verificado en caja blanca contra Redis y confirmado en
//     caja negra con un login exitoso posterior desde la misma huella.
//  4. modo=exigir_captcha con una política que fuerza nivel>=elevado con
//     una sola señal deniega con 429, y el mismo intento con un captcha
//     ya aceptable en el cuerpo pasa (bypass).
//  5. Ningún campo de riesgo (puntaje_riesgo, nivel_riesgo,
//     senales_de_riesgo, ni el nombre de ninguna señal) aparece en el
//     JSON crudo de ninguna respuesta HTTP, exitosa o denegada
//     (INV-RIES-09).
//  6. Un PerfilDeOrigenes que falla (Redis caído, a los efectos de este
//     mecanismo puntual) no rompe el login (INV-RIES-03, fail-open
//     incondicional).
//
// Los tests de dominio (internal/confianza/dominio/*_test.go, sin mocks),
// de aplicación (internal/confianza/aplicacion/reconocimiento_origen_test.go,
// con mocks vía puertos/mocks), del adaptador Redis
// (test/integracion/confianza_perfil_origenes_redis_test.go) y de los tres
// ACL ya cubren cada regla de negocio y cada camino de error por separado;
// este archivo se queda deliberadamente en la superficie HTTP real, que es
// la única forma de verificar de punta a punta que el cableado de
// cmd/api/main.go (perfiles + política + auditoría, todos inyectados vía
// las opciones funcionales de EvaluarTrustSignalCasoDeUso) efectivamente
// llega desde la cabecera X-Device-Fingerprint hasta la fila de auditoria y
// el cuerpo de la respuesta HTTP.
//
// Los tests 1, 2, 3 y 6 usan el servidor de SOLO Identidad
// (nuevoServidorIdentidadReconocimientoOrigen, POST
// /identidad/autenticaciones — el consumidor real que nombra el diseño,
// §0), porque ese endpoint no necesita nada de Acceso para ejercitar el
// paso 2.5 y la promoción. Los tests 4 y 5 necesitan el captcha invisible
// (token_captcha), que SOLO viaja por POST /acceso/sesiones (ver
// acceso/adaptadores/http/dtos.go), así que usan el servidor completo
// Identidad+Acceso (nuevoServidorAccesoReconocimientoOrigen).
//
// Todos los tests requieren DATABASE_URL_APLICACION y REDIS_URL; se omiten
// limpiamente con t.Skip si faltan (mismo criterio que el resto de este
// paquete). El estado que dejan en Redis (la clave confianza:orig:<hash>
// de cada cuenta de prueba) se limpia con t.Cleanup en cada test.
//
// La prueba de carga con k6 de §10 queda fuera de este archivo: k6 no está
// disponible en este entorno (documentado aparte, mismo criterio que
// confianza_colas_virtuales_e2e_test.go).
package integracion

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	accesoauditoria "github.com/r-david1/moterus/internal/acceso/adaptadores/auditoria"
	accesoconfianza "github.com/r-david1/moterus/internal/acceso/adaptadores/confianza"
	accesocripto "github.com/r-david1/moterus/internal/acceso/adaptadores/cripto"
	accesoeventos "github.com/r-david1/moterus/internal/acceso/adaptadores/eventos"
	accesohttp "github.com/r-david1/moterus/internal/acceso/adaptadores/http"
	accesoidentidad "github.com/r-david1/moterus/internal/acceso/adaptadores/identidad"
	accesojwt "github.com/r-david1/moterus/internal/acceso/adaptadores/jwt"
	accesopostgres "github.com/r-david1/moterus/internal/acceso/adaptadores/postgres"
	accesoredis "github.com/r-david1/moterus/internal/acceso/adaptadores/redis"
	accesoaplicacion "github.com/r-david1/moterus/internal/acceso/aplicacion"
	accesodominio "github.com/r-david1/moterus/internal/acceso/dominio"

	confianzaauditoria "github.com/r-david1/moterus/internal/confianza/adaptadores/auditoria"
	confianzaredis "github.com/r-david1/moterus/internal/confianza/adaptadores/redis"
	"github.com/r-david1/moterus/internal/confianza/adaptadores/turnstile"
	confianzaaplicacion "github.com/r-david1/moterus/internal/confianza/aplicacion"
	confianzadominio "github.com/r-david1/moterus/internal/confianza/dominio"
	confianzapuertos "github.com/r-david1/moterus/internal/confianza/puertos"

	"github.com/r-david1/moterus/internal/identidad/adaptadores/auditoria"
	identidadconfianza "github.com/r-david1/moterus/internal/identidad/adaptadores/confianza"
	"github.com/r-david1/moterus/internal/identidad/adaptadores/cripto"
	"github.com/r-david1/moterus/internal/identidad/adaptadores/eventos"
	identidadhttp "github.com/r-david1/moterus/internal/identidad/adaptadores/http"
	"github.com/r-david1/moterus/internal/identidad/adaptadores/notificaciones"
	identidadpostgres "github.com/r-david1/moterus/internal/identidad/adaptadores/postgres"
	identidadaplicacion "github.com/r-david1/moterus/internal/identidad/aplicacion"

	"github.com/r-david1/moterus/internal/plataforma/cache"
	"github.com/r-david1/moterus/internal/plataforma/reloj"
)

// --- entorno compartido: Redis real para el reconocimiento de origen -------

// clienteYPerfilesRedisDePrueba abre un *goredis.Client sobre REDIS_URL (se
// omite limpiamente si no está definido) y construye encima el adaptador
// real confianzaredis.PerfilOrigenes. Devuelve también el cliente crudo
// porque varios tests de este archivo necesitan inspeccionar la clave
// "confianza:orig:<claveCuenta>" directamente (caja blanca) además de
// ejercitar el mecanismo por HTTP (caja negra) — exactamente lo que pide
// el punto 1 del encargo.
func clienteYPerfilesRedisDePrueba(t *testing.T) (*goredis.Client, *confianzaredis.PerfilOrigenes) {
	t.Helper()
	url := os.Getenv("REDIS_URL")
	if url == "" {
		t.Skip("REDIS_URL no está definido: se omiten los tests e2e de reconocimiento de origen " +
			"(requieren Redis real levantado, ver deployments/docker-compose.yml)")
	}
	cliente, err := cache.NuevoClienteRedis(url)
	if err != nil {
		t.Fatalf("cache.NuevoClienteRedis: %v", err)
	}
	if err := cliente.Ping(context.Background()).Err(); err != nil {
		t.Fatalf("no se pudo conectar a Redis en %s: %v", url, err)
	}
	t.Cleanup(func() { _ = cliente.Close() })
	return cliente, confianzaredis.NuevoPerfilDeOrigenes(cliente)
}

// claveRedisOrigenDeCorreo reconstruye el nombre exacto de la clave de
// Redis que confianzaredis.PerfilOrigenes usa para una cuenta (§5.2 del
// diseño: "confianza:orig:<claveCuenta>", con claveCuenta = SHA-256 del
// correo YA normalizado). normalizarCorreoLaxo (identidad/adaptadores/
// confianza/evaluador_confianza_real.go) hace strings.ToLower+TrimSpace, y
// correoUnico (entorno_test.go) ya genera correos en minúsculas, así que
// replicar esa misma normalización aquí basta para que la clave coincida
// byte a byte con la que escribe el proceso bajo prueba.
func claveRedisOrigenDeCorreo(correo string) string {
	normalizado := strings.ToLower(strings.TrimSpace(correo))
	return "confianza:orig:" + confianzadominio.NuevaClaveCuenta(normalizado).String()
}

// campoRedisDispositivo reconstruye el nombre de campo "d:<hash16>" que el
// script `registrar` escribe para una huella cruda dada (§5.2 del diseño).
func campoRedisDispositivo(huellaCruda string) string {
	return "d:" + confianzadominio.HashearHuella(huellaCruda).String()
}

// limpiarPerfilOrigenDePrueba borra la clave de Redis de una cuenta de
// prueba al finalizar el test, mismo criterio de limpieza que el resto de
// este paquete (borrarUsuario, limpiarSesionesDeUsuario).
func limpiarPerfilOrigenDePrueba(t *testing.T, cliente *goredis.Client, correo string) {
	t.Helper()
	t.Cleanup(func() {
		if err := cliente.Del(context.Background(), claveRedisOrigenDeCorreo(correo)).Err(); err != nil {
			t.Logf("no se pudo limpiar el perfil de origen de prueba de %q: %v", correo, err)
		}
	})
}

// opcionesReconocimientoOrigenDePrueba arma las opciones funcionales
// comunes que necesita EvaluarTrustSignalCasoDeUso para tener la extensión
// de reconocimiento de origen activa de punta a punta: el puerto de
// perfiles ya conectado, la auditoría real (confianza/adaptadores/
// auditoria, mismo pool y misma tabla que el resto del sistema) y el reloj
// real — exactamente lo que cmd/api/main.go#construirEvaluadorDeRiesgo
// monta cuando REDIS_URL está configurado. politicaOpcional puede ir vacía
// (zero value): en ese caso NuevoEvaluarTrustSignalCasoDeUso arranca con
// dominio.PoliticaRiesgoPorDefecto() (modo observar, §1.5 del diseño).
func opcionesReconocimientoOrigenDePrueba(pool *pgxpool.Pool, perfiles confianzapuertos.PerfilDeOrigenes, politicaOpcional confianzadominio.PoliticaRiesgo) []confianzaaplicacion.OpcionEvaluarTrustSignal {
	opciones := []confianzaaplicacion.OpcionEvaluarTrustSignal{
		confianzaaplicacion.ConPerfilesDeOrigen(perfiles),
		confianzaaplicacion.ConAuditoriaDeRiesgo(confianzaauditoria.NuevoRegistroAuditoria(pool)),
		confianzaaplicacion.ConRelojDeRiesgo(reloj.NuevoReal()),
	}
	if !politicaOpcional.EsVacia() {
		opciones = append(opciones, confianzaaplicacion.ConPoliticaRiesgo(politicaOpcional))
	}
	return opciones
}

// politicaRiesgoExigirCaptchaDePrueba construye una PoliticaRiesgo donde
// UNA sola señal (dispositivo_desconocido) ya alcanza NivelRiesgoElevado,
// en modo exigir_captcha — mismo criterio exacto que
// TestEvaluarRiesgoDeOrigen_INV_RIES_02_NuncaProduceRequiereStepUp
// (internal/confianza/aplicacion/reconocimiento_origen_test.go):
// pesoDispositivoDesconocido=1.0 con umbralElevado=0.1 fuerza el nivel con
// una sola señal, sin depender de red_desconocida (peso 0), que en un
// entorno de test HTTP no se controla tan finamente como la huella.
func politicaRiesgoExigirCaptchaDePrueba(t *testing.T) confianzadominio.PoliticaRiesgo {
	t.Helper()
	politica, err := confianzadominio.NuevaPoliticaRiesgo(
		1.0, 0.0, 0.0,
		0.1, 0.2,
		3,
		confianzadominio.ModoRiesgoExigirCaptcha,
		180*24*time.Hour,
		20,
	)
	if err != nil {
		t.Fatalf("política de riesgo de prueba inválida: %v", err)
	}
	return politica
}

// --- servidor de prueba: SOLO Identidad -------------------------------------

// nuevoServidorIdentidadReconocimientoOrigen reproduce el mismo cableado que
// nuevoServidorIdentidadConConfianzaReal (entorno_test.go) pero además
// construye EvaluarTrustSignalCasoDeUso con las opciones de reconocimiento
// de origen que se le pasen — típicamente las de
// opcionesReconocimientoOrigenDePrueba. Se omite limpiamente si REDIS_URL
// no está definido, mismo criterio que el resto de este paquete.
func nuevoServidorIdentidadReconocimientoOrigen(t *testing.T, pool *pgxpool.Pool, opciones ...confianzaaplicacion.OpcionEvaluarTrustSignal) *fiber.App {
	t.Helper()

	url := os.Getenv("REDIS_URL")
	if url == "" {
		t.Skip("REDIS_URL no está definido: se omiten los tests e2e de reconocimiento de origen")
	}
	clienteRedisCrudo, err := cache.NuevoClienteRedis(url)
	if err != nil {
		t.Fatalf("no se pudo construir el cliente Redis: %v", err)
	}
	t.Cleanup(func() { _ = clienteRedisCrudo.Close() })
	if err := clienteRedisCrudo.Ping(context.Background()).Err(); err != nil {
		t.Fatalf("no se pudo conectar a Redis en %s: %v", url, err)
	}

	relojReal := reloj.NuevoReal()

	repositorioUsuarios := identidadpostgres.NuevoRepositorioUsuarios(pool)
	repositorioTokensVerificacion := identidadpostgres.NuevoRepositorioTokensVerificacion(pool)
	unidadDeTrabajo := identidadpostgres.NuevaUnidadDeTrabajo(pool)
	generadorIDs := identidadpostgres.NuevoGeneradorIDs()

	hasher := cripto.NuevoHasherArgon2id()
	verificadorFiltradas := cripto.NuevoVerificadorHIBP(cripto.ConBaseURLHIBP(servidorHIBPPruebas.URL + "/range/"))
	generadorTokens := cripto.NuevoGeneradorTokens()

	loggerSilencioso := slog.New(slog.NewTextHandler(io.Discard, nil))
	registroAuditoria := auditoria.NuevoRegistroAuditoria(pool)
	publicadorEventos := eventos.NuevoPublicadorLog(loggerSilencioso)
	notificadorCorreo := notificaciones.NuevoNotificadorCorreoLog(loggerSilencioso)

	// Mismo ensamblaje que construirEvaluadorConfianza en cmd/api/main.go,
	// más las opciones de reconocimiento de origen del llamador
	// (construirEvaluadorDeRiesgo): un LimitadorTasa real sobre Redis + un
	// VerificadorCaptcha en modo fail-open de desarrollo (sin secretKey,
	// entornoApp != "production" — Verificar siempre devuelve 1.0 sin
	// llamar a la red, ver turnstile.NuevoVerificadorCaptcha).
	limitador := confianzaredis.NuevoLimitadorTasa(clienteRedisCrudo)
	verificadorCaptcha := turnstile.NuevoVerificadorCaptcha("", "", "development")
	evaluarTrustSignal := confianzaaplicacion.NuevoEvaluarTrustSignalCasoDeUso(limitador, verificadorCaptcha, opciones...)
	evaluadorConfianza := identidadconfianza.NuevoEvaluadorConfianzaReal(confianzapuertos.EvaluadorDeRiesgo(evaluarTrustSignal))

	registrador := identidadaplicacion.NuevoRegistrarUsuarioCasoDeUso(
		repositorioUsuarios, hasher, verificadorFiltradas, evaluadorConfianza,
		registroAuditoria, publicadorEventos, relojReal, generadorIDs, unidadDeTrabajo,
		generadorTokens, repositorioTokensVerificacion, notificadorCorreo,
	)
	autenticador := identidadaplicacion.NuevoAutenticarUsuarioCasoDeUso(
		repositorioUsuarios, hasher, evaluadorConfianza,
		registroAuditoria, publicadorEventos, relojReal, unidadDeTrabajo,
	)
	consultor := identidadaplicacion.NuevoObtenerUsuarioCasoDeUso(repositorioUsuarios, registroAuditoria, relojReal)
	verificadorCorreo := identidadaplicacion.NuevoVerificarCorreoCasoDeUso(
		repositorioUsuarios, repositorioTokensVerificacion, registroAuditoria, publicadorEventos, relojReal, unidadDeTrabajo,
	)
	reenviadorVerificacion := identidadaplicacion.NuevoReenviarVerificacionCasoDeUso(
		repositorioUsuarios, generadorTokens, repositorioTokensVerificacion, notificadorCorreo, relojReal,
	)

	manejador := identidadhttp.NuevoManejadorIdentidad(registrador, autenticador, consultor, verificadorCorreo, reenviadorVerificacion, evaluadorConfianza, nil, nil)

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	identidadhttp.RegistrarRutas(app, manejador, nil, nil)
	return app
}

// --- servidor de prueba: Identidad + Acceso (necesario para token_captcha) -

// nuevoServidorAccesoReconocimientoOrigen reproduce el mismo cableado que
// nuevoServidorAccesoInterno (acceso_test.go), sin MFA (innecesario para
// estos tests: INV-RIES-02 garantiza que este mecanismo nunca produce
// RequiereStepUp, así que ningún login de este archivo puede terminar
// pidiendo un segundo factor), con la extensión de reconocimiento de
// origen activa en el evaluador de Confianza de IDENTIDAD — que es quien
// evalúa Confianza durante el login (ver el comentario de cabecera de
// acceso/adaptadores/confianza/evaluador_confianza.go: "Acceso lo consulta
// en RenovarSesion y en CerrarTodas... en login no, porque Identidad ya lo
// hizo dentro de AutenticarUsuario"). Existe porque
// POST /acceso/sesiones es el ÚNICO endpoint que acepta token_captcha en
// el cuerpo (acceso/adaptadores/http/dtos.go) — POST
// /identidad/autenticaciones no lo tiene.
func nuevoServidorAccesoReconocimientoOrigen(t *testing.T, pool *pgxpool.Pool, opciones ...confianzaaplicacion.OpcionEvaluarTrustSignal) *fiber.App {
	t.Helper()

	url := os.Getenv("REDIS_URL")
	if url == "" {
		t.Skip("REDIS_URL no está definido: se omiten los tests e2e de reconocimiento de origen")
	}
	clienteRedisCrudo, err := cache.NuevoClienteRedis(url)
	if err != nil {
		t.Fatalf("no se pudo construir el cliente Redis: %v", err)
	}
	t.Cleanup(func() { _ = clienteRedisCrudo.Close() })
	if err := clienteRedisCrudo.Ping(context.Background()).Err(); err != nil {
		t.Fatalf("no se pudo conectar a Redis en %s: %v", url, err)
	}

	relojReal := reloj.NuevoReal()
	loggerSilencioso := slog.New(slog.NewTextHandler(io.Discard, nil))

	// --- Identidad -----------------------------------------------------------
	repositorioUsuarios := identidadpostgres.NuevoRepositorioUsuarios(pool)
	repositorioTokensVerificacion := identidadpostgres.NuevoRepositorioTokensVerificacion(pool)
	unidadDeTrabajoIdentidad := identidadpostgres.NuevaUnidadDeTrabajo(pool)
	generadorIDsIdentidad := identidadpostgres.NuevoGeneradorIDs()

	hasher := cripto.NuevoHasherArgon2id()
	verificadorFiltradas := cripto.NuevoVerificadorHIBP(cripto.ConBaseURLHIBP(servidorHIBPPruebas.URL + "/range/"))
	generadorTokensIdentidad := cripto.NuevoGeneradorTokens()

	registroAuditoriaIdentidad := auditoria.NuevoRegistroAuditoria(pool)
	publicadorEventosIdentidad := eventos.NuevoPublicadorLog(loggerSilencioso)
	notificadorCorreo := notificaciones.NuevoNotificadorCorreoLog(loggerSilencioso)

	limitador := confianzaredis.NuevoLimitadorTasa(clienteRedisCrudo)
	verificadorCaptcha := turnstile.NuevoVerificadorCaptcha("", "", "development")
	evaluarTrustSignal := confianzaaplicacion.NuevoEvaluarTrustSignalCasoDeUso(limitador, verificadorCaptcha, opciones...)
	evaluadorConfianzaIdentidad := identidadconfianza.NuevoEvaluadorConfianzaReal(confianzapuertos.EvaluadorDeRiesgo(evaluarTrustSignal))

	registrador := identidadaplicacion.NuevoRegistrarUsuarioCasoDeUso(
		repositorioUsuarios, hasher, verificadorFiltradas, evaluadorConfianzaIdentidad,
		registroAuditoriaIdentidad, publicadorEventosIdentidad, relojReal, generadorIDsIdentidad, unidadDeTrabajoIdentidad,
		generadorTokensIdentidad, repositorioTokensVerificacion, notificadorCorreo,
	)
	autenticadorIdentidad := identidadaplicacion.NuevoAutenticarUsuarioCasoDeUso(
		repositorioUsuarios, hasher, evaluadorConfianzaIdentidad,
		registroAuditoriaIdentidad, publicadorEventosIdentidad, relojReal, unidadDeTrabajoIdentidad,
	)
	consultorIdentidad := identidadaplicacion.NuevoObtenerUsuarioCasoDeUso(repositorioUsuarios, registroAuditoriaIdentidad, relojReal)
	verificadorCorreo := identidadaplicacion.NuevoVerificarCorreoCasoDeUso(
		repositorioUsuarios, repositorioTokensVerificacion, registroAuditoriaIdentidad, publicadorEventosIdentidad, relojReal, unidadDeTrabajoIdentidad,
	)
	reenviadorVerificacion := identidadaplicacion.NuevoReenviarVerificacionCasoDeUso(
		repositorioUsuarios, generadorTokensIdentidad, repositorioTokensVerificacion, notificadorCorreo, relojReal,
	)

	manejadorIdentidad := identidadhttp.NuevoManejadorIdentidad(
		registrador, autenticadorIdentidad, consultorIdentidad, verificadorCorreo, reenviadorVerificacion, evaluadorConfianzaIdentidad, nil, nil,
	)

	// --- Acceso ----------------------------------------------------------------
	politica := accesodominio.PoliticaSesionPorDefecto()

	sesiones := accesopostgres.NuevoRepositorioSesiones(pool)
	uow := accesopostgres.NuevaUnidadDeTrabajo(pool)
	generadorIDs := accesopostgres.NuevoGeneradorIDs()
	generadorRefrescos := accesocripto.NuevoGeneradorTokensRefresco()
	registroAuditoriaAcceso := accesoauditoria.NuevoRegistroAuditoria(pool)
	publicadorEventosAcceso := accesoeventos.NuevoPublicadorLog(loggerSilencioso)

	llaveEfimera, err := accesojwt.GenerarLlaveEfimera()
	if err != nil {
		t.Fatalf("GenerarLlaveEfimera(): %v", err)
	}
	llavero, err := accesojwt.NuevoLlavero(llaveEfimera, nil)
	if err != nil {
		t.Fatalf("NuevoLlavero(): %v", err)
	}
	firmador := accesojwt.NuevoFirmador(llavero, "https://acceso.test.moterus.local", "moterus-test", politica.ToleranciaReloj())
	listaRevocacion := accesoredis.NuevaListaRevocacion(clienteRedisCrudo)

	// evaluadorConfianzaAcceso: no-op a propósito. El comentario de cabecera
	// de esta función explica por qué el login no lo consulta; solo
	// RenovarSesion/CerrarTodas lo harían, y ningún test de este archivo
	// ejercita esos dos caminos.
	evaluadorConfianzaAcceso := accesoconfianza.NuevoEvaluadorConfianzaNoOp(loggerSilencioso)

	autenticadorACL := accesoidentidad.NuevoAutenticadorIdentidad(autenticadorIdentidad)
	consultorEstadoSujeto := accesoidentidad.NuevoConsultorEstadoSujeto(consultorIdentidad)

	// emisorStepUp: adaptador real, construido solo para satisfacer la
	// firma de NuevoIniciarSesionCasoDeUso — INV-RIES-02 garantiza que
	// nunca se invoca en estos tests (este mecanismo nunca produce
	// RequiereStepUp, y ningún usuario de este archivo tiene MFA propio).
	emisorStepUp := accesojwt.NuevoEmisorTokenStepUp(llavero, politica.ToleranciaReloj())

	iniciador := accesoaplicacion.NuevoIniciarSesionCasoDeUso(
		autenticadorACL, sesiones, generadorRefrescos, firmador, listaRevocacion,
		registroAuditoriaAcceso, publicadorEventosAcceso, relojReal, generadorIDs, uow, politica,
		"https://acceso.test.moterus.local", "moterus-test",
		emisorStepUp,
	)
	renovador := accesoaplicacion.NuevoRenovarSesionCasoDeUso(
		evaluadorConfianzaAcceso, sesiones, generadorRefrescos, firmador, consultorEstadoSujeto, listaRevocacion,
		registroAuditoriaAcceso, relojReal, generadorIDs, uow, politica,
		"https://acceso.test.moterus.local", "moterus-test",
	)
	validador := accesoaplicacion.NuevoValidarAccesoCasoDeUso(firmador, listaRevocacion, sesiones, registroAuditoriaAcceso, relojReal)
	cerrador := accesoaplicacion.NuevoCerrarSesionCasoDeUso(sesiones, evaluadorConfianzaAcceso, listaRevocacion, registroAuditoriaAcceso, relojReal, uow, politica)
	consultorSesiones := accesoaplicacion.NuevoListarSesionesCasoDeUso(sesiones)

	// completadorSegundoFactor: nil a propósito, ningún test de este
	// archivo llama a POST /acceso/sesiones/segundo-factor.
	manejadorAcceso := accesohttp.NuevoManejadorAcceso(iniciador, renovador, cerrador, consultorSesiones, firmador, nil)

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	accesohttp.RegistrarRutas(app, manejadorAcceso, validador, nil)
	identidadhttp.RegistrarRutas(app, manejadorIdentidad, validador, nil)
	return app
}

// --- helpers HTTP propios de este archivo -----------------------------------

// reiniciarLimitesIPDePrueba reinicia los contadores de rate limiting POR
// IP de las acciones "login" y "registro" (ADR 0018,
// confianza/dominio.PoliticaLimitesPorDefecto: login 5/1min, registro
// 10/1min) para la IP "0.0.0.0" — la que c.IP() resuelve SIEMPRE para
// cualquier request de este paquete, porque httptest.NewRequest (usado por
// app.Test) nunca fija RemoteAddr. Esto no es una suposición: ya está
// verificado empíricamente y documentado en
// TestHTTP_ReenviarVerificacion_GuardianDePerimetroReal_BloqueaTrasElUmbral
// (verificacion_correo_test.go), que reinicia la clave análoga de
// reenvio_verificacion por el mismo motivo. Sin este reinicio antes de
// CADA registro/login, las llamadas que hace cada test de este archivo
// (algunas superan 5 logins por sí solas: 3 de calentamiento + 3 fallidos
// + 1 final) y las de otros tests de este paquete que compartan la misma
// corrida competirían por el mismo cupo de IP y producirían 429 por el
// limitador de tasa de ADR 0018 — un mecanismo que este archivo
// deliberadamente NO está ejercitando (eso ya lo cubre
// confianza_redis_test.go y verificacion_correo_test.go) — en vez de por
// el reconocimiento de origen, contaminando el resultado de estos tests.
func reiniciarLimitesIPDePrueba(t *testing.T, limitador *confianzaredis.LimitadorTasa) {
	t.Helper()
	for _, clave := range []string{"confianza:rl:ip:login:0.0.0.0", "confianza:rl:ip:registro:0.0.0.0"} {
		if err := limitador.Reiniciar(context.Background(), clave); err != nil {
			t.Logf("no se pudo reiniciar %s: %v", clave, err)
		}
	}
}

// usuarioActivoDePruebaSinLimiteDeIP envuelve usuarioActivoDePrueba
// (acceso_test.go) reiniciando antes el límite de IP de "registro" (ver
// reiniciarLimitesIPDePrueba): sin esto, el registro de usuario de cada
// test competiría por el mismo cupo de 10/min compartido con el resto del
// paquete en la misma corrida.
func usuarioActivoDePruebaSinLimiteDeIP(t *testing.T, app *fiber.App, limitador *confianzaredis.LimitadorTasa, poolDueno *pgxpool.Pool, prefijo string) (idUsuario, correo string) {
	t.Helper()
	reiniciarLimitesIPDePrueba(t, limitador)
	return usuarioActivoDePrueba(t, app, poolDueno, prefijo)
}

// loginIdentidadConHuella hace POST /identidad/autenticaciones con la
// cabecera X-Device-Fingerprint fijada a huella (si no está vacía) y
// decodifica la respuesta en autenticarRespuestaPrueba (http_flujo_test.go).
// Reinicia el límite de IP de login antes de cada llamada (ver
// reiniciarLimitesIPDePrueba): este archivo aísla deliberadamente el
// reconocimiento de origen del limitador de tasa por IP de ADR 0018.
func loginIdentidadConHuella(t *testing.T, app *fiber.App, limitador *confianzaredis.LimitadorTasa, correo, contrasena, huella string) (int, autenticarRespuestaPrueba) {
	t.Helper()
	reiniciarLimitesIPDePrueba(t, limitador)
	req := peticionJSON(t, http.MethodPost, "/identidad/autenticaciones", autenticarPeticionPrueba{
		Correo: correo, Contrasena: contrasena,
	})
	if huella != "" {
		req.Header.Set("X-Device-Fingerprint", huella)
	}
	var resultado autenticarRespuestaPrueba
	status := respuestaHTTP(t, app, req, &resultado)
	return status, resultado
}

// iniciarSesionPeticionConCaptchaDePrueba es el cuerpo de POST
// /acceso/sesiones incluyendo token_captcha (resultadoSesionDePrueba /
// iniciarSesionDePrueba, en acceso_test.go, no lo transportan porque
// ningún test de ese archivo lo necesita).
type iniciarSesionPeticionConCaptchaDePrueba struct {
	Correo       string `json:"correo"`
	Contrasena   string `json:"contrasena"`
	TokenCaptcha string `json:"token_captcha,omitempty"`
}

// crudoHTTP ejecuta req contra app y devuelve el status, el cuerpo SIN
// decodificar y las cabeceras — a diferencia de respuestaHTTP
// (entorno_test.go), que descarta ambas cosas. Lo necesitan los tests que
// inspeccionan el JSON crudo (INV-RIES-09) o una cabecera de respuesta
// (Retry-After).
func crudoHTTP(t *testing.T, app *fiber.App, req *http.Request) (int, []byte, http.Header) {
	t.Helper()
	resp, err := app.Test(req, 10_000)
	if err != nil {
		t.Fatalf("app.Test(%s %s): %v", req.Method, req.URL.Path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	crudo, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("leyendo el cuerpo de la respuesta: %v", err)
	}
	return resp.StatusCode, crudo, resp.Header
}

// iniciarSesionConOrigen construye la petición HTTP de login (POST
// /acceso/sesiones) con huella y token_captcha propios, y devuelve la
// respuesta cruda (crudoHTTP) para que el llamador decida si decodificarla
// o inspeccionarla como texto.
func iniciarSesionConOrigen(t *testing.T, app *fiber.App, limitador *confianzaredis.LimitadorTasa, correo, contrasena, huella, tokenCaptcha string) (int, []byte, http.Header) {
	t.Helper()
	reiniciarLimitesIPDePrueba(t, limitador)
	req := peticionJSON(t, http.MethodPost, "/acceso/sesiones", iniciarSesionPeticionConCaptchaDePrueba{
		Correo: correo, Contrasena: contrasena, TokenCaptcha: tokenCaptcha,
	})
	if huella != "" {
		req.Header.Set("X-Device-Fingerprint", huella)
	}
	return crudoHTTP(t, app, req)
}

// =============================================================================
// 1. Login exitoso repetido desde la misma huella: la respuesta no cambia y
//    el perfil de Redis acumula el historial esperado (caja blanca).
// =============================================================================

// TestReconocimientoOrigen_MismaHuellaTrasHistorialMinimo_NoCambiaLaRespuesta
// hace tres logins exitosos desde "huella-A" (dominio.PoliticaRiesgoPorDefecto
// ().MinimoExitosParaJuzgar() == 3), confirma en Redis que el perfil
// acumuló exactamente ese historial y que el campo d:<hash de huella-A>
// quedó registrado, y luego hace un cuarto login también desde huella-A: la
// respuesta HTTP debe tener EXACTAMENTE la misma forma que las anteriores
// (sin RequiereSegundoFactor, sin ningún campo nuevo — INV-RIES-09), porque
// un dispositivo ya conocido nunca produce dispositivo_desconocido.
func TestReconocimientoOrigen_MismaHuellaTrasHistorialMinimo_NoCambiaLaRespuesta(t *testing.T) {
	pool := poolAplicacion(t)
	dueno := poolDueno(t)
	clienteRedisCrudo, perfiles := clienteYPerfilesRedisDePrueba(t)
	limitador := clienteRedis(t)

	app := nuevoServidorIdentidadReconocimientoOrigen(t, pool,
		opcionesReconocimientoOrigenDePrueba(pool, perfiles, confianzadominio.PoliticaRiesgo{})...,
	)

	idUsuario, correo := usuarioActivoDePruebaSinLimiteDeIP(t, app, limitador, dueno, "origen-misma-huella")
	t.Cleanup(func() { borrarUsuario(t, dueno, idUsuario) })
	limpiarPerfilOrigenDePrueba(t, clienteRedisCrudo, correo)

	const minimoExitosParaJuzgar = 3 // dominio.PoliticaRiesgoPorDefecto()
	var ultima autenticarRespuestaPrueba
	for i := 0; i < minimoExitosParaJuzgar; i++ {
		status, resultado := loginIdentidadConHuella(t, app, limitador, correo, contrasenaFuerteDePrueba, "huella-A")
		if status != http.StatusOK {
			t.Fatalf("login de calentamiento %d desde huella-A: status=%d, esperado 200", i, status)
		}
		ultima = resultado
	}

	// Caja blanca: el perfil de Redis acumuló los 3 éxitos y el campo de
	// dispositivo de huella-A quedó presente.
	clave := claveRedisOrigenDeCorreo(correo)
	campos, err := clienteRedisCrudo.HGetAll(context.Background(), clave).Result()
	if err != nil {
		t.Fatalf("HGETALL %s: %v", clave, err)
	}
	if campos["#exitos"] != strconv.Itoa(minimoExitosParaJuzgar) {
		t.Fatalf("#exitos = %q, esperado %d tras %d logins exitosos", campos["#exitos"], minimoExitosParaJuzgar, minimoExitosParaJuzgar)
	}
	campoHuellaA := campoRedisDispositivo("huella-A")
	if _, ok := campos[campoHuellaA]; !ok {
		t.Fatalf("el campo %q no está presente tras %d logins exitosos desde huella-A: %v", campoHuellaA, minimoExitosParaJuzgar, campos)
	}

	// Cuarto login, mismo dispositivo: la forma de la respuesta no cambia.
	status, cuarta := loginIdentidadConHuella(t, app, limitador, correo, contrasenaFuerteDePrueba, "huella-A")
	if status != http.StatusOK {
		t.Fatalf("cuarto login desde huella-A: status=%d, esperado 200", status)
	}
	if cuarta.RequiereSegundoFactor {
		t.Fatalf("un dispositivo ya conocido no debería requerir segundo factor: %+v", cuarta)
	}
	if cuarta.Estado != ultima.Estado || cuarta.CorreoNormalizado != ultima.CorreoNormalizado {
		t.Fatalf("la forma de la respuesta cambió entre logins desde el mismo dispositivo: antes=%+v después=%+v", ultima, cuarta)
	}

	camposFinal, err := clienteRedisCrudo.HGetAll(context.Background(), clave).Result()
	if err != nil {
		t.Fatalf("HGETALL %s (tras el cuarto login): %v", clave, err)
	}
	if camposFinal["#exitos"] != "4" {
		t.Fatalf("#exitos tras el cuarto login = %q, esperado 4", camposFinal["#exitos"])
	}

	assertCadenaAuditoriaIntegra(t, dueno)
}

// =============================================================================
// 2. Una huella distinta produce dispositivo_desconocido y una fila
//    origen.nuevo en auditoria, con la cadena de hashes íntegra.
// =============================================================================

// TestReconocimientoOrigen_HuellaDistinta_AuditaOrigenNuevo repite el
// historial mínimo desde huella-A y después hace un login exitoso desde
// huella-B (nunca vista): confirma que aparece exactamente una fila
// auditoria con accion='origen.nuevo', usuario_id correcto y
// huella_dispositivo='huella-B' (§1.6/INV-RIES-13 del diseño), que la
// cadena de hashes de auditoria sigue íntegra
// (verificar_cadena_auditoria(), mismo verificador que usa el resto de
// este paquete — ver assertCadenaAuditoriaIntegra en entorno_test.go), y
// que la respuesta HTTP del login sigue sin requerir segundo factor
// (INV-RIES-02: el modo por defecto es observar, nunca bloquea).
func TestReconocimientoOrigen_HuellaDistinta_AuditaOrigenNuevo(t *testing.T) {
	pool := poolAplicacion(t)
	dueno := poolDueno(t)
	clienteRedisCrudo, perfiles := clienteYPerfilesRedisDePrueba(t)
	limitador := clienteRedis(t)

	app := nuevoServidorIdentidadReconocimientoOrigen(t, pool,
		opcionesReconocimientoOrigenDePrueba(pool, perfiles, confianzadominio.PoliticaRiesgo{})...,
	)

	idUsuario, correo := usuarioActivoDePruebaSinLimiteDeIP(t, app, limitador, dueno, "origen-huella-distinta")
	t.Cleanup(func() { borrarUsuario(t, dueno, idUsuario) })
	limpiarPerfilOrigenDePrueba(t, clienteRedisCrudo, correo)

	for i := 0; i < 3; i++ {
		status, _ := loginIdentidadConHuella(t, app, limitador, correo, contrasenaFuerteDePrueba, "huella-A")
		if status != http.StatusOK {
			t.Fatalf("login de calentamiento %d desde huella-A: status=%d, esperado 200", i, status)
		}
	}

	status, resultado := loginIdentidadConHuella(t, app, limitador, correo, contrasenaFuerteDePrueba, "huella-B")
	if status != http.StatusOK {
		t.Fatalf("login desde huella-B (nueva): status=%d, esperado 200 (modo observar nunca bloquea)", status)
	}
	if resultado.RequiereSegundoFactor {
		t.Fatalf("INV-RIES-02: un dispositivo nuevo jamás debe producir RequiereSegundoFactor por confianza: %+v", resultado)
	}

	var accion, resultadoCol, huella string
	err := dueno.QueryRow(context.Background(),
		`SELECT accion, resultado, huella_dispositivo FROM auditoria WHERE accion = 'origen.nuevo' AND usuario_id = $1::uuid`,
		idUsuario,
	).Scan(&accion, &resultadoCol, &huella)
	if err != nil {
		t.Fatalf("consultando la fila origen.nuevo para el usuario %s: %v", idUsuario, err)
	}
	if accion != "origen.nuevo" || resultadoCol != "exito" {
		t.Fatalf("fila de auditoría inesperada: accion=%q resultado=%q", accion, resultadoCol)
	}
	if huella != "huella-B" {
		t.Fatalf("huella_dispositivo de la fila origen.nuevo = %q, esperado %q", huella, "huella-B")
	}

	assertCadenaAuditoriaIntegra(t, dueno)
}

// =============================================================================
// 3. INV-RIES-05: tres intentos fallidos desde una huella nueva NUNCA la
//    promueven — "el test que más importa de todos".
// =============================================================================

// TestReconocimientoOrigen_INV_RIES_05_IntentosFallidosNuncaPromuevenLaHuella
// arma historial suficiente desde huella-A, hace tres intentos FALLIDOS
// (contraseña incorrecta) desde huella-C y confirma en caja blanca que el
// campo d:<hash de huella-C> NUNCA aparece en Redis pese a los tres
// intentos. Como confirmación positiva (caja negra, sugerida por el propio
// encargo), un login EXITOSO posterior desde huella-C debe comportarse
// como un dispositivo genuinamente nuevo (audita origen.nuevo, y solo
// entonces el campo de Redis aparece) — si los intentos fallidos hubieran
// promovido huella-C de alguna forma, este último paso no vería nada nuevo
// que auditar.
func TestReconocimientoOrigen_INV_RIES_05_IntentosFallidosNuncaPromuevenLaHuella(t *testing.T) {
	pool := poolAplicacion(t)
	dueno := poolDueno(t)
	clienteRedisCrudo, perfiles := clienteYPerfilesRedisDePrueba(t)
	limitador := clienteRedis(t)

	app := nuevoServidorIdentidadReconocimientoOrigen(t, pool,
		opcionesReconocimientoOrigenDePrueba(pool, perfiles, confianzadominio.PoliticaRiesgo{})...,
	)

	idUsuario, correo := usuarioActivoDePruebaSinLimiteDeIP(t, app, limitador, dueno, "origen-inv-ries-05")
	t.Cleanup(func() { borrarUsuario(t, dueno, idUsuario) })
	limpiarPerfilOrigenDePrueba(t, clienteRedisCrudo, correo)
	clave := claveRedisOrigenDeCorreo(correo)

	// Historial: 3 logins exitosos desde huella-A (INV-RIES-04: el mínimo
	// para que el perfil empiece a producir señales).
	for i := 0; i < 3; i++ {
		status, _ := loginIdentidadConHuella(t, app, limitador, correo, contrasenaFuerteDePrueba, "huella-A")
		if status != http.StatusOK {
			t.Fatalf("login de calentamiento %d desde huella-A: status=%d, esperado 200", i, status)
		}
	}

	// 3 intentos con contraseña incorrecta, todos desde huella-C.
	campoHuellaC := campoRedisDispositivo("huella-C")
	for i := 0; i < 3; i++ {
		status, _ := loginIdentidadConHuella(t, app, limitador, correo, "definitivamente-no-es-la-contrasena", "huella-C")
		if status != http.StatusUnauthorized {
			t.Fatalf("intento fallido %d desde huella-C: status=%d, esperado 401", i, status)
		}
	}

	campos, err := clienteRedisCrudo.HGetAll(context.Background(), clave).Result()
	if err != nil {
		t.Fatalf("HGETALL %s: %v", clave, err)
	}
	if _, existe := campos[campoHuellaC]; existe {
		t.Fatalf("INV-RIES-05 violada: el campo %q quedó registrado en Redis tras SOLO intentos fallidos: %v", campoHuellaC, campos)
	}
	if campos["#exitos"] != "3" {
		t.Fatalf("#exitos = %q tras los 3 fallos, esperado que siguiera en 3 (los fallos no incrementan el contador de éxitos)", campos["#exitos"])
	}

	// Confirmación positiva: un login EXITOSO ahora desde huella-C debe
	// comportarse como un dispositivo nuevo de verdad.
	status, resultado := loginIdentidadConHuella(t, app, limitador, correo, contrasenaFuerteDePrueba, "huella-C")
	if status != http.StatusOK {
		t.Fatalf("login exitoso desde huella-C: status=%d, esperado 200", status)
	}
	if resultado.RequiereSegundoFactor {
		t.Fatalf("INV-RIES-02: no debía requerir segundo factor: %+v", resultado)
	}

	var huellaAuditada string
	err = dueno.QueryRow(context.Background(),
		`SELECT huella_dispositivo FROM auditoria WHERE accion = 'origen.nuevo' AND usuario_id = $1::uuid ORDER BY secuencia DESC LIMIT 1`,
		idUsuario,
	).Scan(&huellaAuditada)
	if err != nil {
		t.Fatalf("consultando la fila origen.nuevo más reciente para el usuario %s: %v", idUsuario, err)
	}
	if huellaAuditada != "huella-C" {
		t.Fatalf("la fila origen.nuevo más reciente trae huella_dispositivo=%q, esperado %q "+
			"(si trajera otra cosa, sería indicio de que huella-C ya se había promovido antes, violando INV-RIES-05)",
			huellaAuditada, "huella-C")
	}

	camposFinal, err := clienteRedisCrudo.HGetAll(context.Background(), clave).Result()
	if err != nil {
		t.Fatalf("HGETALL %s (tras el login exitoso): %v", clave, err)
	}
	if _, existe := camposFinal[campoHuellaC]; !existe {
		t.Fatalf("tras el login EXITOSO desde huella-C, el campo %q debería estar presente (recién promovido): %v", campoHuellaC, camposFinal)
	}

	assertCadenaAuditoriaIntegra(t, dueno)
}

// =============================================================================
// 4. modo=exigir_captcha con riesgo elevado: 429, y el mismo intento con un
//    captcha ya aceptable pasa (bypass).
// =============================================================================

// TestReconocimientoOrigen_ModoExigirCaptcha_DeniegaYElBypassConCaptchaPasa
// usa una política que fuerza NivelRiesgo>=elevado con la sola señal
// dispositivo_desconocido (politicaRiesgoExigirCaptchaDePrueba, mismo
// criterio que TestEvaluarRiesgoDeOrigen_INV_RIES_02_NuncaProduceRequiereStepUp)
// en modo exigir_captcha: tras el historial mínimo desde huella-A, un
// login desde huella-B SIN token_captcha se deniega con 429; el MISMO
// intento con un token_captcha no vacío (Turnstile en modo desarrollo sin
// secretKey es fail-open: cualquier token produce puntaje 1.0, ver
// turnstile.NuevoVerificadorCaptcha) pasa por el bypass del paso 2.5.e del
// diseño y el login se completa con éxito (201, con token de acceso).
//
// Nota sobre Retry-After: a diferencia de otros motivos de denegación de
// Confianza (limite_ip_excedido, limite_cuenta_excedido_*), la fricción de
// "riesgo_de_origen_requiere_captcha" no tiene un cooldown real que
// esperar — resolver un captcha es inmediato. Aun así, evaluarRiesgoDeOrigen
// (evaluar_trust_signal.go) fija un ReintentarEn corto y fijo
// (reintentarEnCaptchaPorRiesgo, 5s) para que el 429 reutilice la misma
// forma de cuerpo que ErrAccesoDenegadoPorConfianza tal como pide §1/§10
// del diseño ("con su Retry-After"): mapearErrorDominio solo omite la
// cabecera cuando ReintentarEn es cero, así que este motivo SÍ la lleva.
// Este test verifica que la cabecera está presente en vez de asumir su
// ausencia.
func TestReconocimientoOrigen_ModoExigirCaptcha_DeniegaYElBypassConCaptchaPasa(t *testing.T) {
	pool := poolAplicacion(t)
	dueno := poolDueno(t)
	clienteRedisCrudo, perfiles := clienteYPerfilesRedisDePrueba(t)
	limitador := clienteRedis(t)

	politica := politicaRiesgoExigirCaptchaDePrueba(t)
	app := nuevoServidorAccesoReconocimientoOrigen(t, pool,
		opcionesReconocimientoOrigenDePrueba(pool, perfiles, politica)...,
	)

	idUsuario, correo := usuarioActivoDePruebaSinLimiteDeIP(t, app, limitador, dueno, "origen-exigir-captcha")
	t.Cleanup(func() { borrarUsuario(t, dueno, idUsuario) })
	t.Cleanup(func() { limpiarSesionesDeUsuario(t, dueno, idUsuario) })
	limpiarPerfilOrigenDePrueba(t, clienteRedisCrudo, correo)

	for i := 0; i < 3; i++ {
		status, _, _ := iniciarSesionConOrigen(t, app, limitador, correo, contrasenaFuerteDePrueba, "huella-A", "")
		if status != http.StatusCreated {
			t.Fatalf("login de calentamiento %d desde huella-A: status=%d, esperado 201", i, status)
		}
	}

	// Huella nueva, sin captcha: debe denegarse.
	statusDenegado, cuerpoDenegado, cabecerasDenegado := iniciarSesionConOrigen(t, app, limitador, correo, contrasenaFuerteDePrueba, "huella-B", "")
	if statusDenegado != http.StatusTooManyRequests {
		t.Fatalf("login desde huella-B sin captcha: status=%d, esperado 429; cuerpo: %s", statusDenegado, cuerpoDenegado)
	}
	if retryAfter := cabecerasDenegado.Get("Retry-After"); retryAfter != "5" {
		t.Errorf("Retry-After = %q, esperado \"5\" (reintentarEnCaptchaPorRiesgo, ver comentario del test)", retryAfter)
	}

	// Mismo intento, con un captcha ya aceptable: debe pasar (bypass).
	statusOK, cuerpoOK, _ := iniciarSesionConOrigen(t, app, limitador, correo, contrasenaFuerteDePrueba, "huella-B", "token-captcha-de-prueba")
	if statusOK != http.StatusCreated {
		t.Fatalf("login desde huella-B CON captcha aceptable: status=%d, esperado 201 (bypass, §3.1.e del diseño); cuerpo: %s", statusOK, cuerpoOK)
	}
	var resultado resultadoSesionDePrueba
	if err := json.Unmarshal(cuerpoOK, &resultado); err != nil {
		t.Fatalf("decodificando la respuesta del bypass: %v\ncuerpo: %s", err, cuerpoOK)
	}
	if resultado.IDUsuario != idUsuario {
		t.Fatalf("id_usuario = %q, esperado %q", resultado.IDUsuario, idUsuario)
	}

	assertCadenaAuditoriaIntegra(t, dueno)
}

// =============================================================================
// 5. INV-RIES-09: ningún campo de riesgo cruza la frontera HTTP, ni en una
//    respuesta exitosa ni en una denegada.
// =============================================================================

// prohibidosPorINVRIES09 son los nombres/subcadenas que jamás deben
// aparecer en el JSON crudo de una respuesta HTTP de login: los tres
// campos nuevos de Decision (PuntajeRiesgo/NivelRiesgo/SenalesDeRiesgo) y
// el código textual de cada señal del catálogo cerrado (§4 del diseño,
// INV-RIES-09). La comparación es case-insensitive porque lo que importa
// es que el DATO no viaje, sin importar la convención de mayúsculas que
// tomaría al serializarse.
var prohibidosPorINVRIES09 = []string{
	"puntaje_riesgo", "puntajeriesgo",
	"nivel_riesgo", "nivelriesgo",
	"senales_de_riesgo", "senalesderiesgo",
	"dispositivo_desconocido",
	"huella_ausente",
	"red_desconocida",
}

func assertSinCamposDeRiesgo(t *testing.T, cuerpo []byte) {
	t.Helper()
	textoEnMinusculas := strings.ToLower(string(cuerpo))
	for _, prohibido := range prohibidosPorINVRIES09 {
		if strings.Contains(textoEnMinusculas, prohibido) {
			t.Errorf("INV-RIES-09: la respuesta HTTP contiene %q; cuerpo: %s", prohibido, cuerpo)
		}
	}
}

// TestReconocimientoOrigen_INV_RIES_09_NingunCampoDeRiesgoEnLaRespuestaHTTP
// inspecciona el JSON crudo (no el DTO tipado, que ya garantiza esto por
// construcción — este test verifica que también es cierto de punta a
// punta) de dos respuestas: un login exitoso sin fricción y un login
// denegado con 429 por riesgo de origen elevado (misma política que el
// test anterior, para garantizar que efectivamente hubo señales de riesgo
// calculadas y que aun así no aparecen en el cuerpo).
func TestReconocimientoOrigen_INV_RIES_09_NingunCampoDeRiesgoEnLaRespuestaHTTP(t *testing.T) {
	pool := poolAplicacion(t)
	dueno := poolDueno(t)
	clienteRedisCrudo, perfiles := clienteYPerfilesRedisDePrueba(t)
	limitador := clienteRedis(t)

	politica := politicaRiesgoExigirCaptchaDePrueba(t)
	app := nuevoServidorAccesoReconocimientoOrigen(t, pool,
		opcionesReconocimientoOrigenDePrueba(pool, perfiles, politica)...,
	)

	idUsuario, correo := usuarioActivoDePruebaSinLimiteDeIP(t, app, limitador, dueno, "origen-inv-ries-09")
	t.Cleanup(func() { borrarUsuario(t, dueno, idUsuario) })
	t.Cleanup(func() { limpiarSesionesDeUsuario(t, dueno, idUsuario) })
	limpiarPerfilOrigenDePrueba(t, clienteRedisCrudo, correo)

	var cuerpoExitoso []byte
	for i := 0; i < 3; i++ {
		status, cuerpo, _ := iniciarSesionConOrigen(t, app, limitador, correo, contrasenaFuerteDePrueba, "huella-A", "")
		if status != http.StatusCreated {
			t.Fatalf("login de calentamiento %d desde huella-A: status=%d, esperado 201; cuerpo: %s", i, status, cuerpo)
		}
		cuerpoExitoso = cuerpo
	}
	assertSinCamposDeRiesgo(t, cuerpoExitoso)

	// Huella nueva, riesgo elevado, exigir_captcha, sin captcha: denegado.
	statusDenegado, cuerpoDenegado, _ := iniciarSesionConOrigen(t, app, limitador, correo, contrasenaFuerteDePrueba, "huella-B", "")
	if statusDenegado != http.StatusTooManyRequests {
		t.Fatalf("login desde huella-B sin captcha: status=%d, esperado 429; cuerpo: %s", statusDenegado, cuerpoDenegado)
	}
	assertSinCamposDeRiesgo(t, cuerpoDenegado)

	assertCadenaAuditoriaIntegra(t, dueno)
}

// =============================================================================
// 6. INV-RIES-03: un PerfilDeOrigenes que falla no rompe el login
//    (fail-open incondicional).
// =============================================================================

// perfilDeOrigenesFallaDePrueba implementa confianzapuertos.PerfilDeOrigenes
// fallando siempre en Consultar y en Registrar, simulando una caída de
// Redis ACOTADA a este mecanismo puntual — mismo criterio y mismo patrón
// que estadoColaFallaAlReclamarDePrueba
// (confianza_colas_virtuales_e2e_test.go): un doble de prueba que falla
// exactamente en la operación que se quiere ejercitar, en vez de apagar el
// Redis real completo (que también tumbaría el limitador de tasa y el
// captcha, mezclando dos invariantes distintas en un solo test). Nivel de
// aislamiento elegido deliberadamente para poder verificar INV-RIES-03 vía
// HTTP real sin arriesgar falsos negativos por otros mecanismos de
// Confianza fallando al mismo tiempo.
type perfilDeOrigenesFallaDePrueba struct{}

var errPerfilDeOrigenesFallaDePrueba = errors.New("confianza/redis: fallo simulado de infraestructura (prueba de fail-open, INV-RIES-03)")

func (perfilDeOrigenesFallaDePrueba) Consultar(context.Context, confianzapuertos.ConsultaPerfilOrigen) (confianzapuertos.VistaPerfilOrigen, error) {
	return confianzapuertos.VistaPerfilOrigen{}, errPerfilDeOrigenesFallaDePrueba
}

func (perfilDeOrigenesFallaDePrueba) Registrar(context.Context, confianzapuertos.RegistrarOrigenObservado) error {
	return errPerfilDeOrigenesFallaDePrueba
}

func (perfilDeOrigenesFallaDePrueba) Olvidar(context.Context, string) error { return nil }

var _ confianzapuertos.PerfilDeOrigenes = perfilDeOrigenesFallaDePrueba{}

// TestReconocimientoOrigen_INV_RIES_03_FailOpenCuandoElPerfilDeOrigenesFalla
// monta el servidor de Identidad con un PerfilDeOrigenes que siempre falla
// (perfilDeOrigenesFallaDePrueba) y confirma que el login sigue
// funcionando con normalidad: 200, sin requerir segundo factor, y sin
// error propagado — exactamente INV-RIES-03 ("cualquier error del puerto
// PerfilDeOrigenes... produce cero señales... con slog.Warn, nunca agrega
// fricción").
func TestReconocimientoOrigen_INV_RIES_03_FailOpenCuandoElPerfilDeOrigenesFalla(t *testing.T) {
	pool := poolAplicacion(t)
	dueno := poolDueno(t)
	limitador := clienteRedis(t)

	app := nuevoServidorIdentidadReconocimientoOrigen(t, pool,
		confianzaaplicacion.ConPerfilesDeOrigen(perfilDeOrigenesFallaDePrueba{}),
		confianzaaplicacion.ConAuditoriaDeRiesgo(confianzaauditoria.NuevoRegistroAuditoria(pool)),
		confianzaaplicacion.ConRelojDeRiesgo(reloj.NuevoReal()),
	)

	idUsuario, correo := usuarioActivoDePruebaSinLimiteDeIP(t, app, limitador, dueno, "origen-fail-open")
	t.Cleanup(func() { borrarUsuario(t, dueno, idUsuario) })

	status, resultado := loginIdentidadConHuella(t, app, limitador, correo, contrasenaFuerteDePrueba, "huella-cualquiera")
	if status != http.StatusOK {
		t.Fatalf("login con PerfilDeOrigenes caído: status=%d, esperado 200 (INV-RIES-03: fail-open incondicional)", status)
	}
	if resultado.IDUsuario != idUsuario {
		t.Fatalf("id_usuario = %q, esperado %q", resultado.IDUsuario, idUsuario)
	}
	if resultado.RequiereSegundoFactor {
		t.Fatalf("no se esperaba requerir segundo factor con el motor de riesgo caído: %+v", resultado)
	}

	// Repetido para confirmar que el fallo no deja al caso de uso en un
	// estado inconsistente entre llamadas (best-effort real, no solo "la
	// primera vez").
	status2, resultado2 := loginIdentidadConHuella(t, app, limitador, correo, contrasenaFuerteDePrueba, "huella-cualquiera")
	if status2 != http.StatusOK {
		t.Fatalf("segundo login con PerfilDeOrigenes caído: status=%d, esperado 200", status2)
	}
	if resultado2.RequiereSegundoFactor {
		t.Fatalf("no se esperaba requerir segundo factor con el motor de riesgo caído (segundo intento): %+v", resultado2)
	}

	assertCadenaAuditoriaIntegra(t, dueno)
}
