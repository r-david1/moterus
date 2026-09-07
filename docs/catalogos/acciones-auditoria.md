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

### MFA/OTP (docs/design/otp-mfa.md)

| Acción | Recurso | Descripción | Evento de dominio | Valores de `resultado` |
|---|---|---|---|---|
| `usuario.mfa_habilitado` | `usuario` | Generación de un secreto TOTP nuevo y creación del `FactorMFA` sin confirmar todavía. | `FactorMFAHabilitado` | `exito` |
| `usuario.mfa_confirmado` | `usuario` | Primer código TOTP correcto: el `FactorMFA` queda confirmado y `Usuario.tieneMFA` pasa a `true` (INV-ID-08/INV-MFA-01). | `FactorMFAConfirmado` | `exito` |
| `usuario.mfa_deshabilitado` | `usuario` | El usuario desactivó su segundo factor, tras demostrar posesión con un código propio (ADR 0039). | `FactorMFADeshabilitado` | `exito` |
| `usuario.codigo_respaldo_consumido` | `usuario` | Un código de respaldo de un solo uso se consumió con éxito durante la verificación OTP; permite que el usuario vea en su historial cuántos le quedan. | `CodigoRespaldoConsumido` | `exito` |
| `usuario.otp_verificacion_fallida` | `usuario` | Un código presentado en el flujo de step-up no coincidió ni con el TOTP esperado ni con ningún código de respaldo disponible — señal de fuerza bruta sobre el segundo factor (mismo criterio que `autorizacion.denegada` de Tenencia). | `VerificacionOTPFallida` | `fallo` |

5 eventos de dominio (`internal/identidad/dominio/eventos.go`) mapean 1:1 a
estas 5 acciones. Van dentro de "Contexto Identidad" (no una sección
propia) porque MFA/OTP es una extensión de Identidad, no un bounded
context nuevo (§0.1 de `docs/design/otp-mfa.md`). Sembradas por la
migración `db/migraciones/000016_acciones_auditoria_mfa.up.sql`, separada
de `000015_crear_factores_mfa.up.sql` (que solo crea las tablas) siguiendo
el mismo criterio que ya usaron Acceso (000006/000007) y Tenencia
(000009/000012): el catálogo de acciones no depende de las tablas de
dominio del propio contexto.

## Agregar una acción nueva

1. Migración nueva (`NNNNNN_agregar_accion_x.up/down.sql`) que haga
   `INSERT INTO auditoria_acciones (...)`. Nunca editar in-place una fila
   ya publicada: el catálogo es append-only igual que la bitácora misma.
2. Agregar la fila a la tabla de arriba.
3. Si la acción corresponde a un evento de dominio nuevo, agregarlo
   primero en `dominio/eventos.go` y en la tabla de eventos de
   `docs/design/identidad-bounded-context.md` (sección 1).

## Contexto Acceso

| Acción | Recurso | Descripción | Evento de dominio | Valores de `resultado` |
|---|---|---|---|---|
| `sesion.iniciada` | `sesion` | Emisión de una sesión nueva tras autenticación exitosa en Identidad. | `SesionIniciada` | `exito` |
| `sesion.renovada` | `sesion` | Rotación del token de refresco: éxito, o fallo por refresco inválido/expirado/sesión no renovable. | `SesionRenovada`, `RenovacionRechazada` | `exito` / `fallo` |
| `sesion.reuso_refresco_detectado` | `sesion` | Se presentó un token de refresco ya consumido: robo probable; revoca la sesión completa. | `ReusoRefrescoDetectado` | `denegado` |
| `sesion.cerrada` | `sesion` | Cierre de sesión iniciado por el propio usuario (individual o de todos los dispositivos). | `SesionCerrada` | `exito` |
| `sesion.revocada` | `sesion` | Revocación NO iniciada por el usuario: cuenta no operativa, cambio de contraseña, límite de sesiones o decisión administrativa. | `SesionRevocada` | `exito` |
| `token_acceso.rechazado` | `token_acceso` | Rechazo de un token de acceso con valor de señal: firma inválida, `kid`/`alg`/`typ` inesperado o sesión en lista de revocación. **Nunca** para token simplemente expirado (INV-ACC-17). | `TokenAccesoRechazado` | `denegado` |

7 eventos de dominio (`internal/acceso/dominio/eventos.go`, pendiente de
implementación) mapean a estas 6 acciones — `sesion.renovada` absorbe tanto
la renovación exitosa como la rechazada, mismo criterio que
`usuario.login` en Identidad. Sembradas por la migración
`db/migraciones/000007_acciones_auditoria_acceso.up.sql`, siguiendo el
procedimiento de "Agregar una acción nueva" de arriba. Ver
`docs/design/acceso-bounded-context.md`, secciones 1.6 y 6.

## Contexto Tenencia

