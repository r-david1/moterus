# ADR 0031 — Aislamiento multi-tenant con RLS en Postgres, vía GUC por transacción, como red de seguridad adicional

## Contexto

`internal/tenencia/adaptadores/postgres/doc.go` prometía desde su creación *"repositorios [...] sobre pgx/sqlc con RLS por tenant"*, y ADR 0017 dejó anotado explícitamente que el aislamiento multi-tenant quedaba como *"ADR pendiente de Tenencia"*. Con el diseño del contexto (`docs/design/tenencia-bounded-context.md` §6.3) hay que fijar el mecanismo concreto antes de escribir la migración `000014`.

La pregunta no es *si* filtrar por organización — el filtrado en la capa de aplicación (todo repositorio de Tenencia recibe un `IDOrganizacion` explícito, INV-TEN-13) ya lo hace. La pregunta es si además hace falta una **segunda barrera, en la base de datos**, que falle cerrado incluso si un caso de uso futuro olvida el filtro, y con qué mecanismo.

## Decisión

**Row Level Security de Postgres sobre `organizaciones`, `membresias` e `invitaciones`, con `FORCE ROW LEVEL SECURITY`, activada por dos GUC de sesión fijados con `SET LOCAL` al abrir cada transacción: `app.usuario_actual` y `app.organizacion_actual`.**

### Mecanismo

- `plataforma/bd` (la `UnidadDeTrabajo`), al abrir una transacción que lleva un `AlcanceDeTenencia` publicado en el `context.Context` por el middleware de autorización, emite `SET LOCAL app.usuario_actual = $1` y `SET LOCAL app.organizacion_actual = $2` con parámetros vinculados (nunca concatenación de strings).
- Dos funciones `STABLE` (`tenencia_usuario_actual()`, `tenencia_organizacion_actual()`) leen esos GUC con `current_setting(..., true)`, devolviendo `NULL` si no están fijados.
- Las políticas de `organizaciones` y `membresias` comparan contra esas funciones; una política cruzada usa una tercera función `SECURITY DEFINER` (`tenencia_es_miembro_activo`) para evitar que la política de `organizaciones` dispare RLS recursivo al consultar `membresias`.
- **Única vía de escape, nombrada y acotada**: `tenencia_resolver_invitacion(hash)`, `SECURITY DEFINER`, porque `AceptarInvitacion` necesita leer una fila de `invitaciones` de alguien que todavía no es miembro de ninguna organización — no hay `app.organizacion_actual` posible en ese punto. Devuelve solo los campos que ese caso de uso necesita, nunca la tabla completa.
- `FORCE ROW LEVEL SECURITY` aplica también al rol dueño de las tablas: las migraciones y semillas de este contexto hacen solo DDL, nunca DML directo sobre estas tres tablas.

### La objeción obvia, respondida por escrito

ADR 0017 rechazó `SET ROLE` como mecanismo de privilegio por conexión, con el argumento de que *"es reversible por cualquier código con acceso a la conexión [...] y depende de que todo el código lo ejecute sin excepción antes de la primera query"*. `SET LOCAL app.organizacion_actual` **parece** el mismo patrón y merece la misma sospecha, pero la dirección del fallo es opuesta:

- Olvidar `SET ROLE` deja al proceso con privilegios *de más* — falla **abierto**.
- Olvidar `SET LOCAL app.organizacion_actual` deja la política sin valor con qué comparar: `current_setting` devuelve `NULL`, la comparación es `NULL`, y **no se ve ninguna fila** — falla **cerrado** (INV-TEN-30).

Un olvido de este mecanismo produce un bug ruidoso e inmediato en desarrollo (cero resultados donde se esperaban filas), no una fuga silenciosa en producción. Además, `SET LOCAL` muere con el `COMMIT`/`ROLLBACK` de la transacción, así que no puede filtrarse a la siguiente petición que tome esa misma conexión física del pool — que es precisamente el otro riesgo de `SET ROLE` sobre un pool compartido que ADR 0017 señaló.

### Precedente ya existente que no hay que tocar

`rol_login_identidad` se creó con `NOBYPASSRLS` explícito desde `000003_rol_login_aplicacion.up.sql` — RLS ya estaba anticipado en el rol de runtime, sin que nada dependiera de él hasta ahora. No hace falta ninguna migración sobre el rol; solo verificar empíricamente el atributo con un test de integración (`test/integracion/privilegios_test.go`, que ya existe para ADR 0017).

## Alternativas consideradas

- **Solo filtrado en la capa de aplicación, sin RLS**: descartada — es exactamente el patrón "confiamos en que ningún caso de uso futuro olvide el `WHERE organizacion_id = ...`", el mismo tipo de garantía "una comprobación en la aplicación y confiamos" que este proyecto ya rechazó dos veces (INV-ID-02 con índice único, INV-ACC-04 con índice único parcial). RLS es la garantía estructural equivalente para el aislamiento multi-tenant.
- **`SET ROLE` a un rol por organización**: descartada de plano — no solo por el argumento ya citado de ADR 0017, sino porque requeriría un rol de Postgres por organización, una idea que no escala con el número de tenants y que además choca directamente con la decisión ya tomada en ADR 0017 de un único rol de runtime de privilegios acotados.
- **Aislamiento a nivel de esquema (un `schema` por organización)**: descartada — no escala operativamente (una migración tendría que aplicarse N veces), complica cualquier consulta agregada cross-tenant que el propio sistema necesite (ej. reportes internos), y Postgres RLS ya resuelve el mismo problema sin ese costo.
- **RLS sin `FORCE`**: descartada — sin `FORCE`, el rol dueño de la tabla se salta la política, y la garantía de aislamiento volvería a depender de con qué rol se ejecuta cada consulta, que es exactamente el problema que ADR 0017 ya cerró para el resto del sistema.

## Consecuencias

- Nueva migración `000014_rls_tenencia.{up,down}.sql`, aplicada **después** de que existan casos de uso reales que la ejerzan (§10 del diseño, paso 6) — una política RLS sin tráfico que la atraviese es una política no probada.
- `plataforma/bd` gana la responsabilidad de emitir `SET LOCAL` a partir de un `AlcanceDeTenencia` (§11.4 del diseño); si no hay alcance en el contexto, no se emite nada y las políticas fallan cerradas — no se permite un valor por defecto ni un alcance comodín.
- Las transacciones de Identidad y Acceso, que no fijan alcance de Tenencia y no tocan sus tablas, no cambian de comportamiento.
- Los jobs de mantenimiento futuros (purga de invitaciones vencidas, reportes) necesitan un rol Postgres propio con `BYPASSRLS`, nunca el rol de la API — se crea cuando exista el primer job, no antes.
- El mismo mecanismo (GUC + función `STABLE` + política) es el que se reutilizará para el aislamiento por organización de la lectura de `auditoria` que `000002_crear_auditoria.up.sql` dejó anotado como pendiente, con la salvedad ya escrita ahí: nunca aplica al verificador de integridad, que revisa la cadena completa o no la revisa.
- Requiere un test de integración dedicado que verifique tanto el comportamiento *fail-closed* (sin `SET LOCAL`, cero filas) como el aislamiento cruzado real (dos organizaciones, ninguna ve las membresías ni los datos de la otra).

## Estado

Aceptado.
