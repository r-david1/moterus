# ADR 0033 — Invitar a una organización crea un agregado `Invitacion`, no una `Membresia` en estado pendiente

## Contexto

Para invitar a alguien a una organización por correo, había dos formas obvias de modelarlo: (a) crear directamente una `Membresia` en un estado `pendiente`, que se activa cuando el invitado acepta, o (b) modelar la invitación como un agregado propio y de ciclo de vida corto, que al aceptarse crea una `Membresia` nueva en estado `activa`. `docs/design/tenencia-bounded-context.md` §0 la nombró como candidata (0033).

La decisión no es solo de modelado: Tenencia **no puede resolver un correo a un `usuario_id`** (Identidad expone `ConsultorDeUsuarios.ObtenerPorID`, nunca una búsqueda por correo — exponer una rompería INV-ID-11/ADR 0013, que existen precisamente para que ningún endpoint permita comprobar si un correo está registrado). Eso descarta de raíz la opción (a): no se puede crear una `Membresia` con un `usuario_id` que todavía no se conoce, porque el invitado —si no tiene cuenta— ni siquiera existe como usuario en el momento de invitar.

`db/migraciones/000013_crear_invitaciones.up.sql` implementa el agregado propio: tabla `invitaciones` con `correo_destinatario` (no `usuario_id`), `hash_token` (SHA-256 del token, el valor en claro nunca se persiste — INV-TEN-23), `rol_propuesto`, `estado` (`pendiente`/`aceptada`/`revocada`/`expirada`) y una ventana de vigencia. Aceptarla —vía `RepositorioInvitaciones.BuscarPorHash`, que atraviesa RLS por la función `SECURITY DEFINER tenencia_resolver_invitacion` porque quien acepta todavía no es miembro de ninguna organización— crea una `Membresia` nueva **en la misma unidad de trabajo**, dos agregados coordinados por el caso de uso, no un agregado que contenga al otro.

## Decisión

**Invitar crea un agregado `Invitacion` independiente, identificado por correo y un token de un solo uso; nunca una `Membresia` en un estado intermedio.** El invitado se resuelve a un `usuario_id` real recién en el momento de aceptar —ya autenticado, con su propia sesión— y solo entonces nace la `Membresia`, ya en estado `activa`, sin pasar por ningún estado "pendiente" a nivel de membresía.

Esto resuelve de paso el problema de reinvitación: `invitaciones_pendiente_idx` es un índice único **parcial** sobre `(organizacion_id, correo_destinatario) WHERE estado = 'pendiente'` — el caso de uso `Invitar` revoca cualquier invitación pendiente previa para el mismo par antes de insertar la nueva, en la misma transacción, así que el índice nunca ve dos filas `pendiente` simultáneas para el mismo destinatario. Una `Membresia` en estado pendiente habría necesitado resolver el mismo problema con un mecanismo propio, duplicando lo que `Invitacion` ya resuelve.

**El token por sí solo no basta (INV-TEN-21).** `AceptarInvitacionCasoDeUso` no se conforma con un hash de token válido: además exige que el sujeto autenticado esté activo (`SujetoElegible.Existe && .Activo`) y que su correo normalizado coincida exactamente con `CorreoDestinatario` de la invitación — `invitacion.Aceptar(correoSujeto, ahora)` rechaza la aceptación si no coincide. La razón es que un enlace de invitación es, en la práctica, un secreto de baja entropía operativa: se reenvía por chat, queda en historiales, se indexa por clientes de correo. Exigir además el control del correo exacto al que se envió convierte la fuga de un enlace en un no-evento — quien lo intercepte no puede canjearlo sin controlar también (o haber verificado) esa cuenta de correo específica.

## Alternativas consideradas

- **`Membresia` en estado `pendiente`, con `usuario_id` nulo hasta la aceptación**: descartado de raíz — Tenencia no tiene forma de saber si el correo invitado ya corresponde a un usuario existente sin pedirle a Identidad un oráculo de existencia que **no debe existir** (ADR 0013); modelar la membresía antes de tener un `usuario_id` real dejaría una columna `NOT NULL` violada o un `usuario_id` inventado.
- **`Membresia` en estado `pendiente`, con `correo_destinatario` en vez de `usuario_id` hasta la aceptación**: descartado — mezclaría en un solo agregado dos identidades de clave distintas (correo antes de aceptar, `usuario_id` después), lo que rompería la garantía de unicidad de INV-TEN-09 (que es sobre `usuario_id`, no sobre correo) y complicaría cualquier consulta que asuma que una `Membresia` siempre tiene un `usuario_id` válido.
- **Guardar el token de invitación en claro**: descartado — mismo criterio que los tokens de verificación de correo de Identidad (INV-ID-21) y los de refresco de Acceso: un secreto de un solo uso se persiste como hash (SHA-256), nunca en claro, para que una fuga de la base de datos no equivalga a una fuga de invitaciones canjeables.

## Consecuencias

- Una invitación resuelta (aceptada, revocada o expirada) se conserva como evidencia con su estado terminal — igual criterio que la cadena de tokens de refresco de Acceso: sin `DELETE` en los privilegios de la tabla (`REVOKE DELETE, TRUNCATE ON invitaciones FROM rol_aplicacion`), porque "¿quién invitó a esta persona, cuándo, con qué rol?" es información que debe sobrevivir a la resolución de la invitación.
- Aceptar una invitación exige que el invitado esté **ya autenticado** en el sistema (con cuenta propia, verificada) antes de poder canjear el token — no hay un flujo de "crear cuenta y unirse a la organización en un solo paso" en el MVP. Es una fricción adicional para el invitado sin cuenta previa, aceptada porque separar ambos flujos (registro en Identidad, luego aceptación en Tenencia) es más simple y no reintroduce el problema que la decisión de arriba evita.
- Dos agregados (`Invitacion`, `Membresia`) coordinados en una sola transacción por el caso de uso `AceptarInvitacion` es el patrón a replicar si algún día aparece otro flujo de "algo externo se convierte en una entidad interna tras una confirmación" — no una `Membresia`/entidad con un estado a medio construir.

## Estado

Aceptado (implementado de facto; documentado retroactivamente).
