package dominio

import "time"

// EventoDominio es el contrato que implementan los eventos que el contexto
// Tenencia audita y publica. Mismo contrato que identidad/dominio.EventoDominio
// y acceso/dominio.EventoDominio (NombreEvento, OcurridoEn, IDAgregado),
// redeclarado aquí a propósito (§1.6 y §1.7 del diseño; INV-TEN-27):
// tenencia/dominio no puede importar identidad/dominio ni acceso/dominio.
//
// Igual que en Acceso, no todos estos eventos los acumula un agregado
// internamente: los que representan una transición incondicional y
// autocontenida de un agregado (OrganizacionCreada, OrganizacionActualizada,
// EstadoOrganizacionCambiado, MiembroAgregado, RolDeMiembroCambiado,
// EstadoMembresiaCambiado, MiembroRemovido, MiembroInvitado,
// InvitacionResuelta cuando el desenlace es aceptar/revocar/expirar) se
// acumulan en Organizacion, Membresia o Invitacion y se drenan con
// EventosPendientes tras persistir. AutorizacionDenegada y los desenlaces de
// InvitacionResuelta con resultado "fallo" dependen de contexto de
// orquestación que ningún agregado tiene en el momento de la denegación (no
// hay membresía que cargar, o el token no resolvió a ninguna invitación), y
// los construye tenencia/aplicacion directamente con los constructores NuevoX
// de este archivo. Ningún evento transporta el token de invitación en claro
// ni su hash (INV-TEN-23, verificable con un test de dominio que serializa
// cada evento y busca esos campos).
type EventoDominio interface {
	// NombreEvento identifica el tipo de evento (p. ej. "OrganizacionCreada").
	NombreEvento() string
	// OcurridoEn indica cuándo ocurrió el evento, siempre con la hora
	// provista por el puerto Reloj (INV-TEN-14): el dominio nunca llama a
	// time.Now().
	OcurridoEn() time.Time
	// IDAgregado identifica al agregado relacionado (IDOrganizacion,
	// IDMembresia o IDInvitacion según el evento).
	IDAgregado() string
}

// OrganizacionCreada se emite cuando una Organizacion nueva nace en estado
// activa, junto con la membresía propietario de su fundador (accion de
// auditoría: organizacion.creada).
type OrganizacionCreada struct {
	IDOrganizacion string
	Alias          string
	CreadaPor      string
	ocurridoEn     time.Time
}

// NuevoOrganizacionCreada construye el evento OrganizacionCreada.
func NuevoOrganizacionCreada(id IDOrganizacion, alias AliasOrganizacion, creadaPor IDUsuario, ocurridoEn time.Time) OrganizacionCreada {
	return OrganizacionCreada{
		IDOrganizacion: id.String(),
		Alias:          alias.Normalizado(),
		CreadaPor:      creadaPor.String(),
		ocurridoEn:     ocurridoEn,
	}
}

func (e OrganizacionCreada) NombreEvento() string  { return "OrganizacionCreada" }
func (e OrganizacionCreada) OcurridoEn() time.Time { return e.ocurridoEn }
func (e OrganizacionCreada) IDAgregado() string    { return e.IDOrganizacion }

// OrganizacionActualizada se emite al renombrar o cambiar el alias de una
// organización (accion de auditoría: organizacion.actualizada). Campos
// lleva la lista de campos modificados ("nombre" y/o "alias"), nunca los
// valores viejos y nuevos completos.
type OrganizacionActualizada struct {
	IDOrganizacion string
	Campos         []string
	ocurridoEn     time.Time
}

// NuevoOrganizacionActualizada construye el evento OrganizacionActualizada.
func NuevoOrganizacionActualizada(id IDOrganizacion, campos []string, ocurridoEn time.Time) OrganizacionActualizada {
	copia := make([]string, len(campos))
	copy(copia, campos)
	return OrganizacionActualizada{
		IDOrganizacion: id.String(),
		Campos:         copia,
		ocurridoEn:     ocurridoEn,
	}
}

