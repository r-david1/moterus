-- Contexto Tenencia — diez acciones nuevas en el catálogo cerrado
-- `auditoria_acciones` (secciones 1.6 y 6.4 de
-- docs/design/tenencia-bounded-context.md). Formato exigido por
-- `auditoria_acciones_formato` (^[a-z_]+\.[a-z_]+$) — las diez cumplen.
--
-- Numeración 000012 a propósito, no 000010: las migraciones 000010
-- (invitaciones) y 000011 (RLS de Tenencia) quedan deliberadamente para más
-- adelante (§10 del diseño), pero el catálogo de auditoría no depende de
-- ninguna de las dos y no hay razón para bloquearlo.
--
-- Ver docs/catalogos/acciones-auditoria.md, sección "Contexto Tenencia", para
-- la versión legible de esta misma tabla.

INSERT INTO auditoria_acciones (accion, contexto, recurso, descripcion) VALUES
    ('organizacion.creada',           'tenencia', 'organizacion', 'Alta de una organizacion nueva, junto con la membresia propietario de su fundador.'),
    ('organizacion.actualizada',      'tenencia', 'organizacion', 'Cambio de nombre o alias de una organizacion.'),
    ('organizacion.estado_cambiado',  'tenencia', 'organizacion', 'Transicion de EstadoOrganizacion: suspender, reactivar o archivar.'),
    ('membresia.creada',              'tenencia', 'membresia',    'Alta de una membresia: fundacion de la organizacion, alta directa o aceptacion de invitacion.'),
    ('membresia.rol_cambiado',        'tenencia', 'membresia',    'Cambio de rol de un miembro, incluida la transferencia de propiedad.'),
    ('membresia.estado_cambiado',     'tenencia', 'membresia',    'Suspension o reactivacion de una membresia sin removerla.'),
    ('membresia.removida',            'tenencia', 'membresia',    'Remocion de un miembro, por decision de un administrador o por iniciativa propia.'),
    ('membresia.invitada',            'tenencia', 'invitacion',   'Emision de una invitacion por correo con un rol propuesto.'),
    ('membresia.invitacion_resuelta', 'tenencia', 'invitacion',   'Desenlace de una invitacion: aceptada, revocada, expirada o intento fallido de redencion.'),
    ('autorizacion.denegada',         'tenencia', 'autorizacion', 'Denegacion de una autorizacion: sin membresia, membresia suspendida, organizacion no operativa o rol insuficiente. Las concesiones NO se auditan (INV-TEN-25).');
