# ADR 0029 — Modelo de roles de Tenencia: catálogo cerrado y totalmente ordenado, sin RBAC configurable

## Contexto

El diseño del contexto Tenencia (`docs/design/tenencia-bounded-context.md`) necesita fijar, antes de escribir ningún tipo Go, cómo se modela "qué puede hacer un miembro dentro de una organización". Hay dos caminos clásicos:

- **RBAC configurable por organización**: cada organización define sus propios roles y qué permisos lleva cada uno, persistidos en tablas propias (`roles`, `roles_permisos`).
- **Catálogo cerrado**: un número fijo de roles, iguales para todas las organizaciones, con una matriz rol→permiso fija en código.

Sin esto fijado, no se pueden escribir las firmas de `Membresia.CambiarRol`, la regla de dominancia (quién puede tocar a quién) ni el esquema de la tabla `membresias`.

## Decisión

### 1. Catálogo cerrado de tres roles, **totalmente ordenado**

```
propietario (30) > administrador (20) > miembro (10)
```

El orden total (no solo un conjunto con nombres) es la pieza que hace que dos preguntas centrales del dominio sean comparaciones de enteros en vez de tablas de casos:

- "¿tiene este sujeto al menos el rol X?" → `rol.Nivel() >= requerido.Nivel()`.
- "¿puede este sujeto tocar a este otro miembro?" → la regla de dominancia (INV-TEN-20): nadie puede otorgar un rol estrictamente superior al propio, ni modificar o remover una membresía cuyo rol sea estrictamente superior al propio.

La matriz rol→permiso (`organizacion.ver/editar/archivar`, `miembro.ver/invitar/cambiar_rol/remover`, `propiedad.transferir`) es una tabla **literal en código** (`Rol.Permisos()`), no datos en una tabla de base de datos.

### 2. Sin roles personalizados por organización en el MVP

No hay editor de roles, no hay tabla `roles` con FK a `organizaciones`, no hay permisos "a la carta". Cualquier organización usa exactamente el mismo catálogo de tres roles con la misma matriz.

### 3. El camino de evolución queda nombrado, no cerrado

`RepositorioRolesPersonalizados` y `RepositorioPermisosDeRol` quedan declarados como puertos aplazados (§2.3 del diseño) para que, si algún día se justifica RBAC configurable, el catálogo cerrado de hoy se convierta en la **semilla por defecto** — es un cambio aditivo, no una migración destructiva.

## Alternativas consideradas

- **RBAC configurable desde el MVP**: descartado. Son dos tablas más, decisiones de autorización dirigidas por datos (no verificables en compilación ni en un test de dominio puro), y una superficie de configuración incorrecta que el propio tenant podría explotar (un administrador editando la matriz de permisos para escalar su propio rol). Ningún consumidor de este hito lo pide: construirlo ahora es exactamente el "diseñar a medias" que ADR 0002 descartó para la capa de producto — una abstracción pagada antes de tener una necesidad real que la justifique.
- **Roles como VOs sin orden (solo un conjunto con nombres)**: descartado — sin orden total, la regla de dominancia se convierte en una tabla de pares explícita (`¿puede admin tocar a propietario? ¿puede propietario tocar a administrador?`) que crece cuadráticamente con cada rol nuevo y es fácil de dejar incompleta.
- **Un único rol "miembro" sin distinción de privilegios**: descartado de entrada — sin al menos un rol con privilegio de administración de la organización misma, no hay forma de expresar "quién puede archivarla o cambiar su nombre" sin volver a caer en autorización ad hoc fuera de Tenencia.

## Consecuencias

- `internal/tenencia/dominio` define `Rol` como value object enum con `Nivel() int`, `DominaA(otro Rol) bool` y `Permisos() []Permiso`, sin dependencia de infraestructura.
- El invariante INV-TEN-20 (regla de dominancia) se prueba en el dominio como comparaciones puras de `Rol`, sin mocks ni base de datos.
- Migrar a RBAC configurable en el futuro requiere un ADR nuevo y es responsabilidad exclusiva de quien lo proponga demostrar que el catálogo cerrado ya no alcanza — no es una opción que se "activa".
- La tabla `membresias` almacena el rol como el texto del catálogo cerrado (`CHECK` en el esquema, migración `000009`), no un `role_id` foráneo.

## Estado

Aceptado.
