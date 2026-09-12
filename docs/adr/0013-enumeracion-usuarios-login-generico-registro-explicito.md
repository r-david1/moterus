# ADR 0013 — Enumeración de usuarios: login siempre genérico, registro devuelve conflicto explícito

## Contexto

Cualquier endpoint que reciba un correo y responda distinto según si esa cuenta existe es, por definición, un oráculo de enumeración de usuarios: un atacante puede recorrer una lista de correos candidatos y aprender cuáles están registrados en el sistema, sin necesitar ninguna credencial. Identidad tiene dos endpoints que reciben un correo en un punto de entrada público: `POST /identidad/autenticaciones` (login) y `POST /identidad/usuarios` (registro), y cada uno enfrenta el trade-off de forma distinta.

`docs/design/identidad-bounded-context.md` §7 nombró esta decisión como candidata (0013); se implementó de facto (INV-ID-11 en el código) sin disputarse durante la implementación.

## Decisión

**En login, la protección contra enumeración es absoluta: correo inexistente y contraseña incorrecta producen exactamente el mismo error, con tiempo de respuesta equivalente** (`internal/identidad/aplicacion/autenticar_usuario.go`: "correo inexistente → mismo error genérico + tiempo equivalente"; "contraseña incorrecta → mismo error genérico"). Un atacante no puede distinguir "esa cuenta no existe" de "esa cuenta existe pero la contraseña está mal" — la única señal que un login exitoso da es, precisamente, el éxito.

**En registro, en cambio, se acepta revelar el conflicto explícitamente**: `POST /identidad/usuarios` con un correo ya registrado devuelve `ErrCorreoYaRegistrado` (409), no un éxito genérico ni un error ambiguo.

**Por qué la asimetría es deliberada, no una inconsistencia**: en login, ocultar la existencia de la cuenta protege directamente contra ataques de fuerza bruta dirigidos (si un atacante supiera que una cuenta existe, concentraría sus intentos ahí) y contra el reconocimiento pasivo de una base de usuarios completa. En registro, el costo de ocultarlo sería una degradación de UX real y constante (todo usuario que intente registrarse con un correo que ya tiene cuenta necesita saberlo, para poder ir a "olvidé mi contraseña" en vez de crear una cuenta duplicada fallida) a cambio de una protección que en la práctica es débil: registrar es una acción rara, mientras que loguear ocurre en cada sesión — el rate limiting de Confianza sobre `registro` (que exige captcha y limita por IP) ya acota el costo de usar el endpoint como oráculo de fuerza bruta, y el correo tiene que verificarse antes de que la cuenta quede operativa, lo que ya exige control sobre esa casilla.

## Alternativas consideradas

- **Ocultar también el conflicto en registro** (responder siempre 202/201 genérico, y solo notificar por correo si había una cuenta previa — el mismo patrón que ya usa `ReenviarVerificacionCasoDeUso`): evaluado y descartado para el registro inicial por el costo de UX descrito arriba; es, de hecho, el patrón que **sí** se adoptó para `POST /identidad/usuarios/reenvios-verificacion` (INV-ID-11 extendida), donde el endpoint es de uso repetido y de menor fricción esperar un mensaje neutro.
- **Revelar la existencia de la cuenta también en login** (p. ej. "esa cuenta no existe" vs. "contraseña incorrecta"): descartado de plano — es el oráculo de enumeración más directo posible sobre el endpoint de mayor tráfico del sistema.

## Consecuencias

- `autenticar_usuario.go` y su test dedicado (`TestHTTP_Login_ContrasenaIncorrectaYCorreoInexistente_MismoStatusYCuerpo`) verifican la igualdad de cuerpo y código HTTP entre ambos casos.
- `ReenviarVerificacionCasoDeUso.Reenviar` sigue el mismo criterio de respuesta neutra que login, no el de registro — documentado explícitamente como "mismo criterio que login, INV-ID-11" en su propio comentario.
- Cualquier endpoint público nuevo que reciba un correo tiene que decidir explícitamente en cuál de los dos lados de esta asimetría cae, y justificarlo — no hay un default implícito.

## Estado

Aceptado (implementado de facto; documentado retroactivamente).
