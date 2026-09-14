// Package configuracion traduce variables de entorno a un struct
// tipado, validado al arrancar el proceso. No contiene reglas de
// negocio: es kernel técnico compartido por todos los bounded contexts.
package configuracion

import (
	"fmt"
	"os"
	"strconv"
)

// Config agrupa la configuración mínima necesaria para arrancar el
// servidor HTTP. Se irá extendiendo (BD, Redis, JWT, telemetría) a
// medida que los demás adaptadores se implementen.
type Config struct {
	// Puerto en el que escucha el servidor Fiber.
	Puerto int
	// EntornoApp identifica el entorno de ejecución (dev|staging|prod).
	EntornoApp string
	// URLBaseDeDatos es la cadena de conexión a PostgreSQL (DSN pgx) usada
	// por herramientas administrativas (el migrador): requiere privilegios
	// de DDL, por lo que normalmente apunta al rol dueño de la base.
	URLBaseDeDatos string
	// URLBaseDeDatosAplicacion es el DSN que usa el proceso api en tiempo
	// de ejecución. Debe apuntar a un rol de login con privilegios
	// acotados (rol_login_identidad, ver migración 000003) — nunca al rol
	// dueño/superusuario de URLBaseDeDatos, o el REVOKE UPDATE/DELETE de
	// ADR 0005 queda sin efecto (un superusuario ignora los permisos de
	// tabla). Si no se define, se usa URLBaseDeDatos con un WARN explícito.
	URLBaseDeDatosAplicacion string
	// URLRedis es la cadena de conexión a Redis (env REDIS_URL) que usa el
	// contexto Confianza para el limitador de tasa (ADR 0018). Si está
	// vacía, Identidad monta EvaluadorConfianzaNoOp (sin rate limiting ni
	// captcha real) con el WARN de arranque ya existente — no es un error
	// de configuración en desarrollo, pero nunca debe pasar así a
	// producción. Ver docker-compose.yml: el servicio "redis" ya está
	// provisionado por defecto en desarrollo local.
	URLRedis string
	// TurnstileSecretKey es la llave secreta server-side de Cloudflare
	// Turnstile (env TURNSTILE_SECRET_KEY, ADR 0003/0018) usada para
	// verificar el token de captcha contra la API de Cloudflare. Si está
	// vacía, el adaptador turnstile.VerificadorCaptcha decide fail-open
	// (WARN, permite con puntaje 1.0) o fail-closed según EntornoApp — ver
	// internal/confianza/adaptadores/turnstile.
	TurnstileSecretKey string
	// TurnstileVerifyURL permite sobreescribir el endpoint de verificación
	// de Cloudflare (env TURNSTILE_VERIFY_URL) — pensado para tests con un
	// httptest.Server local, nunca hace falta en producción.
	TurnstileVerifyURL string
	// AccesoEmisor es el claim `iss` de los tokens de acceso (env
	// ACCESO_EMISOR, ADR 0020). Sin default aplicado aquí a propósito: la
	// decisión de fallar el arranque en producción o usar un valor de
	// desarrollo vive en cmd/api/main.go (esta paquetería no tiene reglas
	// de negocio ni de arranque).
	AccesoEmisor string
	// AccesoAudiencia es el claim `aud` fijo de los tokens de acceso (env
	// ACCESO_AUDIENCIA, ADR 0002: un solo producto ⇒ audiencia fija).
	AccesoAudiencia string
	// AccesoLlaveFirma es la llave privada activa de firma de tokens de
	// acceso (env ACCESO_LLAVE_FIRMA): PEM (PKCS8) o seed/llave Ed25519 en
	// base64 (ADR 0020 §4). Sin ella en APP_ENV=production el proceso api
	// no arranca; fuera de producción se genera una llave efímera con WARN.
	AccesoLlaveFirma string
	// AccesoLlavesVerificacionPrevias es la lista separada por comas de
	// llaves de verificación previas (env
	// ACCESO_LLAVES_VERIFICACION_PREVIAS, ADR 0020 §4), cada una en el
	// mismo formato que AccesoLlaveFirma. Se mantienen publicadas en el
	// JWKS durante una ventana de rotación (≥ vidaTokenAcceso).
	AccesoLlavesVerificacionPrevias string
	// IdentidadLlaveCifradoMFA es la llave simétrica AES-256-GCM (env
	// IDENTIDAD_LLAVE_CIFRADO_MFA, docs/design/otp-mfa.md §2.2/ADR 0038)
	// que identidad/adaptadores/cripto.CifradorSecretosAESGCM usa para
	// cifrar/descifrar en reposo el secreto TOTP de cada FactorMFA: 32
	// bytes en base64. Mismo criterio de gestión que ACCESO_LLAVE_FIRMA:
	// sin ella en APP_ENV=production el proceso no arranca; fuera de
	// producción se genera una llave efímera en memoria con WARN explícito
	// (los secretos MFA cifrados con ella quedan indescifrables al
	// reiniciar el proceso).
	IdentidadLlaveCifradoMFA string
	// ResendAPIKey es la llave de API de Resend (env RESEND_API_KEY, ADR
	// 0054) usada por plataforma/correo.ClienteResend para enviar correo
	// transaccional real (verificación de cuenta en Identidad, invitaciones
	// en Tenencia). Si está vacía, ambos contextos caen a su adaptador
	// log-only (NotificadorCorreoLog/NotificadorInvitacionesLog) fuera de
	// producción, con el mismo WARN explícito que el resto del proyecto; en
	// producción el proceso no arranca sin ella (mismo criterio que
	// TURNSTILE_SECRET_KEY, ADR 0053).
	ResendAPIKey string
	// ResendRemitente es la dirección "From" con la que se envían los
	// correos (env RESEND_REMITENTE), formato "Nombre <correo@dominio>".
	// Debe ser un remitente de un dominio verificado en Resend — una
	// dirección arbitraria no funciona. Mismo criterio de arranque que
	// ResendAPIKey.
	ResendRemitente string
	// URLFrontend es la URL base de un futuro cliente/frontend (env
	// URL_FRONTEND) que este servicio no incluye (ADR 0002: el producto es
	// el servicio de auth, no un frontend). Si está definida, los correos
	// de verificación/invitación incluyen un enlace
	// "{URLFrontend}/<ruta>?token=<token>"; si no, el correo presenta el
	// token en claro para uso directo contra la API. Opcional: ningún
	// adaptador falla al arrancar sin ella.
	URLFrontend string
}

