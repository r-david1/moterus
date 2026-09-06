// Este archivo complementa TestAcceso_LoginConMFA_FlujoCompleto
// (acceso_test.go), que ya cubre el camino feliz completo de OTP/MFA
// (habilitar -> confirmar -> login con step-up -> completar -> sesión con
// amr=["pwd","otp"]) y la mitad de INV-MFA-03 (un token de step-up usado
// como Bearer normal). Los tests de aquí cubren la superficie de seguridad
// que §10 de docs/design/otp-mfa.md exige y que el flujo feliz deja fuera a
// propósito: reuso de un código de respaldo, rate limiting real de
// Confianza sobre verificar_otp, ADR 0039 (deshabilitar exige código
// propio), el fix de re-habilitación del commit 1e06146, un token de
// step-up expirado, la dirección complementaria de INV-MFA-03, y el límite
// de un factor confirmado del ADR 0037.
package integracion

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
)

// --- helpers propios de este archivo ----------------------------------------

// habilitarYConfirmarMFADePrueba ejecuta habilitar + confirmar MFA para el
// usuario autenticado por bearer (con un código TOTP calculado a mano,
// mismo helper totpCodigoDePrueba que TestAcceso_LoginConMFA_FlujoCompleto
// ya usa), y devuelve el ID del factor recién confirmado, su secreto TOTP
// en claro, y los 10 códigos de respaldo en claro (ADR 0040).
func habilitarYConfirmarMFADePrueba(t *testing.T, app *fiber.App, bearer string) (idFactor, secreto string, codigosRespaldo []string) {
	t.Helper()

	var habilitado struct {
		IDFactor            string `json:"id_factor"`
		SecretoEnClaro      string `json:"secreto_en_claro"`
		URIProvisionamiento string `json:"uri_provisionamiento"`
	}
	peticionHabilitar := peticionJSON(t, http.MethodPost, "/identidad/usuarios/actual/factores-mfa", nil)
	peticionHabilitar.Header.Set("Authorization", "Bearer "+bearer)
	status := respuestaHTTP(t, app, peticionHabilitar, &habilitado)
	if status != http.StatusOK {
		t.Fatalf("habilitar MFA: status=%d", status)
	}
	if habilitado.SecretoEnClaro == "" || habilitado.IDFactor == "" {
		t.Fatalf("habilitar MFA: respuesta incompleta: %+v", habilitado)
	}

	codigoConfirmacion := totpCodigoDePrueba(t, habilitado.SecretoEnClaro, time.Now())
	var confirmado struct {
		CodigosRespaldo []string `json:"codigos_respaldo"`
	}
	peticionConfirmar := peticionJSON(t, http.MethodPost, "/identidad/usuarios/actual/factores-mfa/confirmacion", map[string]any{
		"id_factor": habilitado.IDFactor,
		"codigo":    codigoConfirmacion,
	})
	peticionConfirmar.Header.Set("Authorization", "Bearer "+bearer)
	status = respuestaHTTP(t, app, peticionConfirmar, &confirmado)
	if status != http.StatusOK {
		t.Fatalf("confirmar factor MFA: status=%d", status)
	}
	if len(confirmado.CodigosRespaldo) != 10 {
		t.Fatalf("confirmar factor MFA: se esperaban 10 códigos de respaldo, hubo %d", len(confirmado.CodigosRespaldo))
	}
	return habilitado.IDFactor, habilitado.SecretoEnClaro, confirmado.CodigosRespaldo
}

// provocarStepUpDePrueba hace login con contraseña contra una cuenta con
// MFA ya activo y devuelve el token_step_up de la respuesta 401 (§3.5 del
// diseño otp-mfa.md).
func provocarStepUpDePrueba(t *testing.T, app *fiber.App, correo string) string {
	t.Helper()
	var errStepUp errorSegundoFactorRespuesta
	status := respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/acceso/sesiones", map[string]any{
		"correo":     correo,
		"contrasena": contrasenaFuerteDePrueba,
	}), &errStepUp)
	if status != http.StatusUnauthorized {
		t.Fatalf("login con MFA activo: status=%d, se esperaba 401", status)
	}
	tokenStepUp := errStepUp.valor("token_step_up")
	if tokenStepUp == "" {
		t.Fatalf("la respuesta de step-up no trajo token_step_up: %+v", errStepUp)
	}
	return tokenStepUp
}

