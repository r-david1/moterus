package redis

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/r-david1/moterus/internal/confianza/dominio"
	"github.com/r-david1/moterus/internal/confianza/puertos"
)

// prefijoConfianzaCola es el prefijo de todas las claves de Redis de una
// sala de espera (§6.2 del diseño colas-virtuales.md). El limitador de
// tasa usa "confianza:rl:" (limitador_tasa.go); no colisionan.
const prefijoConfianzaCola = "confianza:cola:"

// margenTicketPorDefecto es el margen que se suma al TTL de un ticket por
// encima de "turnoDe(rango) + ventanaReclamo - ahora" (§6.2 del diseño:
// "+60s"), para que un reloj ligeramente desincronizado entre réplicas, o
// la latencia de la propia llamada, no expire un ticket un instante antes
// de que su ventana de reclamo termine.
const margenTicketPorDefecto = 60 * time.Second

func claveCfg(clave string) string          { return prefijoConfianzaCola + clave + ":cfg" }
func claveSeq(clave string) string          { return prefijoConfianzaCola + clave + ":seq" }
func claveTicket(clave, hash string) string { return prefijoConfianzaCola + clave + ":t:" + hash }
func patronTickets(clave string) string     { return prefijoConfianzaCola + clave + ":t:*" }

// funcionesLuaCursorTurno define, en Lua, las dos funciones puras que
// replican exactamente dominio.SalaDeEspera.CursorEn y
// dominio.SalaDeEspera.TurnoDe (§1.5 del diseño). Se antepone al cuerpo de
// cada uno de los tres scripts (ingresar/reclamar/consultar) para que los
// tres compartan la MISMA aritmética textual — evitar una tercera copia
// manuscrita por script es la mitad de la mitigación de la tensión de
// duplicación que documenta §1.5 ("la aritmética vive dos veces: dominio y
// Lua"); la otra mitad es el test de consistencia
// (test/integracion/confianza_cola_redis_test.go).
//
//   - cursor_de(ahora, base, desde, ritmo) = base + floor((ahora-desde)*ritmo/1000),
//     con (ahora-desde) recortado a 0 cuando es negativo — mismo criterio
//     conservador que dominio.SalaDeEspera.CursorEn (INV-COLA-04: un reloj
//     desincronizado nunca produce un cursor negativo).
//   - turno_de(rango, base, desde, ritmo) = desde cuando (rango-base) <= 0,
//     o desde + ceil((rango-base)*1000/ritmo) en caso contrario — mismo
//     criterio que dominio.SalaDeEspera.TurnoDe. La guarda "<= 0" es
//     deliberada: el script `reclamar` del propio diseño, tomado literal,
//     no la tenía y producía un turno ANTERIOR a `desde` para un rango ya
//     alcanzado por un cursorBase posterior (tras CambiarRitmo) — se
//     corrigió acá para que coincida con el dominio.
const funcionesLuaCursorTurno = `
local function cursor_de(ahora, base, desde, ritmo)
  local ms = ahora - desde
  if ms < 0 then ms = 0 end
  return base + math.floor(ms * ritmo / 1000)
end

local function turno_de(rango, base, desde, ritmo)
  local delta = rango - base
  if delta <= 0 then
    return desde
  end
  return desde + math.ceil(delta * 1000 / ritmo)
end
`

