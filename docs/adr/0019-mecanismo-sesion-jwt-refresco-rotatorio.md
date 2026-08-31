# ADR 0019 — Mecanismo de sesión: JWT de acceso de vida corta + token de refresco opaco rotatorio, con la sesión autoritativa en Postgres

## Contexto

ADR 0009 fijó la frontera: Identidad autentica credenciales y nunca emite token ni sesión; el contexto **Acceso** es responsable de emitir las credenciales de sesión. Lo que ADR 0009 dejó abierto es *qué* emite Acceso exactamente. Antes de escribir una sola línea de `internal/acceso/` (hoy es solo un esqueleto de `doc.go`) hay que decidir el mecanismo, porque de él dependen las tablas, los puertos, las invariantes y la mitad de los endpoints del contexto.

La disyuntiva clásica:

- **JWT stateless puro**: el servidor no guarda nada; validar es verificar una firma. Escala horizontalmente sin estado compartido y permite que otros servicios validen sin llamar a Acceso. A cambio, **no hay revocación inmediata**: un token robado sirve hasta que expira, salvo que se agregue una lista negra — que es estado, es decir, precisamente lo que se quería evitar.
- **Sesión opaca en servidor**: un identificador aleatorio contra una fila en Postgres o Redis. Revocación instantánea y exacta, cero criptografía que configurar mal, cero riesgo de *algorithm confusion*. A cambio, **cada petición pega a la base de datos o al cache**, y cualquier verificador externo necesita un endpoint de introspección, con Acceso convertido en dependencia dura de cada request de cada servicio.

Contexto específico de este proyecto que inclina la balanza:

1. **ADR 0009 ya comprometió la dirección** en sus Consecuencias: *"El contexto Acceso, cuando se diseñe, es responsable de: emisión/firma JWT (`RS256`/`EdDSA`, JWKS con rotación de `kid`), refresh tokens, y la decisión final de qué devolver al cliente"*. Reabrirlo para elegir sesión opaca contradiría un ADR aceptado.
2. **ADR 0002** pausó el multi-producto pero no lo descartó, y el nombre del proyecto (Auth-as-a-Service) anticipa que otros servicios verifiquen credenciales sin llamar a Acceso. Con sesión opaca, eso obliga a un endpoint de introspección en el camino crítico de todo el ecosistema.
3. **Contraargumento honesto**: hoy esto es un monolito con un solo Postgres. Una consulta por PK por request cuesta ~0,2 ms y daría revocación exacta sin JWKS, sin rotación de llaves, sin lista negra y sin la familia entera de vulnerabilidades de JWT mal validado. A la escala actual (ADR 0002: un solo producto, sin tráfico de producción) el argumento de rendimiento a favor de JWT es débil, y conviene decirlo en vez de repetir el eslogan de que "JWT escala mejor".
4. **ADR 0005** exige bitácora forense hash-chained, y la cadena está serializada por un `pg_advisory_xact_lock`. Cualquier diseño que audite en el camino caliente (una fila por validación de token) convierte el mecanismo forense en el cuello de botella global.
5. **ADR 0018** trajo Redis al stack, pero como componente **opcional**: sin `REDIS_URL` el servicio arranca igual, con `EvaluadorConfianzaNoOp`. Ningún dato cuya pérdida signifique "todos los usuarios deslogueados" puede vivir solo ahí.

## Decisión

Se adopta un esquema **híbrido**, con tres piezas de responsabilidad separada:

### 1. La sesión es autoritativa y vive en **Postgres** (tabla `sesiones`)

La sesión es la unidad de negocio y de revocación: un agregado `Sesion` con estado (`activa`/`revocada`/`expirada`), ventana de inactividad, vida absoluta y el `OrigenSolicitud` de su creación. Postgres y no Redis, porque su pérdida equivale a desloguear a todo el mundo, porque tiene FK a `usuarios(id)`, y porque su valor forense (correlacionable con la tabla `auditoria` por `id_solicitud` y `usuario_id`) es justamente lo que ADR 0005 persigue.

### 2. El **token de acceso** es un JWT firmado, de vida corta (10 minutos)

