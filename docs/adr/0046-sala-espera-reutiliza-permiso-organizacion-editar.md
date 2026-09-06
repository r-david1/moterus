# ADR 0046 — La administración de salas org-scoped reutiliza `organizacion.editar`, sin un noveno permiso

## Contexto

Los endpoints de administración de salas de espera con alcance `organizacion` (`POST`/`PATCH`/`GET` sobre `/confianza/organizaciones/{idOrganizacion}/salas-espera`) necesitan un permiso del catálogo cerrado de Tenencia (ADR 0029, 8 permisos) para autorizarse. Ninguno de los ocho fue pensado originalmente para "configurar un mecanismo de perímetro de la organización", así que hay que decidir entre reutilizar uno existente o agregar un noveno.

## Decisión

**Se reutiliza `organizacion.editar`.** Una sala de espera es configuración del perímetro de la organización — quien puede editar la organización (renombrarla, cambiar su alias) puede abrir y ajustar su sala de espera.

Hay precedente literal y directo en el propio contexto Tenencia: el alta directa de miembros (`POST /tenencia/organizaciones/{id}/miembros`) terminó implementada exigiendo `miembro.invitar` en vez de `miembro.cambiar_rol`, porque el catálogo cerrado de 8 permisos no distingue "invitar por correo" de "dar de alta directamente" — ambas comparten permiso y regla de dominancia, y separarlas habría exigido un noveno permiso para una distinción que el producto no pedía todavía (nota de discrepancia, `docs/design/tenencia-bounded-context.md` §7). El mismo criterio aplica acá.

## Alternativas consideradas

- **Agregar un noveno permiso** (p. ej. `perimetro.configurar`): descartado por ahora — el catálogo cerrado de ADR 0029 crece por necesidad demostrada, no por anticipación. Hoy no hay ningún caso de negocio donde "quien edita la organización" y "quien configura su sala de espera" deban ser personas distintas.
- **Reutilizar `miembro.cambiar_rol` u otro permiso ya existente sin relación semántica**: descartado — `organizacion.editar` es el que más se acerca conceptualmente (configuración de la organización como entidad, no de sus miembros).

## Consecuencias

- Cero cambios en el catálogo cerrado de permisos de ADR 0029.
- Si en el futuro aparece un caso de negocio real donde "configurar el perímetro" y "editar la organización" deban separarse (p. ej. un rol operativo que administre eventos sin poder renombrar la organización), el catálogo crece de forma aditiva — un noveno permiso no rompe nada de lo ya emitido, porque la autorización se resuelve por consulta en cada petición (ADR 0030), nunca por un claim cacheado.
- Documentado como decisión menor, resoluble en el momento en que el producto lo pida, siguiendo el mismo patrón ya validado por la nota de discrepancia de Tenencia.

## Estado

Aceptado.