// scriptIngresar implementa §3.4/§6.2 del diseño: una sola operación
// atómica que verifica que la sala esté abierta, calcula el cursor,
// verifica la capacidad, asigna un rango nuevo (INCR, INV-COLA-14) y deja
// el ticket con el TTL exacto de su ventana de utilidad (§6.2, punto 3).
//
// KEYS[1] = <clave>:cfg (HASH)
// KEYS[2] = <clave>:seq (contador)
// KEYS[3] = <clave>:t:<hash> (HASH del ticket nuevo)
// ARGV[1] = ahora_ms
// ARGV[2] = margen_ms (margenTicketPorDefecto, en milisegundos)
//
// Devuelve siempre una tupla de 5 elementos {desenlace, rango, cursor,
// longitud, turno_ms} — formato propio de este adaptador (uniforme entre
// los tres scripts), no el que trae el diseño literal (que varía de forma
// entre scripts): lo que sí replica exactamente es la aritmética de cursor/
// turno y los desenlaces del catálogo cerrado dominio.DesenlaceDeAdmision.
const scriptIngresarLua = funcionesLuaCursorTurno + `
local cfg = redis.call('HGETALL', KEYS[1])
if #cfg == 0 then return {'sala_cerrada', 0, 0, 0, 0} end
local c = {}
for i = 1, #cfg, 2 do c[cfg[i]] = cfg[i + 1] end
if c['estado'] ~= 'abierta' then return {'sala_cerrada', 0, 0, 0, 0} end

local ahora     = tonumber(ARGV[1])
local margen    = tonumber(ARGV[2])
local ritmo     = tonumber(c['ritmo'])
local base      = tonumber(c['cursor_base'])
local desde     = tonumber(c['reloj_desde_ms'])
local capacidad = tonumber(c['capacidad'])

local cursor = cursor_de(ahora, base, desde, ritmo)
local ultimo = tonumber(redis.call('GET', KEYS[2]) or '0')
if (ultimo - cursor) >= capacidad then
  return {'cola_llena', 0, cursor, ultimo, 0}
end

local rango = redis.call('INCR', KEYS[2])
local turno = turno_de(rango, base, desde, ritmo)
redis.call('HSET', KEYS[3], 'rango', rango, 'estado', 'esperando', 'emitido_ms', ahora)
local ttl = (turno + tonumber(c['ventana_reclamo_ms']) - ahora) + margen
if ttl < 1 then ttl = 1 end
redis.call('PEXPIRE', KEYS[3], ttl)
return {'esperando', rango, cursor, rango, turno}
`

// scriptReclamar implementa §3.6/§6.2 del diseño: resuelve el desenlace de
// un ticket y, si es "admitido", lo marca "consumido" en la misma
// operación atómica (INV-COLA-06: exactamente uno de dos reclamos
// concurrentes pasa).
//
// KEYS[1] = <clave>:cfg
// KEYS[2] = <clave>:seq (para reportar LongitudCola; el script literal del
// diseño no la incluye en el reclamo — se agrega acá porque
// ResultadoTurno.LongitudCola la necesita también en la respuesta de
// Reclamar/ConsultarTurno, y el costo es un GET adicional sobre una clave
// ya calentada).
// KEYS[3] = <clave>:t:<hash>
// ARGV[1] = ahora_ms
const scriptReclamarLua = funcionesLuaCursorTurno + `
local cfg = redis.call('HGETALL', KEYS[1])
if #cfg == 0 then return {'sala_cerrada', 0, 0, 0, 0} end
local c = {}
for i = 1, #cfg, 2 do c[cfg[i]] = cfg[i + 1] end

local t = redis.call('HMGET', KEYS[3], 'rango', 'estado')
if not t[1] then return {'ticket_desconocido', 0, 0, 0, 0} end
if t[2] == 'consumido' then return {'ticket_consumido', 0, 0, 0, 0} end

local ahora  = tonumber(ARGV[1])
local ritmo  = tonumber(c['ritmo'])
local base   = tonumber(c['cursor_base'])
local desde  = tonumber(c['reloj_desde_ms'])
local rango  = tonumber(t[1])
local cursor = cursor_de(ahora, base, desde, ritmo)
local turno  = turno_de(rango, base, desde, ritmo)
local longitud = tonumber(redis.call('GET', KEYS[2]) or '0')

if rango > cursor then
  return {'esperando', rango, cursor, longitud, turno}
end
if ahora > (turno + tonumber(c['ventana_reclamo_ms'])) then
  redis.call('DEL', KEYS[3])
  return {'turno_caducado', rango, cursor, longitud, turno}
end

redis.call('HSET', KEYS[3], 'estado', 'consumido')
redis.call('PEXPIRE', KEYS[3], tonumber(c['ventana_reclamo_ms']))
return {'admitido', rango, cursor, longitud, turno}
`

