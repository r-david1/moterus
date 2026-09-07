# ADR 0047 — Reconocimiento de origen: tercera extensión de Confianza, no un contexto nuevo (ni de Acceso)

## Contexto

`docs/design/fingerprinting-comportamiento.md` diseña un mecanismo para responder *"¿este intento de login se parece a los intentos que esta cuenta ya hizo antes?"* y convertir la respuesta en fricción graduada (un captcha). Antes de diseñar el mecanismo hay que decidir dónde vive, y a diferencia de ADR 0041 (colas virtuales), acá hay dos candidatos alternativos serios, no solo "un contexto nuevo": **Acceso**, que ya persiste `sesiones.huella_dispositivo` y tiene nombrado el caso de uso aplazado `RecordarDispositivo`; e **Identidad**, que es quien finalmente decide si exige un segundo factor.

## Decisión

**Vive en `internal/confianza/`, como tercera extensión aditiva, con el mismo criterio que ADR 0041 pero con evidencia todavía más fuerte.**

A favor de Confianza:

1. La carta del contexto nombra literalmente *"fingerprinting"* y *"análisis de comportamiento"* junto a rate limiting y colas virtuales.
2. El puerto de entrada ya existe: `confianza/puertos.EvaluadorDeRiesgo` está montado y consumido por Identidad, Acceso y Tenencia, pero hoy no evalúa ningún riesgo real — es un limitador de tasa con un nombre más ambicioso que su contenido.
3. Los campos de salida ya existen y ya están cableados de punta a punta sin que nadie los produzca: `Decision.RequiereStepUp` y `.RequiereCaptcha` se traducen en los tres ACL y llegan hasta `identidad/aplicacion/autenticar_usuario.go`, que ya calcula `requiereSegundoFactor := usuario.TieneMFA() || decision.RequiereStepUp`. Esta extensión es la que hace cierto un nombre de puerto que ya existía.
4. El VO de entrada (`OrigenSolicitud.HuellaDispositivo()`) ya está en `confianza/dominio`, poblado desde `X-Device-Fingerprint`.
5. **El caso es más fuerte que el de las colas virtuales**: esta extensión agrega cero puertos de entrada, cero agregados con ciclo de vida y cero tablas nuevas (frente a los tres puertos y la tabla que sí agregaron las colas).

## El candidato alternativo que hay que responder en serio: ¿no es esto de Acceso?

`sesiones.huella_dispositivo` existe desde la migración `000006`, se puebla en cada login exitoso, y el diseño de Acceso ya nombra `RecordarDispositivo` como caso de uso aplazado. Es una objeción real, no una hipótesis descartable de plano.

**No gana, por la frontera siguiente:**

| | "Dispositivos conectados" (Acceso) | "Origen conocido" (este ADR, Confianza) |
|---|---|---|
| Qué es | una sesión viva que el usuario puede ver y cerrar | un hecho de perímetro: "esta cuenta ya autenticó desde acá antes" |
| Cuándo existe | solo si el login tuvo éxito y emitió sesión | también hace falta saberlo *antes* de emitir nada, en el instante de evaluar |
| Dónde vive | `sesiones`, fuente de verdad de Acceso | índice derivado en Redis, nunca en una tabla de otro contexto |

El argumento decisivo: la evaluación ocurre **antes** de que exista sesión y antes de resolver el usuario (Confianza se evalúa antes de tocar Postgres). Un mecanismo que necesita responder antes de autenticar no puede tener su fuente de verdad en una tabla que solo se escribe después de autenticar, y consultar `sesiones` desde Confianza rompería la frontera hexagonal en la dirección más cara.

**¿Y de Identidad?** No: ADR 0009 es explícito en que Identidad autentica al usuario y no toma decisiones de perímetro. Meter "dispositivos conocidos" dentro del agregado `Usuario` obligaría a cargarlo para evaluar — el trabajo caro que la evaluación previa existe para evitar.

## Alternativas consideradas

- **Contexto nuevo ("Riesgo")**: descartado con el mismo argumento de ADR 0041, reforzado — tendría que redeclarar `OrigenSolicitud`, `Decision`, `Accion` y `Umbral`, y montar su propio adaptador Redis y su propio ACL hacia los tres consumidores, para alojar una función pura y un `HMGET`.
- **Vivir en Acceso**: descartado por la tabla de frontera de arriba — "origen conocido" tiene que existir antes de que Acceso entre en juego.

## Consecuencias

- Confianza gana un puerto de salida nuevo (`PerfilDeOrigenes`) pero cero puertos de entrada nuevos.
- Ningún caso de uso, puerto ni agregado de Identidad, Acceso o Tenencia se modifica más allá de poblar dos campos ya recibidos en sus ACL hacia Confianza.
- La frontera con Acceso queda documentada explícitamente para que nadie intente resolver "dispositivos conocidos" leyendo `sesiones` desde Confianza, ni "origen conocido" leyendo el índice de Confianza desde Acceso.

## Estado

Aceptado.
