package integracion

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/r-david1/moterus/internal/identidad/dominio"
	"github.com/r-david1/moterus/internal/identidad/puertos"
)

// Este archivo ejercita el flujo de verificación de correo (sección 3.4 del
// diseño de Identidad, INV-ID-21/INV-ID-22): POST /identidad/verificaciones-correo
// y POST /identidad/verificaciones-correo/reenvios, contra Postgres real.
// El flujo se había verificado solo a mano (curl/psql) hasta ahora — ver
// docs/design/identidad-bounded-context.md, sección 8, paso 7: "un usuario
// recién registrado queda en pendiente_verificacion sin ninguna forma de
// pasar a activo salvo una actualización manual directa en base de datos".
//
// NotificadorCorreoLog (el adaptador real de este hito) es log-only: el
// token de verificación en claro solo se escribe en el logger del proceso,
// nunca en un puerto de salida legible desde un test. Por eso la mayoría de
// los tests de este archivo NO usan nuevoServidorIdentidad, sino
// nuevoServidorIdentidadConNotificador con notificadorCorreoCaptura (abajo):
// un NotificadorCorreo de prueba que captura el token en memoria en vez de
// solo loguearlo.
//
// Guardián de perímetro (ADR 0018): nuevoServidorIdentidad(ConNotificador)
// monta EvaluadorConfianzaNoOp a propósito (ver el comentario en
// entorno_test.go), así que ninguno de los tests "de negocio" de este
// archivo puede toparse con el rate limit real de reenvío (5/min por IP,
// 3/15min por cuenta) — evita que un 429 de perímetro se confunda con el
// comportamiento de negocio de ReenviarVerificacionCasoDeUso, que es
// exactamente el riesgo señalado en el encargo. El guardián real SÍ se
// ejercita, deliberadamente aparte, en
// TestHTTP_ReenviarVerificacion_GuardianDePerimetroReal_BloqueaTrasElUmbral
// (harness nuevoServidorIdentidadConConfianzaReal, requiere REDIS_URL).

// notificadorCorreoCaptura implementa puertos.NotificadorCorreo capturando
// en memoria el último token en claro enviado a cada correo, en vez de
// enviarlo/loguearlo de verdad. Es el único punto de todo el sistema (aparte
// del propio NotificadorCorreoLog real, dev-only) donde el token de
// verificación en claro es observable — INV-ID-21 lo permite explícitamente
// para el consumidor del puerto, y este test double no hace nada más con él
// que devolverlo a quien pregunta.
type notificadorCorreoCaptura struct {
	mu     sync.Mutex
	tokens map[string]string // correo normalizado -> último token plano
}

var _ puertos.NotificadorCorreo = (*notificadorCorreoCaptura)(nil)

func nuevoNotificadorCorreoCaptura() *notificadorCorreoCaptura {
	return &notificadorCorreoCaptura{tokens: make(map[string]string)}
}

func (n *notificadorCorreoCaptura) EnviarVerificacion(_ context.Context, correo dominio.Correo, tokenPlano string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.tokens[correo.Normalizado()] = tokenPlano
	return nil
}

// tokenPara devuelve el token en claro capturado para correo, o falla el
// test si nunca se capturó ninguno (p. ej. porque el registro/reenvío
// falló antes de llegar a NotificadorCorreo).
func (n *notificadorCorreoCaptura) tokenPara(t *testing.T, correo string) string {
	t.Helper()
	clave := strings.ToLower(strings.TrimSpace(correo))
	n.mu.Lock()
	defer n.mu.Unlock()
	token, ok := n.tokens[clave]
	if !ok {
		t.Fatalf("notificadorCorreoCaptura: no se capturó ningún token de verificación para %q", correo)
	}
	return token
}

// --- DTOs de prueba de este archivo (proyección 1:1 de adaptadores/http/dtos.go) ---

type verificarCorreoPeticionPrueba struct {
	Token string `json:"token"`
}

type reenviarVerificacionPeticionPrueba struct {
	Correo string `json:"correo"`
}

// --- 1. Verificación exitosa ------------------------------------------------

