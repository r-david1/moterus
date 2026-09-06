# ADR 0044 — Caída de Redis en la sala de espera: fail-open por defecto, fail-closed conmutable por sala

## Contexto

ADR 0018 ya fijó una asimetría deliberada para el contexto Confianza: rate limiting es fail-open (*"una caída de Redis no puede tumbar login/registro por completo"*), captcha es fail-closed (*"un atacante no debe poder anular la verificación provocando fallos contra el proveedor"*). La sala de espera necesita el mismo tipo de decisión explícita, y el análisis no da el mismo resultado que ninguno de los dos casos anteriores.

Si Redis cae mientras una sala está abierta, no hay tickets válidos, no hay cursor y no hay cola: el 100% del tráfico de la ruta protegida no puede presentar un ticket válido. Fail-closed puro significa, literalmente, `503` para todos los logins del sistema hasta que Redis vuelva.

## Decisión

**Fail-open por defecto (`modoDegradado = permitir`), con fail-closed conmutable por sala (`modoDegradado = rechazar`) para el operador que lo declare explícitamente al abrir la sala o en caliente. El modo se decide desde una instantánea en memoria respaldada por Postgres, nunca leyendo Redis.**

El argumento decisivo es de probabilidades, no de principios: la caída de Redis es un evento cierto y observado; el pico simultáneo contra el que la sala protege es hipotético en ese mismo instante. Elegir fail-closed puro es aceptar un daño seguro (caída total de la autenticación, causada por el mecanismo que existe para *mejorar* la disponibilidad) para evitar uno posible.

Pero el argumento contrario es real y hay que reconocerlo: si la sala está abierta es porque el pico está ocurriendo *ahora*. En pleno evento, "dejar pasar a todos" es exactamente la avalancha que la sala existe para contener, y puede tumbar el sistema real (Postgres, no solo Redis) — con el agravante de que, con Redis caído, el rate limiting de ADR 0018 también está fail-open, así que el perímetro entero desaparece a la vez.

Por eso el modo es **conmutable por sala**, no una constante global: un operador que abre una sala para un evento crítico, donde tumbar el backend cuesta más que rechazar tráfico, declara `rechazar` sin necesitar un despliegue.

El detalle que hace esto implementable: **el modo degradado no puede leerse del componente que puede estar caído.** Un `modoDegradado` que hubiera que leer de Redis sería papel mojado en el instante exacto en que hace falta. Por eso vive en la instantánea en memoria que el reconciliador (`docs/design/colas-virtuales.md` §3.7) mantiene actualizada desde Postgres cada 15 segundos — el mismo mecanismo que le permite al middleware saber si hay una sala vigente sin tocar Redis.

## Alternativas consideradas

- **Fail-open puro para toda sala, sin excepción (igual que rate limiting)**: descartado — deja al operador de un evento crítico sin ninguna defensa real: el escenario que la sala existe para prevenir (una avalancha que tumba Postgres) queda completamente desprotegido justo cuando Redis, el componente que sostiene la protección, es el primero en caer bajo esa misma carga.
- **Fail-closed puro para toda sala (igual que captcha)**: descartado por el análisis de arriba — convierte una caída de un componente de caché en una caída total de la autenticación del sistema completo, incluso para las rutas de login que no tienen nada que ver con el evento que motivó abrir la sala.
- **Leer el modo degradado de la propia configuración de Redis (`cfg` hash)**: descartado — es exactamente el caso que falla cuando más se necesita: si Redis no responde, tampoco se puede leer de dónde saldría el modo a aplicar.

## Consecuencias

- `PoliticaSala.modoDegradado` es parte de la configuración de la sala, persistida en Postgres (`salas_espera.modo_degradado`), no una variable de entorno ni una constante en código.
- El reconciliador (`ReconciliarSalas`) es el componente crítico que sostiene esta decisión: sin su instantánea en memoria, el modo degradado no sería consultable con Redis caído.
- Ante una caída detectada con `modoDegradado = permitir`: `slog.Error` (no `Warn` — es más grave que un contador de rate limit perdido) y un contador de métrica dedicado, para que la degradación sea visible aunque no bloquee tráfico.
- Ante una caída detectada con `modoDegradado = rechazar`: `503` con `desenlace: "sala_no_disponible"` y `Retry-After` corto (30 s).
- Decidir el `modoDegradado` pasa a ser parte del checklist de "listo para el evento" al abrir una sala, no un detalle de configuración por omisión — mismo espíritu que la nota de ADR 0018 sobre `TURNSTILE_SECRET_KEY`.

## Estado

Aceptado.
