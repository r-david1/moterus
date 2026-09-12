# ADR 0010 — `UnidadDeTrabajo` como puerto, con la transacción viajando dentro del `context.Context`

## Contexto

ADR 0005 exige que cada mutación de negocio y su fila de auditoría correspondiente se persistan en la misma transacción de Postgres (si la fila de negocio se guarda pero la auditoría falla, o viceversa, queda un hueco en la cadena hash-chained). Los casos de uso de Identidad necesitan una forma de expresar "estas dos o tres escrituras van juntas o no va ninguna" sin que la capa de aplicación conozca `pgx` ni ningún detalle del driver de base de datos.

`docs/design/identidad-bounded-context.md` §7 nombró esta decisión como candidata (0010) al diseñar el hexágono; se implementó de facto junto con el resto de la capa de aplicación e infraestructura sin que hiciera falta disputarla durante la implementación, así que nunca se separó en su propio documento hasta ahora.

## Decisión

**`UnidadDeTrabajo` es un puerto de salida con una única forma:**

```go
type UnidadDeTrabajo interface {
    Ejecutar(ctx context.Context, fn func(ctx context.Context) error) error
}
```

El caso de uso llama `uow.Ejecutar(ctx, func(ctx context.Context) error { ... })` y, dentro de esa función, invoca a los repositorios normalmente — **la transacción activa viaja implícita dentro del `ctx`** que la implementación de `Ejecutar` construye y pasa a la función. Cada repositorio, al recibir ese `ctx`, comprueba si ya hay una transacción publicada (vía una clave no exportada del paquete de infraestructura) y la usa si existe, o cae al pool compartido si no.

## Alternativas consideradas

- **Pasar `*pgx.Tx` explícito por la firma de cada método del puerto**: descartada — contaminaría `puertos/salida.go` (y por lo tanto `aplicacion/`) con un tipo del driver, rompiendo la regla de que la capa de aplicación no conoce infraestructura concreta.
- **Un patrón de "unit of work" con métodos `Commit`/`Rollback` explícitos que el caso de uso invoca a mano**: descartada — es más fácil de usar mal (olvidar el `Rollback` en un `return` temprano) que un cierre (`func`) que la propia implementación de `Ejecutar` garantiza cerrar en cualquier salida, con o sin error.
- **Una transacción ambiente global (goroutine-local o similar)**: Go no tiene almacenamiento por goroutine; `context.Context` es el mecanismo idiomático para este tipo de valor con alcance de una sola petición.

## Consecuencias

- Todo caso de uso que escriba negocio + auditoría en la misma operación usa `uow.Ejecutar` exactamente con esta forma — patrón ya replicado en Acceso, Tenencia y Confianza (colas de acceso virtual, reconocimiento de origen).
- El `ctx` de Go pasa a llevar un valor implícito (la transacción activa) que hay que documentar explícitamente en el comentario del puerto, para que nadie construya un `context.Context` "limpio" a mitad de un flujo transaccional por error.
- Los mocks de test (`puertos/mocks/mocks.go`) implementan `UnidadDeTrabajo.Ejecutar` invocando `fn(ctx)` directamente, sin ninguna transacción real — suficiente para los tests unitarios de aplicación, que no verifican atomicidad (eso lo hacen los tests de integración contra Postgres real).

## Estado

Aceptado (implementado de facto; documentado retroactivamente).