// hashCodigoRespaldoDePrueba reimplementa (independientemente, mismo
// criterio que totpCodigoDePrueba) HashearCodigoRespaldo
// (identidad/dominio/codigo_respaldo.go): SHA-256 hex del código en claro,
// para poder consultar codigos_respaldo_mfa.hash_codigo directamente desde
// el test.
func hashCodigoRespaldoDePrueba(codigo string) string {
	suma := sha256.Sum256([]byte(codigo))
	return hex.EncodeToString(suma[:])
}

// --- INV-MFA-06: un código de respaldo consumido no se puede reutilizar ----

// TestAcceso_MFA_CodigoRespaldoConsumido_NoReutilizable cubre lo que
// TestAcceso_LoginConMFA_FlujoCompleto deja fuera a propósito (esa prueba
// solo usa el TOTP): completar el segundo factor con un código de
// respaldo, verificar en Postgres que queda marcado usado_en, e intentar
// reutilizarlo en un segundo login con MFA -> debe rechazarse.
func TestAcceso_MFA_CodigoRespaldoConsumido_NoReutilizable(t *testing.T) {
	pool := poolAplicacion(t)
	dueno := poolDueno(t)
	servidor := nuevoServidorAcceso(t, pool)
	app := servidor.App

	idUsuario, correo := usuarioActivoDePrueba(t, app, dueno, "mfa-respaldo")
	t.Cleanup(func() { borrarUsuario(t, dueno, idUsuario) })
	t.Cleanup(func() { limpiarSesionesDeUsuario(t, dueno, idUsuario) })
	t.Cleanup(func() { limpiarFactoresMFADeUsuario(t, dueno, idUsuario) })

	sesionInicial := iniciarSesionDePrueba(t, app, correo)
	idFactor, _, codigosRespaldo := habilitarYConfirmarMFADePrueba(t, app, sesionInicial.TokenAcceso)
	codigoRespaldo := codigosRespaldo[0]

	// 1er login con MFA: completar el segundo factor con un código de
	// respaldo (no el TOTP) en vez de esperar que el TOTP falle primero —
	// FactorMFA.VerificarCodigo ya prueba el TOTP y, si no coincide, los
	// códigos de respaldo (§1.5 del diseño); un código de respaldo de 10
	// caracteres nunca coincide con el formato de un CodigoTOTP de 6
	// dígitos, así que llega directo a la rama de respaldo.
	tokenStepUp1 := provocarStepUpDePrueba(t, app, correo)
	var sesion1 resultadoSesionDePrueba
	status := respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/acceso/sesiones/segundo-factor", map[string]any{
		"token_step_up": tokenStepUp1,
		"codigo":        codigoRespaldo,
	}), &sesion1)
	if status != http.StatusCreated {
		t.Fatalf("completar segundo factor con un código de respaldo: status=%d", status)
	}
	if sesion1.IDUsuario != idUsuario {
		t.Fatalf("completar segundo factor: id_usuario = %q, se esperaba %q", sesion1.IDUsuario, idUsuario)
	}

	// Verificación en BD: el código quedó consumido (usado_en poblado).
	hashHex := hashCodigoRespaldoDePrueba(codigoRespaldo)
	var usadoEn *time.Time
	if err := dueno.QueryRow(context.Background(),
		`SELECT usado_en FROM codigos_respaldo_mfa WHERE factor_id = $1 AND hash_codigo = $2`, idFactor, hashHex,
	).Scan(&usadoEn); err != nil {
		t.Fatalf("consultando codigos_respaldo_mfa: %v", err)
	}
	if usadoEn == nil {
		t.Fatalf("codigos_respaldo_mfa.usado_en no quedó poblado tras el primer uso del código de respaldo (INV-MFA-06)")
	}

	// 2do login con MFA: reutilizar el MISMO código de respaldo ya
	// consumido -> debe rechazarse (INV-MFA-06, mismo error genérico que
	// cualquier otro motivo de rechazo, INV-MFA-08).
	tokenStepUp2 := provocarStepUpDePrueba(t, app, correo)
	var errResp errorHumaRespuesta
	status = respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/acceso/sesiones/segundo-factor", map[string]any{
		"token_step_up": tokenStepUp2,
		"codigo":        codigoRespaldo,
	}), &errResp)
	if status != http.StatusUnauthorized {
		t.Fatalf("reutilizar un código de respaldo ya consumido: status=%d, se esperaba 401 (INV-MFA-06)", status)
	}

	assertCadenaAuditoriaIntegra(t, dueno)
}

