package dominio

import "time"

// Usuario es el agregado raíz del contexto Identidad: representa la
// identidad global de un sujeto capaz de autenticarse, sin estar ligado a
// ninguna organización (eso es Membresia, del contexto Tenencia). Toda
// mutación ocurre por un método de negocio; no hay campos exportados ni
// setters, y los getters devuelven copias de valores, nunca punteros
// internos (INV-ID-09).
type Usuario struct {
	id             IDUsuario
	correo         Correo
	credencial     Credencial
	estado         EstadoUsuario
	tieneMFA       bool
	creadoEn       time.Time
	actualizadoEn  time.Time
	ultimoAccesoEn *time.Time

	eventos []EventoDominio
}

// RegistrarUsuario crea un nuevo agregado Usuario en estado
// pendiente_verificacion (INV-ID-07, nunca en activo) y acumula el evento
// UsuarioRegistrado. id, correo y hash deben venir ya validados por sus
// propios constructores (INV-ID-01: no hay usuario sin correo válido ni sin
// credencial).
func RegistrarUsuario(id IDUsuario, correo Correo, hash HashContrasena, ahora time.Time) (*Usuario, error) {
	if id.EsVacio() {
		return nil, &ErrIDUsuarioInvalido{Motivo: "no puede estar vacío"}
	}
	credencial, err := NuevaCredencial(hash, ahora)
	if err != nil {
		return nil, err
	}
	u := &Usuario{
		id:            id,
		correo:        correo,
		credencial:    credencial,
		estado:        EstadoPendienteVerificacion,
		tieneMFA:      false,
		creadoEn:      ahora,
		actualizadoEn: ahora,
	}
	u.agregarEvento(NuevoUsuarioRegistrado(id, correo, ahora))
	return u, nil
}

// Reconstituir reconstruye un agregado Usuario a partir de datos ya
// validados y persistidos (p. ej. una fila de la tabla usuarios mapeada por
// el adaptador Postgres). A diferencia de RegistrarUsuario, no acumula
// eventos: no representa una operación de negocio nueva, sino la
// rehidratación de una ya ocurrida.
func Reconstituir(
	id IDUsuario,
	correo Correo,
	credencial Credencial,
	estado EstadoUsuario,
	tieneMFA bool,
	creadoEn time.Time,
	actualizadoEn time.Time,
	ultimoAccesoEn *time.Time,
) *Usuario {
	var copia *time.Time
	if ultimoAccesoEn != nil {
		v := *ultimoAccesoEn
		copia = &v
	}
	return &Usuario{
		id:             id,
		correo:         correo,
		credencial:     credencial,
		estado:         estado,
		tieneMFA:       tieneMFA,
		creadoEn:       creadoEn,
		actualizadoEn:  actualizadoEn,
		ultimoAccesoEn: copia,
	}
}

// --- getters (copias de valor, nunca punteros internos: INV-ID-09) --------

// ID devuelve el identificador del usuario.
func (u *Usuario) ID() IDUsuario { return u.id }

// Correo devuelve el correo del usuario. Es inmutable desde fuera del
// agregado (INV-ID-03): no existe un setter.
func (u *Usuario) Correo() Correo { return u.correo }

// Credencial devuelve la credencial vigente del usuario.
func (u *Usuario) Credencial() Credencial { return u.credencial }

// Estado devuelve el estado actual del ciclo de vida de la cuenta.
func (u *Usuario) Estado() EstadoUsuario { return u.estado }

// TieneMFA indica si el usuario tiene al menos un factor MFA confirmado.
func (u *Usuario) TieneMFA() bool { return u.tieneMFA }

// CreadoEn devuelve la marca de tiempo de creación del usuario.
func (u *Usuario) CreadoEn() time.Time { return u.creadoEn }

// ActualizadoEn devuelve la marca de tiempo de la última mutación del
// agregado (INV-ID-10: siempre con la hora provista por el puerto Reloj).
func (u *Usuario) ActualizadoEn() time.Time { return u.actualizadoEn }

