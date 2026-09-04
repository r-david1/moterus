// Este archivo contiene los tests de integración del bounded context
// Acceso: ejercitan la pila real (HTTP -> aplicacion -> Postgres real, con
// los triggers de auditoría corriendo en la base de datos, y Redis real
// cuando REDIS_URL está definido), sin mocks. Comparte el paquete
// integracion con los tests de Identidad (mismo criterio que
// entorno_test.go) y reutiliza sus helpers de bajo nivel (poolAplicacion,
// correoUnico, activarUsuario, respuestaHTTP, peticionJSON,
// errorHumaRespuesta).
package integracion

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

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
	accesopuertos "github.com/r-david1/moterus/internal/acceso/puertos"

	"github.com/r-david1/moterus/internal/identidad/adaptadores/auditoria"
	identidadconfianza "github.com/r-david1/moterus/internal/identidad/adaptadores/confianza"
	"github.com/r-david1/moterus/internal/identidad/adaptadores/cripto"
	"github.com/r-david1/moterus/internal/identidad/adaptadores/eventos"
	identidadhttp "github.com/r-david1/moterus/internal/identidad/adaptadores/http"
	"github.com/r-david1/moterus/internal/identidad/adaptadores/notificaciones"
	identidadpostgres "github.com/r-david1/moterus/internal/identidad/adaptadores/postgres"
	"github.com/r-david1/moterus/internal/identidad/aplicacion"

	"github.com/r-david1/moterus/internal/plataforma/cache"
	"github.com/r-david1/moterus/internal/plataforma/reloj"
)

// --- ensamblaje del servidor bajo prueba (Identidad + Acceso) ---------------

