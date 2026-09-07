package http

import (
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humafiber"
	"github.com/gofiber/fiber/v2"

	"github.com/r-david1/moterus/internal/acceso/puertos"
	confianzahttp "github.com/r-david1/moterus/internal/confianza/adaptadores/http"
	confianzadominio "github.com/r-david1/moterus/internal/confianza/dominio"
	confianzapuertos "github.com/r-david1/moterus/internal/confianza/puertos"
)

// prefijo es el prefijo de ruta de los endpoints de Acceso (§7 del
// diseño). /.well-known/jwks.json es la única excepción: RFC 8615 fija ese
// prefijo en la raíz, no bajo /acceso.
const prefijo = "/acceso"

// RegistrarRutas monta el middleware de OrigenSolicitud y las siete
// operaciones del MVP de Acceso (sección 7 del diseño) sobre app, con Huma
// v2 (ADR 0006). validador es el puertos.ValidadorDeAccesos que
// MiddlewareAutenticacion consume para las rutas protegidas — el mismo
// puerto que el resto del sistema usa para autenticar cualquier endpoint
// no público (§2.4 del diseño).
//
// portero es el confianza/puertos.PorteroDeSala que confianzahttp.
// MiddlewareSalaDeEspera consume para proteger POST /acceso/sesiones (§12
// de docs/design/colas-virtuales.md: acceso.iniciar_sesion, alcance
// sistema). Va PRIMERO en la cadena de esa operación (INV-COLA-09: antes de
// cualquier trabajo caro — Argon2id, Postgres, la evaluación de Confianza
// que ya hace Identidad dentro de AutenticarUsuario). cmd/api/main.go pasa
// el adaptador real (Redis) o confianza/adaptadores/porteronoop si
// REDIS_URL no está configurada. Puede ir nil ÚNICAMENTE para no romper los
// tests de integración existentes de este paquete (mismo criterio que
// identidad/adaptadores/http.RegistrarRutas frente a validador nil): con
// nil, el endpoint se registra sin el middleware, igual que antes de esta
// extensión.
func RegistrarRutas(app *fiber.App, m *ManejadorAcceso, validador puertos.ValidadorDeAccesos, portero confianzapuertos.PorteroDeSala) huma.API {
	app.Use(middlewareOrigenSolicitud)

	// Rutas de metadatos (OpenAPI/docs/schemas) con prefijo propio: Acceso e
	// Identidad montan cada uno su propia instancia de huma.API sobre el
	// mismo *fiber.App (ver cmd/api/main.go), y humafiber.NewV2 registra
	// /openapi.json, /openapi.yaml, /docs y /schemas/* con
	// huma.DefaultConfig sin prefijo. Sin este override, las dos
	// instancias compiten por las mismas rutas exactas y Fiber sirve solo
	// la primera registrada (Acceso, en main.go) — la spec/docs de
	// Identidad quedaban silenciosamente inalcanzables. Corregido dándole
	// a cada contexto su propio namespace de metadatos.
	cfgAcceso := huma.DefaultConfig("Acceso", "0.1.0")
	cfgAcceso.OpenAPIPath = prefijo + "/openapi"
	cfgAcceso.DocsPath = prefijo + "/docs"
	cfgAcceso.SchemasPath = prefijo + "/schemas"
	api := humafiber.NewV2(app, cfgAcceso)

	metaPublico := map[string]any{
		"x-auth-nivel": "publico-sin-token",
		"x-sala-espera": "ruta protegible acceso.iniciar_sesion, alcance sistema " +
			"(docs/design/colas-virtuales.md §1.6/§12): MiddlewareSalaDeEspera montado primero en la cadena (INV-COLA-09).",
	}
	metaPublicoRefresco := map[string]any{
		"x-auth-nivel": "publico-token-refresco-en-cuerpo",
		"x-rate-limit": "ADR 0018/0019: EvaluadorConfianza, accion=renovacion_sesion (30/min por IP, 10/min por sesión).",
	}
	metaBearer := map[string]any{
		"x-auth-nivel": "bearer-acceso",
	}
	metaBearerAltoValor := map[string]any{
		"x-auth-nivel": "bearer-acceso-sesion-viva",
		"x-rate-limit": "ADR 0018/0019: EvaluadorConfianza, accion=cierre_masivo_sesiones (5/min por IP, 3/15min por usuario).",
	}

	opIniciarSesion := huma.Operation{
		OperationID: "acceso-iniciar-sesion",
		Method:      http.MethodPost,
		Path:        prefijo + "/sesiones",
		Summary:     "Iniciar sesión (login)",
		Description: "Orquesta el login completo (ADR 0009): delega la verificación de credenciales en Identidad " +
			"y, si es exitosa, emite un token de acceso (JWT, 10 min) y un token de refresco opaco rotatorio. " +
			"No evalúa Confianza aquí: ya lo hace Identidad dentro de AutenticarUsuario.",
		DefaultStatus: http.StatusCreated,
		Tags:          []string{"Acceso"},
		Metadata:      metaPublico,
	}
	if portero != nil {
		opIniciarSesion.Middlewares = huma.Middlewares{confianzahttp.MiddlewareSalaDeEspera(api, portero, confianzadominio.RutaAccesoIniciarSesion)}
	} else {
		slog.Warn("acceso/adaptadores/http: RegistrarRutas se llamó sin portero de Confianza — " +
			"POST /acceso/sesiones queda sin sala de espera. NO USAR EN PRODUCCIÓN.")
	}
	huma.Register(api, opIniciarSesion, m.IniciarSesion)

	huma.Register(api, huma.Operation{
		OperationID: "acceso-completar-segundo-factor",
		Method:      http.MethodPost,
		Path:        prefijo + "/sesiones/segundo-factor",
		Summary:     "Completar el login con el segundo factor (MFA)",
		Description: "Paso 2 del login cuando POST /acceso/sesiones devolvió token_step_up (§3.6 del diseño " +
			"otp-mfa.md). Sin Bearer: la credencial de este endpoint es el propio token de step-up del cuerpo " +
			"(TTL de 5 minutos, INV-MFA-03/04) — nunca un token de acceso normal, que el validador de step-up " +
			"rechaza explícitamente. Verifica el código (TOTP o de respaldo) contra Identidad y, si es válido, " +
			"emite la sesión completa con amr=[\"pwd\",\"otp\"]. 401 único para token inválido/expirado, " +
			"Confianza denegado o código incorrecto (INV-MFA-08: no se distingue el motivo).",
		DefaultStatus: http.StatusCreated,
		Tags:          []string{"Acceso", "MFA"},
		Metadata: map[string]any{
			"x-auth-nivel": "publico-token-step-up-en-cuerpo",
			"x-rate-limit": "ADR 0018/docs/design/otp-mfa.md §7: EvaluadorConfianza, accion=verificar_otp (5/15min por usuario, 20/15min por IP) — oráculo de fuerza bruta sobre un código de 6 dígitos.",
		},
	}, m.CompletarSegundoFactor)

	huma.Register(api, huma.Operation{
		OperationID: "acceso-renovar-sesion",
		Method:      http.MethodPost,
		Path:        prefijo + "/sesiones/renovaciones",
		Summary:     "Renovar la sesión (rotar el token de refresco)",
		Description: "Consume el token de refresco presentado y emite uno nuevo (rotación obligatoria, INV-ACC-05). " +
			"Presentar un token ya consumido revoca toda la sesión (INV-ACC-06). Refresco desconocido/expirado/" +
			"consumido/malformado producen la misma respuesta observable (INV-ACC-21).",
		Tags:     []string{"Acceso"},
		Metadata: metaPublicoRefresco,
	}, m.RenovarSesion)

	huma.Register(api, huma.Operation{
		OperationID: "acceso-cerrar-sesion-actual",
		Method:      http.MethodDelete,
		Path:        prefijo + "/sesiones/actual",
		Summary:     "Cerrar la sesión actual (logout individual)",
		Description: "Revoca la sesión del propio token Bearer. Idempotente: cerrar una sesión ya revocada " +
			"devuelve el mismo 204 sin auditar de nuevo.",
		DefaultStatus: http.StatusNoContent,
		Tags:          []string{"Acceso"},
		Metadata:      metaBearer,
		Middlewares:   huma.Middlewares{MiddlewareAutenticacion(api, validador, false)},
	}, m.CerrarSesionActual)

	huma.Register(api, huma.Operation{
		OperationID: "acceso-cerrar-sesion",
		Method:      http.MethodDelete,
		Path:        prefijo + "/sesiones/{id}",
		Summary:     "Cerrar una sesión concreta",
		Description: "Revoca la sesión {id} si pertenece al sujeto autenticado. Una sesión ajena o inexistente " +
			"produce 404 en ambos casos (nunca 403: eso confirmaría que el id existe).",
		DefaultStatus: http.StatusNoContent,
		Tags:          []string{"Acceso"},
		Metadata:      metaBearer,
		Middlewares:   huma.Middlewares{MiddlewareAutenticacion(api, validador, false)},
	}, m.CerrarSesion)

	huma.Register(api, huma.Operation{
		OperationID: "acceso-cerrar-todas-las-sesiones",
		Method:      http.MethodDelete,
		Path:        prefijo + "/sesiones",
		Summary:     "Cerrar todas las sesiones (logout de todos los dispositivos)",
		Description: "Revoca TODAS las sesiones activas del usuario, incluida la actual (decisión de diseño: " +
			"la sección 7 del documento no define un mecanismo de 'preservar la actual' para esta ruta — ver " +
			"el informe de la tarea). Operación de alto valor: exige ExigirSesionViva=true (una ventana de " +
			"revocación de hasta 10 minutos no es aceptable aquí) y evalúa Confianza.",
		DefaultStatus: http.StatusNoContent,
		Tags:          []string{"Acceso"},
		Metadata:      metaBearerAltoValor,
		Middlewares:   huma.Middlewares{MiddlewareAutenticacion(api, validador, true)},
	}, m.CerrarTodasLasSesiones)

	huma.Register(api, huma.Operation{
		OperationID: "acceso-listar-sesiones",
		Method:      http.MethodGet,
		Path:        prefijo + "/sesiones",
		Summary:     "Listar mis sesiones activas",
		Description: "Devuelve las sesiones activas del usuario autenticado (pantalla de \"dispositivos " +
			"conectados\"), marcando cuál es la sesión actual. No se audita.",
		Tags:        []string{"Acceso"},
		Metadata:    metaBearer,
		Middlewares: huma.Middlewares{MiddlewareAutenticacion(api, validador, false)},
	}, m.ListarSesiones)

	huma.Register(api, huma.Operation{
		OperationID: "acceso-jwks",
		Method:      http.MethodGet,
		Path:        "/.well-known/jwks.json",
		Summary:     "Conjunto de llaves públicas de verificación (JWKS)",
		Description: "RFC 7517/8615. Incluye la llave activa y cualquier llave previa configurada durante una " +
			"ventana de rotación (ADR 0020). Público por diseño: no requiere autenticación ni pasa por Confianza.",
		Tags: []string{"Acceso"},
		Metadata: map[string]any{
			"x-auth-nivel": "publico-sin-token",
			"x-rate-limit": "ninguno: endpoint de descubrimiento de llaves públicas (ADR 0020 §4).",
		},
	}, m.JWKS)

	return api
}
