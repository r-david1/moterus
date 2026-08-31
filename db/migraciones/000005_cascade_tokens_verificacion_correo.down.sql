ALTER TABLE tokens_verificacion_correo
    DROP CONSTRAINT tokens_verificacion_correo_usuario_id_fkey,
    ADD CONSTRAINT tokens_verificacion_correo_usuario_id_fkey
        FOREIGN KEY (usuario_id) REFERENCES usuarios(id);
