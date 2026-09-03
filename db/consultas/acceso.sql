-- Queries sqlc del contexto Acceso. Implementan
-- acceso/puertos.RepositorioSesiones (ver
-- docs/design/acceso-bounded-context.md, sección 2.2, y la migración
-- 000006_crear_sesiones.up.sql).
--
-- Regla de frontera (INV-ACC-20, mismo criterio que INV-ID-20 de
-- Identidad): solo el adaptador acceso/adaptadores/postgres consume este
-- código generado. Ningún otro contexto importa
-- internal/acceso/adaptadores/postgres/sqlc.
--
-- Nota sobre RevocarSesionesActivasDeUsuario: el parámetro "excepto" NO se
-- vuelve NULL cuando el caso de uso no quiere preservar ninguna sesión
-- (dominio.IDSesion{} vacío) — el adaptador (mapeo.go) lo traduce al UUID
-- nulo "00000000-0000-0000-0000-000000000000", que dominio.IDSesionDesde ya
-- rechaza como identificador válido (esUUIDNulo), así que nunca puede
-- coincidir con el id real de una sesión (los IDSesion son UUIDv7). Evita
-- tener que manejar NULL en el predicado SQL para el caso común.

-- name: CrearSesion :one
INSERT INTO sesiones (
    id, usuario_id, estado, generacion, creada_en, actualizada_en,
    expira_inactividad_en, expira_absoluto_en,
    ip_origen, agente_usuario, huella_dispositivo
) VALUES (
    $1, $2, $3, $4, $5, $5, $6, $7, $8, $9, $10
)
RETURNING *;

-- name: ActualizarSesion :one
-- Persiste el estado completo del agregado tras una mutación de negocio
-- (Rotar, Revocar, MarcarExpirada). No hay un flag "es nuevo" en el
-- dominio (INV-ACC-09: sin campos exportados), así que el adaptador
-- intenta primero este UPDATE; si no afecta ninguna fila (pgx.ErrNoRows en
-- un :one con RETURNING) es porque la sesión todavía no existe y entonces
-- se hace el INSERT (CrearSesion) — mismo patrón que
-- identidad/adaptadores/postgres.RepositorioUsuarios.Guardar.
UPDATE sesiones
SET estado = $2,
    generacion = $3,
    actualizada_en = $4,
    ultima_renovacion_en = $5,
    expira_inactividad_en = $6,
    revocada_en = $7,
    motivo_revocacion = $8
WHERE id = $1
RETURNING *;

-- name: ObtenerSesionPorID :one
SELECT * FROM sesiones WHERE id = $1;

-- name: ContarSesionesActivasDeUsuario :one
SELECT COUNT(*) FROM sesiones WHERE usuario_id = $1 AND estado = 'activa';

-- name: ListarSesionesActivasDeUsuario :many
SELECT * FROM sesiones WHERE usuario_id = $1 AND estado = 'activa' ORDER BY creada_en DESC;

-- name: RevocarSesionesActivasDeUsuario :many
-- Operación de conjunto (RepositorioSesiones.RevocarActivasDeUsuario, §2.2
-- del diseño): revocar N sesiones cargando N agregados sería absurdo.
-- Devuelve los ids revocados para que el caso de uso audite una fila por
-- sesión y los propague a la lista de revocación de Redis.
UPDATE sesiones
SET estado = 'revocada',
    revocada_en = $2,
    motivo_revocacion = $3,
    actualizada_en = $2
WHERE usuario_id = $1 AND estado = 'activa' AND id <> $4
RETURNING id;

-- name: CrearTokenRefresco :exec
-- ON CONFLICT DO NOTHING: RepositorioSesiones.Guardar debe ser idempotente
-- respecto a los tokens ya persistidos y sin cambios (§2.2 del diseño). El
-- índice único parcial tokens_refresco_vigente_por_sesion_idx (migración
-- 000006) es quien realmente hace cumplir INV-ACC-04 bajo concurrencia: una
-- violación de ESE índice (no de esta clave primaria) es la que el
-- adaptador traduce a dominio.ErrConcurrenciaSesion.
INSERT INTO tokens_refresco (hash_token, sesion_id, generacion, emitido_en, expira_en)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (hash_token) DO NOTHING;

-- name: ConsumirTokenRefresco :exec
-- Marca el token recién rotado (Sesion.RefrescoRecienConsumido) como
-- consumido, con su sucesor. Debe ejecutarse ANTES de CrearTokenRefresco en
-- la misma rotación: hasta que este UPDATE quita al token anterior de la
-- vigencia (consumido_en pasa de NULL a NOT NULL), el índice único parcial
-- seguiría contándolo como "vigente" y el INSERT del nuevo token violaría
-- la unicidad incluso sin ninguna concurrencia real.
UPDATE tokens_refresco
SET consumido_en = $2, hash_sucesor = $3
WHERE hash_token = $1;

-- name: ObtenerTokenRefrescoPorHash :one
SELECT * FROM tokens_refresco WHERE hash_token = $1;
