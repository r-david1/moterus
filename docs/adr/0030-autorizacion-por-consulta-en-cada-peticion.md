# ADR 0030 — La autorización se resuelve por consulta en cada petición, nunca por claim en el token ni por caché

## Contexto

ADR 0025 (candidato de Acceso, ver `docs/design/acceso-bounded-context.md` §sección de invariantes, INV-ACC-12) ya decidió que el token de acceso **no lleva** `roles`, `permisos`, `org_id` ni `tenant_id`, y dejó explícitamente delegada en el contexto que resolviera la autorización la decisión adyacente: *si no va en el token, ¿dónde y cuándo se resuelve?*

Con el diseño de Tenencia (`docs/design/tenencia-bounded-context.md` §3.7) hay que cerrar esa pregunta, porque de ella depende la firma de `VerificadorDeAutorizacion.Autorizar` — el puerto público que cualquier otro contexto va a consumir en el camino caliente de cada petición protegida.

Las tres opciones reales:

1. **Claim en el JWT** (`roles`, `org_id`) — ya descartada por ADR 0025/INV-ACC-12, no se reabre aquí.
2. **Caché en Redis** con invalidación al escribir.
3. **Consulta a Postgres en cada petición**, sin caché.

## Decisión

**Cada petición que necesita autorización llama a `Autorizar` y ese caso de uso hace exactamente una lectura indexada a Postgres, sin caché de ningún tipo.**

Presupuesto fijado para `AutorizarCasoDeUso` (§3.7 del diseño): una consulta por el índice único parcial `(organizacion_id, usuario_id) WHERE estado <> 'removida'` (con `JOIN` al estado de la propia organización), cero llamadas a otros contextos, cero escrituras en el camino feliz (solo se audita la denegación, nunca la concesión — INV-TEN-25).

### Por qué ni siquiera un caché en Redis

Un claim en el JWT es un caché de hasta `vidaTokenAcceso` (10 minutos) con la peor invalidación posible: quitarle un rol a alguien no surte efecto hasta que el token expira, y el claim queda desincronizado con Tenencia sin que nada lo señale. Un caché en Redis con invalidación activa tiene el mismo defecto de fondo con menos honestidad: sigue introduciendo una ventana de *staleness*, exactamente en el dato del sistema que menos la tolera — la decisión de si un sujeto puede hacer algo ahora mismo — a cambio de ahorrar una lectura por clave primaria lógica que ya es barata.

### Por qué el costo real es aceptable

Es una lectura sobre un índice único parcial, en una tabla de baja cardinalidad (miembros de una organización, no todos los usuarios del sistema), en el mismo pool de conexiones que ya sirve la ruta `ExigirSesionViva=true` de Acceso (que también pega a Postgres en cada petición de alto valor, ADR 0019). Es órdenes de magnitud más barata que el hash Argon2id de 64 MiB que ADR 0008 ya acepta pagar en cada login.

### La consecuencia positiva, concreta y verificable

`RemoverMiembro` o `CambiarRol` surten efecto en la **petición siguiente**, sin ventana (INV-TEN-15). Un sistema cuya razón de ser es la autenticación y autorización no debería tener que explicar por qué revocar un permiso tarda diez minutos.

## Alternativas consideradas

- **Claim `roles`/`org_id` en el JWT**: descartada — reabre ADR 0025/INV-ACC-12 sin ninguna razón nueva; el argumento contra la invalidación tardía ya está escrito ahí.
- **Caché en Redis con invalidación en cada escritura de Tenencia**: descartada — añade una fuente más de inconsistencia posible (¿qué pasa si la invalidación falla o se pierde un evento?), complejidad operativa nueva (otra dependencia dura del camino caliente de autorización, cuando Redis en este proyecto es explícitamente un *acelerador*, no una frontera de seguridad — mismo criterio que INV-ACC-15 para la lista de revocación de Acceso), a cambio de un ahorro de latencia que el propio sistema ya demuestra no necesitar (Argon2id es más caro y se acepta en cada login).
- **Caché en memoria del proceso, con invalidación por evento**: descartada por la misma razón que Redis, agravada: en un despliegue con más de una réplica, cada proceso tendría su propia copia potencialmente desincronizada, y no hay bus de eventos real en el stack (`PublicadorEventos` es log-only) para invalidarlas de forma consistente.

## Consecuencias

- `VerificadorDeAutorizacion.Autorizar` es el puerto de entrada más invocado de todo el sistema, y su implementación (`AutorizarCasoDeUso`) es la primera pieza de `tenencia/aplicacion` que debe escribirse y probarse (§10 del diseño, paso 5) — desbloquea a Identidad y a Confianza.
- `RepositorioMembresias.BuscarVigente` debe estar respaldado por el índice único parcial de la migración `000009`, no por un `SELECT` sin índice — es un requisito de rendimiento normativo, no una optimización futura.
- Ningún otro contexto (Identidad, Confianza) puede asumir que la autorización que obtuvo hace un minuto sigue siendo válida ahora: deben volver a preguntar en cada petición, nunca reutilizar una decisión anterior.
- Si en el futuro el volumen de tráfico demuestra que esta lectura es un cuello de botella real (medido, no anticipado), la vía de escape es optimizar la consulta o el índice, no introducir staleness — reabrir esta decisión exige un ADR nuevo con datos de producción que lo justifiquen.

## Estado

Aceptado.