func TestHTTP_VerificarCorreo_Exitoso_TransicionaUsuarioDePendienteVerificacionAActivo(t *testing.T) {
	pool := poolAplicacion(t)
	duenoPool := poolDueno(t)
	captura := nuevoNotificadorCorreoCaptura()
	app := nuevoServidorIdentidadConNotificador(t, pool, captura)

	correo := correoUnico(t, "verificacion-exitosa")
	var registro registroRespuestaPrueba
	statusRegistro := respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/identidad/usuarios", registroPeticionPrueba{
		Correo: correo, Contrasena: contrasenaFuerteDePrueba,
	}), &registro)
	t.Cleanup(func() { borrarUsuario(t, duenoPool, registro.IDUsuario) })
	if statusRegistro != http.StatusOK {
		t.Fatalf("el registro (setup) = %d, esperado %d", statusRegistro, http.StatusOK)
	}

	token := captura.tokenPara(t, correo)

	status := respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/identidad/verificaciones-correo", verificarCorreoPeticionPrueba{
		Token: token,
	}), nil)
	if status != http.StatusOK {
		t.Fatalf("POST /identidad/verificaciones-correo con token válido = %d, esperado %d", status, http.StatusOK)
	}

	// Verificación directa contra la tabla usuarios (INV-ID-07/06): el
	// estado persistido, no solo el status HTTP.
	var estadoBD string
	if err := duenoPool.QueryRow(t.Context(), `SELECT estado FROM usuarios WHERE id = $1`, registro.IDUsuario).Scan(&estadoBD); err != nil {
		t.Fatalf("consultando usuarios tras la verificación: %v", err)
	}
	if estadoBD != "activo" {
		t.Errorf("estado persistido tras verificar = %q, esperado %q", estadoBD, "activo")
	}

	// El token es de un solo uso (sección 3.4 del diseño): se elimina de la
	// tabla al consumirse con éxito.
	var quedanTokens int
	if err := duenoPool.QueryRow(t.Context(), `SELECT count(*) FROM tokens_verificacion_correo WHERE usuario_id = $1`, registro.IDUsuario).Scan(&quedanTokens); err != nil {
		t.Fatalf("consultando tokens_verificacion_correo: %v", err)
	}
	if quedanTokens != 0 {
		t.Errorf("tokens_verificacion_correo con usuario_id=%s tras verificar = %d filas, esperado 0 (token de un solo uso)", registro.IDUsuario, quedanTokens)
	}
}

// --- 2 y 3. Token ya usado y token inventado: mismo 404, mismo cuerpo -------

// TestHTTP_VerificarCorreo_TokenReusadoYTokenInventado_MismoStatusYCuerpo
// formaliza el mismo criterio anti-enumeración que ya cubre
// TestHTTP_Login_ContrasenaIncorrectaYCorreoInexistente_MismoStatusYCuerpo
// (INV-ID-11) para el endpoint de verificación de correo: un token que ya
// se consumió y un token que nunca existió deben producir exactamente el
// mismo status y el mismo cuerpo — ninguno de los dos debe revelar si
// "alguna vez hubo un token ahí" o no.
func TestHTTP_VerificarCorreo_TokenReusadoYTokenInventado_MismoStatusYCuerpo(t *testing.T) {
	pool := poolAplicacion(t)
	duenoPool := poolDueno(t)
	captura := nuevoNotificadorCorreoCaptura()
	app := nuevoServidorIdentidadConNotificador(t, pool, captura)

	correo := correoUnico(t, "verificacion-token-reusado")
	var registro registroRespuestaPrueba
	statusRegistro := respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/identidad/usuarios", registroPeticionPrueba{
		Correo: correo, Contrasena: contrasenaFuerteDePrueba,
	}), &registro)
	t.Cleanup(func() { borrarUsuario(t, duenoPool, registro.IDUsuario) })
	if statusRegistro != http.StatusOK {
		t.Fatalf("el registro (setup) = %d, esperado %d", statusRegistro, http.StatusOK)
	}
	token := captura.tokenPara(t, correo)

	// Primer consumo: éxito (setup, ya cubierto en detalle por el test de
	// arriba; aquí solo hace falta que el token quede gastado).
	statusPrimerUso := respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/identidad/verificaciones-correo", verificarCorreoPeticionPrueba{
		Token: token,
	}), nil)
	if statusPrimerUso != http.StatusOK {
		t.Fatalf("primer uso del token (setup) = %d, esperado %d", statusPrimerUso, http.StatusOK)
	}

	// Segundo uso del MISMO token, ya consumido.
	var respuestaTokenReusado errorHumaRespuesta
	statusTokenReusado := respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/identidad/verificaciones-correo", verificarCorreoPeticionPrueba{
		Token: token,
	}), &respuestaTokenReusado)

	// Token que nunca existió (string arbitraria, misma forma que un token
	// real: base64url de alta entropía).
	var respuestaTokenInventado errorHumaRespuesta
	statusTokenInventado := respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/identidad/verificaciones-correo", verificarCorreoPeticionPrueba{
		Token: "token-que-nunca-existio-" + correoUnico(t, "invento"),
	}), &respuestaTokenInventado)

	if statusTokenReusado != http.StatusNotFound {
		t.Errorf("status (token reusado) = %d, esperado %d", statusTokenReusado, http.StatusNotFound)
	}
	if statusTokenInventado != http.StatusNotFound {
		t.Errorf("status (token inventado) = %d, esperado %d", statusTokenInventado, http.StatusNotFound)
	}
	if statusTokenReusado != statusTokenInventado {
		t.Fatalf("status distinto entre token reusado (%d) y token inventado (%d): filtra información por el canal HTTP",
			statusTokenReusado, statusTokenInventado)
	}
	if respuestaTokenReusado != respuestaTokenInventado {
		t.Fatalf("cuerpo distinto entre token reusado (%+v) y token inventado (%+v): filtra información por el canal HTTP",
			respuestaTokenReusado, respuestaTokenInventado)
	}
	if respuestaTokenReusado.Detail != "token de verificación inválido" {
		t.Errorf("detail = %q, esperado el mensaje genérico %q", respuestaTokenReusado.Detail, "token de verificación inválido")
	}
}

