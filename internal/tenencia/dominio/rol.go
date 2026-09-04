package dominio

// Rol es el value object enum, cerrado y totalmente ordenado, que
// representa el nivel de un miembro dentro de una organización (§1.3 y
// ADR candidato 0029). El orden total es lo que convierte "¿tiene al menos
// el rol X?" y "¿puede tocar a este otro miembro?" en comparaciones de
// enteros en vez de una tabla de casos (INV-TEN-10, INV-TEN-20).
type Rol struct {
	valor string
	nivel int
}

var (
	// RolPropietario es el rol de mayor nivel: el único con
	// organizacion.archivar y propiedad.transferir. Toda organización tiene
	// al menos un miembro con este rol, activo, en todo momento (INV-TEN-06).
	RolPropietario = Rol{valor: "propietario", nivel: 30}
	// RolAdministrador puede editar la organización y gestionar miembros,
	// sujeto a la regla de dominancia: nunca puede tocar a un propietario ni
	// crear uno (INV-TEN-20).
	RolAdministrador = Rol{valor: "administrador", nivel: 20}
	// RolMiembro es el rol de menor nivel: solo puede ver la organización y
	// la lista de miembros.
	RolMiembro = Rol{valor: "miembro", nivel: 10}
)

// catalogoRoles es el catálogo cerrado completo, usado por RolDesde para
// validar contra valores desconocidos.
var catalogoRoles = map[string]Rol{
	RolPropietario.valor:   RolPropietario,
	RolAdministrador.valor: RolAdministrador,
	RolMiembro.valor:       RolMiembro,
}

// RolDesde valida un valor persistido (o recibido de un comando) contra el
// catálogo cerrado propietario > administrador > miembro.
func RolDesde(valor string) (Rol, error) {
	r, ok := catalogoRoles[valor]
	if !ok {
		return Rol{}, &ErrRolInvalido{Valor: valor}
	}
	return r, nil
}

// Valor devuelve la representación canónica del rol.
func (r Rol) Valor() string { return r.valor }

// String implementa fmt.Stringer.
func (r Rol) String() string { return r.valor }

// EsVacio indica si el value object nunca fue construido (zero value).
func (r Rol) EsVacio() bool { return r.valor == "" }

// EsIgual compara dos roles por su valor.
func (r Rol) EsIgual(otro Rol) bool { return r.valor == otro.valor }

// Nivel devuelve el nivel numérico del rol en el orden total
// propietario(30) > administrador(20) > miembro(10).
func (r Rol) Nivel() int { return r.nivel }

// DominaA indica si r tiene un nivel estrictamente superior al de otro. Es
// la comparación atómica sobre la que se construye ReglaDeDominancia
// (INV-TEN-20): "propietario domina a administrador y a miembro",
// "administrador domina a miembro", ningún rol se domina a sí mismo.
func (r Rol) DominaA(otro Rol) bool { return r.nivel > otro.nivel }

// matrizDePermisos es la tabla literal rol -> permisos concedidos (§1.4 del
// diseño: MatrizDePermisos). Los permisos marcados con (*) en el diseño
// (miembro.cambiar_rol y miembro.remover para administrador) están
// concedidos en la matriz sin condición: la restricción adicional que
// impide a un administrador tocar a un propietario es la regla de
// dominancia (INV-TEN-20), aplicada por los métodos de negocio de
// Membresia, no por esta tabla.
var matrizDePermisos = map[string][]Permiso{
	RolPropietario.valor: {
		PermisoOrganizacionVer,
		PermisoOrganizacionEditar,
		PermisoOrganizacionArchivar,
		PermisoMiembroVer,
		PermisoMiembroInvitar,
		PermisoMiembroCambiarRol,
		PermisoMiembroRemover,
		PermisoPropiedadTransferir,
	},
	RolAdministrador.valor: {
		PermisoOrganizacionVer,
		PermisoOrganizacionEditar,
		PermisoMiembroVer,
		PermisoMiembroInvitar,
		PermisoMiembroCambiarRol,
		PermisoMiembroRemover,
	},
	RolMiembro.valor: {
		PermisoOrganizacionVer,
		PermisoMiembroVer,
	},
}

// Permisos implementa el servicio de dominio MatrizDePermisos (§1.4 del
// diseño): función pura Rol -> []Permiso, expuesta como método de Rol
// (mismo patrón que EstadoUsuario.PuedeTransicionarA). Devuelve una copia:
// el llamador nunca recibe un slice compartido con la tabla interna.
func (r Rol) Permisos() []Permiso {
	permisos := matrizDePermisos[r.valor]
	copia := make([]Permiso, len(permisos))
	copy(copia, permisos)
	return copia
}

// TienePermiso indica si la matriz de permisos de este rol incluye el
// permiso dado. Es la comprobación atómica del paso 4 de
// EvaluadorDeAutorizacion (§1.4 del diseño).
func (r Rol) TienePermiso(p Permiso) bool {
	for _, concedido := range matrizDePermisos[r.valor] {
		if concedido.EsIgual(p) {
			return true
		}
	}
	return false
}

// --- ReglaDeDominancia (§1.4 del diseño, INV-TEN-20) ------------------------

// ValidarOtorgamiento aplica el primer paso de la regla de dominancia:
// nadie puede otorgar un rol estrictamente superior al propio. Se invoca al
// agregar un miembro, invitarlo o cambiarle el rol.
func ValidarOtorgamiento(ejecutor, nuevo Rol) error {
	if nuevo.Nivel() > ejecutor.Nivel() {
		return &ErrRolSuperiorAlPropio{Ejecutor: ejecutor.valor, Nuevo: nuevo.valor}
	}
	return nil
}

// ValidarDominancia aplica el segundo paso de la regla de dominancia: nadie
// puede modificar ni remover una membresía cuyo rol sea estrictamente
// superior al propio. Se invoca al cambiar el rol de otro miembro, al
// suspenderlo o al removerlo — nunca al actuar sobre la propia membresía
// (abandonar/degradarse, tercer paso de la regla, sujeto solo a INV-TEN-06).
func ValidarDominancia(ejecutor, objetivo Rol) error {
	if objetivo.Nivel() > ejecutor.Nivel() {
		return &ErrMembresiaDominante{Ejecutor: ejecutor.valor, Objetivo: objetivo.valor}
	}
	return nil
}
