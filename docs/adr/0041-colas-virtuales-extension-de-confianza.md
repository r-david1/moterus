# ADR 0041 — Colas de acceso virtual: extensión del contexto Confianza, no un contexto nuevo

## Contexto

`docs/design/colas-virtuales.md` diseña un mecanismo de sala de espera virtual para proteger la capacidad agregada del sistema ante picos de tráfico legítimo (apertura de inscripciones, lanzamiento de una organización grande) — a diferencia del rate limiting de ADR 0018, que protege contra el comportamiento anómalo de un origen concreto. La ficha original del agente `colas-virtuales` no se pronuncia sobre dónde vive esto en la arquitectura hexagonal; hay que decidirlo antes de escribir una sola línea de dominio.

La pregunta no es trivial: hoy `confianza/dominio` es política sin estado (`Umbral`, `PoliticaLimites`, `Decision`) y `confianza/adaptadores/postgres/doc.go` sigue vacío ("Pendiente de implementación"). Esta extensión introduce en Confianza su primer agregado con ciclo de vida y su primera tabla en Postgres — un salto cualitativo real que merece ser reconocido, no esquivado.

## Decisión

**La sala de espera vive en `internal/confianza/`, como extensión aditiva del bounded context existente, no como un contexto nuevo.**

Cuatro razones, en orden de peso:

1. **La carta del contexto ya la incluye.** La definición de Confianza en el proyecto es *"rate limiting, fingerprinting, análisis de comportamiento, captcha, colas virtuales"* — todo lo que decide "¿confío en este request?". No hay que decidir dónde ponerlo: hay que decidir si esa asignación previa sigue teniendo sentido, y la tiene.
2. **La infraestructura ya estaba reservada, por escrito.** `internal/plataforma/cache/doc.go` dice, desde antes de que existiera una línea de Confianza: *"cliente Redis compartido […] usado por rate limiting, colas virtuales y cachés de lectura"*. ADR 0018 citó ese mismo comentario para elegir Redis; sería incoherente usarlo como precedente para el limitador y descartarlo para la pieza que el propio comentario nombra explícitamente.
3. **Es la misma clase de decisión, en el mismo punto del flujo.** El limitador y la sala responden en el mismo instante (antes del caso de uso real), con la misma forma de respuesta, sobre el mismo motor de estado efímero, en el mismo borde HTTP. Cambia la pregunta (*"¿este cliente abusa?"* vs. *"¿hay capacidad agregada ahora?"*), no el momento ni el mecanismo.
4. **Un contexto nuevo duplicaría cinco piezas para un solo agregado.** Tendría que redeclarar `IDUsuario`, `IDOrganizacion`, `OrigenSolicitud` y `EventoDominio` (cuarta repetición de la misma discusión que ya resolvieron Identidad, Acceso y Tenencia), su propio adaptador Redis sobre el mismo cliente, su propio ACL hacia Auditoría y su propio ACL hacia Tenencia.

## Alternativas consideradas

- **Contexto nuevo `salas-espera` o `perimetro`**: descartado. El mejor argumento a favor era real (Confianza deja de ser "un evaluador puro" y pasa a ser "un contexto con estado"), pero el salto es de **tamaño**, no de **responsabilidad**: la tabla `salas_espera` no modela un concepto de negocio nuevo, modela la configuración operativa del propio mecanismo de perímetro — igual que `PoliticaLimites` modela la configuración del limitador, con la diferencia de que ésta debe poder cambiarse en caliente durante un evento y sobrevivir a un reinicio. Un contexto cuya razón de ser es "es que tiene una tabla" no es un bounded context, es un paquete.
- **Colgar la sala de Acceso** (por ser el consumidor más importante): descartado — la sala protege tres rutas de dos contextos distintos (Acceso e Identidad) más una de Tenencia; colgarla de uno de sus consumidores rompería la misma separación que ADR 0009 ya estableció entre autenticar y emitir tokens.

## Consecuencias

- `internal/confianza/adaptadores/postgres/doc.go` y `.../http/doc.go` dejan de decir "Pendiente de implementación".
- Confianza gana, por primera vez, un agregado con ciclo de vida (`SalaDeEspera`), una tabla en Postgres (`salas_espera`) y una bitácora de auditoría propia.
- Confianza gana también, por primera vez, los puertos `GeneradorIDs`, `Reloj` y `RegistroAuditoria`, y un ACL propio hacia `tenencia/puertos` (`confianza/adaptadores/tenencia/`) — misma regla de frontera que INV-TEN-28.
- Ningún caso de uso, puerto ni agregado de Identidad, Acceso o Tenencia se modifica más allá del registro de rutas: la extensión es aditiva y reversible (quitar el middleware de las tres rutas protegidas la desactiva por completo).

## Estado

Aceptado.
