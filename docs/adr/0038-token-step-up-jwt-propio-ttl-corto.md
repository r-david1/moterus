# ADR 0038 — Token de step-up: JWT propio de Acceso, TTL de 5 minutos, `typ` distintivo

## Contexto

`docs/design/acceso-bounded-context.md` §2.3 (puertos aplazados) ya nombró `EmisorTokenStepUp`: *"token de vida corta que representa 'credenciales OK, falta el segundo factor' — depende de que Identidad implemente VerificarOTP"*. Con el diseño de OTP/MFA (`docs/design/otp-mfa.md`) hay que fijar exactamente qué es ese token, cuánto vive, y cómo se distingue del token de acceso normal — sin esto fijado, `go-infraestructura` no puede escribir el emisor.

El problema concreto: entre que `POST /acceso/sesiones` confirma que la contraseña es correcta y que el cliente presenta el código OTP, el servidor necesita una forma de decir "ya sé quién sos, pero todavía no termino de verificarte" sin emitir una sesión completa (INV-ACC-03 ya lo prohíbe) y sin obligar al cliente a reenviar la contraseña.

## Decisión

**El token de step-up es un JWT propio, firmado con la misma llave de firma de Acceso (ADR 0020), con un `typ` de cabecera distinto de `at+jwt` y un TTL de 5 minutos, sin refresco posible.**

- **Mismo firmador, mismo `Llavero`**: no hace falta una segunda llave ni un segundo JWKS — es la misma infraestructura criptográfica de ADR 0020, con un `typ` distinto en la cabecera.
- **`typ` distintivo (p. ej. `step-up+jwt`, nunca `at+jwt`)**: cada validador del sistema (el middleware de autenticación de Acceso, Identidad y Tenencia) ya exige `typ == "at+jwt"` como parte de INV-ACC-14 (defensa contra *algorithm confusion*). Un token de step-up con un `typ` distinto queda automáticamente rechazado por cualquier endpoint que no sea explícitamente `POST /acceso/sesiones/segundo-factor` — sin necesitar código nuevo en esos validadores.
- **5 minutos, no 10**: el token de acceso normal vive 10 minutos (ADR 0019) porque protege operaciones ya autenticadas. El token de step-up protege una ventana de fuerza bruta sobre un código de 6 dígitos (10⁶ combinaciones) — cuanto más corta la ventana, menos intentos caben antes de que expire, incluso antes de que el rate limiting de Confianza actúe. 5 minutos es tiempo de sobra para que un humano lea su app autenticadora y tipee el código, y la mitad de la ventana de un token de acceso normal.
- **Sin refresco**: si expira, el cliente vuelve a loguear desde cero (usuario + contraseña). No existe un "refrescar el step-up" — sería alargar indefinidamente la ventana de ataque que el TTL corto está pensado para acotar.
- **Claims mínimos**: `sub` (usuario) y el motivo del step-up (`mfa_habilitado` | `confianza_baja`, el mismo valor que Identidad ya calculaba) — nunca un claim de autorización ni PII, mismo criterio que ADR candidato 0025 de Acceso para el token de acceso normal.

## Alternativas consideradas

- **Reutilizar el token de acceso normal con un claim `step_up_pendiente: true`**: descartada de plano — significaría que cada endpoint protegido del sistema tendría que revisar ese claim antes de confiar en el token, una responsabilidad nueva repartida en todos los validadores existentes, contra exactamente el tipo de error que INV-ACC-14 ya previene con `typ`. Un JWT con un propósito distinto necesita un `typ` distinto, no un claim más para que cada consumidor recuerde revisar.
- **Estado en Redis/Postgres en vez de un JWT**: descartada por el mismo argumento que ya usó ADR 0019 para la sesión completa — un JWT autocontenido no necesita una consulta a base de datos para validarse, y la única razón para preferir estado en servidor (revocación inmediata) no aplica aquí: la ventana de 5 minutos ya es la frontera de seguridad, no hace falta poder revocar un token de step-up antes de que expire por sí solo.
- **TTL de 10 minutos (igual al token de acceso)**: descartada — no hay ninguna razón para que la ventana de fuerza bruta sobre un código de 6 dígitos sea tan larga como la vida útil de una sesión ya autenticada; son amenazas de naturaleza distinta.

## Consecuencias

- `acceso/puertos.EmisorTokenStepUp` (`Emitir`, `Validar`) se implementa sobre el mismo `Llavero` de `internal/acceso/adaptadores/jwt/` (ADR 0020), sin nueva gestión de llaves ni nuevas variables de entorno.
- `ErrSegundoFactorRequerido` gana el campo `TokenStepUp string` — cambio aditivo sobre un tipo ya cerrado.
- Todo validador existente (Acceso, Identidad, Tenencia) sigue rechazando un token de step-up sin ningún cambio de código, porque ya exigen `typ == "at+jwt"`.
- El endpoint nuevo `POST /acceso/sesiones/segundo-factor` es el único punto del sistema que acepta un `typ` distinto — documentado explícitamente para que no se copie el patrón en otro lugar sin la misma justificación.

## Estado

Aceptado.
