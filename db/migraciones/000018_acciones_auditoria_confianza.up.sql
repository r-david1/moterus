-- Contexto Confianza — tres acciones nuevas en el catálogo cerrado
-- `auditoria_acciones` (§1.7 y §6.1 de docs/design/colas-virtuales.md).
-- Formato exigido por `auditoria_acciones_formato` (^[a-z_]+\.[a-z_]+$) —
-- las tres cumplen.
--
-- Migración separada de 000017 (crea la tabla salas_espera), mismo criterio
-- ya fijado por 000007 (acciones de Acceso, separada de 000006), 000012
-- (acciones de Tenencia) y 000016 (acciones de MFA, separada de 000015): el
-- catálogo de auditoría no depende de las tablas de dominio del propio
-- contexto (la FK real es `auditoria.accion -> auditoria_acciones.accion`,
-- no hacia `salas_espera`), así que no hay razón para acoplar ambas
-- migraciones.
--
-- Son las PRIMERAS filas de auditoría del contexto Confianza. La columna
-- `auditoria_acciones.contexto` (000002) es TEXT NOT NULL sin CHECK: el
-- catálogo de contextos no es cerrado a nivel de esquema, así que
-- 'confianza' se inserta sin necesidad de extender ninguna restricción
-- existente — se verificó explícitamente que 000002 no define un
-- `auditoria_acciones_contexto_valido` ni equivalente antes de asumirlo.
--
-- Ver docs/catalogos/acciones-auditoria.md, sección "Confianza", para la
-- versión legible de esta misma tabla.

INSERT INTO auditoria_acciones (accion, contexto, recurso, descripcion) VALUES
    ('sala_espera.abierta',        'confianza', 'sala_espera', 'Se abrio una cola de acceso virtual sobre una ruta protegida, con su ritmo de admision y capacidad iniciales.'),
    ('sala_espera.ritmo_cambiado', 'confianza', 'sala_espera', 'Se cambio el ritmo de admision de una sala abierta durante el evento; incluye el cursor y la longitud de cola al momento del cambio.'),
    ('sala_espera.cerrada',        'confianza', 'sala_espera', 'La sala paso a drenando o cerrada; incluye los totales de ingresos y admitidos del evento.');
