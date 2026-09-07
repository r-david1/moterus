-- Queries sqlc del contexto Confianza. Implementan
-- confianza/puertos.RepositorioSalasDeEspera (ver
-- docs/design/colas-virtuales.md, sección 2.2/6.1, y la migración
-- 000017_crear_salas_espera).
--
-- Regla de frontera (INV-COLA, mismo criterio que INV-ID-20/INV-ACC-20/
-- INV-TEN-29): solo el adaptador confianza/adaptadores/postgres consume
-- este código generado. `creada_por` y `alcance_organizacion_id` son
-- integridad referencial hacia usuarios/organizaciones, no permiso de
-- lectura de esas tablas: ninguna consulta de este archivo hace JOIN contra
-- `usuarios` ni `organizaciones`.
--
-- No hay RLS sobre `salas_espera` (§6.1 del diseño: Confianza no es
-- multi-tenant sobre esta tabla), así que estas queries no dependen de
-- ningún `SET LOCAL app.*` previo, a diferencia de las de Tenencia.

-- name: GuardarSalaDeEspera :exec
-- Upsert del agregado completo: los campos de identidad (alias, alcance,
-- ruta, creada_por, creada_en) se fijan solo en el INSERT inicial porque
-- ninguna mutación de negocio de SalaDeEspera los cambia después de
-- creada la fila (dominio/sala_espera.go no expone ningún método que los
-- toque); los campos operativos (estado, ritmo, capacidad, ventana,
-- modo_degradado, cursor_base, reloj_desde, abierta_en, cerrada_en) se
-- reescriben en cada Guardar. version_config se incrementa en cada UPDATE
-- como bitácora de cuántas veces cambió la configuración operativa; el
-- agregado de dominio no la lee ni la expone (no es una invariante de
-- negocio, solo trazabilidad de la fila).
INSERT INTO salas_espera (
    id, alias, alcance_tipo, alcance_organizacion_id, ruta_protegida, estado,
    ritmo_admision, capacidad_maxima_cola, ventana_reclamo_ms, modo_degradado,
    cursor_base, reloj_desde, creada_por, creada_en, actualizada_en,
    abierta_en, cerrada_en
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, now(), $15, $16
)
ON CONFLICT (id) DO UPDATE SET
    alias                 = EXCLUDED.alias,
    estado                = EXCLUDED.estado,
    ritmo_admision        = EXCLUDED.ritmo_admision,
    capacidad_maxima_cola = EXCLUDED.capacidad_maxima_cola,
    ventana_reclamo_ms    = EXCLUDED.ventana_reclamo_ms,
    modo_degradado        = EXCLUDED.modo_degradado,
    cursor_base           = EXCLUDED.cursor_base,
    reloj_desde           = EXCLUDED.reloj_desde,
    actualizada_en        = now(),
    version_config        = salas_espera.version_config + 1,
    abierta_en            = EXCLUDED.abierta_en,
    cerrada_en            = EXCLUDED.cerrada_en;

-- name: ObtenerSalaDeEsperaPorID :one
SELECT * FROM salas_espera WHERE id = $1;

-- name: ObtenerSalaDeEsperaPorAlias :one
SELECT * FROM salas_espera WHERE alias = $1;

-- name: ListarSalasDeEsperaVigentes :many
-- Consulta del reconciliador (§3.7 del diseño), una cada 15s por réplica:
-- se apoya en el índice parcial salas_espera_vigentes_idx.
SELECT * FROM salas_espera
WHERE estado IN ('abierta', 'drenando')
ORDER BY creada_en;
