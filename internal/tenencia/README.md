# Bounded Context: Tenencia

> Este README es la puerta de entrada para cualquier equipo (frontend u otro
> servicio backend) que necesite **consumir** este contexto por HTTP hoy.
> Para el diseño completo (agregados, invariantes, puertos, casos de uso),
> ver `docs/design/tenencia-bounded-context.md` — su cabecera ya está
> actualizada a "implementada"; este README es la referencia operativa que
> la complementa, no la reemplaza. Para las decisiones de arquitectura no
> obvias citadas aquí, ver `docs/adr/0029-modelo-roles-catalogo-cerrado.md`,
> `docs/adr/0030-autorizacion-por-consulta-en-cada-peticion.md` y
> `docs/adr/0031-rls-multi-tenant-guc-por-transaccion.md`.

## Responsabilidad

Tenencia responde una sola pregunta: **¿qué puede hacer este sujeto, y
dentro de qué organización?** Alta y ciclo de vida de organizaciones (los
*tenants* de ADR 0002), membresías (qué usuario pertenece a qué
organización y con qué rol de un catálogo cerrado), la decisión de
autorización en sí misma, e invitaciones por correo con su redención.

Tenencia **no** hace (y por diseño no debe hacerse aquí):

- Verificar quién es el sujeto o si puede probarlo — eso es **Identidad**
  (ADR 0009). Tenencia recibe el `IDSujeto` ya resuelto por el token de
  acceso, nunca ve un JWT ni un `usuario_id` de la query como identidad del
  ejecutor (INV-TEN-12).
- Emitir o validar tokens/sesiones — **Acceso**. Tenencia consume
  `acceso/puertos.ValidadorDeAccesos` **solo desde su adaptador HTTP**
  (`internal/tenencia/adaptadores/http/middleware_autenticacion.go`); la
  capa de aplicación nunca sabe que existe un JWT (INV-TEN-28).
- Rate limiting, captcha, score de riesgo — **Confianza**, vía el ACL
  `tenencia/adaptadores/confianza/`. Se evalúa en `crear_organizacion`,
  `invitar_miembro` y `aceptar_invitacion` (§11.3 del diseño).
- Persistir la bitácora forense — **Auditoría** (mismo mecanismo de
  Identidad/Acceso: tabla `auditoria`, hash-chaining, ADR 0005), vía el ACL
  `tenencia/adaptadores/auditoria/`. Tenencia es el primer contexto que
  puebla `auditoria.organizacion_id` con un valor real (ver "Auditoría" más
  abajo).
- Resolver un correo a un `usuario_id` — Identidad no expone esa búsqueda
  (sería un oráculo de enumeración de cuentas), así que invitar es un
  agregado propio con token opaco, no "crear una membresía en estado
  pendiente".
- Editor de roles por organización — el catálogo de tres roles
  (`propietario > administrador > miembro`) es fijo y cerrado en código
  (ADR 0029); no hay RBAC configurable en este MVP.

## Puertos que expone a otros contextos, y los que consume

**Expone** (contrato público del contexto — cualquier otro contexto que
necesite autorizar consume esto, nunca las tablas `organizaciones`/
`membresias`):

| Puerto | Consumidor real hoy | Para qué |
|---|---|---|
| `puertos.VerificadorDeAutorizacion` (`Autorizar`) | **Identidad**, vía `identidad/adaptadores/tenencia.AutorizadorConsultas` | Cierra la autorización de `GET /identidad/usuarios/{id}` (ver `internal/identidad/README.md`) |
| `puertos.ConsultorDeMembresias` (`RolEnOrganizacion`, `ListarMiembros`, `ListarOrganizacionesDeUsuario`) | **Identidad** (mismo ACL, comprueba que el objetivo también es miembro de la organización) | Completa la regla de autorización de terceros de Identidad — ver §11.2 del diseño |
| *(indirecto)* `TenantID` en `confianza/puertos.Solicitud` | **Confianza** | Poblado por el middleware de autorización de Tenencia con el `{idOrganizacion}` ya autorizado de la ruta; habilita el límite por tenant que antes quedaba fuera por falta de una clave real (§11.3 del diseño) — Confianza no consume un puerto de Tenencia directamente, solo recibe el valor que Tenencia ya resolvió |

