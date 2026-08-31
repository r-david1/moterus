# ADR 0006 — Huma sobre Fiber para documentación OpenAPI automática

## Contexto

Al construir un servicio en Go (ver ADR 0001), la generación automática de documentación OpenAPI no viene resuelta por defecto — a diferencia de frameworks de otros ecosistemas donde el modelo de datos tipado ya trae el schema incorporado, Go no tiene un equivalente nativo.

## Investigación

Dos caminos evaluados para cerrar esa brecha:

- **swaggo/swag**: genera la spec a partir de comentarios anotados manualmente sobre cada handler + comando `swag init`. Documenta, pero no valida — la validación de input se sigue haciendo aparte (`go-playground/validator`), y el comentario puede desincronizarse del código real.
- **Huma v2**: framework que envuelve el router (adapter oficial para Fiber en `adapters/humafiber`) y genera OpenAPI 3.1 + UI de docs automáticamente a partir de los structs de entrada/salida de cada operación, con validación incluida vía tags de struct (`format`, `minLength`, `doc`...). El modelo de datos ES la documentación y la validación, no algo aparte que hay que mantener sincronizado a mano.

Verificación práctica: `huma v2.34.0` es la última versión compatible con Go 1.24 — las versiones más nuevas (`v2.35.0+`) exigen Go 1.25, lo cual importa si el entorno de build no está actualizado.

## Decisión

Adoptar **Huma v2** (`adapters/humafiber`, función `humafiber.NewV2`) montado sobre Fiber v2. Fiber se mantiene como router base (ADR 0001 no se reabre); Huma se agrega como capa de definición de operaciones, validación y generación de documentación.

Consecuencia directa: `go-playground/validator` deja de usarse para lo que pasa por operaciones registradas en Huma — la validación vive en los tags del struct de entrada.

## Alternativas consideradas

- **swaggo/swag**: descartado como default — mantener documentación y validación como dos cosas separadas obliga a sincronizarlas a mano en cada cambio. Queda como opción de respaldo si algún endpoint no encaja bien en el modelo de structs de Huma.

## Consecuencias

- `/openapi.json` y la UI de docs en `/docs` se generan solos con cada operación nueva — el agente `documentacion` deja de mantener ese archivo a mano, solo verifica que esté enlazado desde la guía de integración.
- Fijar la versión de Huma en el `go.mod` según la versión de Go del entorno de build (`v2.34.0` para Go 1.24; revisar antes de subir a una versión más nueva si se actualiza el toolchain a 1.25+).

## Estado

Aceptado.
