# ADR 0042 — El ticket de cola es un token opaco, no un JWT

## Contexto

`docs/design/colas-virtuales.md` §1.4 necesita fijar cómo se representa el turno de un cliente en la sala de espera. La ficha original del agente `colas-virtuales` proponía *"un `queue_token` (JWT corto, firmado, con `position` y `issued_at`)"* — el mismo tipo de solución que ya se usó para el token de step-up (ADR 0038). El repositorio tiene los dos patrones ya construidos (JWT de propósito propio, y token opaco de alta entropía como `mot_rt_` de Acceso o `mot_inv_` de Tenencia) y hay que elegir con criterio, no por costumbre o por copiar el precedente más reciente.

## Decisión

**El ticket de cola es un token opaco (`mot_cola_` + ≥43 caracteres base64url, 32 bytes de entropía), del que Redis guarda solo el SHA-256. No es un JWT.**

La pregunta que decide todo: ¿el estado que representa el token cabe entero adentro y no cambia mientras el token vive? Para el token de step-up (ADR 0038), sí: usuario + motivo, fijo durante 5 minutos. Para un ticket de cola, no: la posición **cambia por definición** mientras el ticket existe. Un JWT que la llevara adentro quedaría desactualizado en el instante mismo de firmarlo, y el cliente que lo sondee necesitaría igual una consulta al servidor — firmar la posición no ahorra la lectura de estado, solo agrega criptografía encima.

Tres razones adicionales, cada una suficiente por sí sola:

1. **Costo en el peor momento.** Emitir un JWT es una firma Ed25519 por entrante. Un pico de 5 000 ingresos en 10 s son 5 000 firmas dentro del mecanismo cuyo propósito es *no* gastar CPU en un pico. El token opaco son 32 bytes de `crypto/rand` y un SHA-256.
2. **El orden FIFO exige estado de todos modos.** Ya hace falta un contador y un cursor en Redis (ver ADR 0043); no existe la variante "sin estado" que justificaría el costo del JWT.
3. **La defensa contra confusión de propósito sale gratis y estructural.** ADR 0038 tuvo que inventar un `typ` de cabecera distinto porque el token de step-up comparte formato y llave de firma con el token de acceso — sin ese chequeo explícito, un validador con un bug podría aceptar uno por otro. Un ticket opaco, validado por un componente que no conoce ni la llave de Acceso ni el formato JWT, no puede confundirse con un token de acceso ni siquiera por un validador con un bug: no comparte llave, ni formato, ni namespace, ni validador (INV-COLA-03 se cumple por construcción, no por comprobación).

## Alternativas consideradas

- **JWT firmado con la llave de Acceso (ADR 0020), como propone la ficha**: descartado por las tres razones de arriba. Introducir un tercer JWT firmado con esa llave, para un mecanismo de perímetro que no autoriza nada, agrandaría la superficie de *type confusion* del sistema a cambio de nada.
- **JWT con llave propia de Confianza**: descartado — habría que gestionar una tercera llave y un tercer JWKS solo para evitar la colisión de `typ` con Acceso, cuando el problema de fondo (la posición desactualizada) sigue sin resolverse.

## Consecuencias

- `dominio.TicketPlano`/`dominio.HashTicket` siguen exactamente el patrón de `mot_rt_` (Acceso) y `mot_inv_` (Tenencia): generación por `GeneradorTickets` (puerto de salida), comparación en tiempo constante, `String()`/`MarshalJSON` redactados.
- El ticket viaja en la cabecera `X-Ticket-Cola`, nunca en la ruta ni en el cuerpo salvo la única respuesta de ingreso — mismo criterio que el token de refresco.
- No hay una segunda llave de firma ni un segundo JWKS que gestionar para Confianza.

## Estado

Aceptado.
