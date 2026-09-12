// Package redis implementa puertos.LimitadorTasa contra Redis
// (go-redis/v9), respaldado por el cliente compartido de
// internal/plataforma/cache. Es la implementación real de rate limiting
// del bounded context Confianza (ADR 0018): Redis, no Postgres, porque el
// caso de uso es contadores efímeros de muy alta frecuencia de escritura
// (cada intento de login/registro/reenvío incrementa al menos dos claves),
// y Postgres no es el motor adecuado para ese patrón de acceso a este
// volumen (ver la justificación completa en el ADR).
package redis

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/r-david1/moterus/internal/confianza/dominio"
	"github.com/r-david1/moterus/internal/confianza/puertos"
)

// backoffMaximoPorDefecto acota cuánto puede crecer el cooldown
// exponencial de una clave que sigue recibiendo solicitudes pese a estar
// bloqueada: sin tope, un atacante persistente terminaría con un TTL de
// días, lo que en la práctica sería indistinguible de un baneo permanente
// no reversible sin intervención manual — dos horas es agresivo pero
// recuperable.
const backoffMaximoPorDefecto = 2 * time.Hour

// scriptPermitir implementa un contador de ventana fija (INCR + PEXPIRE)
// con cooldown exponencial cuando el límite ya se superó, todo en un único
// script Lua para que el incremento, la lectura del TTL y la posible
// extensión del TTL sean atómicos (evita condiciones de carrera entre
// llamadas concurrentes a la misma clave, algo que INCR+EXPIRE hechos como
// dos comandos Go separados no garantiza).
//
// KEYS[1] = clave
// ARGV[1] = límite (entero)
// ARGV[2] = ventana en milisegundos
// ARGV[3] = backoff máximo en milisegundos
//
// Devuelve {permitido (1/0), restantes, ttl_ms}.
const scriptPermitir = `
local key = KEYS[1]
local limite = tonumber(ARGV[1])
local ventana_ms = tonumber(ARGV[2])
local max_backoff_ms = tonumber(ARGV[3])

local count = redis.call('INCR', key)
if count == 1 then
  redis.call('PEXPIRE', key, ventana_ms)
end

local ttl = redis.call('PTTL', key)
if ttl < 0 then
  redis.call('PEXPIRE', key, ventana_ms)
  ttl = ventana_ms
end

if count <= limite then
  return {1, limite - count, ttl}
end

local exceso = count - limite
local backoff = ventana_ms
local i = 2
while i <= exceso do
  backoff = backoff * 2
  if backoff >= max_backoff_ms then
    backoff = max_backoff_ms
    break
  end
  i = i + 1
end

if backoff > ttl then
  redis.call('PEXPIRE', key, backoff)
  ttl = backoff
end

return {0, 0, ttl}
`

// LimitadorTasa implementa puertos.LimitadorTasa contra un
// *redis.Client (go-redis/v9) ya conectado (ver
// internal/plataforma/cache.NuevoClienteRedis).
type LimitadorTasa struct {
	cliente       *goredis.Client
	script        *goredis.Script
	backoffMaximo time.Duration
}

var _ puertos.LimitadorTasa = (*LimitadorTasa)(nil)

// OpcionLimitadorTasa configura LimitadorTasa en su construcción.
type OpcionLimitadorTasa func(*LimitadorTasa)

// ConBackoffMaximo sobreescribe backoffMaximoPorDefecto (útil en tests
// para no esperar horas).
func ConBackoffMaximo(d time.Duration) OpcionLimitadorTasa {
	return func(l *LimitadorTasa) { l.backoffMaximo = d }
}

// NuevoLimitadorTasa construye el adaptador sobre un cliente Redis ya
// creado. No valida la conexión aquí (igual criterio que bd.NuevoPool):
// el primer Permitir/Reiniciar es el que falla si Redis no responde.
func NuevoLimitadorTasa(cliente *goredis.Client, opciones ...OpcionLimitadorTasa) *LimitadorTasa {
	l := &LimitadorTasa{
		cliente:       cliente,
		script:        goredis.NewScript(scriptPermitir),
		backoffMaximo: backoffMaximoPorDefecto,
	}
	for _, opcion := range opciones {
		opcion(l)
	}
	return l
}

// Permitir ver el comentario de scriptPermitir.
//
// El tope de backoff que se pasa al script (ARGV[3]) sale de
// umbral.BackoffMaximo cuando el llamador lo fijó (> 0) — es el caso del
// nivel Cuenta, que PoliticaLimites.Para() normaliza siempre a su propia
// Ventana (ADR 0052, INV-BLQ-05: el escalado nunca debe crecer en una
// clave que el atacante no posee) — y de l.backoffMaximo (el tope del
// adaptador, 2h por defecto) en caso contrario, que es el caso del nivel
// IP, donde el escalado sí está bien dirigido.
func (l *LimitadorTasa) Permitir(ctx context.Context, clave string, umbral dominio.Umbral) (bool, int, time.Duration, error) {
	if umbral.Limite <= 0 {
		return false, 0, 0, fmt.Errorf("confianza/redis: umbral.Limite debe ser > 0, fue %d", umbral.Limite)
	}
	backoffMaximo := l.backoffMaximo
	if umbral.BackoffMaximo > 0 {
		backoffMaximo = umbral.BackoffMaximo
	}
	res, err := l.script.Run(ctx, l.cliente, []string{clave},
		umbral.Limite,
		umbral.Ventana.Milliseconds(),
		backoffMaximo.Milliseconds(),
	).Result()
	if err != nil {
		return false, 0, 0, fmt.Errorf("confianza/redis: fallo al evaluar el limitador de tasa: %w", err)
	}

	valores, ok := res.([]interface{})
	if !ok || len(valores) != 3 {
		return false, 0, 0, fmt.Errorf("confianza/redis: respuesta inesperada del script de limitador: %#v", res)
	}
	permitido := toInt64(valores[0]) == 1
	restantes := int(toInt64(valores[1]))
	ttlMS := toInt64(valores[2])

	return permitido, restantes, time.Duration(ttlMS) * time.Millisecond, nil
}

// Reiniciar borra el contador de clave.
func (l *LimitadorTasa) Reiniciar(ctx context.Context, clave string) error {
	if err := l.cliente.Del(ctx, clave).Err(); err != nil {
		return fmt.Errorf("confianza/redis: fallo al reiniciar el contador: %w", err)
	}
	return nil
}

// toInt64 normaliza el resultado de EVAL (go-redis puede devolver int64
// directamente para respuestas de tipo integer de Lua).
func toInt64(v interface{}) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	default:
		return 0
	}
}
