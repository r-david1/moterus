package dominio

import "time"

// TokenRefrescoEmitido es la entidad interna del agregado Sesion que
// representa un eslabón concreto de la cadena de rotación de tokens de
// refresco (tabla 1.2 del diseño: vive dentro del agregado porque un token
// de refresco no existe fuera de una sesión y "rotar" es una operación
// indivisible entre ambos). Se conserva incluso después de consumido: es la
// evidencia que hace posible la detección de reuso (INV-ACC-06) y el rastro
// forense de la sesión.
type TokenRefrescoEmitido struct {
	hash        HashTokenRefresco
	generacion  int
	emitidoEn   time.Time
	expiraEn    time.Time
	consumidoEn *time.Time
	hashSucesor *HashTokenRefresco
}

func nuevoTokenRefrescoEmitido(hash HashTokenRefresco, generacion int, emitidoEn, expiraEn time.Time) TokenRefrescoEmitido {
	return TokenRefrescoEmitido{
		hash:       hash,
		generacion: generacion,
		emitidoEn:  emitidoEn,
		expiraEn:   expiraEn,
	}
}

// ReconstituirTokenRefrescoEmitido reconstruye un TokenRefrescoEmitido a
// partir de datos ya validados y persistidos (p. ej. una fila de la tabla
// tokens_refresco). consumidoEn y hashSucesor pueden ir nil: un token
// todavía vigente no tiene ninguno de los dos.
func ReconstituirTokenRefrescoEmitido(
	hash HashTokenRefresco,
	generacion int,
	emitidoEn time.Time,
	expiraEn time.Time,
	consumidoEn *time.Time,
	hashSucesor *HashTokenRefresco,
) TokenRefrescoEmitido {
	t := TokenRefrescoEmitido{
		hash:       hash,
		generacion: generacion,
		emitidoEn:  emitidoEn,
		expiraEn:   expiraEn,
	}
	if consumidoEn != nil {
		copia := *consumidoEn
		t.consumidoEn = &copia
	}
	if hashSucesor != nil {
		copia := *hashSucesor
		t.hashSucesor = &copia
	}
	return t
}

// Hash devuelve el hash SHA-256 de este token de refresco.
func (t TokenRefrescoEmitido) Hash() HashTokenRefresco { return t.hash }

// Generacion devuelve el número de rotación de este token dentro de la
// cadena de la sesión (0 para el primero, emitido en IniciarSesion).
func (t TokenRefrescoEmitido) Generacion() int { return t.generacion }

// EmitidoEn devuelve cuándo se emitió este token.
func (t TokenRefrescoEmitido) EmitidoEn() time.Time { return t.emitidoEn }

// ExpiraEn devuelve la expiración propia de este token, acotada por la
// vida absoluta de la sesión (nunca por sí sola: ver Sesion.Rotar).
func (t TokenRefrescoEmitido) ExpiraEn() time.Time { return t.expiraEn }

// ConsumidoEn devuelve cuándo se consumió este token (al rotarlo) y un
// booleano que indica si ya fue consumido.
func (t TokenRefrescoEmitido) ConsumidoEn() (time.Time, bool) {
	if t.consumidoEn == nil {
		return time.Time{}, false
	}
	return *t.consumidoEn, true
}

// HashSucesor devuelve el hash del token que lo reemplazó en la rotación, y
// un booleano que indica si tiene sucesor. Documenta la cadena de rotación
// y permite reconstruir el linaje completo de una sesión comprometida.
func (t TokenRefrescoEmitido) HashSucesor() (HashTokenRefresco, bool) {
	if t.hashSucesor == nil {
		return HashTokenRefresco{}, false
	}
	return *t.hashSucesor, true
}

// EstaConsumido indica si este token ya fue rotado (INV-ACC-05: no existe
// la reutilización legítima).
func (t TokenRefrescoEmitido) EstaConsumido() bool { return t.consumidoEn != nil }

// EstaExpirado indica si, a la hora dada, este token superó su ventana de
// vigencia.
func (t TokenRefrescoEmitido) EstaExpirado(ahora time.Time) bool {
	return ahora.After(t.expiraEn)
}

