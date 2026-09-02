-- Contexto Acceso — agregado Sesion y su cadena de rotación de tokens de
-- refresco (sección 6 de docs/design/acceso-bounded-context.md, ADR 0019).
--
-- Notas de diseño (no reinventar sin volver a leer el documento):
--   * SIN RLS: no hay organizacion_id que aislar hasta que exista Tenencia
--     (mismo criterio que la nota final de 000002_crear_auditoria.up.sql).
--   * `sesiones.id` SIN `DEFAULT gen_random_uuid()`, a diferencia de
--     `usuarios`: los IDSesion son UUIDv7 generados por el puerto
--     GeneradorIDs de la aplicación, y un default de v4 en la base
--     enmascararía en silencio un bug que produjera IDs del tipo equivocado.
--   * La FK autorreferencial `hash_sucesor` documenta la cadena de rotación
--     y permite reconstruir el linaje completo de una sesión comprometida en
--     una sola consulta recursiva.
--   * Rechazo explícito de `ON DELETE CASCADE` desde `usuarios`: los usuarios
--     no se borran (se anonimizan), y una cascada silenciosa destruiría
--     evidencia forense de la sesión.
--   * El índice único parcial `tokens_refresco_vigente_por_sesion_idx` hace
--     de INV-ACC-04 ("a lo sumo un token vigente por sesión") una garantía
--     ESTRUCTURAL, no una comprobación en la aplicación: bajo dos
--     renovaciones concurrentes con el mismo refresco, una de las dos
--     transacciones falla con violación de unicidad y el adaptador la
--     traduce a ErrConcurrenciaSesion.

CREATE TABLE sesiones (
    id                     UUID PRIMARY KEY,           -- UUIDv7 generado por la app
    usuario_id             UUID NOT NULL REFERENCES usuarios(id),
    estado                 TEXT NOT NULL,
    generacion             INTEGER NOT NULL DEFAULT 0,
    creada_en              TIMESTAMPTZ NOT NULL,
    actualizada_en         TIMESTAMPTZ NOT NULL,
    ultima_renovacion_en   TIMESTAMPTZ,
    expira_inactividad_en  TIMESTAMPTZ NOT NULL,
    expira_absoluto_en     TIMESTAMPTZ NOT NULL,
    revocada_en            TIMESTAMPTZ,
    motivo_revocacion      TEXT,
    ip_origen              INET,
    agente_usuario         TEXT,
    huella_dispositivo     TEXT,

    CONSTRAINT sesiones_estado_valido CHECK (estado IN ('activa','revocada','expirada')),
    CONSTRAINT sesiones_revocacion_coherente CHECK ((estado = 'revocada') = (revocada_en IS NOT NULL)),
    CONSTRAINT sesiones_revocacion_motivada CHECK (estado <> 'revocada' OR motivo_revocacion IS NOT NULL),
    CONSTRAINT sesiones_motivo_valido CHECK (motivo_revocacion IS NULL OR motivo_revocacion IN (
        'cierre_usuario','cierre_masivo_usuario','reuso_refresco_detectado',
        'cuenta_no_operativa','contrasena_cambiada','limite_sesiones_excedido',
        'revocacion_administrativa')),
    CONSTRAINT sesiones_ventanas_coherentes CHECK (expira_inactividad_en <= expira_absoluto_en),
    CONSTRAINT sesiones_generacion_no_negativa CHECK (generacion >= 0)
);

CREATE INDEX sesiones_usuario_activas_idx ON sesiones (usuario_id, creada_en DESC) WHERE estado = 'activa';
CREATE INDEX sesiones_expiracion_idx      ON sesiones (expira_absoluto_en)         WHERE estado = 'activa';

CREATE TABLE tokens_refresco (
    hash_token   TEXT PRIMARY KEY,
    sesion_id    UUID NOT NULL REFERENCES sesiones(id),
    generacion   INTEGER NOT NULL,
    emitido_en   TIMESTAMPTZ NOT NULL,
    expira_en    TIMESTAMPTZ NOT NULL,
    consumido_en TIMESTAMPTZ,
    hash_sucesor TEXT REFERENCES tokens_refresco(hash_token),

    CONSTRAINT tokens_refresco_hash_formato CHECK (hash_token ~ '^[0-9a-f]{64}$'),
    CONSTRAINT tokens_refresco_sucesor_coherente CHECK (hash_sucesor IS NULL OR consumido_en IS NOT NULL)
);

-- INV-ACC-04 como garantía ESTRUCTURAL, no como comprobación en la aplicación:
-- a lo sumo un token vigente (no consumido) por sesión. Bajo dos renovaciones
-- concurrentes con el mismo refresco, una de las dos transacciones falla con
-- violación de unicidad y el adaptador la traduce a ErrConcurrenciaSesion.
CREATE UNIQUE INDEX tokens_refresco_vigente_por_sesion_idx
    ON tokens_refresco (sesion_id) WHERE consumido_en IS NULL;

CREATE INDEX tokens_refresco_sesion_idx ON tokens_refresco (sesion_id, generacion DESC);

-- ADR 0017: una tabla nueva NO alcanza con crearla — rol_aplicacion necesita
-- GRANT explícito o el proceso api (que corre como rol_login_identidad, sin
-- privilegios heredados sobre tablas nuevas) no puede tocarla.
REVOKE ALL ON sesiones        FROM PUBLIC;
REVOKE ALL ON tokens_refresco FROM PUBLIC;
GRANT SELECT, INSERT, UPDATE ON sesiones        TO rol_aplicacion;
GRANT SELECT, INSERT, UPDATE ON tokens_refresco TO rol_aplicacion;
-- Sin DELETE, a propósito y por el mismo criterio que `usuarios` (y a
-- diferencia de tokens_verificacion_correo, que sí lo necesita): una sesión no
-- se borra, transiciona de estado, y su cadena de tokens consumidos es la
-- evidencia que hace posible la detección de reuso. La purga por retención es
-- un job de mantenimiento con su propio rol, no una operación de la API.
REVOKE DELETE, TRUNCATE ON sesiones        FROM rol_aplicacion;
REVOKE DELETE, TRUNCATE ON tokens_refresco FROM rol_aplicacion;
