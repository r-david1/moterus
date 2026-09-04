package http

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humafiber"
	"github.com/gofiber/fiber/v2"

	accesopuertos "github.com/r-david1/moterus/internal/acceso/puertos"
	"github.com/r-david1/moterus/internal/tenencia/dominio"
	"github.com/r-david1/moterus/internal/tenencia/puertos"
)

// prefijo es el prefijo de ruta de todos los endpoints de Tenencia (§7 del
// diseño).
const prefijo = "/tenencia"

// RegistrarRutas monta el middleware de OrigenSolicitud y las operaciones
// del MVP de Tenencia (§7 del diseño) sobre app, con Huma v2 (ADR 0006).
//
// validador es el acceso/puertos.ValidadorDeAccesos que
// middlewareAutenticacionTenencia consume (§2.4 del diseño: "Tenencia
// consume ValidadorDeAccesos solo desde el adaptador HTTP"). autorizador es
// el propio VerificadorDeAutorizacion de Tenencia (AutorizarCasoDeUso),
// montado como middleware de autorización (§3.7). alcance es el adaptador
// trivial que publica el AlcanceDeTenencia en el ctx tras una autorización
// exitosa.
//
// Rutas de metadatos (OpenAPI/docs/schemas) con prefijo propio: Identidad y
// Acceso ya montan cada uno su propia instancia de huma.API sobre el mismo
// *fiber.App (cmd/api/main.go) con overrides de OpenAPIPath/DocsPath/
// SchemasPath (ver el comentario de acceso/adaptadores/http/rutas.go, el
// bug que ese override corrige). Tenencia sigue exactamente el mismo
// patrón para no repetirlo: sin el override, tres instancias de huma.API
// competirían por /openapi.json, /docs y /schemas/*, y Fiber serviría solo
// la primera registrada.
func RegistrarRutas(app *fiber.App, m *ManejadorTenencia, validador accesopuertos.ValidadorDeAccesos, autorizador puertos.VerificadorDeAutorizacion, alcance puertos.AlcanceDeTenencia) huma.API {
	app.Use(middlewareOrigenSolicitud)

	cfg := huma.DefaultConfig("Tenencia", "0.1.0")
	cfg.OpenAPIPath = prefijo + "/openapi"
	cfg.DocsPath = prefijo + "/docs"
	cfg.SchemasPath = prefijo + "/schemas"
	api := humafiber.NewV2(app, cfg)

	autenticacion := middlewareAutenticacionTenencia(api, validador)
	conPermiso := func(permiso dominio.Permiso) huma.Middlewares {
		return huma.Middlewares{autenticacion, middlewareAutorizacionTenencia(api, autorizador, alcance, permiso)}
	}
	soloAutenticado := huma.Middlewares{autenticacion}

	metaBearer := func(permiso string, rateLimit string) map[string]any {
		meta := map[string]any{"x-auth-nivel": "bearer-acceso"}
		if permiso != "" {
			meta["x-permiso-requerido"] = permiso
		}
		if rateLimit != "" {
			meta["x-rate-limit"] = rateLimit
		}
		return meta
	}

	rateLimitCrearOrganizacion := "ADR 0018/§11.3: EvaluadorConfianza, accion=crear_organizacion (5/hora por usuario, 20/hora por IP)."
	rateLimitInvitarMiembro := "ADR 0018/§11.3: EvaluadorConfianza, accion=invitar_miembro (20/hora por organización, 5/min por IP)."
	rateLimitAceptarInvitacion := "ADR 0018/§11.3: EvaluadorConfianza, accion=aceptar_invitacion (10/min por IP)."
	sinLimitePropio := "ninguno propio de Tenencia; protegido por autenticación Bearer y por el rol exigido."

	// --- Organizaciones ---------------------------------------------------

	huma.Register(api, huma.Operation{
		OperationID:   "tenencia-crear-organizacion",
		Method:        http.MethodPost,
		Path:          prefijo + "/organizaciones",
		Summary:       "Crear una organización",
		Description:   "Cualquier sujeto autenticado y activo puede fundar una organización (§3.1 del diseño): nace con su fundador como propietario, en la misma transacción (INV-TEN-03).",
		DefaultStatus: http.StatusCreated,
		Tags:          []string{"Tenencia"},
		Metadata:      metaBearer("", rateLimitCrearOrganizacion),
		Middlewares:   soloAutenticado,
	}, m.CrearOrganizacion)

	huma.Register(api, huma.Operation{
		OperationID: "tenencia-listar-mis-organizaciones",
		Method:      http.MethodGet,
		Path:        prefijo + "/organizaciones",
		Summary:     "Listar las organizaciones propias",
		Description: "Devuelve las organizaciones a las que el sujeto autenticado pertenece, con su rol (§3.6 del diseño). No requiere autorización de Tenencia ni se audita.",
		Tags:        []string{"Tenencia"},
		Metadata:    metaBearer("", sinLimitePropio),
		Middlewares: soloAutenticado,
	}, m.ListarMisOrganizaciones)

	huma.Register(api, huma.Operation{
		OperationID: "tenencia-obtener-organizacion",
		Method:      http.MethodGet,
		Path:        prefijo + "/organizaciones/{idOrganizacion}",
		Summary:     "Consultar una organización por ID",
		Description: "Exige organizacion.ver. 404 si el sujeto no es miembro (INV-TEN-17, indistinguible de \"no existe\").",
		Tags:        []string{"Tenencia"},
		Metadata:    metaBearer(dominio.PermisoOrganizacionVer.Valor(), sinLimitePropio),
		Middlewares: conPermiso(dominio.PermisoOrganizacionVer),
	}, m.ObtenerOrganizacion)

	huma.Register(api, huma.Operation{
		OperationID: "tenencia-actualizar-organizacion",
		Method:      http.MethodPatch,
		Path:        prefijo + "/organizaciones/{idOrganizacion}",
		Summary:     "Renombrar o cambiar el alias de una organización",
		Description: "Exige organizacion.editar.",
		Tags:        []string{"Tenencia"},
		Metadata:    metaBearer(dominio.PermisoOrganizacionEditar.Valor(), sinLimitePropio),
		Middlewares: conPermiso(dominio.PermisoOrganizacionEditar),
	}, m.ActualizarOrganizacion)

	huma.Register(api, huma.Operation{
		OperationID: "tenencia-cambiar-estado-organizacion",
		Method:      http.MethodPost,
		Path:        prefijo + "/organizaciones/{idOrganizacion}/cambios-estado",
		Summary:     "Suspender, reactivar o archivar una organización",
		Description: "Exige organizacion.archivar (§3.4 del diseño). archivada es terminal e irreversible (INV-TEN-04).",
		Tags:        []string{"Tenencia"},
		Metadata:    metaBearer(dominio.PermisoOrganizacionArchivar.Valor(), sinLimitePropio),
		Middlewares: conPermiso(dominio.PermisoOrganizacionArchivar),
	}, m.CambiarEstadoOrganizacion)

	// --- Membresías ---------------------------------------------------------

	huma.Register(api, huma.Operation{
		OperationID: "tenencia-listar-miembros",
		Method:      http.MethodGet,
		Path:        prefijo + "/organizaciones/{idOrganizacion}/miembros",
		Summary:     "Listar los miembros de una organización",
		Description: "Exige miembro.ver. Nunca incluye correo ni nombre del usuario (INV-TEN-29).",
		Tags:        []string{"Tenencia"},
		Metadata:    metaBearer(dominio.PermisoMiembroVer.Valor(), sinLimitePropio),
		Middlewares: conPermiso(dominio.PermisoMiembroVer),
	}, m.ListarMiembros)

	huma.Register(api, huma.Operation{
		OperationID:   "tenencia-agregar-miembro",
		Method:        http.MethodPost,
		Path:          prefijo + "/organizaciones/{idOrganizacion}/miembros",
		Summary:       "Agregar un miembro por alta directa (sin invitación)",
		Description:   "Exige miembro.invitar (§3.3 del diseño: el catálogo cerrado de 8 permisos no distingue \"invitar por correo\" de \"dar de alta directamente\", ambas comparten permiso y regla de dominancia). No es el camino del producto — el camino del producto es invitar por correo.",
		DefaultStatus: http.StatusCreated,
		Tags:          []string{"Tenencia"},
		Metadata:      metaBearer(dominio.PermisoMiembroInvitar.Valor(), sinLimitePropio),
		Middlewares:   conPermiso(dominio.PermisoMiembroInvitar),
	}, m.AgregarMiembro)

	huma.Register(api, huma.Operation{
		OperationID: "tenencia-cambiar-rol-miembro",
		Method:      http.MethodPatch,
		Path:        prefijo + "/organizaciones/{idOrganizacion}/miembros/{idUsuario}",
		Summary:     "Cambiar el rol de un miembro",
		Description: "Exige miembro.cambiar_rol, sujeto a la regla de dominancia (INV-TEN-20): nadie otorga un rol superior al propio ni modifica una membresía de rol superior al propio.",
		Tags:        []string{"Tenencia"},
		Metadata:    metaBearer(dominio.PermisoMiembroCambiarRol.Valor(), sinLimitePropio),
		Middlewares: conPermiso(dominio.PermisoMiembroCambiarRol),
	}, m.CambiarRolMiembro)

	huma.Register(api, huma.Operation{
		OperationID:   "tenencia-remover-miembro",
		Method:        http.MethodDelete,
		Path:          prefijo + "/organizaciones/{idOrganizacion}/miembros/{idUsuario}",
		Summary:       "Remover a un miembro",
		Description:   "Exige miembro.remover, sujeto a la regla de dominancia (INV-TEN-20). Transición a removida, nunca DELETE físico (INV-TEN-08).",
		DefaultStatus: http.StatusNoContent,
		Tags:          []string{"Tenencia"},
		Metadata:      metaBearer(dominio.PermisoMiembroRemover.Valor(), sinLimitePropio),
		Middlewares:   conPermiso(dominio.PermisoMiembroRemover),
	}, m.RemoverMiembro)

	huma.Register(api, huma.Operation{
		OperationID:   "tenencia-abandonar-organizacion",
		Method:        http.MethodDelete,
		Path:          prefijo + "/organizaciones/{idOrganizacion}/miembros/actual",
		Summary:       "Abandonar la organización (salir por iniciativa propia)",
		Description:   "No exige un permiso de la matriz (§3.2 del diseño: \"un sujeto siempre puede actuar sobre su propia membresía\"); el middleware de autorización usa miembro.ver como gate mínimo de \"sos miembro activo de una organización operativa\" para poder fijar el AlcanceDeTenencia. El invariante del último propietario (INV-TEN-06) sigue aplicando: el único propietario no puede abandonar sin transferir antes.",
		DefaultStatus: http.StatusNoContent,
		Tags:          []string{"Tenencia"},
		Metadata:      metaBearer(dominio.PermisoMiembroVer.Valor(), sinLimitePropio),
		Middlewares:   conPermiso(dominio.PermisoMiembroVer),
	}, m.AbandonarOrganizacion)

	huma.Register(api, huma.Operation{
		OperationID:   "tenencia-transferir-propiedad",
		Method:        http.MethodPost,
		Path:          prefijo + "/organizaciones/{idOrganizacion}/transferencias-propiedad",
		Summary:       "Transferir la propiedad de la organización",
		Description:   "Exige propiedad.transferir. El destinatario debe ser miembro activo previo.",
		DefaultStatus: http.StatusNoContent,
		Tags:          []string{"Tenencia"},
		Metadata:      metaBearer(dominio.PermisoPropiedadTransferir.Valor(), sinLimitePropio),
		Middlewares:   conPermiso(dominio.PermisoPropiedadTransferir),
	}, m.TransferirPropiedad)

	// --- Invitaciones ---------------------------------------------------------

	huma.Register(api, huma.Operation{
		OperationID:   "tenencia-invitar-miembro",
		Method:        http.MethodPost,
		Path:          prefijo + "/organizaciones/{idOrganizacion}/invitaciones",
		Summary:       "Invitar a un miembro por correo",
		Description:   "Exige miembro.invitar, sujeto a la regla de dominancia sobre el rol propuesto (INV-TEN-20). El token en claro NUNCA aparece en la respuesta (INV-TEN-23): solo sale del proceso por el notificador.",
		DefaultStatus: http.StatusCreated,
		Tags:          []string{"Tenencia"},
		Metadata:      metaBearer(dominio.PermisoMiembroInvitar.Valor(), rateLimitInvitarMiembro),
		Middlewares:   conPermiso(dominio.PermisoMiembroInvitar),
	}, m.InvitarMiembro)

	huma.Register(api, huma.Operation{
		OperationID:   "tenencia-revocar-invitacion",
		Method:        http.MethodDelete,
		Path:          prefijo + "/organizaciones/{idOrganizacion}/invitaciones/{idInvitacion}",
		Summary:       "Revocar una invitación pendiente",
		Description:   "Exige miembro.invitar. Sin DELETE físico: transición a revocada (evidencia forense).",
		DefaultStatus: http.StatusNoContent,
		Tags:          []string{"Tenencia"},
		Metadata:      metaBearer(dominio.PermisoMiembroInvitar.Valor(), sinLimitePropio),
		Middlewares:   conPermiso(dominio.PermisoMiembroInvitar),
	}, m.RevocarInvitacion)

	huma.Register(api, huma.Operation{
		OperationID:   "tenencia-aceptar-invitacion",
		Method:        http.MethodPost,
		Path:          prefijo + "/invitaciones/aceptaciones",
		Summary:       "Aceptar una invitación",
		Description:   "Requiere autenticación (Bearer) pero NO autorización de Tenencia: quien acepta no es miembro todavía (§3.5 del diseño). Deliberadamente fuera de /organizaciones/{id} (el cliente que llega desde el enlace del correo tiene el token, no el ID de la organización). Token desconocido, malformado, revocado, ya aceptado, expirado o de destinatario distinto producen la MISMA respuesta observable (INV-TEN-24).",
		DefaultStatus: http.StatusCreated,
		Tags:          []string{"Tenencia"},
		Metadata:      metaBearer("", rateLimitAceptarInvitacion),
		Middlewares:   soloAutenticado,
	}, m.AceptarInvitacion)

	return api
}
