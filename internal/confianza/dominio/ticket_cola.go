package dominio

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strings"
	"time"
)

// prefijoTicket marca estructuralmente un ticket de cola de Confianza, para
// poder rechazar de un vistazo (sin tocar Redis) cualquier cosa que no
// tenga esta forma. Mismo criterio que "mot_rt_" en Acceso y "mot_inv_" en
// Tenencia (§1.3 del diseño); es también la mitad estructural de
// INV-COLA-03 (un ticket nunca puede confundirse con un token de otro
// contexto porque no comparte prefijo, formato ni llave de firma — no es
// JWT, no tiene firma, §1.4 del diseño).
const prefijoTicket = "mot_cola_"

// longitudMinimaSecretoTicket es la longitud mínima del secreto que sigue
// al prefijo: base64url sin relleno de 32 bytes de entropía codifica en 43
// caracteres.
const longitudMinimaSecretoTicket = 43

// TicketPlano es un value object efímero que envuelve, en texto claro, el
// ticket que identifica el turno de un cliente en una SalaDeEspera
// (§1.3/§1.4 del diseño). Nunca se persiste (Redis solo guarda su
// HashTicket), nunca se registra en logs, nunca viaja en un evento de
// dominio ni en una fila de auditoría (INV-COLA-07): implementa
// String()/GoString()/MarshalJSON devolviendo "[REDACTADO]", mismo patrón
// que TokenInvitacionPlano en tenencia/dominio y SecretoTOTPPlano en
// identidad/dominio. Sale del proceso exactamente una vez: en la respuesta
// de IngresarASala.
type TicketPlano struct {
	valor string
}

// NuevoTicketPlano valida la forma estructural de un ticket: prefijo
// "mot_cola_" y al menos 43 caracteres base64url de secreto. No consulta
// Redis: es un rechazo barato para tráfico obviamente malformado.
func NuevoTicketPlano(valor string) (TicketPlano, error) {
	v := strings.TrimSpace(valor)
	if v == "" {
		return TicketPlano{}, &ErrTicketPlanoInvalido{Motivo: "no puede estar vacío"}
	}
	if !strings.HasPrefix(v, prefijoTicket) {
		return TicketPlano{}, &ErrTicketPlanoInvalido{Motivo: "prefijo desconocido"}
	}
	secreto := v[len(prefijoTicket):]
	if len(secreto) < longitudMinimaSecretoTicket {
		return TicketPlano{}, &ErrTicketPlanoInvalido{Motivo: "longitud de secreto insuficiente"}
	}
	if !esBase64URLTicket(secreto) {
		return TicketPlano{}, &ErrTicketPlanoInvalido{Motivo: "el secreto contiene caracteres no permitidos"}
	}
	return TicketPlano{valor: v}, nil
}

// Hash calcula el HashTicket (SHA-256, hex) de este ticket en claro. Es la
// única vía por la que un TicketPlano se convierte en algo persistible en
// Redis.
func (t TicketPlano) Hash() HashTicket { return HashearTicket(t) }

// Valor expone el ticket en claro. Solo debe invocarlo el caso de uso
// IngresarASala, para devolverlo al cliente en su única aparición; nunca
// debe registrarse en logs, serializarse ni auditarse (INV-COLA-07).
func (t TicketPlano) Valor() string { return t.valor }

// EsVacio indica si el value object nunca fue construido (zero value).
func (t TicketPlano) EsVacio() bool { return t.valor == "" }

// String redacta el ticket (INV-COLA-07).
func (t TicketPlano) String() string { return "[REDACTADO]" }

// GoString redacta el ticket en el formato %#v.
func (t TicketPlano) GoString() string { return "[REDACTADO]" }

// MarshalJSON redacta el ticket si el value object se serializa por
// descuido.
func (t TicketPlano) MarshalJSON() ([]byte, error) {
	return []byte(`"[REDACTADO]"`), nil
}

func esBase64URLTicket(v string) bool {
	for _, r := range v {
		switch {
		case r >= 'A' && r <= 'Z':
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_':
		default:
			return false
		}
	}
	return true
}

// longitudHashTicket es la longitud en caracteres hexadecimales de un
// digest SHA-256 (32 bytes -> 64 caracteres hex).
const longitudHashTicket = 64

// HashTicket es el value object que envuelve el hash SHA-256 (hex, en
// minúsculas) de un ticket de cola. Es lo único que Redis guarda
// (confianza:cola:<clave>:t:<sha256>, §6.2 del diseño): el ticket en claro
// nunca se persiste (INV-COLA-07).
//
// Por qué SHA-256 y no Argon2id (mismo razonamiento ya usado tres veces en
// el repositorio: identidad, acceso y tenencia): es un secreto aleatorio de
// alta entropía generado por el sistema, no hay ataque de diccionario que
// encarecer.
type HashTicket struct {
	valor string
}

