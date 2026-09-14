package puertos

import (
	"context"
	"time"

	"github.com/r-david1/moterus/internal/tenencia/dominio"
)

// --- Persistencia -------------------------------------------------------------

// RepositorioOrganizaciones es el puerto de salida para la persistencia del
// agregado Organizacion.
type RepositorioOrganizaciones interface {
	Guardar(ctx context.Context, o *dominio.Organizacion) error
	BuscarPorID(ctx context.Context, id dominio.IDOrganizacion) (*dominio.Organizacion, error)
	BuscarPorAlias(ctx context.Context, a dominio.AliasOrganizacion) (*dominio.Organizacion, error)

	// CargarParaActualizar toma el candado de la fila raíz (SELECT ... FOR
	// UPDATE) y es OBLIGATORIO antes de cualquier operación que pueda alterar
	// el conteo de propietarios activos (§1.2, INV-TEN-06). La fila de
	// organizaciones es el token de consistencia del conjunto de membresías.
	// Solo tiene sentido dentro de una UnidadDeTrabajo.
	CargarParaActualizar(ctx context.Context, id dominio.IDOrganizacion) (*dominio.Organizacion, error)
}

// RepositorioMembresias es el puerto de salida para la persistencia del
// agregado Membresia.
type RepositorioMembresias interface {
	Guardar(ctx context.Context, m *dominio.Membresia) error
	BuscarPorID(ctx context.Context, id dominio.IDMembresia) (*dominio.Membresia, error)

	// BuscarVigente resuelve la membresía NO removida del par (usuario,
	// organización). Es la consulta del camino caliente de autorización: se
	// sirve del índice único parcial de §6, una sola lectura por PK lógica.
	BuscarVigente(ctx context.Context, u dominio.IDUsuario, o dominio.IDOrganizacion) (*dominio.Membresia, error)

	ListarDeOrganizacion(ctx context.Context, o dominio.IDOrganizacion) ([]*dominio.Membresia, error)
	ListarDeUsuario(ctx context.Context, u dominio.IDUsuario) ([]*dominio.Membresia, error)

	ContarPropietariosActivos(ctx context.Context, o dominio.IDOrganizacion) (int, error)
	ContarActivasDeOrganizacion(ctx context.Context, o dominio.IDOrganizacion) (int, error)
	ContarOrganizacionesPropiasDeUsuario(ctx context.Context, u dominio.IDUsuario) (int, error)
}

// RepositorioInvitaciones es el puerto de salida para la persistencia del
// agregado Invitacion.
type RepositorioInvitaciones interface {
	Guardar(ctx context.Context, i *dominio.Invitacion) error
	BuscarPorID(ctx context.Context, id dominio.IDInvitacion) (*dominio.Invitacion, error)

	// BuscarPorHash es la única consulta que atraviesa el aislamiento por
	// organización, porque quien acepta todavía no es miembro de ninguna: el
	// token ES la capacidad. Se implementa sobre la función SECURITY DEFINER
	// documentada en §6.3, que es la ÚNICA vía de escape de RLS del contexto.
	BuscarPorHash(ctx context.Context, h dominio.HashTokenInvitacion) (*dominio.Invitacion, error)

	BuscarPendiente(ctx context.Context, o dominio.IDOrganizacion, c dominio.CorreoDestinatario) (*dominio.Invitacion, error)
	ListarPendientesDeOrganizacion(ctx context.Context, o dominio.IDOrganizacion) ([]*dominio.Invitacion, error)
	ContarPendientesDeOrganizacion(ctx context.Context, o dominio.IDOrganizacion) (int, error)
}

// --- Infraestructura neutra ------------------------------------------------------

// Reloj es el puerto de salida para obtener la hora actual. El dominio nunca
// llama a time.Now() (INV-TEN-14); los casos de uso lo hacen a través de
// este puerto para poder fijar el tiempo en los tests.
type Reloj interface{ Ahora() time.Time }

// GeneradorIDs es el puerto de salida para generar identificadores nuevos.
// El dominio nunca genera UUIDs por sí mismo.
type GeneradorIDs interface {
	NuevoIDOrganizacion() (dominio.IDOrganizacion, error) // UUIDv7
	NuevoIDMembresia() (dominio.IDMembresia, error)       // UUIDv7
	NuevoIDInvitacion() (dominio.IDInvitacion, error)     // UUIDv7
}

// GeneradorTokens genera el secreto opaco de la invitación. Mismo contrato
// conceptual que el homónimo de Identidad; tipo propio para no importarlo.
type GeneradorTokens interface {
	GenerarTokenInvitacion() (dominio.TokenInvitacionPlano, error)
}

// UnidadDeTrabajo es el puerto de salida que agrupa la escritura de negocio
// y el registro de auditoría en una sola transacción (mismo criterio que
// acceso/puertos.UnidadDeTrabajo, ADR 0005).
type UnidadDeTrabajo interface {
	Ejecutar(ctx context.Context, fn func(ctx context.Context) error) error
}

