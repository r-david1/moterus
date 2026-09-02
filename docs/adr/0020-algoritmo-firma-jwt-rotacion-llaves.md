# ADR 0020 — Algoritmo de firma del token de acceso y rotación de llaves

## Contexto

ADR 0019 fijó que el token de acceso es un JWT con firma **asimétrica** (nunca HMAC, INV-ACC-13) y JWKS público en `/.well-known/jwks.json`, pero dejó explícitamente para este ADR: (1) la elección concreta entre EdDSA/Ed25519 y RS256, (2) el formato del `kid`, y (3) el mecanismo de rotación de llaves. Sin esto fijado, `go-infraestructura` no puede escribir `FirmadorTokensAcceso` — es una dependencia dura de la implementación, no un detalle pulible después.

## Decisión

### 1. Algoritmo: **EdDSA/Ed25519** como firma primaria; RS256 queda soportado por el verificador como escape

Ed25519 da firmas de 64 bytes (contra ~256 de RSA-2048), verificación más rápida, y no tiene parámetros que configurar mal (a diferencia de RSA, donde el tamaño de llave y el padding sí importan). El JWKS puede publicar llaves de ambos tipos a la vez con `kid` distintos, así que cambiar el algoritmo de firma activo es un evento de rotación de llaves, no una ruptura de compatibilidad: `FirmadorTokensAcceso.Firmar` firma siempre con la llave activa (Ed25519 desde el día uno), y `ValidadorDeAccesos` verifica cualquier `alg` presente en el JWKS vigente. HMAC (HS256) queda descartado sin reabrir la discusión — ya la cierra INV-ACC-13/ADR 0019.

### 2. Biblioteca: `github.com/lestrrat-go/jwx/v2`

El proyecto no tiene hoy ninguna dependencia de JWT/JWK en `go.mod`. Se adopta `jwx/v2` (paquetes `jwt`, `jwk`, `jws`) sobre alternativas como `golang-jwt/jwt`, porque construir y servir un JWKS real (RFC 7517) — con thumbprints RFC 7638 y soporte nativo de `OKP`/Ed25519 — es responsabilidad de primera clase de `jwx`, mientras que `golang-jwt` da solo la parte de firma/verificación y dejaría el JWKS a mano. `FirmadorTokensAcceso` y `ValidadorDeAccesos` (puertos de `internal/acceso/puertos`) encapsulan la biblioteca: ningún tipo de `jwx` cruza esos puertos hacia `aplicacion` ni `dominio` (INV-ACC-18).

Verificaciones obligatorias en el validador, cada una con su propio test (mismo criterio que la lista de mitigaciones de ADR 0019):
- Rechazo explícito de `alg: none`.
- El algoritmo de verificación se deriva del `kid` presente en el JWKS, **nunca** del campo `alg` de la cabecera del token (evita *algorithm confusion*).
- `typ` de cabecera debe ser `at+jwt` (RFC 9068); cualquier otro valor se rechaza.
- `iss` y `aud` se comparan siempre contra los valores de configuración (`ACCESO_EMISOR`/`ACCESO_AUDIENCIA`), nunca se confía en el valor del claim por sí solo.

### 3. `kid`: thumbprint RFC 7638 de la llave pública

`kid = base64url(SHA-256(JWK_canónico))`. Determinista a partir de la llave misma (no un contador ni un UUID arbitrario): dos despliegues que cargan la misma llave producen el mismo `kid`, lo que hace el JWKS idempotente ante reinicios y evita colisiones accidentales entre entornos.

### 4. Fase 1 (MVP): llaves en configuración, rotación por despliegue

