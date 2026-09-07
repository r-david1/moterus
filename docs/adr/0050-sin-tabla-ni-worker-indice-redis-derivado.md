# ADR 0050 — Sin tabla nueva ni worker: un índice derivado en Redis, la evidencia ya vive en `auditoria`

## Contexto

`docs/design/fingerprinting-comportamiento.md` necesita decidir si el reconocimiento de origen requiere una tabla nueva ("dispositivos conocidos por usuario") y algún mecanismo de procesamiento en segundo plano. La ficha del agente `fingerprinting-comportamiento` proponía un worker separado consumiendo de una cola de mensajería (Redis Streams o NATS) para el análisis pesado.

## Decisión

**No hay tabla nueva y no hay worker. El único almacenamiento nuevo es un índice derivado en Redis (un HASH por cuenta, TTL de 180 días), y el análisis corre síncrono, en el mismo proceso, porque son lecturas O(1).**

**Por qué no hace falta tabla**: la tabla `auditoria` ya guarda, desde la migración `000002`, `usuario_id`, `huella_dispositivo`, `ip_origen` y `marca_tiempo` en cada evento `usuario.login` — eso *es* el historial de dispositivos por usuario: append-only, hash-chained, con separación de privilegios y verificador de integridad independiente (ADR 0005). Una tabla `dispositivos_conocidos` sería una copia mutable, sin cadena de hashes y con menos información, del dato que ya está en la bitácora forense.

**Por qué ese historial no puede consultarse directamente en el login**: `auditoria` está serializada por `pg_advisory_xact_lock(20260830, 5)` en cada `INSERT` (ADR 0005). Consultarla en el camino caliente de autenticación la pondría en el peor lugar posible del sistema — el mismo razonamiento exacto que ya estableció INV-COLA-08 para Postgres en las colas virtuales, aplicado acá a la tabla que ese razonamiento no había llegado a nombrar. Lo que hace falta no es más almacenamiento: es un índice de consulta rápida ("¿este hash pertenece a un conjunto pequeño?"), que es una pregunta de caché, no de base de datos relacional — el mismo argumento que ya usó ADR 0018 para elegir Redis sobre Postgres para los contadores de rate limiting.

**Por qué no hace falta worker ni cola de mensajería**: introducir Redis Streams o NATS sería infraestructura nueva que contradice ADR 0002 (un solo producto, un solo proceso). Pero la razón de fondo es más simple: el análisis que este MVP puede hacer de verdad son lecturas O(1) sobre un HASH de cuatro campos, no correlación de eventos ni agregaciones pesadas — no hay nada que valga la pena diferir a un worker. Ni siquiera hace falta la goroutine con `time.Ticker` que el reconciliador de colas virtuales sí necesita (ADR 0043): allá hay que reconciliar Postgres (fuente de verdad) contra Redis (estado efímero); acá no hay una fuente de verdad en Postgres que reconciliar. La escritura del índice ya corre fuera de la transacción de login y en modo best-effort, dentro de `RegistrarResultado`, exactamente el mismo punto donde hoy se reinicia el contador de rate limiting tras un login exitoso.

## Consecuencia que se acepta por escrito

Tras un `FLUSHALL` de Redis (o simplemente al expirar el TTL), el mecanismo queda ciego para una cuenta durante sus siguientes logins hasta reconstruir el mínimo de historial exigido: no hay reconciliador que lo repueble desde Postgres, porque no hay nada en Postgres que reponer. Esto es aceptable **mientras el mecanismo sea fricción y no defensa** (ADR 0051): perder el índice no abre ningún agujero de seguridad, solo deja de agregar temporalmente un captcha. Si algún día este mecanismo pasara a alimentar una decisión de mayor peso (p. ej. step-up), esta propiedad tendría que revisarse.

## Alternativas consideradas

- **Tabla `dispositivos_conocidos` en Postgres**: descartada — sería peor evidencia que la que ya existe en `auditoria`, y consultarla en el login competiría con el advisory lock de la auditoría.
- **Worker con Redis Streams o NATS**: descartado — infraestructura nueva no justificada por la carga de trabajo real (lecturas O(1)), y contraria al alcance de un solo proceso de ADR 0002.
- **Goroutine con `time.Ticker`, mismo patrón que el reconciliador de colas**: descartada explícitamente — no hay estado que reconciliar entre dos fuentes, porque solo existe una (Redis).

## Estado

Aceptado.
