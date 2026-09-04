package dominio

import "time"

// Membresia es el agregado raíz del contexto Tenencia que representa la
// pertenencia de un Usuario a una Organizacion con un Rol. Toda mutación
// ocurre por un método de negocio; no hay campos exportados ni setters, y
// los getters devuelven copias de valores, nunca punteros internos
// (INV-TEN-11).
//
// Es deliberadamente un agregado propio, NO una colección dentro de
// Organizacion (§1.2 del diseño). El costo explícito de esa decisión es
// INV-TEN-06 ("al menos un propietario activo"), un invariante entre
// agregados que este tipo no puede sostener por sí solo: los métodos que
// pueden reducir el conteo de propietarios (CambiarRol, Suspender, Remover,
// Abandonar) reciben ese conteo como PARÁMETRO — nunca lo consultan, para
// mantener el dominio sin E/S (INV-TEN-27) — y rechazan la operación con
// ErrUltimoPropietario si dejaría la organización en cero. La garantía
// completa (candado de fila + CONSTRAINT TRIGGER diferido) es
// responsabilidad de la capa de aplicación e infraestructura (§1.2, §6 del
// diseño).
type Membresia struct {
	id             IDMembresia
	organizacionID IDOrganizacion
	usuarioID      IDUsuario
	rol            Rol
	estado         EstadoMembresia
	otorgadaPor    *IDUsuario
	creadaEn       time.Time
	actualizadaEn  time.Time
	removidaEn     *time.Time

	eventos []EventoDominio
}

// nuevaMembresia valida los campos comunes a los tres constructores y
// construye el agregado en estado activa. No acumula evento: cada
// constructor público lo hace con el Via correspondiente.
func nuevaMembresia(id IDMembresia, organizacionID IDOrganizacion, usuarioID IDUsuario, rol Rol, ahora time.Time) (*Membresia, error) {
	if id.EsVacio() {
		return nil, &ErrIDMembresiaInvalido{Motivo: "no puede estar vacío"}
	}
	if organizacionID.EsVacio() {
		return nil, &ErrIDOrganizacionInvalido{Motivo: "no puede estar vacío"}
	}
	if usuarioID.EsVacio() {
		return nil, &ErrIDUsuarioInvalido{Motivo: "no puede estar vacío"}
	}
	if rol.EsVacio() {
		return nil, &ErrRolInvalido{Valor: ""}
	}
	return &Membresia{
		id:             id,
		organizacionID: organizacionID,
		usuarioID:      usuarioID,
		rol:            rol,
		estado:         EstadoMembresiaActiva,
		creadaEn:       ahora,
		actualizadaEn:  ahora,
	}, nil
}

// FundarMembresia crea la membresía propietario de quien funda una
// Organizacion (INV-TEN-03). otorgadaPor va vacío (nil): nadie "otorga" la
// membresía fundacional. Acumula MiembroAgregado con Via=ViaFundacion. El
// caso de uso CrearOrganizacion debe invocarla en la misma UnidadDeTrabajo
// que CrearOrganizacion (§3.1 del diseño).
func FundarMembresia(id IDMembresia, organizacionID IDOrganizacion, usuarioID IDUsuario, ahora time.Time) (*Membresia, error) {
	m, err := nuevaMembresia(id, organizacionID, usuarioID, RolPropietario, ahora)
	if err != nil {
		return nil, err
	}
	m.agregarEvento(NuevoMiembroAgregado(id, organizacionID, usuarioID, RolPropietario, ViaFundacion, ahora))
	return m, nil
}