// NuevoHashTicket valida que el valor tenga la forma de un digest SHA-256
// en hexadecimal minúscula (64 caracteres). Se usa para envolver un hash ya
// calculado, típicamente al leerlo de Redis.
func NuevoHashTicket(valor string) (HashTicket, error) {
	v := strings.TrimSpace(valor)
	if len(v) != longitudHashTicket {
		return HashTicket{}, &ErrHashTicketInvalido{Motivo: "longitud distinta de 64 caracteres"}
	}
	for _, r := range v {
		if !esHexadecimalMinusculaTicket(r) {
			return HashTicket{}, &ErrHashTicketInvalido{Motivo: "no es hexadecimal en minúsculas"}
		}
	}
	return HashTicket{valor: v}, nil
}

// HashearTicket calcula el hash SHA-256 (hex, minúsculas) de un TicketPlano
// ya validado.
func HashearTicket(plano TicketPlano) HashTicket {
	suma := sha256.Sum256([]byte(plano.valor))
	return HashTicket{valor: hex.EncodeToString(suma[:])}
}

// Valor expone el hash en hexadecimal para que el adaptador Redis lo use
// como parte de la clave de estado. No es un secreto de por sí (es un
// digest de un solo sentido).
func (h HashTicket) Valor() string { return h.valor }

// EsVacio indica si el value object nunca fue construido (zero value).
func (h HashTicket) EsVacio() bool { return h.valor == "" }

// EsIgual compara dos hashes en tiempo constante (crypto/subtle), para que
// ninguna comparación de un secreto de alta entropía filtre información por
// temporización.
func (h HashTicket) EsIgual(otro HashTicket) bool {
	return subtle.ConstantTimeCompare([]byte(h.valor), []byte(otro.valor)) == 1
}

// String implementa fmt.Stringer. A diferencia de TicketPlano, el hash no
// es el secreto en sí (es un digest de un solo sentido) y no se redacta.
func (h HashTicket) String() string { return h.valor }

func esHexadecimalMinusculaTicket(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')
}

// --- RangoEnCola / PosicionEnCola --------------------------------------------

// RangoEnCola es el value object que envuelve el ordinal monótono de
// ingreso de un ticket (tabla 1.3 del diseño): es el orden FIFO completo,
// sin timestamps involucrados en decidir quién va primero. Lo asigna
// exactamente una vez el `INCR` de Redis (INV-COLA-14): el dominio nunca lo
// genera por sí mismo, solo lo valida al envolver un valor ya asignado.
type RangoEnCola struct {
	valor int64
}

// NuevoRango valida que el rango sea positivo (≥ 1): el contador de Redis
// arranca en 1 en el primer INCR, nunca en 0.
func NuevoRango(valor int64) (RangoEnCola, error) {
	if valor < 1 {
		return RangoEnCola{}, &ErrRangoEnColaInvalido{Motivo: "debe ser mayor o igual a 1"}
	}
	return RangoEnCola{valor: valor}, nil
}

// Valor devuelve el ordinal de ingreso.
func (r RangoEnCola) Valor() int64 { return r.valor }

// EsVacio indica si el value object nunca fue construido (zero value).
func (r RangoEnCola) EsVacio() bool { return r.valor == 0 }

// PosicionEnCola es el value object derivado que responde "cuántos turnos
// faltan" (tabla 1.3 del diseño): max(0, rango - cursor). Es lo único que
// ve el usuario; no se construye directamente, solo a través de
// TicketDeCola.Posicion.
type PosicionEnCola struct {
	valor int64
}

func nuevaPosicionEnCola(rango RangoEnCola, cursor int64) PosicionEnCola {
	pos := rango.Valor() - cursor
	if pos < 0 {
		pos = 0
	}
	return PosicionEnCola{valor: pos}
}

// Valor devuelve cuántos turnos faltan antes del propio.
func (p PosicionEnCola) Valor() int64 { return p.valor }

// --- EstadoTicket -----------------------------------------------------------

// EstadoTicket es el value object enum que representa el ciclo de vida de
// un TicketDeCola (§1.6 del diseño): esperando -> consumido (terminal). No
// hay un estado "admitido" persistido: la admisión es una propiedad
// derivada de rango ≤ cursor(ahora), no un hecho que alguien escriba (ver
// TicketDeCola.Desenlace).
type EstadoTicket struct {
	valor string
}

var (
	// EstadoTicketEsperando es el estado de nacimiento de todo ticket.
	EstadoTicketEsperando = EstadoTicket{valor: "esperando"}
	// EstadoTicketConsumido es terminal (INV-COLA-06): un ticket se
	// reclama exactamente una vez.
	EstadoTicketConsumido = EstadoTicket{valor: "consumido"}
)

// EstadoTicketDesde valida un valor persistido contra el catálogo cerrado
// de estados.
func EstadoTicketDesde(valor string) (EstadoTicket, error) {
	v := strings.TrimSpace(valor)
	switch v {
	case EstadoTicketEsperando.valor, EstadoTicketConsumido.valor:
		return EstadoTicket{valor: v}, nil
	default:
		return EstadoTicket{}, &ErrEstadoTicketInvalido{Valor: valor}
	}
}

// String devuelve la representación canónica del estado.
func (e EstadoTicket) String() string { return e.valor }

// EsVacio indica si el value object nunca fue construido (zero value).
func (e EstadoTicket) EsVacio() bool { return e.valor == "" }