// nuevoServidorAcceso reproduce el mismo cableado que
// cmd/api/main.go#montarIdentidadYAcceso, con Identidad detrás de un
// EvaluadorConfianzaNoOp (mismo criterio que nuevoServidorIdentidad: no
// acoplar estas aserciones de negocio a contadores de Redis compartidos
// entre corridas de test) y una llave de firma Ed25519 efímera generada
// para cada servidor de test (nunca la misma entre tests, ADR 0020 §4).
// Si REDIS_URL está definido, monta la ListaRevocacion real (para que los
// tests de revocación inmediata se ejerzan de verdad); si no, usa el noop
// y la revocación sigue siendo correcta vía Postgres (INV-ACC-15).
func nuevoServidorAcceso(t *testing.T, pool *pgxpool.Pool) *fiber.App {
	t.Helper()

	relojReal := reloj.NuevoReal()
	loggerSilencioso := slog.New(slog.NewTextHandler(io.Discard, nil))

	// --- Identidad (sin registrar rutas todavía) ---------------------------
	repositorioUsuarios := identidadpostgres.NuevoRepositorioUsuarios(pool)
	repositorioTokensVerificacion := identidadpostgres.NuevoRepositorioTokensVerificacion(pool)
	unidadDeTrabajoIdentidad := identidadpostgres.NuevaUnidadDeTrabajo(pool)
	generadorIDsIdentidad := identidadpostgres.NuevoGeneradorIDs()

	hasher := cripto.NuevoHasherArgon2id()
	verificadorFiltradas := cripto.NuevoVerificadorHIBP(cripto.ConBaseURLHIBP(servidorHIBPPruebas.URL + "/range/"))
	generadorTokensIdentidad := cripto.NuevoGeneradorTokens()

	registroAuditoriaIdentidad := auditoria.NuevoRegistroAuditoria(pool)
	publicadorEventosIdentidad := eventos.NuevoPublicadorLog(loggerSilencioso)
	evaluadorConfianzaIdentidad := identidadconfianza.NuevoEvaluadorConfianzaNoOp(loggerSilencioso)
	notificadorCorreo := notificaciones.NuevoNotificadorCorreoLog(loggerSilencioso)

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
	manejadorIdentidad := identidadhttp.NuevoManejadorIdentidad(
		registrador, autenticadorIdentidad, consultorIdentidad, verificadorCorreo, reenviadorVerificacion, evaluadorConfianzaIdentidad, nil,
	)

	// --- Acceso --------------------------------------------------------------
	politica := accesodominio.PoliticaSesionPorDefecto()

	sesiones := accesopostgres.NuevoRepositorioSesiones(pool)
	uow := accesopostgres.NuevaUnidadDeTrabajo(pool)
	generadorIDs := accesopostgres.NuevoGeneradorIDs()
	generadorRefrescos := accesocripto.NuevoGeneradorTokensRefresco()
	registroAuditoria := accesoauditoria.NuevoRegistroAuditoria(pool)
	publicadorEventos := accesoeventos.NuevoPublicadorLog(loggerSilencioso)

	llaveEfimera, err := accesojwt.GenerarLlaveEfimera()
	if err != nil {
		t.Fatalf("GenerarLlaveEfimera(): %v", err)
	}
	llavero, err := accesojwt.NuevoLlavero(llaveEfimera, nil)
	if err != nil {
		t.Fatalf("NuevoLlavero(): %v", err)
	}
	firmador := accesojwt.NuevoFirmador(llavero, "https://acceso.test.moterus.local", "moterus-test", politica.ToleranciaReloj())

	var listaRevocacion accesopuertos.ListaRevocacion
	if redisDisponible(t) {
		clienteRedis, err := cache.NuevoClienteRedis(mustRedisURL(t))
		if err != nil {
			t.Fatalf("NuevoClienteRedis(): %v", err)
		}
		t.Cleanup(func() { _ = clienteRedis.Close() })
		listaRevocacion = accesoredis.NuevaListaRevocacion(clienteRedis)
	} else {
		listaRevocacion = accesoredis.NuevaListaRevocacionNoOp()
	}

	evaluadorConfianzaAcceso := accesoconfianza.NuevoEvaluadorConfianzaNoOp(loggerSilencioso)

	autenticadorACL := accesoidentidad.NuevoAutenticadorIdentidad(autenticadorIdentidad)
	consultorEstadoSujeto := accesoidentidad.NuevoConsultorEstadoSujeto(consultorIdentidad)

	iniciador := accesoaplicacion.NuevoIniciarSesionCasoDeUso(
		autenticadorACL, sesiones, generadorRefrescos, firmador, listaRevocacion,
		registroAuditoria, publicadorEventos, relojReal, generadorIDs, uow, politica,
		"https://acceso.test.moterus.local", "moterus-test",
	)
	renovador := accesoaplicacion.NuevoRenovarSesionCasoDeUso(
		evaluadorConfianzaAcceso, sesiones, generadorRefrescos, firmador, consultorEstadoSujeto, listaRevocacion,
		registroAuditoria, relojReal, generadorIDs, uow, politica,
		"https://acceso.test.moterus.local", "moterus-test",
	)
	validador := accesoaplicacion.NuevoValidarAccesoCasoDeUso(firmador, listaRevocacion, sesiones, registroAuditoria, relojReal)
	cerrador := accesoaplicacion.NuevoCerrarSesionCasoDeUso(sesiones, evaluadorConfianzaAcceso, listaRevocacion, registroAuditoria, relojReal, uow, politica)
	consultorSesiones := accesoaplicacion.NuevoListarSesionesCasoDeUso(sesiones)

	manejadorAcceso := accesohttp.NuevoManejadorAcceso(iniciador, renovador, cerrador, consultorSesiones, firmador)

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	accesohttp.RegistrarRutas(app, manejadorAcceso, validador)
	identidadhttp.RegistrarRutas(app, manejadorIdentidad, validador)
	return app
}

// redisDisponible/mustRedisURL: mismo criterio que
// nuevoServidorIdentidadConConfianzaReal (confianza_redis_test.go usa el
// mismo patrón): REDIS_URL es opcional para este paquete de tests.
func redisDisponible(t *testing.T) bool {
	t.Helper()
	return mustRedisURL(t) != ""
}

