-- Contexto Identidad — agregado FactorMFA y su VO CodigoRespaldoMFA (sección 6
-- de docs/design/otp-mfa.md, ADR 0037/0040).
--
-- Notas de diseño (no reinventar sin volver a leer el documento):
--   * `factores_mfa.id` SIN `DEFAULT gen_random_uuid()`, mismo criterio que
--     `sesiones`/`organizaciones` (000006/000009): es un identificador
--     generado por el puerto GeneradorIDs de la aplicación (hoy un shim
--     temporal en `aplicacion/habilitar_mfa.go`, ver su comentario), y un
--     default en la base enmascararía en silencio un bug que produjera IDs
--     del tipo equivocado.
--   * `codigos_respaldo_mfa.id`, en cambio, SÍ lleva
--     `DEFAULT gen_random_uuid()`: a diferencia de FactorMFA,
--     `dominio.CodigoRespaldoMFA` es un value object sin identidad propia
--     (solo `hashCodigo`/`usadoEn`, ver `dominio/codigo_respaldo.go`) — no
--     hay ningún puerto que genere un ID para él, la fila necesita un
--     identificador propio solo por conveniencia relacional (igual criterio
--     que `usuarios.id` en 000001, que tampoco tiene una contraparte de
--     dominio que lo genere a mano).
--   * `secreto_cifrado` es BYTEA, no TEXT: es la salida binaria de
--     `CifradorSecretos` (AES-256-GCM, ADR 0038), cifrado SIMÉTRICO
--     REVERSIBLE, nunca un hash — el servidor tiene que poder descifrarlo en
--     cada verificación (INV-MFA-02).
--   * `confirmado` es un hecho histórico: una vez `true`, nunca vuelve a
--     `false` (lo garantiza únicamente el dominio, `FactorMFA.Confirmar`,
--     esta migración no lo impone con un CHECK porque no hay forma barata de
--     expresar "nunca decrece" sin un trigger dedicado, y no se justifica
--     todavía). `activo`, en cambio, sí puede volver a `false`
--     (`FactorMFA.Deshabilitar`) y a `true` de nuevo si el usuario vuelve a
--     habilitar MFA desde cero con un `FactorMFA` nuevo. Ver el comentario
--     de `RepositorioFactoresMFA` en `internal/identidad/puertos/salida.go`
--     y el commit 1e06146 (que corrigió el diseño original de este
--     documento, que no había anticipado esta columna): "confirmados", en
--     `BuscarConfirmadosDeUsuario`/`ContarConfirmadosDeUsuario`, significa
--     SIEMPRE `confirmado = true AND activo = true`.
--   * El índice único parcial de abajo filtra por AMBAS columnas
--     (`confirmado = true AND activo = true`), no solo por `confirmado`: si
--     filtrara solo por `confirmado`, un factor deshabilitado (que sigue
--     `confirmado = true` para siempre) bloquearía cualquier `HabilitarMFA`
--     posterior del mismo usuario — exactamente el bug que el commit citado
--     arriba corrigió en el dominio antes de que existiera este esquema.

CREATE TABLE factores_mfa (
    id              UUID        PRIMARY KEY,  -- generado por la app (GeneradorIDs), sin default
    usuario_id      UUID        NOT NULL REFERENCES usuarios(id),
    tipo            TEXT        NOT NULL,
    secreto_cifrado BYTEA       NOT NULL,
    confirmado      BOOLEAN     NOT NULL DEFAULT false,
    activo          BOOLEAN     NOT NULL DEFAULT true,
    creado_en       TIMESTAMPTZ NOT NULL,
    confirmado_en   TIMESTAMPTZ,

    -- ADR 0037: catálogo cerrado de un único valor en el MVP. Crece de forma
    -- aditiva (email/sms), nunca "a medias".
    CONSTRAINT factores_mfa_tipo_valido CHECK (tipo IN ('totp')),
    CONSTRAINT factores_mfa_confirmacion_coherente CHECK (confirmado = (confirmado_en IS NOT NULL))
);

