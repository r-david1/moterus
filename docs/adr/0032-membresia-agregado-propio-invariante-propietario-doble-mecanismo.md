# ADR 0032 — `Membresia` es agregado propio; "al menos un propietario" se sostiene con candado de fila + trigger diferido

## Contexto

`docs/design/tenencia-bounded-context.md` §1.2 tenía que resolver dos preguntas relacionadas y las agrupó bajo la candidata 0032: (a) si `Membresia` vive como colección dentro del agregado `Organizacion` o como agregado propio referenciado por identificador, y (b) cómo se sostiene INV-TEN-06 ("una organización activa siempre tiene al menos un propietario activo"), que por construcción es un invariante **entre** agregados si (a) se responde con "agregado propio".

El repositorio tenía dos precedentes opuestos para (a): Identidad sacó `FactorMFA`/`CodigoOTP` de `Usuario` por tener ciclo de vida y frecuencia de escritura distintos; Acceso metió `TokenRefrescoEmitido` dentro de `Sesion` porque un token de refresco no existe fuera de una sesión y la operación "rotar" es indivisible entre ambos. `Membresia` cumple la primera mitad del criterio de Acceso (no existe fuera de una organización) pero falla la segunda: cambiarle el rol a un miembro no muta la organización, y una organización puede tener miles de miembros — cargar la colección completa para tocar una fila es el olor que Identidad rechazó.

## Decisión

**`Membresia` es un agregado propio**, referenciado por `IDOrganizacion`/`IDUsuario`, no una colección dentro de `Organizacion` — gana el precedente de Identidad. La unicidad de membresía activa por (organización, usuario) se garantiza con un índice único **parcial** de Postgres (`membresias_vigente_idx`, `WHERE estado <> 'removida'`), nunca con un "SELECT antes de INSERT" en la capa de aplicación: es el mismo patrón, verbatim, que `tokens_refresco_vigente_por_sesion_idx` de Acceso (INV-ACC-04) y la unicidad de correo de Identidad (INV-ID-02). Es parcial y no total porque `Membresia` nunca se borra físicamente (INV-TEN-08) — remover a alguien transiciona la fila a `removida`, y una re-admisión posterior crea una fila **nueva**; un índice total dejaría la clave ocupada para siempre por la fila histórica.

El costo explícito de haber sacado `Membresia` de `Organizacion` es que INV-TEN-06 pasa a ser un invariante entre agregados, que por definición no se sostiene dentro de uno solo. Se sostiene con **dos mecanismos complementarios**, ninguno sustituto del otro:

1. **Serialización en el caso de uso**: todo caso de uso que pueda reducir el conteo de propietarios (`CambiarRol`, `RemoverMiembro`, `AbandonarOrganizacion`, `TransferirPropiedad`, `AgregarMiembro`) llama primero a `RepositorioOrganizaciones.CargarParaActualizar` (`SELECT ... FOR UPDATE` sobre la fila de `organizaciones`) y solo entonces cuenta propietarios activos. La fila raíz actúa como *token de consistencia* del conjunto de membresías — da el error de negocio correcto y accionable (`ErrUltimoPropietario`, por ejemplo) antes de tocar la base.
2. **Garantía estructural en Postgres**: dos `CONSTRAINT TRIGGER ... DEFERRABLE INITIALLY DEFERRED` (`membresias_al_menos_un_propietario` sobre `membresias`, `organizaciones_al_menos_un_propietario` sobre `UPDATE OF estado` en `organizaciones`) que, al hacer commit, rechazan cualquier organización `activa` que se quede sin propietario activo. Cubren los dos lados del invariante: perder al último propietario por una mutación en `membresias`, y reactivar una organización cuyos propietarios fueron removidos mientras estaba suspendida. Diferidos a propósito: `CrearOrganizacion` inserta la organización antes que la membresía, y `TransferirPropiedad` pasa por estados intermedios legítimos dentro de la misma transacción — un trigger inmediato rompería ambos flujos.

## Alternativas consideradas

- **`Membresia` como colección dentro de `Organizacion`**: descartado — obligaría a cargar y bloquear la organización completa (potencialmente miles de membresías) para una operación tan frecuente y localizada como cambiar el rol de un solo miembro.
- **Solo el candado de fila (sin trigger)**: descartado — depende de que **todo** caso de uso presente y futuro recuerde llamar a `CargarParaActualizar` antes de mutar. Un caso de uso nuevo que lo olvide, o una mutación directa a la base fuera de la aplicación, rompería el invariante sin que nada lo impida.
- **Solo el trigger (sin candado)**: descartado — el trigger solo puede abortar la transacción entera con un error genérico de constraint; sin el candado de fila, dos transacciones concurrentes que reducen el conteo de propietarios pasarían ambas su propia validación en memoria antes de que cualquiera haga commit, y una de las dos fallaría con un error de base de datos crudo en vez de un error de dominio tipado y accionable.
- **Consulta previa en el caso de uso, sin índice único**, para la unicidad de membresía: descartado — dos peticiones concurrentes de invitar/re-admitir al mismo usuario podrían pasar ambas la comprobación antes de insertar ("check-then-act" sin aislamiento serializable), produciendo dos membresías activas para el mismo par.
- **Índice único total** sobre `(organizacion_id, usuario_id)`: descartado — rompería la re-admisión de un miembro removido (INV-TEN-08 exige una fila nueva, no la reutilización de la removida).

## Consecuencias

- Cualquier caso de uso nuevo que pueda reducir el conteo de propietarios de una organización **debe** llamar a `CargarParaActualizar` primero — es una convención que ningún compilador verifica, así que el trigger diferido es la red de seguridad real, no una redundancia decorativa.
- La aplicación puede seguir haciendo una consulta previa de unicidad como optimización de UX (mensaje de error más específico, sin depender del texto de una violación de índice de Postgres), pero nunca puede confiar únicamente en ella: el índice es la frontera real, y el caso de uso debe traducir el error de unicidad de Postgres (`23505`) a su error de dominio tipado si la consulta previa no lo atrapó primero.
- Cualquier value object o entidad nueva del proyecto que necesite "a lo sumo una fila activa por clave, pero historial conservado" debe usar el mismo patrón de índice único parcial condicionado al estado — ya es la tercera vez que aparece (Identidad, Acceso, Tenencia). Cualquier invariante nuevo **entre** agregados que dependa de un conteo debe evaluarse contra el mismo par candado-de-fila-raíz + trigger diferido, no contra uno solo de los dos.

## Estado

Aceptado (implementado de facto; documentado retroactivamente).
