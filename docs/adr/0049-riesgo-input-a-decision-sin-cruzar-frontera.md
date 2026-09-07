# ADR 0049 — El riesgo de origen es un input más a `Decision`, y nunca cruza la frontera de contexto

## Contexto

`docs/design/fingerprinting-comportamiento.md` necesita decidir cómo se relaciona el puntaje de riesgo de origen con el `TrustScore`/`Decision` que Confianza ya calcula para rate limiting y captcha, y si ese puntaje puede llegar a un consumidor externo (Identidad, Acceso, Tenencia) o a un cliente HTTP.

## Decisión

**El resultado del análisis (`PuntajeRiesgo`, `NivelRiesgo`, `SenalesDeRiesgo`) es un input más a la `Decision` que `EvaluarTrustSignalCasoDeUso` ya produce — no un score paralelo con su propio puerto — pero esos tres campos nunca cruzan la frontera de contexto: ninguno de los tres ACL (`identidad|acceso|tenencia/adaptadores/confianza`) los mapea hacia su propio `DecisionConfianza`.**

**Por qué es un input a la misma `Decision`, no un mecanismo paralelo**: un score paralelo exigiría un puerto de entrada paralelo, un ACL paralelo en cada uno de los tres consumidores, y un punto de consumo paralelo en cada caso de uso — exactamente la clase de duplicación que ADR 0041 ya rechazó para las colas virtuales. `Decision` ya es el vehículo que Identidad/Acceso/Tenencia saben interpretar; el riesgo de origen solo necesita sumar su voto a esa misma decisión (vía `RequiereCaptcha`, nunca vía `RequiereStepUp` — ver ADR 0051).

**Por qué esos tres campos nunca cruzan la frontera, que es la mitad no obvia de esta decisión**: `Decision.Puntaje` (el puntaje de *captcha*) sí llega hasta el cliente HTTP, vía `ResultadoAutenticacion.PuntajeConfianza` — es una decisión ya tomada y este ADR no la reabre. Pero reutilizar ese mismo camino para publicar el puntaje de *riesgo de origen* le daría a un atacante una señal directa de cuánto le falta para disparar el detector, permitiéndole calibrar sus intentos contra el clasificador en vivo. Esa es la diferencia entre un puntaje que existe para que el usuario demuestre que es humano (captcha, público por diseño) y uno que existe para detectar un patrón sin que el atacante pueda iterar contra él.

## Alternativas consideradas

- **Puerto y ACL paralelos para el riesgo de origen**: descartados por la duplicación que ya rechazó ADR 0041.
- **Publicar `PuntajeRiesgo`/`NivelRiesgo` junto con `Decision.Puntaje`**: descartado — convertiría el mecanismo de detección en un oráculo consultable por el propio atacante en cada intento.
- **Un endpoint de consulta de riesgo para el propio usuario** ("por qué me pidieron captcha"): descartado por el mismo motivo, y porque no hay ningún consumidor que lo necesite hoy.

## Consecuencias

- Los tres ACL poblan los campos nuevos de `Solicitud`/`ResultadoIntento` que sí necesitan (huella, `IDUsuario`, `IDSolicitud`) pero tienen prohibido mapear `PuntajeRiesgo`/`NivelRiesgo`/`SenalesDeRiesgo` hacia su `DecisionConfianza` — cada uno lleva un test que falla si alguien agrega ese mapeo (INV-RIES-09).
- Ninguna respuesta HTTP existente ni futura puede exponer el puntaje de riesgo de origen sin violar explícitamente esta decisión.
- `Decision.RequiereCaptcha` sigue siendo el único canal de salida observable de este mecanismo.

## Estado

Aceptado.