// Sesion es el agregado raíz del contexto Acceso: representa el episodio de
// acceso de un Usuario, desde que se emite hasta que se revoca o expira.
// Contiene la cadena de rotación de sus tokens de refresco (al menos el
// vigente y, tras una rotación, el que se acaba de consumir) y las ventanas
// de expiración por inactividad y absoluta. Toda mutación ocurre por un
// método de negocio; no hay campos exportados ni setters, y los getters
// devuelven copias de valores, nunca punteros internos (INV-ACC-09).
//
// "revocada" y "expirada" son estados terminales: una sesión nunca vuelve a
// "activa" (INV-ACC-07). Volver a entrar siempre crea un IDSesion nuevo.
type Sesion struct {
	id        IDSesion
	usuarioID IDUsuario
	estado    EstadoSesion

	generacion              int
	refrescoVigente         *TokenRefrescoEmitido
	refrescoRecienConsumido *TokenRefrescoEmitido

	origenCreacion OrigenSolicitud

	creadaEn            time.Time
	actualizadaEn       time.Time
	ultimaRenovacionEn  *time.Time
	expiraInactividadEn time.Time
	expiraAbsolutoEn    time.Time

	revocadaEn       *time.Time
	motivoRevocacion MotivoRevocacion

	eventos []EventoDominio
}

// IniciarSesion crea un nuevo agregado Sesion en estado activa
// (INV-ACC-02: solo se invoca tras un SujetoAutenticado exitoso de
// Identidad, nunca verifica contraseñas por sí misma), con generacion=0 y
// las ventanas de expiración calculadas a partir de politica. Acumula el
// evento SesionIniciada. Todavía no tiene ningún token de refresco: eso lo
// fija EmitirPrimerRefresco, un paso separado (§3.1 del diseño, pasos 4 y
// 5) porque el token en claro se genera fuera del dominio (puerto
// GeneradorTokensRefresco) y solo su hash entra al agregado.
func IniciarSesion(id IDSesion, usuarioID IDUsuario, origen OrigenSolicitud, ahora time.Time, politica PoliticaSesion) (*Sesion, error) {
	if id.EsVacio() {
		return nil, &ErrIDSesionInvalido{Motivo: "no puede estar vacío"}
	}
	if usuarioID.EsVacio() {
		return nil, &ErrIDUsuarioInvalido{Motivo: "no puede estar vacío"}
	}
	expiraAbsolutoEn := ahora.Add(politica.VidaAbsolutaSesion())
	expiraInactividadEn := minTime(ahora.Add(politica.InactividadMaxima()), expiraAbsolutoEn)
	s := &Sesion{
		id:                  id,
		usuarioID:           usuarioID,
		estado:              EstadoSesionActiva,
		generacion:          0,
		origenCreacion:      origen,
		creadaEn:            ahora,
		actualizadaEn:       ahora,
		expiraInactividadEn: expiraInactividadEn,
		expiraAbsolutoEn:    expiraAbsolutoEn,
	}
	s.agregarEvento(NuevoSesionIniciada(id, usuarioID, expiraAbsolutoEn, ahora))
	return s, nil
}

// Reconstituir reconstruye un agregado Sesion a partir de datos ya
// validados y persistidos (p. ej. una fila de la tabla sesiones mapeada por
// el adaptador Postgres, con su token de refresco vigente ya resuelto). A
// diferencia de IniciarSesion, no acumula eventos: no representa una
// operación de negocio nueva, sino la rehidratación de una ya ocurrida.
// refrescoVigente puede ir nil solo en escenarios de reconstrucción parcial
// (p. ej. una proyección de solo lectura); los métodos de negocio que lo
// requieren (Rotar) fallan explícitamente si falta.
func Reconstituir(
	id IDSesion,
	usuarioID IDUsuario,
	estado EstadoSesion,
	generacion int,
	refrescoVigente *TokenRefrescoEmitido,
	origenCreacion OrigenSolicitud,
	creadaEn time.Time,
	actualizadaEn time.Time,
	ultimaRenovacionEn *time.Time,
	expiraInactividadEn time.Time,
	expiraAbsolutoEn time.Time,
	revocadaEn *time.Time,
	motivoRevocacion MotivoRevocacion,
) *Sesion {
	s := &Sesion{
		id:                  id,
		usuarioID:           usuarioID,
		estado:              estado,
		generacion:          generacion,
		origenCreacion:      origenCreacion,
		creadaEn:            creadaEn,
		actualizadaEn:       actualizadaEn,
		expiraInactividadEn: expiraInactividadEn,
		expiraAbsolutoEn:    expiraAbsolutoEn,
		motivoRevocacion:    motivoRevocacion,
	}
	if refrescoVigente != nil {
		copia := *refrescoVigente
		s.refrescoVigente = &copia
	}
	if ultimaRenovacionEn != nil {
		copia := *ultimaRenovacionEn
		s.ultimaRenovacionEn = &copia
	}
	if revocadaEn != nil {
		copia := *revocadaEn
		s.revocadaEn = &copia
	}
	return s
}

// --- getters (copias de valor, nunca punteros internos: INV-ACC-09) -------

