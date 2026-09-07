-- Reversión de 000019_acciones_auditoria_riesgo.
--
-- Fallará por la FK de auditoria.accion si ya hay filas en auditoria con
-- esta acción, y eso es correcto: la bitácora es append-only (ADR 0005) y
-- una migración hacia atrás no puede destruir evidencia. Mismo
-- comportamiento que el down de 000018.

DELETE FROM auditoria_acciones WHERE accion = 'origen.nuevo';