// AgregarMiembro crea una membresía por alta directa: el ejecutor ya
// conocía el usuario_id (herramienta administrativa, migración de datos,
// futuro aprovisionamiento SCIM — §3.3 del diseño). otorgadaPor es
// obligatorio. Acumula MiembroAgregado con Via=ViaAltaDirecta.
func AgregarMiembro(id IDMembresia, organizacionID IDOrganizacion, usuarioID IDUsuario, rol Rol, otorgadaPor IDUsuario, ahora time.Time) (*Membresia, error) {
	m, err := nuevaMembresia(id, organizacionID, usuarioID, rol, ahora)
	if err != nil {
		return nil, err
	}
	if otorgadaPor.EsVacio() {
		return nil, &ErrIDUsuarioInvalido{Motivo: "otorgadaPor no puede estar vacío"}
	}
	copia := otorgadaPor
	m.otorgadaPor = &copia
	m.agregarEvento(NuevoMiembroAgregado(id, organizacionID, usuarioID, rol, ViaAltaDirecta, ahora))
	return m, nil
}

// CrearMembresiaDesdeInvitacion crea una membresía como consecuencia de
// aceptar una Invitacion (§3.5 del diseño, paso 8 de AceptarInvitacion).
// otorgadaPor identifica a quien invitó. Acumula MiembroAgregado con
// Via=ViaInvitacion. El caso de uso debe invocarla en la misma
// UnidadDeTrabajo que Invitacion.Aceptar.
func CrearMembresiaDesdeInvitacion(id IDMembresia, organizacionID IDOrganizacion, usuarioID IDUsuario, rol Rol, otorgadaPor IDUsuario, ahora time.Time) (*Membresia, error) {
	m, err := nuevaMembresia(id, organizacionID, usuarioID, rol, ahora)
	if err != nil {
		return nil, err
	}
	if otorgadaPor.EsVacio() {
		return nil, &ErrIDUsuarioInvalido{Motivo: "otorgadaPor no puede estar vacío"}
	}
	copia := otorgadaPor
	m.otorgadaPor = &copia
	m.agregarEvento(NuevoMiembroAgregado(id, organizacionID, usuarioID, rol, ViaInvitacion, ahora))
	return m, nil
}

// ReconstituirMembresia reconstruye un agregado Membresia a partir de datos
// ya validados y persistidos. A diferencia de los constructores anteriores,
// no acumula eventos: no representa una operación de negocio nueva, sino la
// rehidratación de una ya ocurrida. otorgadaPor y removidaEn pueden ir nil.
func ReconstituirMembresia(
	id IDMembresia,
	organizacionID IDOrganizacion,
	usuarioID IDUsuario,
	rol Rol,
	estado EstadoMembresia,
	otorgadaPor *IDUsuario,
	creadaEn time.Time,
	actualizadaEn time.Time,
	removidaEn *time.Time,
) *Membresia {
	m := &Membresia{
		id:             id,
		organizacionID: organizacionID,
		usuarioID:      usuarioID,
		rol:            rol,
		estado:         estado,
		creadaEn:       creadaEn,
		actualizadaEn:  actualizadaEn,
	}
	if otorgadaPor != nil {
		copia := *otorgadaPor
		m.otorgadaPor = &copia
	}
	if removidaEn != nil {
		copia := *removidaEn
		m.removidaEn = &copia
	}
	return m
}

// --- getters (copias de valor, nunca punteros internos: INV-TEN-11) --------

// ID devuelve el identificador de la membresía.
func (m *Membresia) ID() IDMembresia { return m.id }

// OrganizacionID devuelve el identificador de la organización a la que
// pertenece esta membresía.
func (m *Membresia) OrganizacionID() IDOrganizacion { return m.organizacionID }

// UsuarioID devuelve el identificador del usuario dueño de la membresía.
func (m *Membresia) UsuarioID() IDUsuario { return m.usuarioID }

// Rol devuelve el rol vigente de la membresía.
func (m *Membresia) Rol() Rol { return m.rol }

// Estado devuelve el estado actual del ciclo de vida de la membresía.
func (m *Membresia) Estado() EstadoMembresia { return m.estado }

// OtorgadaPor devuelve el identificador de quien otorgó esta membresía, y
// un booleano que indica si hay uno (vacío en la membresía fundacional,
// que nadie "otorga").
func (m *Membresia) OtorgadaPor() (IDUsuario, bool) {
	if m.otorgadaPor == nil {
		return IDUsuario{}, false
	}
	return *m.otorgadaPor, true
}

