# ADR 0002 — Alcance de un solo producto (multi-producto pausado)

## Contexto

La idea original era construir el sistema como un **Auth-as-a-Service reutilizable** entre varios proyectos del usuario (varios "productos" distintos), cada uno con sus propios tenants — un usuario con una sola cuenta pudiendo pertenecer a organizaciones de distintos productos, con JWT diferenciado por `aud` según el producto.

Al revisar el caso de uso real, se confirmó que **por ahora el sistema es para un solo producto**.

## Decisión

Se elimina la capa de "producto" del modelo de datos y del dominio. El sistema queda como **auth multi-tenant de un solo producto**: usuarios (globales al servicio) → membresías → organizaciones, sin tabla de productos ni `aud` variable.

## Alternativas consideradas

- **Mantener el modelo multi-producto desde ahora, aunque solo se use un producto**: descartado — es complejidad y una tabla completa (`productos`) que no resuelve ningún problema actual; se prefirió simplicidad sobre construir para una necesidad hipotética.
- **Diseñar "a medias" (dejar el campo pero no usarlo)**: descartado — genera ambigüedad sobre si el campo es requerido o no, y complica las migraciones si después se decide no usarlo nunca.

## Consecuencias

- El JWT usa un `aud` fijo del servicio en vez de uno por producto.
- Si en el futuro se retoma la idea original de reutilizar el servicio entre varios proyectos (`dashboard-vacaciones`, `torneos-deportivos`, `tramitayopal`, `app-barberias`, `parkapp`), se requiere un ADR nuevo y una migración que reintroduzca la tabla de productos y el `aud` variable — no es un simple "activar una opción", es un cambio de modelo de datos.
- Los agentes de Claude Code para este proyecto no mencionan productos ni multi-producto como default.

## Estado

Aceptado.