func (e OrganizacionActualizada) NombreEvento() string  { return "OrganizacionActualizada" }
func (e OrganizacionActualizada) OcurridoEn() time.Time { return e.ocurridoEn }
func (e OrganizacionActualizada) IDAgregado() string    { return e.IDOrganizacion }

// EstadoOrganizacionCambiado se emite al suspender, reactivar o archivar
// una organización (accion de auditoría: organizacion.estado_cambiado).
type EstadoOrganizacionCambiado struct {
	IDOrganizacion string
	Origen         string
	Destino        string
	Motivo         string
	ocurridoEn     time.Time
}

// NuevoEstadoOrganizacionCambiado construye el evento EstadoOrganizacionCambiado.
func NuevoEstadoOrganizacionCambiado(id IDOrganizacion, origen, destino EstadoOrganizacion, motivo string, ocurridoEn time.Time) EstadoOrganizacionCambiado {
	return EstadoOrganizacionCambiado{
		IDOrganizacion: id.String(),
		Origen:         origen.String(),
		Destino:        destino.String(),
		Motivo:         motivo,
		ocurridoEn:     ocurridoEn,
	}
}

func (e EstadoOrganizacionCambiado) NombreEvento() string  { return "EstadoOrganizacionCambiado" }
func (e EstadoOrganizacionCambiado) OcurridoEn() time.Time { return e.ocurridoEn }
func (e EstadoOrganizacionCambiado) IDAgregado() string    { return e.IDOrganizacion }

// Catálogo cerrado del campo Via de MiembroAgregado.
const (
	// ViaFundacion identifica la membresía propietario creada junto con la
	// organización (INV-TEN-03).
	ViaFundacion = "fundacion"
	// ViaInvitacion identifica una membresía nacida de aceptar una
	// invitación.
	ViaInvitacion = "invitacion"
	// ViaAltaDirecta identifica una membresía creada por un administrador
	// que ya conocía el usuario_id (§3.3 del diseño).
	ViaAltaDirecta = "alta_directa"
)

// MiembroAgregado se emite cuando nace una Membresia nueva, cubriendo tanto
// la fundación de la organización como la alta directa y la que resulta de
// aceptar una invitación (accion de auditoría: membresia.creada).
type MiembroAgregado struct {
	IDMembresia    string
	IDOrganizacion string
	IDUsuario      string
	Rol            string
	Via            string
	ocurridoEn     time.Time
}

// NuevoMiembroAgregado construye el evento MiembroAgregado.
func NuevoMiembroAgregado(id IDMembresia, organizacionID IDOrganizacion, usuarioID IDUsuario, rol Rol, via string, ocurridoEn time.Time) MiembroAgregado {
	return MiembroAgregado{
		IDMembresia:    id.String(),
		IDOrganizacion: organizacionID.String(),
		IDUsuario:      usuarioID.String(),
		Rol:            rol.Valor(),
		Via:            via,
		ocurridoEn:     ocurridoEn,
	}
}

func (e MiembroAgregado) NombreEvento() string  { return "MiembroAgregado" }
func (e MiembroAgregado) OcurridoEn() time.Time { return e.ocurridoEn }
func (e MiembroAgregado) IDAgregado() string    { return e.IDMembresia }

// RolDeMiembroCambiado se emite cuando cambia el rol de un miembro
// (incluida la transferencia de propiedad), accion de auditoría:
// membresia.rol_cambiado. Es el dato más pedido en cualquier auditoría de
// accesos.
type RolDeMiembroCambiado struct {
	IDMembresia    string
	IDOrganizacion string
	IDUsuario      string
	RolAnterior    string
	RolNuevo       string
	ocurridoEn     time.Time
}

// NuevoRolDeMiembroCambiado construye el evento RolDeMiembroCambiado.
func NuevoRolDeMiembroCambiado(id IDMembresia, organizacionID IDOrganizacion, usuarioID IDUsuario, rolAnterior, rolNuevo Rol, ocurridoEn time.Time) RolDeMiembroCambiado {
	return RolDeMiembroCambiado{
		IDMembresia:    id.String(),
		IDOrganizacion: organizacionID.String(),
		IDUsuario:      usuarioID.String(),
		RolAnterior:    rolAnterior.Valor(),
		RolNuevo:       rolNuevo.Valor(),
		ocurridoEn:     ocurridoEn,
	}
}

