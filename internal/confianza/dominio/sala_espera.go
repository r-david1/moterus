package dominio

import (
	"strings"
	"time"

	"golang.org/x/text/unicode/norm"
)

// longitudMinimaAliasSala y longitudMaximaAliasSala acotan AliasSala (tabla
// 1.3 del diseño).
const (
	longitudMinimaAliasSala = 3
	longitudMaximaAliasSala = 48
)

// AliasSala es el value object que representa el identificador legible y
// único de una SalaDeEspera: es lo que aparece en la URL de ingreso
// (/confianza/salas-espera/{aliasSala}/tickets), lo que un frontend
// hardcodea para un evento conocido (p. ej. "inscripciones-2026"). No es un
// secreto y no autoriza nada (tabla 1.3 del diseño). Sigue al pie de la
// letra las reglas de AliasOrganizacion en tenencia/dominio (TrimSpace,
// NFC, minúsculas, [a-z0-9-], 3–48, sin '-' en los bordes ni '--'), salvo la
// lista de alias reservados: una sala de espera no comparte namespace de
// rutas con las organizaciones, así que esa lista no aplica aquí.
type AliasSala struct {
	valor string
}

// NuevoAliasSala valida y normaliza un alias crudo: TrimSpace, NFC,
// minúsculas completas, solo [a-z0-9-], longitud entre 3 y 48, no empieza
// ni termina en '-', sin '--' consecutivos.
//
// Nota de implementación: la normalización Unicode usa NFC completo vía
// golang.org/x/text/unicode/norm — la misma excepción documentada, única y
// explícita al "cero dependencias externas en el dominio" que ya usan
// identidad/dominio.Correo y tenencia/dominio.AliasOrganizacion.
func NuevoAliasSala(crudo string) (AliasSala, error) {
	v := strings.TrimSpace(crudo)
	if v == "" {
		return AliasSala{}, &ErrAliasSalaInvalido{Motivo: "no puede estar vacío"}
	}
	v = norm.NFC.String(v)
	v = strings.ToLower(v)

	if utf8RuneCountAlias(v) < longitudMinimaAliasSala || utf8RuneCountAlias(v) > longitudMaximaAliasSala {
		return AliasSala{}, &ErrAliasSalaInvalido{Motivo: "la longitud debe estar entre 3 y 48 caracteres"}
	}
	for _, r := range v {
		if !esCaracterDeAliasSala(r) {
			return AliasSala{}, &ErrAliasSalaInvalido{Motivo: "solo se permiten letras minúsculas, dígitos y guiones"}
		}
	}
	if strings.HasPrefix(v, "-") || strings.HasSuffix(v, "-") {
		return AliasSala{}, &ErrAliasSalaInvalido{Motivo: "no puede empezar ni terminar en guion"}
	}
	if strings.Contains(v, "--") {
		return AliasSala{}, &ErrAliasSalaInvalido{Motivo: "no puede contener guiones consecutivos"}
	}
	return AliasSala{valor: v}, nil
}

// Normalizado devuelve la forma canónica (minúsculas, NFC) del alias.
func (a AliasSala) Normalizado() string { return a.valor }

// String implementa fmt.Stringer devolviendo la forma normalizada. No es un
// secreto: viaja en la ruta HTTP de ingreso.
func (a AliasSala) String() string { return a.valor }

// EsVacio indica si el value object nunca fue construido (zero value).
func (a AliasSala) EsVacio() bool { return a.valor == "" }

// EsIgual compara dos alias por su forma normalizada.
func (a AliasSala) EsIgual(otro AliasSala) bool { return a.valor == otro.valor }

func esCaracterDeAliasSala(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-'
}

func utf8RuneCountAlias(s string) int { return len([]rune(s)) }

// --- SalaDeEspera --------------------------------------------------------

// SalaDeEspera es el agregado raíz del contexto Confianza que representa
// una cola de acceso virtual sobre una ruta protegida (§1.2/§1.3 del
// diseño). Es un agregado de configuración operativa: se muta pocas veces
// (abrir, cambiar el ritmo, drenar, cerrar) y se lee muchísimo, pero nunca
// desde Postgres en el camino caliente (INV-COLA-08) — la proyección a
// Redis (fuera del dominio) es la que atiende cada ingreso, consulta y
// reclamo.
//
// Toda mutación ocurre por un método de negocio; no hay campos exportados
// ni setters, y los getters devuelven copias de valores, nunca punteros
// internos.
type SalaDeEspera struct {
	id         IDSalaDeEspera
	alias      AliasSala
	alcance    AlcanceSala
	ruta       RutaProtegida
	estado     EstadoSala
	politica   PoliticaSala
	cursorBase int64
	relojDesde time.Time
	creadaPor  *IDUsuario
	creadaEn   time.Time
	abiertaEn  *time.Time
	cerradaEn  *time.Time

	eventos []EventoDominio
}