// scriptConsultarLua implementa §3.5/§6.2 del diseño ("el mismo cálculo que
// reclamar sin la escritura final, solo refresca el PEXPIRE"): a
// diferencia de scriptReclamarLua, nunca muta 'estado' ni hace DEL —
// cualquier desenlace (incluidos ticket_consumido/turno_caducado) es un
// resultado de solo lectura válido (§1.8 del diseño: un ticket no protege
// ningún secreto). El refresco de TTL ocurre siempre que el ticket exista,
// sea cual sea su desenlace: es lo que hace que la cola se limpie sola
// cuando el cliente deja de sondear (§6.2, punto 3).
//
// KEYS[1] = <clave>:cfg
// KEYS[2] = <clave>:seq
// KEYS[3] = <clave>:t:<hash>
// ARGV[1] = ahora_ms
// ARGV[2] = margen_ms
const scriptConsultarLua = funcionesLuaCursorTurno + `
local cfg = redis.call('HGETALL', KEYS[1])
if #cfg == 0 then return {'sala_cerrada', 0, 0, 0, 0} end
local c = {}
for i = 1, #cfg, 2 do c[cfg[i]] = cfg[i + 1] end

local t = redis.call('HMGET', KEYS[3], 'rango', 'estado')
if not t[1] then return {'ticket_desconocido', 0, 0, 0, 0} end

local ahora   = tonumber(ARGV[1])
local margen  = tonumber(ARGV[2])
local ritmo   = tonumber(c['ritmo'])
local base    = tonumber(c['cursor_base'])
local desde   = tonumber(c['reloj_desde_ms'])
local ventana = tonumber(c['ventana_reclamo_ms'])
local rango   = tonumber(t[1])
local cursor  = cursor_de(ahora, base, desde, ritmo)
local turno   = turno_de(rango, base, desde, ritmo)
local longitud = tonumber(redis.call('GET', KEYS[2]) or '0')

local ttl = (turno + ventana - ahora) + margen
if ttl < 1 then ttl = 1 end
redis.call('PEXPIRE', KEYS[3], ttl)

if t[2] == 'consumido' then
  return {'ticket_consumido', rango, cursor, longitud, turno}
end
if rango > cursor then
  return {'esperando', rango, cursor, longitud, turno}
end
if ahora > (turno + ventana) then
  return {'turno_caducado', rango, cursor, longitud, turno}
end
return {'admitido', rango, cursor, longitud, turno}
`

// scriptProyectarLua implementa Proyectar (§3.7/§6.2 del diseño): escribe
// la configuración de una sala de forma idempotente, con dos reglas de
// concurrencia resueltas atómicamente:
//
//  1. "Una proyección vieja nunca pisa a una nueva" (comentario de
//     puertos.ProyeccionSala.Version): si la versión ya almacenada es
//     mayor que la entrante, el script no escribe nada (dos réplicas del
//     reconciliador, o el reconciliador y un caso de uso de
//     administración, pueden competir por proyectar la misma sala casi al
//     mismo tiempo).
//  2. cursor_base/reloj_desde_ms —el ancla del reloj de admisión— solo se
//     sobrescriben sin condición cuando la versión ENTRANTE es más nueva
//     que la almacenada (una mutación real: Abrir/CambiarRitmo). Cuando la
//     versión es la MISMA que la ya almacenada (una reproyección rutinaria
//     del reconciliador sobre una sala sin cambios), se usa HSETNX: si el
//     HASH ya existe con esos campos, no se tocan (preserva la
//     continuidad del cursor); si el HASH no existe o le faltan esos
//     campos —Redis perdió el estado de la sala—, HSETNX los fija por
//     primera vez con los valores que trae la proyección (que, para ese
//     caso, ya vienen en 0/ahora desde ReconciliarSalasCasoDeUso,
//     INV-COLA-13). Ningún otro campo (alias/estado/ritmo/capacidad/
//     ventana) necesita esta distinción: no tienen un problema de
//     continuidad análogo al del reloj de admisión, así que siempre se
//     escriben con HSET.
//
// KEYS[1] = <clave>:cfg
// ARGV[1] = alias
// ARGV[2] = estado
// ARGV[3] = ritmo
// ARGV[4] = capacidad
// ARGV[5] = ventana_reclamo_ms
// ARGV[6] = cursor_base
// ARGV[7] = reloj_desde_ms
// ARGV[8] = version
const scriptProyectarLua = `
local version_actual = redis.call('HGET', KEYS[1], 'version')
local nueva_version = tonumber(ARGV[8])

if version_actual and tonumber(version_actual) > nueva_version then
  return 0
end

redis.call('HSET', KEYS[1],
  'alias', ARGV[1],
  'estado', ARGV[2],
  'ritmo', ARGV[3],
  'capacidad', ARGV[4],
  'ventana_reclamo_ms', ARGV[5],
  'version', ARGV[8])

if version_actual and tonumber(version_actual) == nueva_version then
  redis.call('HSETNX', KEYS[1], 'cursor_base', ARGV[6])
  redis.call('HSETNX', KEYS[1], 'reloj_desde_ms', ARGV[7])
else
  redis.call('HSET', KEYS[1], 'cursor_base', ARGV[6], 'reloj_desde_ms', ARGV[7])
end

return 1
`

