-- Contexto Identidad — agregado Usuario (ver docs/design/identidad-bounded-context.md).
-- Un solo producto (ADR 0002): sin tabla de productos ni columna de producto.
-- Convención de nombres: ADR 0004 (espanol, snake_case, plural, sin tildes/ñ, sin prefijo).

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE usuarios (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    correo TEXT NOT NULL,
    contrasena_hash TEXT NOT NULL,
    estado TEXT NOT NULL DEFAULT 'pendiente_verificacion',
    tiene_mfa BOOLEAN NOT NULL DEFAULT false,
    creado_en TIMESTAMPTZ NOT NULL DEFAULT now(),
    actualizado_en TIMESTAMPTZ NOT NULL DEFAULT now(),
    ultimo_acceso_en TIMESTAMPTZ,

    CONSTRAINT usuarios_estado_valido CHECK (
        estado IN (
            'pendiente_verificacion',
            'activo',
            'suspendido',
            'bloqueado',
            'anonimizado'
        )
    ),
    -- Defensa en profundidad de INV-ID-02: la app (VO Correo) ya normaliza a
    -- minusculas antes de persistir; esta constraint evita que una fila entre
    -- sin pasar por ese normalizador (ej. un INSERT manual o un bug futuro).
    CONSTRAINT usuarios_correo_normalizado CHECK (correo = lower(correo))
);

-- INV-ID-02: unicidad de correo por su forma normalizada, garantizada por
-- indice unico en base de datos, no por consulta previa en la aplicacion.
CREATE UNIQUE INDEX usuarios_correo_idx ON usuarios (correo);

COMMENT ON TABLE usuarios IS 'Agregado Usuario del contexto Identidad. Único producto (ADR 0002): tabla global, sin organizacion_id.';
COMMENT ON COLUMN usuarios.correo IS 'Correo normalizado (minúsculas, NFC) por el VO Correo antes de persistir.';
COMMENT ON COLUMN usuarios.contrasena_hash IS 'Hash en formato PHC (Argon2id, ver ADR 0008). Nunca se registra en logs.';
COMMENT ON COLUMN usuarios.estado IS 'EstadoUsuario: pendiente_verificacion | activo | suspendido | bloqueado | anonimizado.';
COMMENT ON COLUMN usuarios.ultimo_acceso_en IS 'Nulo hasta el primer login exitoso.';
