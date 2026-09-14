# ADR 0027 — Acceso duplica tres value objects triviales en vez de compartirlos con Identidad

## Contexto

`acceso/dominio` necesita, para construir sus propios agregados (`Sesion`, `ReclamacionesAcceso`), tres conceptos que `identidad/dominio` ya modela: un identificador de usuario (`IDUsuario`), una dirección IP (`DireccionIP`) y el origen de una solicitud HTTP (`OrigenSolicitud`, que envuelve IP + user-agent). `docs/design/acceso-bounded-context.md` §1.7 la nombró como candidata (0027): compartir esos tres tipos entre contextos (shared kernel de dominio) o duplicarlos.

Se implementó la duplicación: `internal/acceso/dominio/identificadores.go` define su propio `IDUsuario` (solo valida forma de UUID; nunca genera IDs nuevos, siempre recibe uno ya resuelto por Identidad) y `internal/acceso/dominio/origen_solicitud.go` define su propio `DireccionIP` y `OrigenSolicitud` — ninguno de los dos importa un tipo de `identidad/dominio`. El puente entre ambos vive exclusivamente en `internal/acceso/adaptadores/identidad/` (el ACL, único paquete de Acceso autorizado a importar `identidad/puertos` — INV-ACC-19), que traduce `acceso/dominio.OrigenSolicitud` → `identidad/dominio.OrigenSolicitud` al llamar al caso de uso de Identidad (`origenIdentidadDesde`).

## Decisión

**`acceso/dominio` nunca importa `identidad/dominio` (INV-ACC-18), ni se promueven estos tres tipos a un paquete compartido en `internal/plataforma`.** Se acepta la duplicación deliberada de ~120 líneas sin lógica de negocio compartida entre los dos contextos, con la traducción concentrada en el único ACL autorizado a cruzar la frontera.

La alternativa —un shared kernel de dominio— fue descartada porque un tipo compartido entre bounded contexts es un acoplamiento que no se puede deshacer después sin una migración coordinada de ambos: un cambio en el VO `Correo` de Identidad (una regla de normalización nueva, un campo adicional) podría romper la compilación de Acceso sin que nadie en Acceso lo hubiera decidido, y la frontera entre los dos contextos dejaría de existir en la práctica aunque siguiera dibujada en cualquier diagrama. `internal/plataforma` tampoco es el lugar correcto: es kernel **técnico** (reloj, IDs, config), no de negocio — la propia sección 5.2 del diseño de Identidad ya lo dice explícitamente: "si dos contextos necesitan compartir una entidad, eso es señal de que la frontera está mal trazada o de que falta un puerto", no una invitación a moverla a un paquete común.

## Alternativas consideradas

- **Shared kernel en `internal/plataforma`** (mover `IDUsuario`, `DireccionIP`, `OrigenSolicitud` a un paquete común importado por ambos contextos): descartado — es exactamente el acoplamiento que la arquitectura hexagonal/DDD de este proyecto existe para evitar; convertiría dos contextos independientes en uno que se despliega y evoluciona junto por un tecnicismo de reutilización de tres tipos triviales.
- **`acceso/dominio` importa `identidad/dominio` directamente** para reutilizar sus VOs: descartado de plano — rompe INV-ACC-18 (la regla de frontera más fácil de violar sin querer) y hace que cualquier cambio de Identidad, incluso uno que no le importa a Acceso, obligue a recompilar y potencialmente romper Acceso.
- **Traducir en el punto de uso, sin un ACL dedicado**: descartado — dispersaría la traducción por todo `acceso/aplicacion`, y cualquier caso de uso nuevo que necesitara hablar con Identidad tendría que reinventar la conversión; concentrarla en `acceso/adaptadores/identidad/` es lo que permite verificar con un solo test de imports (INV-ACC-19) que nadie más cruzó la frontera.

## Consecuencias

- Un cambio en la forma de `identidad/dominio.OrigenSolicitud` (agregar un campo, cambiar una regla de validación) no rompe la compilación de Acceso — solo exige actualizar la función de traducción en el ACL si el cambio es semánticamente relevante para lo que Acceso le envía a Identidad. Es el beneficio directo de la duplicación.
- El coste es real y se paga a conciencia: ~120 líneas casi idénticas entre `acceso/dominio/identificadores.go`/`origen_solicitud.go` e `identidad/dominio/identificadores.go`/`origen_solicitud.go`, sin lógica de negocio compartida. Un test de arquitectura (`test/arquitectura`, INV-ACC-18/19) custodia que esta duplicación no se "arregle" importando directamente en el futuro.
- El mismo criterio aplica, por construcción, a cualquier contexto nuevo que necesite un concepto que otro contexto ya modela: la pregunta correcta no es "¿cómo evito repetir este tipo?" sino "¿qué ACL traduce entre los dos, y qué puerto de entrada consume?".

## Estado

Aceptado (implementado de facto; documentado retroactivamente).