// ID devuelve el identificador de la sesión.
func (s *Sesion) ID() IDSesion { return s.id }

// UsuarioID devuelve el identificador del usuario dueño de la sesión.
func (s *Sesion) UsuarioID() IDUsuario { return s.usuarioID }

// Estado devuelve el estado actual del ciclo de vida de la sesión.
func (s *Sesion) Estado() EstadoSesion { return s.estado }

// Generacion devuelve el número de rotación vigente de la sesión.
func (s *Sesion) Generacion() int { return s.generacion }

// RefrescoVigente devuelve el token de refresco actualmente vigente y un
// booleano que indica si existe (falso antes de EmitirPrimerRefresco).
func (s *Sesion) RefrescoVigente() (TokenRefrescoEmitido, bool) {
	if s.refrescoVigente == nil {
		return TokenRefrescoEmitido{}, false
	}
	return *s.refrescoVigente, true
}

// RefrescoRecienConsumido devuelve el token de refresco que la última
// llamada a Rotar, en esta instancia en memoria, acaba de marcar como
// consumido, y un booleano que indica si hay uno. Es lo que
// RepositorioSesiones.Guardar necesita para persistir, en la misma
// transacción, tanto el UPDATE del token consumido como el INSERT del
// nuevo vigente (§2.2 del diseño: "el token recién emitido y el consumido
// en la misma rotación").
func (s *Sesion) RefrescoRecienConsumido() (TokenRefrescoEmitido, bool) {
	if s.refrescoRecienConsumido == nil {
		return TokenRefrescoEmitido{}, false
	}
	return *s.refrescoRecienConsumido, true
}

// OrigenCreacion devuelve el OrigenSolicitud de la creación de la sesión.
// No hay sesiones anónimas ni sin procedencia forense (INV-ACC-01).
func (s *Sesion) OrigenCreacion() OrigenSolicitud { return s.origenCreacion }

// CreadaEn devuelve la marca de tiempo de creación de la sesión. Es también
// el instante de autenticación original (claim auth_time), que no cambia
// con las renovaciones.
func (s *Sesion) CreadaEn() time.Time { return s.creadaEn }

// ActualizadaEn devuelve la marca de tiempo de la última mutación del
// agregado (INV-ACC-10: siempre con la hora provista por el puerto Reloj).
func (s *Sesion) ActualizadaEn() time.Time { return s.actualizadaEn }

// UltimaRenovacionEn devuelve la marca de tiempo de la última rotación
// exitosa y un booleano que indica si la sesión se renovó alguna vez.
func (s *Sesion) UltimaRenovacionEn() (time.Time, bool) {
	if s.ultimaRenovacionEn == nil {
		return time.Time{}, false
	}
	return *s.ultimaRenovacionEn, true
}

// ExpiraInactividadEn devuelve la ventana deslizante de inactividad
// vigente: min(última renovación + inactividadMaxima, ExpiraAbsolutoEn).
func (s *Sesion) ExpiraInactividadEn() time.Time { return s.expiraInactividadEn }

// ExpiraAbsolutoEn devuelve la vida absoluta de la sesión, fijada en su
// creación y que nunca se extiende por renovación (INV-ACC-16).
func (s *Sesion) ExpiraAbsolutoEn() time.Time { return s.expiraAbsolutoEn }

// RevocadaEn devuelve la marca de tiempo de la revocación y un booleano que
// indica si la sesión está revocada.
func (s *Sesion) RevocadaEn() (time.Time, bool) {
	if s.revocadaEn == nil {
		return time.Time{}, false
	}
	return *s.revocadaEn, true
}

// MotivoRevocacion devuelve el motivo de la revocación y un booleano que
// indica si hay uno (INV-ACC-08: toda revocación lleva motivo).
func (s *Sesion) MotivoRevocacion() (MotivoRevocacion, bool) {
	if s.motivoRevocacion.EsVacio() {
		return MotivoRevocacion{}, false
	}
	return s.motivoRevocacion, true
}

// --- mutaciones de negocio --------------------------------------------------

// EmitirPrimerRefresco fija el primer token de refresco de la sesión
// (generacion 0), inmediatamente después de IniciarSesion (§3.1 del
// diseño, paso 5). No emite un evento propio: SesionIniciada ya cubre la
// creación completa del episodio de acceso.
func (s *Sesion) EmitirPrimerRefresco(hash HashTokenRefresco, ahora time.Time, politica PoliticaSesion) error {
	if s.refrescoVigente != nil {
		return &ErrRefrescoYaEmitido{}
	}
	if hash.EsVacio() {
		return &ErrHashTokenRefrescoInvalido{Motivo: "no puede estar vacío"}
	}
	expira := minTime(ahora.Add(politica.VidaTokenRefresco()), s.expiraAbsolutoEn)
	t := nuevoTokenRefrescoEmitido(hash, s.generacion, ahora, expira)
	s.refrescoVigente = &t
	s.actualizadaEn = ahora
	return nil
}

