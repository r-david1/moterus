# ADR 0011 — No hay bloqueo automático de cuenta por intentos fallidos de login

## Contexto

El diseño original de Identidad (`docs/design/identidad-bounded-context.md` §7) dejó nombrado el candidato 0011: *"Bloqueo de cuenta: el estado persistente `bloqueado` es de Identidad; el throttling efímero es de Confianza. Sin esta línea, ambos contextos terminan implementando lockout y se contradicen. Regla: Confianza decide 'ahora no' (Redis, minutos); Identidad decide 'esta cuenta está fuera de servicio' (Postgres, hasta acción explícita)."* El estado `EstadoBloqueado` existe en el dominio de Identidad desde el primer hito, con su máquina de transiciones (`Usuario.Bloquear`/`Reactivar`), pero **ningún caso de uso lo invoca en producción** — verificado: las únicas referencias fuera de `dominio/` están en tests.

Antes de escribir el caso de uso que finalmente "tira de la palanca", hacía falta responder si un bloqueo automático por intentos fallidos repetidos es siquiera deseable en este sistema. `docs/design/bloqueo-cuenta.md` es el análisis completo; este ADR registra su conclusión.

## Decisión

**No se implementa el bloqueo automático de cuenta por intentos fallidos de login.** `Usuario.Bloquear` sigue sin invocarse desde ningún caso de uso, y queda prohibido por invariante (INV-BLQ-01/02) invocarlo desde cualquier camino que un sujeto no autenticado pueda disparar.

**El argumento decisivo es de disponibilidad, no de opinión sobre seguridad**: un lockout automático convierte un ataque de confidencialidad de baja probabilidad en un ataque de disponibilidad garantizado y gratuito contra cualquier cuenta cuyo correo se conozca. El atacante no necesita ninguna credencial, ninguna cuenta propia, ni siquiera saber si la cuenta objetivo existe — solo su correo, que es público por diseño (es lo que cualquiera escribe en el campo de login).

**No existe un umbral con un punto de operación defendible**: un umbral bajo (5–10 intentos) es trivialmente disparable por un tercero con un puñado de peticiones HTTP repartidas en 1-2 IPs; un umbral alto (50-100+) nunca llega a activarse en la práctica, porque el rate limiting de cuenta que Confianza ya implementa (ADR 0018: 5 intentos / 15 min, con cooldown exponencial) ya lleva horas denegando antes de que el conteo de intentos "reales" alcance esos valores. No hay una zona intermedia donde el mecanismo sea simultáneamente difícil de disparar por un atacante y capaz de frenar algo.

**El beneficio defensivo es, además, casi nulo.** El presupuesto de adivinanzas que el sistema ya concede sin lockout (5/15 min por cuenta ⇒ ~480 intentos/día, ~60/día con el escalado) es irrelevante contra contraseñas de al menos 12 caracteres verificadas contra brechas conocidas (ADR 0014): incluso una passphrase mediocre supera por muchos órdenes de magnitud ese presupuesto. Contra la amenaza dominante hoy — credential stuffing con una contraseña ya filtrada de otra brecha — un atacante acierta en el primer intento; un lockout que se dispara en el intento 5 no aporta ninguna protección ahí.

**Ninguna de las tres formas de "salida" que un lockout necesitaría para no ser un bloqueo permanente existe en este despliegue**: no hay rol de administrador de plataforma que pueda reactivar una cuenta (mismo hallazgo que ya cerró ADR 0045); no hay envío real de correo (`NotificadorCorreoLog` es log-only, cualquier enlace de auto-desbloqueo llegaría al log del servidor, no a la persona); y un auto-desbloqueo por tiempo transcurrido es, funcionalmente, el mismo rate limiting que Confianza ya implementa, pero reimplementado en Postgres (la tabla más leída del sistema) y sin la salida por captcha que el mecanismo actual sí ofrece.

**La frontera Identidad/Confianza que la ficha 0011 proponía sí se conserva** (INV-BLQ-03), aplicando el criterio que después fijó ADR 0047 ("¿esto es un hecho de perímetro o un hecho de la cuenta?"): *"N intentos fallidos desde ahí afuera"* es un hecho de perímetro — lo produce quien envía las peticiones, no la cuenta — y un hecho de perímetro nunca debe producir estado persistente en el agregado de otro contexto. Confianza contiene (efímero, reversible solo); Identidad reserva sus transiciones de estado a decisiones humanas (persistente, irreversible sin una acción explícita).

## Alternativas consideradas

Seis mitigaciones candidatas para hacer viable un lockout se evaluaron una por una en `docs/design/bloqueo-cuenta.md` §2.2 y colapsan en dos grupos: las que ofrecen una salida automática (auto-desbloqueo por tiempo, por captcha, por enlace de correo) resultan ser el mismo mecanismo que Confianza ya implementa, con más piezas; las que no ofrecen salida automática (umbral alto, "hasta acción administrativa") son el ataque de disponibilidad de este documento, con distinta decoración. Ninguna sobrevive.

## Consecuencias

- `Usuario.Bloquear`/`Reactivar` y `EstadoBloqueado` quedan intactos en el dominio, reservados para una decisión humana (bloqueo administrativo, cuando exista un sujeto autorizado para tomarla — ver ADR 0045).
- Un test de custodia en `internal/identidad/aplicacion` falla si algún caso de uso invoca `Usuario.Bloquear` (INV-BLQ-02), para que nadie "cierre el hueco" sin releer este documento.
- Corrección de un parámetro de ADR 0018, ADR 0053 y la regla de la salida alcanzable, ambos derivados de este análisis, se documentan por separado (ADR 0052, 0053).
- Sección 10 de `docs/design/bloqueo-cuenta.md` deja las cinco precondiciones bajo las cuales esta decisión sería revisable (canal de correo real, autoservicio de recuperación, rol de operador de plataforma, un disparador que no sea falsificable por un tercero, datos de calibración) — no se cumple ninguna hoy.

## Estado

Aceptado.
