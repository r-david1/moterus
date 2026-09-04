-- Queries sqlc del contexto Tenencia. Implementan
-- tenencia/puertos.RepositorioOrganizaciones, RepositorioMembresias y
-- RepositorioInvitaciones (ver docs/design/tenencia-bounded-context.md,
-- sección 2.2, y las migraciones 000009_crear_organizaciones_membresias,
-- 000013_crear_invitaciones y 000014_rls_tenencia).
--
-- Regla de frontera (INV-TEN-29, mismo criterio que INV-ID-20/INV-ACC-20):
-- solo el adaptador tenencia/adaptadores/postgres consume este código
-- generado. Ninguna consulta de este archivo incluye la tabla `usuarios`
-- en su FROM o su JOIN: las columnas *_id que referencian usuarios(id) son
-- integridad referencial, no permiso de lectura de esa tabla.
--
-- ObtenerInvitacionPorHash usa la función SECURITY DEFINER
-- tenencia_resolver_invitacion (000014), la ÚNICA vía de escape de RLS del
-- contexto: quien acepta una invitación todavía no es miembro de ninguna
-- organización, así que no hay `app.organizacion_actual` posible en ese
-- punto.

-- --- Organizaciones -----------------------------------------------------------

-- name: CrearOrganizacion :one
INSERT INTO organizaciones (
    id, alias, nombre, estado, creada_por, creada_en, actualizada_en,
    archivada_en, motivo_estado
) VALUES (
    $1, $2, $3, $4, $5, $6, $6, $7, $8
)
RETURNING *;

-- name: ActualizarOrganizacion :one
-- Persiste el estado completo del agregado tras una mutación de negocio
-- (Renombrar, CambiarAlias, Suspender, Reactivar, Archivar). Mismo patrón
-- que ActualizarSesion en acceso/db/consultas: el adaptador intenta este
-- UPDATE primero; si no afecta ninguna fila (pgx.ErrNoRows con :one +
-- RETURNING) es porque la organización todavía no existe, y entonces se
-- hace el INSERT (CrearOrganizacion).
UPDATE organizaciones
SET alias = $2,
    nombre = $3,
    estado = $4,
    actualizada_en = $5,
    archivada_en = $6,
    motivo_estado = $7
WHERE id = $1
RETURNING *;

-- name: ObtenerOrganizacionPorID :one
SELECT * FROM organizaciones WHERE id = $1;

-- name: ObtenerOrganizacionPorAlias :one
SELECT * FROM organizaciones WHERE alias = $1;

-- name: ObtenerOrganizacionParaActualizar :one
-- RepositorioOrganizaciones.CargarParaActualizar (§1.2, INV-TEN-06): toma
-- el candado de la fila raíz. Solo tiene sentido dentro de una
-- UnidadDeTrabajo (transacción ya abierta).
SELECT * FROM organizaciones WHERE id = $1 FOR UPDATE;

-- --- Membresías ---------------------------------------------------------------

-- name: CrearMembresia :one
INSERT INTO membresias (
    id, organizacion_id, usuario_id, rol, estado, otorgada_por,
    creada_en, actualizada_en, removida_en
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $7, $8
)
RETURNING *;

-- name: ActualizarMembresia :one
UPDATE membresias
SET rol = $2,
    estado = $3,
    actualizada_en = $4,
    removida_en = $5
WHERE id = $1
RETURNING *;

-- name: ObtenerMembresiaPorID :one
SELECT * FROM membresias WHERE id = $1;

-- name: ObtenerMembresiaVigente :one
-- Camino caliente de autorización (§3.7 del diseño): se sirve del índice
-- único parcial membresias_vigente_idx (INV-TEN-09).
SELECT * FROM membresias WHERE organizacion_id = $1 AND usuario_id = $2 AND estado <> 'removida';

-- name: ListarMembresiasDeOrganizacion :many
SELECT * FROM membresias WHERE organizacion_id = $1 ORDER BY creada_en ASC;

-- name: ListarMembresiasDeUsuario :many
SELECT * FROM membresias WHERE usuario_id = $1 AND estado <> 'removida' ORDER BY creada_en ASC;

-- name: ContarPropietariosActivos :one
SELECT COUNT(*) FROM membresias WHERE organizacion_id = $1 AND rol = 'propietario' AND estado = 'activa';

-- name: ContarActivasDeOrganizacion :one
SELECT COUNT(*) FROM membresias WHERE organizacion_id = $1 AND estado = 'activa';

-- name: ContarOrganizacionesPropiasDeUsuario :one
SELECT COUNT(*) FROM membresias WHERE usuario_id = $1 AND rol = 'propietario' AND estado = 'activa';

-- --- Invitaciones ---------------------------------------------------------------

-- name: CrearInvitacion :one
INSERT INTO invitaciones (
    id, organizacion_id, correo_destinatario, rol_propuesto, estado,
    hash_token, invitada_por, creada_en, expira_en, resuelta_en
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
)
RETURNING *;

-- name: ActualizarInvitacion :one
UPDATE invitaciones
SET estado = $2,
    resuelta_en = $3
WHERE id = $1
RETURNING *;

-- name: ObtenerInvitacionPorID :one
SELECT * FROM invitaciones WHERE id = $1;

-- name: ObtenerInvitacionPorHash :one
-- Única vía de escape de RLS del contexto (§6.3 del diseño, ADR candidato
-- 0031): función SECURITY DEFINER, no una consulta directa a la tabla. Los
-- casts explícitos son necesarios para que sqlc infiera el tipo Go de cada
-- columna: sin ellos, una función que devuelve TABLE(...) queda tipada
-- como `interface{}` columna por columna.
SELECT id::uuid AS id,
       organizacion_id::uuid AS organizacion_id,
       correo_destinatario::text AS correo_destinatario,
       rol_propuesto::text AS rol_propuesto,
       estado::text AS estado,
       hash_token::text AS hash_token,
       invitada_por::uuid AS invitada_por,
       creada_en::timestamptz AS creada_en,
       expira_en::timestamptz AS expira_en,
       resuelta_en::timestamptz AS resuelta_en
FROM tenencia_resolver_invitacion($1)
    AS t(id, organizacion_id, correo_destinatario, rol_propuesto, estado,
         hash_token, invitada_por, creada_en, expira_en, resuelta_en);

-- name: ObtenerInvitacionPendiente :one
SELECT * FROM invitaciones WHERE organizacion_id = $1 AND correo_destinatario = $2 AND estado = 'pendiente';

-- name: ListarInvitacionesPendientesDeOrganizacion :many
SELECT * FROM invitaciones WHERE organizacion_id = $1 AND estado = 'pendiente' ORDER BY creada_en DESC;

-- name: ContarInvitacionesPendientesDeOrganizacion :one
SELECT COUNT(*) FROM invitaciones WHERE organizacion_id = $1 AND estado = 'pendiente';