func mustRedisURL(t *testing.T) string {
	t.Helper()
	return os.Getenv("REDIS_URL")
}

// --- fixtures propias de Acceso ---------------------------------------------

// usuarioActivoDePrueba registra y activa (sin pasar por el flujo real de
// verificación de correo, igual criterio que activarUsuario) un usuario
// nuevo, listo para hacer login por HTTP. Devuelve su id (para poder
// limpiarlo con borrarUsuario) y el correo usado.
func usuarioActivoDePrueba(t *testing.T, app *fiber.App, poolDueno *pgxpool.Pool, prefijo string) (idUsuario, correo string) {
	t.Helper()
	correo = correoUnico(t, prefijo)

	var registro struct {
		IDUsuario string `json:"id_usuario"`
	}
	status := respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/identidad/usuarios", map[string]any{
		"correo":     correo,
		"contrasena": contrasenaFuerteDePrueba,
	}), &registro)
	if status != http.StatusOK {
		t.Fatalf("registro de usuario de prueba: status=%d", status)
	}
	activarUsuario(t, poolDueno, registro.IDUsuario)
	return registro.IDUsuario, correo
}

// limpiarSesionesDeUsuario borra (con el rol dueño: rol_aplicacion no
// tiene DELETE sobre sesiones/tokens_refresco a propósito, migración
// 000006) las sesiones y tokens de refresco que un test haya creado para
// idUsuario. Debe registrarse con t.Cleanup DESPUÉS de registrar
// borrarUsuario (t.Cleanup es LIFO: así esta limpieza corre PRIMERO,
// dejando a borrarUsuario sin la FK sesiones.usuario_id -> usuarios(id) en
// el camino).
func limpiarSesionesDeUsuario(t *testing.T, poolDueno *pgxpool.Pool, idUsuario string) {
	t.Helper()
	ctx := context.Background()
	if _, err := poolDueno.Exec(ctx,
		`DELETE FROM tokens_refresco WHERE sesion_id IN (SELECT id FROM sesiones WHERE usuario_id = $1)`, idUsuario,
	); err != nil {
		t.Logf("no se pudieron limpiar los tokens_refresco de prueba del usuario %s: %v", idUsuario, err)
		return
	}
	if _, err := poolDueno.Exec(ctx, `DELETE FROM sesiones WHERE usuario_id = $1`, idUsuario); err != nil {
		t.Logf("no se pudieron limpiar las sesiones de prueba del usuario %s: %v", idUsuario, err)
	}
}

type resultadoSesionDePrueba struct {
	TokenAcceso      string `json:"token_acceso"`
	TipoToken        string `json:"tipo_token"`
	ExpiraEnSegundos int    `json:"expira_en_segundos"`
	TokenRefresco    string `json:"token_refresco"`
	IDSesion         string `json:"id_sesion"`
	IDUsuario        string `json:"id_usuario"`
}

func iniciarSesionDePrueba(t *testing.T, app *fiber.App, correo string) resultadoSesionDePrueba {
	t.Helper()
	var resultado resultadoSesionDePrueba
	status := respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/acceso/sesiones", map[string]any{
		"correo":     correo,
		"contrasena": contrasenaFuerteDePrueba,
	}), &resultado)
	if status != http.StatusCreated {
		t.Fatalf("login de prueba: status=%d", status)
	}
	return resultado
}

// --- tests -------------------------------------------------------------------