// CargarDesdeEntorno construye un Config leyendo variables de entorno,
// con valores por defecto razonables para desarrollo local.
func CargarDesdeEntorno() (Config, error) {
	puertoCrudo := valorODefecto("PORT", "8080")
	puerto, err := strconv.Atoi(puertoCrudo)
	if err != nil {
		return Config{}, fmt.Errorf("configuracion: PORT invalido %q: %w", puertoCrudo, err)
	}

	return Config{
		Puerto:                          puerto,
		EntornoApp:                      valorODefecto("APP_ENV", "development"),
		URLBaseDeDatos:                  os.Getenv("DATABASE_URL"),
		URLBaseDeDatosAplicacion:        os.Getenv("DATABASE_URL_APLICACION"),
		URLRedis:                        os.Getenv("REDIS_URL"),
		TurnstileSecretKey:              os.Getenv("TURNSTILE_SECRET_KEY"),
		TurnstileVerifyURL:              valorODefecto("TURNSTILE_VERIFY_URL", ""),
		AccesoEmisor:                    os.Getenv("ACCESO_EMISOR"),
		AccesoAudiencia:                 os.Getenv("ACCESO_AUDIENCIA"),
		AccesoLlaveFirma:                os.Getenv("ACCESO_LLAVE_FIRMA"),
		AccesoLlavesVerificacionPrevias: os.Getenv("ACCESO_LLAVES_VERIFICACION_PREVIAS"),
		IdentidadLlaveCifradoMFA:        os.Getenv("IDENTIDAD_LLAVE_CIFRADO_MFA"),
		ResendAPIKey:                    os.Getenv("RESEND_API_KEY"),
		ResendRemitente:                 os.Getenv("RESEND_REMITENTE"),
		URLFrontend:                     os.Getenv("URL_FRONTEND"),
	}, nil
}

func valorODefecto(clave, defecto string) string {
	if v := os.Getenv(clave); v != "" {
		return v
	}
	return defecto
}
