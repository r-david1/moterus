package dominio

import "time"

// Organizacion es el agregado raíz del contexto Tenencia: representa un
// tenant (ADR 0002, sin capa de producto). Toda mutación ocurre por un
// método de negocio; no hay campos exportados ni setters, y los getters
// devuelven copias de valores, nunca punteros internos (INV-TEN-11).
//
// Deliberadamente NO contiene la colección de Membresia (§1.2 del diseño):
// cambiarle el rol a un miembro no muta la organización, y una organización
// puede tener miles de miembros. Membresia es un agregado propio,
// referenciado por IDOrganizacion.
type Organizacion struct {
	id            IDOrganizacion
	alias         AliasOrganizacion
	nombre        NombreOrganizacion
	estado        EstadoOrganizacion
	creadaPor     IDUsuario
	creadaEn      time.Time
	actualizadaEn time.Time
	archivadaEn   *time.Time
	motivoEstado  MotivoCambioEstado

	eventos []EventoDominio
}

// CrearOrganizacion crea un nuevo agregado Organizacion en estado activa
// (INV-TEN-01: nace en activa, nunca en otro estado) y acumula el evento
// OrganizacionCreada. id, alias, nombre y creadaPor deben venir ya
// validados por sus propios constructores.
//
// Nota de invariante entre agregados (INV-TEN-03, INV-TEN-06): esta función
// por sí sola NO garantiza que la organización nazca con un propietario —
// eso lo hace el caso de uso CrearOrganizacion, que en la misma
// UnidadDeTrabajo también invoca FundarMembresia y persiste ambos agregados
// en la misma transacción. El CONSTRAINT TRIGGER diferido de §6 del diseño
// es la garantía estructural de que ningún caso de uso futuro se salte ese
// paso.
func CrearOrganizacion(id IDOrganizacion, alias AliasOrganizacion, nombre NombreOrganizacion, creadaPor IDUsuario, ahora time.Time) (*Organizacion, error) {
	if id.EsVacio() {
		return nil, &ErrIDOrganizacionInvalido{Motivo: "no puede estar vacío"}
	}
	if alias.EsVacio() {
		return nil, &ErrAliasInvalido{Motivo: "no puede estar vacío"}
	}
	if nombre.EsVacio() {
		return nil, &ErrNombreOrganizacionInvalido{Motivo: "no puede estar vacío"}
	}
	if creadaPor.EsVacio() {
		return nil, &ErrIDUsuarioInvalido{Motivo: "no puede estar vacío"}
	}
	o := &Organizacion{
		id:            id,
		alias:         alias,
		nombre:        nombre,
		estado:        EstadoOrganizacionActiva,
		creadaPor:     creadaPor,
		creadaEn:      ahora,
		actualizadaEn: ahora,
	}
	o.agregarEvento(NuevoOrganizacionCreada(id, alias, creadaPor, ahora))
	return o, nil
}

// ReconstituirOrganizacion reconstruye un agregado Organizacion a partir de
// datos ya validados y persistidos (p. ej. una fila de la tabla
// organizaciones mapeada por el adaptador Postgres). A diferencia de
// CrearOrganizacion, no acumula eventos: no representa una operación de
// negocio nueva, sino la rehidratación de una ya ocurrida.
func ReconstituirOrganizacion(
	id IDOrganizacion,
	alias AliasOrganizacion,
	nombre NombreOrganizacion,
	estado EstadoOrganizacion,
	creadaPor IDUsuario,
	creadaEn time.Time,
	actualizadaEn time.Time,
	archivadaEn *time.Time,
	motivoEstado MotivoCambioEstado,
) *Organizacion {
	var copia *time.Time
	if archivadaEn != nil {
		v := *archivadaEn
		copia = &v
	}
	return &Organizacion{
		id:            id,
		alias:         alias,
		nombre:        nombre,
		estado:        estado,
		creadaPor:     creadaPor,
		creadaEn:      creadaEn,
		actualizadaEn: actualizadaEn,
		archivadaEn:   copia,
		motivoEstado:  motivoEstado,
	}
}

