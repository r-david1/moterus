-- Reversión de 000002_crear_auditoria.
--
-- ADVERTENCIA OPERATIVA: bajar esta migración DESTRUYE la bitácora forense.
-- Es aceptable en desarrollo y CI; en producción exige respaldo previo y
-- justificación escrita (ADR 0005: "nunca migraciones destructivas sin
-- backup/plan de rollback documentado").
--
-- Los triggers de bloqueo impiden DELETE/TRUNCATE pero NO impiden DROP TABLE:
-- eso es correcto y deliberado. DROP TABLE requiere ser owner —
-- rol_aplicacion no lo es— y deja rastro en el catálogo y en el WAL, mientras
-- que un DELETE selectivo no dejaría ninguno. El control contra el borrado
-- total no es el trigger: es el anclaje externo y la copia WORM.

DROP TRIGGER IF EXISTS trg_auditoria_anclas_bloquear_truncate         ON auditoria_anclas;
DROP TRIGGER IF EXISTS trg_auditoria_anclas_bloquear_mutacion         ON auditoria_anclas;
DROP TRIGGER IF EXISTS trg_auditoria_verificaciones_bloquear_truncate ON auditoria_verificaciones;
DROP TRIGGER IF EXISTS trg_auditoria_verificaciones_bloquear_mutacion ON auditoria_verificaciones;
DROP TRIGGER IF EXISTS trg_auditoria_bloquear_truncate                ON auditoria;
DROP TRIGGER IF EXISTS trg_auditoria_bloquear_mutacion                ON auditoria;
DROP TRIGGER IF EXISTS trg_auditoria_asignar_cadena                   ON auditoria;

DROP FUNCTION IF EXISTS verificar_cadena_auditoria(BIGINT);
DROP FUNCTION IF EXISTS auditoria_asignar_cadena();
DROP FUNCTION IF EXISTS bloquear_mutacion_auditoria();
DROP FUNCTION IF EXISTS auditoria_hash_de_payload(TEXT);
DROP FUNCTION IF EXISTS auditoria_payload_canonico(
    SMALLINT, UUID, BIGINT, TEXT, TIMESTAMPTZ, TIMESTAMPTZ, UUID, UUID,
    TEXT, TEXT, TEXT, TEXT, INET, TEXT, TEXT, JSONB
);
DROP FUNCTION IF EXISTS auditoria_instante_canonico(TIMESTAMPTZ);
DROP FUNCTION IF EXISTS auditoria_campo_canonico(TEXT);

DROP TABLE IF EXISTS auditoria_anclas;
DROP TABLE IF EXISTS auditoria_verificaciones;
DROP TABLE IF EXISTS auditoria;
DROP TABLE IF EXISTS auditoria_acciones;

-- Sólo se eliminan los roles exclusivos de este contexto. `rol_aplicacion` se
-- deja en pie a propósito: puede haber sido creado por otra migración
-- (contexto Identidad/Tenencia) y tener privilegios sobre tablas ajenas a
-- auditoría. Su creación en la migración `up` es idempotente, así que un
-- ciclo up/down/up no falla.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'rol_auditor_lectura') THEN
        EXECUTE 'DROP OWNED BY rol_auditor_lectura';
        DROP ROLE rol_auditor_lectura;
    END IF;
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'rol_mantenimiento_auditoria') THEN
        EXECUTE 'DROP OWNED BY rol_mantenimiento_auditoria';
        DROP ROLE rol_mantenimiento_auditoria;
    END IF;
END
$$;
