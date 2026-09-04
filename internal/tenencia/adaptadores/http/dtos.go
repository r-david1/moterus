package http

import "time"

// Los DTOs de este archivo son la única frontera de serialización HTTP del
// contexto Tenencia. Nunca envuelven los agregados de dominio.Organizacion/
// Membresia/Invitacion directamente: se proyectan los VistaX de
// tenencia/puertos, que ya son modelos de lectura (ADR 0006: Huma genera el
// OpenAPI y valida a partir de estos tags, no se mantiene una spec a mano
// ni se usa go-playground/validator).

// --- POST /tenencia/organizaciones -------------------------------------------

type crearOrganizacionPeticion struct {
	Nombre string `json:"nombre" minLength:"1" maxLength:"120" doc:"Nombre de presentación de la organización."`
	Alias  string `json:"alias" minLength:"3" maxLength:"48" doc:"Identificador único de la organización (minúsculas, [a-z0-9-])." example:"acme-corp"`
}

// CrearOrganizacionInput es el input Huma de la creación de organización.
type CrearOrganizacionInput struct {
	Body crearOrganizacionPeticion
}

// vistaOrganizacionRespuesta es la proyección 1:1 de puertos.VistaOrganizacion.
type vistaOrganizacionRespuesta struct {
	ID              string    `json:"id"`
	Alias           string    `json:"alias"`
	Nombre          string    `json:"nombre"`
	Estado          string    `json:"estado"`
	CreadaEn        time.Time `json:"creada_en"`
	ActualizadaEn   time.Time `json:"actualizada_en"`
	MiembrosActivos int       `json:"miembros_activos"`
}

// CrearOrganizacionOutput es el output Huma de la creación de organización.
type CrearOrganizacionOutput struct {
	Body vistaOrganizacionRespuesta
}

// --- GET /tenencia/organizaciones (las propias) ------------------------------

// ListarMisOrganizacionesInput es el input Huma de la consulta de las
// organizaciones propias.
type ListarMisOrganizacionesInput struct {
	IncluirNoActivas bool `query:"incluir_no_activas" doc:"Si es true, incluye organizaciones/membresías suspendidas o archivadas."`
}

// vistaMembresiaDeUsuarioRespuesta es la proyección 1:1 de
// puertos.VistaMembresiaDeUsuario.
type vistaMembresiaDeUsuarioRespuesta struct {
	IDOrganizacion     string `json:"id_organizacion"`
	Alias              string `json:"alias"`
	Nombre             string `json:"nombre"`
	Rol                string `json:"rol"`
	EstadoMembresia    string `json:"estado_membresia"`
	EstadoOrganizacion string `json:"estado_organizacion"`
}

// ListarMisOrganizacionesOutput es el output Huma de la consulta.
type ListarMisOrganizacionesOutput struct {
	Body []vistaMembresiaDeUsuarioRespuesta
}

// --- GET /tenencia/organizaciones/{idOrganizacion} ---------------------------

// ObtenerOrganizacionInput es el input Huma de la consulta de una
// organización por ID.
type ObtenerOrganizacionInput struct {
	IDOrganizacion string `path:"idOrganizacion" format:"uuid" doc:"Identificador (UUID) de la organización."`
}

// ObtenerOrganizacionOutput es el output Huma de la consulta.
type ObtenerOrganizacionOutput struct {
	Body vistaOrganizacionRespuesta
}

// --- PATCH /tenencia/organizaciones/{idOrganizacion} -------------------------

type actualizarOrganizacionPeticion struct {
	Nombre *string `json:"nombre,omitempty" maxLength:"120" doc:"Nuevo nombre de presentación. Ausente = no cambiar."`
	Alias  *string `json:"alias,omitempty" maxLength:"48" doc:"Nuevo alias único. Ausente = no cambiar."`
}

// ActualizarOrganizacionInput es el input Huma de la actualización.
type ActualizarOrganizacionInput struct {
	IDOrganizacion string `path:"idOrganizacion" format:"uuid"`
	Body           actualizarOrganizacionPeticion
}

// ActualizarOrganizacionOutput es el output Huma de la actualización.
type ActualizarOrganizacionOutput struct {
	Body vistaOrganizacionRespuesta
}

// --- POST /tenencia/organizaciones/{idOrganizacion}/cambios-estado ----------

type cambiarEstadoOrganizacionPeticion struct {
	Destino string `json:"destino" enum:"activa,suspendida,archivada" doc:"Estado destino de la transición."`
	Motivo  string `json:"motivo,omitempty" maxLength:"280" doc:"Motivo del cambio; obligatorio para suspender o archivar."`
}

// CambiarEstadoOrganizacionInput es el input Huma del cambio de estado.
type CambiarEstadoOrganizacionInput struct {
	IDOrganizacion string `path:"idOrganizacion" format:"uuid"`
	Body           cambiarEstadoOrganizacionPeticion
}

// CambiarEstadoOrganizacionOutput es el output Huma del cambio de estado.
type CambiarEstadoOrganizacionOutput struct {
	Body vistaOrganizacionRespuesta
}

// --- GET /tenencia/organizaciones/{idOrganizacion}/miembros ------------------

// ListarMiembrosInput es el input Huma del listado de miembros.
type ListarMiembrosInput struct {
	IDOrganizacion string `path:"idOrganizacion" format:"uuid"`
}

// vistaMiembroRespuesta es la proyección 1:1 de puertos.VistaMiembro.
type vistaMiembroRespuesta struct {
	IDMembresia string    `json:"id_membresia"`
	IDUsuario   string    `json:"id_usuario"`
	Rol         string    `json:"rol"`
	Estado      string    `json:"estado"`
	CreadaEn    time.Time `json:"creada_en"`
}

