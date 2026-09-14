# ADR 0053 — Regla de la salida alcanzable, como norma transversal

## Contexto

Al analizar el candidato 0011 (bloqueo de cuenta, ADR 0011) surgió un criterio que el sistema ya venía aplicando de forma implícita en varios mecanismos, sin haberlo escrito nunca como regla general. ADR 0051 ya lo había descubierto en un caso particular: producir `Decision.RequiereStepUp` desde el reconocimiento de origen dejaría bloqueado sin salida a cualquier usuario sin MFA confirmado, porque `VerificarOTP` no tiene forma de completar un segundo factor que el usuario nunca configuró.

`docs/design/bloqueo-cuenta.md` §3.2 generaliza ese hallazgo: es el mismo argumento que decide, mecanismo por mecanismo, si una fricción es aceptable o es en realidad un bloqueo disfrazado.

## Decisión

**Todo mecanismo que pueda denegarle el acceso a una cuenta concreta tiene que ofrecerle a su titular legítimo una salida que él pueda recorrer por sí mismo — sin depender de un tercero, de un rol que no existe, ni de un canal no implementado. Un mecanismo cuya salida no está disponible en el despliegue actual no es un mecanismo de contención: es un bloqueo, y no puede desplegarse hasta que su salida exista de verdad.**

Esta regla generaliza INV-RIES-01/INV-RIES-02 (ADR 0051) a todo el sistema, y explica por qué ADR 0011 concluyó que un lockout persistente no es viable hoy: sus tres salidas candidatas (acción administrativa, correo, tiempo) no existen en este despliegue.

La tabla de asimetrías que ADR 0018 empezó (y que ADR 0044 y ADR 0051 continuaron) gana una columna:

| Mecanismo | Ante fallo del motor | Conmutable | **Salida del titular** |
|---|---|---|---|
| Rate limiting por IP | open | no | esperar ≤ 1 min, o cambiar de red |
| Rate limiting por cuenta | open | no | **captcha, en el mismo intento** — depende de `TURNSTILE_SECRET_KEY` |
| Captcha | closed | no | resolverlo |
| Sala de espera (colas virtuales) | open | sí, por sala | esperar el turno (nadie es rechazado) |
| Reconocimiento de origen | open | no | captcha, en el mismo intento |
| Bloqueo persistente de cuenta | — | — | ninguna → por eso no se implementa (ADR 0011) |

**Consecuencia inmediata de aplicar la regla al propio sistema**: la fila "rate limiting por cuenta" depende de que `TURNSTILE_SECRET_KEY` esté configurada para que su salida exista de verdad. Hoy, `APP_ENV=production` sin esa llave arranca con un `slog.Error` y el captcha queda fail-closed — es decir, arranca con la única salida del cooldown de cuenta ya cerrada, violando esta misma regla desde el primer minuto.

## Alternativas consideradas

- **Dejar la regla implícita**, como venía estando: descartado — es precisamente la falta de una norma escrita la que permitió que el escalado exponencial de ADR 0018 se aplicara a la clave de cuenta sin que nadie evaluara si tenía salida (ADR 0052), y la que dejaría a alguien libre de reabrir el lockout de cuenta sin pasar esta prueba.
- **Aplicar la regla solo a mecanismos futuros**, sin auditar los existentes: descartado — el propio proceso de escribir esta regla fue el que encontró la violación ya en producción (ADR 0052); una norma que no se aplica retroactivamente a lo ya construido pierde la mitad de su valor.

## Consecuencias

- Todo diseño futuro que agregue un mecanismo de denegación de acceso por cuenta tiene que completar esta columna antes de implementarse — es una casilla de checklist, no un párrafo de buenas intenciones.
- `TURNSTILE_SECRET_KEY` se trata como una dependencia dura de la salida del rate limiting por cuenta, no una mejora opcional. Con conformidad explícita del usuario del proyecto (2026-09-14), producción **falla al arrancar** sin esa llave (`cmd/api/main.go`, `construirEvaluadorDeRiesgo`) — mismo patrón que ADR 0020 ya usa para `ACCESO_LLAVE_FIRMA`/`ACCESO_EMISOR`/`ACCESO_AUDIENCIA` (no el de ADR 0017, que solo emite un `WARN` y deja arrancar con el rol dueño como fallback); ver `docs/design/bloqueo-cuenta.md` §3.2 punto 2 y §11 paso 4b.
- `EstadoBloqueado` de Identidad queda reservado a una decisión humana con sujeto y motivo (INV-BLQ-08): cuando exista un caso de uso administrativo de bloqueo, reutilizará el evento `EstadoUsuarioCambiado` y la acción `usuario.estado_cambiado` ya existentes, sin migración ni catálogo nuevo — porque ese bloqueo sí tendrá una salida (la misma acción humana que lo abrió, revertida).

## Estado

Aceptado.