// TestHTTP_VerificarCorreo_TokenExpirado_Devuelve410YEliminaElTokenVencido
// no está en la lista original de casos pero cierra el hueco: el diseño
// (sección 3.4) distingue explícitamente "no encontrado" (404) de "expirado"
// (410 Gone), y ese camino no lo cubre ningún otro test. Se fuerza la
// expiración manipulando expira_en directamente en la tabla (con el rol
// dueño) en vez de esperar 24h reales.
func TestHTTP_VerificarCorreo_TokenExpirado_Devuelve410YEliminaElTokenVencido(t *testing.T) {
	pool := poolAplicacion(t)
	duenoPool := poolDueno(t)
	captura := nuevoNotificadorCorreoCaptura()
	app := nuevoServidorIdentidadConNotificador(t, pool, captura)

	correo := correoUnico(t, "verificacion-token-expirado")
	var registro registroRespuestaPrueba
	statusRegistro := respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/identidad/usuarios", registroPeticionPrueba{
		Correo: correo, Contrasena: contrasenaFuerteDePrueba,
	}), &registro)
	t.Cleanup(func() { borrarUsuario(t, duenoPool, registro.IDUsuario) })
	if statusRegistro != http.StatusOK {
		t.Fatalf("el registro (setup) = %d, esperado %d", statusRegistro, http.StatusOK)
	}
	token := captura.tokenPara(t, correo)

	tag, err := duenoPool.Exec(t.Context(),
		`UPDATE tokens_verificacion_correo SET expira_en = now() - interval '1 minute' WHERE usuario_id = $1`,
		registro.IDUsuario)
	if err != nil {
		t.Fatalf("forzando la expiración del token (setup): %v", err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("forzando la expiración del token (setup): se esperaba 1 fila afectada, hubo %d", tag.RowsAffected())
	}

	var errorRespuesta errorHumaRespuesta
	status := respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/identidad/verificaciones-correo", verificarCorreoPeticionPrueba{
		Token: token,
	}), &errorRespuesta)

	if status != http.StatusGone {
		t.Fatalf("POST /identidad/verificaciones-correo con token expirado = %d, esperado %d", status, http.StatusGone)
	}
	if errorRespuesta.Detail != "token de verificación expirado" {
		t.Errorf("detail = %q, esperado %q", errorRespuesta.Detail, "token de verificación expirado")
	}

	// El usuario sigue en pendiente_verificacion: un token expirado nunca
	// transiciona el agregado.
	var estadoBD string
	if err := duenoPool.QueryRow(t.Context(), `SELECT estado FROM usuarios WHERE id = $1`, registro.IDUsuario).Scan(&estadoBD); err != nil {
		t.Fatalf("consultando usuarios tras el intento con token expirado: %v", err)
	}
	if estadoBD != "pendiente_verificacion" {
		t.Errorf("estado persistido tras un intento con token expirado = %q, esperado %q", estadoBD, "pendiente_verificacion")
	}

	// El token vencido se elimina como parte del propio flujo de fallo
	// (verificar_correo.go: "se elimina el token vencido dentro de la misma
	// transacción que su auditoría").
	var quedanTokens int
	if err := duenoPool.QueryRow(t.Context(), `SELECT count(*) FROM tokens_verificacion_correo WHERE usuario_id = $1`, registro.IDUsuario).Scan(&quedanTokens); err != nil {
		t.Fatalf("consultando tokens_verificacion_correo: %v", err)
	}
	if quedanTokens != 0 {
		t.Errorf("tokens_verificacion_correo con usuario_id=%s tras el intento expirado = %d filas, esperado 0", registro.IDUsuario, quedanTokens)
	}
}

// --- 4. Reenvío: respuesta siempre neutra (INV-ID-22) -----------------------

