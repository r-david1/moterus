# ADR 0045 — `AlcanceSala`: sistema vs. organización, y por qué las salas de alcance sistema se operan fuera de la API

## Contexto

La ficha del agente `colas-virtuales` fija como regla dura *"la cola es por tenant + endpoint, nunca global"*. Es correcta para el caso que tenía en mente (una organización grande abre inscripciones y no debe poner en fila a las demás), pero incorrecta como regla universal para este producto, por dos razones que ADR 0002 y ADR 0009 vuelven inevitables:

1. **Bajo ADR 0002 hay un solo producto, un solo proceso, un solo pool de Postgres y una sola CPU.** Un pico de logins de la organización A degrada a la organización B aunque la cola de A esté perfectamente aislada, porque lo que se agota no es un recurso de A: es el pool de conexiones compartido y, sobre todo, la CPU que consume Argon2id con los parámetros fijos de ADR 0008 (64 MiB y t=3 por cada verificación de contraseña). Una cola por organización sobre `POST /acceso/sesiones` protegería la contabilidad, no la capacidad.
2. **En `POST /acceso/sesiones` no existe una organización que resolver.** La credencial es global (ADR 0009: Identidad autentica al usuario, no al miembro de una organización); la membresía se conoce recién después, y por consulta explícita a Tenencia (ADR 0030). Cualquier intento de encolar "por organización" en el login obligaría a que el cliente declare a qué organización pertenece antes de autenticarse — un discriminador declarado por el cliente, en el mecanismo cuyo propósito es contener una avalancha, es un mecanismo que la avalancha omite con solo no mandar el campo.

## Decisión

**`AlcanceSala` es un value object con dos formas, y cada ruta protegida admite solo la que puede resolverse de forma no falsificable:**

| Alcance | Cuándo | Cómo se resuelve la clave |
|---|---|---|
| `sistema` | rutas pre-autenticación (login, registro, aceptar invitación) | no se resuelve nada: la clave es la ruta |
| `organizacion` | rutas org-scoped, con `{idOrganizacion}` ya autorizado por Tenencia | el `IDOrganizacion` de la ruta, el mismo que ya puebla `Solicitud.TenantID` |

No se inventa ningún concepto paralelo de tenant: el alcance por organización usa `IDOrganizacion` de Tenencia tal cual, como clave opaca — el mismo trato que Confianza ya le da hoy en `Solicitud.TenantID` sin poseer la tabla `organizaciones`.

**Segunda parte de la decisión, igual de importante: las salas de alcance `sistema` no tienen endpoints HTTP administrativos.** Este producto no tiene rol de administrador de plataforma — el catálogo de roles de ADR 0029 es intra-organización y no existe nada por encima — y este ADR no lo crea. Una sala de alcance `sistema` se abre por operaciones: un subcomando de CLI que usa los mismos casos de uso (`AbrirSalaDeEspera` con `IDSujeto` vacío) sobre la misma tabla, y el reconciliador la levanta en ≤15 segundos.

## Alternativas consideradas

- **"Por tenant siempre", como propone la ficha**: descartado por las dos razones de arriba — fantasía en el caso más importante (el login) y protección de la métrica equivocada (contabilidad, no capacidad) en el caso general bajo ADR 0002.
- **Un tenant sintético o "por defecto" para las rutas sin organización**: descartado — inventaría un concepto de negocio (un tenant que no representa ninguna organización real) solo para no bifurcar el value object, cambiando código por confusión conceptual.
- **Inventar un rol de administrador de plataforma para operar las salas de alcance sistema por API**: descartado — es una decisión de seguridad de mucho mayor alcance que este diseño (un superusuario global en un servicio de autenticación) y merece su propio ADR y su propio hito, no colarse como efecto secundario de una feature de protección de capacidad.

## Consecuencias

- La intención original de la ficha se conserva donde es realizable (una organización con un pico propio en una ruta suya no arrastra a las demás) y se abandona donde era fantasía (fingir un tenant en el login).
- **Corolario incómodo, aceptado explícitamente**: para el alcance `organizacion`, la sala se evalúa después de autenticación y autorización (INV-COLA-09), porque antes no hay `{idOrganizacion}` autorizado — ya se pagó un JWT verificado y una consulta indexada a Postgres antes de encolar. Es una protección parcial por construcción; quien necesite proteger todo el camino usa una sala de alcance `sistema`.
- Operar una sala de alcance `sistema` es una asimetría deliberada y documentada (CLI/operaciones en vez de HTTP) — no un hueco a cerrar sin releer este ADR.

## Estado

Aceptado.
