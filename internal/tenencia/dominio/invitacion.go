package dominio

import "time"

// Invitacion es el agregado raíz del contexto Tenencia que representa una
// invitación por correo a unirse a una Organizacion con un Rol propuesto.
// Ciclo de vida corto, con token opaco de un solo uso (ADR candidato 0033).
// Toda mutación ocurre por un método de negocio; no hay campos exportados
// ni setters, y los getters devuelven copias de valores, nunca punteros
// internos (INV-TEN-11).
//
// Es un agregado propio y no "una Membresia en estado pendiente" (§0 del
// diseño, consecuencia no obvia #2): Tenencia no puede resolver un correo a
// un usuario_id sin construir un oráculo de enumeración de usuarios, así
// que la invitación vive independiente hasta que el propio invitado,
// autenticado, la redime — momento en el que el caso de uso crea la
// Membresia como un segundo agregado, en la misma transacción (§3.5 del
// diseño).
type Invitacion struct {
	id             IDInvitacion
	organizacionID IDOrganizacion
	destinatario   CorreoDestinatario
	rolPropuesto   Rol
	estado         EstadoInvitacion
	hashToken      HashTokenInvitacion
	invitadaPor    IDUsuario
	creadaEn       time.Time
	expiraEn       time.Time
	resueltaEn     *time.Time

	eventos []EventoDominio
}

// CrearInvitacion crea una nueva Invitacion en estado pendiente y acumula
// el evento MiembroInvitado (IDAgregado = IDInvitacion). expiraEn se
// calcula como creadaEn + politica.VigenciaInvitacion(). hashToken debe
// venir ya calculado por TokenInvitacionPlano.Hash(): el dominio nunca
// genera ni ve el token en claro (INV-TEN-23).
func CrearInvitacion(
	id IDInvitacion,
	organizacionID IDOrganizacion,
	destinatario CorreoDestinatario,
	rolPropuesto Rol,
	hashToken HashTokenInvitacion,
	invitadaPor IDUsuario,
	ahora time.Time,
	politica PoliticaOrganizacion,
) (*Invitacion, error) {
	if id.EsVacio() {
		return nil, &ErrIDInvitacionInvalido{Motivo: "no puede estar vacío"}
	}
	if organizacionID.EsVacio() {
		return nil, &ErrIDOrganizacionInvalido{Motivo: "no puede estar vacío"}
	}
	if destinatario.EsVacio() {
		return nil, &ErrCorreoDestinatarioInvalido{Motivo: "no puede estar vacío"}
	}
	if rolPropuesto.EsVacio() {
		return nil, &ErrRolInvalido{Valor: ""}
	}
	if hashToken.EsVacio() {
		return nil, &ErrHashTokenInvitacionInvalido{Motivo: "no puede estar vacío"}
	}
	if invitadaPor.EsVacio() {
		return nil, &ErrIDUsuarioInvalido{Motivo: "no puede estar vacío"}
	}
	inv := &Invitacion{
		id:             id,
		organizacionID: organizacionID,
		destinatario:   destinatario,
		rolPropuesto:   rolPropuesto,
		estado:         EstadoInvitacionPendiente,
		hashToken:      hashToken,
		invitadaPor:    invitadaPor,
		creadaEn:       ahora,
		expiraEn:       ahora.Add(politica.VigenciaInvitacion()),
	}
	inv.agregarEvento(NuevoMiembroInvitado(id, organizacionID, destinatario, rolPropuesto, ahora))
	return inv, nil
}

// ReconstituirInvitacion reconstruye un agregado Invitacion a partir de
// datos ya validados y persistidos. A diferencia de CrearInvitacion, no
// acumula eventos: no representa una operación de negocio nueva, sino la
// rehidratación de una ya ocurrida.
func ReconstituirInvitacion(
	id IDInvitacion,
	organizacionID IDOrganizacion,
	destinatario CorreoDestinatario,
	rolPropuesto Rol,
	estado EstadoInvitacion,
	hashToken HashTokenInvitacion,
	invitadaPor IDUsuario,
	creadaEn time.Time,
	expiraEn time.Time,
	resueltaEn *time.Time,
) *Invitacion {
	inv := &Invitacion{
		id:             id,
		organizacionID: organizacionID,
		destinatario:   destinatario,
		rolPropuesto:   rolPropuesto,
		estado:         estado,
		hashToken:      hashToken,
		invitadaPor:    invitadaPor,
		creadaEn:       creadaEn,
		expiraEn:       expiraEn,
	}
	if resueltaEn != nil {
		copia := *resueltaEn
		inv.resueltaEn = &copia
	}
	return inv
}

// --- getters (copias de valor, nunca punteros internos: INV-TEN-11) --------

// ID devuelve el identificador administrativo de la invitación. No es el
// token.
func (inv *Invitacion) ID() IDInvitacion { return inv.id }

// OrganizacionID devuelve el identificador de la organización a la que
// invita.
func (inv *Invitacion) OrganizacionID() IDOrganizacion { return inv.organizacionID }

// Destinatario devuelve el correo normalizado del destinatario.
func (inv *Invitacion) Destinatario() CorreoDestinatario { return inv.destinatario }

// RolPropuesto devuelve el rol que tendrá el destinatario si acepta.
func (inv *Invitacion) RolPropuesto() Rol { return inv.rolPropuesto }

// Estado devuelve el estado actual del ciclo de vida de la invitación.
func (inv *Invitacion) Estado() EstadoInvitacion { return inv.estado }

