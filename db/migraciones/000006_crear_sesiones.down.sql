-- Reversión de 000006_crear_sesiones. Aceptable en desarrollo y CI; en
-- producción, revisar si hay sesiones activas antes de bajarla (destruye la
-- cadena de rotación de tokens de refresco, evidencia forense de la sesión).

REVOKE SELECT, INSERT, UPDATE ON tokens_refresco FROM rol_aplicacion;
REVOKE SELECT, INSERT, UPDATE ON sesiones        FROM rol_aplicacion;

-- tokens_refresco primero: referencia a sesiones(id) y a sí misma
-- (hash_sucesor).
DROP TABLE IF EXISTS tokens_refresco;
DROP TABLE IF EXISTS sesiones;
