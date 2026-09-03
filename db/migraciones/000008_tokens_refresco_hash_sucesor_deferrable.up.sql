-- Corrige un bug bloqueante encontrado al verificar manualmente el flujo
-- de rotación de RenovarSesion (sección 3.2 del diseño de Acceso,
-- INV-ACC-05): con la FK tokens_refresco_hash_sucesor_fkey en su modo por
-- defecto (NOT DEFERRABLE, chequeo inmediato por sentencia), la secuencia
-- correcta que exige el índice único parcial
-- tokens_refresco_vigente_por_sesion_idx (000006) es imposible de
-- ejecutar dentro de una sola transacción:
--
--   1. UPDATE tokens_refresco SET consumido_en=..., hash_sucesor=<hash del
--      token nuevo> WHERE hash_token=<token viejo>  -- DEBE ir primero:
--      si el INSERT del paso 2 fuera primero, habría un instante con DOS
--      filas no consumidas para la misma sesion_id y el índice único
--      parcial fallaría de inmediato.
--   2. INSERT INTO tokens_refresco (...) -- el token nuevo.
--
-- Pero el paso 1 referencia en hash_sucesor un hash_token que TODAVÍA no
-- existe (se crea en el paso 2), y con la FK no-diferible eso falla de
-- inmediato con SQLSTATE 23503 ("insert or update on table
-- tokens_refresco violates foreign key constraint
-- tokens_refresco_hash_sucesor_fkey"). El índice único parcial NO puede
-- diferirse (no es un CONSTRAINT UNIQUE respaldado por índice, es un
-- CREATE UNIQUE INDEX directo — Postgres nunca difiere una violación de
-- índice), así que el único lado de la contradicción que puede ceder es
-- la FK: se recrea DEFERRABLE INITIALLY DEFERRED, de modo que su chequeo
-- se pospone al COMMIT de la transacción, momento en el que la fila
-- referenciada por hash_sucesor ya existe (fue insertada en el paso 2, en
-- la misma transacción). El orden consumir-luego-insertar sigue siendo
-- el correcto y el único válido frente al índice único parcial.
--
-- Ver internal/acceso/adaptadores/postgres/repositorio_sesiones.go
-- (comentario de cabecera de Guardar) para el código que depende de esto.

ALTER TABLE tokens_refresco DROP CONSTRAINT tokens_refresco_hash_sucesor_fkey;

ALTER TABLE tokens_refresco
    ADD CONSTRAINT tokens_refresco_hash_sucesor_fkey
    FOREIGN KEY (hash_sucesor) REFERENCES tokens_refresco(hash_token)
    DEFERRABLE INITIALLY DEFERRED;