// --- rate limiting real de Confianza sobre verificar_otp --------------------

// TestAcceso_MFA_VerificarOTP_RateLimitDeConfianzaBloqueaTrasElUmbral
// ejercita, contra Redis real, el guardián de perímetro de
// POST /acceso/sesiones/segundo-factor (accion "verificar_otp", umbral de
// cuenta 5/15min — confianza/dominio/umbral.go, §7 del diseño otp-mfa.md).
// Mismo criterio que TestHTTP_ReenviarVerificacion_GuardianDePerimetroReal_BloqueaTrasElUmbral
// (verificacion_correo_test.go): usa el harness dedicado con
// EvaluadorConfianzaReal en vez del no-op de nuevoServidorAcceso, y
// reinicia el cupo de Redis antes/después para no depender del estado de
// corridas anteriores.
func TestAcceso_MFA_VerificarOTP_RateLimitDeConfianzaBloqueaTrasElUmbral(t *testing.T) {
	pool := poolAplicacion(t)
	dueno := poolDueno(t)
	limitador := clienteRedis(t) // se salta limpiamente sin REDIS_URL.
	servidor := nuevoServidorAccesoConConfianzaReal(t, pool)
	app := servidor.App

	idUsuario, correo := usuarioActivoDePrueba(t, app, dueno, "mfa-rl")
	t.Cleanup(func() { borrarUsuario(t, dueno, idUsuario) })
	t.Cleanup(func() { limpiarSesionesDeUsuario(t, dueno, idUsuario) })
	t.Cleanup(func() { limpiarFactoresMFADeUsuario(t, dueno, idUsuario) })

	sesionInicial := iniciarSesionDePrueba(t, app, correo)
	habilitarYConfirmarMFADePrueba(t, app, sesionInicial.TokenAcceso)

	// Claves documentadas en confianza/aplicacion/evaluar_trust_signal.go
	// (claveLimite) y acceso/aplicacion/soporte.go (claveCuentaPorUsuario):
	// "confianza:rl:cuenta:verificar_otp:usuario:<id>". La IP resuelve
	// siempre a "0.0.0.0" en este paquete (ver el comentario de
	// TestHTTP_ReenviarVerificacion_GuardianDePerimetroReal_BloqueaTrasElUmbral).
	claveCuenta := fmt.Sprintf("confianza:rl:cuenta:verificar_otp:usuario:%s", idUsuario)
	claveIP := "confianza:rl:ip:verificar_otp:0.0.0.0"
	reiniciarLimites := func() {
		_ = limitador.Reiniciar(context.Background(), claveCuenta)
		_ = limitador.Reiniciar(context.Background(), claveIP)
	}
	reiniciarLimites()
	t.Cleanup(reiniciarLimites)

	tokenStepUp := provocarStepUpDePrueba(t, app, correo)

	// CompletarSegundoFactorCasoDeUso evalúa Confianza ANTES de verificar el
	// código (§3.6 paso 2 del diseño), así que un código deliberadamente
	// incorrecto sigue consumiendo el cupo de la cuenta igual que uno
	// correcto lo haría — 5 intentos dentro del umbral, todos rechazados
	// por el código (401), ninguno todavía por Confianza.
	for i := 1; i <= 5; i++ {
		var errResp errorHumaRespuesta
		status := respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/acceso/sesiones/segundo-factor", map[string]any{
			"token_step_up": tokenStepUp,
			"codigo":        "000000",
		}), &errResp)
		if status != http.StatusUnauthorized {
			t.Fatalf("intento #%d (dentro del umbral de cuenta): status=%d, se esperaba 401", i, status)
		}
	}

	// 6to intento: excede el umbral de cuenta (5/15min) -> 429, sin importar
	// que el código siga siendo incorrecto (el guardián de perímetro actúa
	// antes de que el código se evalúe).
	var errBloqueado errorHumaRespuesta
	status := respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/acceso/sesiones/segundo-factor", map[string]any{
		"token_step_up": tokenStepUp,
		"codigo":        "000000",
	}), &errBloqueado)
	if status != http.StatusTooManyRequests {
		t.Fatalf("intento #6 (debe exceder el umbral de cuenta, 5/15min): status=%d, se esperaba 429", status)
	}

	assertCadenaAuditoriaIntegra(t, dueno)
}

