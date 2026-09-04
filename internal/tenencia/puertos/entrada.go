package puertos

import (
	"context"
	"time"

	"github.com/r-david1/moterus/internal/tenencia/dominio"
)

// Nota de ubicación: los tipos ComandoX/ConsultaX/ResultadoX/VistaX se
// definen aquí, junto a las interfaces de entrada que los usan como firma,
// por la misma razón que en identidad/puertos/entrada.go y
// acceso/puertos/entrada.go (sección 2 del diseño): son el contrato del
// puerto y tenencia/aplicacion, que implementa estas interfaces, no puede
// definirlos ella misma sin crear un import circular.
//
// Los comandos y consultas que provienen de un cliente HTTP transportan
// PRIMITIVOS, nunca value objects de dominio (salvo dominio.OrigenSolicitud,
// que no es un dato de negocio a validar sino el contexto forense que el
// middleware/handler ya construyó): el caso de uso es quien construye los
// VOs y decide qué error tipado emitir si no validan.

// --- Organizaciones ----------------------------------------------------------

// GestorDeOrganizaciones es el puerto de entrada que implementan los casos
// de uso CrearOrganizacion, ActualizarOrganizacion y
// CambiarEstadoOrganizacion (§3.1, §3.4 del diseño).
type GestorDeOrganizaciones interface {
	Crear(ctx context.Context, cmd ComandoCrearOrganizacion) (VistaOrganizacion, error)
	Actualizar(ctx context.Context, cmd ComandoActualizarOrganizacion) (VistaOrganizacion, error)
	CambiarEstado(ctx context.Context, cmd ComandoCambiarEstadoOrganizacion) (VistaOrganizacion, error)
}

// ConsultorDeOrganizaciones es el puerto de entrada que implementa el caso
// de uso ObtenerOrganizacion (§3.6 del diseño).
type ConsultorDeOrganizaciones interface {
	ObtenerPorID(ctx context.Context, q ConsultaOrganizacionPorID) (VistaOrganizacion, error)
}

// --- Membresías ---------------------------------------------------------------

// GestorDeMembresias es el puerto de entrada que implementan los casos de
// uso AgregarMiembro, CambiarRol, RemoverMiembro, AbandonarOrganizacion y
// TransferirPropiedad (§3.2, §3.3 del diseño).
type GestorDeMembresias interface {
	Agregar(ctx context.Context, cmd ComandoAgregarMiembro) (VistaMiembro, error)
	CambiarRol(ctx context.Context, cmd ComandoCambiarRol) (VistaMiembro, error)
	Remover(ctx context.Context, cmd ComandoRemoverMiembro) error
	Abandonar(ctx context.Context, cmd ComandoAbandonarOrganizacion) error
	TransferirPropiedad(ctx context.Context, cmd ComandoTransferirPropiedad) error
}

// --- Invitaciones --------------------------------------------------------------

// GestorDeInvitaciones es el puerto de entrada que implementan los casos de
// uso InvitarMiembro, RevocarInvitacion y AceptarInvitacion (§3.5 del
// diseño).
type GestorDeInvitaciones interface {
	Invitar(ctx context.Context, cmd ComandoInvitarMiembro) (VistaInvitacion, error)
	Revocar(ctx context.Context, cmd ComandoRevocarInvitacion) error
	Aceptar(ctx context.Context, cmd ComandoAceptarInvitacion) (VistaMiembro, error)
}

// --- Puertos que Tenencia EXPONE a otros contextos ------------------------------
// Estos dos son el contrato público del bounded context y la razón de ser de
// todo lo demás. Cualquier contexto que necesite autorizar consume estos, y
// jamás las tablas organizaciones/membresias.

// VerificadorDeAutorizacion responde la pregunta del contexto:
// "¿este sujeto puede ejecutar esta acción en esta organización?".
// Es una DECISIÓN, no datos (INV-TEN-16): no devuelve la organización ni la
// lista de miembros, solo si se permite, con qué rol y —si no— por qué.
// Implementa el caso de uso Autorizar (§3.7 del diseño): el camino caliente
// y el contrato con el resto del sistema.
type VerificadorDeAutorizacion interface {
	Autorizar(ctx context.Context, q ConsultaAutorizacion) (Autorizacion, error)
}

// ConsultorDeMembresias expone los hechos crudos, sin aplicar la matriz de
// permisos. Existe separado de VerificadorDeAutorizacion para que un
// consumidor que necesite "el rol" (p. ej. pintarlo en una UI o poblar
// Confianza) no tenga que fabricar una consulta de autorización falsa para
// obtenerlo.
type ConsultorDeMembresias interface {
	ListarOrganizacionesDeUsuario(ctx context.Context, q ConsultaOrganizacionesDeUsuario) ([]VistaMembresiaDeUsuario, error)
	ListarMiembros(ctx context.Context, q ConsultaMiembrosDeOrganizacion) ([]VistaMiembro, error)
	RolEnOrganizacion(ctx context.Context, q ConsultaRolEnOrganizacion) (VistaRolEfectivo, error)
}

