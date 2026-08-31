# ADR 0007 — Idioma de los identificadores de código

## Contexto

El ADR 0001 ya usaba `Usuario` y `Correo` como ejemplo del contexto Identidad, y el ADR 0004 fija las tablas en español. Sin embargo, los agentes `go-dominio`, `go-aplicacion` y `base-datos` (en `.claude/agents/`) decían explícitamente "identificadores en inglés", contradiciendo ambos ADRs. El agente `arquitecto-ddd-hexagonal`, al diseñar el bounded context Identidad (`docs/design/identidad-bounded-context.md`), marcó esto como bloqueante antes de escribir código: sin resolverlo, cada agente habría nombrado tipos de forma distinta.

## Decisión

Los identificadores de las capas **dominio, aplicación y puertos** van en **español**, alineados con el lenguaje ubicuo del proyecto y con los nombres de tabla (`Usuario`, `Correo`, `RepositorioUsuarios`, `AutenticarUsuario`, `ErrCredencialesInvalidas`). Se mantiene **inglés** únicamente donde Go lo impone o es convención universal del lenguaje/ecosistema: `ctx context.Context`, `error`, métodos de interfaces estándar (`String()`, `MarshalJSON`, `Error()`), y nombres de paquetes/tipos de librerías de terceros.

La capa de **infraestructura/adaptadores** puede mezclar ambos cuando integra con librerías en inglés (ej. un tipo `sqlc` generado), pero los nombres de los adaptadores propios siguen la misma convención en español que el resto (`RepositorioUsuariosPostgres`, no `PostgresUserRepository`).

## Alternativas consideradas

- **Todo en inglés** (lo que decían los tres agentes antes de este ADR): descartado — es la convención más común en Go/OSS, pero exige traducir mentalmente en cada frontera dominio↔SQL (tablas ya en español por ADR 0004) y contradice el ejemplo ya fijado en ADR 0001.
- **Dominio en español, infraestructura en inglés**: descartado — genera una frontera menos predecible (¿dónde exactamente cambia el idioma?) sin un beneficio claro; los nombres de adaptadores en español no chocan con las librerías de terceros, que se siguen llamando como ellas definen.

## Consecuencias

- Se corrigieron `.claude/agents/02-go-dominio.md`, `03-go-aplicacion.md` y `09-base-datos.md`: ejemplos de entidades, casos de uso y la línea final de convención de idioma quedaron en español.
- `docs/design/identidad-bounded-context.md` ya fue diseñado en español y no requiere cambios.
- Cualquier agente nuevo que implemente código debe asumir español en dominio/aplicación/puertos sin volver a preguntarlo.

## Estado

Aceptado.