func (e RolDeMiembroCambiado) NombreEvento() string  { return "RolDeMiembroCambiado" }
func (e RolDeMiembroCambiado) OcurridoEn() time.Time { return e.ocurridoEn }
func (e RolDeMiembroCambiado) IDAgregado() string    { return e.IDMembresia }

// EstadoMembresiaCambiado se emite al suspender o reactivar una membresía
// sin removerla (accion de auditoría: membresia.estado_cambiado).
type EstadoMembresiaCambiado struct {
	IDMembresia    string
	IDOrganizacion string
	IDUsuario      string
	Origen         string
	Destino        string
	ocurridoEn     time.Time
}

// NuevoEstadoMembresiaCambiado construye el evento EstadoMembresiaCambiado.
func NuevoEstadoMembresiaCambiado(id IDMembresia, organizacionID IDOrganizacion, usuarioID IDUsuario, origen, destino EstadoMembresia, ocurridoEn time.Time) EstadoMembresiaCambiado {
	return EstadoMembresiaCambiado{
		IDMembresia:    id.String(),
		IDOrganizacion: organizacionID.String(),
		IDUsuario:      usuarioID.String(),
		Origen:         origen.String(),
		Destino:        destino.String(),
		ocurridoEn:     ocurridoEn,
	}
}

func (e EstadoMembresiaCambiado) NombreEvento() string  { return "EstadoMembresiaCambiado" }
func (e EstadoMembresiaCambiado) OcurridoEn() time.Time { return e.ocurridoEn }
func (e EstadoMembresiaCambiado) IDAgregado() string    { return e.IDMembresia }

// MiembroRemovido se emite cuando una membresía transiciona a removida, por
// decisión de un administrador o por iniciativa propia (accion de
// auditoría: membresia.removida).
type MiembroRemovido struct {
	IDMembresia         string
	IDOrganizacion      string
	IDUsuario           string
	RolAlRemover        string
	PorIniciativaPropia bool
	ocurridoEn          time.Time
}

// NuevoMiembroRemovido construye el evento MiembroRemovido.
func NuevoMiembroRemovido(id IDMembresia, organizacionID IDOrganizacion, usuarioID IDUsuario, rolAlRemover Rol, porIniciativaPropia bool, ocurridoEn time.Time) MiembroRemovido {
	return MiembroRemovido{
		IDMembresia:         id.String(),
		IDOrganizacion:      organizacionID.String(),
		IDUsuario:           usuarioID.String(),
		RolAlRemover:        rolAlRemover.Valor(),
		PorIniciativaPropia: porIniciativaPropia,
		ocurridoEn:          ocurridoEn,
	}
}

func (e MiembroRemovido) NombreEvento() string  { return "MiembroRemovido" }
func (e MiembroRemovido) OcurridoEn() time.Time { return e.ocurridoEn }
func (e MiembroRemovido) IDAgregado() string    { return e.IDMembresia }

// MiembroInvitado se emite al emitir una invitación por correo con un rol
// propuesto (accion de auditoría: membresia.invitada). IDAgregado es el
// IDInvitacion. Nunca lleva el token ni su hash (INV-TEN-23).
type MiembroInvitado struct {
	IDInvitacion       string
	IDOrganizacion     string
	CorreoDestinatario string
	RolPropuesto       string
	ocurridoEn         time.Time
}

// NuevoMiembroInvitado construye el evento MiembroInvitado.
func NuevoMiembroInvitado(id IDInvitacion, organizacionID IDOrganizacion, destinatario CorreoDestinatario, rolPropuesto Rol, ocurridoEn time.Time) MiembroInvitado {
	return MiembroInvitado{
		IDInvitacion:       id.String(),
		IDOrganizacion:     organizacionID.String(),
		CorreoDestinatario: destinatario.Normalizado(),
		RolPropuesto:       rolPropuesto.Valor(),
		ocurridoEn:         ocurridoEn,
	}
}

