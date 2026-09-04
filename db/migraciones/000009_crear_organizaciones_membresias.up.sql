-- Contexto Tenencia — agregados Organizacion y Membresia (sección 6.1 de
-- docs/design/tenencia-bounded-context.md).
--
-- Notas de diseño (no reinventar sin volver a leer el documento):
--   * `id` de ambas tablas SIN `DEFAULT gen_random_uuid()`, mismo criterio
--     que `sesiones` (000006): son UUIDv7 generados por el puerto
--     GeneradorIDs de la aplicación, y un default de v4 en la base
--     enmascararía en silencio un bug que produjera IDs del tipo equivocado.
--   * FK de `membresias.usuario_id`, `membresias.otorgada_por` y
--     `organizaciones.creada_por` hacia `usuarios(id)` SOLO por integridad
--     referencial (§2.4 del diseño): el adaptador Postgres de Tenencia nunca
--     lee columnas de `usuarios`, es frontera de agregado, no de contexto.
--   * `organizaciones_alias_idx` (INV-TEN-02) y `membresias_vigente_idx`
--     (INV-TEN-09) son garantías ESTRUCTURALES, no comprobaciones de la
--     aplicación — mismo patrón que `tokens_refresco_vigente_por_sesion_idx`
--     (INV-ACC-04, 000006).
--   * `membresias_vigente_idx` es PARCIAL (`WHERE estado <> 'removida'`) y no
--     total a propósito: readmitir a alguien crea una fila nueva (INV-TEN-08,
--     una membresía nunca se borra físicamente) y un índice total lo
--     impediría.
--   * SIN RLS todavía: es `000011_rls_tenencia`, deliberadamente después de
--     que existan casos de uso reales que la ejerzan (§10 del diseño).
--   * El `CONSTRAINT TRIGGER ... DEFERRABLE INITIALLY DEFERRED` que sigue es
--     el segundo mecanismo de INV-TEN-06 ("una organización activa nunca se
--     queda sin propietario activo"); el primero (candado de fila
--     `SELECT...FOR UPDATE`) es responsabilidad de la aplicación futura, no
--     de esta migración. Diferido por el mismo motivo que la FK
--     `tokens_refresco_hash_sucesor_fkey` de 000008: `CrearOrganizacion`
--     inserta la organización antes que la membresía fundacional, y
--     `TransferirPropiedad` pasa por estados intermedios legítimos dentro de
--     la misma transacción — verificar por sentencia (el comportamiento por
--     defecto) rompería ambos flujos.

CREATE TABLE organizaciones (
    id              UUID        PRIMARY KEY,  -- UUIDv7 generado por la app
    alias           TEXT        NOT NULL,
    nombre          TEXT        NOT NULL,
    estado          TEXT        NOT NULL,
    creada_por      UUID        NOT NULL REFERENCES usuarios(id),
    creada_en       TIMESTAMPTZ NOT NULL,
    actualizada_en  TIMESTAMPTZ NOT NULL,
    archivada_en    TIMESTAMPTZ,
    motivo_estado   TEXT,

    CONSTRAINT organizaciones_estado_valido CHECK (estado IN ('activa','suspendida','archivada')),
    CONSTRAINT organizaciones_alias_formato  CHECK (alias ~ '^[a-z0-9]([a-z0-9-]{1,46}[a-z0-9])$'),
    CONSTRAINT organizaciones_nombre_no_vacio CHECK (length(btrim(nombre)) > 0),
    CONSTRAINT organizaciones_archivado_coherente CHECK ((estado = 'archivada') = (archivada_en IS NOT NULL)),
    CONSTRAINT organizaciones_motivo_presente CHECK (estado = 'activa' OR motivo_estado IS NOT NULL)
);

COMMENT ON TABLE organizaciones IS 'Agregado Organizacion (contexto Tenencia). id es UUIDv7 generado por la aplicacion.';

-- INV-TEN-02 como garantía ESTRUCTURAL: el alias es único.
CREATE UNIQUE INDEX organizaciones_alias_idx ON organizaciones (alias);

CREATE TABLE membresias (
    id              UUID        PRIMARY KEY,  -- UUIDv7 generado por la app
    organizacion_id UUID        NOT NULL REFERENCES organizaciones(id),
    usuario_id      UUID        NOT NULL REFERENCES usuarios(id),
    rol             TEXT        NOT NULL,
    estado          TEXT        NOT NULL,
    otorgada_por    UUID        REFERENCES usuarios(id),  -- NULL en la membresía fundacional
    creada_en       TIMESTAMPTZ NOT NULL,
    actualizada_en  TIMESTAMPTZ NOT NULL,
    removida_en     TIMESTAMPTZ,

    CONSTRAINT membresias_rol_valido    CHECK (rol IN ('propietario','administrador','miembro')),
    CONSTRAINT membresias_estado_valido CHECK (estado IN ('activa','suspendida','removida')),
    CONSTRAINT membresias_remocion_coherente CHECK ((estado = 'removida') = (removida_en IS NOT NULL))
);

COMMENT ON TABLE membresias IS 'Agregado Membresia (contexto Tenencia). Nunca se borra fisicamente (INV-TEN-08): transiciona a estado removida.';

-- INV-TEN-09: a lo sumo una membresía NO removida por (organizacion, usuario).
-- Parcial y no total, a propósito: readmitir a alguien crea una fila nueva
-- (INV-TEN-08) y un índice total lo impediría. Mismo patrón que
-- tokens_refresco_vigente_por_sesion_idx (INV-ACC-04).
CREATE UNIQUE INDEX membresias_vigente_idx
    ON membresias (organizacion_id, usuario_id) WHERE estado <> 'removida';

