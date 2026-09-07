// Package integracion contiene los tests de integración del bounded
// context Identidad: ejercitan la pila real (HTTP → aplicación → Postgres
// real, con los triggers de auditoría corriendo en la base de datos), sin
// mocks. Los tests de dominio y aplicación (internal/identidad/{dominio,
// aplicacion}) ya cubren las reglas de negocio con mocks; aquí solo se
// verifica que los adaptadores concretos cumplen realmente el contrato de
// sus puertos contra la infraestructura real.
//
// Todos los tests de este paquete requieren Postgres real levantado (ver
// deployments/docker-compose.yml) y las migraciones aplicadas (ver
// Makefile: migrate-up). Si DATABASE_URL_APLICACION o DATABASE_URL no están
// definidas en el entorno, cada test se salta limpiamente con t.Skip: así
// `go test ./...` sigue funcionando sin Docker levantado (p. ej. en un CI
// sin BD disponible), sin necesidad de un build tag separado.
package integracion

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	confianzaredis "github.com/r-david1/moterus/internal/confianza/adaptadores/redis"
	"github.com/r-david1/moterus/internal/confianza/adaptadores/turnstile"
	confianzaaplicacion "github.com/r-david1/moterus/internal/confianza/aplicacion"
	confianzapuertos "github.com/r-david1/moterus/internal/confianza/puertos"
	"github.com/r-david1/moterus/internal/identidad/adaptadores/auditoria"
	"github.com/r-david1/moterus/internal/identidad/adaptadores/confianza"
	"github.com/r-david1/moterus/internal/identidad/adaptadores/cripto"
	"github.com/r-david1/moterus/internal/identidad/adaptadores/eventos"
	identidadhttp "github.com/r-david1/moterus/internal/identidad/adaptadores/http"
	"github.com/r-david1/moterus/internal/identidad/adaptadores/notificaciones"
	identidadpostgres "github.com/r-david1/moterus/internal/identidad/adaptadores/postgres"
	"github.com/r-david1/moterus/internal/identidad/aplicacion"
	"github.com/r-david1/moterus/internal/identidad/dominio"
	"github.com/r-david1/moterus/internal/identidad/puertos"
	"github.com/r-david1/moterus/internal/plataforma/cache"
	"github.com/r-david1/moterus/internal/plataforma/ids"
	"github.com/r-david1/moterus/internal/plataforma/reloj"
)

// --- entorno / conexiones ----------------------------------------------------

// servidorHIBPPruebas es un doble local de la API de HIBP (k-anonymity):
// responde siempre 200 con cuerpo vacío ("ninguna contraseña de prueba
// aparece en brechas conocidas"). Se usa en vez del endpoint público real
// para que los tests de este paquete no dependan de red saliente ni paguen
// el timeout de 3s de cripto.VerificadorHIBP en cada registro: es la única
// pieza de la pila que no se ejercita "real" a propósito, documentado aquí
// explícitamente. Todo lo demás (Postgres, Argon2id, triggers de
// auditoría) es la implementación real de producción.
var servidorHIBPPruebas *httptest.Server

func TestMain(m *testing.M) {
	servidorHIBPPruebas = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	codigo := m.Run()
	servidorHIBPPruebas.Close()
	os.Exit(codigo)
}

// dsnAplicacion devuelve el DSN del rol de login acotado (rol_login_identidad,
// migración 000003): el mismo que usa el proceso api en tiempo de ejecución
// (ADR 0017). Si no está definido, salta el test.
func dsnAplicacion(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL_APLICACION")
	if dsn == "" {
		t.Skip("DATABASE_URL_APLICACION no está definido: se omiten los tests de integración " +
			"(requieren Postgres real levantado, ver deployments/docker-compose.yml y Makefile)")
	}
	return dsn
}

// dsnDueno devuelve el DSN del rol dueño de la base. Se usa EXCLUSIVAMENTE
// para inspección/setup directo dentro de los tests (leer auditoria sin
// restricciones, forzar el estado de un usuario que hoy no tiene endpoint
// HTTP propio para activarse) — nunca para levantar el servidor bajo
// prueba, que siempre corre con dsnAplicacion, igual que en producción.
func dsnDueno(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL no está definido: se omiten los tests de integración")
	}
	return dsn
}

