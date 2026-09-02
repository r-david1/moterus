-- Contexto Acceso — seis acciones nuevas en el catálogo cerrado
-- `auditoria_acciones` (secciones 1.6 y 6 de
-- docs/design/acceso-bounded-context.md). Formato exigido por
-- `auditoria_acciones_formato` (^[a-z_]+\.[a-z_]+$) — las seis cumplen.
--
-- Ver docs/catalogos/acciones-auditoria.md, sección "Contexto Acceso", para
-- la versión legible de esta misma tabla y el procedimiento seguido.

INSERT INTO auditoria_acciones (accion, contexto, recurso, descripcion) VALUES
    ('sesion.iniciada',                 'acceso', 'sesion',       'Emision de una sesion nueva tras autenticacion exitosa en Identidad.'),
    ('sesion.renovada',                 'acceso', 'sesion',       'Rotacion del token de refresco: exito, o fallo por refresco invalido/expirado/sesion no renovable.'),
    ('sesion.reuso_refresco_detectado', 'acceso', 'sesion',       'Se presento un token de refresco ya consumido: robo probable; revoca la sesion completa.'),
    ('sesion.cerrada',                  'acceso', 'sesion',       'Cierre de sesion iniciado por el propio usuario (individual o de todos los dispositivos).'),
    ('sesion.revocada',                 'acceso', 'sesion',       'Revocacion NO iniciada por el usuario: cuenta no operativa, cambio de contrasena, limite de sesiones o decision administrativa.'),
    ('token_acceso.rechazado',          'acceso', 'token_acceso', 'Rechazo de un token de acceso con valor de senal: firma invalida, kid/alg/typ inesperado o sesion revocada. NO cubre el token simplemente expirado.');