// EstadoCola implementa puertos.EstadoDeCola contra un *redis.Client
// (go-redis/v9) ya conectado (internal/plataforma/cache.NuevoClienteRedis),
// mismo patrón que LimitadorTasa: los scripts Lua se cargan una sola vez
// con redis.NewScript a nivel de construcción del adaptador, nunca se
// compilan por llamada.
//
// A diferencia de LimitadorTasa, cuya aritmética solo necesita duraciones
// (nunca un instante absoluto), Ingresar/Consultar/Reclamar/Instantanea
// necesitan "ahora" como instante de pared para calcular el cursor de
// admisión (§1.5 del diseño). Por defecto este adaptador usa
// time.Now().UTC(): puertos.Reloj (confianza/puertos) existe para que la
// CAPA DE APLICACIÓN pueda fijar el tiempo en sus tests sin tocar
// infraestructura real (mismo criterio que reloj.Real en
// internal/plataforma/reloj), y este adaptador no depende de él para no
// acoplar infraestructura a un puerto pensado para aplicacion. El reloj
// interno SÍ es reemplazable vía ConRelojEstadoCola: es lo que le permite
// al test de consistencia dominio↔Lua (§1.5 del diseño,
// test/integracion/confianza_cola_redis_test.go) fijar exactamente el
// mismo "ahora" que usa para calcular dominio.SalaDeEspera.CursorEn/
// TurnoDe, en vez de depender de dos llamadas a time.Now() separadas por
// una latencia de red variable.
type EstadoCola struct {
	cliente         *goredis.Client
	scriptIngresar  *goredis.Script
	scriptReclamar  *goredis.Script
	scriptConsultar *goredis.Script
	scriptProyectar *goredis.Script
	margenTicket    time.Duration
	ahora           func() time.Time
}

var _ puertos.EstadoDeCola = (*EstadoCola)(nil)

// OpcionEstadoCola configura EstadoCola en su construcción.
type OpcionEstadoCola func(*EstadoCola)

// ConMargenTicket sobreescribe margenTicketPorDefecto (útil en tests para
// acotar TTLs).
func ConMargenTicket(d time.Duration) OpcionEstadoCola {
	return func(e *EstadoCola) { e.margenTicket = d }
}

// ConRelojEstadoCola sobreescribe la fuente de "ahora" que usan
// Ingresar/Consultar/Reclamar/Instantanea (por defecto, time.Now().UTC()).
// Uso exclusivo de tests: le da al test de consistencia dominio↔Lua control
// exacto sobre el instante que se compara contra
// dominio.SalaDeEspera.CursorEn/TurnoDe.
func ConRelojEstadoCola(ahora func() time.Time) OpcionEstadoCola {
	return func(e *EstadoCola) { e.ahora = ahora }
}

// NuevoEstadoCola construye el adaptador sobre un cliente Redis ya
// conectado. No valida la conexión aquí, mismo criterio que
// NuevoLimitadorTasa y bd.NuevoPool: el primer método es el que falla si
// Redis no responde.
func NuevoEstadoCola(cliente *goredis.Client, opciones ...OpcionEstadoCola) *EstadoCola {
	e := &EstadoCola{
		cliente:         cliente,
		scriptIngresar:  goredis.NewScript(scriptIngresarLua),
		scriptReclamar:  goredis.NewScript(scriptReclamarLua),
		scriptConsultar: goredis.NewScript(scriptConsultarLua),
		scriptProyectar: goredis.NewScript(scriptProyectarLua),
		margenTicket:    margenTicketPorDefecto,
		ahora:           func() time.Time { return time.Now().UTC() },
	}
	for _, opcion := range opciones {
		opcion(e)
	}
	return e
}