// UltimoAccesoEn devuelve la marca de tiempo del último acceso exitoso y un
// booleano que indica si el usuario ha accedido alguna vez. Se evita
// devolver *time.Time para no exponer un puntero interno (INV-ID-09).
func (u *Usuario) UltimoAccesoEn() (time.Time, bool) {
	if u.ultimoAccesoEn == nil {
		return time.Time{}, false
	}
	return *u.ultimoAccesoEn, true
}

// --- verificación de contraseña --------------------------------------------

// VerificadorContrasenas es el contrato mínimo, definido en el propio
// dominio, que Usuario necesita para delegar la verificación criptográfica
// de una contraseña a un adaptador de infraestructura. Se define aquí (y no
// se reutiliza el puerto puertos.HasherContrasenas) porque el dominio no
// puede importar el paquete puertos sin crear un ciclo de imports
// (puertos ya importa dominio). El adaptador Argon2id de infraestructura
// implementa la interfaz más amplia puertos.HasherContrasenas, que a su vez
// satisface esta interfaz reducida sin duplicar lógica.
type VerificadorContrasenas interface {
	Verificar(hash HashContrasena, plana ContrasenaPlana) bool
}

// VerificarContrasena delega en el verificador criptográfico la
// comprobación de que plana corresponde al hash vigente del usuario. No
// muta el agregado ni emite eventos: es una consulta.
func (u *Usuario) VerificarContrasena(verificador VerificadorContrasenas, plana ContrasenaPlana) bool {
	if verificador == nil {
		return false
	}
	return verificador.Verificar(u.credencial.Hash(), plana)
}

// --- mutaciones de credencial -----------------------------------------------

// CambiarContrasena reemplaza el hash de contraseña por decisión propia del
// usuario (el caso de uso exige la contraseña actual antes de llamar a este
// método) y emite ContrasenaCambiada.
func (u *Usuario) CambiarContrasena(nuevoHash HashContrasena, ahora time.Time) error {
	credencial, err := NuevaCredencial(nuevoHash, ahora)
	if err != nil {
		return err
	}
	u.credencial = credencial
	u.actualizadoEn = ahora
	u.agregarEvento(NuevoContrasenaCambiada(u.id, ahora))
	return nil
}

// ReemplazarHash reemplaza el hash de contraseña por un rehash oportunista
// (INV-ID-13): el caso de uso lo invoca tras una verificación exitosa
// cuando el adaptador de hashing indica que el hash vigente ya no cumple
// los parámetros actuales. Emite CredencialRehasheada. Un fallo aquí no
// debe abortar el login: es responsabilidad del caso de uso decidirlo.
func (u *Usuario) ReemplazarHash(nuevoHash HashContrasena, ahora time.Time) error {
	credencial, err := NuevaCredencial(nuevoHash, ahora)
	if err != nil {
		return err
	}
	u.credencial = credencial
	u.actualizadoEn = ahora
	u.agregarEvento(NuevoCredencialRehasheada(u.id, ahora))
	return nil
}

// --- transiciones de estado (INV-ID-05) -------------------------------------

// ConfirmarCorreo transiciona el usuario de pendiente_verificacion a activo
// y emite CorreoVerificado. Es la única acción que produce ese destino
// desde ese origen específico: no debe confundirse con Reactivar, que
// alcanza "activo" desde suspendido o bloqueado.
func (u *Usuario) ConfirmarCorreo(ahora time.Time) error {
	if !u.estado.EsIgual(EstadoPendienteVerificacion) {
		return &ErrTransicionEstadoInvalida{Origen: u.estado, Destino: EstadoActivo}
	}
	u.estado = EstadoActivo
	u.actualizadoEn = ahora
	u.agregarEvento(NuevoCorreoVerificado(u.id, u.correo, ahora))
	return nil
}