Sin estado del lado del servidor y sin consulta a base de datos en el camino feliz. Firma **asimétrica** (llave privada solo en Acceso, públicas por JWKS en `/.well-known/jwks.json`); nunca HMAC con secreto compartido, porque un secreto compartido convierte a cualquier verificador en emisor y eso es incompatible con el alcance multi-producto que ADR 0002 dejó abierto. La elección concreta entre EdDSA/Ed25519 y RS256, y el mecanismo de rotación de llaves, quedan para un ADR propio (candidato 0020): son parametrizaciones de esta decisión, no la decisión.

Claims mínimos: `iss`, `sub`, `aud` (fijo, ADR 0002), `exp`/`iat`/`nbf`, `jti` (UUIDv4), `sid` (el `IDSesion`, clave de revocación), `amr`, `auth_time`, `ver`. **Sin** roles, permisos, `org_id`, `tenant_id`, correo, IP ni huella de dispositivo.

### 3. El **token de refresco** es opaco, rotatorio y de un solo uso

Secreto aleatorio de ≥32 bytes (`crypto/rand`), prefijado `mot_rt_`, del que **solo se persiste el hash SHA-256** — mismo patrón y misma justificación que los tokens de verificación de correo de Identidad (`internal/identidad/aplicacion/tokens_verificacion.go`): es un secreto de alta entropía generado por el sistema, no hay ataque de diccionario que Argon2id pueda encarecer, así que pagar Argon2id en cada renovación sería costo puro.

**Rotación obligatoria en cada uso**, con detección de reuso: presentar un token ya consumido revoca la sesión completa de inmediato y se audita como `sesion.reuso_refresco_detectado / denegado`. El cliente recibe exactamente el mismo error que ante un token desconocido. Es la contramedida estándar (OAuth 2.1 / RFC 6819) contra el robo de refresco: si el atacante lo usa, la víctima queda fuera en su siguiente renovación; si la víctima lo usa primero, el atacante queda fuera. En los dos casos alguien pierde el acceso robado y queda constancia forense.

### 4. La revocación tiene una garantía acotada y explícita

- **Refresco: revocación exacta e inmediata.** La sesión en Postgres es la autoridad; una sesión revocada no vuelve a renovar nunca.
- **Token de acceso: revocación efectiva en ≤ 10 minutos, siempre.** Con Redis disponible, es inmediata: al revocar se publica el `sid` en una lista de revocación con TTL igual a la vida restante del token, y el validador la consulta. **Esa lista es un acelerador, no la frontera de seguridad** (candidato 0023): si Redis no está disponible, la revocación sigue siendo correcta y solo se degrada al peor caso conocido — que es exactamente la garantía que daría un JWT puro, nunca peor.
- **Escape hatch para operaciones de alto valor**: `ValidarAcceso` acepta `ExigirSesionViva bool`, que añade una verificación contra `sesiones`. Se usa donde 10 minutos de ventana no son aceptables (cambio de contraseña, cierre masivo, futuras operaciones administrativas) y se paga solo ahí. Esto conserva lo mejor de la sesión opaca sin imponer su costo en todas las peticiones.
- **Cambio de estado de la cuenta**: cada renovación revalida el estado del sujeto contra Identidad por puerto (`ConsultorDeUsuarios`, nunca un `JOIN` a `usuarios`). Es lo que hace que una suspensión surta efecto **sin depender de un broker de eventos**, que hoy no existe (`identidad/adaptadores/eventos/publicador.go` es log-only). El mecanismo completo de propagación queda en el candidato 0022.

### 5. Se audita la sesión, no la validación

Emisión, renovación (éxito y fallo), reuso, cierre y revocación se auditan en la misma `UnidadDeTrabajo` que la escritura de negocio (ADR 0005). La validación de tokens **no** se audita, salvo los rechazos con valor de señal (firma inválida, `kid`/`alg`/`typ` inesperado, sesión revocada). Un 401 por token expirado es el evento más frecuente del sistema y no es una señal de seguridad; auditarlo pondría el hot path detrás del advisory lock de la cadena de hashes y ahogaría la señal real en ruido.

## Alternativas consideradas

