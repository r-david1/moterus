-- Reversión de 000018_acciones_auditoria_confianza. `auditoria_acciones` es
-- el catálogo, no la bitácora: borrar estas filas es seguro siempre que no
-- haya filas en `auditoria` que las referencien (la FK auditoria.accion ->
-- auditoria_acciones.accion lo impediría con un error claro si las hubiera).

DELETE FROM auditoria_acciones WHERE accion IN (
    'sala_espera.abierta',
    'sala_espera.ritmo_cambiado',
    'sala_espera.cerrada'
);
