package http

import (
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humafiber"
	"github.com/gofiber/fiber/v2"

	accesopuertos "github.com/r-david1/moterus/internal/acceso/puertos"
)

// prefijo es el prefijo de ruta de todos los endpoints de Identidad.
const prefijo = "/identidad"

// RegistrarRutas monta el middleware de OrigenSolicitud y las tres
// operaciones del MVP de Identidad (sección 3 del diseño) sobre app,
// usando Huma v2 + el adaptador humafiber.NewV2 (ADR 0006: Huma genera el
// OpenAPI 3.1 y valida requests/responses a partir de los DTOs, no se
// mantiene una spec a mano).
//
// Nivel de auth de los tres endpoints, explícito por regla dura del
// encargo ("no hay endpoints desnudos"): PÚBLICOS. Ninguno exige un token
// — todavía no existe el contexto Acceso que los emite (ADR 0009) — y
// ninguno aplica rate limiting real: eso es responsabilidad del contexto
// Confianza / agente seguridad-perimetral, fuera de alcance de este
// adaptador. Se deja constancia de ambos hechos en Metadata de cada
// operación para que quede en el OpenAPI generado, no solo en un
// comentario de Go.
// validador es el acceso/puertos.ValidadorDeAccesos que
// middlewareAutenticacionAcceso (middleware_autenticacion.go) consume para
// cerrar el hueco de GET /identidad/usuarios/{id} (ADR 0019
// §Consecuencias). Puede ir nil ÚNICAMENTE para no romper los tests de
// integración existentes de este paquete, que hoy ejercitan Identidad de
// forma aislada sin montar Acceso (test/integracion/entorno_test.go): con
// nil, el endpoint se registra sin el middleware y queda público, igual
// que antes de esta migración. cmd/api/main.go SIEMPRE debe pasar el
// validador real.
func RegistrarRutas(app *fiber.App, m *ManejadorIdentidad, validador accesopuertos.ValidadorDeAccesos) huma.API {
	app.Use(middlewareOrigenSolicitud)

	api := humafiber.NewV2(app, huma.DefaultConfig("Identidad", "0.1.0"))

	metadatosEndpointPublico := map[string]any{
		"x-auth-nivel":   "publico-sin-token",
		"x-rate-limit":   "ADR 0018: rate limiting por IP y por cuenta + captcha invisible vía EvaluadorConfianza (real si REDIS_URL está configurado, no-op si no; ver adaptadores/confianza)",
		"x-adr-frontera": "ADR 0009: no emite JWT ni sesión; ver docs/adr/0009-frontera-identidad-acceso.md",
	}

	huma.Register(api, huma.Operation{
		OperationID: "identidad-registrar-usuario",
		Method:      http.MethodPost,
		Path:        prefijo + "/usuarios",
		Summary:     "Registrar un usuario nuevo",
		Description: "Alta de usuario en estado pendiente_verificacion (INV-ID-07). Público, sin token: es el propio alta de la cuenta.",
		Tags:        []string{"Identidad"},
		Metadata:    metadatosEndpointPublico,
	}, m.Registrar)

	huma.Register(api, huma.Operation{
		OperationID: "identidad-autenticar",
		Method:      http.MethodPost,
		Path:        prefijo + "/autenticaciones",
		Summary:     "Verificar credenciales (sin emitir sesión)",
		Description: "Verifica correo+contraseña y devuelve el resultado de negocio (ResultadoAutenticacion). " +
			"NO emite JWT ni cookie de sesión (ADR 0009, INV-ID-14): es responsabilidad del contexto Acceso, " +
			"todavía no implementado, orquestar este endpoint y emitir el token. Público, sin token propio: " +
			"es el paso previo a obtener uno.",
		Tags:     []string{"Identidad"},
		Metadata: metadatosEndpointPublico,
	}, m.Autenticar)

	opObtenerUsuario := huma.Operation{
		OperationID: "identidad-obtener-usuario",
		Method:      http.MethodGet,
		Path:        prefijo + "/usuarios/{id}",
		Summary:     "Consultar un usuario por ID",
		Description: "Devuelve VistaUsuario (nunca el agregado ni el hash de contraseña). La autorización " +
			"real (¿puede el solicitante ver a este usuario?) es de Tenencia, fuera de alcance: la " +
			"AUTENTICACIÓN (¿quién pregunta?) ya no está pendiente — exige un token de acceso Bearer válido " +
			"(ADR 0019 §Consecuencias: este es el gancho que cierra el hueco que dejó ADR 0009).",
		Tags: []string{"Identidad"},
		Metadata: map[string]any{
			"x-auth-nivel": "bearer-acceso",
		},
	}
	if validador != nil {
		opObtenerUsuario.Middlewares = huma.Middlewares{middlewareAutenticacionAcceso(api, validador)}
	} else {
		opObtenerUsuario.Metadata = metadatosEndpointPublico
		slog.Warn("identidad/adaptadores/http: RegistrarRutas se llamó sin validador de Acceso — " +
			"GET /identidad/usuarios/{id} queda público, sin autenticación. NO USAR EN PRODUCCIÓN.")
	}
	huma.Register(api, opObtenerUsuario, m.ObtenerPorID)

	huma.Register(api, huma.Operation{
		OperationID: "identidad-verificar-correo",
		Method:      http.MethodPost,
		Path:        prefijo + "/verificaciones-correo",
		Summary:     "Confirmar un correo con el token de verificación",
		Description: "Consume un token de un solo uso (sección 3.4 del diseño) y transiciona el usuario de " +
			"pendiente_verificacion a activo. 200 en éxito; 404 si el token es inválido/inexistente/ya " +
			"consumido; 410 si expiró (vigencia 24h). Público, sin token de sesión propio: es el paso " +
			"que un usuario recién registrado hace antes de poder autenticarse.",
		// VerificarCorreoOutput no tiene Body: sin fijar DefaultStatus, Huma
		// asume 204 No Content para una respuesta sin cuerpo (regla por
		// defecto de huma.Register). El encargo pide 200 explícito en éxito.
		DefaultStatus: http.StatusOK,
		Tags:          []string{"Identidad"},
		Metadata:      metadatosEndpointPublico,
	}, m.VerificarCorreo)

	// --- MFA / OTP: autoservicio del propio sujeto (§7 del diseño otp-mfa.md) --
	//
	// Los tres endpoints exigen SIEMPRE Bearer (a diferencia de los cuatro de
	// arriba, hoy públicos): actúan sobre el propio usuario autenticado, sin
	// ID en la ruta ni en el cuerpo. Igual que GET /identidad/usuarios/{id},
	// validador puede ir nil únicamente en tests que ejercitan Identidad de
	// forma aislada; cmd/api/main.go SIEMPRE pasa el validador real.
	metaMFA := map[string]any{
		"x-auth-nivel": "bearer-acceso",
	}
	var middlewaresMFA huma.Middlewares
	if validador != nil {
		middlewaresMFA = huma.Middlewares{middlewareAutenticacionAcceso(api, validador)}
	} else {
		slog.Warn("identidad/adaptadores/http: RegistrarRutas se llamó sin validador de Acceso — " +
			"los endpoints de MFA quedan sin autenticación. NO USAR EN PRODUCCIÓN.")
	}

	huma.Register(api, huma.Operation{
		OperationID: "identidad-habilitar-mfa",
		Method:      http.MethodPost,
		Path:        prefijo + "/usuarios/actual/factores-mfa",
		Summary:     "Habilitar un segundo factor TOTP",
		Description: "Genera un FactorMFA sin confirmar y devuelve el secreto en claro + la URI de " +
			"provisionamiento (QR), la única vez que salen del proceso (INV-MFA-02, §3.1 del diseño otp-mfa.md). " +
			"409 si ya existe un factor confirmado y activo (ADR 0037: uno por usuario en el MVP).",
		Tags:        []string{"Identidad", "MFA"},
		Metadata:    metaMFA,
		Middlewares: middlewaresMFA,
	}, m.HabilitarMFA)

	huma.Register(api, huma.Operation{
		OperationID: "identidad-confirmar-factor-mfa",
		Method:      http.MethodPost,
		Path:        prefijo + "/usuarios/actual/factores-mfa/confirmacion",
		Summary:     "Confirmar el segundo factor con el primer código TOTP",
		Description: "Verifica el primer código TOTP, activa Usuario.tieneMFA (INV-ID-08) y devuelve los 10 " +
			"códigos de respaldo en claro, la única vez (ADR 0040, §3.2 del diseño otp-mfa.md).",
		Tags:        []string{"Identidad", "MFA"},
		Metadata:    metaMFA,
		Middlewares: middlewaresMFA,
	}, m.ConfirmarFactorMFA)

	huma.Register(api, huma.Operation{
		OperationID: "identidad-deshabilitar-mfa",
		Method:      http.MethodDelete,
		Path:        prefijo + "/usuarios/actual/factores-mfa",
		Summary:     "Deshabilitar el segundo factor",
		Description: "Exige un código propio del factor (TOTP o de respaldo) en el cuerpo, además del Bearer " +
			"(ADR 0039, INV-MFA-05): una sesión robada no basta para desarmar la protección de la cuenta.",
		DefaultStatus: http.StatusNoContent,
		Tags:          []string{"Identidad", "MFA"},
		Metadata:      metaMFA,
		Middlewares:   middlewaresMFA,
	}, m.DeshabilitarMFA)

	huma.Register(api, huma.Operation{
		OperationID: "identidad-reenviar-verificacion-correo",
		Method:      http.MethodPost,
		Path:        prefijo + "/verificaciones-correo/reenvios",
		Summary:     "Reenviar el token de verificación de correo",
		Description: "Genera y envía un token de verificación nuevo (invalida el anterior) si el correo existe y sigue " +
			"pendiente de verificación. Responde SIEMPRE 202 Accepted, sin excepción (INV-ID-22): no revela si el " +
			"correo existe, si ya está verificado, ni el estado de la cuenta — mismo criterio anti-enumeración que login.",
		DefaultStatus: http.StatusAccepted,
		Tags:          []string{"Identidad"},
		Metadata:      metadatosEndpointPublico,
	}, m.ReenviarVerificacion)

	return api
}