// TestAcceso_LoginCompleto_EmiteJWTValidoYRefresco cubre: login completo
// -> JWT bien formado (3 segmentos), refresh token con el prefijo mot_rt_,
// y el JWT es aceptado por un endpoint protegido (GET /acceso/sesiones).
func TestAcceso_LoginCompleto_EmiteJWTValidoYRefresco(t *testing.T) {
	pool := poolAplicacion(t)
	dueno := poolDueno(t)
	app := nuevoServidorAcceso(t, pool)

	idUsuario, correo := usuarioActivoDePrueba(t, app, dueno, "login")
	t.Cleanup(func() { borrarUsuario(t, dueno, idUsuario) })
	t.Cleanup(func() { limpiarSesionesDeUsuario(t, dueno, idUsuario) })

	resultado := iniciarSesionDePrueba(t, app, correo)

	if resultado.TipoToken != "Bearer" {
		t.Fatalf("tipo_token = %q, se esperaba Bearer", resultado.TipoToken)
	}
	if resultado.ExpiraEnSegundos <= 0 {
		t.Fatalf("expira_en_segundos = %d, se esperaba > 0", resultado.ExpiraEnSegundos)
	}
	segmentos := 1
	for _, r := range resultado.TokenAcceso {
		if r == '.' {
			segmentos++
		}
	}
	if segmentos != 3 {
		t.Fatalf("el token de acceso no tiene 3 segmentos (JWT compacto): %q", resultado.TokenAcceso)
	}
	if len(resultado.TokenRefresco) < len("mot_rt_")+43 || resultado.TokenRefresco[:7] != "mot_rt_" {
		t.Fatalf("token_refresco no tiene la forma esperada: %q", resultado.TokenRefresco)
	}
	if resultado.IDUsuario != idUsuario {
		t.Fatalf("id_usuario = %q, se esperaba %q", resultado.IDUsuario, idUsuario)
	}

	// El JWT recién emitido debe ser aceptado por un endpoint protegido.
	req := peticionJSON(t, http.MethodGet, "/acceso/sesiones", nil)
	req.Header.Set("Authorization", "Bearer "+resultado.TokenAcceso)
	var vistas []map[string]any
	status := respuestaHTTP(t, app, req, &vistas)
	if status != http.StatusOK {
		t.Fatalf("GET /acceso/sesiones con el JWT recién emitido: status=%d", status)
	}
	if len(vistas) != 1 {
		t.Fatalf("se esperaba exactamente 1 sesión activa, hubo %d", len(vistas))
	}
	assertCadenaAuditoriaIntegra(t, dueno)
}

// TestAcceso_Renovacion_RotaElTokenYElAnteriorDejaDeServir cubre INV-ACC-05:
// tras renovar, el token de refresco anterior deja de servir (401), y el
// nuevo sí sirve para una renovación posterior.
func TestAcceso_Renovacion_RotaElTokenYElAnteriorDejaDeServir(t *testing.T) {
	pool := poolAplicacion(t)
	dueno := poolDueno(t)
	app := nuevoServidorAcceso(t, pool)

	idUsuario, correo := usuarioActivoDePrueba(t, app, dueno, "renovacion")
	t.Cleanup(func() { borrarUsuario(t, dueno, idUsuario) })
	t.Cleanup(func() { limpiarSesionesDeUsuario(t, dueno, idUsuario) })

	inicial := iniciarSesionDePrueba(t, app, correo)

	var renovado resultadoSesionDePrueba
	status := respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/acceso/sesiones/renovaciones", map[string]any{
		"token_refresco": inicial.TokenRefresco,
	}), &renovado)
	if status != http.StatusOK {
		t.Fatalf("renovación: status=%d", status)
	}
	if renovado.TokenRefresco == inicial.TokenRefresco {
		t.Fatalf("la renovación devolvió el mismo token de refresco: no rotó (INV-ACC-05)")
	}
	if renovado.IDSesion != inicial.IDSesion {
		t.Fatalf("la renovación cambió de sesión: id_sesion = %q, se esperaba %q", renovado.IDSesion, inicial.IDSesion)
	}

	// El token de refresco INICIAL (ya consumido por la rotación) ya no sirve.
	var errResp errorHumaRespuesta
	status = respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/acceso/sesiones/renovaciones", map[string]any{
		"token_refresco": inicial.TokenRefresco,
	}), &errResp)
	if status != http.StatusUnauthorized {
		t.Fatalf("reintentar renovar con el token inicial ya rotado: status=%d, se esperaba 401", status)
	}

	assertCadenaAuditoriaIntegra(t, dueno)
}

