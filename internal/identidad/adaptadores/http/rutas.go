package http

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humafiber"
	"github.com/gofiber/fiber/v2"
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
func RegistrarRutas(app *fiber.App, m *ManejadorIdentidad) huma.API {
	app.Use(middlewareOrigenSolicitud)

	api := humafiber.NewV2(app, huma.DefaultConfig("Identidad", "0.1.0"))

	metadatosEndpointPublico := map[string]any{
		"x-auth-nivel":   "publico-sin-token",
		"x-rate-limit":   "pendiente: agente seguridad-perimetral / contexto Confianza (no-op hoy, ver adaptadores/confianza)",
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

	huma.Register(api, huma.Operation{
		OperationID: "identidad-obtener-usuario",
		Method:      http.MethodGet,
		Path:        prefijo + "/usuarios/{id}",
		Summary:     "Consultar un usuario por ID",
		Description: "Devuelve VistaUsuario (nunca el agregado ni el hash de contraseña). La autorización " +
			"real (¿puede el solicitante ver a este usuario?) es de Tenencia/Acceso, fuera de alcance: " +
			"este endpoint queda público a nivel de transporte a propósito, como placeholder hasta que " +
			"exista el middleware de autenticación de Acceso — no debe exponerse así en producción.",
		Tags:     []string{"Identidad"},
		Metadata: metadatosEndpointPublico,
	}, m.ObtenerPorID)

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
