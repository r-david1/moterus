-- Contexto Confianza — reconocimiento de origen (§1.6 de
-- docs/design/fingerprinting-comportamiento.md).
--
-- Tercera migración de catálogo de auditoría de Confianza, tras 000018
-- (colas de acceso virtual). Es la ÚNICA acción que este mecanismo audita
-- (INV-RIES-13): las denegaciones por riesgo viajan como `Motivo` dentro de
-- usuario.login/denegado, tal como ADR 0018 ya fijó para los motivos de
-- rate limiting, y las evaluaciones no se auditan (volumen; mismo criterio
-- que INV-TEN-25 e INV-COLA-11).

INSERT INTO auditoria_acciones (accion, contexto, recurso, descripcion) VALUES
    ('origen.nuevo', 'confianza', 'origen',
     'Una cuenta autentico con exito desde una huella de dispositivo que no estaba en su perfil de origenes conocidos. Se registra una sola vez por par (cuenta, dispositivo) y solo si la cuenta ya tenia al menos un origen conocido.');

-- Sin sección de privilegios, y eso es deliberado, no un olvido de ADR 0017:
-- esta migración NO crea ninguna tabla. rol_aplicacion ya tiene SELECT sobre
-- auditoria_acciones e INSERT sobre auditoria desde 000002. Un GRANT acá
-- sería ruido. Sin RLS por el mismo motivo (y porque auditoria todavía no
-- tiene política de aislamiento por organización — nota final de 000002,
-- que este documento no reabre). Ver ADR 0050.