- **Sesión opaca pura en Postgres (sin JWT)**: la alternativa más fuerte, y la que mejor encaja con la escala *actual* del proyecto. Descartada por tres razones acumuladas: contradice las Consecuencias ya aceptadas de ADR 0009; obliga a un endpoint de introspección en el camino crítico de cualquier consumidor futuro (ADR 0002 dejó esa puerta abierta a propósito); y convierte cada request autenticada del sistema en una consulta a la misma base de datos que ya soporta el resto del dominio. El diseño adoptado conserva su mejor propiedad —la sesión server-side autoritativa— y solo hace stateless la credencial de 10 minutos.
- **Sesión opaca en Redis**: descartada. Redis es un componente **opcional** del despliegue (ADR 0018: sin `REDIS_URL` el servicio arranca igual) y su pérdida significaría desloguear a todos los usuarios. Un dato cuya pérdida tiene ese impacto no puede vivir en la capa de cache.
- **JWT stateless puro sin refresco server-side**: descartado. Sin registro de sesión no hay logout real, ni "cerrar sesión en todos los dispositivos", ni pantalla de dispositivos conectados, ni detección de robo de refresco, ni rastro forense de qué sesiones existieron — todo eso es requisito explícito del contexto.
- **Tokens de refresco de larga vida reutilizables (sin rotación)**: descartado. Un refresco reutilizable robado da acceso indefinido y **silencioso**: no hay ninguna señal que permita detectarlo. La rotación cuesta un `UPDATE` extra por renovación (una cada 10 minutos por usuario) y a cambio convierte el robo en un evento detectable y auditable.
- **HMAC (HS256) con secreto compartido**: descartado por INV-ACC-13. Distribuir el secreto a los verificadores los convierte en emisores; con firma asimétrica, un verificador comprometido no puede fabricar tokens.
- **Guardar el refresco en claro para poder mostrárselo al usuario**: descartado sin discusión — mismo criterio que las contraseñas y que los tokens de verificación de correo.

## Consecuencias

- Aparecen dos tablas nuevas (`sesiones`, `tokens_refresco`) que, por ADR 0017, **necesitan `GRANT` explícito a `rol_aplicacion` en su propia migración**: crearlas no alcanza. Sin `DELETE`, deliberadamente: las sesiones transicionan de estado y su cadena de tokens consumidos es la evidencia que hace posible la detección de reuso.
- Seis acciones nuevas en el catálogo cerrado `auditoria_acciones` (`sesion.iniciada`, `sesion.renovada`, `sesion.reuso_refresco_detectado`, `sesion.cerrada`, `sesion.revocada`, `token_acceso.rechazado`), cada una con su migración, según el procedimiento de `docs/catalogos/acciones-auditoria.md`.
- El proyecto adquiere una dependencia de biblioteca JWT que hoy no tiene en `go.mod` (candidato 0028), y con ella toda la familia de errores clásicos de validación de JWT. Se mitigan con invariantes verificables en tests: rechazo de `alg: none`, algoritmo derivado del `kid` y nunca de la cabecera del token, `typ: at+jwt` obligatorio, `iss`/`aud` verificados siempre.
- El proceso `api` no puede arrancar en `APP_ENV=production` sin llave de firma configurada. A diferencia del captcha de ADR 0018, aquí no hay modo degradado sensato: un servicio de autenticación que no puede firmar tokens no tiene nada que hacer sirviendo tráfico. En desarrollo se genera una llave efímera en memoria con `WARN` explícito de que todos los tokens mueren al reiniciar.
- Existe una ventana de hasta 10 minutos entre revocar una sesión y que su último token de acceso deje de funcionar, cuando Redis no está disponible. Es una propiedad conocida, acotada y documentada del sistema, no un defecto latente: cualquier checklist de "listo para producción" debe incluir `REDIS_URL`, igual que ya incluye `DATABASE_URL_APLICACION` (ADR 0017) y `TURNSTILE_SECRET_KEY` (ADR 0018).
- El middleware que consume `ValidadorDeAccesos` es lo que cierra el hueco documentado en `identidad/adaptadores/http/rutas.go`, donde `GET /identidad/usuarios/{id}` está público "como placeholder hasta que exista el middleware de autenticación de Acceso".

## Estado

Aceptado. Diseño completo en `docs/design/acceso-bounded-context.md`. Implementación no iniciada: `internal/acceso/` es hoy solo un esqueleto de carpetas con `doc.go` de placeholder. Los parámetros que esta decisión deja abiertos a propósito están reservados como ADR candidatos 0020–0028 en la sección 9 de ese documento.