// poolAplicacion abre un pool contra rol_login_identidad y lo cierra al
// terminar el test.
func poolAplicacion(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), dsnAplicacion(t))
	if err != nil {
		t.Fatalf("no se pudo crear el pool con rol_login_identidad: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(context.Background()); err != nil {
		t.Fatalf("no se pudo conectar con rol_login_identidad (revisa docker-compose/migraciones): %v", err)
	}
	return pool
}

// poolDueno abre un pool contra el rol dueño y lo cierra al terminar el test.
func poolDueno(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), dsnDueno(t))
	if err != nil {
		t.Fatalf("no se pudo crear el pool con el rol dueño: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(context.Background()); err != nil {
		t.Fatalf("no se pudo conectar con el rol dueño (revisa docker-compose/migraciones): %v", err)
	}
	return pool
}

// --- ensamblaje del servidor bajo prueba -------------------------------------

// nuevoServidorIdentidad reproduce el mismo cableado que montarIdentidad en
// cmd/api/main.go (que, al vivir en package main, no se puede importar
// desde aquí), conectado con dsnAplicacion — exactamente los privilegios
// acotados de rol_login_identidad con los que corre en producción (ADR
// 0017), nunca con el rol dueño. Devuelve la *fiber.App ya con las rutas de
// Identidad registradas, lista para app.Test(req).
func nuevoServidorIdentidad(t *testing.T, pool *pgxpool.Pool) *fiber.App {
	t.Helper()
	loggerSilencioso := slog.New(slog.NewTextHandler(io.Discard, nil))
	return nuevoServidorIdentidadConNotificador(t, pool, notificaciones.NuevoNotificadorCorreoLog(loggerSilencioso))
}

// nuevoServidorIdentidadConNotificador es la misma construcción que
// nuevoServidorIdentidad pero permite inyectar un puertos.NotificadorCorreo
// propio. Existe porque NotificadorCorreoLog (el adaptador real de este
// hito, sección 3.4 del diseño) es deliberadamente log-only: el token de
// verificación en claro solo queda en el logger, nunca en un puerto de
// salida legible por el test. Los tests de verificación de correo
// (verificacion_correo_test.go) necesitan el token en claro para poder
// ejercer POST /identidad/verificaciones-correo, así que inyectan aquí un
// captor en memoria en vez del NotificadorCorreoLog real — el resto del
// cableado (Postgres, Argon2id, auditoría, evaluador de confianza no-op)
// es idéntico a nuevoServidorIdentidad/montarIdentidad (cmd/api/main.go).
func nuevoServidorIdentidadConNotificador(t *testing.T, pool *pgxpool.Pool, notificadorCorreo puertos.NotificadorCorreo) *fiber.App {
	t.Helper()

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
	// NoOp a propósito, igual que cmd/api/main.go sin REDIS_URL: el guardián
	// de perímetro real (ADR 0018) se ejercita aparte, contra Redis real, en
	// confianza_redis_test.go (el motor de rate limiting) y en el harness
	// dedicado nuevoServidorIdentidadConConfianzaReal (guardián de
	// ReenviarVerificacion end-to-end vía HTTP, verificacion_correo_test.go).
	// Usar aquí el no-op evita que las aserciones de negocio de este
	// paquete (login, registro, verificación de correo) se acoplen a
	// contadores de Redis compartidos entre corridas de test.
	evaluadorConfianza := confianza.NuevoEvaluadorConfianzaNoOp(loggerSilencioso)

	registrador := aplicacion.NuevoRegistrarUsuarioCasoDeUso(
		repositorioUsuarios, hasher, verificadorFiltradas, evaluadorConfianza,
		registroAuditoria, publicadorEventos, relojReal, generadorIDs, unidadDeTrabajo,
		generadorTokens, repositorioTokensVerificacion, notificadorCorreo,
	)
	autenticador := aplicacion.NuevoAutenticarUsuarioCasoDeUso(
		repositorioUsuarios, hasher, evaluadorConfianza,
		registroAuditoria, publicadorEventos, relojReal, unidadDeTrabajo,
	)
	consultor := aplicacion.NuevoObtenerUsuarioCasoDeUso(repositorioUsuarios, registroAuditoria, relojReal)
	verificadorCorreo := aplicacion.NuevoVerificarCorreoCasoDeUso(
		repositorioUsuarios, repositorioTokensVerificacion, registroAuditoria, publicadorEventos, relojReal, unidadDeTrabajo,
	)
	reenviadorVerificacion := aplicacion.NuevoReenviarVerificacionCasoDeUso(
		repositorioUsuarios, generadorTokens, repositorioTokensVerificacion, notificadorCorreo, relojReal,
	)

	manejador := identidadhttp.NuevoManejadorIdentidad(registrador, autenticador, consultor, verificadorCorreo, reenviadorVerificacion, evaluadorConfianza, nil, nil)

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	identidadhttp.RegistrarRutas(app, manejador, nil, nil)
	return app
}