// EsIgual compara dos estados por su valor.
func (e EstadoTicket) EsIgual(otro EstadoTicket) bool { return e.valor == otro.valor }

// EsTerminal indica si el estado no admite ninguna transición de salida
// (consumido; INV-COLA-06).
func (e EstadoTicket) EsTerminal() bool { return e.EsIgual(EstadoTicketConsumido) }

// PuedeTransicionarA indica si existe una transición legal del estado
// origen (e) al estado destino: solo esperando puede avanzar, y únicamente
// a consumido.
func (e EstadoTicket) PuedeTransicionarA(destino EstadoTicket) bool {
	if e.EsTerminal() {
		return false
	}
	return destino.EsIgual(EstadoTicketConsumido)
}

// --- TicketDeCola -------------------------------------------------------

// TicketDeCola es la entidad efímera del contexto Confianza que representa
// el turno de un cliente en una SalaDeEspera (§1.2/§1.3 del diseño). No es
// parte del agregado SalaDeEspera (una sala tiene, por diseño, cientos de
// miles de tickets simultáneos: cargar la sala para emitir uno sería cargar
// la avalancha entera en memoria) y su estado vive en Redis, no en
// Postgres: nunca se persiste allí, nunca acumula eventos de dominio ni se
// audita (§1.7: ni ingresos, ni consultas, ni reclamos).
type TicketDeCola struct {
	hash      HashTicket
	clave     ClaveSala
	rango     RangoEnCola
	estado    EstadoTicket
	emitidoEn time.Time
}

// NuevoTicketDeCola valida y construye un TicketDeCola a partir de campos
// ya resueltos (típicamente al rehidratar el HASH de Redis de
// confianza:cola:<clave>:t:<sha256>, o justo después de emitir uno nuevo
// con rango asignado por el script Lua `ingresar`, §6.2 del diseño).
func NuevoTicketDeCola(hash HashTicket, clave ClaveSala, rango RangoEnCola, estado EstadoTicket, emitidoEn time.Time) (TicketDeCola, error) {
	if hash.EsVacio() {
		return TicketDeCola{}, &ErrHashTicketInvalido{Motivo: "no puede estar vacío"}
	}
	if clave.EsVacio() {
		return TicketDeCola{}, &ErrClaveSalaInvalida{Motivo: "no puede estar vacía"}
	}
	if rango.EsVacio() {
		return TicketDeCola{}, &ErrRangoEnColaInvalido{Motivo: "no puede estar vacío"}
	}
	if estado.EsVacio() {
		return TicketDeCola{}, &ErrEstadoTicketInvalido{Valor: ""}
	}
	return TicketDeCola{
		hash:      hash,
		clave:     clave,
		rango:     rango,
		estado:    estado,
		emitidoEn: emitidoEn,
	}, nil
}

// Hash devuelve el HashTicket que identifica a este ticket.
func (t TicketDeCola) Hash() HashTicket { return t.hash }

// Clave devuelve la ClaveSala de la sala a la que pertenece este ticket.
func (t TicketDeCola) Clave() ClaveSala { return t.clave }

// Rango devuelve el ordinal de ingreso (orden FIFO) de este ticket.
func (t TicketDeCola) Rango() RangoEnCola { return t.rango }

// Estado devuelve el estado actual del ticket (esperando | consumido).
func (t TicketDeCola) Estado() EstadoTicket { return t.estado }

// EmitidoEn devuelve la marca de tiempo de emisión del ticket.
func (t TicketDeCola) EmitidoEn() time.Time { return t.emitidoEn }

// Posicion calcula cuántos turnos faltan antes del propio, dado el cursor
// de admisión vigente (tabla 1.3 del diseño: max(0, rango - cursor)).
func (t TicketDeCola) Posicion(cursor int64) PosicionEnCola {
	return nuevaPosicionEnCola(t.rango, cursor)
}

// Desenlace calcula, de forma pura y determinista, el resultado de
// presentar este ticket ante la sala dada en el instante ahora (§1.5 y §6.2
// del diseño: réplica en dominio del script Lua `reclamar`, para que sea
// testeable sin Redis y exista un punto de comparación en el test de
// consistencia dominio↔Lua). Solo produce cuatro de los siete valores del
// catálogo DesenlaceDeAdmision: admitido, esperando, turno_caducado y
// ticket_consumido — "ticket_desconocido" y "sala_cerrada" ocurren antes de
// poder construir este objeto (no hay HASH en Redis, o no hay
// configuración de sala), y "cola_llena" es un desenlace del ingreso, no
// del reclamo.
func (t TicketDeCola) Desenlace(sala *SalaDeEspera, ahora time.Time) DesenlaceDeAdmision {
	if t.estado.EsIgual(EstadoTicketConsumido) {
		return DesenlaceTicketConsumido
	}
	cursor := sala.CursorEn(ahora)
	if t.rango.Valor() > cursor {
		return DesenlaceEsperando
	}
	turno := sala.TurnoDe(t.rango)
	limite := turno.Add(sala.Politica().VentanaReclamo())
	if ahora.After(limite) {
		return DesenlaceTurnoCaducado
	}
	return DesenlaceAdmitido
}