// CreadaEn devuelve la marca de tiempo de creación de la membresía.
func (m *Membresia) CreadaEn() time.Time { return m.creadaEn }

// ActualizadaEn devuelve la marca de tiempo de la última mutación del
// agregado (INV-TEN-14).
func (m *Membresia) ActualizadaEn() time.Time { return m.actualizadaEn }

// RemovidaEn devuelve la marca de tiempo de la remoción, y un booleano que
// indica si la membresía está removida.
func (m *Membresia) RemovidaEn() (time.Time, bool) {
	if m.removidaEn == nil {
		return time.Time{}, false
	}
	return *m.removidaEn, true
}

// --- mutaciones de negocio --------------------------------------------------

// CambiarRol cambia el rol de la membresía. Es un no-op idempotente (sin
// mutación ni evento) si nuevo es igual al rol vigente (§3.2 del diseño:
// mismo criterio que el logout idempotente de Acceso). En caso contrario
// aplica, en orden, la regla de dominancia completa (INV-TEN-20: nadie
// otorga un rol superior al propio, nadie modifica una membresía de rol
// superior al propio) y, si esta membresía es la propietaria activa que se
// está degradando, el invariante del último propietario (INV-TEN-06):
// propietariosActivos es el conteo ANTES de esta operación, provisto por el
// caso de uso bajo el candado de fila de la organización (§1.2, §3.2 del
// diseño) — el dominio nunca lo consulta por sí mismo.
func (m *Membresia) CambiarRol(nuevo Rol, ejecutor Rol, propietariosActivos int, ahora time.Time) error {
	if m.estado.EsIgual(EstadoMembresiaRemovida) {
		return &ErrTransicionEstadoMembresiaInvalida{Origen: m.estado, Destino: m.estado}
	}
	if nuevo.EsVacio() {
		return &ErrRolInvalido{Valor: ""}
	}
	if nuevo.EsIgual(m.rol) {
		return nil
	}
	if err := ValidarOtorgamiento(ejecutor, nuevo); err != nil {
		return err
	}
	if err := ValidarDominancia(ejecutor, m.rol); err != nil {
		return err
	}
	if m.dejariaOrganizacionSinPropietario(propietariosActivos) {
		return &ErrUltimoPropietario{}
	}
	anterior := m.rol
	m.rol = nuevo
	m.actualizadaEn = ahora
	m.agregarEvento(NuevoRolDeMiembroCambiado(m.id, m.organizacionID, m.usuarioID, anterior, nuevo, ahora))
	return nil
}

// Suspender transiciona la membresía de activa a suspendida (§1.4 del
// diseño: MaquinaEstadosMembresia). Aplica la regla de dominancia
// (INV-TEN-20) y el invariante del último propietario (INV-TEN-06): una
// membresía propietaria suspendida deja de contar como propietario activo.
func (m *Membresia) Suspender(ejecutor Rol, propietariosActivos int, ahora time.Time) error {
	if !m.estado.PuedeTransicionarA(EstadoMembresiaSuspendida) {
		return &ErrTransicionEstadoMembresiaInvalida{Origen: m.estado, Destino: EstadoMembresiaSuspendida}
	}
	if err := ValidarDominancia(ejecutor, m.rol); err != nil {
		return err
	}
	if m.dejariaOrganizacionSinPropietario(propietariosActivos) {
		return &ErrUltimoPropietario{}
	}
	return m.cambiarEstado(EstadoMembresiaSuspendida, ahora)
}

// Reactivar transiciona la membresía de suspendida a activa. No requiere
// conteo de propietarios: reactivar nunca reduce el conteo.
func (m *Membresia) Reactivar(ahora time.Time) error {
	if !m.estado.PuedeTransicionarA(EstadoMembresiaActiva) {
		return &ErrTransicionEstadoMembresiaInvalida{Origen: m.estado, Destino: EstadoMembresiaActiva}
	}
	return m.cambiarEstado(EstadoMembresiaActiva, ahora)
}