`Acceso` **no** consume ningún puerto de Tenencia, deliberadamente: si
Acceso conociera roles, la tentación siguiente sería meterlos en el token y
romper INV-ACC-12 (el JWT no lleva `roles`/`org_id`).

**Consume:**

| Contexto | Puerto/mecanismo | Vía | Para qué |
|---|---|---|---|
| Identidad | `puertos.ConsultorDeUsuarios.ObtenerPorID` | ACL `tenencia/adaptadores/identidad/` (`VerificadorDeSujetos`) | Validar que un `usuario_id` existe y está `activo` antes de crear una membresía (INV-TEN-07); obtener el correo verificado de quien acepta una invitación (INV-TEN-21) |
| Acceso | `puertos.ValidadorDeAccesos` | middleware HTTP propio de Tenencia | Autenticar la petición y obtener el `sub` |
| Confianza | evaluador de riesgo | ACL `tenencia/adaptadores/confianza/` | Acotar creación de organizaciones, invitaciones y redenciones |
| Auditoría | registro forense | ACL `tenencia/adaptadores/auditoria/` | Bitácora, con `organizacion_id` poblado |

## Cómo levantarlo en local

Tenencia **no tiene modo aislado**: se monta siempre junto con Identidad y
Acceso, dentro de la misma función `montarIdentidadYAcceso` de
`cmd/api/main.go` (que a su vez llama a `montarTenencia`, construye el ACL
de Identidad sobre `VerificadorDeAutorizacion`/`ConsultorDeMembresias`, y
solo entonces registra las rutas de Identidad). No existe un `make run`
parcial que levante Tenencia sin los otros dos.

```bash
make docker-up     # levanta Postgres y Redis
make migrate-up     # aplica las migraciones, incluidas 000009 (organizaciones/membresías),
                     # 000012 (catálogo de auditoría de Tenencia), 000013 (invitaciones) y
                     # 000014 (RLS)
make run             # arranca el servidor en :8080 — monta Identidad, Acceso Y Tenencia
```

**Nota sobre la numeración de migraciones**: el diseño en prosa habla de
`000010`/`000011` para invitaciones/RLS — esos números **nunca llegaron a
aplicarse**. `golang-migrate` rastrea un único "version" más alto ya
aplicado, no un conjunto de migraciones aplicadas: una vez que `000012`
(catálogo de auditoría, que no dependía de las otras dos) corrió antes, las
migraciones `000010`/`000011` habrían quedado silenciosamente ignoradas
para siempre por `up`. Se detectó a tiempo y se renumeraron a `000013`
(invitaciones) y `000014` (RLS). El razonamiento completo, y la lección de
"nunca dejar un hueco de numeración reservado para después", está en el
comentario de cabecera de `db/migraciones/000012_acciones_auditoria_tenencia.up.sql`.
Usa siempre los nombres de archivo reales del directorio, no lo que narre
un documento de diseño.

Log de arranque real observado:

```
api: contexto Tenencia montado (postgres+rls, auditoria, eventos-log, notificador-invitaciones-log, <estado de confianza>)
```

### Política de organización (límites anti-abuso, no restricciones de producto)

`dominio.PoliticaOrganizacionPorDefecto()` fija, sin necesidad de
migración ni variable de entorno:

| Parámetro | Valor | Efecto si se excede |
|---|---|---|
| Vigencia de una invitación | 7 días | La invitación expira; hay que reinvitar |
| Miembros activos por organización | 200 | `409` `ErrLimiteMiembrosExcedido` |
| Invitaciones pendientes por organización | 50 | `409` `ErrLimiteInvitacionesExcedido` |
| Organizaciones (`propietario`, activas) por usuario | 20 | `409` `ErrLimiteOrganizacionesExcedido` |

## Documentación OpenAPI (generada, no se edita a mano)

Por ADR 0006, la spec la genera Huma v2 a partir de los DTOs de
`internal/tenencia/adaptadores/http/dtos.go` y las operaciones de
`rutas.go`. **Tenencia monta su propia instancia de `huma.API` con
namespace propio**, exactamente el mismo patrón que Acceso ya adoptó para
no competir con Identidad por las rutas de metadatos por defecto (ver
`internal/acceso/README.md`, el bug real que ese override corrige):

- `GET /tenencia/openapi.json` / `.yaml` — OpenAPI 3.1, solo de Tenencia.
- `GET /tenencia/docs` — UI interactiva (Stoplight Elements).
- `GET /tenencia/schemas/{schema}.json` — resolución de `$ref`.