// --- Comandos: organizaciones -----------------------------------------------

// ComandoCrearOrganizacion transporta la entrada del caso de uso
// CrearOrganizacion (§3.1 del diseño).
type ComandoCrearOrganizacion struct {
	Nombre   string
	Alias    string
	IDSujeto string // SIEMPRE del token validado, nunca del cuerpo (INV-TEN-12)
	Origen   dominio.OrigenSolicitud
}

// ComandoActualizarOrganizacion transporta la entrada del caso de uso
// ActualizarOrganizacion (§3.2 del diseño). Los punteros distinguen "no
// enviado" (nil, no cambiar) de "enviado vacío" (validado y rechazado por el
// VO correspondiente).
type ComandoActualizarOrganizacion struct {
	IDOrganizacion string
	IDSujeto       string
	Nombre         *string // nil = no cambiar
	Alias          *string
	Origen         dominio.OrigenSolicitud
}

// ComandoCambiarEstadoOrganizacion transporta la entrada del caso de uso
// CambiarEstadoOrganizacion (§3.4 del diseño): suspender, reactivar o
// archivar.
type ComandoCambiarEstadoOrganizacion struct {
	IDOrganizacion string
	IDSujeto       string
	Destino        string // "activa" | "suspendida" | "archivada"
	Motivo         string
	Origen         dominio.OrigenSolicitud
}

// --- Comandos: membresías -----------------------------------------------------

// ComandoAgregarMiembro transporta la entrada del caso de uso AgregarMiembro
// (§3.3 del diseño): alta directa, sin invitación.
type ComandoAgregarMiembro struct {
	IDOrganizacion string
	IDSujeto       string // quién ejecuta
	IDUsuario      string // a quién se agrega; debe existir y estar activo en Identidad
	Rol            string
	Origen         dominio.OrigenSolicitud
}

// ComandoCambiarRol transporta la entrada del caso de uso CambiarRol (§3.2
// del diseño), sujeto a la regla de dominancia (INV-TEN-20).
type ComandoCambiarRol struct {
	IDOrganizacion string
	IDSujeto       string
	IDUsuario      string
	NuevoRol       string
	Origen         dominio.OrigenSolicitud
}

// ComandoRemoverMiembro transporta la entrada del caso de uso RemoverMiembro
// (§3.2 del diseño), sujeto a la regla de dominancia (INV-TEN-20).
type ComandoRemoverMiembro struct {
	IDOrganizacion string
	IDSujeto       string
	IDUsuario      string
	Origen         dominio.OrigenSolicitud
}

// ComandoAbandonarOrganizacion transporta la entrada del caso de uso
// AbandonarOrganizacion (§3.2 del diseño): un miembro remueve su propia
// membresía, sujeto solo a INV-TEN-06 (no puede dejar la organización sin
// propietarios activos).
type ComandoAbandonarOrganizacion struct {
	IDOrganizacion string
	IDSujeto       string
	Origen         dominio.OrigenSolicitud
}

// ComandoTransferirPropiedad transporta la entrada del caso de uso
// TransferirPropiedad (§3.2 del diseño).
type ComandoTransferirPropiedad struct {
	IDOrganizacion     string
	IDSujeto           string // debe ser propietario
	IDNuevoPropietario string
	// RolResultanteDelCedente: "administrador" (por defecto) | "miembro" | ""
	// para conservar propietario (co-propiedad explícita en vez de
	// transferencia).
	RolResultanteDelCedente string
	Origen                  dominio.OrigenSolicitud
}

// --- Comandos: invitaciones ---------------------------------------------------

// ComandoInvitarMiembro transporta la entrada del caso de uso InvitarMiembro
// (§3.5 del diseño).
type ComandoInvitarMiembro struct {
	IDOrganizacion string
	IDSujeto       string
	Correo         string
	Rol            string
	Origen         dominio.OrigenSolicitud
}

// ComandoRevocarInvitacion transporta la entrada del caso de uso
// RevocarInvitacion (§3.5 del diseño).
type ComandoRevocarInvitacion struct {
	IDOrganizacion string
	IDSujeto       string
	IDInvitacion   string
	Origen         dominio.OrigenSolicitud
}

// ComandoAceptarInvitacion transporta la entrada del caso de uso
// AceptarInvitacion (§3.5 del diseño). TokenPlano es el secreto en claro
// presentado por el destinatario; nunca se registra en logs (INV-TEN-23).
type ComandoAceptarInvitacion struct {
	TokenPlano string
	IDSujeto   string // del token de acceso ya validado: aceptar exige estar autenticado
	Origen     dominio.OrigenSolicitud
}