-- Camino caliente de autorización (§3.7 del diseño): lectura por (usuario, organizacion).
CREATE INDEX membresias_usuario_idx      ON membresias (usuario_id, organizacion_id) WHERE estado = 'activa';
-- Conteo de propietarios (INV-TEN-06) y listado de miembros.
CREATE INDEX membresias_organizacion_idx ON membresias (organizacion_id, rol)        WHERE estado = 'activa';

-- -----------------------------------------------------------------------------
-- INV-TEN-06 como garantía ESTRUCTURAL: ninguna organización activa se queda
-- sin al menos un propietario activo. Dos triggers cubren los dos lados del
-- invariante: cambios en `membresias` (remover/degradar al último
-- propietario) y cambios en `organizaciones` (reactivar una organización
-- cuyos propietarios fueron suspendidos/removidos mientras estaba
-- suspendida). Ambos DEFERRABLE INITIALLY DEFERRED — ver nota de cabecera.
-- -----------------------------------------------------------------------------
CREATE FUNCTION tenencia_verificar_propietario() RETURNS TRIGGER
LANGUAGE plpgsql AS $$
DECLARE
    v_organizacion UUID := COALESCE(NEW.organizacion_id, OLD.organizacion_id);
    v_estado_org   TEXT;
    v_propietarios INTEGER;
BEGIN
    SELECT estado INTO v_estado_org FROM organizaciones WHERE id = v_organizacion;
    IF v_estado_org IS DISTINCT FROM 'activa' THEN
        RETURN NULL;  -- una organización suspendida o archivada puede quedarse sin propietario
    END IF;

    SELECT count(*) INTO v_propietarios
    FROM membresias
    WHERE organizacion_id = v_organizacion AND rol = 'propietario' AND estado = 'activa';

    IF v_propietarios = 0 THEN
        RAISE EXCEPTION 'la organizacion % quedaria sin propietario activo', v_organizacion
            USING ERRCODE = '23514',
                  CONSTRAINT = 'membresias_al_menos_un_propietario';
    END IF;
    RETURN NULL;
END $$;

COMMENT ON FUNCTION tenencia_verificar_propietario() IS 'INV-TEN-06: dispara sobre cambios en membresias; verifica que la organizacion afectada, si sigue activa, conserve al menos un propietario activo.';

-- DIFERIDO a propósito: CrearOrganizacion inserta la organización antes que la
-- membresía, y TransferirPropiedad pasa por estados intermedios legítimos
-- dentro de la transacción. Verificar por sentencia rompería ambos.
CREATE CONSTRAINT TRIGGER membresias_al_menos_un_propietario
    AFTER INSERT OR UPDATE OR DELETE ON membresias
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION tenencia_verificar_propietario();

-- El mismo invariante por el otro lado: reactivar una organización cuyos
-- propietarios fueron suspendidos/removidos mientras estaba suspendida.
-- Función propia (no `tenencia_verificar_propietario`): aquí el disparador es
-- `organizaciones`, no `membresias`, así que el id de la organización sale de
-- NEW.id, no de NEW.organizacion_id/OLD.organizacion_id.
CREATE FUNCTION tenencia_verificar_propietario_de_organizacion() RETURNS TRIGGER
LANGUAGE plpgsql AS $$
DECLARE
    v_propietarios INTEGER;
BEGIN
    SELECT count(*) INTO v_propietarios
    FROM membresias
    WHERE organizacion_id = NEW.id AND rol = 'propietario' AND estado = 'activa';

    IF v_propietarios = 0 THEN
        RAISE EXCEPTION 'la organizacion % no puede reactivarse sin propietario activo', NEW.id
            USING ERRCODE = '23514',
                  CONSTRAINT = 'membresias_al_menos_un_propietario';
    END IF;
    RETURN NULL;
END $$;

COMMENT ON FUNCTION tenencia_verificar_propietario_de_organizacion() IS 'INV-TEN-06: dispara sobre UPDATE de organizaciones.estado hacia activa; verifica que ya exista al menos un propietario activo antes de permitir la reactivacion.';

CREATE CONSTRAINT TRIGGER organizaciones_al_menos_un_propietario
    AFTER UPDATE OF estado ON organizaciones
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW WHEN (NEW.estado = 'activa')
    EXECUTE FUNCTION tenencia_verificar_propietario_de_organizacion();

-- El adaptador Postgres traduce ERRCODE 23514 con
-- CONSTRAINT = 'membresias_al_menos_un_propietario' a ErrUltimoPropietario,
-- igual que hoy traduce la violación de unicidad a ErrCorreoYaRegistrado.

-- -----------------------------------------------------------------------------
-- Privilegios (ADR 0017: una tabla nueva no alcanza con crearla).
-- -----------------------------------------------------------------------------
REVOKE ALL ON organizaciones, membresias FROM PUBLIC;
GRANT SELECT, INSERT, UPDATE ON organizaciones TO rol_aplicacion;
GRANT SELECT, INSERT, UPDATE ON membresias     TO rol_aplicacion;
-- Sin DELETE, por el mismo criterio que `usuarios` y `sesiones`: una membresía
-- no se borra, transiciona a 'removida' (INV-TEN-08), y una organización no se
-- borra, se archiva. La purga por retención, si alguna vez existe, es un job de
-- mantenimiento con su propio rol, no una operación de la API.
REVOKE DELETE, TRUNCATE ON organizaciones, membresias FROM rol_aplicacion;