func TestHTTP_ReenviarVerificacion_CorreoExistenteYCorreoInexistente_Devuelve202CuerpoVacioIdentico(t *testing.T) {
	pool := poolAplicacion(t)
	duenoPool := poolDueno(t)
	app := nuevoServidorIdentidad(t, pool) // NoOp confianza: ver comentario de cabecera del archivo.

	correoExistente := correoUnico(t, "reenvio-correo-existente")
	var registro registroRespuestaPrueba
	statusRegistro := respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/identidad/usuarios", registroPeticionPrueba{
		Correo: correoExistente, Contrasena: contrasenaFuerteDePrueba,
	}), &registro)
	t.Cleanup(func() { borrarUsuario(t, duenoPool, registro.IDUsuario) })
	if statusRegistro != http.StatusOK {
		t.Fatalf("el registro (setup) = %d, esperado %d", statusRegistro, http.StatusOK)
	}

	statusExistente, cuerpoExistente := cuerpoCrudoHTTP(t, app, peticionJSON(t, http.MethodPost, "/identidad/verificaciones-correo/reenvios", reenviarVerificacionPeticionPrueba{
		Correo: correoExistente,
	}))

	correoInexistente := correoUnico(t, "reenvio-correo-inexistente")
	statusInexistente, cuerpoInexistente := cuerpoCrudoHTTP(t, app, peticionJSON(t, http.MethodPost, "/identidad/verificaciones-correo/reenvios", reenviarVerificacionPeticionPrueba{
		Correo: correoInexistente,
	}))

	if statusExistente != http.StatusAccepted {
		t.Errorf("status (correo existente) = %d, esperado %d", statusExistente, http.StatusAccepted)
	}
	if statusInexistente != http.StatusAccepted {
		t.Errorf("status (correo inexistente) = %d, esperado %d", statusInexistente, http.StatusAccepted)
	}
	if statusExistente != statusInexistente {
		t.Fatalf("INV-ID-22 violada: status distinto entre correo existente (%d) e inexistente (%d)", statusExistente, statusInexistente)
	}
	if len(cuerpoExistente) != 0 {
		t.Errorf("cuerpo (correo existente) = %q, esperado vacío (INV-ID-22)", cuerpoExistente)
	}
	if len(cuerpoInexistente) != 0 {
		t.Errorf("cuerpo (correo inexistente) = %q, esperado vacío (INV-ID-22)", cuerpoInexistente)
	}
	if string(cuerpoExistente) != string(cuerpoInexistente) {
		t.Fatalf("INV-ID-22 violada: cuerpo distinto entre correo existente (%q) e inexistente (%q)", cuerpoExistente, cuerpoInexistente)
	}
}

// TestHTTP_ReenviarVerificacion_CorreoYaActivo_Devuelve202SinTransicionarEstado
// cierra otra rama de INV-ID-22 explícitamente mencionada en el diseño
// ("en cualquier otro caso... ya activo... no hace nada observable"): un
// correo que ya se verificó también recibe el mismo 202 neutro, sin generar
// un token nuevo.
func TestHTTP_ReenviarVerificacion_CorreoYaActivo_Devuelve202SinGenerarTokenNuevo(t *testing.T) {
	pool := poolAplicacion(t)
	duenoPool := poolDueno(t)
	captura := nuevoNotificadorCorreoCaptura()
	app := nuevoServidorIdentidadConNotificador(t, pool, captura)

	correo := correoUnico(t, "reenvio-correo-ya-activo")
	var registro registroRespuestaPrueba
	statusRegistro := respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/identidad/usuarios", registroPeticionPrueba{
		Correo: correo, Contrasena: contrasenaFuerteDePrueba,
	}), &registro)
	t.Cleanup(func() { borrarUsuario(t, duenoPool, registro.IDUsuario) })
	if statusRegistro != http.StatusOK {
		t.Fatalf("el registro (setup) = %d, esperado %d", statusRegistro, http.StatusOK)
	}
	// El registro ya genera un token de verificación propio (sección 3.4 del
	// diseño): se captura ANTES de activar la cuenta a mano, para poder
	// comprobar después que el reenvío no lo tocó — activarUsuario transiciona
	// el estado saltándose el flujo real y deliberadamente no toca
	// tokens_verificacion_correo (ver su comentario en entorno_test.go), así
	// que la fila de ese token original sigue viva y NO debe confundirse con
	// un token nuevo generado por este reenvío.
	tokenOriginalDelRegistro := captura.tokenPara(t, correo)
	activarUsuario(t, duenoPool, registro.IDUsuario)

	status, cuerpo := cuerpoCrudoHTTP(t, app, peticionJSON(t, http.MethodPost, "/identidad/verificaciones-correo/reenvios", reenviarVerificacionPeticionPrueba{
		Correo: correo,
	}))
	if status != http.StatusAccepted {
		t.Fatalf("reenvío sobre cuenta ya activa = %d, esperado %d (INV-ID-22)", status, http.StatusAccepted)
	}
	if len(cuerpo) != 0 {
		t.Errorf("cuerpo = %q, esperado vacío", cuerpo)
	}

	// ReenviarVerificacionCasoDeUso.Reenviar nunca llama a
	// NotificadorCorreo.EnviarVerificacion para una cuenta que no está en
	// pendiente_verificacion: el token capturado sigue siendo el del
	// registro original, no uno nuevo.
	if tokenTrasReenvio := captura.tokenPara(t, correo); tokenTrasReenvio != tokenOriginalDelRegistro {
		t.Errorf("el token capturado cambió tras reenviar sobre una cuenta ya activa (%q -> %q): se envió un correo que no debía enviarse",
			tokenOriginalDelRegistro, tokenTrasReenvio)
	}

	var cantidadTokens int
	if err := duenoPool.QueryRow(t.Context(), `SELECT count(*) FROM tokens_verificacion_correo WHERE usuario_id = $1`, registro.IDUsuario).Scan(&cantidadTokens); err != nil {
		t.Fatalf("consultando tokens_verificacion_correo: %v", err)
	}
	if cantidadTokens != 1 {
		t.Errorf("tokens_verificacion_correo con usuario_id=%s tras reenviar sobre cuenta activa = %d filas, esperado 1 (el original del registro, sin tocar — activarUsuario nunca lo elimina)",
			registro.IDUsuario, cantidadTokens)
	}
}