// Suspender transiciona el usuario de activo a suspendido y emite
// EstadoUsuarioCambiado. motivo es obligatorio: la auditoría exige con qué
// autoridad y por qué se suspende una cuenta.
func (u *Usuario) Suspender(motivo MotivoCambioEstado, ahora time.Time) error {
	if !u.estado.EsIgual(EstadoActivo) {
		return &ErrTransicionEstadoInvalida{Origen: u.estado, Destino: EstadoSuspendido}
	}
	return u.cambiarEstado(EstadoSuspendido, motivo.Valor(), ahora)
}

// Bloquear transiciona el usuario de pendiente_verificacion o de activo a
// bloqueado, y emite EstadoUsuarioCambiado. motivo es obligatorio.
func (u *Usuario) Bloquear(motivo MotivoCambioEstado, ahora time.Time) error {
	if !u.estado.EsIgual(EstadoPendienteVerificacion) && !u.estado.EsIgual(EstadoActivo) {
		return &ErrTransicionEstadoInvalida{Origen: u.estado, Destino: EstadoBloqueado}
	}
	return u.cambiarEstado(EstadoBloqueado, motivo.Valor(), ahora)
}

// Reactivar transiciona el usuario de suspendido o de bloqueado a activo, y
// emite EstadoUsuarioCambiado. La reactivación desde bloqueado solo debe
// invocarse desde un caso de uso administrativo; esa restricción de
// autorización es de la capa de aplicación, no del dominio.
func (u *Usuario) Reactivar(ahora time.Time) error {
	if !u.estado.EsIgual(EstadoSuspendido) && !u.estado.EsIgual(EstadoBloqueado) {
		return &ErrTransicionEstadoInvalida{Origen: u.estado, Destino: EstadoActivo}
	}
	return u.cambiarEstado(EstadoActivo, "", ahora)
}

func (u *Usuario) cambiarEstado(destino EstadoUsuario, motivo string, ahora time.Time) error {
	anterior := u.estado
	u.estado = destino
	u.actualizadoEn = ahora
	u.agregarEvento(NuevoEstadoUsuarioCambiado(u.id, anterior, destino, motivo, ahora))
	return nil
}

// --- acceso -----------------------------------------------------------------

// RegistrarAcceso marca el instante de un inicio de sesión exitoso. No
// valida el estado del usuario: eso es responsabilidad de
// PuedeIniciarSesion, que el caso de uso invoca antes.
func (u *Usuario) RegistrarAcceso(ahora time.Time) {
	copia := ahora
	u.ultimoAccesoEn = &copia
	u.actualizadoEn = ahora
}

// PuedeIniciarSesion aplica INV-ID-06: solo un usuario en estado activo
// puede autenticarse con éxito. Los demás estados producen un error
// específico. El caso de uso debe invocar este método solo después de
// verificar la contraseña (para no filtrar el estado de la cuenta a un
// atacante que no la conoce).
func (u *Usuario) PuedeIniciarSesion() error {
	switch u.estado {
	case EstadoActivo:
		return nil
	case EstadoPendienteVerificacion:
		return &ErrCorreoNoVerificado{}
	case EstadoSuspendido:
		return &ErrCuentaSuspendida{}
	default:
		// EstadoBloqueado y EstadoAnonimizado: ambos son "cuenta fuera de
		// servicio" desde la perspectiva de login; no hay error dedicado
		// para anonimizado en la tabla 1.5, así que se trata como bloqueo.
		return &ErrCuentaBloqueada{}
	}
}

// --- eventos ------------------------------------------------------------

// EventosPendientes drena los eventos acumulados por el agregado: los
// devuelve y vacía el buffer interno. El caso de uso debe llamarlo una sola
// vez, tras persistir el agregado dentro de la misma UnidadDeTrabajo que la
// auditoría (ADR 0005).
func (u *Usuario) EventosPendientes() []EventoDominio {
	eventos := u.eventos
	u.eventos = nil
	return eventos
}

func (u *Usuario) agregarEvento(e EventoDominio) {
	u.eventos = append(u.eventos, e)
}
