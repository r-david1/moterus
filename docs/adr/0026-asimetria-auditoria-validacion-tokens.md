# ADR 0026 — La validación de tokens audita el rechazo por ataque, no el rechazo por expiración

## Contexto

Emisión, renovación, reuso, cierre y revocación de una sesión se auditan siempre (INV-ACC-17), en la misma `UnidadDeTrabajo` que la escritura de negocio — sin excepción, porque son eventos de negocio de baja frecuencia relativa. La validación de un token de acceso (`ValidarAccesoCasoDeUso.Validar`, invocada en **cada** petición autenticada del sistema) es harina de otro costal: es, con enorme diferencia, la operación más frecuente de todo el contexto Acceso. `docs/design/acceso-bounded-context.md` §7 nombró como candidata (0026) si esa validación debía auditarse igual que el resto, o si merecía una regla distinta.

`internal/acceso/aplicacion/validar_acceso.go` implementa una asimetría deliberada: un token expirado (`ErrTokenAccesoExpirado`) se devuelve **sin** llamar a `auditarRechazo`; en cambio, firma inválida, `typ`/`alg`/`kid` inesperado, o una sesión encontrada revocada en la lista de Redis (`SesionRevocada`) sí llaman a `auditarRechazo`, que registra `TokenAccesoRechazado` en la misma cadena forense que el resto del contexto.

## Decisión

**La validación de tokens audita únicamente los rechazos que son señal de ataque, nunca el rechazo por expiración natural.** Un token expirado es, con enorme diferencia, el caso más frecuente entre las peticiones que llegan con un token vencido — ocurre en el camino feliz de cualquier cliente que tarda en renovar, no en un intento de abuso. Los demás motivos de rechazo (firma que no verifica, `kid` desconocido, `typ` distinto del exigido, o una sesión que la lista de revocación marca como revocada pese a presentarse como válida) sí son indicio de que alguien está intentando usar un token que no debería tener, o que ya no debería servir — y esos sí se registran.

La razón operativa es tan importante como la de seguridad: la cadena de hashes de auditoría está serializada por un `pg_advisory_xact_lock` (ADR 0005) para garantizar integridad — es, por diseño, un recurso con un único escritor a la vez. Auditar cada rechazo por expiración pondría el hot path de validación (ejecutado en cada petición autenticada del sistema) detrás de ese lock, convirtiendo el mecanismo forense en el cuello de botella de todo el servicio, y ahogaría la señal real (los pocos rechazos que sí importan) en el ruido de miles de expiraciones normales por hora.

## Alternativas consideradas

- **Auditar todos los rechazos de validación por igual**, sin distinguir motivo: descartado por las dos razones de arriba — coste de rendimiento desproporcionado (serializa el hot path del sistema entero detrás de un advisory lock) y pérdida de señal (un atacante real desaparecería entre miles de expiraciones legítimas en la misma bitácora).
- **No auditar ningún rechazo de validación**, tratando la validación entera como out-of-scope de la auditoría: descartado — perdería la única evidencia forense de un intento real de reuso de sesión revocada o de manipulación de un JWT, que es exactamente el tipo de señal que ADR 0005 existe para capturar.
- **Mover la decisión a una muestra probabilística** (auditar 1 de cada N expiraciones, para tener algo de visibilidad sin el coste completo): descartado — una bitácora forense con hash-chaining pretende ser evidencia completa de lo que pasó, no una muestra; una fila de auditoría que representa "esto pasó, probablemente, unas N veces" no sirve como evidencia.

## Consecuencias

- Un pico anómalo de tokens expirados (p. ej. un cliente con el reloj desincronizado, o un bug que deja de renovar) no deja rastro en la bitácora forense — solo en métricas/logs de aplicación, si existen. Es una limitación de observabilidad aceptada a cambio de no pagar el coste de auditar el camino más transitado del sistema.
- Cualquier nuevo motivo de rechazo de validación que se agregue en el futuro debe clasificarse explícitamente contra esta misma pregunta —¿es un evento esperable del camino feliz, o es indicio de que alguien intenta algo que no debería poder?— antes de decidir si audita o no.
- `internal/acceso/README.md` y el catálogo de auditoría (`docs/catalogos/acciones-auditoria.md`) documentan `TokenAccesoRechazado` como la única acción que este caso de uso puede producir, y solo para los motivos de la lista de arriba.

## Estado

Aceptado (implementado de facto; documentado retroactivamente).