// --- 5. Reenvío genera un token nuevo que reemplaza al anterior -------------

func TestHTTP_ReenviarVerificacion_GeneraTokenNuevoQueReemplazaAlAnterior(t *testing.T) {
	pool := poolAplicacion(t)
	duenoPool := poolDueno(t)
	captura := nuevoNotificadorCorreoCaptura()
	app := nuevoServidorIdentidadConNotificador(t, pool, captura)

	// Un único reenvío en todo el test: el harness usa EvaluadorConfianzaNoOp
	// (ver comentario de cabecera), pero aun así se mantiene la disciplina de
	// "no más de lo necesario" por si este test se reutiliza alguna vez
	// contra un harness con el guardián real activado.
	correo := correoUnico(t, "reenvio-reemplaza-token")
	var registro registroRespuestaPrueba
	statusRegistro := respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/identidad/usuarios", registroPeticionPrueba{
		Correo: correo, Contrasena: contrasenaFuerteDePrueba,
	}), &registro)
	t.Cleanup(func() { borrarUsuario(t, duenoPool, registro.IDUsuario) })
	if statusRegistro != http.StatusOK {
		t.Fatalf("el registro (setup) = %d, esperado %d", statusRegistro, http.StatusOK)
	}
	tokenOriginal := captura.tokenPara(t, correo)

	statusReenvio := respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/identidad/verificaciones-correo/reenvios", reenviarVerificacionPeticionPrueba{
		Correo: correo,
	}), nil)
	if statusReenvio != http.StatusAccepted {
		t.Fatalf("POST .../reenvios = %d, esperado %d", statusReenvio, http.StatusAccepted)
	}
	tokenNuevo := captura.tokenPara(t, correo)

	if tokenNuevo == tokenOriginal {
		t.Fatalf("el reenvío debe generar un token distinto del original; ambos son %q", tokenNuevo)
	}

	// Solo debe quedar un token activo para el usuario (usuario_id UNIQUE,
	// upsert por reenvío — sección 3.4 del diseño).
	var cantidadTokens int
	if err := duenoPool.QueryRow(t.Context(), `SELECT count(*) FROM tokens_verificacion_correo WHERE usuario_id = $1`, registro.IDUsuario).Scan(&cantidadTokens); err != nil {
		t.Fatalf("consultando tokens_verificacion_correo: %v", err)
	}
	if cantidadTokens != 1 {
		t.Fatalf("tokens_verificacion_correo con usuario_id=%s = %d filas, esperado exactamente 1 (upsert)", registro.IDUsuario, cantidadTokens)
	}

	// El token original ya no sirve (fue reemplazado, no coexiste con el nuevo).
	var respuestaTokenOriginal errorHumaRespuesta
	statusTokenOriginal := respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/identidad/verificaciones-correo", verificarCorreoPeticionPrueba{
		Token: tokenOriginal,
	}), &respuestaTokenOriginal)
	if statusTokenOriginal != http.StatusNotFound {
		t.Errorf("verificar con el token ORIGINAL tras un reenvío = %d, esperado %d (debe haber sido invalidado)", statusTokenOriginal, http.StatusNotFound)
	}

	// El token nuevo sí funciona y transiciona al usuario a activo.
	statusTokenNuevo := respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/identidad/verificaciones-correo", verificarCorreoPeticionPrueba{
		Token: tokenNuevo,
	}), nil)
	if statusTokenNuevo != http.StatusOK {
		t.Fatalf("verificar con el token NUEVO (tras reenvío) = %d, esperado %d", statusTokenNuevo, http.StatusOK)
	}

	var estadoBD string
	if err := duenoPool.QueryRow(t.Context(), `SELECT estado FROM usuarios WHERE id = $1`, registro.IDUsuario).Scan(&estadoBD); err != nil {
		t.Fatalf("consultando usuarios tras verificar con el token nuevo: %v", err)
	}
	if estadoBD != "activo" {
		t.Errorf("estado persistido tras verificar con el token nuevo = %q, esperado %q", estadoBD, "activo")
	}
}

// --- 6. Cambio de comportamiento en login antes/después de verificar --------