// NuevaSalaDeEspera construye una SalaDeEspera nueva en estado
// EstadoSalaProgramada (§3.1 del diseño, paso 1-2: construir los VOs y
// verificar que la ruta admite el alcance). creadaPor va nil para una sala
// de alcance sistema (se opera fuera de la API, §7.2); es obligatorio para
// una sala org-scoped, mismo criterio que Membresia.AgregarMiembro exige
// otorgadaPor.
func NuevaSalaDeEspera(
	id IDSalaDeEspera,
	alias AliasSala,
	alcance AlcanceSala,
	ruta RutaProtegida,
	politica PoliticaSala,
	creadaPor *IDUsuario,
	ahora time.Time,
) (*SalaDeEspera, error) {
	if id.EsVacio() {
		return nil, &ErrIDSalaDeEsperaInvalido{Motivo: "no puede estar vacío"}
	}
	if alias.EsVacio() {
		return nil, &ErrAliasSalaInvalido{Motivo: "no puede estar vacío"}
	}
	if alcance.EsVacio() {
		return nil, &ErrAlcanceSalaInvalido{Motivo: "no puede estar vacío"}
	}
	if ruta.EsVacio() {
		return nil, &ErrRutaNoProtegible{Motivo: "no puede estar vacía"}
	}
	if politica.EsVacio() {
		return nil, &ErrPoliticaSalaInvalida{Motivo: "no puede estar vacía"}
	}
	if !alcance.EsSistema() && (creadaPor == nil || creadaPor.EsVacio()) {
		return nil, &ErrIDUsuarioInvalido{Motivo: "creadaPor es obligatorio para una sala de alcance organizacion"}
	}
	if !ruta.AdmiteAlcance(alcance) {
		return nil, &ErrRutaNoProtegible{Motivo: "la ruta " + ruta.String() + " no admite el alcance " + alcance.Tipo().String()}
	}

	s := &SalaDeEspera{
		id:       id,
		alias:    alias,
		alcance:  alcance,
		ruta:     ruta,
		estado:   EstadoSalaProgramada,
		politica: politica,
		creadaEn: ahora,
	}
	if creadaPor != nil && !creadaPor.EsVacio() {
		copia := *creadaPor
		s.creadaPor = &copia
	}
	return s, nil
}

// ReconstituirSalaDeEspera reconstruye un agregado SalaDeEspera a partir de
// datos ya validados y persistidos (fila de la tabla salas_espera, §6.1 del
// diseño). A diferencia de NuevaSalaDeEspera, no valida invariantes de
// negocio ni acumula eventos: no representa una operación nueva, sino la
// rehidratación de una ya ocurrida. Postgres es la fuente de verdad de esta
// reconstrucción (INV-COLA-12): con estos campos alcanza para recalcular
// CursorEn/TurnoDe sin ninguna dependencia de Redis.
func ReconstituirSalaDeEspera(
	id IDSalaDeEspera,
	alias AliasSala,
	alcance AlcanceSala,
	ruta RutaProtegida,
	estado EstadoSala,
	politica PoliticaSala,
	cursorBase int64,
	relojDesde time.Time,
	creadaPor *IDUsuario,
	creadaEn time.Time,
	abiertaEn *time.Time,
	cerradaEn *time.Time,
) *SalaDeEspera {
	s := &SalaDeEspera{
		id:         id,
		alias:      alias,
		alcance:    alcance,
		ruta:       ruta,
		estado:     estado,
		politica:   politica,
		cursorBase: cursorBase,
		relojDesde: relojDesde,
		creadaEn:   creadaEn,
	}
	if creadaPor != nil {
		copia := *creadaPor
		s.creadaPor = &copia
	}
	if abiertaEn != nil {
		copia := *abiertaEn
		s.abiertaEn = &copia
	}
	if cerradaEn != nil {
		copia := *cerradaEn
		s.cerradaEn = &copia
	}
	return s
}

