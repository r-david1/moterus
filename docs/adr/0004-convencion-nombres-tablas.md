# ADR 0004 — Convención de nombres de tablas: español, sin prefijo

## Contexto

Se requería fijar la convención de nombres de tablas en PostgreSQL: en español, y se pidió evaluar un prefijo de proyecto sobre cada nombre de tabla.

## Investigación

Fuentes consultadas (PostgreSQL naming conventions — GeeksforGeeks, Bytebase SQL style guide, guías generales de Postgres):

- Prefijos tipo `tbl_` o Hungarian notation están explícitamente desaconsejados en general.
- Un prefijo que identifica **módulo/proyecto** (no tipo de objeto) es una práctica aceptada como alternativa a usar *schemas* separados de Postgres — pero la alternativa más "nativa" de Postgres es usar un **schema** (`nombre_proyecto.usuarios`) en vez de un prefijo en el nombre de la tabla (`prefijo_usuarios`).
- Consenso general: tablas en plural, `snake_case`, minúsculas; columnas en singular; FK con formato `tabla_singular_id`; booleanos con prefijo `es_`/`tiene_`.

## Decisión

Primero se fijó un prefijo (`mot_`, ligado al nombre de un proyecto de prueba) siguiendo la variante "prefijo de módulo". **Se revirtió esa decisión**: para un solo producto (ver ADR 0002), sin necesidad de compartir el schema `public` con otros sistemas, el prefijo no aporta nada que el nombre ya no diga — se descartó a favor de nombres de tabla **directos, sin prefijo**: `usuarios`, `organizaciones`, `membresias`, `auditoria`.

Se mantiene el resto de la convención investigada: español, snake_case, plural, sin tildes ni ñ en identificadores (fricción real con `sqlc`/migraciones — tildes solo en comentarios `COMMENT ON`), columnas en singular, FK como `tabla_id`, booleanos `es_`/`tiene_`.

## Alternativas consideradas

- **Prefijo de proyecto (`mot_usuarios`)**: probado y descartado — agrega ruido a cada referencia sin resolver un problema real en un esquema de un solo producto.
- **Schema de Postgres dedicado (`nombre_proyecto.usuarios`)**: evaluado como la opción "más correcta" a nivel Postgres, pero descartado por ahora por la gestión adicional que implica (permisos, `search_path`, portabilidad con herramientas que asumen `public`). Queda como opción a reconsiderar si el proyecto crece a multi-producto (ver ADR 0002) y se necesita separar namespaces reales.

## Consecuencias

- Todas las tablas nuevas siguen el patrón sin prefijo desde este punto en adelante.
- Si se reintroduce el escenario multi-producto, este ADR es el punto de partida para decidir entre prefijo por producto o schemas separados — no hay que rehacer la investigación desde cero.

## Estado

Aceptado (con reversión documentada del prefijo inicial).