// --- ADR 0039: deshabilitar MFA exige un código propio ----------------------

// TestIdentidad_MFA_Deshabilitar_ExigeCodigoPropio cubre INV-MFA-05: ni
// omitir el código ni presentar uno incorrecto basta para deshabilitar
// MFA con un Bearer válido — el factor sigue confirmado y activo hasta que
// se presenta un código correcto.
func TestIdentidad_MFA_Deshabilitar_ExigeCodigoPropio(t *testing.T) {
	pool := poolAplicacion(t)
	dueno := poolDueno(t)
	servidor := nuevoServidorAcceso(t, pool)
	app := servidor.App

	idUsuario, correo := usuarioActivoDePrueba(t, app, dueno, "mfa-deshabilitar")
	t.Cleanup(func() { borrarUsuario(t, dueno, idUsuario) })
	t.Cleanup(func() { limpiarSesionesDeUsuario(t, dueno, idUsuario) })
	t.Cleanup(func() { limpiarFactoresMFADeUsuario(t, dueno, idUsuario) })

	sesionInicial := iniciarSesionDePrueba(t, app, correo)
	idFactor, secreto, _ := habilitarYConfirmarMFADePrueba(t, app, sesionInicial.TokenAcceso)

	assertFactorConfirmadoActivo := func(esperadoActivo bool) {
		t.Helper()
		var confirmado, activo bool
		if err := dueno.QueryRow(context.Background(),
			`SELECT confirmado, activo FROM factores_mfa WHERE id = $1`, idFactor,
		).Scan(&confirmado, &activo); err != nil {
			t.Fatalf("consultando factores_mfa: %v", err)
		}
		if !confirmado {
			t.Fatalf("factores_mfa.confirmado = false, se esperaba true (hecho histórico, nunca vuelve a false)")
		}
		if activo != esperadoActivo {
			t.Fatalf("factores_mfa.activo = %v, se esperaba %v", activo, esperadoActivo)
		}
	}

	// Sin código (cadena vacía, viola minLength:6 del esquema): rechazado,
	// MFA sigue activo.
	peticionSinCodigo := peticionJSON(t, http.MethodDelete, "/identidad/usuarios/actual/factores-mfa", map[string]any{
		"codigo": "",
	})
	peticionSinCodigo.Header.Set("Authorization", "Bearer "+sesionInicial.TokenAcceso)
	status := respuestaHTTP(t, app, peticionSinCodigo, nil)
	if status >= 200 && status < 300 {
		t.Fatalf("deshabilitar sin código: status=%d, se esperaba un error (ADR 0039)", status)
	}
	assertFactorConfirmadoActivo(true)

	// Código con formato válido pero incorrecto: rechazado (422,
	// ErrCodigoOTPInvalido, INV-MFA-08), MFA sigue activo.
	peticionCodigoIncorrecto := peticionJSON(t, http.MethodDelete, "/identidad/usuarios/actual/factores-mfa", map[string]any{
		"codigo": "000000",
	})
	peticionCodigoIncorrecto.Header.Set("Authorization", "Bearer "+sesionInicial.TokenAcceso)
	var errResp errorHumaRespuesta
	status = respuestaHTTP(t, app, peticionCodigoIncorrecto, &errResp)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("deshabilitar con código incorrecto: status=%d, se esperaba 422 (INV-MFA-08)", status)
	}
	assertFactorConfirmadoActivo(true)

	// Código correcto: éxito, MFA queda deshabilitado (confirmado se queda
	// en true para siempre, activo pasa a false).
	codigoCorrecto := totpCodigoDePrueba(t, secreto, time.Now())
	peticionCorrecta := peticionJSON(t, http.MethodDelete, "/identidad/usuarios/actual/factores-mfa", map[string]any{
		"codigo": codigoCorrecto,
	})
	peticionCorrecta.Header.Set("Authorization", "Bearer "+sesionInicial.TokenAcceso)
	status = respuestaHTTP(t, app, peticionCorrecta, nil)
	if status != http.StatusNoContent {
		t.Fatalf("deshabilitar con código correcto: status=%d, se esperaba 204", status)
	}
	assertFactorConfirmadoActivo(false)

	assertCadenaAuditoriaIntegra(t, dueno)
}