func (e MiembroInvitado) NombreEvento() string  { return "MiembroInvitado" }
func (e MiembroInvitado) OcurridoEn() time.Time { return e.ocurridoEn }
func (e MiembroInvitado) IDAgregado() string    { return e.IDInvitacion }

// Catálogo cerrado del campo Desenlace de InvitacionResuelta.
const (
	// DesenlaceAceptada: el destinatario redimió el token con éxito.
	DesenlaceAceptada = "aceptada"
	// DesenlaceRevocada: un administrador canceló la invitación.
	DesenlaceRevocada = "revocada"
	// DesenlaceExpirada: se agotó la ventana de vigencia.
	DesenlaceExpirada = "expirada"
	// DesenlaceIntentoFallido: un intento de redención no prosperó (token
	// desconocido, malformado, revocado, ya aceptado, expirado o de
	// destinatario distinto — INV-TEN-24).
	DesenlaceIntentoFallido = "intento_fallido"
)

// Catálogo cerrado del campo Resultado de InvitacionResuelta.
const (
	ResultadoExito = "exito"
	ResultadoFallo = "fallo"
)

// InvitacionResuelta es una sola acción de auditoría para los cuatro
// desenlaces posibles de una invitación — aceptación, revocación,
// expiración e intento fallido de redención —, distinguidos por Desenlace y
// Resultado (accion de auditoría: membresia.invitacion_resuelta). Mismo
// criterio de agregación que usuario.login y sesion.renovada en los otros
// contextos. IDAgregado va vacío cuando el token presentado no resolvió a
// ninguna invitación (no hay agregado que nombrar).
type InvitacionResuelta struct {
	IDInvitacion   string
	IDOrganizacion string
	Desenlace      string
	Resultado      string
	ocurridoEn     time.Time
}

// NuevoInvitacionResuelta construye el evento InvitacionResuelta.
// idInvitacion e idOrganizacion pueden ir vacíos (intento fallido sobre un
// token que no resolvió a ninguna invitación).
func NuevoInvitacionResuelta(idInvitacion, idOrganizacion, desenlace, resultado string, ocurridoEn time.Time) InvitacionResuelta {
	return InvitacionResuelta{
		IDInvitacion:   idInvitacion,
		IDOrganizacion: idOrganizacion,
		Desenlace:      desenlace,
		Resultado:      resultado,
		ocurridoEn:     ocurridoEn,
	}
}

func (e InvitacionResuelta) NombreEvento() string  { return "InvitacionResuelta" }
func (e InvitacionResuelta) OcurridoEn() time.Time { return e.ocurridoEn }
func (e InvitacionResuelta) IDAgregado() string    { return e.IDInvitacion }

// AutorizacionDenegada se emite cada vez que Autorizar deniega una
// petición (accion de auditoría: autorizacion.denegada, resultado
// denegado). No existe evento de autorización concedida: auditar cada
// concesión pondría el camino caliente de todo el sistema detrás del
// advisory lock de la cadena de hashes (INV-TEN-25).
type AutorizacionDenegada struct {
	IDUsuario      string
	IDOrganizacion string
	Permiso        string
	Motivo         string
	RolActual      string
	ocurridoEn     time.Time
}

// NuevoAutorizacionDenegada construye el evento AutorizacionDenegada.
// rolActual va vacío cuando el motivo es sin_membresia.
func NuevoAutorizacionDenegada(usuarioID IDUsuario, organizacionID IDOrganizacion, permiso Permiso, motivo MotivoDenegacion, rolActual string, ocurridoEn time.Time) AutorizacionDenegada {
	return AutorizacionDenegada{
		IDUsuario:      usuarioID.String(),
		IDOrganizacion: organizacionID.String(),
		Permiso:        permiso.Valor(),
		Motivo:         motivo.Valor(),
		RolActual:      rolActual,
		ocurridoEn:     ocurridoEn,
	}
}

func (e AutorizacionDenegada) NombreEvento() string  { return "AutorizacionDenegada" }
func (e AutorizacionDenegada) OcurridoEn() time.Time { return e.ocurridoEn }
func (e AutorizacionDenegada) IDAgregado() string    { return e.IDOrganizacion }
