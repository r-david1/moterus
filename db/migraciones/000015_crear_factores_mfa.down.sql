-- Reversión de 000015_crear_factores_mfa. Aceptable en desarrollo y CI; en
-- producción, revisar si hay usuarios con MFA habilitado antes de bajarla
-- (perderían su segundo factor y sus códigos de respaldo).

REVOKE SELECT, INSERT, UPDATE ON factores_mfa, codigos_respaldo_mfa FROM rol_aplicacion;

-- codigos_respaldo_mfa primero: referencia a factores_mfa(id).
DROP TABLE IF EXISTS codigos_respaldo_mfa;
DROP TABLE IF EXISTS factores_mfa;