// --- el fix del commit 1e06146: re-habilitar tras deshabilitar -------------

// TestIdentidad_MFA_RehabilitarTrasDeshabilitar_NoFallaConLimiteExcedido
// formaliza como test automatizado el bug real que el commit 1e06146
// corrigió (distinguir EstaConfirmado(), hecho histórico que nunca vuelve a
// false, de EstaActivo(), que sí puede volver a false y a true de nuevo):
// sin esa distinción — tanto en el dominio (factor_mfa.go) como en el
// índice único parcial de la migración 000015 — un factor recién
// deshabilitado seguiría contando como "confirmado" para siempre y
// cualquier HabilitarMFA posterior del mismo usuario fallaría con
// ErrLimiteFactoresMFAExcedido. Es el test más importante de este archivo:
// nadie debería poder reintroducir ese bug sin que este test lo detecte.
func TestIdentidad_MFA_RehabilitarTrasDeshabilitar_NoFallaConLimiteExcedido(t *testing.T) {
	pool := poolAplicacion(t)
	dueno := poolDueno(t)
	servidor := nuevoServidorAcceso(t, pool)
	app := servidor.App

	idUsuario, correo := usuarioActivoDePrueba(t, app, dueno, "mfa-rehabilitar")
	t.Cleanup(func() { borrarUsuario(t, dueno, idUsuario) })
	t.Cleanup(func() { limpiarSesionesDeUsuario(t, dueno, idUsuario) })
	t.Cleanup(func() { limpiarFactoresMFADeUsuario(t, dueno, idUsuario) })

	sesionInicial := iniciarSesionDePrueba(t, app, correo)
	idFactorInicial, secretoInicial, _ := habilitarYConfirmarMFADePrueba(t, app, sesionInicial.TokenAcceso)

	// Deshabilitar el factor confirmado con su propio código.
	codigoCorrecto := totpCodigoDePrueba(t, secretoInicial, time.Now())
	peticionDeshabilitar := peticionJSON(t, http.MethodDelete, "/identidad/usuarios/actual/factores-mfa", map[string]any{
		"codigo": codigoCorrecto,
	})
	peticionDeshabilitar.Header.Set("Authorization", "Bearer "+sesionInicial.TokenAcceso)
	status := respuestaHTTP(t, app, peticionDeshabilitar, nil)
	if status != http.StatusNoContent {
		t.Fatalf("deshabilitar MFA: status=%d, se esperaba 204", status)
	}

	// Volver a habilitar MFA para el mismo usuario: NO debe fallar con
	// ErrLimiteFactoresMFAExcedido (409) pese a que el factor anterior sigue
	// confirmado=true en la base para siempre.
	var habilitadoDeNuevo struct {
		IDFactor            string `json:"id_factor"`
		SecretoEnClaro      string `json:"secreto_en_claro"`
		URIProvisionamiento string `json:"uri_provisionamiento"`
	}
	peticionHabilitar := peticionJSON(t, http.MethodPost, "/identidad/usuarios/actual/factores-mfa", nil)
	peticionHabilitar.Header.Set("Authorization", "Bearer "+sesionInicial.TokenAcceso)
	status = respuestaHTTP(t, app, peticionHabilitar, &habilitadoDeNuevo)
	if status != http.StatusOK {
		t.Fatalf("re-habilitar MFA tras deshabilitar: status=%d, se esperaba 200 (NO ErrLimiteFactoresMFAExcedido/409 — commit 1e06146)", status)
	}
	if habilitadoDeNuevo.IDFactor == "" || habilitadoDeNuevo.IDFactor == idFactorInicial {
		t.Fatalf("re-habilitar MFA: se esperaba un id_factor nuevo y no vacío, obtuvo %q (inicial %q)", habilitadoDeNuevo.IDFactor, idFactorInicial)
	}

	// Cierra el ciclo completo confirmando el factor nuevo: habilitar ->
	// deshabilitar -> habilitar -> confirmar, sin ningún error de límite en
	// el camino.
	codigoConfirmacion := totpCodigoDePrueba(t, habilitadoDeNuevo.SecretoEnClaro, time.Now())
	var confirmado struct {
		CodigosRespaldo []string `json:"codigos_respaldo"`
	}
	peticionConfirmar := peticionJSON(t, http.MethodPost, "/identidad/usuarios/actual/factores-mfa/confirmacion", map[string]any{
		"id_factor": habilitadoDeNuevo.IDFactor,
		"codigo":    codigoConfirmacion,
	})
	peticionConfirmar.Header.Set("Authorization", "Bearer "+sesionInicial.TokenAcceso)
	status = respuestaHTTP(t, app, peticionConfirmar, &confirmado)
	if status != http.StatusOK {
		t.Fatalf("confirmar el factor re-habilitado: status=%d", status)
	}
	if len(confirmado.CodigosRespaldo) != 10 {
		t.Fatalf("confirmar el factor re-habilitado: se esperaban 10 códigos de respaldo, hubo %d", len(confirmado.CodigosRespaldo))
	}

	assertCadenaAuditoriaIntegra(t, dueno)
}