// ListarMiembrosOutput es el output Huma del listado de miembros.
type ListarMiembrosOutput struct {
	Body []vistaMiembroRespuesta
}

// --- POST /tenencia/organizaciones/{idOrganizacion}/miembros (alta directa) -

type agregarMiembroPeticion struct {
	IDUsuario string `json:"usuario_id" format:"uuid" doc:"Identificador del usuario a agregar; debe existir y estar activo en Identidad."`
	Rol       string `json:"rol" enum:"propietario,administrador,miembro" doc:"Rol otorgado, sujeto a la regla de dominancia."`
}

// AgregarMiembroInput es el input Huma del alta directa de un miembro.
type AgregarMiembroInput struct {
	IDOrganizacion string `path:"idOrganizacion" format:"uuid"`
	Body           agregarMiembroPeticion
}

// AgregarMiembroOutput es el output Huma del alta directa.
type AgregarMiembroOutput struct {
	Body vistaMiembroRespuesta
}

// --- PATCH /tenencia/organizaciones/{idOrganizacion}/miembros/{idUsuario} --

type cambiarRolPeticion struct {
	NuevoRol string `json:"rol" enum:"propietario,administrador,miembro" doc:"Rol nuevo, sujeto a la regla de dominancia (INV-TEN-20)."`
}

// CambiarRolInput es el input Huma del cambio de rol.
type CambiarRolInput struct {
	IDOrganizacion string `path:"idOrganizacion" format:"uuid"`
	IDUsuario      string `path:"idUsuario" format:"uuid"`
	Body           cambiarRolPeticion
}

// CambiarRolOutput es el output Huma del cambio de rol.
type CambiarRolOutput struct {
	Body vistaMiembroRespuesta
}

// --- DELETE /tenencia/organizaciones/{idOrganizacion}/miembros/{idUsuario} -

// RemoverMiembroInput es el input Huma de la remoción de un miembro.
type RemoverMiembroInput struct {
	IDOrganizacion string `path:"idOrganizacion" format:"uuid"`
	IDUsuario      string `path:"idUsuario" format:"uuid"`
}

// RemoverMiembroOutput es el output Huma. Cuerpo vacío (204).
type RemoverMiembroOutput struct{}

// --- DELETE /tenencia/organizaciones/{idOrganizacion}/miembros/actual ------

// AbandonarOrganizacionInput es el input Huma de abandonar la organización.
type AbandonarOrganizacionInput struct {
	IDOrganizacion string `path:"idOrganizacion" format:"uuid"`
}

// AbandonarOrganizacionOutput es el output Huma. Cuerpo vacío (204).
type AbandonarOrganizacionOutput struct{}

// --- POST .../transferencias-propiedad --------------------------------------

type transferirPropiedadPeticion struct {
	IDNuevoPropietario      string `json:"nuevo_propietario_id" format:"uuid" doc:"Debe ser miembro activo previo de la organización."`
	RolResultanteDelCedente string `json:"rol_resultante_del_cedente,omitempty" enum:"administrador,miembro," doc:"Rol del cedente tras transferir. Vacío = conservar propietario (co-propiedad)."`
}

// TransferirPropiedadInput es el input Huma de la transferencia de
// propiedad.
type TransferirPropiedadInput struct {
	IDOrganizacion string `path:"idOrganizacion" format:"uuid"`
	Body           transferirPropiedadPeticion
}

// TransferirPropiedadOutput es el output Huma. Cuerpo vacío (204).
type TransferirPropiedadOutput struct{}

// --- POST .../invitaciones ----------------------------------------------------

type invitarMiembroPeticion struct {
	Correo string `json:"correo" format:"email" doc:"Correo del destinatario de la invitación."`
	Rol    string `json:"rol" enum:"propietario,administrador,miembro" doc:"Rol propuesto, sujeto a la regla de dominancia."`
}

// InvitarMiembroInput es el input Huma de la invitación.
type InvitarMiembroInput struct {
	IDOrganizacion string `path:"idOrganizacion" format:"uuid"`
	Body           invitarMiembroPeticion
}

// vistaInvitacionRespuesta es la proyección 1:1 de puertos.VistaInvitacion.
// Deliberadamente SIN el token: el valor en claro nunca sale por HTTP
// (INV-TEN-23).
type vistaInvitacionRespuesta struct {
	ID             string    `json:"id"`
	IDOrganizacion string    `json:"id_organizacion"`
	Destinatario   string    `json:"destinatario"`
	Rol            string    `json:"rol"`
	Estado         string    `json:"estado"`
	CreadaEn       time.Time `json:"creada_en"`
	ExpiraEn       time.Time `json:"expira_en"`
}

// InvitarMiembroOutput es el output Huma de la invitación.
type InvitarMiembroOutput struct {
	Body vistaInvitacionRespuesta
}

// --- DELETE .../invitaciones/{idInvitacion} ----------------------------------

// RevocarInvitacionInput es el input Huma de la revocación.
type RevocarInvitacionInput struct {
	IDOrganizacion string `path:"idOrganizacion" format:"uuid"`
	IDInvitacion   string `path:"idInvitacion" format:"uuid"`
}

// RevocarInvitacionOutput es el output Huma. Cuerpo vacío (204).
type RevocarInvitacionOutput struct{}

// --- POST /tenencia/invitaciones/aceptaciones --------------------------------

type aceptarInvitacionPeticion struct {
	Token string `json:"token" minLength:"1" doc:"Token de invitación en claro recibido por correo (prefijo mot_inv_)."`
}

// AceptarInvitacionInput es el input Huma de la aceptación.
type AceptarInvitacionInput struct {
	Body aceptarInvitacionPeticion
}

// AceptarInvitacionOutput es el output Huma de la aceptación.
type AceptarInvitacionOutput struct {
	Body vistaMiembroRespuesta
}
