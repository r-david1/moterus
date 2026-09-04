-- Reversión de 000014_rls_tenencia. No toca datos, solo el aparato de RLS.

REVOKE EXECUTE ON FUNCTION tenencia_resolver_invitacion(TEXT) FROM rol_aplicacion;
DROP FUNCTION IF EXISTS tenencia_resolver_invitacion(TEXT);

DROP POLICY IF EXISTS invitaciones_aislamiento   ON invitaciones;
DROP POLICY IF EXISTS organizaciones_aislamiento ON organizaciones;
DROP POLICY IF EXISTS membresias_aislamiento      ON membresias;

ALTER TABLE invitaciones   NO FORCE ROW LEVEL SECURITY;
ALTER TABLE invitaciones   DISABLE ROW LEVEL SECURITY;
ALTER TABLE organizaciones NO FORCE ROW LEVEL SECURITY;
ALTER TABLE organizaciones DISABLE ROW LEVEL SECURITY;
ALTER TABLE membresias     NO FORCE ROW LEVEL SECURITY;
ALTER TABLE membresias     DISABLE ROW LEVEL SECURITY;

DROP FUNCTION IF EXISTS tenencia_es_miembro_activo(UUID);
DROP FUNCTION IF EXISTS tenencia_organizacion_actual();
DROP FUNCTION IF EXISTS tenencia_usuario_actual();