// --- getters (copias de valor, nunca punteros internos) --------------------

// ID devuelve el identificador de la sala.
func (s *SalaDeEspera) ID() IDSalaDeEspera { return s.id }

// Alias devuelve el alias público de la sala.
func (s *SalaDeEspera) Alias() AliasSala { return s.alias }

// Alcance devuelve el alcance (sistema | organizacion) de la sala.
func (s *SalaDeEspera) Alcance() AlcanceSala { return s.alcance }

// Ruta devuelve la ruta protegida por esta sala.
func (s *SalaDeEspera) Ruta() RutaProtegida { return s.ruta }

// Estado devuelve el estado actual del ciclo de vida de la sala.
func (s *SalaDeEspera) Estado() EstadoSala { return s.estado }

// Politica devuelve la configuración operativa vigente de la sala.
func (s *SalaDeEspera) Politica() PoliticaSala { return s.politica }

// CursorBase devuelve el ancla del reloj de admisión (§1.5 del diseño): el
// valor del cursor en el instante RelojDesde.
func (s *SalaDeEspera) CursorBase() int64 { return s.cursorBase }

// RelojDesde devuelve el instante desde el que se cuenta el reloj de
// admisión vigente.
func (s *SalaDeEspera) RelojDesde() time.Time { return s.relojDesde }

// CreadaPor devuelve el identificador de quien creó la sala, y un booleano
// que indica si hay uno (vacío para una sala de alcance sistema, operada
// fuera de la API, §7.2).
func (s *SalaDeEspera) CreadaPor() (IDUsuario, bool) {
	if s.creadaPor == nil {
		return IDUsuario{}, false
	}
	return *s.creadaPor, true
}

// CreadaEn devuelve la marca de tiempo de creación de la sala.
func (s *SalaDeEspera) CreadaEn() time.Time { return s.creadaEn }

// AbiertaEn devuelve la marca de tiempo de la apertura más reciente, y un
// booleano que indica si la sala llegó a abrirse alguna vez.
func (s *SalaDeEspera) AbiertaEn() (time.Time, bool) {
	if s.abiertaEn == nil {
		return time.Time{}, false
	}
	return *s.abiertaEn, true
}

// CerradaEn devuelve la marca de tiempo del cierre, y un booleano que
// indica si la sala está cerrada.
func (s *SalaDeEspera) CerradaEn() (time.Time, bool) {
	if s.cerradaEn == nil {
		return time.Time{}, false
	}
	return *s.cerradaEn, true
}

// --- mutaciones de negocio --------------------------------------------------

// Abrir transiciona la sala a EstadoSalaAbierta (§1.6 del diseño). Desde
// EstadoSalaProgramada es la primera apertura: el reloj de admisión
// arranca desde cero (cursorBase=0, relojDesde=ahora). Desde
// EstadoSalaDrenando es una reapertura durante el drenaje (p. ej. un
// segundo lote): el cursor se recalcula a su valor actual antes de fijar el
// nuevo ancla, para que sea continuo (nadie salta de golpe, nadie
// retrocede) — mismo criterio de continuidad que CambiarRitmo (§1.5).
// Acumula el evento SalaDeEsperaAbierta.
func (s *SalaDeEspera) Abrir(ahora time.Time) error {
	if !s.estado.PuedeTransicionarA(EstadoSalaAbierta) {
		return &ErrTransicionEstadoSalaInvalida{Origen: s.estado, Destino: EstadoSalaAbierta}
	}
	if s.estado.EsIgual(EstadoSalaProgramada) {
		s.cursorBase = 0
	} else {
		s.cursorBase = s.CursorEn(ahora)
	}
	s.relojDesde = ahora
	s.estado = EstadoSalaAbierta
	abiertaEn := ahora
	s.abiertaEn = &abiertaEn
	s.agregarEvento(NuevoSalaDeEsperaAbierta(s.id, s.alias, s.alcance, s.ruta, s.politica, ahora))
	return nil
}