// AlcanceDeTenencia publica en el ctx el par (usuario_actual, organizacion_actual)
// que la UnidadDeTrabajo traduce a `SET LOCAL app.usuario_actual` /
// `SET LOCAL app.organizacion_actual` al abrir la transacción. Es el punto —el
// único— donde la decisión de autorización de la aplicación y las políticas RLS
// de Postgres se mantienen sincronizadas (§6.3, ADR candidato 0031).
type AlcanceDeTenencia interface {
	ConAlcance(ctx context.Context, idUsuario, idOrganizacion string) context.Context
}

// --- Cruce de bounded contexts (anticorrupción) -----------------------------------

// VerificadorDeSujetos es el ACL sobre identidad/puertos.ConsultorDeUsuarios,
// deliberadamente ESTRECHO: Tenencia necesita dos hechos (¿existe? ¿está
// activo?) y un tercero solo en el flujo de invitación (su correo verificado,
// para INV-TEN-21). No necesita —ni debe poder— leer el resto de VistaUsuario.
type VerificadorDeSujetos interface {
	// EsElegible se invoca antes de crear cualquier membresía (INV-TEN-07).
	EsElegible(ctx context.Context, idUsuario string) (SujetoElegible, error)
}

// SujetoElegible es la salida de VerificadorDeSujetos.EsElegible.
type SujetoElegible struct {
	Existe            bool
	Activo            bool   // Estado == "activo": ni pendiente, ni suspendido, ni bloqueado
	CorreoNormalizado string // solo se usa en AceptarInvitacion; nunca se persiste en Tenencia
}

// EvaluadorConfianza es el puerto de salida implementado sobre el contexto
// Confianza. Acota creación de organizaciones, invitaciones y redenciones
// (§2.4 del diseño).
type EvaluadorConfianza interface {
	Evaluar(ctx context.Context, s SolicitudEvaluacion) (DecisionConfianza, error)
	RegistrarResultado(ctx context.Context, r ResultadoIntento) error
}

// RegistroAuditoria es el puerto de salida implementado sobre el contexto
// Auditoría.
//
// A diferencia de Identidad y Acceso, la firma lleva IDOrganizacion: es la
// pieza que puebla auditoria.organizacion_id, NULL desde la migración
// 000002.
type RegistroAuditoria interface {
	Registrar(ctx context.Context, e dominio.EventoDominio, org dominio.IDOrganizacion,
		origen dominio.OrigenSolicitud) error
}

// PublicadorEventos es el puerto de salida para la integración asíncrona. Se
// invoca fuera de la UnidadDeTrabajo: es best-effort, no transaccional.
type PublicadorEventos interface {
	Publicar(ctx context.Context, eventos ...dominio.EventoDominio) error
}

// NotificadorInvitaciones entrega el token EN CLARO al destinatario. Es el único
// lugar del sistema por el que ese valor puede salir del proceso (INV-TEN-23).
// Implementación de producción: NotificadorInvitacionesTransaccional, vía
// SMTP genérico (ADR 0054/0055); NotificadorInvitacionesLog es el
// fallback de desarrollo.
type NotificadorInvitaciones interface {
	EnviarInvitacion(ctx context.Context, destinatario dominio.CorreoDestinatario,
		nombreOrganizacion string, rol dominio.Rol, tokenPlano string, expiraEn time.Time) error
}

// --- Tipos de apoyo de los puertos de cruce ---------------------------------
//
// Viven en puertos, no en dominio, porque son el contrato con otro contexto
// (o con Confianza) y no lenguaje ubicuo de Tenencia.

// SolicitudEvaluacion es la entrada de EvaluadorConfianza.Evaluar.
type SolicitudEvaluacion struct {
	Accion       string // "crear_organizacion" | "invitar_miembro" | "aceptar_invitacion"
	ClaveCuenta  string // "usuario:<id>" u "organizacion:<id>"; Tenencia no conoce el correo del sujeto
	TenantID     string // por fin no vacío: cierra el hueco de confianza/puertos.Solicitud.TenantID
	Origen       dominio.OrigenSolicitud
	TokenCaptcha string
}

// DecisionConfianza es la salida de EvaluadorConfianza.Evaluar.
type DecisionConfianza struct {
	Permitido    bool
	Puntaje      float64
	Motivo       string
	ReintentarEn time.Duration
}

// ResultadoIntento es la entrada de EvaluadorConfianza.RegistrarResultado.
type ResultadoIntento struct {
	Accion      string
	ClaveCuenta string
	TenantID    string
	Origen      dominio.OrigenSolicitud
	Exitoso     bool
	IDUsuario   string
}

// Nota (§2.1 del diseño, no un puerto de salida de Tenencia):
// acceso/puertos.ValidadorDeAccesos. Tenencia lo consume SOLO desde el
// adaptador HTTP (el middleware), nunca desde tenencia/aplicacion. La capa
// de aplicación de Tenencia recibe un IDSujeto que YA es un identificador
// confiable; nunca ve un token, nunca sabe que existe un JWT.
