-- Contexto Confianza — agregado SalaDeEspera (§6.1 de
-- docs/design/colas-virtuales.md). Primer agregado con estado y primera
-- tabla en Postgres del contexto Confianza.
--
-- Notas de diseño (no reinventar sin volver a leer el documento):
--   * `id` SIN `DEFAULT gen_random_uuid()`: UUIDv7 generado por el puerto
--     GeneradorIDs, mismo criterio que 000009/000013/000015.
--   * Esta tabla NO se lee en el camino caliente (INV-COLA-08). La leen solo
--     los endpoints de administración y el reconciliador (§3.7), una vez cada
--     15 s por réplica.
--   * `cursor_base`/`reloj_desde` son el reloj de admisión (§1.5). Se
--     reescriben en cada cambio de ritmo para que el cursor sea continuo.
--   * Sin RLS: `alcance_organizacion_id` es una clave opaca para Confianza,
--     que NO es un contexto multi-tenant sobre esta tabla — la autorización
--     de los endpoints org-scoped la resuelve Tenencia por puerto (§7.2),
--     y las políticas de 000014 aplican a las tablas de Tenencia, no a ésta.
--     Ver ADR candidato 0045.
--   * Sin DELETE en los privilegios: una sala cerrada es evidencia de un
--     evento operativo ("¿con qué ritmo se abrió el lunes?"), se conserva con
--     estado terminal — mismo criterio que invitaciones y sesiones.
--   * Los catálogos cerrados de esta tabla (alcance_tipo, ruta_protegida,
--     estado, modo_degradado) se verificaron carácter por carácter contra
--     los VOs de dominio ya commiteados en internal/confianza/dominio/
--     (TipoAlcanceSistema/TipoAlcanceOrganizacion en alcance_sala.go,
--     RutaAccesoIniciarSesion/RutaIdentidadRegistrarUsuario/
--     RutaTenenciaAceptarInvitacion en alcance_sala.go,
--     EstadoSalaProgramada/Abierta/Drenando/Cerrada en estados_sala.go,
--     ModoDegradadoPermitir/Rechazar en politica_sala.go) — coinciden
--     exactamente, así como los rangos de ritmo/capacidad/ventana con las
--     constantes de politica_sala.go.

CREATE TABLE salas_espera (
    id                      UUID        PRIMARY KEY,  -- UUIDv7 generado por la app
    alias                   TEXT        NOT NULL,     -- normalizado por el VO AliasSala
    alcance_tipo            TEXT        NOT NULL,
    alcance_organizacion_id UUID        REFERENCES organizaciones(id),
    ruta_protegida          TEXT        NOT NULL,
    estado                  TEXT        NOT NULL,
    ritmo_admision          INTEGER     NOT NULL,
    capacidad_maxima_cola   BIGINT      NOT NULL,
    ventana_reclamo_ms      INTEGER     NOT NULL,
    modo_degradado          TEXT        NOT NULL,
    cursor_base             BIGINT      NOT NULL DEFAULT 0,
    reloj_desde             TIMESTAMPTZ NOT NULL,
    version_config          BIGINT      NOT NULL DEFAULT 1,
    creada_por              UUID        REFERENCES usuarios(id),  -- NULL para alcance sistema (§7.2)
    creada_en               TIMESTAMPTZ NOT NULL,
    actualizada_en          TIMESTAMPTZ NOT NULL,
    abierta_en              TIMESTAMPTZ,
    cerrada_en              TIMESTAMPTZ,

    CONSTRAINT salas_espera_alcance_valido CHECK (alcance_tipo IN ('sistema','organizacion')),
    -- El alcance y su organización son coherentes o la fila no existe.
    CONSTRAINT salas_espera_alcance_coherente CHECK (
        (alcance_tipo = 'sistema'      AND alcance_organizacion_id IS NULL) OR
        (alcance_tipo = 'organizacion' AND alcance_organizacion_id IS NOT NULL)
    ),
    CONSTRAINT salas_espera_ruta_valida CHECK (
        ruta_protegida IN ('acceso.iniciar_sesion','identidad.registrar_usuario','tenencia.aceptar_invitacion')
    ),
    CONSTRAINT salas_espera_estado_valido CHECK (estado IN ('programada','abierta','drenando','cerrada')),
    CONSTRAINT salas_espera_modo_degradado_valido CHECK (modo_degradado IN ('permitir','rechazar')),
    CONSTRAINT salas_espera_ritmo_rango      CHECK (ritmo_admision BETWEEN 1 AND 10000),
    CONSTRAINT salas_espera_capacidad_rango  CHECK (capacidad_maxima_cola BETWEEN 100 AND 5000000),
    CONSTRAINT salas_espera_ventana_rango    CHECK (ventana_reclamo_ms BETWEEN 30000 AND 900000),
    CONSTRAINT salas_espera_cierre_coherente CHECK ((estado = 'cerrada') = (cerrada_en IS NOT NULL))
);

COMMENT ON TABLE salas_espera IS 'Configuracion operativa de las colas de acceso virtual (contexto Confianza). El estado efimero de la cola vive en Redis; esta tabla es la fuente de verdad duradera y auditable (INV-COLA-12).';

-- Alias público y único: es lo que aparece en la URL de ingreso.
CREATE UNIQUE INDEX salas_espera_alias_idx ON salas_espera (alias);

-- INV-COLA-01: a lo sumo una sala NO CERRADA por (alcance, ruta). Parcial, no
-- total: reabrir una sala para el mismo evento el mes que viene crea una fila
-- nueva y la anterior (cerrada) queda como evidencia.
CREATE UNIQUE INDEX salas_espera_vigente_idx
    ON salas_espera (alcance_tipo, COALESCE(alcance_organizacion_id, '00000000-0000-0000-0000-000000000000'::uuid), ruta_protegida)
    WHERE estado <> 'cerrada';

-- Consulta del reconciliador (§3.7), una cada 15 s por réplica.
CREATE INDEX salas_espera_vigentes_idx ON salas_espera (estado) WHERE estado IN ('abierta','drenando');

-- -----------------------------------------------------------------------------
-- Privilegios (ADR 0017: una tabla nueva no alcanza con crearla).
-- -----------------------------------------------------------------------------
REVOKE ALL ON salas_espera FROM PUBLIC;
GRANT SELECT, INSERT, UPDATE ON salas_espera TO rol_aplicacion;
REVOKE DELETE, TRUNCATE ON salas_espera FROM rol_aplicacion;
