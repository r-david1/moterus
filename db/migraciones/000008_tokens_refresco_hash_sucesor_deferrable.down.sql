-- Reversión de 000008: vuelve la FK a su forma original NOT DEFERRABLE.
-- Nota: revertir esta migración reintroduce el bug bloqueante documentado
-- en 000008_tokens_refresco_hash_sucesor_deferrable.up.sql — RenovarSesion
-- vuelve a fallar con SQLSTATE 23503 en la primera rotación. Se provee de
-- todas formas por simetría con el resto de migraciones del proyecto.

ALTER TABLE tokens_refresco DROP CONSTRAINT tokens_refresco_hash_sucesor_fkey;

ALTER TABLE tokens_refresco
    ADD CONSTRAINT tokens_refresco_hash_sucesor_fkey
    FOREIGN KEY (hash_sucesor) REFERENCES tokens_refresco(hash_token);
