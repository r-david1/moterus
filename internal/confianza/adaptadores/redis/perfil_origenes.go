package redis

import (
	"context"
	"fmt"
	"strconv"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/r-david1/moterus/internal/confianza/puertos"
)

// prefijoConfianzaOrigen es el prefijo de la única clave de Redis por cuenta
// que usa el reconocimiento de origen (§5.2 del diseño
// fingerprinting-comportamiento.md). No colisiona con "confianza:rl:" (el
// limitador de tasa, limitador_tasa.go) ni con "confianza:cola:" (las salas
// de espera, estado_cola.go).
const prefijoConfianzaOrigen = "confianza:orig:"

// campoExitos y campoHuella son los nombres de los dos campos contador del
// HASH de perfil (§5.2 del diseño): "#exitos" cuenta todos los logins
// exitosos registrados, "#huella" cuenta cuántos de ellos traían huella de
// dispositivo (es el dato que necesita la señal huella_ausente, §1.4 del
// diseño: mira exitosConHuella, no exitos).
const (
	campoExitos = "#exitos"
	campoHuella = "#huella"
)

func claveOrigen(clave string) string { return prefijoConfianzaOrigen + clave }

// campoDispositivo y campoRed arman el NOMBRE DE CAMPO completo del HASH
// ("d:<hash16>" / "n:<hash16>") a partir del hash ya calculado por el
// dominio. Se arman del lado de Go, nunca dentro del script Lua (§5.2 del
// diseño: "ARGV[2]/ARGV[3] son los NOMBRES DE CAMPO completos ya armados por
// Go... así el script no concatena strings").
func campoDispositivo(hashHuella string) string { return "d:" + hashHuella }
func campoRed(hashRed string) string            { return "n:" + hashRed }

// scriptRegistrarLua implementa Registrar (§5.2 del diseño), casi literal:
// acumula el éxito, promueve el dispositivo y/o la red observados (si la
// petición traía uno), poda el más antiguo de cada familia por encima del
// techo de la política, y refresca el TTL — todo en una sola operación
// atómica, mismo criterio y mismo motivo que scriptPermitir del limitador de
// tasa (ADR 0018).
//
// KEYS[1] = confianza:orig:<clave>
// ARGV[1] = ahora_ms
// ARGV[2] = campo de dispositivo completo ("d:<hash16>"), o "" si la
//
//	petición no traía huella
//
// ARGV[3] = campo de red completo ("n:<hash16>"), o "" si la IP era
//
//	privada/loopback/ausente
//
// ARGV[4] = maximoOrigenesRecordados (techo POR FAMILIA: hasta ese número de
//
//	dispositivos y, por separado, hasta ese número de redes)
//
// ARGV[5] = vidaPerfil en milisegundos
//
// Dos notas que el diseño deja explícitas y que no conviene esconder al
// "mejorar" este script:
//
//  1. La poda descarta como máximo UN campo por familia por invocación, no
//     hasta bajar del techo. Como cada invocación agrega a lo sumo un campo
//     nuevo por familia, esto basta para que el conjunto nunca crezca sin
//     límite; un `while` sería igual de correcto y estrictamente más caro en
//     el caso estacionario (nada que podar).
//  2. El `HGETALL` dentro del bucle de poda es deliberado: no hay forma más
//     barata en Redis puro de encontrar "el campo más viejo de un prefijo
//     dado" dentro de un HASH. Es aceptable porque corre en el camino FRÍO
//     (solo tras un login exitoso, nunca en la evaluación), y porque la
//     aritmética de decisión (Senales, pesos, umbrales) NO vive acá: este
//     script solo acumula, nunca decide — a diferencia del limitador de tasa
//     y de los scripts de sala, que sí tienen que replicar aritmética del
//     dominio.
const scriptRegistrarLua = `
local ahora = tonumber(ARGV[1])
redis.call('HINCRBY', KEYS[1], '#exitos', 1)

if ARGV[2] ~= '' then
  redis.call('HSET', KEYS[1], ARGV[2], ahora)
  redis.call('HINCRBY', KEYS[1], '#huella', 1)
end
if ARGV[3] ~= '' then
  redis.call('HSET', KEYS[1], ARGV[3], ahora)
end

local maximo = tonumber(ARGV[4])
for _, prefijo in ipairs({'d:', 'n:'}) do
  local campos, viejo, viejoTs, n = redis.call('HGETALL', KEYS[1]), nil, nil, 0
  for i = 1, #campos, 2 do
    if string.sub(campos[i], 1, 2) == prefijo then
      n = n + 1
      local ts = tonumber(campos[i+1])
      if viejoTs == nil or ts < viejoTs then viejo, viejoTs = campos[i], ts end
    end
  end
  if n > maximo and viejo ~= nil then redis.call('HDEL', KEYS[1], viejo) end
end

redis.call('PEXPIRE', KEYS[1], tonumber(ARGV[5]))
return 1
`

