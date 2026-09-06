# ADR 0043 — Admisión por cursor derivado del reloj, no un worker sobre un ZSET

## Contexto

`docs/design/colas-virtuales.md` §1.5 necesita un mecanismo para decidir, en cada instante, a quién le toca pasar. La ficha original del agente `colas-virtuales` proponía *"un worker libera N usuarios/segundo hacia el endpoint real"* sobre *"una cola FIFO en Redis (Sorted Set con timestamp de entrada como score)"* — el diseño clásico de una sala de espera. Con varias réplicas de la API corriendo simultáneamente (el proceso `api` puede escalarse horizontalmente aunque hoy no lo haga), hay que decidir si ese diseño sigue siendo el correcto.

## Decisión

**El orden es un ordinal monótono asignado por `INCR` de Redis; la admisión es una función pura del reloj (`cursor(t) = cursorBase + floor((t − relojDesde) × ritmoAdmision)`), sin worker ni ZSET.**

```
admitido(ticket) ⟺ ticket.rango ≤ cursor(ahora)
turnoDe(rango)   = relojDesde + (rango − cursorBase) / ritmoAdmision
```

`cursorBase` y `relojDesde` viven en el agregado `SalaDeEspera` y solo se reescriben cuando cambia el ritmo de admisión, de modo que el cursor es continuo en el cambio (nadie salta de golpe, nadie retrocede).

Por qué esto es mejor que un worker con ZSET, punto por punto:

- **No hay proceso extra ni elección de líder.** Con varias réplicas, un worker de admisión necesita o un líder (con su propio mecanismo de elección y su propio modo degradado si el líder muere durante el pico) o coordinación entre réplicas. El cursor derivado lo calcula cualquier réplica, siempre igual, sin coordinarse con nadie.
- **El turno es determinista y no depende del comportamiento de los demás.** Con un worker que "libera a los N primeros vivos", abandonar la cola adelanta a los de atrás: la ETA puede bajar, pero eso implica que también puede subir (reintentos, reingresos, cambios de ritmo mal aplicados). Con el cursor, `turnoDe(rango)` es una hora fija: la espera estimada nunca empeora (INV-COLA-05) — la propiedad de UX más importante de una sala de espera.
- **La aritmética es O(1)** y cabe en un script Lua de diez líneas. Un ZSET es O(log N) por operación y exige purga de los abandonados; la purga es otro job que mantener.
- **El error se comete siempre del lado seguro.** Los turnos de quienes abandonaron o no reclamaron no se devuelven al cupo: el sistema protegido recibe *menos* de `ritmoAdmision` admisiones por segundo, nunca más (INV-COLA-04). Es la dirección de fallo correcta para un mecanismo de protección de capacidad.

## Alternativas consideradas

- **ZSET + worker liberador, como propone la ficha**: descartado por las razones de arriba. Es el diseño de manual, pero exige coordinación entre réplicas y produce una ETA que puede empeorar, la propiedad que menos se le puede pedir a una sala de espera.
- **Timestamps como score en vez de un ordinal**: descartado — dos ingresos en el mismo milisegundo (perfectamente posible bajo un pico) necesitarían desempate, y el desempate es exactamente lo que un contador atómico ya resuelve sin coordinación adicional.

## Consecuencias

- `capacidadMaximaCola` se mide como `secuencia − cursor(ahora)`, que sobrecuenta a los abandonados; se acepta porque el techo existe para proteger la memoria de Redis y sobrecontar solo lo hace más conservador.
- **Deuda técnica aceptada y documentada**: la aritmética del cursor vive dos veces — como funciones puras del dominio (`SalaDeEspera.CursorEn`, `TurnoDe`) para tests y caminos no atómicos, y replicada dentro de los scripts Lua para que la decisión sea atómica bajo concurrencia. Es la misma duplicación que ya existe entre `dominio.Umbral` y `scriptPermitir` del limitador de tasa (ADR 0018). Mitigación obligatoria: un test de consistencia sobre un corpus de casos compartido, ejecutando el script Lua contra un Redis real y comparando caso por caso con la función de dominio — sin ese test, las dos copias se desincronizan en silencio.
- No hay job de purga: el TTL de cada ticket es su ventana de utilidad, calculada en el ingreso.

## Estado

Aceptado.
