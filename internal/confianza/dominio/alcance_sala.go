package dominio

import (
	"strings"
	"time"
)

// --- TipoAlcance -------------------------------------------------------------

// TipoAlcance es el catálogo cerrado de las dos formas que puede tomar
// AlcanceSala (§1.1 y tabla 1.3 del diseño).
type TipoAlcance struct {
	valor string
}

var (
	// TipoAlcanceSistema identifica una sala que protege una ruta
	// pre-autenticación: la clave es la propia ruta, sin discriminador de
	// tenant (§1.1: "en POST /acceso/sesiones no existe una organización
	// que resolver").
	TipoAlcanceSistema = TipoAlcance{valor: "sistema"}
	// TipoAlcanceOrganizacion identifica una sala org-scoped: la clave
	// incluye el IDOrganizacion ya autorizado por el middleware de
	// Tenencia (§1.1, INV-COLA-09).
	TipoAlcanceOrganizacion = TipoAlcance{valor: "organizacion"}
)

// String devuelve la representación canónica del tipo de alcance.
func (t TipoAlcance) String() string { return t.valor }

// EsIgual compara dos tipos de alcance por su valor.
func (t TipoAlcance) EsIgual(otro TipoAlcance) bool { return t.valor == otro.valor }

// EsVacio indica si el value object nunca fue construido (zero value).
func (t TipoAlcance) EsVacio() bool { return t.valor == "" }

// --- AlcanceSala ---------------------------------------------------------

// AlcanceSala es el value object de dos formas que decide qué tan agregada
// es la protección de una SalaDeEspera (§1.1 del diseño). Es inmutable: solo
// se construye con AlcanceSistema() o AlcanceOrganizacion(id), que hacen
// estructuralmente imposible construir una combinación incoherente (un
// alcance "organizacion" sin IDOrganizacion, o uno "sistema" con uno) — no
// hace falta una validación cruzada adicional porque cada constructor solo
// puede producir la forma que le corresponde.
type AlcanceSala struct {
	tipo           TipoAlcance
	organizacionID *IDOrganizacion
}

// AlcanceSistema construye el alcance de una sala pre-autenticación.
func AlcanceSistema() AlcanceSala {
	return AlcanceSala{tipo: TipoAlcanceSistema}
}

// AlcanceOrganizacion construye el alcance de una sala org-scoped. El
// IDOrganizacion no puede estar vacío.
func AlcanceOrganizacion(id IDOrganizacion) (AlcanceSala, error) {
	if id.EsVacio() {
		return AlcanceSala{}, &ErrAlcanceSalaInvalido{Motivo: "el alcance organizacion exige un IDOrganizacion no vacío"}
	}
	copia := id
	return AlcanceSala{tipo: TipoAlcanceOrganizacion, organizacionID: &copia}, nil
}

// ReconstituirAlcanceSala reconstruye un AlcanceSala a partir de los dos
// campos ya persistidos (alcance_tipo, alcance_organizacion_id de la tabla
// salas_espera, §6.1) y verifica la coherencia entre ambos (el CHECK
// salas_espera_alcance_coherente del lado de Postgres, replicado aquí para
// que un dato corrupto no produzca un agregado inconsistente en memoria).
func ReconstituirAlcanceSala(tipo string, organizacionID string) (AlcanceSala, error) {
	organizacionID = strings.TrimSpace(organizacionID)
	switch tipo {
	case TipoAlcanceSistema.valor:
		if organizacionID != "" {
			return AlcanceSala{}, &ErrAlcanceSalaInvalido{Motivo: "el alcance sistema no admite un IDOrganizacion"}
		}
		return AlcanceSistema(), nil
	case TipoAlcanceOrganizacion.valor:
		id, err := IDOrganizacionDesde(organizacionID)
		if err != nil {
			return AlcanceSala{}, err
		}
		return AlcanceOrganizacion(id)
	default:
		return AlcanceSala{}, &ErrAlcanceSalaInvalido{Motivo: "tipo de alcance desconocido: " + tipo}
	}
}

// Tipo devuelve el tipo de alcance (sistema | organizacion).
func (a AlcanceSala) Tipo() TipoAlcance { return a.tipo }

// EsSistema indica si el alcance es el de una sala pre-autenticación.
func (a AlcanceSala) EsSistema() bool { return a.tipo.EsIgual(TipoAlcanceSistema) }

// OrganizacionID devuelve el identificador de la organización dueña de la
// sala, y un booleano que indica si el alcance es org-scoped.
func (a AlcanceSala) OrganizacionID() (IDOrganizacion, bool) {
	if a.organizacionID == nil {
		return IDOrganizacion{}, false
	}
	return *a.organizacionID, true
}

// EsVacio indica si el value object nunca fue construido (zero value).
func (a AlcanceSala) EsVacio() bool { return a.tipo.EsVacio() }

// EsIgual compara dos alcances por su tipo y, si aplica, su organización.
func (a AlcanceSala) EsIgual(otro AlcanceSala) bool {
	if !a.tipo.EsIgual(otro.tipo) {
		return false
	}
	idA, okA := a.OrganizacionID()
	idB, okB := otro.OrganizacionID()
	if okA != okB {
		return false
	}
	if !okA {
		return true
	}
	return idA.EsIgual(idB)
}