| Acción | Recurso | Descripción | Evento de dominio | Valores de `resultado` |
|---|---|---|---|---|
| `organizacion.creada` | `organizacion` | Alta de una organización nueva, junto con la membresía propietario de su fundador. | `OrganizacionCreada` | `exito` |
| `organizacion.actualizada` | `organizacion` | Cambio de nombre o alias de una organización. | `OrganizacionActualizada` | `exito` |
| `organizacion.estado_cambiado` | `organizacion` | Transición de `EstadoOrganizacion`: suspender, reactivar o archivar. | `EstadoOrganizacionCambiado` | `exito` |
| `membresia.creada` | `membresia` | Alta de una membresía: fundación de la organización, alta directa o aceptación de invitación. | `MiembroAgregado` | `exito` |
| `membresia.rol_cambiado` | `membresia` | Cambio de rol de un miembro, incluida la transferencia de propiedad. | `RolDeMiembroCambiado` | `exito` |
| `membresia.estado_cambiado` | `membresia` | Suspensión o reactivación de una membresía sin removerla. | `EstadoMembresiaCambiado` | `exito` |
| `membresia.removida` | `membresia` | Remoción de un miembro, por decisión de un administrador o por iniciativa propia. | `MiembroRemovido` | `exito` |
| `membresia.invitada` | `invitacion` | Emisión de una invitación por correo con un rol propuesto. **Nunca** el token ni su hash. | `MiembroInvitado` | `exito` |
| `membresia.invitacion_resuelta` | `invitacion` | Desenlace de una invitación: aceptada, revocada, expirada o intento fallido de redención — distinguidas por `detalles.desenlace` y `resultado`, mismo criterio que `usuario.login` y `sesion.renovada`. | `InvitacionResuelta` | `exito` / `fallo` |
| `autorizacion.denegada` | `autorizacion` | Denegación de una autorización: sin membresía, membresía suspendida, organización no operativa o rol insuficiente. Las concesiones **no** se auditan (INV-TEN-25). | `AutorizacionDenegada` | `denegado` |

10 eventos de dominio (`internal/tenencia/dominio/eventos.go`) mapean 1:1 a
estas 10 acciones. Sembradas por la migración
`db/migraciones/000012_acciones_auditoria_tenencia.up.sql`, siguiendo el
procedimiento de "Agregar una acción nueva" de arriba. Ver
`docs/design/tenencia-bounded-context.md`, secciones 1.6 y 6.4.

## Contexto Confianza

| Acción | Recurso | Descripción | Evento de dominio | Valores de `resultado` |
|---|---|---|---|---|
| `sala_espera.abierta` | `sala_espera` | Se abrió una cola de acceso virtual sobre una ruta protegida, con su ritmo de admisión y capacidad iniciales. | `SalaDeEsperaAbierta` | `exito` |
| `sala_espera.ritmo_cambiado` | `sala_espera` | Se cambió el ritmo de admisión de una sala abierta durante el evento; incluye el cursor y la longitud de cola al momento del cambio — el dato que un post-mortem del evento va a pedir primero. | `RitmoDeAdmisionCambiado` | `exito` |
| `sala_espera.cerrada` | `sala_espera` | La sala pasó a `drenando` o `cerrada` (distinguidas por `detalles.destino`); incluye los totales de ingresos y admitidos del evento. | `SalaDeEsperaCerrada` | `exito` |
| `origen.nuevo` | `origen` | Una cuenta autenticó con éxito desde una huella de dispositivo que no estaba en su perfil de orígenes conocidos. Se registra una sola vez por par (cuenta, dispositivo) y solo si la cuenta ya tenía al menos un origen conocido. | `OrigenNuevoObservado` | `exito` |

3 eventos de dominio (`internal/confianza/dominio/eventos.go`) mapean 1:1 a
las 3 acciones de colas de acceso virtual. Sembradas por la migración
`db/migraciones/000018_acciones_auditoria_confianza.up.sql`, separada de
`000017_crear_salas_espera.up.sql` (que solo crea la tabla) siguiendo el
mismo criterio que ya usaron Acceso (000006/000007), Tenencia
(000009/000012) y MFA/OTP (000015/000016). Son las primeras filas de
auditoría del contexto Confianza (§1.7 de `docs/design/colas-virtuales.md`):
ni los ingresos, ni las consultas de turno, ni los reclamos se auditan
(INV-COLA-11, mismo razonamiento que INV-TEN-25) — solo las mutaciones del
ciclo de vida de una sala (abrir, cambiar ritmo, drenar, cerrar). Ver
`docs/design/colas-virtuales.md`, secciones 1.7 y 6.1.

`origen.nuevo` es la ÚNICA acción del reconocimiento de origen
(INV-RIES-13, tercera extensión de Confianza): sembrada por
`db/migraciones/000019_acciones_auditoria_riesgo.up.sql`, sin tabla ni
`GRANT` propios (no crea nada nuevo que privilegiar, ver ADR 0050). Las
denegaciones por riesgo de origen viajan como `Motivo` dentro de
`usuario.login`/`denegado`, tal como ADR 0018 ya fijó para los motivos de
rate limiting; las evaluaciones no se auditan por volumen. Ver
`docs/design/fingerprinting-comportamiento.md`, secciones 1.6 y 5.4.

## Otros contextos

Sin acciones propias todavía.
