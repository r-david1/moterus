-- Reversión de 000007_acciones_auditoria_acceso. `auditoria_acciones` es el
-- catálogo, no la bitácora: borrar estas filas es seguro siempre que no haya
-- filas en `auditoria` que las referencien (la FK auditoria.accion ->
-- auditoria_acciones.accion lo impediría con un error claro si las hubiera).

DELETE FROM auditoria_acciones WHERE accion IN (
    'sesion.iniciada',
    'sesion.renovada',
    'sesion.reuso_refresco_detectado',
    'sesion.cerrada',
    'sesion.revocada',
    'token_acceso.rechazado'
);