// --- INV-MFA-04: un token de step-up expirado se rechaza --------------------

// TestAcceso_MFA_TokenStepUpExpirado_Rechazado emite un token de step-up
// directamente con el mismo EmisorTokenStepUp/Llavero que el servidor de
// prueba ya usa internamente, con `ahora` ya en el pasado — así se ejercita
// la expiración real (5 minutos, ADR 0038) sin tener que esperarla de
// verdad en el test.
func TestAcceso_MFA_TokenStepUpExpirado_Rechazado(t *testing.T) {
	pool := poolAplicacion(t)
	dueno := poolDueno(t)
	servidor := nuevoServidorAcceso(t, pool)
	app := servidor.App

	idUsuario, correo := usuarioActivoDePrueba(t, app, dueno, "mfa-stepup-exp")
	t.Cleanup(func() { borrarUsuario(t, dueno, idUsuario) })
	t.Cleanup(func() { limpiarSesionesDeUsuario(t, dueno, idUsuario) })
	t.Cleanup(func() { limpiarFactoresMFADeUsuario(t, dueno, idUsuario) })

	sesionInicial := iniciarSesionDePrueba(t, app, correo)
	_, secreto, _ := habilitarYConfirmarMFADePrueba(t, app, sesionInicial.TokenAcceso)

	// ahora=hace 10 minutos => exp=hace 5 minutos, muy por fuera de la
	// tolerancia de reloj de 60s (PoliticaSesionPorDefecto).
	ahoraPasado := time.Now().Add(-10 * time.Minute)
	tokenExpirado, err := servidor.EmisorStepUp.Emitir(context.Background(), idUsuario, "mfa_habilitado", ahoraPasado)
	if err != nil {
		t.Fatalf("emitiendo un token de step-up ya expirado: %v", err)
	}

	// Código TOTP correcto y vigente: el rechazo debe venir del token
	// expirado, no del código (para que el test no se confunda con
	// TestIdentidad_MFA_Deshabilitar_ExigeCodigoPropio).
	codigoCorrecto := totpCodigoDePrueba(t, secreto, time.Now())
	var errResp errorHumaRespuesta
	status := respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/acceso/sesiones/segundo-factor", map[string]any{
		"token_step_up": tokenExpirado.Compacto(),
		"codigo":        codigoCorrecto,
	}), &errResp)
	if status != http.StatusUnauthorized {
		t.Fatalf("completar segundo factor con un token de step-up expirado: status=%d, se esperaba 401 (INV-MFA-04)", status)
	}

	assertCadenaAuditoriaIntegra(t, dueno)
}

