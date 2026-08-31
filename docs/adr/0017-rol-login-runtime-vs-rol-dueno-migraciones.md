# ADR 0017 — El proceso `api` corre con un rol de login de privilegios acotados, distinto del rol dueño usado para migraciones

## Contexto

Al implementar los adaptadores de infraestructura de Identidad se detectó que el servicio se conectaba a Postgres con el mismo rol que corre las migraciones (`auth_service`), que en el `docker-compose` de desarrollo es el rol dueño de la base — superusuario. ADR 0005 depende de `REVOKE UPDATE, DELETE ON auditoria FROM rol_aplicacion` para garantizar que la bitácora es append-only; **un superusuario ignora los permisos de tabla**, así que corriendo así esa garantía no protegía nada en la práctica: cualquier código que corriera con las credenciales del proceso `api` (o una inyección SQL exitosa contra él) podía alterar o borrar la bitácora sin que ningún control de BD lo impidiera.

Adicionalmente, `rol_aplicacion` (creado en la migración 000002) nunca recibió ningún `GRANT` sobre la tabla `usuarios` — la migración 000001 no pudo otorgárselo porque el rol todavía no existía en ese punto, y nadie lo cerró después.

## Decisión

Migración `000003_rol_login_aplicacion`:

1. Otorga a `rol_aplicacion` los privilegios que le faltaban sobre `usuarios`: `SELECT, INSERT, UPDATE` — sin `DELETE`, porque el dominio nunca borra un `Usuario` físicamente, solo transiciona su `EstadoUsuario`.
2. Crea `rol_login_identidad`: un rol **con** `LOGIN`, sin privilegios propios, que hereda los de `rol_aplicacion` por membresía (`GRANT rol_aplicacion TO rol_login_identidad`).
3. El proceso `api` se conecta con `rol_login_identidad`, nunca con el rol dueño de la base. Las migraciones (`cmd/migrador`) siguen usando el rol dueño (`DATABASE_URL`), porque `CREATE TABLE`/`GRANT`/`CREATE ROLE` exigen esos privilegios.

En código: `configuracion.Config` distingue `URLBaseDeDatos` (DDL, migraciones) de `URLBaseDeDatosAplicacion` (runtime del proceso `api`, env `DATABASE_URL_APLICACION`). Si `DATABASE_URL_APLICACION` no está definida, `cmd/api` cae a `URLBaseDeDatos` pero emite un `WARN` explícito en el arranque — mismo patrón que el `WARN` de `EvaluadorConfianza` no-op: el modo inseguro es posible (no rompe el arranque en dev) pero nunca silencioso.

Verificado empíricamente: con `rol_login_identidad`, `INSERT`/`SELECT`/`UPDATE` sobre `usuarios` y `SELECT`/`INSERT` sobre `auditoria` funcionan; `UPDATE`/`DELETE` sobre `auditoria` fallan con `permission denied` (no solo con el error del trigger — el permiso se niega antes de que el trigger llegue a evaluarse).

## Alternativas consideradas

- **`SET ROLE rol_aplicacion` dentro de cada conexión, conectando siempre como el rol dueño**: descartado — `SET ROLE` es reversible por cualquier código con acceso a la conexión (basta un `RESET ROLE` o una nueva conexión del pool sin el `SET`), y depende de que *todo* el código lo ejecute sin excepción antes de la primera query. Un rol de login separado hace que la restricción sea estructural (viene del `pg_hba`/credenciales de conexión, no de disciplina en cada query) y no puede "olvidarse".
- **Dejar `usuarios` sin `GRANT` explícito y confiar en que el owner (`auth_service`) siga siendo quien corre todo**: es lo que había — descartado precisamente porque es la causa raíz de este ADR.
- **Row-Level Security en vez de separación de roles**: no aplica aquí — RLS resuelve aislamiento *entre* filas para el mismo rol (multi-tenant, ADR pendiente de Tenencia); este problema es de privilegios *entre* roles distintos (aplicación vs. administración), un caso de uso distinto.

## Consecuencias

- Cualquier adaptador nuevo que necesite una tabla nueva debe recordar otorgarle privilegios explícitos a `rol_aplicacion` en la misma migración que crea la tabla — ya no alcanza con crear la tabla y asumir que "ya se va a poder usar".
- La contraseña de `rol_login_identidad` en la migración (`identidad_app_dev_password`) es un valor de desarrollo fijo, igual que `auth_service_dev_password` en `docker-compose.yml` — en un despliegue real se reemplaza vía secret manager, no vive en el archivo de migración versionado.
- El WARN de fallback en `cmd/api` es deuda intencional: aceptable en desarrollo local, pero cualquier checklist de "listo para producción" debe verificar que `DATABASE_URL_APLICACION` esté definida y que el rol de conexión NO sea el dueño de la base.

## Estado

Aceptado e implementado. Verificado contra Postgres real: registro, login (éxito/fallo/denegado) y consulta funcionando end-to-end con `rol_login_identidad`, cadena de auditoría íntegra tras las pruebas.
