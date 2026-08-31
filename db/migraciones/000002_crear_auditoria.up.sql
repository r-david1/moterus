-- =============================================================================
-- Contexto AUDITORÍA — bitácora forense append-only con hash-chaining SHA-256.
-- Implementa ADR 0005 (docs/adr/0005-auditoria-forense-hash-chaining.md).
-- Convención de nombres: ADR 0004 (espanol, snake_case, plural, sin tildes/ñ).
--
-- Tres propiedades que esta migración tiene que garantizar, en este orden:
--   1. TAMPER-EVIDENT  : cada fila encadena el hash de la anterior; alterar,
--                        borrar o reordenar rompe la cadena de forma
--                        matemáticamente detectable (no depende de confiar
--                        en permisos).
--   2. APPEND-ONLY     : por permisos de rol (REVOKE) + trigger de bloqueo,
--                        no por convención ni por disciplina del código Go.
--   3. NO FORJABLE     : la cadena la calcula la BASE DE DATOS en un trigger
--                        BEFORE INSERT. La aplicación NO envía secuencia ni
--                        hashes; si los envía, se sobrescriben. Un adaptador
--                        Go comprometido no puede fabricar una cadena válida.
--
-- POR QUÉ EL HASH SE CALCULA EN POSTGRES Y NO EN EL ADAPTADOR GO
-- ---------------------------------------------------------------------------
--   a) Atomicidad de la lectura del eslabón previo: el hash_anterior se lee
--      bajo pg_advisory_xact_lock dentro de la MISMA transacción del INSERT.
--      Calculándolo en Go habría una ventana de carrera (leer último hash →
--      insertar) que produce bifurcaciones de cadena bajo concurrencia.
--   b) Superficie de forja: si el adaptador calculara el hash, el rol de la
--      aplicación podría insertar cualquier par (hash_anterior, hash_actual)
--      internamente consistente pero con el payload adulterado. Con el
--      trigger, el rol de aplicación literalmente no tiene forma de escribir
--      esas columnas.
--   c) Independencia del verificador: el job Go (cmd/verificador-auditoria)
--      reimplementa la misma serialización canónica y RECALCULA los hashes
--      desde las columnas crudas. Dos implementaciones independientes: si el
--      trigger fuera reemplazado por uno malicioso, el verificador Go lo
--      detecta; si el verificador Go fuera manipulado, la función SQL
--      verificar_cadena_auditoria() sigue disponible para el rol auditor.
--
-- Coste aceptado y documentado: el advisory lock serializa los INSERT de
-- auditoría (y, por tanto, la parte final de cada transacción de negocio que
-- audita). Para el volumen de un servicio de autenticación es asumible; si
-- alguna vez deja de serlo, la salida es particionar la cadena por dominio de
-- eventos (una cadena por partición) — NO quitar el lock.
-- =============================================================================


-- -----------------------------------------------------------------------------
-- 1. Roles (idempotentes: 000001 o un despliegue previo pueden haberlos creado)
-- -----------------------------------------------------------------------------
DO $$
BEGIN
    -- Rol de la APLICACIÓN: sólo puede INSERTAR y LEER auditoría.
    -- La API debe conectarse con este rol, NUNCA con el owner ni con superusuario:
    -- REVOKE no aplica al owner de la tabla, así que ejecutar la app como owner
    -- anularía toda la separación de privilegios de esta migración.
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'rol_aplicacion') THEN
        CREATE ROLE rol_aplicacion NOLOGIN;
    END IF;

    -- Rol de sólo lectura para peritaje/exportación de evidencia. Separado del
    -- rol de la aplicación: quien consulta la bitácora no es quien la escribe.
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'rol_auditor_lectura') THEN
        CREATE ROLE rol_auditor_lectura NOLOGIN;
    END IF;

    -- Rol del job de verificación de integridad y del anclaje externo. Puede
    -- leer la cadena y anotar checkpoints/anclas, pero tampoco puede mutarla.
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'rol_mantenimiento_auditoria') THEN
        CREATE ROLE rol_mantenimiento_auditoria NOLOGIN;
    END IF;
END
$$;