Con tres instancias de `huma.API` sobre el mismo `*fiber.App` (Identidad
sin prefijo, Acceso en `/acceso/*`, Tenencia en `/tenencia/*`), sin este
override las tres competirían por `/openapi.json`/`/docs`/`/schemas/*` y
Fiber solo serviría la primera registrada — ver el comentario de cabecera
de `internal/tenencia/adaptadores/http/rutas.go`, que cita explícitamente
el precedente de Acceso para no repetir el mismo bug dos veces.

## Invariantes de seguridad que le importan a quien integra

La lista completa (INV-TEN-01 a INV-TEN-30) está en
`docs/design/tenencia-bounded-context.md` §4. Las que un integrador
necesita conocer sin leer el diseño completo:

- **INV-TEN-06** — toda organización `activa` tiene **al menos una**
  membresía `activa` con rol `propietario`. Ninguna operación puede
  dejarla sin propietario: se rechaza con `409` `ErrUltimoPropietario`
  ("transferí la propiedad antes de salir" — el callejón sin salida más
  probable del producto). Sostenido por un candado de fila (`SELECT ...
  FOR UPDATE` sobre `organizaciones`) **y** un `CONSTRAINT TRIGGER`
  diferido — ninguno de los dos solo alcanza.
- **INV-TEN-09** — a lo sumo **una** membresía no removida por par
  (usuario, organización), garantizado por un índice único parcial, no por
  una comprobación en la aplicación. Readmitir a alguien crea una
  membresía **nueva** (`IDMembresia` nuevo), nunca reactiva la vieja.
- **INV-TEN-13** — la organización de una petición es **siempre explícita**
  (el `{idOrganizacion}` de la ruta). Nunca sale de un claim del token
  (INV-ACC-12 ya prohíbe que exista), ni de estado de sesión, ni de un
  subdominio. Si tu integración necesita saber "la organización activa",
  eso vive en tu propio estado de cliente, no en el JWT.
- **INV-TEN-17** — "no sos miembro" y "la organización no existe" producen
  la **misma** respuesta observable: `404`, mismo cuerpo. Solo "sos
  miembro pero tu rol no alcanza" produce `403`. **No intentes distinguir
  estos casos en el cliente** a partir del status — un `403` en el primer
  caso confirmaría la existencia de una organización ajena.
- **INV-TEN-20** — regla de dominancia: nadie puede otorgar un rol
  estrictamente superior al propio, ni modificar o remover una membresía
  de rol estrictamente superior al propio. Un `administrador` puede
  degradar a otro `administrador` (mismo nivel) pero nunca tocar a un
  `propietario`. Aplica también al rol **propuesto** en una invitación: un
  `administrador` no puede invitar a alguien como `propietario`.
- **INV-TEN-23** — el token de invitación en claro **nunca** aparece en la
  respuesta HTTP de `POST .../invitaciones` (`vistaInvitacionRespuesta` no
  tiene campo para él), nunca se persiste (solo su hash SHA-256), nunca se
  loguea ni viaja en un evento de auditoría. Sale del proceso **solo** por
  `NotificadorInvitaciones` (hoy un stub log-only, mismo patrón que
  `NotificadorCorreoLog` de Identidad).
- **INV-TEN-25** — solo se auditan las **denegaciones** de autorización
  (`autorizacion.denegada`); nunca existe un evento de "autorización
  concedida". `Autorizar` corre en el camino más caliente de todo el
  sistema (una vez por petición autorizada de cualquier contexto);
  auditar concesiones pondría todo el tráfico protegido detrás del
  advisory lock que serializa la cadena de hashes (ADR 0005).
- **INV-TEN-30** — RLS es **red de seguridad, no el mecanismo de
  autorización**. La decisión la toma `VerificadorDeAutorizacion`; RLS
  garantiza que un `WHERE organizacion_id = $1` olvidado devuelva **cero
  filas** en vez de filtrar datos de otro tenant. Ver el gotcha real más
  abajo — es exactamente el caso donde esta distinción dejó de ser
  teórica.

## Los 14 endpoints reales

Prefijo `/tenencia`, con la única excepción de aceptar invitación (no
cuelga de `/organizaciones/{id}`, ver más abajo). `rutas.go` registra 14
operaciones — no 11: cuenta cada método+ruta con `huma.Register` una vez.
Los DTOs de abajo son los campos reales de
`internal/tenencia/adaptadores/http/dtos.go`.

