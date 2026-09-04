-- Contexto Tenencia — aislamiento multi-tenant con Row Level Security
-- (sección 6.3 de docs/design/tenencia-bounded-context.md, ADR 0031).
--
-- Materializa lo que internal/tenencia/adaptadores/postgres/doc.go ya
-- prometía ("con RLS por tenant") y lo que ADR 0017 dejó anotado como
-- "multi-tenant, ADR pendiente de Tenencia". Deliberadamente después de
-- 000009/000012/000013, una vez que ya existen casos de uso reales
-- (internal/tenencia/aplicacion/) que la ejercitan.
--
-- Mecanismo (ADR 0031): dos GUC de sesión fijados con SET LOCAL por
-- plataforma/bd al abrir cada transacción con AlcanceDeTenencia publicado en
-- el ctx: app.usuario_actual y app.organizacion_actual. Si no se fijan,
-- current_setting(..., true) devuelve NULL, la comparación de la política es
-- NULL, y no se ve ninguna fila: falla CERRADO (INV-TEN-30), a diferencia de
-- SET ROLE (rechazado en ADR 0017) que falla ABIERTO si se olvida.
--
-- Precedente que no hay que tocar: rol_login_identidad se creó con
-- NOBYPASSRLS explícito en 000003_rol_login_aplicacion.up.sql — RLS ya
-- estaba anticipado en el rol de runtime.
--
-- FORCE ROW LEVEL SECURITY aplica también al rol dueño de las tablas: esta
-- migración hace solo DDL, nunca DML directo sobre organizaciones,
-- membresias o invitaciones.

-- -----------------------------------------------------------------------------
-- Lectores de los GUC de alcance. STABLE, sin SECURITY DEFINER: solo leen la
-- configuración de la transacción actual.
-- -----------------------------------------------------------------------------
CREATE FUNCTION tenencia_usuario_actual() RETURNS UUID
LANGUAGE sql STABLE AS $$
    SELECT nullif(current_setting('app.usuario_actual', true), '')::uuid
$$;

COMMENT ON FUNCTION tenencia_usuario_actual() IS 'Lee el GUC de sesion app.usuario_actual fijado con SET LOCAL por plataforma/bd (ADR 0031). NULL si no esta fijado: las politicas RLS fallan cerradas.';

CREATE FUNCTION tenencia_organizacion_actual() RETURNS UUID
LANGUAGE sql STABLE AS $$
    SELECT nullif(current_setting('app.organizacion_actual', true), '')::uuid
$$;

COMMENT ON FUNCTION tenencia_organizacion_actual() IS 'Lee el GUC de sesion app.organizacion_actual fijado con SET LOCAL por plataforma/bd (ADR 0031). NULL si no esta fijado: las politicas RLS fallan cerradas.';

-- SECURITY DEFINER: la política de `organizaciones` necesita consultar
-- `membresias`, que a su vez tiene RLS. Sin esto Postgres aplicaría RLS
-- también dentro de la subconsulta y la política se volvería
-- recursiva/vacía. Es el patrón estándar para políticas que cruzan tablas.
CREATE FUNCTION tenencia_es_miembro_activo(p_organizacion UUID) RETURNS BOOLEAN
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, public AS $$
    SELECT EXISTS (
        SELECT 1 FROM membresias m
        WHERE m.organizacion_id = p_organizacion
          AND m.usuario_id      = tenencia_usuario_actual()
          AND m.estado          = 'activa'
    )
$$;

COMMENT ON FUNCTION tenencia_es_miembro_activo(UUID) IS 'SECURITY DEFINER: evita que la politica de organizaciones dispare RLS recursivo al consultar membresias (ADR 0031).';