// nuevoServidorIdentidadConConfianzaReal reproduce el mismo cableado que
// nuevoServidorIdentidad pero con EvaluadorConfianzaReal (ADR 0018) en vez
// del no-op — exactamente lo que cmd/api/main.go monta cuando REDIS_URL
// está configurado (construirEvaluadorConfianza). Existe para poder
// verificar con un test de integración real que el guardián de perímetro
// de POST /identidad/verificaciones-correo/reenvios (handlers.go,
// ManejadorIdentidad.ReenviarVerificacion) efectivamente bloquea con 429
// tras el umbral de PoliticaLimitesPorDefecto — algo que
// nuevoServidorIdentidad (no-op a propósito) nunca puede ejercer. Se salta
// limpiamente si REDIS_URL no está definido, mismo criterio que
// clienteRedis en confianza_redis_test.go.
func nuevoServidorIdentidadConConfianzaReal(t *testing.T, pool *pgxpool.Pool) *fiber.App {
	t.Helper()

	url := os.Getenv("REDIS_URL")
	if url == "" {
		t.Skip("REDIS_URL no está definido: se omite el test del guardián de perímetro real (ADR 0018)")
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

	// Mismo ensamblaje que construirEvaluadorConfianza en cmd/api/main.go:
	// LimitadorTasa real sobre Redis + VerificadorCaptcha en modo fail-open
	// de desarrollo (sin secretKey, entornoApp != "production"). Ningún
	// endpoint de este paquete envía TokenCaptcha, así que el captcha nunca
	// llega a evaluarse de verdad (huboToken == false, ver
	// EvaluarTrustSignalCasoDeUso.Evaluar) — solo hace falta construirlo.
	limitador := confianzaredis.NuevoLimitadorTasa(clienteRedisCrudo)
	verificadorCaptcha := turnstile.NuevoVerificadorCaptcha("", "", "development")
	evaluarTrustSignal := confianzaaplicacion.NuevoEvaluarTrustSignalCasoDeUso(limitador, verificadorCaptcha)
	evaluadorConfianza := confianza.NuevoEvaluadorConfianzaReal(confianzapuertos.EvaluadorDeRiesgo(evaluarTrustSignal))

	registrador := aplicacion.NuevoRegistrarUsuarioCasoDeUso(
		repositorioUsuarios, hasher, verificadorFiltradas, evaluadorConfianza,
		registroAuditoria, publicadorEventos, relojReal, generadorIDs, unidadDeTrabajo,
		generadorTokens, repositorioTokensVerificacion, notificadorCorreo,
	)
	autenticador := aplicacion.NuevoAutenticarUsuarioCasoDeUso(
		repositorioUsuarios, hasher, evaluadorConfianza,
		registroAuditoria, publicadorEventos, relojReal, unidadDeTrabajo,
	)
	consultor := aplicacion.NuevoObtenerUsuarioCasoDeUso(repositorioUsuarios, registroAuditoria, relojReal)
	verificadorCorreo := aplicacion.NuevoVerificarCorreoCasoDeUso(
		repositorioUsuarios, repositorioTokensVerificacion, registroAuditoria, publicadorEventos, relojReal, unidadDeTrabajo,
	)
	reenviadorVerificacion := aplicacion.NuevoReenviarVerificacionCasoDeUso(
		repositorioUsuarios, generadorTokens, repositorioTokensVerificacion, notificadorCorreo, relojReal,
	)

	manejador := identidadhttp.NuevoManejadorIdentidad(registrador, autenticador, consultor, verificadorCorreo, reenviadorVerificacion, evaluadorConfianza, nil, nil)

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	identidadhttp.RegistrarRutas(app, manejador, nil, nil)
	return app
}

// --- datos de prueba ----------------------------------------------------------

// contrasenaFuerteDePrueba cumple PoliticaContrasena (>=12 caracteres, sin
// secuencias triviales, sin el correo ni el dominio) para cualquier correo
// de dominio "ejemplo-integracion.test" generado por correoUnico.
const contrasenaFuerteDePrueba = "Integr4cion-Segura-Prueba!"

// correoUnico genera un correo con un sufijo aleatorio para que cada
// ejecución de test use una cuenta propia y no colisione con datos de
// ejecuciones anteriores (INV-ID-02: el correo es único a nivel de BD).
func correoUnico(t *testing.T, prefijo string) string {
	t.Helper()
	return fmt.Sprintf("qa-%s-%d-%d@ejemplo-integracion.test", prefijo, time.Now().UnixNano(), rand.Intn(1_000_000))
}

// borrarUsuario elimina físicamente (solo el rol dueño puede: rol_aplicacion
// no tiene DELETE sobre usuarios a propósito, ver migración 000003) un
// usuario de prueba al terminar el test, para no acumular basura en la BD
// de desarrollo entre corridas. Los eventos de auditoría que haya generado
// NO se borran: la tabla es append-only por diseño (ADR 0005) y eso es
// exactamente lo que se está verificando en este paquete.
func borrarUsuario(t *testing.T, poolDueno *pgxpool.Pool, idUsuario string) {
	t.Helper()
	if idUsuario == "" {
		return
	}
	// tokens_verificacion_correo.usuario_id tiene ON DELETE CASCADE
	// (migración 000005) precisamente para que este DELETE no necesite
	// borrar el token huérfano por separado.
	if _, err := poolDueno.Exec(context.Background(), `DELETE FROM usuarios WHERE id = $1`, idUsuario); err != nil {
		t.Logf("no se pudo limpiar el usuario de prueba %s: %v", idUsuario, err)
	}
}

// activarUsuario transiciona un usuario a estado 'activo' directamente en
// la BD (con el rol dueño), sin pasar por el flujo real de verificación de
// correo (POST /identidad/verificaciones-correo, sección 3.4 del diseño).
// Sigue siendo útil como fixture en tests que solo necesitan un usuario ya
// activo (p. ej. de login) sin ejercitar el mecanismo de verificación en sí.
func activarUsuario(t *testing.T, poolDueno *pgxpool.Pool, idUsuario string) {
	t.Helper()
	tag, err := poolDueno.Exec(context.Background(), `UPDATE usuarios SET estado = 'activo' WHERE id = $1`, idUsuario)
	if err != nil {
		t.Fatalf("no se pudo activar el usuario de prueba %s: %v", idUsuario, err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("activarUsuario: se esperaba 1 fila afectada, hubo %d", tag.RowsAffected())
	}
}

// nuevoUsuarioDePrueba construye (sin persistir) un *dominio.Usuario válido
// con el correo indicado, para los tests del adaptador RepositorioUsuarios
// que ejercitan el puerto directamente, sin pasar por HTTP/aplicacion.
func nuevoUsuarioDePrueba(t *testing.T, correoCrudo string) *dominio.Usuario {
	t.Helper()
	correo, err := dominio.NuevoCorreo(correoCrudo)
	if err != nil {
		t.Fatalf("NuevoCorreo(%q): %v", correoCrudo, err)
	}
	crudo, err := ids.GenerarUUIDv7()
	if err != nil {
		t.Fatalf("GenerarUUIDv7(): %v", err)
	}
	id, err := dominio.IDUsuarioDesde(crudo)
	if err != nil {
		t.Fatalf("IDUsuarioDesde(%q): %v", crudo, err)
	}
	// Hash PHC sintéticamente válido: estos tests de adaptador no ejercitan
	// Argon2id (eso ya lo cubre cripto/hasher_argon2id_test.go), solo
	// necesitan un HashContrasena con formato reconocible por el VO.
	hash, err := dominio.NuevoHashContrasena("$argon2id$v=19$m=65536,t=3,p=1$c2FsZGVwcnVlYmE$aGFzaGRlcHJ1ZWJhaW50ZWdyYWNpb24")
	if err != nil {
		t.Fatalf("NuevoHashContrasena(): %v", err)
	}
	usuario, err := dominio.RegistrarUsuario(id, correo, hash, time.Now().UTC())
	if err != nil {
		t.Fatalf("RegistrarUsuario(): %v", err)
	}
	usuario.EventosPendientes() // drenado: estos tests de adaptador no auditan por sí mismos.
	return usuario
}

// origenDePrueba construye un dominio.OrigenSolicitud mínimo válido para
// pasar a RegistroAuditoria.Registrar en los tests que lo invocan
// directamente (fuera de un handler HTTP real).
func origenDePrueba(t *testing.T) dominio.OrigenSolicitud {
	t.Helper()
	origen, err := dominio.NuevoOrigenSolicitud("127.0.0.1", "go-test-integracion", "", "id-solicitud-prueba")
	if err != nil {
		t.Fatalf("NuevoOrigenSolicitud(): %v", err)
	}
	return origen
}

// --- verificación de la cadena de auditoría ----------------------------------

// assertCadenaAuditoriaIntegra invoca verificar_cadena_auditoria() (función
// SQL de la migración 000002, sección 9) y falla el test si devuelve
// cualquier fila de discrepancia. Requiere el rol dueño: es el único con
// privilegio implícito de EXECUTE sobre la función en este entorno de
// desarrollo (GRANT EXECUTE solo se otorgó a rol_mantenimiento_auditoria y
// rol_auditor_lectura; el dueño la creó y por tanto la puede ejecutar sin
// GRANT explícito).
func assertCadenaAuditoriaIntegra(t *testing.T, poolDueno *pgxpool.Pool) {
	t.Helper()
	filas, err := poolDueno.Query(context.Background(), `SELECT secuencia, problema FROM verificar_cadena_auditoria()`)
	if err != nil {
		t.Fatalf("verificar_cadena_auditoria(): %v", err)
	}
	defer filas.Close()

	var discrepancias []string
	for filas.Next() {
		var secuencia int64
		var problema string
		if err := filas.Scan(&secuencia, &problema); err != nil {
			t.Fatalf("escaneando verificar_cadena_auditoria(): %v", err)
		}
		discrepancias = append(discrepancias, fmt.Sprintf("secuencia=%d problema=%s", secuencia, problema))
	}
	if err := filas.Err(); err != nil {
		t.Fatalf("iterando verificar_cadena_auditoria(): %v", err)
	}
	if len(discrepancias) > 0 {
		t.Fatalf("verificar_cadena_auditoria() encontró %d discrepancia(s), la cadena de hashes está rota: %v",
			len(discrepancias), discrepancias)
	}
}

// --- helpers HTTP -------------------------------------------------------------

// respuestaHTTP ejecuta req contra app y devuelve el status code y el
// cuerpo ya decodificado en destino (si destino no es nil).
func respuestaHTTP(t *testing.T, app *fiber.App, req *http.Request, destino any) int {
	t.Helper()
	resp, err := app.Test(req, 10_000) // 10s: Argon2id (64 MiB, ADR 0008) más margen de CI.
	if err != nil {
		t.Fatalf("app.Test(%s %s): %v", req.Method, req.URL.Path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	cuerpo, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("leyendo el cuerpo de la respuesta: %v", err)
	}
	if destino != nil && len(cuerpo) > 0 {
		if err := json.Unmarshal(cuerpo, destino); err != nil {
			t.Fatalf("decodificando el cuerpo de la respuesta (%s): %v\ncuerpo: %s", req.URL.Path, err, cuerpo)
		}
	}
	return resp.StatusCode
}

// peticionJSON construye un *http.Request con cuerpo JSON.
func peticionJSON(t *testing.T, metodo, ruta string, cuerpo any) *http.Request {
	t.Helper()
	var lector io.Reader
	if cuerpo != nil {
		b, err := json.Marshal(cuerpo)
		if err != nil {
			t.Fatalf("json.Marshal: %v", err)
		}
		lector = bytes.NewReader(b)
	}
	req := httptest.NewRequest(metodo, ruta, lector)
	req.Header.Set("Content-Type", "application/json")
	return req
}

// errorHumaRespuesta es la forma común de huma.ErrorModel que interesa
// verificar en los tests (título, status, detalle); ver
// identidad/adaptadores/http/errores_http.go.
type errorHumaRespuesta struct {
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail"`
}
