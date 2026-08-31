# ADR 0008 — Argon2id como algoritmo de hashing de contraseñas, con parámetros fijos

## Contexto

El puerto `HasherContrasenas` (sección 2.2 del diseño de Identidad) necesita una implementación concreta antes de que `go-infraestructura` pueda construir el adaptador. La elección de algoritmo y de parámetros de costo afecta directamente el presupuesto de CPU por login — hay que fijar los números, no dejarlos "a discreción del adaptador".

## Decisión

**Argon2id** (variante híbrida, resistente tanto a ataques de canal lateral como a GPU cracking), vía `golang.org/x/crypto/argon2`, con estos parámetros iniciales:

| Parámetro | Valor | Nota |
|---|---|---|
| Memoria | 64 MiB (`m=65536`) | Piso recomendado por RFC 9106 para uso interactivo. |
| Iteraciones | `t=3` | |
| Paralelismo | `p=1` | Un solo hilo — evita que el costo real dependa del número de cores del host. |
| Tamaño de sal | 16 bytes, aleatoria por hash (`crypto/rand`) | Nunca reutilizada. |
| Tamaño de salida | 32 bytes | |
| Formato de almacenamiento | Cadena codificada estilo PHC (`$argon2id$v=19$m=65536,t=3,p=1$<sal>$<hash>`) | Autocontenida: permite subir parámetros sin migrar filas viejas. |

`golang.org/x/crypto` es la misma familia que `golang.org/x/text` (ADR/excepción ya aprobada en el dominio para `Correo`, ver INV-ID-18) pero aquí no aplica esa restricción: `HasherContrasenas` es un **adaptador** en `identidad/adaptadores`, no vive en `identidad/dominio` — INV-ID-18 (cero dependencias externas) es una regla del paquete `dominio`, no del proyecto completo.

`NecesitaRehash(hash) bool` en el puerto compara los parámetros embebidos en el hash almacenado contra los parámetros vigentes de la constante del adaptador; si difieren, el caso de uso `AutenticarUsuario` dispara `ReemplazarHash` tras una autenticación exitosa (INV-ID-13) — así subir memoria/iteraciones en el futuro no exige una migración masiva ni forzar un reset de contraseñas.

**bcrypt** se mantiene solo como ruta de lectura para hashes heredados si en algún momento se migra una base de usuarios existente (no aplica hoy — greenfield) — no se implementa hasta que haya un caso real de migración.

## Alternativas consideradas

- **bcrypt como algoritmo principal**: descartado — sin resistencia a ataques por GPU/ASIC comparable a Argon2id; sigue siendo aceptable pero ya no es la recomendación vigente de OWASP.
- **scrypt**: descartado — Argon2id es el ganador de la Password Hashing Competition y la recomendación explícita de RFC 9106 y OWASP; no hay razón para preferir scrypt sobre Argon2id en un proyecto nuevo.
- **Parámetros más altos (p. ej. 256 MiB)**: descartado por ahora — 64 MiB/t=3/p=1 es el piso recomendado por RFC 9106 para uso interactivo (login síncrono con el usuario esperando respuesta); subir el piso es una decisión operacional reversible vía `NecesitaRehash`, no hace falta sobre-invertir antes de tener datos reales de carga.

## Consecuencias

- El adaptador Argon2id vive en `internal/identidad/adaptadores/cripto`, no en `dominio` — coherente con INV-ID-18.
- Cambiar los parámetros en el futuro (subir memoria/iteraciones) no requiere migración de datos: se detecta y corrige de forma oportunista vía `NecesitaRehash` + `ReemplazarHash` en el siguiente login exitoso de cada usuario.
- El presupuesto de CPU por login queda acotado y conocido desde el diseño, relevante para dimensionar rate limiting (Confianza) y capacidad del servicio.

## Estado

Aceptado. Implementación pendiente en el agente `go-infraestructura`.