// TestHTTP_FlujoLogin_AntesYDespuesDeVerificarCorreo replica, dentro del
// mismo test, el motivo exacto por el que autenticar_usuario.go exige
// PuedeIniciarSesion() DESPUÉS de verificar la contraseña (INV-ID-06): antes
// de verificar el correo, un login con credenciales correctas debe fallar
// con 403 "correo no verificado" (dominio.ErrCorreoNoVerificado, ver
// TestHTTP_Login_AntesDeVerificarCorreo_Devuelve403 en http_flujo_test.go,
// que solo cubre esta primera mitad); después de verificar, exactamente las
// mismas credenciales deben autenticar con éxito y sin esa restricción.
func TestHTTP_FlujoLogin_AntesYDespuesDeVerificarCorreo(t *testing.T) {
	pool := poolAplicacion(t)
	duenoPool := poolDueno(t)
	captura := nuevoNotificadorCorreoCaptura()
	app := nuevoServidorIdentidadConNotificador(t, pool, captura)

	correo := correoUnico(t, "login-antes-y-despues-de-verificar")
	var registro registroRespuestaPrueba
	statusRegistro := respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/identidad/usuarios", registroPeticionPrueba{
		Correo: correo, Contrasena: contrasenaFuerteDePrueba,
	}), &registro)
	t.Cleanup(func() { borrarUsuario(t, duenoPool, registro.IDUsuario) })
	if statusRegistro != http.StatusOK {
		t.Fatalf("el registro (setup) = %d, esperado %d", statusRegistro, http.StatusOK)
	}

	// Antes de verificar: 403, correo no verificado (INV-ID-06).
	var errorAntes errorHumaRespuesta
	statusAntes := respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/identidad/autenticaciones", autenticarPeticionPrueba{
		Correo: correo, Contrasena: contrasenaFuerteDePrueba,
	}), &errorAntes)
	if statusAntes != http.StatusForbidden {
		t.Fatalf("login antes de verificar = %d, esperado %d", statusAntes, http.StatusForbidden)
	}
	if errorAntes.Detail != "correo no verificado" {
		t.Errorf("detail (antes de verificar) = %q, esperado %q", errorAntes.Detail, "correo no verificado")
	}

	token := captura.tokenPara(t, correo)
	statusVerificar := respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/identidad/verificaciones-correo", verificarCorreoPeticionPrueba{
		Token: token,
	}), nil)
	if statusVerificar != http.StatusOK {
		t.Fatalf("verificación de correo (setup) = %d, esperado %d", statusVerificar, http.StatusOK)
	}

	// Después de verificar: 200, mismo correo y misma contraseña, sin la
	// restricción de correo no verificado.
	var respuestaDespues autenticarRespuestaPrueba
	statusDespues := respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/identidad/autenticaciones", autenticarPeticionPrueba{
		Correo: correo, Contrasena: contrasenaFuerteDePrueba,
	}), &respuestaDespues)
	if statusDespues != http.StatusOK {
		t.Fatalf("login después de verificar = %d, esperado %d", statusDespues, http.StatusOK)
	}
	if respuestaDespues.Estado != "activo" {
		t.Errorf("Estado (después de verificar) = %q, esperado %q", respuestaDespues.Estado, "activo")
	}
	if respuestaDespues.IDUsuario != registro.IDUsuario {
		t.Errorf("IDUsuario = %q, esperado %q", respuestaDespues.IDUsuario, registro.IDUsuario)
	}
}

// --- 7. Auditoría: acciones y cadena íntegra ---------------------------------

