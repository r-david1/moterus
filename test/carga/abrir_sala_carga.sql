-- Abre, directamente en Postgres, la sala de espera de alcance "sistema"
-- que usa test/carga/colas_virtuales_test.js.
--
-- Por qué directo en SQL y no por HTTP: las tres rutas del catálogo cerrado
-- (acceso.iniciar_sesion, identidad.registrar_usuario,
-- tenencia.aceptar_invitacion) solo admiten AlcanceSistema
-- (RutaProtegida.AdmiteAlcance, internal/confianza/dominio/alcance_sala.go),
-- y el único endpoint HTTP de administración
-- (POST /confianza/organizaciones/{id}/salas-espera) solo abre salas
-- org-scoped. Abrir una sala de alcance sistema es, por diseño, una
-- operación fuera de la API pública (ADR 0045: este producto no tiene rol
-- de administrador de plataforma) — se opera por SQL/CLI, no por HTTP.
--
-- Uso:
--   docker exec -i auth-service-postgres psql -U auth_service -d auth_service \
--     < test/carga/abrir_sala_carga.sql
--
-- Después de correr esto, el servidor (`make run`) tarda hasta 15s en
-- notar la sala nueva: ReconciliarSalas relee salas_espera cada 15s y
-- reemplaza la instantánea en memoria que lee el camino caliente
-- (INV-COLA-08: PorteroDeSala nunca toca Postgres directamente). Esperar
-- ese ciclo antes de lanzar k6.
--
-- reloj_desde = now() + 20s, NO now(): el cursor de admisión
-- (cursor(t) = cursor_base + (t - reloj_desde)·ritmo, §1.5 del diseño)
-- corre en tiempo real desde el instante en que la sala queda "abierta",
-- sin importar si alguien pide un ticket o no. Si se fija reloj_desde=now()
-- y después se espera ~20s a que el reconciliador la note antes de lanzar
-- k6, esos ~20s de espera ya "preadmitieron" ~20×ritmo posiciones fantasma
-- que nadie reclamó — el primer ticket real llega con el cursor muy por
-- delante de la posición 0 y se admite de inmediato junto con cientos más,
-- exactamente el falso positivo que midió analizar_admision.sh la primera
-- vez que se corrió esta prueba (314 admisiones en el mismo segundo,
-- contra un ritmo de 50/s). Adelantar reloj_desde 20s compensa la espera:
-- el cursor llega a 0 justo cuando el tráfico real empieza, no antes.
--
-- El alcance "sistema" no tiene organización dueña ni discrimina por ella:
-- el estado de la cola en Redis se indexa por ClaveSala = "sistema:<ruta>",
-- no por alias ni por id de esta fila. Si ya corriste este script antes,
-- limpiá también Redis (ver el comentario de cabecera de
-- colas_virtuales_test.js) para no arrancar con un cursor/cola heredados
-- de una corrida anterior.
INSERT INTO salas_espera (
    id, alias, alcance_tipo, alcance_organizacion_id, ruta_protegida, estado,
    ritmo_admision, capacidad_maxima_cola, ventana_reclamo_ms, modo_degradado,
    cursor_base, reloj_desde, version_config, creada_por, creada_en, actualizada_en, abierta_en
) VALUES (
    gen_random_uuid(), 'carga-k6-login', 'sistema', NULL, 'acceso.iniciar_sesion', 'abierta',
    50, 6000, 60000, 'permitir',
    0, now() + interval '20 seconds', 1, NULL, now(), now(), now()
);