func (m *Membresia) cambiarEstado(destino EstadoMembresia, ahora time.Time) error {
	origen := m.estado
	m.estado = destino
	m.actualizadaEn = ahora
	m.agregarEvento(NuevoEstadoMembresiaCambiado(m.id, m.organizacionID, m.usuarioID, origen, destino, ahora))
	return nil
}

// Remover transiciona la membresía a removida (terminal, INV-TEN-08: nunca
// se borra físicamente) por decisión de un administrador. Aplica la regla
// de dominancia (INV-TEN-20) y el invariante del último propietario
// (INV-TEN-06).
func (m *Membresia) Remover(ejecutor Rol, propietariosActivos int, ahora time.Time) error {
	if !m.estado.PuedeTransicionarA(EstadoMembresiaRemovida) {
		return &ErrTransicionEstadoMembresiaInvalida{Origen: m.estado, Destino: EstadoMembresiaRemovida}
	}
	if err := ValidarDominancia(ejecutor, m.rol); err != nil {
		return err
	}
	if m.dejariaOrganizacionSinPropietario(propietariosActivos) {
		return &ErrUltimoPropietario{}
	}
	m.remover(ahora, false)
	return nil
}

// Abandonar transiciona la membresía a removida por iniciativa del propio
// sujeto. No aplica la regla de dominancia (un sujeto siempre puede actuar
// sobre su propia membresía, tercer paso de INV-TEN-20), pero sí el
// invariante del último propietario (INV-TEN-06): el único propietario no
// puede abandonar sin transferir la propiedad antes.
func (m *Membresia) Abandonar(propietariosActivos int, ahora time.Time) error {
	if !m.estado.PuedeTransicionarA(EstadoMembresiaRemovida) {
		return &ErrTransicionEstadoMembresiaInvalida{Origen: m.estado, Destino: EstadoMembresiaRemovida}
	}
	if m.dejariaOrganizacionSinPropietario(propietariosActivos) {
		return &ErrUltimoPropietario{}
	}
	m.remover(ahora, true)
	return nil
}

func (m *Membresia) remover(ahora time.Time, porIniciativaPropia bool) {
	rolAlRemover := m.rol
	m.estado = EstadoMembresiaRemovida
	removidaEn := ahora
	m.removidaEn = &removidaEn
	m.actualizadaEn = ahora
	m.agregarEvento(NuevoMiembroRemovido(m.id, m.organizacionID, m.usuarioID, rolAlRemover, porIniciativaPropia, ahora))
}

// dejariaOrganizacionSinPropietario implementa el lado de Membresia de
// INV-TEN-06: verdadero cuando esta membresía es, hoy, un propietario
// activo, y la operación en curso la sacaría de ese conjunto (cambiando su
// rol, suspendiéndola o removiéndola) sin que quede ningún otro propietario
// activo. propietariosActivos es el conteo ANTES de la operación,
// suministrado por el caso de uso.
func (m *Membresia) dejariaOrganizacionSinPropietario(propietariosActivos int) bool {
	if !m.estado.EsIgual(EstadoMembresiaActiva) {
		return false
	}
	if !m.rol.EsIgual(RolPropietario) {
		return false
	}
	return propietariosActivos <= 1
}

// Permite implementa el atajo de conveniencia sobre EvaluadorDeAutorizacion
// (§1.4 del diseño): equivale a Autorizar(estadoOrg, m, permiso).Permitido().
func (m *Membresia) Permite(permiso Permiso, estadoOrg EstadoOrganizacion) bool {
	return Autorizar(estadoOrg, m, permiso).Permitido()
}

// --- eventos ------------------------------------------------------------

// EventosPendientes drena los eventos acumulados por el agregado: los
// devuelve y vacía el buffer interno. El caso de uso debe llamarlo una sola
// vez, tras persistir el agregado dentro de la misma UnidadDeTrabajo que la
// auditoría (ADR 0005, INV-TEN-25).
func (m *Membresia) EventosPendientes() []EventoDominio {
	eventos := m.eventos
	m.eventos = nil
	return eventos
}

func (m *Membresia) agregarEvento(e EventoDominio) {
	m.eventos = append(m.eventos, e)
}
