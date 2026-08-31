# ADR 0005 — Auditoría a nivel forense: hash-chaining + append-only por permisos

## Contexto

Se requirió que todo el sistema quede auditado a un nivel defendible forense/legalmente — no solo logging técnico, sino un registro que sirva como evidencia ante un auditor externo o un perito.

## Investigación

Fuentes consultadas sobre tamper-evident/forensic audit logging (2026):

- **Hash-chaining (SHA-256)**: cada registro incluye el hash del registro anterior + sus propios datos — el mismo mecanismo que usan certificate-transparency logs, commits de git y ledgers tipo blockchain. Cualquier alteración, borrado o reordenamiento rompe la cadena de forma matemáticamente detectable.
- **Append-only por permisos, no por convención**: la práctica consistente en las fuentes es reforzar la inmutabilidad con controles reales — `REVOKE` de `UPDATE`/`DELETE` sobre el rol de aplicación, separación entre quien puede modificar el sistema y quien puede alterar el registro de lo que hizo.
- **Qué hace un registro "defendible"**: identidad del actor, autoridad con la que actuó, contexto/intención de la acción, e integridad + cadena de custodia — no basta con que el registro exista, tiene que ser completo y consistente.
- **Anclaje externo** (RFC 3161, firmas de lotes, replicación a almacenamiento WORM independiente) como capa adicional para probar que el registro existía en un momento dado, más allá de su consistencia interna.

## Decisión

Tabla `auditoria` append-only: `REVOKE UPDATE, DELETE` del rol de aplicación + trigger `BEFORE UPDATE OR DELETE` que lanza excepción como defensa adicional. Cada evento incluye `hash_anterior`/`hash_actual` (SHA-256). Se audita desde el primer caso de uso de Identidad, no como fase posterior del proyecto. El anclaje externo (firma de lotes, timestamping RFC 3161, replicación WORM) queda documentado como próximo paso, no bloqueante para el estado actual del proyecto.

## Alternativas consideradas

- **Logging convencional (sin hash-chaining)**: descartado — responde "qué pasó" pero no "cómo pruebo que no fue alterado después", que era el requisito explícito.
- **Confiar solo en permisos de archivo/BD sin hash-chaining**: descartado como única medida — es tamper-resistant pero no tamper-evident; si alguien con privilegios elevados lo altera, no queda rastro matemático, solo el registro de que tenía acceso.
- **Blockchain/DLT completo para el log**: no se adoptó — la complejidad operacional no se justifica en esta etapa; el hash-chaining simple ya da la propiedad de detección que se necesita, y el anclaje externo (cuando se implemente) cubre la necesidad de "prueba de existencia en el tiempo" sin montar una cadena propia.

## Consecuencias

- Todo caso de uso auditable emite su evento de auditoría como parte de la misma transacción cuando es una acción de seguridad crítica (no queda como tarea async "para después").
- Se requiere un job periódico de verificación de integridad de la cadena — pendiente de implementar junto con el resto del contexto Confianza/Auditoría.
- El catálogo de acciones auditables (`accion`) debe mantenerse cerrado y documentado, no como strings libres inventados por cada caso de uso.

## Estado

Aceptado e implementado: `db/migraciones/000002_crear_auditoria` crea la tabla, los triggers de cadena/bloqueo y la separación de roles descrita arriba; verificado funcionalmente contra Postgres real. Pendiente: job periódico de verificación (`cmd/verificador-auditoria`, ya implementado como comando manual) programado como cron/systemd timer, y el anclaje externo (RFC 3161 / WORM) mencionado en Consecuencias.