-- -----------------------------------------------------------------------------
-- RLS forzado en las tres tablas del contexto.
-- -----------------------------------------------------------------------------
ALTER TABLE membresias     ENABLE ROW LEVEL SECURITY;
ALTER TABLE membresias     FORCE  ROW LEVEL SECURITY;
ALTER TABLE organizaciones ENABLE ROW LEVEL SECURITY;
ALTER TABLE organizaciones FORCE  ROW LEVEL SECURITY;
ALTER TABLE invitaciones   ENABLE ROW LEVEL SECURITY;
ALTER TABLE invitaciones   FORCE  ROW LEVEL SECURITY;

-- membresias: dos cláusulas, una por patrón de acceso real.
--   (a) "estoy operando dentro de una organización"  → organizacion_id = org actual
--   (b) "estoy mirando mis propias membresías"       → usuario_id      = usuario actual
-- Sin (b), ListarMisOrganizaciones sería imposible sin desactivar RLS.
CREATE POLICY membresias_aislamiento ON membresias
    FOR ALL TO rol_aplicacion
    USING      (organizacion_id = tenencia_organizacion_actual()
                OR usuario_id   = tenencia_usuario_actual())
    WITH CHECK (organizacion_id = tenencia_organizacion_actual()
                OR usuario_id   = tenencia_usuario_actual());

CREATE POLICY organizaciones_aislamiento ON organizaciones
    FOR ALL TO rol_aplicacion
    USING      (id = tenencia_organizacion_actual() OR tenencia_es_miembro_activo(id))
    -- creada_por cubre el INSERT de CrearOrganizacion, cuando todavía no hay
    -- membresía ni organización "actual".
    WITH CHECK (id = tenencia_organizacion_actual()
                OR tenencia_es_miembro_activo(id)
                OR creada_por = tenencia_usuario_actual());

CREATE POLICY invitaciones_aislamiento ON invitaciones
    FOR ALL TO rol_aplicacion
    USING      (organizacion_id = tenencia_organizacion_actual())
    WITH CHECK (organizacion_id = tenencia_organizacion_actual());

-- -----------------------------------------------------------------------------
-- Única vía de escape, nombrada y acotada: AceptarInvitacion tiene que
-- resolver un token de alguien que todavía no es miembro de ninguna
-- organización, así que no hay app.organizacion_actual posible en ese punto.
-- El token ES la capacidad: quien lo presenta prueba haber recibido el
-- correo. Recibe el hash (no el token en claro) y devuelve TODOS los campos
-- que dominio.ReconstituirInvitacion necesita para rehidratar el agregado
-- completo (RepositorioInvitaciones.BuscarPorHash devuelve *dominio.Invitacion,
-- no una proyección parcial) — en particular invitada_por, sin el cual
-- AceptarInvitacion no podría crear la Membresia resultante
-- (dominio.CrearMembresiaDesdeInvitacion exige otorgadaPor no vacío).
-- -----------------------------------------------------------------------------
CREATE FUNCTION tenencia_resolver_invitacion(p_hash TEXT)
RETURNS TABLE (id UUID, organizacion_id UUID, correo_destinatario TEXT,
               rol_propuesto TEXT, estado TEXT, hash_token TEXT,
               invitada_por UUID, creada_en TIMESTAMPTZ, expira_en TIMESTAMPTZ,
               resuelta_en TIMESTAMPTZ)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, public AS $$
    SELECT i.id, i.organizacion_id, i.correo_destinatario,
           i.rol_propuesto, i.estado, i.hash_token,
           i.invitada_por, i.creada_en, i.expira_en, i.resuelta_en
    FROM invitaciones i
    WHERE i.hash_token = p_hash
$$;

COMMENT ON FUNCTION tenencia_resolver_invitacion(TEXT) IS 'Unica via de escape de RLS del contexto (ADR 0031): resuelve una invitacion por hash de token para AceptarInvitacion, antes de que el aceptante sea miembro de ninguna organizacion.';

REVOKE ALL ON FUNCTION tenencia_resolver_invitacion(TEXT) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION tenencia_resolver_invitacion(TEXT) TO rol_aplicacion;
