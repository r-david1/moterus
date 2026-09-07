package http

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humafiber"
	"github.com/gofiber/fiber/v2"

	accesopuertos "github.com/r-david1/moterus/internal/acceso/puertos"
	confianzatenencia "github.com/r-david1/moterus/internal/confianza/adaptadores/tenencia"
	"github.com/r-david1/moterus/internal/confianza/puertos"
)

// prefijo es el prefijo de ruta de todos los endpoints de Confianza (§7 del
// diseño docs/design/colas-virtuales.md).
const prefijo = "/confianza"

// RegistrarRutas monta el middleware de OrigenSolicitud y las operaciones
// de colas de acceso virtual de Confianza (§7 del diseño) sobre app, con
// Huma v2 (ADR 0006).
//
// validador es el acceso/puertos.ValidadorDeAccesos que
// middlewareAutenticacion consume para los endpoints de administración
// org-scoped (§7.2 del diseño: Bearer + acceso/puertos, mismo patrón que
// Tenencia). autorizador es el ACL propio de Confianza sobre Tenencia
// (confianza/adaptadores/tenencia.VerificadorAutorizacion), montado como
// middleware de autorización (§7.2).
//
// Esta función NO se invoca todavía desde cmd/api/main.go (tarea
// posterior, §12 del diseño): tampoco monta MiddlewareSalaDeEspera sobre
// ninguna ruta de otro contexto — eso también es la tarea posterior, junto
// con la acción AccionIngresoASala del catálogo de Confianza (§12).
//
// Rutas de metadatos (OpenAPI/docs/schemas) con prefijo propio: cada
// contexto monta su propia instancia de huma.API sobre el mismo *fiber.App
// (cmd/api/main.go) con overrides de OpenAPIPath/DocsPath/SchemasPath (ver
// el comentario de acceso/adaptadores/http/rutas.go, el bug que ese
// override corrige). Confianza sigue exactamente el mismo patrón: sin el
// override, las instancias de huma.API competirían por /openapi.json,
// /docs y /schemas/*, y Fiber serviría solo la primera registrada.
func RegistrarRutas(app *fiber.App, m *ManejadorConfianza, validador accesopuertos.ValidadorDeAccesos, autorizador puertos.VerificadorDeAutorizacion) huma.API {
	app.Use(middlewareOrigenSolicitud)

	cfg := huma.DefaultConfig("Confianza", "0.1.0")
	cfg.OpenAPIPath = prefijo + "/openapi"
	cfg.DocsPath = prefijo + "/docs"
	cfg.SchemasPath = prefijo + "/schemas"
	api := humafiber.NewV2(app, cfg)

	autenticacion := middlewareAutenticacion(api, validador)
	conPermiso := func(permiso string) huma.Middlewares {
		return huma.Middlewares{autenticacion, middlewareAutorizacion(api, autorizador, permiso)}
	}

	metaPublico := map[string]any{
		"x-auth-nivel": "publico-sin-token",
		"x-rate-limit": "ADR 0018/§12 del diseño colas-virtuales.md: EvaluadorDeRiesgo, accion=ingreso_a_sala (20/min por IP) — cableado pendiente de la tarea de integración con otros contextos (§12).",
	}
	metaBearer := func(permiso string) map[string]any {
		return map[string]any{
			"x-auth-nivel":        "bearer-acceso",
			"x-permiso-requerido": permiso,
		}
	}

	// --- Endpoints públicos (§7.1 del diseño) --------------------------------

	huma.Register(api, huma.Operation{
		OperationID:   "confianza-ingresar-sala",
		Method:        http.MethodPost,
		Path:          prefijo + "/salas-espera/{aliasSala}/tickets",
		Summary:       "Ingresar a una sala de espera y obtener un turno",
		Description:   "Sin autenticación (§7.1 del diseño): es anterior a cualquier sesión, por definición. Devuelve el ticket en claro, única vez que sale del proceso (INV-COLA-07).",
		DefaultStatus: http.StatusCreated,
		Tags:          []string{"Confianza"},
		Metadata:      metaPublico,
	}, m.IngresarASala)

	huma.Register(api, huma.Operation{
		OperationID: "confianza-consultar-turno",
		Method:      http.MethodGet,
		Path:        prefijo + "/salas-espera/{aliasSala}/turno",
		Summary:     "Consultar el turno de un ticket de cola",
		Description: "El ticket viaja en la cabecera X-Ticket-Cola, nunca en la ruta (§7.1 del diseño). Cualquier desenlace (incluidos ticket_desconocido/ticket_consumido/turno_caducado) es un 200 con el desenlace en el cuerpo: un ticket de cola no protege ningún secreto (§1.8 del diseño).",
		Tags:        []string{"Confianza"},
		Metadata:    map[string]any{"x-auth-nivel": "publico-sin-token"},
	}, m.ConsultarTurno)

	huma.Register(api, huma.Operation{
		OperationID: "confianza-obtener-sala-publica",
		Method:      http.MethodGet,
		Path:        prefijo + "/salas-espera/{aliasSala}",
		Summary:     "Consultar el estado agregado de una sala de espera",
		Description: "Endpoint público y cacheable (Cache-Control: public, max-age=5, apto para CDN, §7.1 del diseño). Nunca revela la organización dueña, la ruta protegida ni la existencia de otras salas (INV-COLA-15).",
		Tags:        []string{"Confianza"},
		Metadata:    map[string]any{"x-auth-nivel": "publico-sin-token"},
	}, m.ObtenerSalaPublica)

	// --- Endpoints de administración (§7.2 del diseño) -----------------------
	//
	// Las salas de alcance sistema no tienen endpoints HTTP (§7.2 del
	// diseño: "este producto no tiene rol de administrador de plataforma");
	// se abren por un subcomando de CLI fuera de esta API. Estos dos
	// endpoints son exclusivamente org-scoped.

	huma.Register(api, huma.Operation{
		OperationID:   "confianza-abrir-sala",
		Method:        http.MethodPost,
		Path:          prefijo + "/organizaciones/{idOrganizacion}/salas-espera",
		Summary:       "Abrir una sala de espera org-scoped",
		Description:   "Exige organizacion.editar, reutilizado del catálogo cerrado de Tenencia en vez de agregar un noveno permiso (ADR candidato 0046, §7.2 del diseño).",
		DefaultStatus: http.StatusCreated,
		Tags:          []string{"Confianza"},
		Metadata:      metaBearer(confianzatenencia.PermisoOrganizacionEditar),
		Middlewares:   conPermiso(confianzatenencia.PermisoOrganizacionEditar),
	}, m.AbrirSala)

	huma.Register(api, huma.Operation{
		OperationID: "confianza-actualizar-sala",
		Method:      http.MethodPatch,
		Path:        prefijo + "/organizaciones/{idOrganizacion}/salas-espera/{idSala}",
		Summary:     "Cambiar el ritmo de admisión o el estado de una sala de espera",
		Description: "Exige organizacion.editar. Cambia el ritmo de admisión (durante el pico, §3.2 del diseño) y/o transiciona el estado (drenar/cerrar/reabrir, §3.3 del diseño) según los campos presentes en el cuerpo.",
		Tags:        []string{"Confianza"},
		Metadata:    metaBearer(confianzatenencia.PermisoOrganizacionEditar),
		Middlewares: conPermiso(confianzatenencia.PermisoOrganizacionEditar),
	}, m.ActualizarSala)

	return api
}
