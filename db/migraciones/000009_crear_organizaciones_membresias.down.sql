-- Reversión de 000009_crear_organizaciones_membresias. Aceptable en
-- desarrollo y CI; en producción, revisar si hay organizaciones/membresías
-- activas antes de bajarla.

REVOKE SELECT, INSERT, UPDATE ON membresias     FROM rol_aplicacion;
REVOKE SELECT, INSERT, UPDATE ON organizaciones FROM rol_aplicacion;

DROP TRIGGER IF EXISTS organizaciones_al_menos_un_propietario ON organizaciones;
DROP TRIGGER IF EXISTS membresias_al_menos_un_propietario     ON membresias;

DROP FUNCTION IF EXISTS tenencia_verificar_propietario_de_organizacion();
DROP FUNCTION IF EXISTS tenencia_verificar_propietario();

-- membresias primero: referencia a organizaciones(id) y a usuarios(id).
DROP TABLE IF EXISTS membresias;
DROP TABLE IF EXISTS organizaciones;
