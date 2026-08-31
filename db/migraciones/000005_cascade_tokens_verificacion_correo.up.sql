-- Cierra un hueco encontrado por tests-qa al escribir los tests de
-- integración de VerificarCorreo/ReenviarVerificacion (sección 3.4 del
-- diseño): tokens_verificacion_correo.usuario_id referenciaba usuarios(id)
-- sin ON DELETE CASCADE. Un Usuario nunca se borra físicamente en el flujo
-- de negocio real (el dominio solo transiciona EstadoUsuario, nunca hace
-- DELETE — ver ADR 0017, por eso rol_aplicacion no tiene DELETE sobre
-- usuarios), así que esto no afecta producción; pero sí rompía la limpieza
-- de fixtures de tests de integración (DELETE FROM usuarios con el rol
-- dueño, que sí puede borrar), dejando usuarios de prueba huérfanos cada
-- vez que un test no llegaba a consumir el token con éxito.
ALTER TABLE tokens_verificacion_correo
    DROP CONSTRAINT tokens_verificacion_correo_usuario_id_fkey,
    ADD CONSTRAINT tokens_verificacion_correo_usuario_id_fkey
        FOREIGN KEY (usuario_id) REFERENCES usuarios(id) ON DELETE CASCADE;
