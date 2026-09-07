# ADR 0048 — El fingerprint del MVP es pasivo y del lado servidor, sin FingerprintJS ni proveedor de terceros

## Contexto

`docs/design/fingerprinting-comportamiento.md` necesita decidir cómo se obtiene la "huella de dispositivo" que alimenta el reconocimiento de origen. La ficha del agente `fingerprinting-comportamiento` proponía una librería cliente tipo FingerprintJS generando un `visitor_id` estable, complementada con señales pasivas del servidor (TLS fingerprint JA3/JA4, orden de cabeceras).

## Decisión

**El MVP usa exclusivamente lo que el sistema ya recibe: la cabecera `X-Device-Fingerprint` (ya poblada en `OrigenSolicitud.HuellaDispositivo()` en los cuatro contextos) y el prefijo de red de la IP de origen. No se adopta ninguna librería cliente ni proveedor de terceros.**

El insumo ya llega y ya se persiste: `auditoria.huella_dispositivo` (migración `000002`) y `sesiones.huella_dispositivo` (migración `000006`) guardan ese dato desde antes de que este diseño existiera. El hueco real no era *conseguir* una huella, era *dejar de tirar* la que ya llega. Adoptar una librería cliente (con su versión gratuita limitada o de pago, su superficie de privacidad adicional y su acoplamiento del backend a un formato de terceros) para resolver un problema cuyo insumo ya está en el `context.Context` sería construir la parte cara del problema antes que la barata.

**Contrapartida que se acepta por escrito, no se esconde**: la huella es un dato del cliente y por lo tanto falsificable — omitir la cabecera, o mandar un valor distinto en cada request, es trivial. El diseño lo asume como premisa (INV-RIES-06 de `fingerprinting-comportamiento.md`): la huella *nunca autoriza*, solo alimenta un puntaje, y existe la señal `huella_ausente` precisamente para que omitir la cabecera no sea una evasión gratis (cuesta lo mismo que traer una huella nueva).

## Señales evaluadas y descartadas para el MVP, con su motivo

| Señal | Veredicto |
|---|---|
| TLS fingerprint (JA3/JA4) | Backlog — ningún proxy/gateway del despliegue actual lo expone. Entra como una señal más el día que exista, sin tocar el modelo. |
| Geovelocidad imposible | Backlog con puerto nombrado (`LocalizadorDeIP`) — es la señal más valiosa, pero exige una base GeoIP o una API de terceros; merece su propio hito, no un renglón de este MVP. |
| Horario inusual vs. histórico | Backlog — exige un histograma por usuario (estado durable nuevo) y tiene mala relación señal/ruido (turnos, viajes). |
| Frecuencia anómala de acciones sensibles | Ya resuelto por ADR 0018 (`invitar_miembro`, `crear_organizacion`, `cierre_masivo_sesiones` ya tienen su propio umbral) — fuera de alcance por duplicación, no por dificultad. |
| Renovación de sesión desde huella distinta | Es de **Acceso**, no de Confianza: es una comparación entre dos campos del propio agregado `Sesion`. Se anota para que nadie lo resuelva desde acá. |
| Velocidad de tecleo / interacción del formulario | Descartado — exige instrumentar el frontend, es el dato más invasivo de la lista y el de peor relación señal/ruido. |

## Alternativas consideradas

- **FingerprintJS u otra librería cliente**: descartada por las razones de arriba.
- **TLS fingerprint como señal del MVP**: descartada — no hay componente en el despliegue actual que lo exponga; forzarlo exigiría infraestructura de red nueva para una sola señal.

## Consecuencias

- Ninguna dependencia nueva en el frontend ni en el backend para este mecanismo.
- La huella sigue viajando exactamente como hoy (`X-Device-Fingerprint` → `OrigenSolicitud.HuellaDispositivo()`); este ADR no cambia su transporte, solo empieza a usarla.
- `LocalizadorDeIP` y las demás señales de backlog quedan nombradas en `docs/design/fingerprinting-comportamiento.md` §2.3 para que un hito futuro no tenga que reinventar el vocabulario.

## Estado

Aceptado.