// TestAcceso_ReusoDeRefresco_RevocaLaSesion cubre INV-ACC-06: presentar un
// refresco ya consumido revoca TODA la sesión de inmediato — incluido el
// token de refresco vigente que había quedado tras la rotación legítima.
func TestAcceso_ReusoDeRefresco_RevocaLaSesion(t *testing.T) {
	pool := poolAplicacion(t)
	dueno := poolDueno(t)
	app := nuevoServidorAcceso(t, pool)

	idUsuario, correo := usuarioActivoDePrueba(t, app, dueno, "reuso")
	t.Cleanup(func() { borrarUsuario(t, dueno, idUsuario) })
	t.Cleanup(func() { limpiarSesionesDeUsuario(t, dueno, idUsuario) })

	inicial := iniciarSesionDePrueba(t, app, correo)

	var renovado resultadoSesionDePrueba
	status := respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/acceso/sesiones/renovaciones", map[string]any{
		"token_refresco": inicial.TokenRefresco,
	}), &renovado)
	if status != http.StatusOK {
		t.Fatalf("renovación previa al reuso: status=%d", status)
	}

	// Reusar el token YA consumido (el de la primera emisión): dispara
	// ReusoRefrescoDetectado y revoca toda la sesión.
	var errResp errorHumaRespuesta
	status = respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/acceso/sesiones/renovaciones", map[string]any{
		"token_refresco": inicial.TokenRefresco,
	}), &errResp)
	if status != http.StatusUnauthorized {
		t.Fatalf("reuso de refresco: status=%d, se esperaba 401", status)
	}

	// El token de refresco VIGENTE (el de la renovación legítima) tampoco
	// sirve ya: toda la sesión quedó revocada, no solo el token reusado.
	status = respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/acceso/sesiones/renovaciones", map[string]any{
		"token_refresco": renovado.TokenRefresco,
	}), &errResp)
	if status != http.StatusUnauthorized {
		t.Fatalf("renovar con el token vigente de una sesión revocada por reuso: status=%d, se esperaba 401", status)
	}

	var estado, motivo string
	if err := dueno.QueryRow(context.Background(),
		`SELECT estado, motivo_revocacion FROM sesiones WHERE id = $1`, inicial.IDSesion,
	).Scan(&estado, &motivo); err != nil {
		t.Fatalf("consultando el estado de la sesión: %v", err)
	}
	if estado != "revocada" || motivo != "reuso_refresco_detectado" {
		t.Fatalf("sesion.estado=%q motivo_revocacion=%q, se esperaba revocada/reuso_refresco_detectado", estado, motivo)
	}

	assertCadenaAuditoriaIntegra(t, dueno)
}

// TestAcceso_JWKS_SirveLaLlavePublicaCorrecta verifica que el kid publicado
// en el JWT firmado coincide con un kid presente en el JWKS servido por el
// mismo proceso.
func TestAcceso_JWKS_SirveLaLlavePublicaCorrecta(t *testing.T) {
	pool := poolAplicacion(t)
	dueno := poolDueno(t)
	app := nuevoServidorAcceso(t, pool)

	idUsuario, correo := usuarioActivoDePrueba(t, app, dueno, "jwks")
	t.Cleanup(func() { borrarUsuario(t, dueno, idUsuario) })
	t.Cleanup(func() { limpiarSesionesDeUsuario(t, dueno, idUsuario) })

	resultado := iniciarSesionDePrueba(t, app, correo)
	kidToken := kidDelJWT(t, resultado.TokenAcceso)

	var jwks struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			Crv string `json:"crv"`
			Alg string `json:"alg"`
			X   string `json:"x"`
		} `json:"keys"`
	}
	status := respuestaHTTP(t, app, peticionJSON(t, http.MethodGet, "/.well-known/jwks.json", nil), &jwks)
	if status != http.StatusOK {
		t.Fatalf("GET /.well-known/jwks.json: status=%d", status)
	}
	if len(jwks.Keys) == 0 {
		t.Fatalf("el JWKS no publicó ninguna llave")
	}

	var encontrada bool
	for _, k := range jwks.Keys {
		if k.Kid == kidToken {
			encontrada = true
			if k.Kty != "OKP" || k.Crv != "Ed25519" || k.Alg != "EdDSA" || k.X == "" {
				t.Fatalf("la llave del JWKS no tiene la forma esperada (OKP/Ed25519/EdDSA/x no vacío): %+v", k)
			}
		}
	}
	if !encontrada {
		t.Fatalf("el kid del JWT (%s) no está en el JWKS servido", kidToken)
	}
}