COMMENT ON TABLE factores_mfa IS 'Agregado FactorMFA (contexto Identidad, docs/design/otp-mfa.md). id generado por la aplicacion.';
COMMENT ON COLUMN factores_mfa.secreto_cifrado IS 'Secreto TOTP CIFRADO (AES-256-GCM, CifradorSecretos) — reversible, nunca un hash (INV-MFA-02). El secreto en claro nunca se persiste.';
COMMENT ON COLUMN factores_mfa.confirmado IS 'Hecho historico: true desde la primera verificacion exitosa, nunca vuelve a false. No confundir con activo.';
COMMENT ON COLUMN factores_mfa.activo IS 'Estado vigente: true al habilitar, false tras Deshabilitar. "Confirmado" a efectos de INV-MFA-01/limite del MVP significa confirmado=true AND activo=true (ver puertos/salida.go).';

-- INV-MFA-01 (lado "no más de uno", ADR 0037) como garantía ESTRUCTURAL: a lo
-- sumo un factor confirmado Y activo por usuario en el MVP. Parcial por
-- ambas columnas, no solo por `confirmado` — ver nota de cabecera: de lo
-- contrario un factor deshabilitado bloquearía para siempre un HabilitarMFA
-- posterior del mismo usuario.
-- Sirve doble propósito: además de la restricción de unicidad, ES el índice
-- que resuelve el camino caliente de VerificarOTP (§3.4 del diseño) —
-- BuscarConfirmadosDeUsuario filtra exactamente por esta misma condición
-- (usuario_id, confirmado = true, activo = true), así que un índice de
-- lectura aparte sería redundante.
CREATE UNIQUE INDEX factores_mfa_confirmado_activo_por_usuario_idx
    ON factores_mfa (usuario_id) WHERE confirmado = true AND activo = true;

CREATE TABLE codigos_respaldo_mfa (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),  -- VO sin identidad propia en el dominio
    factor_id    UUID        NOT NULL REFERENCES factores_mfa(id),
    hash_codigo  TEXT        NOT NULL,
    usado_en     TIMESTAMPTZ,

    -- SHA-256 hex de 64 caracteres (HashearCodigoRespaldo), mismo formato ya
    -- usado por tokens_refresco.hash_token y tokens_verificacion_correo.
    CONSTRAINT codigos_respaldo_hash_formato CHECK (hash_codigo ~ '^[0-9a-f]{64}$')
);

COMMENT ON TABLE codigos_respaldo_mfa IS 'VO CodigoRespaldoMFA dentro del agregado FactorMFA (ADR 0040): 10 filas por factor confirmado, generadas junto con la confirmacion, nunca antes.';
COMMENT ON COLUMN codigos_respaldo_mfa.hash_codigo IS 'SHA-256 hex del codigo de respaldo. El valor en claro NUNCA se persiste (solo se muestra una vez, en la respuesta de ConfirmarFactorMFA).';

-- Un código de respaldo es único en todo el sistema (misma entropía y mismo
-- criterio de unicidad global que hash_token de tokens_refresco/invitaciones).
CREATE UNIQUE INDEX codigos_respaldo_hash_idx ON codigos_respaldo_mfa (hash_codigo);
-- Camino caliente de VerificarCodigo: buscar los códigos disponibles de un factor.
CREATE INDEX codigos_respaldo_factor_disponibles_idx
    ON codigos_respaldo_mfa (factor_id) WHERE usado_en IS NULL;

-- -----------------------------------------------------------------------------
-- Privilegios (ADR 0017: una tabla nueva no alcanza con crearla). Sin RLS:
-- ninguna de las dos tablas tiene organizacion_id, se acotan por usuario_id
-- (a través de factores_mfa) como todo lo demás de Identidad.
-- -----------------------------------------------------------------------------
REVOKE ALL ON factores_mfa, codigos_respaldo_mfa FROM PUBLIC;
GRANT SELECT, INSERT, UPDATE ON factores_mfa, codigos_respaldo_mfa TO rol_aplicacion;
-- Sin DELETE, mismo criterio que el resto del sistema (ADR 0017): un factor
-- deshabilitado transiciona a activo=false (Deshabilitar), no se borra; un
-- código de respaldo consumido queda marcado con usado_en, no se borra.
REVOKE DELETE, TRUNCATE ON factores_mfa, codigos_respaldo_mfa FROM rol_aplicacion;