-- -----------------------------------------------------------------------------
-- 2. Catálogo cerrado de acciones (INV-ID-17)
--    El valor de `accion` no es un string libre: es una FK. Agregar una acción
--    nueva exige una migración, no un literal inventado en un caso de uso.
--    Documentado en docs/catalogos/acciones-auditoria.md.
-- -----------------------------------------------------------------------------
CREATE TABLE auditoria_acciones (
    accion       TEXT PRIMARY KEY,
    contexto     TEXT NOT NULL,
    recurso      TEXT NOT NULL,
    descripcion  TEXT NOT NULL,
    creada_en    TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT auditoria_acciones_formato CHECK (accion ~ '^[a-z_]+\.[a-z_]+$')
);

COMMENT ON TABLE auditoria_acciones IS 'Catálogo cerrado de acciones auditables (ADR 0005). Fuente de verdad de docs/catalogos/acciones-auditoria.md.';
COMMENT ON COLUMN auditoria_acciones.accion IS 'Verbo canónico recurso.accion en snake_case; inmutable una vez publicado.';
COMMENT ON COLUMN auditoria_acciones.recurso IS 'Recurso por defecto que afecta la acción; se replica en auditoria.recurso para poder consultar sin JOIN.';

INSERT INTO auditoria_acciones (accion, contexto, recurso, descripcion) VALUES
    ('usuario.registrado',            'identidad', 'usuario', 'Alta de un usuario nuevo en estado pendiente_verificacion.'),
    ('usuario.registro_rechazado',    'identidad', 'usuario', 'Intento de alta rechazado (Confianza, contrasena filtrada o correo duplicado).'),
    ('usuario.login',                 'identidad', 'usuario', 'Intento de autenticacion por credenciales: exito, fallo o denegado por Confianza.'),
    ('usuario.step_up_requerido',     'identidad', 'usuario', 'La autenticacion exigio un segundo factor (MFA propio o step-up de Confianza).'),
    ('usuario.contrasena_cambiada',   'identidad', 'usuario', 'El usuario cambio su contrasena con la actual ya verificada.'),
    ('usuario.credencial_rehasheada', 'identidad', 'usuario', 'Rehash oportunista del hash de contrasena tras un login exitoso (INV-ID-13).'),
    ('usuario.correo_verificado',     'identidad', 'usuario', 'Confirmacion del correo: transicion pendiente_verificacion -> activo.'),
    ('usuario.estado_cambiado',       'identidad', 'usuario', 'Transicion de EstadoUsuario sin evento propio: suspender, bloquear o reactivar.'),
    ('usuario.consultado',            'identidad', 'usuario', 'Un tercero (no el propio usuario) consulto el perfil de un usuario.');


