-- Reversión de 000003_rol_login_aplicacion.

REVOKE rol_aplicacion FROM rol_login_identidad;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'rol_login_identidad') THEN
        DROP ROLE rol_login_identidad;
    END IF;
END
$$;

REVOKE SELECT, INSERT, UPDATE ON usuarios FROM rol_aplicacion;
