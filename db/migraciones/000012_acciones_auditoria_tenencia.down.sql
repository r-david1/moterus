-- Reversión de 000012_acciones_auditoria_tenencia. `auditoria_acciones` es el
-- catálogo, no la bitácora: borrar estas filas es seguro siempre que no haya
-- filas en `auditoria` que las referencien (la FK auditoria.accion ->
-- auditoria_acciones.accion lo impediría con un error claro si las hubiera).

DELETE FROM auditoria_acciones WHERE accion IN (
    'organizacion.creada',
    'organizacion.actualizada',
    'organizacion.estado_cambiado',
    'membresia.creada',
    'membresia.rol_cambiado',
    'membresia.estado_cambiado',
    'membresia.removida',
    'membresia.invitada',
    'membresia.invitacion_resuelta',
    'autorizacion.denegada'
);