// Rotar consume el token de refresco vigente y emite uno nuevo: marca el
// anterior como consumido con hashSucesor apuntando al nuevo, incrementa
// generacion, recalcula ExpiraInactividadEn como
// min(ahora + inactividadMaxima, ExpiraAbsolutoEn) (INV-ACC-16, servicio de
// dominio PoliticaRotacion) y acumula SesionRenovada. Es la única forma
// legítima de reutilizar un token de refresco: consumirlo y sustituirlo,
// nunca reusarlo (INV-ACC-05). Requiere que la sesión esté activa y que ya
// tenga un refresco vigente; el caso de uso debe haber llamado
// PuedeRenovarse antes para decidir si corresponde intentar rotar.
func (s *Sesion) Rotar(hashNuevo HashTokenRefresco, ahora time.Time, politica PoliticaSesion) error {
	if !s.estado.EsIgual(EstadoSesionActiva) {
		return &ErrTransicionEstadoSesionInvalida{Origen: s.estado, Destino: EstadoSesionActiva}
	}
	if s.refrescoVigente == nil {
		return &ErrRefrescoNoEmitido{}
	}
	if hashNuevo.EsVacio() {
		return &ErrHashTokenRefrescoInvalido{Motivo: "no puede estar vacío"}
	}

	consumido := *s.refrescoVigente
	consumidoEn := ahora
	consumido.consumidoEn = &consumidoEn
	sucesor := hashNuevo
	consumido.hashSucesor = &sucesor

	nuevaGeneracion := s.generacion + 1
	expiraRefresco := minTime(ahora.Add(politica.VidaTokenRefresco()), s.expiraAbsolutoEn)
	nuevo := nuevoTokenRefrescoEmitido(hashNuevo, nuevaGeneracion, ahora, expiraRefresco)

	s.refrescoRecienConsumido = &consumido
	s.refrescoVigente = &nuevo
	s.generacion = nuevaGeneracion
	s.expiraInactividadEn = CalcularExpiraInactividad(ahora, s.expiraAbsolutoEn, politica)
	renovadaEn := ahora
	s.ultimaRenovacionEn = &renovadaEn
	s.actualizadaEn = ahora

	s.agregarEvento(NuevoSesionRenovada(s.id, s.usuarioID, nuevaGeneracion, ahora))
	return nil
}

// Revocar transiciona la sesión de activa a revocada (terminal,
// INV-ACC-07) y registra motivo y momento. motivo es obligatorio y debe
// venir del catálogo cerrado (INV-ACC-08). No acumula un evento por sí
// misma: a diferencia de IniciarSesion/Rotar, el evento a auditar depende
// de contexto que esta capa no tiene (si la revocación fue iniciada por el
// propio usuario -> SesionCerrada, por el sistema -> SesionRevocada, o es
// consecuencia de un reuso detectado -> ReusoRefrescoDetectado); lo
// construye acceso/aplicacion con los constructores de eventos.go. Ver la
// nota de diseño en el doc comment de EventoDominio.
func (s *Sesion) Revocar(motivo MotivoRevocacion, ahora time.Time) error {
	if !s.estado.EsIgual(EstadoSesionActiva) {
		return &ErrTransicionEstadoSesionInvalida{Origen: s.estado, Destino: EstadoSesionRevocada}
	}
	if motivo.EsVacio() {
		return &ErrMotivoRevocacionRequerido{}
	}
	s.estado = EstadoSesionRevocada
	revocadaEn := ahora
	s.revocadaEn = &revocadaEn
	s.motivoRevocacion = motivo
	s.actualizadaEn = ahora
	return nil
}

// MarcarExpirada transiciona la sesión de activa a expirada (terminal,
// INV-ACC-07). Es la detección perezosa de expiración: el caso de uso la
// invoca cuando PuedeRenovarse devolvió ErrSesionExpirada (§3.2 del
// diseño, paso 4), no hay un job de fondo que expire sesiones
// proactivamente. No acumula evento propio: la auditoría de ese intento de
// renovación ya la cubre RenovacionRechazada, construido por el caso de
// uso.
func (s *Sesion) MarcarExpirada(ahora time.Time) error {
	if !s.estado.EsIgual(EstadoSesionActiva) {
		return &ErrTransicionEstadoSesionInvalida{Origen: s.estado, Destino: EstadoSesionExpirada}
	}
	s.estado = EstadoSesionExpirada
	s.actualizadaEn = ahora
	return nil
}