// --- getters (copias de valor, nunca punteros internos: INV-TEN-11) --------

// ID devuelve el identificador de la organización.
func (o *Organizacion) ID() IDOrganizacion { return o.id }

// Alias devuelve el alias normalizado de la organización.
func (o *Organizacion) Alias() AliasOrganizacion { return o.alias }

// Nombre devuelve el nombre de presentación de la organización.
func (o *Organizacion) Nombre() NombreOrganizacion { return o.nombre }

// Estado devuelve el estado actual del ciclo de vida de la organización.
func (o *Organizacion) Estado() EstadoOrganizacion { return o.estado }

// CreadaPor devuelve el identificador del usuario que fundó la
// organización. Es un dato forense, no una fuente de autorización
// (INV-TEN-19): si el fundador cede la propiedad y sale, pierde todo
// acceso.
func (o *Organizacion) CreadaPor() IDUsuario { return o.creadaPor }

// CreadaEn devuelve la marca de tiempo de creación de la organización.
func (o *Organizacion) CreadaEn() time.Time { return o.creadaEn }

// ActualizadaEn devuelve la marca de tiempo de la última mutación del
// agregado (INV-TEN-14: siempre con la hora provista por el puerto Reloj).
func (o *Organizacion) ActualizadaEn() time.Time { return o.actualizadaEn }

// ArchivadaEn devuelve la marca de tiempo en que la organización se
// archivó, y un booleano que indica si está archivada.
func (o *Organizacion) ArchivadaEn() (time.Time, bool) {
	if o.archivadaEn == nil {
		return time.Time{}, false
	}
	return *o.archivadaEn, true
}

// MotivoEstado devuelve el motivo de la suspensión o el archivado vigente,
// y un booleano que indica si hay uno (vacío mientras la organización está
// activa).
func (o *Organizacion) MotivoEstado() (MotivoCambioEstado, bool) {
	if o.motivoEstado.EsVacio() {
		return MotivoCambioEstado{}, false
	}
	return o.motivoEstado, true
}

// --- mutaciones de negocio --------------------------------------------------

// Renombrar cambia el nombre de presentación de la organización y acumula
// OrganizacionActualizada. Es un no-op idempotente (sin mutación ni evento)
// si el nombre nuevo es igual al vigente.
func (o *Organizacion) Renombrar(nombre NombreOrganizacion, ahora time.Time) error {
	if nombre.EsVacio() {
		return &ErrNombreOrganizacionInvalido{Motivo: "no puede estar vacío"}
	}
	if o.nombre.EsIgual(nombre) {
		return nil
	}
	o.nombre = nombre
	o.actualizadaEn = ahora
	o.agregarEvento(NuevoOrganizacionActualizada(o.id, []string{"nombre"}, ahora))
	return nil
}

// CambiarAlias cambia el alias único de la organización y acumula
// OrganizacionActualizada. Es un no-op idempotente (sin mutación ni evento)
// si el alias nuevo es igual al vigente. La unicidad global del alias
// (INV-TEN-02) la garantiza el índice único de la base de datos: este
// método no la comprueba, solo aplica la mutación local.
func (o *Organizacion) CambiarAlias(alias AliasOrganizacion, ahora time.Time) error {
	if alias.EsVacio() {
		return &ErrAliasInvalido{Motivo: "no puede estar vacío"}
	}
	if o.alias.EsIgual(alias) {
		return nil
	}
	o.alias = alias
	o.actualizadaEn = ahora
	o.agregarEvento(NuevoOrganizacionActualizada(o.id, []string{"alias"}, ahora))
	return nil
}

