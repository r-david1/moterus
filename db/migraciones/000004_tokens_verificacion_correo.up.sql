-- Contexto Identidad — mecanismo de verificación de correo (sección 3.4 del
-- diseño, docs/design/identidad-bounded-context.md).
--
-- El token de verificación NO se modela como campo del agregado Usuario
-- (evita ensuciar el aggregate con un concepto de vida corta y ciclo
-- propio): vive en su propia tabla/repositorio. Solo se guarda el hash
-- SHA-256 del token, nunca el valor plano (INV-ID-21) — mismo criterio que
-- las contraseñas, aunque el algoritmo es distinto (SHA-256, no Argon2id:
-- ver justificación en internal/identidad/aplicacion/tokens_verificacion.go).
--
-- usuario_id es UNIQUE a propósito: un usuario tiene como máximo un token
-- activo. Esto permite que RepositorioTokensVerificacion.Guardar sea un
-- upsert real (INSERT ... ON CONFLICT (usuario_id) DO UPDATE) — un reenvío
-- invalida el token anterior sin un paso de borrado previo.

CREATE TABLE tokens_verificacion_correo (
    hash_token TEXT PRIMARY KEY,
    usuario_id UUID NOT NULL UNIQUE REFERENCES usuarios(id),
    expira_en TIMESTAMPTZ NOT NULL,
    creado_en TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE tokens_verificacion_correo IS 'Tokens de un solo uso para verificación de correo (sección 3.4 del diseño). Solo se persiste el hash SHA-256 del token, nunca el valor plano (INV-ID-21). Vigencia 24h, constante en aplicacion/tokens_verificacion.go.';
COMMENT ON COLUMN tokens_verificacion_correo.hash_token IS 'Hash SHA-256 (hex) del token en claro. Nunca se persiste el token plano.';
COMMENT ON COLUMN tokens_verificacion_correo.usuario_id IS 'UNIQUE: un usuario tiene como máximo un token activo. Guardar es upsert por esta columna (ON CONFLICT DO UPDATE), no dos pasos separados.';
COMMENT ON COLUMN tokens_verificacion_correo.expira_en IS 'Vigencia de 24 horas desde la emisión (sección 3.4 del diseño).';

-- Privilegios de rol_aplicacion (mismo patrón que 000003_rol_login_aplicacion):
-- a diferencia de usuarios, aquí SÍ hace falta DELETE porque los tokens se
-- consumen (verificación exitosa) o se sobreescriben (reenvío, vía upsert).
GRANT SELECT, INSERT, UPDATE, DELETE ON tokens_verificacion_correo TO rol_aplicacion;
