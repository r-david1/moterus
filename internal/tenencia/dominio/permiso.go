package dominio

// Permiso es el value object enum cerrado que representa una capacidad
// concreta dentro de una organización (tabla 1.3 y §1.4 del diseño). Es el
// vocabulario con el que otros contextos piden autorización a Tenencia
// (VerificadorDeAutorizacion, puertos/entrada.go), así que es parte del
// contrato público del contexto y no se construye concatenando strings
// (mismo criterio que las acciones de auditoría del catálogo cerrado
// auditoria_acciones — dos catálogos distintos que casualmente comparten la
// forma "recurso.accion").
type Permiso struct {
	valor string
}

var (
	// PermisoOrganizacionVer permite consultar los datos de la organización.
	PermisoOrganizacionVer = Permiso{valor: "organizacion.ver"}
	// PermisoOrganizacionEditar permite renombrar o cambiar el alias.
	PermisoOrganizacionEditar = Permiso{valor: "organizacion.editar"}
	// PermisoOrganizacionArchivar permite suspender, reactivar o archivar la
	// organización.
	PermisoOrganizacionArchivar = Permiso{valor: "organizacion.archivar"}
	// PermisoMiembroVer permite listar los miembros de la organización.
	PermisoMiembroVer = Permiso{valor: "miembro.ver"}
	// PermisoMiembroInvitar permite emitir y revocar invitaciones.
	PermisoMiembroInvitar = Permiso{valor: "miembro.invitar"}
	// PermisoMiembroCambiarRol permite cambiar el rol de un miembro, sujeto
	// a la regla de dominancia (INV-TEN-20).
	PermisoMiembroCambiarRol = Permiso{valor: "miembro.cambiar_rol"}
	// PermisoMiembroRemover permite remover a un miembro, sujeto a la regla
	// de dominancia (INV-TEN-20).
	PermisoMiembroRemover = Permiso{valor: "miembro.remover"}
	// PermisoPropiedadTransferir permite transferir la propiedad de la
	// organización a otro miembro.
	PermisoPropiedadTransferir = Permiso{valor: "propiedad.transferir"}
)

// catalogoPermisos es el catálogo cerrado completo, usado por PermisoDesde
// para validar contra valores desconocidos.
var catalogoPermisos = map[string]Permiso{
	PermisoOrganizacionVer.valor:      PermisoOrganizacionVer,
	PermisoOrganizacionEditar.valor:   PermisoOrganizacionEditar,
	PermisoOrganizacionArchivar.valor: PermisoOrganizacionArchivar,
	PermisoMiembroVer.valor:           PermisoMiembroVer,
	PermisoMiembroInvitar.valor:       PermisoMiembroInvitar,
	PermisoMiembroCambiarRol.valor:    PermisoMiembroCambiarRol,
	PermisoMiembroRemover.valor:       PermisoMiembroRemover,
	PermisoPropiedadTransferir.valor:  PermisoPropiedadTransferir,
}

// PermisoDesde valida un valor contra el catálogo cerrado de 8 permisos. Un
// permiso fuera del catálogo es un bug del llamador (p. ej. otro contexto
// pidiendo autorización con un string mal escrito), no una denegación: el
// caso de uso Autorizar lo traduce a ErrPermisoDesconocido, que el
// adaptador HTTP mapea a 500 y no a 403 (§3.7 del diseño).
func PermisoDesde(valor string) (Permiso, error) {
	p, ok := catalogoPermisos[valor]
	if !ok {
		return Permiso{}, &ErrPermisoDesconocido{Valor: valor}
	}
	return p, nil
}

// Valor devuelve la representación canónica "recurso.accion" del permiso.
func (p Permiso) Valor() string { return p.valor }

// String implementa fmt.Stringer.
func (p Permiso) String() string { return p.valor }

// EsVacio indica si el value object nunca fue construido (zero value).
func (p Permiso) EsVacio() bool { return p.valor == "" }

// EsIgual compara dos permisos por su valor.
func (p Permiso) EsIgual(otro Permiso) bool { return p.valor == otro.valor }
