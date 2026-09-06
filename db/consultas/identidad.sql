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

-- Queries de factores_mfa/codigos_respaldo_mfa. Implementan
-- identidad/puertos.RepositorioFactoresMFA (docs/design/otp-mfa.md §2.2,
-- ADR 0037/0040, migración 000015).

-- name: CrearFactorMFA :one
INSERT INTO factores_mfa (
    id, usuario_id, tipo, secreto_cifrado, confirmado, activo, creado_en, confirmado_en
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
)
RETURNING *;

-- name: ActualizarFactorMFA :one
-- creado_en y usuario_id nunca cambian tras la creación (INV-MFA-01: un
-- FactorMFA no cambia de dueño ni de fecha de alta).
UPDATE factores_mfa
SET
    secreto_cifrado = $2,
    confirmado = $3,
    activo = $4,
    confirmado_en = $5
WHERE id = $1
RETURNING *;

-- name: ObtenerFactorMFAPorID :one
SELECT * FROM factores_mfa WHERE id = $1;

-- name: ObtenerFactoresMFAConfirmadosActivosDeUsuario :many
-- "Confirmados", en este puerto, significa SIEMPRE confirmado=true AND
-- activo=true (ver el comentario de RepositorioFactoresMFA en
-- identidad/puertos/salida.go y la nota de cabecera de la migración
-- 000015): un factor deshabilitado sigue confirmado=true para siempre
-- (hecho histórico) pero deja de contar aquí.
SELECT * FROM factores_mfa
WHERE usuario_id = $1 AND confirmado = true AND activo = true;

-- name: ContarFactoresMFAConfirmadosActivosDeUsuario :one
SELECT count(*) FROM factores_mfa
WHERE usuario_id = $1 AND confirmado = true AND activo = true;

-- name: UpsertCodigoRespaldoMFA :exec
-- codigos_respaldo_mfa tiene DELETE revocado para rol_aplicacion (000015:
-- "un código de respaldo consumido queda marcado con usado_en, no se
-- borra"), así que RepositorioFactoresMFA.Guardar nunca borra e inserta de
-- nuevo la colección: hace upsert por hash_codigo (único en todo el
-- sistema) tanto para la creación inicial (ConfirmarFactorMFA, 10 filas con
-- usado_en NULL) como para marcar un código consumido después
-- (VerificarOTP/DeshabilitarMFA).
INSERT INTO codigos_respaldo_mfa (
    factor_id, hash_codigo, usado_en
) VALUES (
    $1, $2, $3
)
ON CONFLICT (hash_codigo) DO UPDATE
SET usado_en = EXCLUDED.usado_en;

-- name: ObtenerCodigosRespaldoDeFactor :many
SELECT * FROM codigos_respaldo_mfa WHERE factor_id = $1;