// Proyectar ver el comentario de scriptProyectarLua.
func (e *EstadoCola) Proyectar(ctx context.Context, p puertos.ProyeccionSala) error {
	if p.Clave == "" {
		return fmt.Errorf("confianza/redis: ProyeccionSala.Clave no puede estar vacía")
	}
	_, err := e.scriptProyectar.Run(ctx, e.cliente, []string{claveCfg(p.Clave)},
		p.Alias,
		p.Estado,
		p.RitmoAdmision,
		p.CapacidadMaximaCola,
		p.VentanaReclamo.Milliseconds(),
		p.CursorBase,
		p.RelojDesde.UTC().UnixMilli(),
		p.Version,
	).Result()
	if err != nil {
		return fmt.Errorf("confianza/redis: fallo al proyectar la sala %q: %w", p.Clave, err)
	}
	return nil
}

// Retirar borra las tres claves de una sala (§3.3 del diseño: se invoca al
// cerrar): cfg, seq, y todos los tickets vivos con ese prefijo. Usa SCAN
// (nunca KEYS, que bloquea Redis entero mientras recorre el keyspace) para
// encontrar los tickets, en lotes de hasta 500 por vuelta — mismo criterio
// que cualquier limpieza masiva de este repositorio.
func (e *EstadoCola) Retirar(ctx context.Context, clave string) error {
	if err := e.cliente.Del(ctx, claveCfg(clave), claveSeq(clave)).Err(); err != nil {
		return fmt.Errorf("confianza/redis: fallo al retirar la configuración de la sala %q: %w", clave, err)
	}

	patron := patronTickets(clave)
	var cursor uint64
	for {
		claves, siguienteCursor, err := e.cliente.Scan(ctx, cursor, patron, 500).Result()
		if err != nil {
			return fmt.Errorf("confianza/redis: fallo al escanear los tickets de la sala %q: %w", clave, err)
		}
		if len(claves) > 0 {
			if err := e.cliente.Del(ctx, claves...).Err(); err != nil {
				return fmt.Errorf("confianza/redis: fallo al borrar tickets de la sala %q: %w", clave, err)
			}
		}
		cursor = siguienteCursor
		if cursor == 0 {
			break
		}
	}
	return nil
}

// Ingresar ver el comentario de scriptIngresarLua.
func (e *EstadoCola) Ingresar(ctx context.Context, clave string, hash string) (puertos.EstadoTicket, error) {
	ahora := e.ahora().UnixMilli()
	res, err := e.scriptIngresar.Run(ctx, e.cliente,
		[]string{claveCfg(clave), claveSeq(clave), claveTicket(clave, hash)},
		ahora, e.margenTicket.Milliseconds(),
	).Result()
	if err != nil {
		return puertos.EstadoTicket{}, fmt.Errorf("confianza/redis: fallo al ingresar en la sala %q: %w", clave, err)
	}
	return estadoTicketDesdeResultado(res)
}

// Consultar ver el comentario de scriptConsultarLua.
func (e *EstadoCola) Consultar(ctx context.Context, clave string, hash string) (puertos.EstadoTicket, error) {
	ahora := e.ahora().UnixMilli()
	res, err := e.scriptConsultar.Run(ctx, e.cliente,
		[]string{claveCfg(clave), claveSeq(clave), claveTicket(clave, hash)},
		ahora, e.margenTicket.Milliseconds(),
	).Result()
	if err != nil {
		return puertos.EstadoTicket{}, fmt.Errorf("confianza/redis: fallo al consultar el turno en la sala %q: %w", clave, err)
	}
	return estadoTicketDesdeResultado(res)
}

// Reclamar ver el comentario de scriptReclamarLua.
func (e *EstadoCola) Reclamar(ctx context.Context, clave string, hash string) (puertos.EstadoTicket, error) {
	ahora := e.ahora().UnixMilli()
	res, err := e.scriptReclamar.Run(ctx, e.cliente,
		[]string{claveCfg(clave), claveSeq(clave), claveTicket(clave, hash)},
		ahora,
	).Result()
	if err != nil {
		return puertos.EstadoTicket{}, fmt.Errorf("confianza/redis: fallo al reclamar el turno en la sala %q: %w", clave, err)
	}
	return estadoTicketDesdeResultado(res)
}

