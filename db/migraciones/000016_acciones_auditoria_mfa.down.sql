-- Reversión de 000016_acciones_auditoria_mfa. `auditoria_acciones` es el
-- catálogo, no la bitácora: borrar estas filas es seguro siempre que no haya
-- filas en `auditoria` que las referencien (la FK auditoria.accion ->
-- auditoria_acciones.accion lo impediría con un error claro si las hubiera).

DELETE FROM auditoria_acciones WHERE accion IN (
    'usuario.mfa_habilitado',
    'usuario.mfa_confirmado',
    'usuario.mfa_deshabilitado',
    'usuario.codigo_respaldo_consumido',
    'usuario.otp_verificacion_fallida'
);