// PerfilOrigenes implementa puertos.PerfilDeOrigenes contra un
// *redis.Client (go-redis/v9) ya conectado (internal/plataforma/cache.
// NuevoClienteRedis), mismo patrón que LimitadorTasa y EstadoCola: el script
// Lua se carga una sola vez con redis.NewScript a nivel de construcción del
// adaptador, nunca se compila por llamada.
//
// Cualquier fallo de Redis se envuelve y se propaga tal cual: el fail-open
// de INV-RIES-03 lo decide la capa de aplicación (evaluar_trust_signal.go),
// no este adaptador. Este adaptador solo reporta con honestidad si pudo o no
// hablar con Redis.
type PerfilOrigenes struct {
	cliente         *goredis.Client
	scriptRegistrar *goredis.Script
	ahora           func() time.Time
}

var _ puertos.PerfilDeOrigenes = (*PerfilOrigenes)(nil)

// OpcionPerfilOrigenes configura PerfilOrigenes en su construcción.
type OpcionPerfilOrigenes func(*PerfilOrigenes)

// ConRelojPerfilOrigenes sobreescribe la fuente de "ahora" que usa Registrar
// (por defecto, time.Now().UTC()). Uso exclusivo de tests: le da al test de
// integración control determinista sobre qué origen queda marcado como "más
// antiguo" al podar (mismo criterio que ConRelojEstadoCola).
func ConRelojPerfilOrigenes(ahora func() time.Time) OpcionPerfilOrigenes {
	return func(p *PerfilOrigenes) { p.ahora = ahora }
}

// NuevoPerfilDeOrigenes construye el adaptador sobre un cliente Redis ya
// conectado. No valida la conexión aquí (mismo criterio que
// NuevoLimitadorTasa y NuevoEstadoCola): el primer Consultar/Registrar es el
// que falla si Redis no responde.
func NuevoPerfilDeOrigenes(cliente *goredis.Client, opciones ...OpcionPerfilOrigenes) *PerfilOrigenes {
	p := &PerfilOrigenes{
		cliente:         cliente,
		scriptRegistrar: goredis.NewScript(scriptRegistrarLua),
		ahora:           func() time.Time { return time.Now().UTC() },
	}
	for _, opcion := range opciones {
		opcion(p)
	}
	return p
}