// Instantanea lee cfg y seq directamente (sin script Lua: son dos lecturas
// sin ninguna escritura que proteger, §3.7/§7.1 del diseño) en un pipeline
// para ahorrar una ida y vuelta de red. Si la configuración de la sala no
// existe en Redis, devuelve dominio.ErrEstadoDeColaNoDisponible: es el
// contrato exacto que exige ReconciliarSalasCasoDeUso (INV-COLA-13: "si
// Instantanea devuelve error, Redis perdió el estado de esta sala") — un
// InstantaneaCola{} vacío sin error sería indistinguible de "una sala
// recién proyectada, sin ingresos todavía", que es el caso que la
// invariante necesita poder descartar.
func (e *EstadoCola) Instantanea(ctx context.Context, clave string) (puertos.InstantaneaCola, error) {
	pipe := e.cliente.Pipeline()
	cfgCmd := pipe.HGetAll(ctx, claveCfg(clave))
	seqCmd := pipe.Get(ctx, claveSeq(clave))
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, goredis.Nil) {
		return puertos.InstantaneaCola{}, fmt.Errorf("confianza/redis: fallo al leer el estado de la sala %q: %w", clave, err)
	}

	cfg := cfgCmd.Val()
	if len(cfg) == 0 {
		return puertos.InstantaneaCola{}, &dominio.ErrEstadoDeColaNoDisponible{
			Motivo: fmt.Sprintf("no hay configuración proyectada para la clave %q", clave),
		}
	}

	ritmo, err := strconv.ParseInt(cfg["ritmo"], 10, 64)
	if err != nil {
		return puertos.InstantaneaCola{}, fmt.Errorf("confianza/redis: campo 'ritmo' corrupto en la sala %q: %w", clave, err)
	}
	cursorBase, err := strconv.ParseInt(cfg["cursor_base"], 10, 64)
	if err != nil {
		return puertos.InstantaneaCola{}, fmt.Errorf("confianza/redis: campo 'cursor_base' corrupto en la sala %q: %w", clave, err)
	}
	relojDesdeMS, err := strconv.ParseInt(cfg["reloj_desde_ms"], 10, 64)
	if err != nil {
		return puertos.InstantaneaCola{}, fmt.Errorf("confianza/redis: campo 'reloj_desde_ms' corrupto en la sala %q: %w", clave, err)
	}

	var ingresos int64
	if seqValor := seqCmd.Val(); seqValor != "" {
		ingresos, err = strconv.ParseInt(seqValor, 10, 64)
		if err != nil {
			return puertos.InstantaneaCola{}, fmt.Errorf("confianza/redis: contador de secuencia corrupto en la sala %q: %w", clave, err)
		}
	}

	ahora := e.ahora().UnixMilli()
	cursor := cursorEnMS(ahora, cursorBase, relojDesdeMS, ritmo)

	return puertos.InstantaneaCola{
		LongitudAproximada: ingresos - cursor,
		Cursor:             cursor,
		Ingresos:           ingresos,
	}, nil
}

// cursorEnMS replica en Go, para Instantanea (que no pasa por ningún
// script Lua), la misma fórmula de scriptFuncionesLuaCursorTurno /
// dominio.SalaDeEspera.CursorEn: cursorBase + floor((ahora-relojDesde) *
// ritmo / 1000), recortando a 0 el avance cuando ahora es anterior a
// relojDesde.
func cursorEnMS(ahoraMS, cursorBaseMS, relojDesdeMS, ritmo int64) int64 {
	ms := ahoraMS - relojDesdeMS
	if ms < 0 {
		ms = 0
	}
	return cursorBaseMS + (ms*ritmo)/1000
}

// estadoTicketDesdeResultado decodifica la tupla de 5 elementos
// {desenlace, rango, cursor, longitud, turno_ms} que devuelven
// scriptIngresarLua/scriptReclamarLua/scriptConsultarLua.
func estadoTicketDesdeResultado(res interface{}) (puertos.EstadoTicket, error) {
	valores, ok := res.([]interface{})
	if !ok || len(valores) != 5 {
		return puertos.EstadoTicket{}, fmt.Errorf("confianza/redis: respuesta inesperada del script de cola: %#v", res)
	}
	desenlace, ok := valores[0].(string)
	if !ok {
		return puertos.EstadoTicket{}, fmt.Errorf("confianza/redis: desenlace inesperado en la respuesta del script de cola: %#v", valores[0])
	}
	turnoMS := toInt64(valores[4])
	return puertos.EstadoTicket{
		Desenlace:       desenlace,
		Rango:           toInt64(valores[1]),
		Cursor:          toInt64(valores[2]),
		LongitudCola:    toInt64(valores[3]),
		TurnoEstimadoEn: time.UnixMilli(turnoMS).UTC(),
	}, nil
}
