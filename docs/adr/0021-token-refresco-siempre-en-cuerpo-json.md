# ADR 0021 — El token de refresco viaja siempre en el cuerpo JSON, nunca en cookie

## Contexto

`docs/design/acceso-bounded-context.md` §7 dejó abierta la decisión del transporte del token de refresco: por defecto una cookie `HttpOnly; Secure; SameSite=Strict; Path=/acceso/sesiones/renovaciones` para clientes navegador, con un modo "cuerpo JSON" opt-in para clientes no navegador (móviles, servidor a servidor). La nombró como candidata (0021) explícitamente por sus aristas: CSRF, clientes móviles, SPAs servidas desde otro dominio que la API.

Al implementarse, esa bifurcación no se construyó. `internal/acceso/adaptadores/http/dtos.go` (`resultadoSesionRespuesta`) devuelve `TokenRefresco` como un campo más del cuerpo JSON, igual que `TokenAcceso`, tanto en `POST /acceso/sesiones` (login) como en `POST /acceso/sesiones/renovaciones` (renovación). No existe ningún código que fije una cookie en ninguna respuesta de Acceso — el propio comentario del DTO en el código deja constancia de que la decisión quedó "sin resolver" respecto del plan original del diseño.

## Decisión

**El token de refresco viaja siempre en el cuerpo JSON de la respuesta, igual que el token de acceso. No existe transporte por cookie.** El cliente lo recibe una vez (en `POST /acceso/sesiones` o en cada `POST /acceso/sesiones/renovaciones`) y es responsable de guardarlo y presentarlo en la siguiente renovación — el mismo contrato para cualquier tipo de cliente (navegador, móvil, servidor a servidor), sin una rama de comportamiento condicionada al tipo de cliente que llama.

Esto simplifica el propio problema que motivó el candidato: sin cookie, no hay superficie CSRF sobre los endpoints de Acceso que dependa de credenciales ambientales — el token de acceso tampoco viaja en cookie (siempre `Authorization: Bearer`), así que ninguna petición de la API es vulnerable a CSRF por construcción, independientemente de qué se decida para el refresco.

## Alternativas consideradas

- **Cookie `HttpOnly` por defecto con modo JSON opt-in** (el plan original del diseño): descartado en la implementación — exige que el servidor sepa distinguir "cliente navegador" de "cliente no navegador" antes de decidir el transporte de la primera respuesta, una heurística frágil (`User-Agent`, cabecera custom) que el propio diseño no llegó a resolver, y que además solo tiene sentido si existe ya un frontend propio consumiendo estas cookies con el dominio correcto — que hoy no existe (ADR 0002: el producto es el servicio de auth, no un frontend).
- **Cookie siempre, sin modo JSON**: descartado — rompería cualquier cliente no navegador (CLI, servidor a servidor, pruebas de integración) sin ganancia real, porque el token de acceso —el que de verdad autoriza cada petición— nunca iba a ir en cookie de todos modos.

## Consecuencias

- Un cliente navegador que use este servicio directamente es responsable de guardar el token de refresco fuera de una cookie `HttpOnly` (p. ej. en memoria, nunca en `localStorage` si se puede evitar) — una responsabilidad que hoy recae en el integrador, no en el servicio. Es una limitación real frente al plan original, aceptada porque no hay hoy un frontend propio que la sufra.
- Revisar esta decisión si en algún momento existe un frontend de primera parte para este servicio: en ese escenario sí valdría la pena la cookie `HttpOnly`, porque el dominio y el ciclo de vida del frontend estarían controlados por el mismo equipo, y las aristas de CSRF/SameSite dejarían de ser una incógnita operativa.
- `docs/design/acceso-bounded-context.md` §7 queda desactualizado en este punto — el diseño original describe el plan de cookie condicional que no se construyó; el código es la fuente de verdad.

## Estado

Aceptado (implementado de facto; documentado retroactivamente).