// HashToken devuelve el hash SHA-256 del token de invitación. El valor en
// claro nunca se persiste ni se expone por este agregado (INV-TEN-23).
func (inv *Invitacion) HashToken() HashTokenInvitacion { return inv.hashToken }

// InvitadaPor devuelve el identificador de quien emitió la invitación.
func (inv *Invitacion) InvitadaPor() IDUsuario { return inv.invitadaPor }

// CreadaEn devuelve la marca de tiempo de creación de la invitación.
func (inv *Invitacion) CreadaEn() time.Time { return inv.creadaEn }

// ExpiraEn devuelve el instante en que la invitación deja de ser vigente si
// no se resuelve antes.
func (inv *Invitacion) ExpiraEn() time.Time { return inv.expiraEn }

// ResueltaEn devuelve la marca de tiempo en que la invitación se resolvió
// (aceptada, revocada o expirada), y un booleano que indica si ya se
// resolvió.
func (inv *Invitacion) ResueltaEn() (time.Time, bool) {
	if inv.resueltaEn == nil {
		return time.Time{}, false
	}
	return *inv.resueltaEn, true
}

// --- mutaciones de negocio --------------------------------------------------

// EstaVigente indica si, a la hora dada, la invitación sigue pendiente y
// dentro de su ventana de expiración. Es la comprobación que el caso de uso
// AceptarInvitacion invoca antes de redimir el token.
func (inv *Invitacion) EstaVigente(ahora time.Time) bool {
	return inv.estado.EsIgual(EstadoInvitacionPendiente) && ahora.Before(inv.expiraEn)
}

// Aceptar transiciona la invitación de pendiente a aceptada (§1.4 del
// diseño: MaquinaEstadosInvitacion) y acumula InvitacionResuelta con
// Desenlace=DesenlaceAceptada, Resultado=ResultadoExito. Aplica INV-TEN-21:
// el correo del sujeto que acepta debe coincidir, en tiempo constante, con
// el destinatario. Cualquier fallo (invitación no vigente, correo distinto)
// devuelve un error sin mutar el agregado ni acumular evento: la auditoría
// del intento fallido (con su propio desenlace) es responsabilidad del caso
// de uso, que no siempre tiene un agregado que cargar (p. ej. hash
// desconocido), exactamente el mismo criterio que RenovacionRechazada en
// acceso/dominio.
func (inv *Invitacion) Aceptar(correoDelSujeto CorreoDestinatario, ahora time.Time) error {
	if !inv.EstaVigente(ahora) {
		return &ErrInvitacionInvalida{}
	}
	if !inv.destinatario.EsIgualConstante(correoDelSujeto) {
		return &ErrInvitacionAjena{}
	}
	inv.estado = EstadoInvitacionAceptada
	resueltaEn := ahora
	inv.resueltaEn = &resueltaEn
	inv.agregarEvento(NuevoInvitacionResuelta(inv.id.String(), inv.organizacionID.String(), DesenlaceAceptada, ResultadoExito, ahora))
	return nil
}

// Revocar transiciona la invitación de pendiente a revocada y acumula
// InvitacionResuelta con Desenlace=DesenlaceRevocada, Resultado=ResultadoExito.
// Sin DELETE (§6 del diseño): la invitación resuelta es evidencia forense.
func (inv *Invitacion) Revocar(ahora time.Time) error {
	if !inv.estado.PuedeTransicionarA(EstadoInvitacionRevocada) {
		return &ErrTransicionEstadoInvitacionInvalida{Origen: inv.estado, Destino: EstadoInvitacionRevocada}
	}
	inv.estado = EstadoInvitacionRevocada
	resueltaEn := ahora
	inv.resueltaEn = &resueltaEn
	inv.agregarEvento(NuevoInvitacionResuelta(inv.id.String(), inv.organizacionID.String(), DesenlaceRevocada, ResultadoExito, ahora))
	return nil
}

// MarcarExpirada transiciona la invitación de pendiente a expirada y
// acumula InvitacionResuelta con Desenlace=DesenlaceExpirada,
// Resultado=ResultadoExito. Es la detección perezosa de expiración: el
// caso de uso la invoca cuando encuentra una invitación pendiente cuya
// ventana ya se agotó, no hay un job de fondo que expire proactivamente.
func (inv *Invitacion) MarcarExpirada(ahora time.Time) error {
	if !inv.estado.PuedeTransicionarA(EstadoInvitacionExpirada) {
		return &ErrTransicionEstadoInvitacionInvalida{Origen: inv.estado, Destino: EstadoInvitacionExpirada}
	}
	inv.estado = EstadoInvitacionExpirada
	resueltaEn := ahora
	inv.resueltaEn = &resueltaEn
	inv.agregarEvento(NuevoInvitacionResuelta(inv.id.String(), inv.organizacionID.String(), DesenlaceExpirada, ResultadoExito, ahora))
	return nil
}

// --- eventos ------------------------------------------------------------

// EventosPendientes drena los eventos acumulados por el agregado: los
// devuelve y vacía el buffer interno. El caso de uso debe llamarlo una sola
// vez, tras persistir el agregado dentro de la misma UnidadDeTrabajo que la
// auditoría (ADR 0005, INV-TEN-25).
func (inv *Invitacion) EventosPendientes() []EventoDominio {
	eventos := inv.eventos
	inv.eventos = nil
	return eventos
}

func (inv *Invitacion) agregarEvento(e EventoDominio) {
	inv.eventos = append(inv.eventos, e)
}