// Clave devuelve el discriminador textual del alcance: "sistema" o
// "org:<uuid>". Es la mitad izquierda de ClaveSala (tabla 1.3 del diseño).
func (a AlcanceSala) Clave() string {
	if a.EsSistema() {
		return TipoAlcanceSistema.valor
	}
	id, _ := a.OrganizacionID()
	return "org:" + id.String()
}

// --- RutaProtegida ---------------------------------------------------------

// RutaProtegida es el catálogo cerrado de rutas que una SalaDeEspera puede
// proteger (§1.6 del diseño). Una ruta que no pertenece a este catálogo no
// puede protegerse: cada entrada exige una decisión explícita sobre dónde
// va en la cadena de middlewares y sobre INV-COLA-10, así que el catálogo
// cerrado es, en sí mismo, el mecanismo que hace cumplir esa invariante —
// las rutas excluidas a propósito (renovación de sesión, segundo factor,
// JWKS) simplemente nunca se agregan aquí.
type RutaProtegida struct {
	valor string
}

var (
	// RutaAccesoIniciarSesion protege POST /acceso/sesiones: el caso
	// canónico (Argon2id + Postgres + auditoría por intento).
	RutaAccesoIniciarSesion = RutaProtegida{valor: "acceso.iniciar_sesion"}
	// RutaIdentidadRegistrarUsuario protege POST /identidad/usuarios:
	// apertura masiva de inscripciones.
	RutaIdentidadRegistrarUsuario = RutaProtegida{valor: "identidad.registrar_usuario"}
	// RutaTenenciaAceptarInvitacion protege
	// POST /tenencia/invitaciones/aceptaciones: onboarding masivo.
	RutaTenenciaAceptarInvitacion = RutaProtegida{valor: "tenencia.aceptar_invitacion"}
)

// RutaProtegidaDesde valida un valor contra el catálogo cerrado de rutas
// protegibles.
func RutaProtegidaDesde(valor string) (RutaProtegida, error) {
	v := strings.TrimSpace(valor)
	switch v {
	case RutaAccesoIniciarSesion.valor, RutaIdentidadRegistrarUsuario.valor, RutaTenenciaAceptarInvitacion.valor:
		return RutaProtegida{valor: v}, nil
	default:
		return RutaProtegida{}, &ErrRutaNoProtegible{Motivo: "ruta fuera del catálogo cerrado: " + v}
	}
}

// String devuelve el identificador canónico de la ruta.
func (r RutaProtegida) String() string { return r.valor }

// EsVacio indica si el value object nunca fue construido (zero value).
func (r RutaProtegida) EsVacio() bool { return r.valor == "" }

// EsIgual compara dos rutas por su valor.
func (r RutaProtegida) EsIgual(otro RutaProtegida) bool { return r.valor == otro.valor }

// AdmiteAlcance indica si esta ruta puede protegerse con el alcance dado
// (tabla del §1.6 del diseño). Las tres rutas del catálogo actual son
// exclusivamente pre-autenticación y solo admiten AlcanceSistema; una ruta
// org-scoped futura declararía aquí su propio criterio.
func (r RutaProtegida) AdmiteAlcance(a AlcanceSala) bool {
	switch r.valor {
	case RutaAccesoIniciarSesion.valor, RutaIdentidadRegistrarUsuario.valor, RutaTenenciaAceptarInvitacion.valor:
		return a.EsSistema()
	default:
		return false
	}
}

// VidaDeLaCredencialDeEntrada devuelve cuánto tiempo sigue siendo válida la
// credencial que trajo al cliente hasta esta ruta, para que INV-COLA-10
// pueda contrastarla contra la espera máxima esperable de una sala. Cero
// significa "sin restricción activa conocida": ninguna de las tres rutas
// del catálogo actual depende de una credencial de vida corta (son
// pre-autenticación); la invariante se cumple hoy por construcción del
// catálogo cerrado (las rutas excluidas — segundo factor con step-up de 5
// minutos, renovación de sesión — directamente no están aquí, §1.6). Este
// método queda como el punto de extensión para una futura ruta org-scoped
// cuya credencial de entrada sí tenga una vida acotada.
func (r RutaProtegida) VidaDeLaCredencialDeEntrada() time.Duration {
	return 0
}

// --- ClaveSala ---------------------------------------------------------

// ClaveSala es el discriminador de unicidad de INV-COLA-01 y el prefijo de
// todas las claves de Redis de una sala: "<alcance>:<ruta>", p. ej.
// "sistema:acceso.iniciar_sesion" u "org:0193...:tenencia.aceptar_invitacion"
// (tabla 1.3 del diseño). Se construye solo desde el agregado
// (SalaDeEspera.Clave()), nunca concatenando strings sueltos en un
// adaptador.
type ClaveSala struct {
	valor string
}

// nuevaClaveSala construye la clave a partir del alcance y la ruta de una
// sala. No exportada: solo SalaDeEspera.Clave() la invoca.
func nuevaClaveSala(alcance AlcanceSala, ruta RutaProtegida) ClaveSala {
	return ClaveSala{valor: alcance.Clave() + ":" + ruta.String()}
}

// String devuelve la representación textual de la clave.
func (c ClaveSala) String() string { return c.valor }

// EsVacio indica si el value object nunca fue construido (zero value).
func (c ClaveSala) EsVacio() bool { return c.valor == "" }

// EsIgual compara dos claves por su valor.
func (c ClaveSala) EsIgual(otro ClaveSala) bool { return c.valor == otro.valor }