// CambiarRitmo actualiza el ritmo de admisión de una sala vigente (abierta
// o drenando; §3.2 del diseño: es el caso de uso que se usa DURANTE el
// pico). Recalcula cursorBase = CursorEn(ahora) y relojDesde = ahora, para
// que el cursor sea continuo en el cambio (§1.5). Es un no-op idempotente
// (sin mutación ni evento) si nuevo es igual al ritmo vigente, mismo
// criterio que Membresia.CambiarRol en tenencia/dominio.
//
// longitudCola es la longitud aproximada de la cola en el momento del
// cambio: el dominio no la consulta por sí mismo (INV-COLA-08, sin E/S), la
// suministra el caso de uso a partir de EstadoDeCola.Instantanea — mismo
// patrón que Membresia.CambiarRol recibe propietariosActivos como
// parámetro en vez de contarlos. Se audita en el evento
// RitmoDeAdmisionCambiado porque es "el dato que un post-mortem del evento
// va a pedir primero" (§3.2 del diseño).
func (s *SalaDeEspera) CambiarRitmo(nuevo RitmoAdmision, longitudCola int64, ahora time.Time) error {
	if !s.estaVigente() {
		return &ErrSalaNoVigente{Estado: s.estado}
	}
	if nuevo.EsVacio() {
		return &ErrRitmoAdmisionInvalido{Motivo: "no puede estar vacío"}
	}
	anterior := s.politica.RitmoAdmision()
	if anterior.EsIgual(nuevo) {
		return nil
	}
	cursorAlCambiar := s.CursorEn(ahora)
	s.cursorBase = cursorAlCambiar
	s.relojDesde = ahora
	s.politica = s.politica.ConRitmoAdmision(nuevo)
	s.agregarEvento(NuevoRitmoDeAdmisionCambiado(s.id, anterior, nuevo, cursorAlCambiar, longitudCola, ahora))
	return nil
}

// Drenar transiciona la sala a EstadoSalaDrenando (§1.6 del diseño): deja
// de admitir ingresos nuevos, pero los tickets vivos siguen avanzando (el
// reloj de admisión no se toca). ingresosTotales/admitidosTotales los
// suministra el caso de uso desde EstadoDeCola.Instantanea (mismo criterio
// de "parámetro, no E/S del dominio" que CambiarRitmo). Acumula el evento
// SalaDeEsperaCerrada con Destino=EstadoSalaDrenando (§1.7 del diseño: un
// solo evento cubre ambos destinos, distinguidos por su campo Destino).
func (s *SalaDeEspera) Drenar(ingresosTotales, admitidosTotales int64, ahora time.Time) error {
	if !s.estado.PuedeTransicionarA(EstadoSalaDrenando) {
		return &ErrTransicionEstadoSalaInvalida{Origen: s.estado, Destino: EstadoSalaDrenando}
	}
	s.estado = EstadoSalaDrenando
	s.agregarEvento(NuevoSalaDeEsperaCerrada(s.id, EstadoSalaDrenando, ingresosTotales, admitidosTotales, ahora))
	return nil
}

// Cerrar transiciona la sala a EstadoSalaCerrada (terminal, §1.6 del
// diseño), alcanzable desde abierta o drenando. Acumula el evento
// SalaDeEsperaCerrada con Destino=EstadoSalaCerrada.
func (s *SalaDeEspera) Cerrar(ingresosTotales, admitidosTotales int64, ahora time.Time) error {
	if !s.estado.PuedeTransicionarA(EstadoSalaCerrada) {
		return &ErrTransicionEstadoSalaInvalida{Origen: s.estado, Destino: EstadoSalaCerrada}
	}
	s.estado = EstadoSalaCerrada
	cerradaEn := ahora
	s.cerradaEn = &cerradaEn
	s.agregarEvento(NuevoSalaDeEsperaCerrada(s.id, EstadoSalaCerrada, ingresosTotales, admitidosTotales, ahora))
	return nil
}

func (s *SalaDeEspera) estaVigente() bool {
	return s.estado.EsIgual(EstadoSalaAbierta) || s.estado.EsIgual(EstadoSalaDrenando)
}

// --- aritmética del cursor (§1.5 del diseño) ---------------------------

// CursorEn calcula, de forma pura y determinista, el cursor de admisión en
// el instante ahora:
//
//	cursor(t) = cursorBase + floor((t - relojDesde) × ritmoAdmision)
//
// Réplica en dominio del cálculo que el script Lua `ingresar`/`reclamar`
// hace en Redis (§6.2 del diseño), para que sea testeable pasando ahora
// como parámetro (mismo patrón que INV-TEN-06 en tenencia/dominio, probado
// pasando el conteo de propietarios) y exista un punto de comparación en el
// test de consistencia dominio↔Lua. Un ahora anterior a relojDesde (reloj
// desincronizado) se trata como "sin avance todavía", nunca como cursor
// negativo: es la lectura conservadora, en la misma dirección de fallo que
// documenta INV-COLA-04.
func (s *SalaDeEspera) CursorEn(ahora time.Time) int64 {
	ritmo := int64(s.politica.RitmoAdmision().PorSegundo())
	ms := ahora.Sub(s.relojDesde).Milliseconds()
	if ms < 0 {
		ms = 0
	}
	return s.cursorBase + (ms*ritmo)/1000
}

