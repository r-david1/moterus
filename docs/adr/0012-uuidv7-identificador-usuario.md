# ADR 0012 — UUIDv7 como identificador de usuario

## Contexto

`IDUsuario` necesita un formato de identificador para la tabla más leída y más escrita del sistema (`usuarios`). Postgres indexa la clave primaria en un B-tree; el orden de inserción de las filas afecta directamente la localidad de ese índice y, con volumen, el rendimiento de escritura.

`docs/design/identidad-bounded-context.md` §7 nombró esta decisión como candidata (0012); se implementó de facto junto con el resto del dominio (`IDUsuario`, en `internal/identidad/dominio/identificadores.go`, y el adaptador `internal/identidad/adaptadores/postgres/generador_ids.go`) sin disputarse durante la implementación.

## Decisión

**`IDUsuario` se genera como UUID versión 7** (`internal/plataforma/ids.GenerarUUIDv7`, sobre `github.com/google/uuid`), no UUIDv4.

UUIDv7 codifica un timestamp de milisegundos en sus primeros 48 bits, seguido de bytes aleatorios — es decir, es **ordenable en el tiempo de creación** manteniendo el resto de las propiedades de un UUID (128 bits, prácticamente sin colisión, generable sin coordinación central). Para una clave primaria que se inserta constantemente, eso significa que las filas nuevas siempre caen al final del B-tree del índice de la PK, en vez de dispersarse aleatoriamente por todo el árbol como ocurre con UUIDv4 — menos fragmentación de páginas, mejor localidad de caché, escrituras más baratas.

**Contrapartida aceptada explícitamente**: UUIDv7 filtra el instante aproximado de creación del registro (a diferencia de UUIDv4, que no filtra nada). Para un `IDUsuario` interno esto es aceptable — no es un secreto ni un token que un atacante pueda usar para inferir algo sensible por sí solo. La misma generación **no** se usa para tokens de un solo uso o secretos de alta entropía (tokens de verificación de correo, de refresco, de invitación, tickets de cola): esos siguen el patrón de `crypto/rand` + hash, documentado en cada contexto por separado, precisamente porque ahí la filtración de tiempo sí sería una debilidad.

## Alternativas consideradas

- **UUIDv4 (aleatorio puro)**: descartado por la fragmentación de índice B-tree bajo volumen de escritura sostenido.
- **Un entero autoincremental (`BIGSERIAL`)**: descartado — expone el conteo total de usuarios registrados (enumerable secuencialmente) y no es generable del lado de la aplicación sin ida y vuelta a la base, lo que complica la construcción del agregado antes de persistir.
- **ULID u otro formato ordenable no estándar**: descartado — UUIDv7 ya es un estándar (RFC 9562) con soporte de librería madura, sin necesidad de adoptar un formato de codificación propio.

## Consecuencias

- El mismo criterio se replicó en todos los identificadores de agregado de cada contexto nuevo desde entonces: `IDFactorMFA`, `IDOrganizacion`, `IDMembresia`, `IDInvitacion`, `IDSalaDeEspera` — todos UUIDv7, todos generados por el puerto `GeneradorIDs` de su propio contexto envolviendo `internal/plataforma/ids.GenerarUUIDv7`.
- Nunca se usa UUIDv7 para un secreto de alta entropía (tokens, tickets, códigos) — ver el comentario de cada value object de token en cada contexto para el criterio contrario (aleatoriedad pura, sin componente de tiempo).

## Estado

Aceptado (implementado de facto; documentado retroactivamente).
