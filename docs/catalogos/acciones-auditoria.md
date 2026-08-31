# Catálogo de acciones auditables

Fuente de verdad: tabla `auditoria_acciones`, sembrada en
`db/migraciones/000002_crear_auditoria.up.sql`. Este documento es la
versión legible para humanos de esa misma tabla — si difieren, la tabla
manda y este archivo está desactualizado.

Es un catálogo **cerrado** (INV-ID-17): una acción nueva exige una
migración que la agregue a `auditoria_acciones`, nunca un literal
inventado directamente en un caso de uso. La FK `auditoria.accion ->
auditoria_acciones.accion` lo hace cumplir a nivel de base de datos.

Formato: `recurso.accion`, snake_case (`auditoria_acciones_formato`).

## Contexto Identidad

| Acción | Recurso | Descripción | Evento de dominio | Valores de `resultado` |
|---|---|---|---|---|
| `usuario.registrado` | `usuario` | Alta de un usuario nuevo en estado `pendiente_verificacion`. | `UsuarioRegistrado` | `exito` |
| `usuario.registro_rechazado` | `usuario` | Intento de alta rechazado (Confianza, contraseña filtrada o correo duplicado). | `RegistroRechazado` | `denegado` / `fallo` |
| `usuario.login` | `usuario` | Intento de autenticación por credenciales. Una sola acción cubre los tres desenlaces posibles — se distinguen por `resultado`, no por acciones separadas. | `AutenticacionExitosa`, `AutenticacionFallida`, `AutenticacionDenegada` | `exito` / `fallo` / `denegado` |
| `usuario.step_up_requerido` | `usuario` | La autenticación exigió un segundo factor (MFA propio o step-up de Confianza). | `SegundoFactorRequerido` | `exito` |
| `usuario.contrasena_cambiada` | `usuario` | El usuario cambió su contraseña con la actual ya verificada. | `ContrasenaCambiada` | `exito` |
| `usuario.credencial_rehasheada` | `usuario` | Rehash oportunista del hash de contraseña tras un login exitoso (INV-ID-13). | `CredencialRehasheada` | `exito` |
| `usuario.correo_verificado` | `usuario` | Confirmación del correo: transición `pendiente_verificacion` → `activo` (éxito), o intento con token inválido/expirado/huérfano (fallo). | `CorreoVerificado`, `VerificacionCorreoFallida` | `exito` / `fallo` |
| `usuario.estado_cambiado` | `usuario` | Transición de `EstadoUsuario` sin evento propio: suspender, bloquear o reactivar. | `EstadoUsuarioCambiado` | `exito` |
| `usuario.consultado` | `usuario` | Un tercero (no el propio usuario) consultó el perfil de un usuario. Consultar el perfil propio no se audita. | `UsuarioConsultado` | `exito` |

12 eventos de dominio (`internal/identidad/dominio/eventos.go`) mapean a
estas 9 acciones — `usuario.login` absorbe los tres eventos de intento de
autenticación porque son la misma acción de negocio con distinto
desenlace, no acciones distintas.

## Agregar una acción nueva

1. Migración nueva (`NNNNNN_agregar_accion_x.up/down.sql`) que haga
   `INSERT INTO auditoria_acciones (...)`. Nunca editar in-place una fila
   ya publicada: el catálogo es append-only igual que la bitácora misma.
2. Agregar la fila a la tabla de arriba.
3. Si la acción corresponde a un evento de dominio nuevo, agregarlo
   primero en `dominio/eventos.go` y en la tabla de eventos de
   `docs/design/identidad-bounded-context.md` (sección 1).

## Otros contextos

Sin acciones propias todavía — se agregan aquí a medida que Tenencia,
Acceso y Confianza empiecen a emitir auditoría.

**Acceso (propuestas, todavía NO en la tabla `auditoria_acciones`):** el
diseño del contexto (`docs/design/acceso-bounded-context.md`, secciones 1.6
y 6) prevé seis acciones — `sesion.iniciada`, `sesion.renovada`,
`sesion.reuso_refresco_detectado`, `sesion.cerrada`, `sesion.revocada` y
`token_acceso.rechazado` — que se publicarán aquí junto con la migración
`000007_acciones_auditoria_acceso`, siguiendo el procedimiento de arriba.
Hasta que esa migración exista, no son válidas: la FK las rechazaría.