// TurnoDe calcula, de forma pura y determinista, el instante en que un
// rango dado llega a su turno:
//
//	turnoDe(rango) = relojDesde + (rango - cursorBase) / ritmoAdmision
//
// No depende de ahora ni de cuántos abandonen la cola (INV-COLA-05): es
// exclusivamente función de rango y de la configuración vigente
// (cursorBase, relojDesde, ritmoAdmision). Un rango ya alcanzado por el
// cursorBase (delta ≤ 0) tiene su turno en relojDesde o antes; se devuelve
// relojDesde por simplicidad, dado que en ese caso el ticket ya está en
// condiciones de ser admitido de todos modos. El redondeo hacia arriba
// (ceil, en milisegundos) replica exactamente al script Lua `reclamar`
// (§6.2 del diseño): un turno nunca se reporta antes de que el cursor
// realmente lo alcance.
func (s *SalaDeEspera) TurnoDe(rango RangoEnCola) time.Time {
	ritmo := int64(s.politica.RitmoAdmision().PorSegundo())
	delta := rango.Valor() - s.cursorBase
	if delta <= 0 {
		return s.relojDesde
	}
	ms := ceilDiv(delta*1000, ritmo)
	return s.relojDesde.Add(time.Duration(ms) * time.Millisecond)
}

// AdmiteIngreso decide si un ingreso nuevo cabe en la sala ahora mismo,
// dado longitud (el último rango ya asignado por el contador de Redis,
// "secuencia" en §6.2 del diseño — el dominio no lo consulta por sí mismo,
// lo recibe como parámetro). Replica las dos guardas del script Lua
// `ingresar`, en el mismo orden: la sala debe estar EstadoSalaAbierta
// (ni programada, ni drenando —que ya no admite ingresos nuevos—, ni
// cerrada) y la cola no debe estar en su capacidad máxima. Este último
// chequeo sobrecuenta a quienes abandonaron sin reclamar (longitud -
// cursor incluye turnos que nunca se van a reclamar), lo cual es aceptado
// a propósito: el techo existe para proteger la memoria de Redis, y
// sobrecontar solo lo hace más conservador (§1.5 del diseño).
func (s *SalaDeEspera) AdmiteIngreso(longitud int64, ahora time.Time) error {
	if !s.estado.EsIgual(EstadoSalaAbierta) {
		return &ErrSalaNoAbierta{Estado: s.estado}
	}
	cursor := s.CursorEn(ahora)
	if longitud-cursor >= s.politica.CapacidadMaximaCola() {
		return &ErrColaLlena{}
	}
	return nil
}

// Clave devuelve la ClaveSala de esta sala: "<alcance>:<ruta>" (tabla 1.3
// del diseño). Es el discriminador de unicidad de INV-COLA-01 y el prefijo
// de todas sus claves de Redis.
func (s *SalaDeEspera) Clave() ClaveSala {
	return nuevaClaveSala(s.alcance, s.ruta)
}

// --- eventos ------------------------------------------------------------

// EventosPendientes drena los eventos acumulados por el agregado: los
// devuelve y vacía el buffer interno. El caso de uso debe llamarlo una sola
// vez, tras persistir el agregado dentro de la misma unidad de trabajo que
// la auditoría (INV-COLA-11).
func (s *SalaDeEspera) EventosPendientes() []EventoDominio {
	eventos := s.eventos
	s.eventos = nil
	return eventos
}

func (s *SalaDeEspera) agregarEvento(e EventoDominio) {
	s.eventos = append(s.eventos, e)
}

// ceilDiv calcula la división entera redondeada hacia arriba de dos enteros
// positivos (a ≥ 0, b > 0). Se usa en TurnoDe para replicar exactamente
// math.ceil(a/b) del script Lua `reclamar` (§6.2 del diseño).
func ceilDiv(a, b int64) int64 {
	if a <= 0 {
		return 0
	}
	return (a + b - 1) / b
}