// --- Autorización ---------------------------------------------------------------

// ConsultaAutorizacion transporta la entrada de
// VerificadorDeAutorizacion.Autorizar (§3.7 del diseño): el camino caliente
// del sistema. Origen viaja aquí (adición sobre el diseño original) porque
// AutorizarCasoDeUso audita cada denegación (INV-TEN-25) y esa auditoría es
// precisamente la señal pensada para detectar intentos de escalada — sin
// IP/agente de quien la disparó, pierde la mitad de su valor forense. El
// middleware de autorización de cualquier contexto consumidor ya tiene el
// OrigenSolicitud de la petición (lo construyó para autenticar), así que
// completar este campo no le cuesta nada nuevo.
type ConsultaAutorizacion struct {
	IDUsuario      string // SIEMPRE el `sub` de un token ya validado por Acceso
	IDOrganizacion string // SIEMPRE explícito: nunca sale del token ni de la sesión
	Permiso        string // catálogo cerrado de dominio.Permiso
	Origen         dominio.OrigenSolicitud
}

// Autorizacion es la respuesta del contrato público. Permitido y Motivo van
// separados a propósito: el consumidor necesita distinguir "no sos miembro"
// (404) de "sos miembro pero no alcanza" (403), y esa distinción no puede
// deducirse de un simple bool.
type Autorizacion struct {
	Permitido      bool
	IDOrganizacion string
	Rol            string // vacío si no hay membresía
	Motivo         string // "" | "sin_membresia" | "membresia_suspendida" |
	// "organizacion_no_operativa" | "rol_insuficiente"
}

// ConsultaRolEnOrganizacion transporta la entrada de
// ConsultorDeMembresias.RolEnOrganizacion.
type ConsultaRolEnOrganizacion struct {
	IDUsuario      string
	IDOrganizacion string
}

// VistaRolEfectivo es la salida de ConsultorDeMembresias.RolEnOrganizacion:
// los hechos crudos de la membresía (si existe) y de la organización, sin
// aplicar la matriz de permisos.
type VistaRolEfectivo struct {
	EsMiembro          bool
	Rol                string
	EstadoMembresia    string
	EstadoOrganizacion string
}

// --- Modelos de lectura -----------------------------------------------------------

// ConsultaOrganizacionPorID transporta la entrada del caso de uso
// ObtenerOrganizacion (§3.6 del diseño).
type ConsultaOrganizacionPorID struct {
	IDOrganizacion string
	IDSujeto       string
	Origen         dominio.OrigenSolicitud
}

// ConsultaOrganizacionesDeUsuario transporta la entrada del caso de uso
// ListarMisOrganizaciones (§3.6 del diseño).
type ConsultaOrganizacionesDeUsuario struct {
	IDUsuario        string
	IncluirNoActivas bool
}

// ConsultaMiembrosDeOrganizacion transporta la entrada del caso de uso
// ListarMiembros (§3.6 del diseño).
type ConsultaMiembrosDeOrganizacion struct {
	IDOrganizacion string
	IDSujeto       string
	Origen         dominio.OrigenSolicitud
}

// VistaOrganizacion es un modelo de LECTURA de una organización.
type VistaOrganizacion struct {
	ID              string
	Alias           string
	Nombre          string
	Estado          string
	CreadaEn        time.Time
	ActualizadaEn   time.Time
	MiembrosActivos int
}

// VistaMiembro es un modelo de LECTURA. Deliberadamente NO trae el correo ni
// el nombre del usuario: Tenencia no los conoce (INV-TEN-29) y componerlos
// exigiría una llamada por miembro a ConsultorDeUsuarios de Identidad. La
// composición, si el producto la necesita, es trabajo del BFF/adaptador HTTP,
// no del caso de uso.
type VistaMiembro struct {
	IDMembresia string
	IDUsuario   string
	Rol         string
	Estado      string
	CreadaEn    time.Time
}

// VistaMembresiaDeUsuario es un modelo de LECTURA para la salida de
// ConsultorDeMembresias.ListarOrganizacionesDeUsuario: una fila por
// organización a la que el usuario pertenece, con el rol y los dos estados
// relevantes ya resueltos.
type VistaMembresiaDeUsuario struct {
	IDOrganizacion     string
	Alias              string
	Nombre             string
	Rol                string
	EstadoMembresia    string
	EstadoOrganizacion string
}

// VistaInvitacion es un modelo de LECTURA de una invitación. Sin token: el
// valor en claro NUNCA sale por HTTP (INV-TEN-23).
type VistaInvitacion struct {
	ID             string
	IDOrganizacion string
	Destinatario   string
	Rol            string
	Estado         string
	CreadaEn       time.Time
	ExpiraEn       time.Time
}