// TestHTTP_VerificarCorreo_FlujoCompletoAuditado_AccionesEsperadasYCadenaIntegra
// ejercita registro -> intento con token inventado (fallo) -> verificación
// real (éxito) -> reuso del token ya consumido (fallo) -> login (éxito), y
// después inspecciona directamente la tabla auditoria (rol dueño) para
// verificar que la acción usuario.correo_verificado (catálogo cerrado,
// docs/catalogos/acciones-auditoria.md) quedó registrada con
// resultado=exito en el caso feliz y resultado=fallo en ambos casos
// inválidos, y que verificar_cadena_auditoria() sigue íntegra (mismo patrón
// que TestAuditoria_FlujoHTTPCompleto_GeneraAccionesEsperadasYCadenaIntegra
// en auditoria_test.go, que no cubre este endpoint).
//
// Correlación: el token inventado nunca resuelve a un usuario_id (la fila de
// auditoría queda con usuario_id NULL), así que ese caso se correlaciona por
// id_solicitud (cabecera X-Request-Id, ver adaptadores/http/middleware.go)
// en vez de por usuario_id.
func TestHTTP_VerificarCorreo_FlujoCompletoAuditado_AccionesEsperadasYCadenaIntegra(t *testing.T) {
	pool := poolAplicacion(t)
	duenoPool := poolDueno(t)
	captura := nuevoNotificadorCorreoCaptura()
	app := nuevoServidorIdentidadConNotificador(t, pool, captura)

	correo := correoUnico(t, "auditoria-verificacion-correo")
	var registro registroRespuestaPrueba
	statusRegistro := respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/identidad/usuarios", registroPeticionPrueba{
		Correo: correo, Contrasena: contrasenaFuerteDePrueba,
	}), &registro)
	t.Cleanup(func() { borrarUsuario(t, duenoPool, registro.IDUsuario) })
	if statusRegistro != http.StatusOK {
		t.Fatalf("el registro (setup) = %d, esperado %d", statusRegistro, http.StatusOK)
	}

	idSolicitudTokenInventado := "id-solicitud-" + correoUnico(t, "token-inventado")
	peticionTokenInventado := peticionJSON(t, http.MethodPost, "/identidad/verificaciones-correo", verificarCorreoPeticionPrueba{
		Token: "token-jamas-emitido-" + correoUnico(t, "x"),
	})
	peticionTokenInventado.Header.Set("X-Request-Id", idSolicitudTokenInventado)
	statusTokenInventado := respuestaHTTP(t, app, peticionTokenInventado, nil)
	if statusTokenInventado != http.StatusNotFound {
		t.Fatalf("token inventado (setup) = %d, esperado %d", statusTokenInventado, http.StatusNotFound)
	}

	token := captura.tokenPara(t, correo)
	statusVerificar := respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/identidad/verificaciones-correo", verificarCorreoPeticionPrueba{
		Token: token,
	}), nil)
	if statusVerificar != http.StatusOK {
		t.Fatalf("verificación exitosa (setup) = %d, esperado %d", statusVerificar, http.StatusOK)
	}

	idSolicitudTokenReusado := "id-solicitud-" + correoUnico(t, "token-reusado")
	peticionTokenReusado := peticionJSON(t, http.MethodPost, "/identidad/verificaciones-correo", verificarCorreoPeticionPrueba{
		Token: token,
	})
	peticionTokenReusado.Header.Set("X-Request-Id", idSolicitudTokenReusado)
	statusTokenReusado := respuestaHTTP(t, app, peticionTokenReusado, nil)
	if statusTokenReusado != http.StatusNotFound {
		t.Fatalf("reuso del token ya consumido (setup) = %d, esperado %d", statusTokenReusado, http.StatusNotFound)
	}

	statusLogin := respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/identidad/autenticaciones", autenticarPeticionPrueba{
		Correo: correo, Contrasena: contrasenaFuerteDePrueba,
	}), nil)
	if statusLogin != http.StatusOK {
		t.Fatalf("login tras verificar (setup) = %d, esperado %d", statusLogin, http.StatusOK)
	}

	// --- verificación directa de la bitácora (rol dueño, sin restricciones) ---

	var resultadoExito string
	if err := duenoPool.QueryRow(t.Context(),
		`SELECT resultado FROM auditoria WHERE accion = 'usuario.correo_verificado' AND usuario_id = $1::uuid`,
		registro.IDUsuario,
	).Scan(&resultadoExito); err != nil {
		t.Fatalf("consultando auditoria (caso éxito, correlacionado por usuario_id): %v", err)
	}
	if resultadoExito != "exito" {
		t.Errorf("resultado de usuario.correo_verificado (caso éxito) = %q, esperado %q", resultadoExito, "exito")
	}

	var resultadoTokenInventado string
	if err := duenoPool.QueryRow(t.Context(),
		`SELECT resultado FROM auditoria WHERE accion = 'usuario.correo_verificado' AND id_solicitud = $1`,
		idSolicitudTokenInventado,
	).Scan(&resultadoTokenInventado); err != nil {
		t.Fatalf("consultando auditoria (token inventado, correlacionado por id_solicitud): %v", err)
	}
	if resultadoTokenInventado != "fallo" {
		t.Errorf("resultado de usuario.correo_verificado (token inventado) = %q, esperado %q", resultadoTokenInventado, "fallo")
	}

	var resultadoTokenReusado string
	if err := duenoPool.QueryRow(t.Context(),
		`SELECT resultado FROM auditoria WHERE accion = 'usuario.correo_verificado' AND id_solicitud = $1`,
		idSolicitudTokenReusado,
	).Scan(&resultadoTokenReusado); err != nil {
		t.Fatalf("consultando auditoria (token reusado, correlacionado por id_solicitud): %v", err)
	}
	if resultadoTokenReusado != "fallo" {
		t.Errorf("resultado de usuario.correo_verificado (token reusado) = %q, esperado %q", resultadoTokenReusado, "fallo")
	}

	var resultadoLogin string
	if err := duenoPool.QueryRow(t.Context(),
		`SELECT resultado FROM auditoria WHERE accion = 'usuario.login' AND usuario_id = $1::uuid`,
		registro.IDUsuario,
	).Scan(&resultadoLogin); err != nil {
		t.Fatalf("consultando auditoria (login tras verificar): %v", err)
	}
	if resultadoLogin != "exito" {
		t.Errorf("resultado de usuario.login tras verificar = %q, esperado %q", resultadoLogin, "exito")
	}

	assertCadenaAuditoriaIntegra(t, duenoPool)
}

// --- Seguridad: el guardián de perímetro real bloquea reenvíos en ráfaga ----