-- -----------------------------------------------------------------------------
-- 3. Tabla auditoria
-- -----------------------------------------------------------------------------
CREATE TABLE auditoria (
    id                 UUID        NOT NULL DEFAULT gen_random_uuid(),

    -- Version del algoritmo de serializacion canonica + hash. Va DENTRO del
    -- payload hasheado: si algun dia cambia el formato, el verificador sabe
    -- que regla aplicar a cada tramo historico sin invalidar la cadena vieja.
    version_hash       SMALLINT    NOT NULL DEFAULT 1,

    -- Estrictamente consecutiva y sin huecos: la asigna el trigger como
    -- (maximo anterior + 1) bajo advisory lock. NO se usa IDENTITY/SEQUENCE
    -- a proposito: las secuencias no son transaccionales y un ROLLBACK dejaria
    -- huecos legitimos, haciendo indistinguible un hueco normal de un borrado.
    secuencia          BIGINT      NOT NULL,

    -- Momento en que la BASE DE DATOS registro el evento. clock_timestamp()
    -- (no now()): now() queda congelado al inicio de la transaccion y en una
    -- transaccion larga falsearia la cronologia.
    marca_tiempo       TIMESTAMPTZ NOT NULL,

    -- Momento en que el evento de dominio OCURRIO segun el reloj de la
    -- aplicacion. Separado de marca_tiempo a proposito: la divergencia entre
    -- ambos es en si misma una senal forense (desfase de reloj, reintento,
    -- escritura diferida).
    ocurrido_en        TIMESTAMPTZ NOT NULL,

    -- Quien. NULL cuando el actor no se resolvio (login con correo inexistente,
    -- registro denegado por Confianza antes de crear el agregado).
    usuario_id         UUID,
    -- Con que autoridad / en que tenant. NULL hasta que exista Tenencia.
    organizacion_id    UUID,

    accion             TEXT        NOT NULL REFERENCES auditoria_acciones (accion),
    recurso            TEXT        NOT NULL,
    recurso_id         TEXT,
    resultado          TEXT        NOT NULL,

    -- Contexto forense: viaja en el VO OrigenSolicitud de Identidad.
    ip_origen          INET,
    huella_dispositivo TEXT,
    id_solicitud       TEXT,

    -- Contexto adicional. NUNCA contrasenas, hashes, tokens ni codigos OTP.
    detalles           JSONB       NOT NULL DEFAULT '{}'::jsonb,

    hash_anterior      TEXT        NOT NULL,
    hash_actual        TEXT        NOT NULL,

    CONSTRAINT auditoria_pkey PRIMARY KEY (id),
    CONSTRAINT auditoria_secuencia_unica UNIQUE (secuencia),
    CONSTRAINT auditoria_secuencia_positiva CHECK (secuencia >= 1),
    CONSTRAINT auditoria_version_hash_conocida CHECK (version_hash = 1),

    CONSTRAINT auditoria_resultado_valido CHECK (
        resultado IN ('exito', 'fallo', 'denegado')
    ),
    CONSTRAINT auditoria_hash_formato CHECK (
        hash_anterior ~ '^[0-9a-f]{64}$' AND hash_actual ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT auditoria_detalles_objeto CHECK (jsonb_typeof(detalles) = 'object'),

    -- Defensa en profundidad contra fuga de secretos en `detalles`. No es
    -- exhaustiva (solo claves de primer nivel): la regla dura sigue siendo que
    -- el ACL de cada contexto nunca construya ese JSON con material sensible.
    CONSTRAINT auditoria_detalles_sin_secretos CHECK (
        NOT (detalles ?| ARRAY[
            'contrasena', 'contrasena_plana', 'contrasena_hash', 'hash_contrasena',
            'password', 'password_hash', 'secreto', 'secreto_totp',
            'token', 'access_token', 'refresh_token', 'jwt',
            'otp', 'codigo_otp', 'codigo'
        ])
    )
);

COMMENT ON TABLE auditoria IS 'Bitácora forense append-only con hash-chaining SHA-256 (ADR 0005). Sólo INSERT y SELECT: UPDATE/DELETE/TRUNCATE bloqueados por permisos y por trigger.';
COMMENT ON COLUMN auditoria.version_hash IS 'Versión del algoritmo de serialización canónica; forma parte del payload hasheado.';
COMMENT ON COLUMN auditoria.secuencia IS 'Consecutiva estricta sin huecos, asignada por trigger bajo advisory lock. Un hueco = fila borrada.';
COMMENT ON COLUMN auditoria.marca_tiempo IS 'clock_timestamp() del INSERT, forzado por trigger. Precisión de microsegundos.';
COMMENT ON COLUMN auditoria.ocurrido_en IS 'EventoDominio.OcurridoEn() según el puerto Reloj de la aplicación.';
COMMENT ON COLUMN auditoria.usuario_id IS 'Actor. NULL si no se resolvió (p. ej. login con correo inexistente, INV-ID-11).';
COMMENT ON COLUMN auditoria.accion IS 'FK al catálogo cerrado auditoria_acciones (INV-ID-17).';
COMMENT ON COLUMN auditoria.id_solicitud IS 'Correlaciona con las trazas distribuidas (middleware de plataforma/servidor).';
COMMENT ON COLUMN auditoria.detalles IS 'Contexto adicional JSONB. Prohibido: contraseñas, hashes, tokens, OTPs.';
COMMENT ON COLUMN auditoria.hash_anterior IS 'hash_actual de la fila secuencia-1; 64 ceros en el registro génesis.';
COMMENT ON COLUMN auditoria.hash_actual IS 'SHA-256 hex del payload canónico. Lo calcula el trigger, nunca la aplicación.';

-- Índice único sobre hash_anterior: garantía ESTRUCTURAL de que la cadena
-- nunca se bifurca. Dos filas no pueden apuntar al mismo eslabón previo, y
-- sólo puede existir un registro génesis (hash_anterior = 64 ceros).
CREATE UNIQUE INDEX auditoria_hash_anterior_idx ON auditoria (hash_anterior);
CREATE UNIQUE INDEX auditoria_hash_actual_idx ON auditoria (hash_actual);

-- Hot paths de consulta forense (nunca full scan).
CREATE INDEX auditoria_usuario_tiempo_idx    ON auditoria (usuario_id, marca_tiempo DESC) WHERE usuario_id IS NOT NULL;
CREATE INDEX auditoria_accion_tiempo_idx     ON auditoria (accion, marca_tiempo DESC);
CREATE INDEX auditoria_resultado_tiempo_idx  ON auditoria (resultado, marca_tiempo DESC) WHERE resultado <> 'exito';
CREATE INDEX auditoria_solicitud_idx         ON auditoria (id_solicitud) WHERE id_solicitud IS NOT NULL;
CREATE INDEX auditoria_ip_tiempo_idx         ON auditoria (ip_origen, marca_tiempo DESC) WHERE ip_origen IS NOT NULL;


-- -----------------------------------------------------------------------------
-- 4. Serialización canónica del payload
--
--    Codificación tipo netstring: cada campo se emite como
--        <octetos>:<valor>,        y  NULL  como  -1:,
--    Se eligió sobre un simple `campo1||'|'||campo2` porque el separador plano
--    es vulnerable a inyección de delimitador: un `huella_dispositivo` con un
--    '|' podría desplazar el resto de los campos y producir el mismo hash para
--    dos eventos distintos. Con prefijo de longitud la decodificación es única.
--    Además distingue NULL de cadena vacía, que en auditoría no es lo mismo
--    ("no había IP" vs "la IP venía vacía").
--
--    ORDEN CANÓNICO DE LOS 16 CAMPOS (normativo — el verificador Go lo replica
--    exactamente en internal/auditoria/dominio/cadena.go):
--       1 version_hash          9 accion
--       2 id                   10 recurso
--       3 secuencia            11 recurso_id
--       4 hash_anterior        12 resultado
--       5 marca_tiempo         13 ip_origen
--       6 ocurrido_en          14 huella_dispositivo
--       7 usuario_id           15 id_solicitud
--       8 organizacion_id      16 detalles
--
--    Reglas de renderizado a texto:
--      - timestamptz : to_char(v AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')
--      - uuid / inet : representación textual canónica de Postgres
--      - jsonb       : `::text` de jsonb (Postgres ya normaliza orden de claves,
--                      espacios y duplicados, así que el texto es determinista)
--      - bigint/smallint : decimal sin ceros a la izquierda
--
--    hash_actual = encode(sha256(convert_to(payload, 'UTF8')), 'hex')
-- -----------------------------------------------------------------------------
CREATE FUNCTION auditoria_campo_canonico(p_valor TEXT)
RETURNS TEXT
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
    SELECT CASE
        WHEN p_valor IS NULL THEN '-1:,'
        ELSE octet_length(p_valor)::text || ':' || p_valor || ','
    END;
$$;

COMMENT ON FUNCTION auditoria_campo_canonico(TEXT) IS 'Codifica un campo como netstring <octetos>:<valor>, ; NULL como -1:, . Evita inyección de delimitador.';

CREATE FUNCTION auditoria_instante_canonico(p_valor TIMESTAMPTZ)
RETURNS TEXT
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
    SELECT CASE
        WHEN p_valor IS NULL THEN NULL
        ELSE to_char(p_valor AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')
    END;
$$;

COMMENT ON FUNCTION auditoria_instante_canonico(TIMESTAMPTZ) IS 'ISO-8601 UTC con 6 dígitos de microsegundos; independiente del TimeZone de la sesión.';

CREATE FUNCTION auditoria_payload_canonico(
    p_version_hash       SMALLINT,
    p_id                 UUID,
    p_secuencia          BIGINT,
    p_hash_anterior      TEXT,
    p_marca_tiempo       TIMESTAMPTZ,
    p_ocurrido_en        TIMESTAMPTZ,
    p_usuario_id         UUID,
    p_organizacion_id    UUID,
    p_accion             TEXT,
    p_recurso            TEXT,
    p_recurso_id         TEXT,
    p_resultado          TEXT,
    p_ip_origen          INET,
    p_huella_dispositivo TEXT,
    p_id_solicitud       TEXT,
    p_detalles           JSONB
)
RETURNS TEXT
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
    SELECT auditoria_campo_canonico(p_version_hash::text)
        || auditoria_campo_canonico(p_id::text)
        || auditoria_campo_canonico(p_secuencia::text)
        || auditoria_campo_canonico(p_hash_anterior)
        || auditoria_campo_canonico(auditoria_instante_canonico(p_marca_tiempo))
        || auditoria_campo_canonico(auditoria_instante_canonico(p_ocurrido_en))
        || auditoria_campo_canonico(p_usuario_id::text)
        || auditoria_campo_canonico(p_organizacion_id::text)
        || auditoria_campo_canonico(p_accion)
        || auditoria_campo_canonico(p_recurso)
        || auditoria_campo_canonico(p_recurso_id)
        || auditoria_campo_canonico(p_resultado)
        || auditoria_campo_canonico(p_ip_origen::text)
        || auditoria_campo_canonico(p_huella_dispositivo)
        || auditoria_campo_canonico(p_id_solicitud)
        || auditoria_campo_canonico(p_detalles::text);
$$;

COMMENT ON FUNCTION auditoria_payload_canonico IS 'Serialización canónica v1 de una fila de auditoria (16 campos netstring). Replicada en internal/auditoria/dominio/cadena.go.';

CREATE FUNCTION auditoria_hash_de_payload(p_payload TEXT)
RETURNS TEXT
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
    SELECT encode(sha256(convert_to(p_payload, 'UTF8')), 'hex');
$$;


-- -----------------------------------------------------------------------------
-- 5. Trigger BEFORE INSERT: asigna secuencia y encadena los hashes
-- -----------------------------------------------------------------------------
CREATE FUNCTION auditoria_asignar_cadena()
RETURNS TRIGGER
LANGUAGE plpgsql AS $$
DECLARE
    v_secuencia_anterior BIGINT;
    v_hash_anterior      TEXT;
BEGIN
    -- Serializa TODA la escritura de auditoría. Sin esto, dos INSERT
    -- concurrentes leerían el mismo último eslabón y bifurcarían la cadena.
    -- Clave fija y dedicada; el lock se libera al terminar la transacción.
    PERFORM pg_advisory_xact_lock(20260830, 5);

    SELECT a.secuencia, a.hash_actual
      INTO v_secuencia_anterior, v_hash_anterior
      FROM auditoria a
     ORDER BY a.secuencia DESC
     LIMIT 1;

    -- Columnas controladas por la base de datos: se sobrescribe siempre lo que
    -- haya mandado el cliente. La aplicación NO participa en la cadena.
    NEW.version_hash  := 1;
    NEW.id            := COALESCE(NEW.id, gen_random_uuid());
    NEW.secuencia     := COALESCE(v_secuencia_anterior, 0) + 1;
    NEW.hash_anterior := COALESCE(v_hash_anterior, repeat('0', 64));
    NEW.marca_tiempo  := clock_timestamp();
    NEW.ocurrido_en   := COALESCE(NEW.ocurrido_en, NEW.marca_tiempo);
    NEW.detalles      := COALESCE(NEW.detalles, '{}'::jsonb);

    NEW.hash_actual := auditoria_hash_de_payload(
        auditoria_payload_canonico(
            NEW.version_hash, NEW.id, NEW.secuencia, NEW.hash_anterior,
            NEW.marca_tiempo, NEW.ocurrido_en, NEW.usuario_id, NEW.organizacion_id,
            NEW.accion, NEW.recurso, NEW.recurso_id, NEW.resultado,
            NEW.ip_origen, NEW.huella_dispositivo, NEW.id_solicitud, NEW.detalles
        )
    );

    RETURN NEW;
END;
$$;

COMMENT ON FUNCTION auditoria_asignar_cadena() IS 'Asigna secuencia, hash_anterior y hash_actual. La aplicación no puede escribir esas columnas: el trigger las sobrescribe.';

CREATE TRIGGER trg_auditoria_asignar_cadena
    BEFORE INSERT ON auditoria
    FOR EACH ROW EXECUTE FUNCTION auditoria_asignar_cadena();


-- -----------------------------------------------------------------------------
-- 6. Bloqueo de mutación (defensa en profundidad sobre el REVOKE)
--    El REVOKE no alcanza al owner de la tabla ni a un superusuario; el trigger
--    sí los alcanza. Un superusuario todavía puede desactivar el trigger o
--    poner session_replication_role='replica', pero entonces la cadena de
--    hashes deja constancia matemática del resultado: es exactamente la razón
--    por la que el hash-chaining no es redundante con los permisos.
-- -----------------------------------------------------------------------------
CREATE FUNCTION bloquear_mutacion_auditoria()
RETURNS TRIGGER
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION
        'auditoria es append-only (ADR 0005): % no permitido sobre %',
        TG_OP, TG_TABLE_NAME
        USING ERRCODE = 'insufficient_privilege';
END;
$$;

COMMENT ON FUNCTION bloquear_mutacion_auditoria() IS 'Lanza excepción ante UPDATE/DELETE/TRUNCATE sobre las tablas de auditoría.';

CREATE TRIGGER trg_auditoria_bloquear_mutacion
    BEFORE UPDATE OR DELETE ON auditoria
    FOR EACH ROW EXECUTE FUNCTION bloquear_mutacion_auditoria();

CREATE TRIGGER trg_auditoria_bloquear_truncate
    BEFORE TRUNCATE ON auditoria
    FOR EACH STATEMENT EXECUTE FUNCTION bloquear_mutacion_auditoria();


-- -----------------------------------------------------------------------------
-- 7. Checkpoints del verificador de integridad
--    El job (cmd/verificador-auditoria) arranca desde el último checkpoint
--    íntegro en vez de recorrer la cadena completa cada vez. También
--    append-only: el historial de verificaciones es evidencia por sí mismo.
-- -----------------------------------------------------------------------------
CREATE TABLE auditoria_verificaciones (
    id                    UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    ejecutada_en          TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    secuencia_desde       BIGINT      NOT NULL,
    secuencia_hasta       BIGINT      NOT NULL,
    registros_verificados BIGINT      NOT NULL,
    resultado             TEXT        NOT NULL,
    secuencia_rota        BIGINT,
    hash_ultimo_verificado TEXT,
    detalle               TEXT,
    ejecutor              TEXT        NOT NULL DEFAULT current_user,

    CONSTRAINT auditoria_verificaciones_resultado_valido CHECK (
        resultado IN ('integra', 'rota', 'vacia')
    ),
    CONSTRAINT auditoria_verificaciones_rota_localizada CHECK (
        (resultado = 'rota') = (secuencia_rota IS NOT NULL)
    )
);

CREATE INDEX auditoria_verificaciones_tiempo_idx ON auditoria_verificaciones (ejecutada_en DESC);
CREATE INDEX auditoria_verificaciones_integras_idx ON auditoria_verificaciones (secuencia_hasta DESC) WHERE resultado = 'integra';

COMMENT ON TABLE auditoria_verificaciones IS 'Historial append-only de corridas del verificador de cadena. El último registro `integra` es el checkpoint de arranque del job.';

CREATE TRIGGER trg_auditoria_verificaciones_bloquear_mutacion
    BEFORE UPDATE OR DELETE ON auditoria_verificaciones
    FOR EACH ROW EXECUTE FUNCTION bloquear_mutacion_auditoria();

CREATE TRIGGER trg_auditoria_verificaciones_bloquear_truncate
    BEFORE TRUNCATE ON auditoria_verificaciones
    FOR EACH STATEMENT EXECUTE FUNCTION bloquear_mutacion_auditoria();


-- -----------------------------------------------------------------------------
-- 8. Anclas: sellado periódico del último hash (ADR 0005, "anclaje externo")
--    Aquí sólo vive el registro del ancla. La firma se produce fuera con una
--    clave dedicada (distinta de la clave JWT) y, cuando haya presupuesto, se
--    complementa con timestamping RFC 3161 y copia WORM independiente. La
--    tabla existe desde ahora para que el anclaje sea una tarea de operación,
--    no una migración futura que reabra el esquema.
-- -----------------------------------------------------------------------------
CREATE TABLE auditoria_anclas (
    id                 UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    creada_en          TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    secuencia_hasta    BIGINT      NOT NULL,
    hash_anclado       TEXT        NOT NULL,
    algoritmo_firma    TEXT        NOT NULL,
    firma              TEXT        NOT NULL,
    identificador_clave TEXT       NOT NULL,
    referencia_externa TEXT,

    CONSTRAINT auditoria_anclas_secuencia_unica UNIQUE (secuencia_hasta),
    CONSTRAINT auditoria_anclas_hash_formato CHECK (hash_anclado ~ '^[0-9a-f]{64}$')
);

COMMENT ON TABLE auditoria_anclas IS 'Sellos firmados del último hash de la cadena (prueba de existencia en el tiempo). referencia_externa guarda el token RFC 3161 o la URL del objeto WORM cuando exista.';

CREATE TRIGGER trg_auditoria_anclas_bloquear_mutacion
    BEFORE UPDATE OR DELETE ON auditoria_anclas
    FOR EACH ROW EXECUTE FUNCTION bloquear_mutacion_auditoria();

CREATE TRIGGER trg_auditoria_anclas_bloquear_truncate
    BEFORE TRUNCATE ON auditoria_anclas
    FOR EACH STATEMENT EXECUTE FUNCTION bloquear_mutacion_auditoria();


-- -----------------------------------------------------------------------------
-- 9. Verificación de la cadena dentro de la base de datos
--    Segunda implementación, independiente del job Go. Devuelve una fila por
--    cada anomalía encontrada, en orden de secuencia (la primera fila es el
--    primer punto de ruptura).
-- -----------------------------------------------------------------------------
CREATE FUNCTION verificar_cadena_auditoria(p_desde BIGINT DEFAULT 1)
RETURNS TABLE (
    secuencia      BIGINT,
    problema       TEXT,
    valor_esperado TEXT,
    valor_encontrado TEXT
)
LANGUAGE plpgsql STABLE AS $$
DECLARE
    r                    RECORD;
    v_secuencia_esperada BIGINT;
    v_hash_esperado      TEXT;
    v_hash_recalculado   TEXT;
BEGIN
    IF p_desde <= 1 THEN
        v_secuencia_esperada := 1;
        v_hash_esperado      := repeat('0', 64);
    ELSE
        SELECT a.hash_actual INTO v_hash_esperado
          FROM auditoria a WHERE a.secuencia = p_desde - 1;
        IF v_hash_esperado IS NULL THEN
            secuencia := p_desde - 1; problema := 'checkpoint_inexistente';
            valor_esperado := NULL; valor_encontrado := NULL;
            RETURN NEXT; RETURN;
        END IF;
        v_secuencia_esperada := p_desde;
    END IF;

    FOR r IN
        SELECT * FROM auditoria a WHERE a.secuencia >= p_desde ORDER BY a.secuencia ASC
    LOOP
        IF r.secuencia <> v_secuencia_esperada THEN
            secuencia := r.secuencia; problema := 'hueco_en_secuencia';
            valor_esperado := v_secuencia_esperada::text;
            valor_encontrado := r.secuencia::text;
            RETURN NEXT;
            v_secuencia_esperada := r.secuencia;
        END IF;

        IF r.hash_anterior IS DISTINCT FROM v_hash_esperado THEN
            secuencia := r.secuencia; problema := 'eslabon_roto';
            valor_esperado := v_hash_esperado;
            valor_encontrado := r.hash_anterior;
            RETURN NEXT;
        END IF;

        v_hash_recalculado := auditoria_hash_de_payload(
            auditoria_payload_canonico(
                r.version_hash, r.id, r.secuencia, r.hash_anterior,
                r.marca_tiempo, r.ocurrido_en, r.usuario_id, r.organizacion_id,
                r.accion, r.recurso, r.recurso_id, r.resultado,
                r.ip_origen, r.huella_dispositivo, r.id_solicitud, r.detalles
            )
        );

        IF v_hash_recalculado <> r.hash_actual THEN
            secuencia := r.secuencia; problema := 'contenido_alterado';
            valor_esperado := v_hash_recalculado;
            valor_encontrado := r.hash_actual;
            RETURN NEXT;
        END IF;

        v_hash_esperado      := r.hash_actual;
        v_secuencia_esperada := r.secuencia + 1;
    END LOOP;

    RETURN;
END;
$$;

COMMENT ON FUNCTION verificar_cadena_auditoria(BIGINT) IS 'Recorre la cadena desde p_desde y devuelve una fila por anomalía (hueco_en_secuencia | eslabon_roto | contenido_alterado). Sin filas = cadena íntegra.';


-- -----------------------------------------------------------------------------
-- 10. Separación de privilegios
--     Quien escribe la bitácora, quien la verifica y quien la lee son tres
--     roles distintos. Ninguno de los tres puede modificarla ni borrarla.
-- -----------------------------------------------------------------------------
REVOKE ALL ON auditoria                FROM PUBLIC;
REVOKE ALL ON auditoria_acciones       FROM PUBLIC;
REVOKE ALL ON auditoria_verificaciones FROM PUBLIC;
REVOKE ALL ON auditoria_anclas         FROM PUBLIC;

-- Aplicación: sólo puede añadir evidencia y leerla. Nunca alterarla.
GRANT SELECT, INSERT ON auditoria          TO rol_aplicacion;
GRANT SELECT         ON auditoria_acciones TO rol_aplicacion;
-- Explícito aunque redundante: deja la intención registrada en el catálogo de
-- privilegios, para que una revisión de permisos la vea sin leer esta migración.
REVOKE UPDATE, DELETE, TRUNCATE ON auditoria                FROM rol_aplicacion;
REVOKE UPDATE, DELETE, TRUNCATE ON auditoria_acciones       FROM rol_aplicacion;
REVOKE INSERT, UPDATE, DELETE, TRUNCATE ON auditoria_verificaciones FROM rol_aplicacion;
REVOKE INSERT, UPDATE, DELETE, TRUNCATE ON auditoria_anclas         FROM rol_aplicacion;

-- Mantenimiento/verificación: lee la cadena, anota checkpoints y anclas.
GRANT SELECT         ON auditoria                TO rol_mantenimiento_auditoria;
GRANT SELECT         ON auditoria_acciones       TO rol_mantenimiento_auditoria;
GRANT SELECT, INSERT ON auditoria_verificaciones TO rol_mantenimiento_auditoria;
GRANT SELECT, INSERT ON auditoria_anclas         TO rol_mantenimiento_auditoria;
REVOKE INSERT, UPDATE, DELETE, TRUNCATE ON auditoria FROM rol_mantenimiento_auditoria;

-- Auditor externo/perito: sólo lectura sobre todo. Ni siquiera puede insertar
-- una verificación (no debe poder dejar rastro propio en la evidencia).
GRANT SELECT ON auditoria                TO rol_auditor_lectura;
GRANT SELECT ON auditoria_acciones       TO rol_auditor_lectura;
GRANT SELECT ON auditoria_verificaciones TO rol_auditor_lectura;
GRANT SELECT ON auditoria_anclas         TO rol_auditor_lectura;

REVOKE ALL ON FUNCTION verificar_cadena_auditoria(BIGINT) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION verificar_cadena_auditoria(BIGINT)
    TO rol_mantenimiento_auditoria, rol_auditor_lectura;

-- Las funciones de serialización quedan ejecutables por PUBLIC a propósito:
-- el trigger BEFORE INSERT corre con los privilegios del rol que inserta, así
-- que rol_aplicacion necesita poder invocarlas. No exponen nada sensible.

-- Nota RLS: `auditoria` no lleva política de aislamiento por tenant todavía
-- porque `organizacion_id` es NULL hasta que exista el contexto Tenencia.
-- Cuando llegue, el aislamiento aplica al rol_auditor_lectura (un auditor de
-- una organización no debe ver eventos de otra), nunca al verificador de
-- integridad: la cadena se verifica completa o no se verifica.