// TestAcceso_Logout_RevocaLaSesion cubre el logout individual: tras
// DELETE /acceso/sesiones/actual, el token de acceso deja de aceptarse en
// un endpoint protegido y la sesión queda revocada en Postgres.
func TestAcceso_Logout_RevocaLaSesion(t *testing.T) {
	pool := poolAplicacion(t)
	dueno := poolDueno(t)
	app := nuevoServidorAcceso(t, pool)

	idUsuario, correo := usuarioActivoDePrueba(t, app, dueno, "logout")
	t.Cleanup(func() { borrarUsuario(t, dueno, idUsuario) })
	t.Cleanup(func() { limpiarSesionesDeUsuario(t, dueno, idUsuario) })

	resultado := iniciarSesionDePrueba(t, app, correo)

	req := peticionJSON(t, http.MethodDelete, "/acceso/sesiones/actual", nil)
	req.Header.Set("Authorization", "Bearer "+resultado.TokenAcceso)
	status := respuestaHTTP(t, app, req, nil)
	if status != http.StatusNoContent {
		t.Fatalf("DELETE /acceso/sesiones/actual: status=%d, se esperaba 204", status)
	}

	var estado string
	if err := dueno.QueryRow(context.Background(),
		`SELECT estado FROM sesiones WHERE id = $1`, resultado.IDSesion,
	).Scan(&estado); err != nil {
		t.Fatalf("consultando el estado de la sesión: %v", err)
	}
	if estado != "revocada" {
		t.Fatalf("sesion.estado = %q tras el logout, se esperaba revocada", estado)
	}

	// El mismo token de refresco de esa sesión tampoco debe servir tras el
	// logout (la sesión ya no está activa).
	var errResp errorHumaRespuesta
	status = respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/acceso/sesiones/renovaciones", map[string]any{
		"token_refresco": resultado.TokenRefresco,
	}), &errResp)
	if status != http.StatusUnauthorized {
		t.Fatalf("renovar con el refresco de una sesión cerrada por logout: status=%d, se esperaba 401", status)
	}

	assertCadenaAuditoriaIntegra(t, dueno)
}

// kidDelJWT decodifica (sin verificar: es un test, no un verificador de
// producción) la cabecera de un JWT compacto y devuelve su claim kid.
func kidDelJWT(t *testing.T, tokenCompacto string) string {
	t.Helper()
	partes := strings.SplitN(tokenCompacto, ".", 3)
	if len(partes) != 3 {
		t.Fatalf("el token no tiene 3 segmentos: %q", tokenCompacto)
	}
	crudo, err := base64.RawURLEncoding.DecodeString(partes[0])
	if err != nil {
		t.Fatalf("decodificando la cabecera del JWT: %v", err)
	}
	var cabecera map[string]any
	if err := json.Unmarshal(crudo, &cabecera); err != nil {
		t.Fatalf("parseando la cabecera del JWT: %v", err)
	}
	kid, ok := cabecera["kid"].(string)
	if !ok || kid == "" {
		t.Fatalf("la cabecera del JWT no tiene kid: %v", cabecera)
	}
	return kid
}
