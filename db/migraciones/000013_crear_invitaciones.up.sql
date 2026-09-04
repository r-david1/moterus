-- Contexto Tenencia — agregado Invitacion (sección 6.2 de
-- docs/design/tenencia-bounded-context.md).
--
-- Notas de diseño (no reinventar sin volver a leer el documento):
--   * `id` SIN `DEFAULT gen_random_uuid()`, mismo criterio que
--     `organizaciones`/`membresias` (000009): UUIDv7 generado por el puerto
--     GeneradorIDs de la aplicación.
--   * `hash_token` es el SHA-256 hex del token de invitación; el valor en
--     claro NUNCA se persiste (INV-TEN-23). Es la clave de búsqueda de
--     `RepositorioInvitaciones.BuscarPorHash`, que atraviesa RLS vía la
--     función SECURITY DEFINER `tenencia_resolver_invitacion` (000014) —
--     quien acepta una invitación todavía no es miembro de ninguna
--     organización.
--   * FK de `invitada_por` hacia `usuarios(id)` SOLO por integridad
--     referencial, mismo criterio que las FKs de 000009: el adaptador
--     Postgres de Tenencia nunca lee columnas de `usuarios`.
--   * `invitaciones_hash_idx` y `invitaciones_pendiente_idx` (INV-TEN-22) son
--     garantías ESTRUCTURALES, no comprobaciones de la aplicación.
--   * `invitaciones_pendiente_idx` es PARCIAL (`WHERE estado = 'pendiente'`):
--     el caso de uso `Invitar` (internal/tenencia/aplicacion/invitar_miembro.go)
--     reinvita revocando primero cualquier invitación pendiente previa para
--     el mismo (organizacion, correo) — UPDATE a 'revocada' seguido de un
--     INSERT nuevo dentro de la misma transacción, en ese orden, así que el
--     índice parcial nunca ve dos filas 'pendiente' simultáneas para la
--     misma pareja. Un índice único total sobre (organizacion_id,
--     correo_destinatario) rompería ese flujo de reinvitación.
--   * SIN RLS todavía: es `000014_rls_tenencia`, deliberadamente después de
--     que existan casos de uso reales que la ejerzan (§10 del diseño).
--   * Sin DELETE en los privilegios: una invitación resuelta es evidencia
--     ("¿quién invitó a esta persona, cuándo, con qué rol?"), se conserva con
--     `estado` terminal, igual que la cadena de tokens de refresco de Acceso.

CREATE TABLE invitaciones (
    id                  UUID        PRIMARY KEY,  -- UUIDv7 generado por la app
    organizacion_id     UUID        NOT NULL REFERENCES organizaciones(id),
    correo_destinatario TEXT        NOT NULL,     -- normalizado por el VO CorreoDestinatario
    rol_propuesto       TEXT        NOT NULL,
    estado              TEXT        NOT NULL,
    hash_token          TEXT        NOT NULL,     -- SHA-256 hex; el valor en claro NUNCA se persiste
    invitada_por        UUID        NOT NULL REFERENCES usuarios(id),
    creada_en           TIMESTAMPTZ NOT NULL,
    expira_en           TIMESTAMPTZ NOT NULL,
    resuelta_en         TIMESTAMPTZ,

    CONSTRAINT invitaciones_rol_valido    CHECK (rol_propuesto IN ('propietario','administrador','miembro')),
    CONSTRAINT invitaciones_estado_valido CHECK (estado IN ('pendiente','aceptada','revocada','expirada')),
    CONSTRAINT invitaciones_hash_formato  CHECK (hash_token ~ '^[0-9a-f]{64}$'),
    CONSTRAINT invitaciones_ventana_coherente CHECK (expira_en > creada_en),
    CONSTRAINT invitaciones_resolucion_coherente CHECK ((estado = 'pendiente') = (resuelta_en IS NULL))
);

COMMENT ON TABLE invitaciones IS 'Agregado Invitacion (contexto Tenencia). Nunca se borra fisicamente: transiciona a un estado terminal (aceptada/revocada/expirada) que se conserva como evidencia de auditoria.';

-- Clave de búsqueda de RepositorioInvitaciones.BuscarPorHash.
CREATE UNIQUE INDEX invitaciones_hash_idx ON invitaciones (hash_token);

-- INV-TEN-22: a lo sumo una invitación pendiente por (organizacion, correo).
-- Parcial y no total, a propósito: reinvitar revoca la pendiente anterior y
-- crea una fila nueva (ver nota de cabecera) y un índice total lo impediría.
CREATE UNIQUE INDEX invitaciones_pendiente_idx
    ON invitaciones (organizacion_id, correo_destinatario) WHERE estado = 'pendiente';

-- Listado de invitaciones de una organización, más recientes primero.
CREATE INDEX invitaciones_organizacion_idx ON invitaciones (organizacion_id, creada_en DESC);

-- -----------------------------------------------------------------------------
-- Privilegios (ADR 0017: una tabla nueva no alcanza con crearla).
-- -----------------------------------------------------------------------------
REVOKE ALL ON invitaciones FROM PUBLIC;
GRANT SELECT, INSERT, UPDATE ON invitaciones TO rol_aplicacion;
REVOKE DELETE, TRUNCATE ON invitaciones FROM rol_aplicacion;