### Organizaciones

#### `POST /tenencia/organizaciones` — Crear una organización

Cualquier sujeto autenticado y `activo` puede fundar una organización:
nace con su fundador como `propietario`, en la **misma transacción**
(INV-TEN-03) — nunca hay un instante, ni transitorio, sin dueño.

Auth: Bearer. Permiso Tenencia: **ninguno** (es el único caso de uso que no
requiere autorización previa: todavía no existe nada respecto de lo cual
autorizar). Rate limit: ADR 0018/§11.3, `crear_organizacion` — 5/hora por
usuario, 20/hora por IP.

Request:

```json
{ "nombre": "Acme Corp", "alias": "acme-corp" }
```

Response `201 Created`:

```json
{
  "id": "0199...",
  "alias": "acme-corp",
  "nombre": "Acme Corp",
  "estado": "activa",
  "creada_en": "2026-09-04T10:00:00Z",
  "actualizada_en": "2026-09-04T10:00:00Z",
  "miembros_activos": 1
}
```

Errores: `401`; `403` `ErrSujetoNoElegible` (el usuario no existe o no está
`activo` en Identidad — en particular, **un usuario en
`pendiente_verificacion` no puede fundar una organización**); `409`
`ErrAliasYaRegistrado` / `ErrLimiteOrganizacionesExcedido`; `422`
`ErrAliasInvalido` / `ErrNombreOrganizacionInvalido`; `429`
`ErrAccesoDenegadoPorConfianza` (`Retry-After`).

#### `GET /tenencia/organizaciones` — Listar mis organizaciones

Las organizaciones a las que el sujeto autenticado pertenece, con su rol.
No requiere autorización de Tenencia (es la consulta del propio sujeto,
análoga a `GET /acceso/sesiones`) y **no se audita**.

Auth: Bearer. Permiso: ninguno. Query opcional: `incluir_no_activas`
(bool) — si es `true`, incluye organizaciones/membresías suspendidas o
archivadas.

Response `200 OK`:

```json
[
  {
    "id_organizacion": "0199...",
    "alias": "acme-corp",
    "nombre": "Acme Corp",
    "rol": "propietario",
    "estado_membresia": "activa",
    "estado_organizacion": "activa"
  }
]
```

Errores: `401`.

#### `GET /tenencia/organizaciones/{idOrganizacion}` — Consultar una organización

Exige `organizacion.ver`.

Response `200 OK`: mismo cuerpo que la creación (`vistaOrganizacionRespuesta`).

Errores: `401`; `404` si el sujeto no es miembro (INV-TEN-17, mismo cuerpo
que "no existe").

#### `PATCH /tenencia/organizaciones/{idOrganizacion}` — Renombrar / cambiar alias

Exige `organizacion.editar` (`propietario` o `administrador`).

Request (ambos campos opcionales; ausente = no cambiar):

```json
{ "nombre": "Acme Corporation", "alias": "acme" }
```

Response `200 OK`: `vistaOrganizacionRespuesta` actualizada.

Errores: `401`; `404`; `409` `ErrAliasYaRegistrado`; `422` si el nuevo
alias/nombre no cumple el formato del VO.

#### `POST /tenencia/organizaciones/{idOrganizacion}/cambios-estado` — Suspender / reactivar / archivar

Exige `organizacion.archivar` (solo `propietario`). `archivada` es
terminal e irreversible (INV-TEN-04); suspender/archivar **no** muta
ninguna membresía — el acceso se apaga por conjunción en tiempo de
autorización (§1.4 del diseño), operación O(1) y reversible salvo archivar.

Request:

```json
{ "destino": "suspendida", "motivo": "Impago de la suscripción" }
```

Response `200 OK`: `vistaOrganizacionRespuesta` con el nuevo `estado`.

Errores: `401`; `403`; `404`; `409` transición inválida (p. ej. reactivar
una `archivada`).

### Membresías

#### `GET /tenencia/organizaciones/{idOrganizacion}/miembros` — Listar miembros

Exige `miembro.ver`. **Nunca** incluye correo ni nombre del usuario
(INV-TEN-29): Tenencia no los conoce y no hace `JOIN` contra `usuarios`.

Response `200 OK`:

```json
[
  {
    "id_membresia": "0199...",
    "id_usuario": "0199...",
    "rol": "propietario",
    "estado": "activa",
    "creada_en": "2026-09-04T10:00:00Z"
  }
]
```

Errores: `401`; `404` (INV-TEN-17).

#### `POST /tenencia/organizaciones/{idOrganizacion}/miembros` — Agregar un miembro (alta directa)

Exige **`miembro.invitar`** (no `miembro.cambiar_rol` — ver la nota de
discrepancia con el diseño más abajo). No es el camino del producto: el
camino del producto es invitar por correo, porque una UI no conoce
`usuario_id` ajenos y pedírselos sería una función de enumeración. Existe
para herramientas administrativas / migración de datos / futuro SCIM.

Request:

```json
{ "usuario_id": "0199...", "rol": "miembro" }
```

Response `201 Created`: `vistaMiembroRespuesta` (igual forma que el
listado).

Errores: `401`; `403` regla de dominancia sobre el rol otorgado / sujeto no
elegible; `404`; `409` `ErrMembresiaDuplicada` / `ErrLimiteMiembrosExcedido`.

> **Nota de discrepancia diseño vs. implementación**: §7 del diseño
> (`docs/design/tenencia-bounded-context.md`) lista este endpoint con
> permiso `miembro.cambiar_rol`. El código real (`rutas.go`, comentario
> incluido) exige `miembro.invitar`, razonando que el catálogo cerrado de 8
> permisos no distingue "invitar por correo" de "dar de alta
> directamente" — ambas comparten permiso y regla de dominancia. El código
> es la fuente de verdad; el diseño queda anotado como desactualizado en
> ese punto (ver la nota en `docs/design/tenencia-bounded-context.md`).

#### `PATCH /tenencia/organizaciones/{idOrganizacion}/miembros/{idUsuario}` — Cambiar el rol de un miembro

Exige `miembro.cambiar_rol`, sujeto a la regla de dominancia (INV-TEN-20).
Si el nuevo rol es igual al actual, es un no-op idempotente que **no
audita**.

Request:

```json
{ "rol": "administrador" }
```

Response `200 OK`: `vistaMiembroRespuesta`.

Errores: `401`; `403` `ErrRolSuperiorAlPropio` / `ErrMembresiaDominante`;
`404`; `409` `ErrUltimoPropietario` (degradar al único propietario).

#### `DELETE /tenencia/organizaciones/{idOrganizacion}/miembros/{idUsuario}` — Remover a un miembro

Exige `miembro.remover`, sujeto a la regla de dominancia. Transición a
`removida`, **nunca** `DELETE` físico (INV-TEN-08); readmitir crea una
membresía nueva.

Response: `204 No Content`.

Errores: `401`; `403`; `404`; `409` `ErrUltimoPropietario`.

#### `DELETE /tenencia/organizaciones/{idOrganizacion}/miembros/actual` — Abandonar la organización

