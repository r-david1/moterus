-- Reversión de 000017_crear_salas_espera. Aceptable en desarrollo y CI; en
-- producción, revisar si hay salas abiertas/drenando antes de bajarla (se
-- perdería la evidencia de configuración de eventos operativos pasados).

REVOKE SELECT, INSERT, UPDATE ON salas_espera FROM rol_aplicacion;

DROP TABLE IF EXISTS salas_espera;