// PuedeRenovarse implementa el servicio de dominio EvaluadorVentanas (§1.4
// del diseño): función pura que aplica, en este orden, estado ≠ activa ->
// ErrSesionRevocada; ahora > ExpiraAbsolutoEn -> ErrSesionExpirada; ahora >
// ExpiraInactividadEn -> ErrSesionExpirada. El orden importa para el motivo
// que termina en la auditoría: una sesión ya marcada revocada o expirada en
// la base de datos siempre se reporta como ErrSesionRevocada (ya no hay
// nada que "expirar" nuevamente); ErrSesionExpirada es exclusivamente la
// detección en caliente de una sesión que sigue en estado activa pero cuyo
// reloj ya se agotó, y es la señal para que el caso de uso llame después a
// MarcarExpirada.
func (s *Sesion) PuedeRenovarse(ahora time.Time) error {
	if !s.estado.EsIgual(EstadoSesionActiva) {
		motivo := ""
		if m, ok := s.MotivoRevocacion(); ok {
			motivo = m.Valor()
		}
		return &ErrSesionRevocada{Motivo: motivo}
	}
	if ahora.After(s.expiraAbsolutoEn) {
		return &ErrSesionExpirada{}
	}
	if ahora.After(s.expiraInactividadEn) {
		return &ErrSesionExpirada{}
	}
	return nil
}

// EstaViva indica si, a la hora dada, la sesión sigue activa y dentro de
// sus dos ventanas de expiración. Es la comprobación que usa ValidarAcceso
// cuando ExigirSesionViva=true (§3.3 del diseño), para operaciones de alto
// valor donde una ventana de revocación de hasta 10 minutos no es
// aceptable.
func (s *Sesion) EstaViva(ahora time.Time) bool {
	return s.PuedeRenovarse(ahora) == nil
}

// metodosAutenticacionFase1 es el valor fijo del claim amr mientras el
// contexto Acceso no soporte un segundo factor (fase 2, "otp": ver §1.4 del
// diseño, estado pendiente_segundo_factor deliberadamente fuera del MVP).
// Vive aquí, no como campo de Sesion, porque hoy el agregado no tiene
// ningún dato de negocio que lo haga variar: es una decisión de
// implementación documentada, no una invariante del diseño.
var metodosAutenticacionFase1 = []string{"pwd"}

// ReclamacionesParaToken construye el ReclamacionesAcceso que
// FirmadorTokensAcceso debe firmar para un nuevo token de acceso de esta
// sesión (§1.2 del diseño: el token de acceso es una proyección firmada del
// estado de la sesión, no una entidad con estado propio). auth_time es
// siempre CreadaEn(): el instante de la autenticación original de la
// sesión, que no se actualiza en cada renovación (§7 del diseño).
//
// Nota de implementación: el diagrama de clases del diseño abrevia esta
// firma a (jti, ahora, politica) sin emisor/audiencia, igual que omite
// ctx context.Context en todas partes por ser un detalle de puerto; emisor
// y audiencia son configuración (ACCESO_EMISOR/ACCESO_AUDIENCIA) que
// acceso/dominio no puede leer por sí mismo (INV-ACC-18: nada de config ni
// de E/S en el dominio), así que el caso de uso debe suministrarlos.
func (s *Sesion) ReclamacionesParaToken(jti IDTokenAcceso, emisor, audiencia string, ahora time.Time, politica PoliticaSesion) (ReclamacionesAcceso, error) {
	return NuevasReclamacionesAcceso(
		emisor,
		s.usuarioID,
		audiencia,
		s.id,
		jti,
		metodosAutenticacionFase1,
		s.creadaEn,
		ahora,
		ahora.Add(politica.VidaTokenAcceso()),
		1,
	)
}

// --- eventos ------------------------------------------------------------

// EventosPendientes drena los eventos acumulados por el agregado: los
// devuelve y vacía el buffer interno. Solo SesionIniciada (IniciarSesion) y
// SesionRenovada (Rotar) se acumulan aquí; ver el doc comment de
// EventoDominio para el resto. El caso de uso debe llamarlo una sola vez,
// tras persistir el agregado dentro de la misma UnidadDeTrabajo que la
// auditoría (ADR 0005).
func (s *Sesion) EventosPendientes() []EventoDominio {
	eventos := s.eventos
	s.eventos = nil
	return eventos
}

func (s *Sesion) agregarEvento(e EventoDominio) {
	s.eventos = append(s.eventos, e)
}
