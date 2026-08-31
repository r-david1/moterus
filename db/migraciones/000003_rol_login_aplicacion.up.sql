-- -----------------------------------------------------------------------------
-- Rol de login para el servicio de aplicación (Identidad).
--
-- Gap detectado tras implementar los adapters de infraestructura: la
-- migración 000002 ya creaba `rol_aplicacion` (NOLOGIN, ADR 0005) y le
-- otorgaba privilegios acotados sobre `auditoria*`, pero (a) nunca existió
-- un rol CON login que heredara esos privilegios — el servicio se conectaba
-- con el rol dueño de la base (superusuario en el docker-compose de
-- desarrollo), lo que anula por completo el REVOKE UPDATE/DELETE de ADR
-- 0005 (un superusuario ignora los permisos de tabla) — y (b) `usuarios`
-- (000001) nunca tuvo ningún GRANT explícito hacia `rol_aplicacion`.
--
-- Esta migración cierra ambos huecos.
-- -----------------------------------------------------------------------------

-- 1. Privilegios de rol_aplicacion sobre usuarios (000001 no los otorgó
--    porque rol_aplicacion todavía no existía en esa migración). Sin DELETE
--    a propósito: los usuarios nunca se borran físicamente, solo cambian de
--    EstadoUsuario (dominio, PuedeTransicionarA).
GRANT SELECT, INSERT, UPDATE ON usuarios TO rol_aplicacion;

-- 2. Rol CON login que el proceso del servicio usa en tiempo de ejecución.
--    Hereda los privilegios de rol_aplicacion vía membresía (INHERIT es el
--    comportamiento por defecto de CREATE ROLE). Contraseña de desarrollo
--    fija a propósito, igual que auth_service_dev_password en
--    docker-compose — en un entorno real se gestiona vía secret manager, no
--    en una migración versionada.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'rol_login_identidad') THEN
        CREATE ROLE rol_login_identidad
            LOGIN
            PASSWORD 'identidad_app_dev_password'
            NOSUPERUSER
            NOCREATEDB
            NOCREATEROLE
            NOREPLICATION
            NOBYPASSRLS;
    END IF;
END
$$;

GRANT rol_aplicacion TO rol_login_identidad;

COMMENT ON ROLE rol_login_identidad IS 'Rol de conexión del proceso api (Identidad). No es dueño de ninguna tabla; solo hereda los privilegios acotados de rol_aplicacion (ADR 0005). El pool de la aplicación debe conectarse con este rol, nunca con el rol dueño/superusuario de la base.';