// --- INV-MFA-03, dirección complementaria -----------------------------------

// TestAcceso_MFA_TokenDeAccesoNormalComoStepUp_Rechazado cubre la mitad de
// INV-MFA-03 que TestAcceso_LoginConMFA_FlujoCompleto (paso 7) deja fuera:
// esa prueba ya confirma que un token de step-up nunca sirve como Bearer
// normal; esta confirma la dirección inversa — un token de acceso normal
// (typ="at+jwt"), perfectamente válido como tal, nunca debe aceptarse donde
// se espera un token de step-up (typ="step-up+jwt"). No hace falta que la
// cuenta tenga MFA habilitado: el rechazo ocurre en la validación del
// propio token, antes de consultar nada de Identidad.
func TestAcceso_MFA_TokenDeAccesoNormalComoStepUp_Rechazado(t *testing.T) {
	pool := poolAplicacion(t)
	dueno := poolDueno(t)
	servidor := nuevoServidorAcceso(t, pool)
	app := servidor.App

	idUsuario, correo := usuarioActivoDePrueba(t, app, dueno, "mfa-typ-inverso")
	t.Cleanup(func() { borrarUsuario(t, dueno, idUsuario) })
	t.Cleanup(func() { limpiarSesionesDeUsuario(t, dueno, idUsuario) })

	sesionInicial := iniciarSesionDePrueba(t, app, correo)

	var errResp errorHumaRespuesta
	status := respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/acceso/sesiones/segundo-factor", map[string]any{
		"token_step_up": sesionInicial.TokenAcceso,
		"codigo":        "000000",
	}), &errResp)
	if status != http.StatusUnauthorized {
		t.Fatalf("usar un token de acceso normal como token_step_up: status=%d, se esperaba 401 (INV-MFA-03)", status)
	}

	assertCadenaAuditoriaIntegra(t, dueno)
}

// --- ADR 0037: como máximo un factor confirmado y activo a la vez ----------

// TestIdentidad_MFA_Habilitar_LimiteDeUnFactorConfirmado cubre el lado
// "no más de uno simultáneo" de INV-MFA-01/ADR 0037: con un factor ya
// confirmado y activo, un HabilitarMFA adicional se rechaza con
// ErrLimiteFactoresMFAExcedido — a diferencia de
// TestIdentidad_MFA_RehabilitarTrasDeshabilitar_NoFallaConLimiteExcedido,
// aquí el factor original nunca se deshabilita.
func TestIdentidad_MFA_Habilitar_LimiteDeUnFactorConfirmado(t *testing.T) {
	pool := poolAplicacion(t)
	dueno := poolDueno(t)
	servidor := nuevoServidorAcceso(t, pool)
	app := servidor.App

	idUsuario, correo := usuarioActivoDePrueba(t, app, dueno, "mfa-limite")
	t.Cleanup(func() { borrarUsuario(t, dueno, idUsuario) })
	t.Cleanup(func() { limpiarSesionesDeUsuario(t, dueno, idUsuario) })
	t.Cleanup(func() { limpiarFactoresMFADeUsuario(t, dueno, idUsuario) })

	sesionInicial := iniciarSesionDePrueba(t, app, correo)
	habilitarYConfirmarMFADePrueba(t, app, sesionInicial.TokenAcceso)

	peticionHabilitar := peticionJSON(t, http.MethodPost, "/identidad/usuarios/actual/factores-mfa", nil)
	peticionHabilitar.Header.Set("Authorization", "Bearer "+sesionInicial.TokenAcceso)
	var errResp errorHumaRespuesta
	status := respuestaHTTP(t, app, peticionHabilitar, &errResp)
	if status != http.StatusConflict {
		t.Fatalf("habilitar un segundo factor con uno ya confirmado y activo: status=%d, se esperaba 409 (ADR 0037)", status)
	}

	assertCadenaAuditoriaIntegra(t, dueno)
}