// Consultar responde, en un solo viaje de ida y vuelta a Redis, todo lo que
// dominio.PerfilDeOrigen necesita.
//
// El camino caliente (paso 2.5 de EvaluarTrustSignal, §3.1 del diseño) solo
// necesita las cuatro señales de un HMGET: no le importa OrigenesConocidos.
// El camino de promoción (RegistrarResultado, §3.4 del diseño) SÍ necesita
// OrigenesConocidos para decidir si auditar OrigenNuevoObservado. El puerto
// (puertos.PerfilDeOrigenes) declara un único método Consultar para ambos
// casos, sin ningún parámetro que distinga "para qué lo estás pidiendo" —
// así que este adaptador no puede, desde adentro, saltarse el cálculo de
// OrigenesConocidos solo quien lo necesita.
//
// La solución elegida: calcular OrigenesConocidos SIEMPRE, pero con una
// segunda operación deliberadamente barata (HLEN, O(1) en Redis, a
// diferencia de HGETALL que es O(n) sobre el tamaño del HASH) y en el MISMO
// viaje de red que el HMGET, vía Pipeline — mismo criterio exacto que
// EstadoCola.Instantanea, que combina HGetAll+Get en un pipeline "para
// ahorrar una ida y vuelta de red". El HMGET ya devuelve si '#exitos' y
// '#huella' existen (nil o no) sin ningún costo adicional, así que
// OrigenesConocidos = HLEN(clave) menos esos dos contadores cuando están
// presentes: no hace falta traer los campos "d:*"/"n:*" para contarlos.
//
// Esto es una operación de Redis más que la única lectura que el diseño
// describe para el camino caliente ("una lectura de 4 campos", §2.2 y §5.2)
// — la desviación es deliberada: HLEN es O(1), viaja en el mismo pipeline
// (mismo costo de red que un HMGET solo) y evita duplicar este método en dos
// variantes por un campo que la aplicación descarta la mayoría de las veces.
func (p *PerfilOrigenes) Consultar(ctx context.Context, q puertos.ConsultaPerfilOrigen) (puertos.VistaPerfilOrigen, error) {
	if q.Clave == "" {
		return puertos.VistaPerfilOrigen{}, fmt.Errorf("confianza/redis: ConsultaPerfilOrigen.Clave no puede estar vacía")
	}
	clave := claveOrigen(q.Clave)

	// Un campo vacío ("d:"/"n:" sin hash) nunca lo escribe Registrar (ver su
	// comentario), así que pedirlo devolvería siempre nil de todas formas.
	// En vez de arriesgarse a construir ese campo huérfano, si la petición
	// no trae huella/red simplemente se pide un nombre de campo vacío, que
	// jamás existe en el HASH: el HMGET devuelve nil para esa posición, tal
	// como si el campo no estuviera, sin necesidad de ramificar la llamada.
	campoD := ""
	if q.HashHuella != "" {
		campoD = campoDispositivo(q.HashHuella)
	}
	campoN := ""
	if q.HashRed != "" {
		campoN = campoRed(q.HashRed)
	}

	pipe := p.cliente.Pipeline()
	hmget := pipe.HMGet(ctx, clave, campoD, campoN, campoExitos, campoHuella)
	hlen := pipe.HLen(ctx, clave)
	if _, err := pipe.Exec(ctx); err != nil {
		return puertos.VistaPerfilOrigen{}, fmt.Errorf("confianza/redis: fallo al consultar el perfil de origen: %w", err)
	}

	valores, err := hmget.Result()
	if err != nil {
		return puertos.VistaPerfilOrigen{}, fmt.Errorf("confianza/redis: fallo al leer los campos del perfil de origen: %w", err)
	}
	if len(valores) != 4 {
		return puertos.VistaPerfilOrigen{}, fmt.Errorf("confianza/redis: respuesta inesperada de HMGET sobre el perfil de origen: %#v", valores)
	}
	longitud, err := hlen.Result()
	if err != nil {
		return puertos.VistaPerfilOrigen{}, fmt.Errorf("confianza/redis: fallo al contar los campos del perfil de origen: %w", err)
	}

	exitos, err := int64DesdeCampoRedis(valores[2])
	if err != nil {
		return puertos.VistaPerfilOrigen{}, fmt.Errorf("confianza/redis: campo %q corrupto en el perfil de origen: %w", campoExitos, err)
	}
	exitosConHuella, err := int64DesdeCampoRedis(valores[3])
	if err != nil {
		return puertos.VistaPerfilOrigen{}, fmt.Errorf("confianza/redis: campo %q corrupto en el perfil de origen: %w", campoHuella, err)
	}

	origenesConocidos := longitud
	if valores[2] != nil {
		origenesConocidos--
	}
	if valores[3] != nil {
		origenesConocidos--
	}
	if origenesConocidos < 0 {
		origenesConocidos = 0
	}

	return puertos.VistaPerfilOrigen{
		Exitos:              exitos,
		ExitosConHuella:     exitosConHuella,
		DispositivoConocido: valores[0] != nil,
		RedConocida:         valores[1] != nil,
		OrigenesConocidos:   int(origenesConocidos),
	}, nil
}

// Registrar ver el comentario de scriptRegistrarLua.
func (p *PerfilOrigenes) Registrar(ctx context.Context, cmd puertos.RegistrarOrigenObservado) error {
	if cmd.Clave == "" {
		return fmt.Errorf("confianza/redis: RegistrarOrigenObservado.Clave no puede estar vacía")
	}

	campoD := ""
	if cmd.HashHuella != "" {
		campoD = campoDispositivo(cmd.HashHuella)
	}
	campoN := ""
	if cmd.HashRed != "" {
		campoN = campoRed(cmd.HashRed)
	}

	_, err := p.scriptRegistrar.Run(ctx, p.cliente, []string{claveOrigen(cmd.Clave)},
		p.ahora().UnixMilli(),
		campoD,
		campoN,
		cmd.MaximoOrigenesRecordados,
		cmd.VidaPerfil.Milliseconds(),
	).Result()
	if err != nil {
		return fmt.Errorf("confianza/redis: fallo al registrar el origen observado: %w", err)
	}
	return nil
}

// Olvidar borra el perfil completo de una cuenta (§3.5 del diseño).
func (p *PerfilOrigenes) Olvidar(ctx context.Context, clave string) error {
	if err := p.cliente.Del(ctx, claveOrigen(clave)).Err(); err != nil {
		return fmt.Errorf("confianza/redis: fallo al olvidar el perfil de origen: %w", err)
	}
	return nil
}

// int64DesdeCampoRedis interpreta el valor crudo de un campo de HASH
// devuelto por HMGET: nil (el campo no existe) es 0, cualquier otro valor se
// parsea como el contador entero que escribe HINCRBY.
func int64DesdeCampoRedis(v interface{}) (int64, error) {
	if v == nil {
		return 0, nil
	}
	s, ok := v.(string)
	if !ok {
		return 0, fmt.Errorf("valor inesperado %#v (%T)", v, v)
	}
	return strconv.ParseInt(s, 10, 64)
}