// TestHTTP_ReenviarVerificacion_GuardianDePerimetroReal_BloqueaTrasElUmbral
// es el caso de seguridad que el resto de este archivo deja fuera a
// propósito (ver comentario de cabecera): ejercita
// ManejadorIdentidad.ReenviarVerificacion con EvaluadorConfianzaReal
// (ADR 0018) sobre Redis real, para comprobar que el guardián de perímetro
// del endpoint de reenvío efectivamente devuelve 429 después del umbral
// documentado en PoliticaLimitesPorDefecto para "reenvio_verificacion":
// cuenta = 3 intentos / 15 min. Se limita a 4 llamadas (3 permitidas + 1
// bloqueada) contra un correo único, muy por debajo del límite por IP
// (5/min) para no acoplar este test a los contadores por IP compartidos con
// otros tests que puedan usar el mismo harness en la misma corrida.
func TestHTTP_ReenviarVerificacion_GuardianDePerimetroReal_BloqueaTrasElUmbral(t *testing.T) {
	pool := poolAplicacion(t)
	duenoPool := poolDueno(t)
	limitador := clienteRedis(t) // se salta limpiamente sin REDIS_URL.
	app := nuevoServidorIdentidadConConfianzaReal(t, pool)

	correo := correoUnico(t, "reenvio-guardian-real")

	// Clave documentada en confianza/aplicacion/evaluar_trust_signal.go
	// (claveLimite): "confianza:rl:<nivel>:<accion>:<identificador>". La
	// clave de IP usa "0.0.0.0": httptest.NewRequest dentro de app.Test no
	// fija RemoteAddr, así que c.IP() (middlewareOrigenSolicitud) resuelve
	// siempre a esa dirección para cualquier request de este paquete —
	// verificado empíricamente contra Redis real (docker exec redis-cli
	// KEYS 'confianza:rl:*'). Se reinician ANTES de empezar (por si quedó
	// contaminada por una corrida anterior interrumpida) y al terminar. Si
	// otro test futuro llega a ejercer reenvio_verificacion contra este
	// mismo harness en la misma corrida, comparte este cupo de IP; hoy es
	// el único.
	claveCuenta := fmt.Sprintf("confianza:rl:cuenta:reenvio_verificacion:%s", strings.ToLower(strings.TrimSpace(correo)))
	claveIP := "confianza:rl:ip:reenvio_verificacion:0.0.0.0"
	reiniciarLimites := func() {
		_ = limitador.Reiniciar(context.Background(), claveCuenta)
		_ = limitador.Reiniciar(context.Background(), claveIP)
	}
	reiniciarLimites()
	t.Cleanup(reiniciarLimites)

	var registro registroRespuestaPrueba
	statusRegistro := respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/identidad/usuarios", registroPeticionPrueba{
		Correo: correo, Contrasena: contrasenaFuerteDePrueba,
	}), &registro)
	t.Cleanup(func() { borrarUsuario(t, duenoPool, registro.IDUsuario) })
	if statusRegistro != http.StatusOK {
		t.Fatalf("el registro (setup) = %d, esperado %d", statusRegistro, http.StatusOK)
	}

	for i := 1; i <= 3; i++ {
		status := respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/identidad/verificaciones-correo/reenvios", reenviarVerificacionPeticionPrueba{
			Correo: correo,
		}), nil)
		if status != http.StatusAccepted {
			t.Fatalf("reenvío #%d (dentro del umbral de cuenta) = %d, esperado %d", i, status, http.StatusAccepted)
		}
	}

	var errorRespuesta errorHumaRespuesta
	statusBloqueado := respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/identidad/verificaciones-correo/reenvios", reenviarVerificacionPeticionPrueba{
		Correo: correo,
	}), &errorRespuesta)
	if statusBloqueado != http.StatusTooManyRequests {
		t.Fatalf("reenvío #4 (debe exceder el umbral de cuenta, 3/15min) = %d, esperado %d", statusBloqueado, http.StatusTooManyRequests)
	}
	if errorRespuesta.Status != http.StatusTooManyRequests {
		t.Errorf("cuerpo.status = %d, esperado %d", errorRespuesta.Status, http.StatusTooManyRequests)
	}
}

// cuerpoCrudoHTTP es como respuestaHTTP (entorno_test.go) pero devuelve el
// cuerpo crudo (sin decodificar) además del status: lo necesitan los tests
// de este archivo que comparan cuerpos textualmente byte a byte (INV-ID-22)
// en vez de decodificar a un struct conocido.
func cuerpoCrudoHTTP(t *testing.T, app *fiber.App, req *http.Request) (int, []byte) {
	t.Helper()
	resp, err := app.Test(req, 10_000) // 10s, mismo margen que respuestaHTTP (Argon2id, ADR 0008).
	if err != nil {
		t.Fatalf("app.Test(%s %s): %v", req.Method, req.URL.Path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	cuerpo, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("leyendo el cuerpo de la respuesta: %v", err)
	}
	return resp.StatusCode, cuerpo
}
