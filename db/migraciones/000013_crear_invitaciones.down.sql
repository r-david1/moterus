-- Reversión de 000013_crear_invitaciones. Aceptable en desarrollo y CI; en
-- producción, revisar si hay invitaciones pendientes antes de bajarla.

REVOKE SELECT, INSERT, UPDATE ON invitaciones FROM rol_aplicacion;

DROP TABLE IF EXISTS invitaciones;