No exige un permiso de la matriz (§3.2 del diseño: "un sujeto siempre
puede actuar sobre su propia membresía"); el middleware usa `miembro.ver`
como gate mínimo de "sos miembro activo de una organización operativa"
para poder fijar el `AlcanceDeTenencia`. El único propietario **no puede**
abandonar sin transferir antes (INV-TEN-06).

Response: `204 No Content`.

Errores: `401`; `404`; `409` `ErrUltimoPropietario`.

#### `POST /tenencia/organizaciones/{idOrganizacion}/transferencias-propiedad` — Transferir la propiedad

Exige `propiedad.transferir` (solo `propietario`). El destinatario debe
ser miembro activo previo (transferir a alguien que no es miembro sería
agregarlo y coronarlo en un solo paso: dos hechos de negocio distintos).

Request:

```json
{ "nuevo_propietario_id": "0199...", "rol_resultante_del_cedente": "administrador" }
```

(`rol_resultante_del_cedente` vacío = conservar `propietario`, co-propiedad
explícita.)

Response: `204 No Content`.

Errores: `401`; `403`; `404`; `409` (destinatario no es miembro activo,
etc.).

### Invitaciones

#### `POST /tenencia/organizaciones/{idOrganizacion}/invitaciones` — Invitar a un miembro por correo

Exige `miembro.invitar`, sujeto a la regla de dominancia sobre el rol
**propuesto**. Si ya existe una invitación `pendiente` para ese par
(organización, correo), se **revoca y se emite una nueva** — reinvitar es
legítimo y frecuente, fallar con `409` sería mala respuesta a una
necesidad real.

Auth: Bearer. Rate limit: ADR 0018/§11.3, `invitar_miembro` — 20/hora por
organización, 5/min por IP (este endpoint envía correo a terceros: sin
límite es un relay de spam con la reputación del dominio del servicio
como munición).

Request:

```json
{ "correo": "nuevo@ejemplo.com", "rol": "miembro" }
```

Response `201 Created` — **sin el token en claro** (INV-TEN-23):

```json
{
  "id": "0199...",
  "id_organizacion": "0199...",
  "destinatario": "nuevo@ejemplo.com",
  "rol": "miembro",
  "estado": "pendiente",
  "creada_en": "2026-09-04T10:00:00Z",
  "expira_en": "2026-09-11T10:00:00Z"
}
```

Errores: `401`; `403` regla de dominancia sobre el rol propuesto; `404`;
`409` `ErrLimiteInvitacionesExcedido`; `429` `ErrAccesoDenegadoPorConfianza`.

#### `DELETE /tenencia/organizaciones/{idOrganizacion}/invitaciones/{idInvitacion}` — Revocar una invitación

Exige `miembro.invitar`. Transición a `revocada`, sin `DELETE` físico
(evidencia forense).

Response: `204 No Content`.

Errores: `401`; `403`; `404`.

#### `POST /tenencia/invitaciones/aceptaciones` — Aceptar una invitación

Requiere **autenticación** (Bearer) pero **NO** autorización de Tenencia:
quien acepta no es miembro todavía, así que el middleware de autorización
no tiene nada contra qué autorizarlo. Por eso **no cuelga de
`/organizaciones/{id}`** — el cliente que llega desde el enlace del correo
tiene el token de invitación, no el ID de la organización (mismo criterio
por el que `/.well-known/jwks.json` de Acceso queda fuera del prefijo
`/acceso`).

Token desconocido, malformado, revocado, ya aceptado, expirado **o de
destinatario distinto** producen la **misma** respuesta observable
(INV-TEN-24): quien acepta debe estar autenticado, activo, y su correo
verificado debe coincidir con el destinatario en **tiempo constante**
(INV-TEN-21) — poseer el enlace no basta, un enlace reenviado o filtrado
no vale nada en manos de un tercero.

Rate limit: ADR 0018/§11.3, `aceptar_invitacion` — 10/min por IP (es un
oráculo de fuerza bruta sobre tokens de invitación, igual que la
renovación de refresco en Acceso).

Request:

```json
{ "token": "mot_inv_..." }
```

Response `201 Created`: `vistaMiembroRespuesta` de la membresía recién
creada (o la existente, si el sujeto ya era miembro — idempotente, sin
duplicar ni auditar un alta que no ocurrió).

Errores: `401` (sin token de acceso Bearer); `404` `ErrInvitacionInvalida`
/ `ErrInvitacionAjena` (indistinguibles); `409` organización no operativa /
límite de miembros excedido; `429`.

> **Ausencia notable respecto del diseño**: §7 del diseño lista también
> `GET /tenencia/organizaciones/{idOrganizacion}/invitaciones` (listar
> invitaciones de una organización). Ese endpoint **no está implementado**
> — `rutas.go` no lo registra. Si tu integración necesita ver invitaciones
> pendientes de una organización, ese endpoint no existe todavía.

### Formato del error `401`

Mismo criterio que Acceso e Identidad: cabecera `WWW-Authenticate: Bearer
error="invalid_token"` y cuerpo RFC 9457. El middleware de autenticación
de Tenencia no distingue "falta la cabecera" de "token inválido o
expirado" en el status — ambos son `401`.

## Gotcha real de implementación: RLS que se comparaba consigo mismo

Encontrado y corregido durante la verificación en vivo de este cierre
(servidor real, Postgres+Redis reales), **no un pendiente**:
`internal/plataforma/bd.ReforzarAlcanceTenencia` reemitía el `SET LOCAL`
de RLS en cada llamada de repositorio usando los mismos parámetros que la
propia consulta estaba a punto de ejecutar. Eso hacía que la política de
aislamiento comparara `organizacion_id = organizacion_id` — **siempre
verdadero** — y anulaba exactamente la defensa en profundidad que ADR
0031 promete (INV-TEN-30).

La corrección: `ReforzarAlcanceTenencia` ahora es un **no-op** cuando el
contexto ya trae un `AlcanceTenencia` publicado por el middleware de
autorización HTTP (el alcance confiable, ya verificado contra
`VerificadorDeAutorizacion`, que debe prevalecer sobre cualquier parámetro
que una consulta posterior pase). Solo auto-deriva el alcance de sus
propios parámetros cuando no hay ninguno establecido todavía —
exactamente los casos nombrados en el propio comentario de la función:
`AutorizarCasoDeUso` (que es quien decide si el middleware debe dejar
pasar la petición, así que todavía no hay alcance que reforzar), los ACL
de Identidad/Confianza que invocan a Tenencia in-process, `CrearOrganizacion`
(el único caso de uso sin autorización previa) y `AceptarInvitacion` (que
no cuelga de una ruta con `{idOrganizacion}` porque el aceptante todavía no
es miembro de ninguna organización).

Verificado a mano con y sin el fix: **sin** él, un usuario autorizado solo
para su propia organización podía leer membresías de una organización
ajena con que un caso de uso pasara ese ID por error a
`ReforzarAlcanceTenencia`; **con** él, la misma consulta devuelve **0
filas**. El razonamiento completo — con el porqué exacto de la dirección
del fallo — está en el comentario de cabecera de
`internal/plataforma/bd/alcance_tenencia.go`; no se repite aquí para no
desincronizarse de la fuente.

## Auditoría

Se auditan: alta de organización, actualización, cambio de estado, alta de
membresía (fundación / alta directa / invitación aceptada), cambio de rol,
cambio de estado de membresía, remoción, invitación emitida (nunca el
token ni su hash), desenlace de invitación (aceptada / revocada / expirada
/ intento fallido) y **denegación** de autorización — nunca la concesión
(INV-TEN-25, porque `Autorizar` corre en el camino más caliente de
cualquier contexto). Catálogo completo:
`docs/catalogos/acciones-auditoria.md`, sección "Contexto Tenencia"
(migración `db/migraciones/000012_acciones_auditoria_tenencia.up.sql`).

**Tenencia es el primer contexto que puebla `auditoria.organizacion_id`**
con un valor real — esa columna existe desde `000002_crear_auditoria.up.sql`
con el comentario `NULL hasta que exista Tenencia`, y ya formaba parte del
payload canónico del hash-chaining sin necesitar ninguna migración nueva.
La cadena de auditoría se verificó íntegra en vivo
(`verificar_cadena_auditoria()`, 0 discrepancias) con filas de Tenencia
llevando `organizacion_id` poblado por primera vez.

## Aislamiento multi-tenant (RLS)

`organizaciones`, `membresias` e `invitaciones` tienen `FORCE ROW LEVEL
SECURITY` (ADR 0031), activada por dos GUC de sesión
(`app.usuario_actual`, `app.organizacion_actual`) fijados con `SET LOCAL`
al abrir cada transacción a partir del `AlcanceDeTenencia` que publicó el
middleware de autorización. **Es una red de seguridad, no el mecanismo**
(INV-TEN-30): la decisión la toma `VerificadorDeAutorizacion` antes de que
la transacción se abra. Sin `SET LOCAL` fijado, las políticas fallan
**cerradas** (cero filas), nunca abiertas — ver el gotcha de arriba para
un ejemplo concreto de por qué esa dirección de fallo importa en la
práctica, no solo en la teoría del ADR.

## Referencias

- Diseño completo: `docs/design/tenencia-bounded-context.md`
- ADR 0029 (modelo de roles): `docs/adr/0029-modelo-roles-catalogo-cerrado.md`
- ADR 0030 (autorización por consulta, sin caché): `docs/adr/0030-autorizacion-por-consulta-en-cada-peticion.md`
- ADR 0031 (RLS multi-tenant): `docs/adr/0031-rls-multi-tenant-guc-por-transaccion.md`
- README de Identidad (consumidor de `VerificadorDeAutorizacion`/`ConsultorDeMembresias`): `internal/identidad/README.md`
- README de Acceso (patrón de namespace OpenAPI que Tenencia replica; puerto `ValidadorDeAccesos` que consume Tenencia): `internal/acceso/README.md`
- Índice completo de ADRs: `docs/adr/README.md`
