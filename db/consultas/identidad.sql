-- Queries sqlc del contexto Identidad. Implementan
-- identidad/puertos.RepositorioUsuarios (ver
-- docs/design/identidad-bounded-context.md, sección 2.2).
--
-- Regla de frontera (INV-ID-20): solo el adaptador
-- identidad/adaptadores/postgres consume este código generado. Ningún otro
-- contexto importa internal/identidad/adaptadores/postgres/sqlc.

-- name: CrearUsuario :one
INSERT INTO usuarios (
    id,
    correo,
    contrasena_hash,
    estado,
    tiene_mfa,
    creado_en,
    actualizado_en
) VALUES (
    $1, $2, $3, $4, $5, $6, $6
)
RETURNING *;

-- name: ObtenerUsuarioPorID :one
SELECT *
FROM usuarios
WHERE id = $1;

-- name: ObtenerUsuarioPorCorreo :one
SELECT *
FROM usuarios
WHERE correo = $1;

-- name: ActualizarUsuario :one
-- Persiste el estado completo del agregado tras una mutación de negocio
-- (cambio de contraseña, transición de estado, flip de tiene_mfa,
-- actualizado_en). ultimo_acceso_en se actualiza aparte con
-- RegistrarAccesoUsuario, ya que RegistrarAcceso no siempre corre en la
-- misma llamada que las demás mutaciones.
UPDATE usuarios
SET
    contrasena_hash = $2,
    estado = $3,
    tiene_mfa = $4,
    actualizado_en = $5
WHERE id = $1
RETURNING *;

-- name: RegistrarAccesoUsuario :one
-- Usuario.RegistrarAcceso(ahora): marca el último inicio de sesión exitoso.
UPDATE usuarios
SET
    ultimo_acceso_en = $2,
    actualizado_en = $2
WHERE id = $1
RETURNING *;

-- Queries de tokens_verificacion_correo. Implementan
-- identidad/puertos.RepositorioTokensVerificacion (sección 3.4 del diseño).

-- name: UpsertTokenVerificacionCorreo :exec
-- usuario_id es UNIQUE (000004): un reenvío invalida el token anterior sin
-- un paso de borrado previo (RepositorioTokensVerificacion.Guardar debe ser
-- un upsert real, no dos queries separados).
INSERT INTO tokens_verificacion_correo (
    hash_token, usuario_id, expira_en
) VALUES (
    $1, $2, $3
)
ON CONFLICT (usuario_id) DO UPDATE
SET hash_token = EXCLUDED.hash_token,
    expira_en = EXCLUDED.expira_en;

-- name: ObtenerTokenVerificacionCorreoPorHash :one
SELECT *
FROM tokens_verificacion_correo
WHERE hash_token = $1;

-- name: EliminarTokenVerificacionCorreoPorUsuario :exec
DELETE FROM tokens_verificacion_correo
WHERE usuario_id = $1;