// Suspender transiciona la organización de activa a suspendida (§1.4 del
// diseño: MaquinaEstadosOrganizacion) y acumula EstadoOrganizacionCambiado.
// motivo es obligatorio: la auditoría exige con qué autoridad y por qué se
// suspende un tenant completo. No muta ninguna membresía (INV-TEN-05): el
// acceso de todos los miembros se apaga por conjunción en tiempo de
// autorización.
func (o *Organizacion) Suspender(motivo MotivoCambioEstado, ahora time.Time) error {
	if !o.estado.PuedeTransicionarA(EstadoOrganizacionSuspendida) {
		return &ErrTransicionEstadoOrganizacionInvalida{Origen: o.estado, Destino: EstadoOrganizacionSuspendida}
	}
	if motivo.EsVacio() {
		return &ErrMotivoCambioEstadoInvalido{Motivo: "es obligatorio para suspender una organización"}
	}
	return o.cambiarEstado(EstadoOrganizacionSuspendida, motivo, ahora)
}

// Reactivar transiciona la organización de suspendida a activa y acumula
// EstadoOrganizacionCambiado. Restituye exactamente el estado previo, lo
// que hace la operación reversible sin guardar un snapshot (§3.4 del
// diseño).
func (o *Organizacion) Reactivar(ahora time.Time) error {
	if !o.estado.PuedeTransicionarA(EstadoOrganizacionActiva) {
		return &ErrTransicionEstadoOrganizacionInvalida{Origen: o.estado, Destino: EstadoOrganizacionActiva}
	}
	return o.cambiarEstado(EstadoOrganizacionActiva, MotivoCambioEstado{}, ahora)
}

// Archivar transiciona la organización de activa o suspendida a archivada
// (terminal e irreversible, INV-TEN-04) y acumula EstadoOrganizacionCambiado.
// motivo es obligatorio. No borra nada (§3.4 del diseño): el borrado real,
// si el producto alguna vez lo necesita, es un caso de uso de retención con
// su propio ADR.
func (o *Organizacion) Archivar(motivo MotivoCambioEstado, ahora time.Time) error {
	if !o.estado.PuedeTransicionarA(EstadoOrganizacionArchivada) {
		return &ErrTransicionEstadoOrganizacionInvalida{Origen: o.estado, Destino: EstadoOrganizacionArchivada}
	}
	if motivo.EsVacio() {
		return &ErrMotivoCambioEstadoInvalido{Motivo: "es obligatorio para archivar una organización"}
	}
	if err := o.cambiarEstado(EstadoOrganizacionArchivada, motivo, ahora); err != nil {
		return err
	}
	archivadaEn := ahora
	o.archivadaEn = &archivadaEn
	return nil
}

func (o *Organizacion) cambiarEstado(destino EstadoOrganizacion, motivo MotivoCambioEstado, ahora time.Time) error {
	origen := o.estado
	o.estado = destino
	o.motivoEstado = motivo
	o.actualizadaEn = ahora
	o.agregarEvento(NuevoEstadoOrganizacionCambiado(o.id, origen, destino, motivo.Valor(), ahora))
	return nil
}

// EstaOperativa implementa la comprobación de INV-TEN-18: solo una
// organización en estado activa concede autorización a sus miembros.
// Devuelve ErrOrganizacionNoOperativa si está suspendida o archivada.
func (o *Organizacion) EstaOperativa() error {
	if !o.estado.EsIgual(EstadoOrganizacionActiva) {
		return &ErrOrganizacionNoOperativa{Estado: o.estado.String()}
	}
	return nil
}

// --- eventos ------------------------------------------------------------

// EventosPendientes drena los eventos acumulados por el agregado: los
// devuelve y vacía el buffer interno. El caso de uso debe llamarlo una sola
// vez, tras persistir el agregado dentro de la misma UnidadDeTrabajo que la
// auditoría (ADR 0005, INV-TEN-25).
func (o *Organizacion) EventosPendientes() []EventoDominio {
	eventos := o.eventos
	o.eventos = nil
	return eventos
}

func (o *Organizacion) agregarEvento(e EventoDominio) {
	o.eventos = append(o.eventos, e)
}
