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
		Puerto:                   puerto,
		EntornoApp:               valorODefecto("APP_ENV", "development"),
		URLBaseDeDatos:           os.Getenv("DATABASE_URL"),
		URLBaseDeDatosAplicacion: os.Getenv("DATABASE_URL_APLICACION"),
		URLRedis:                 os.Getenv("REDIS_URL"),
		TurnstileSecretKey:       os.Getenv("TURNSTILE_SECRET_KEY"),
		TurnstileVerifyURL:       valorODefecto("TURNSTILE_VERIFY_URL", ""),
	}, nil
}

func valorODefecto(clave, defecto string) string {
	if v := os.Getenv(clave); v != "" {
		return v
	}
	return defecto
}