- La llave privada activa se provee por variable de entorno (`ACCESO_LLAVE_FIRMA`, PEM o seed de 32 bytes en base64 para Ed25519); no se genera ni persiste en base de datos todavía.
- El conjunto de verificación (lo que expone `/.well-known/jwks.json`) incluye la llave activa **más** cualquier llave anterior configurada explícitamente (`ACCESO_LLAVES_VERIFICACION_PREVIAS`, lista separada por comas), que debe mantenerse publicada durante al menos `vidaTokenAcceso` (10 minutos) después de rotar — tiempo suficiente para que ningún token firmado con la llave saliente quede sin forma de verificarse.
- Rotar una llave es: generar el nuevo par, desplegar con la nueva como activa y la saliente añadida a las previas, esperar ≥ `vidaTokenAcceso`, y en un despliegue posterior quitar la llave saliente de las previas. Es un procedimiento operativo, no código.
- **Arranque**: en `APP_ENV=production` sin `ACCESO_LLAVE_FIRMA` configurada, el proceso `api` no arranca (mismo criterio que ADR 0019 §Consecuencias — un servicio de autenticación que no puede firmar no tiene nada que hacer sirviendo tráfico). Fuera de producción, si falta, se genera una llave Ed25519 efímera en memoria con `WARN` explícito de que todos los tokens firmados mueren al reiniciar el proceso.
- **Rotación con estado en base de datos** (múltiples llaves activas simultáneas gestionadas sin redeploy, historial de llaves consultable) queda fuera del MVP — es el candidato reservado como `RepositorioLlavesFirma` en la sección 2.3 del diseño del contexto.

## Alternativas consideradas

- **RS256 como algoritmo primario**: descartado como *primario* (queda como escape soportado por el verificador) porque no aporta nada sobre Ed25519 en este proyecto — no hay hoy ningún consumidor no-Go con soporte deficiente de EdDSA que lo obligue, y las firmas más pequeñas y la ausencia de parámetros mal configurables pesan más que la "familiaridad" de RSA.
- **`golang-jwt/jwt` en vez de `jwx/v2`**: descartada para esta fase porque habría que construir el servido de JWKS (serialización RFC 7517, cálculo de thumbprint RFC 7638) a mano; `jwx` ya lo resuelve y reduce la superficie de código propio manejando bytes criptográficos.
- **`kid` como UUID aleatorio por llave**: descartado — no determinista, dos instancias con la misma llave (p. ej. en un despliegue con múltiples réplicas leyendo la misma variable de entorno) publicarían `kid` distintos para la misma llave, rompiendo la idempotencia del JWKS.
- **Rotación de llaves con estado en Postgres desde el MVP**: descartada por ahora — el mecanismo por variable de entorno es suficiente para el volumen de despliegues actual (ADR 0002: un solo producto) y evita añadir una tabla y su ciclo de vida antes de tener un caso de uso real que lo requiera.

## Consecuencias

- Nueva dependencia en `go.mod`: `github.com/lestrrat-go/jwx/v2` (y sus transitivas). Es la única biblioteca de criptografía de tokens del proyecto; ningún otro paquete debe importar `jwt`/`jwk`/`jws` directamente salvo los adaptadores detrás de `FirmadorTokensAcceso`/`ValidadorDeAccesos`.
- Configuración nueva obligatoria en producción: `ACCESO_LLAVE_FIRMA`, `ACCESO_EMISOR`, `ACCESO_AUDIENCIA`; opcional `ACCESO_LLAVES_VERIFICACION_PREVIAS` durante una ventana de rotación.
- La rotación de llaves es, por ahora, un procedimiento manual de despliegue documentado aquí, no una operación de la API. Si el volumen de operación lo justifica más adelante, migra al candidato `RepositorioLlavesFirma` sin romper el contrato del puerto (que ya expone `LlavesPublicas` como una operación async del firmador).
- El endpoint `/.well-known/jwks.json` se vuelve público por diseño (es exactamente su propósito): no requiere autenticación ni pasa por `EvaluadorConfianza`.

## Estado

Aceptado. Implementación pendiente en `internal/acceso/adaptadores/` (`FirmadorTokensAcceso` sobre `jwx/v2`).
