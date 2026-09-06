-- Contexto Identidad — MFA/OTP: cinco acciones nuevas en el catálogo cerrado
-- `auditoria_acciones` (secciones 1.6 y 6 de docs/design/otp-mfa.md).
-- Formato exigido por `auditoria_acciones_formato` (^[a-z_]+\.[a-z_]+$) —
-- las cinco cumplen.
--
-- Migración separada de 000015 (crea las tablas de MFA), mismo criterio ya
-- fijado por 000007 (acciones de Acceso, separada de 000006 crear_sesiones)
-- y 000012 (acciones de Tenencia, separada de 000009/000013): el catálogo
-- de auditoría no depende de las tablas de dominio del propio contexto (la
-- FK real es `auditoria.accion -> auditoria_acciones.accion`, no hacia
-- `factores_mfa`), así que no hay razón para acoplar ambas migraciones. A
-- diferencia del caso de Tenencia (000012 tuvo que renumerarse por el
-- incidente de secuenciación documentado en su propio comentario de
-- cabecera), aquí no hay ningún hueco: 000016 es simplemente el siguiente
-- número libre después de 000015.
--
-- Ver docs/catalogos/acciones-auditoria.md, sección "MFA/OTP", para la
-- versión legible de esta misma tabla.

INSERT INTO auditoria_acciones (accion, contexto, recurso, descripcion) VALUES
    ('usuario.mfa_habilitado',            'identidad', 'usuario', 'Generacion de un secreto TOTP nuevo y creacion del FactorMFA sin confirmar todavia.'),
    ('usuario.mfa_confirmado',            'identidad', 'usuario', 'Primer codigo TOTP correcto: el FactorMFA queda confirmado y Usuario.tieneMFA pasa a true.'),
    ('usuario.mfa_deshabilitado',         'identidad', 'usuario', 'El usuario desactivo su segundo factor, tras demostrar posesion con un codigo propio (ADR 0039).'),
    ('usuario.codigo_respaldo_consumido', 'identidad', 'usuario', 'Un codigo de respaldo de un solo uso se consumio con exito durante la verificacion OTP.'),
    ('usuario.otp_verificacion_fallida',  'identidad', 'usuario', 'Un codigo presentado en el flujo de step-up no coincidio ni con el TOTP esperado ni con ningun codigo de respaldo disponible.');
