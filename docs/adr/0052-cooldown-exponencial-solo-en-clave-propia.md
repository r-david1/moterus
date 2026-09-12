# ADR 0052 — El cooldown exponencial del limitador de tasa se acota por nivel: escala en IP, nunca más allá de la ventana en cuenta

## Contexto

ADR 0018 implementó el limitador de tasa de Confianza con un cooldown exponencial (`scriptPermitir`, `internal/confianza/adaptadores/redis/limitador_tasa.go`): cada solicitud que sigue llegando después de superado el límite duplica el TTL de bloqueo restante, con un tope de 2 horas (`backoffMaximoPorDefecto`). Ese script se aplica sin distinción tanto a la clave de rate limiting por IP (`confianza:rl:ip:login:<ip>`) como a la clave por cuenta (`confianza:rl:cuenta:login:<correoNormalizado>`).

Al diseñar el candidato 0011 (bloqueo de cuenta, ver ADR 0011) se encontró que esa falta de distinción es un bug de seguridad ya en producción, no una hipótesis.

## El hallazgo, verificado con la aritmética exacta

La clave de IP y la clave de cuenta tienen una diferencia esencial que ADR 0018 no analizó: **quién posee la clave y quién paga el costo del escalado no son la misma persona en ambos casos**.

| Clave | Quién posee la clave | Quién provoca el escalado | Quién paga el cooldown |
|---|---|---|---|
| `confianza:rl:ip:login:<ip>` | el atacante (es su red) | el atacante | el atacante — el escalado es un castigo bien dirigido |
| `confianza:rl:cuenta:login:<correo>` | **la víctima** (es su identidad) | el atacante | **la víctima** — el escalado castiga a quien no hizo nada |

Con el umbral real de `login`/cuenta (5 intentos, ventana de 15 minutos) y el escalado `ventana × 2^exceso`: el intento 6 mantiene el TTL en 15 min (sin escalar todavía, `exceso=1`), el 7º lo lleva a 30 min, el 8º a 60 min, y el 9º alcanza el tope de 2 horas (`120 min`, que coincide con `backoffMaximoPorDefecto`). **Con 9 peticiones HTTP, cualquiera que conozca el correo de una cuenta la deja en cooldown de 2 horas**, sin necesitar ninguna credencial ni saber si la cuenta existe.

La única salida de ese cooldown es un captcha con puntaje aceptable — pero en un despliegue de producción sin `TURNSTILE_SECRET_KEY` configurada, el verificador de captcha es fail-closed por diseño (ADR 0003/0018), así que ningún token puede ser "aceptable" y esa salida no existe: el cooldown de 2 horas se vuelve un bloqueo duro sin bypass.

**Nota de exactitud documental**: la sección "Consecuencias" de ADR 0018 describe este escenario como *"un bloqueo duro de 15 minutos"*. Con el escalado exponencial del mismo ADR, en realidad son hasta 2 horas — una subestimación de 8×. Se corrige el texto de ADR 0018 en el mismo movimiento que este ADR.

## Decisión

**El cooldown exponencial se aplica solo al nivel `ip`. El nivel `cuenta` nunca extiende su bloqueo más allá de su ventana nominal**, sin importar cuántas solicitudes adicionales reciba mientras está en cooldown (INV-BLQ-05).

Forma estructural, no una comprobación en el punto de llamada:

```go
// internal/confianza/dominio/umbral.go
type Umbral struct {
    Limite  int
    Ventana time.Duration
    // BackoffMaximo acota el cooldown exponencial de esta clave. Cero
    // usa el tope por defecto del adaptador (2h), correcto para IP.
    BackoffMaximo time.Duration
}

func (p PoliticaLimites) Para(accion Accion) LimitesPorAccion {
    limites := /* ... lookup con fallback fail-safe ... */
    // Estructural: ninguna acción, ni siquiera la rama fail-safe de una
    // acción no registrada, puede producir un umbral de cuenta con
    // escalado — el escalado castiga a quien posee la clave, y la
    // clave de cuenta la posee la víctima potencial, no el atacante.
    limites.Cuenta.BackoffMaximo = limites.Cuenta.Ventana
    return limites
}
```

El adaptador (`limitador_tasa.go`) pasa `umbral.BackoffMaximo` al script Lua cuando es mayor que cero, y cae al tope por defecto del adaptador (2h) cuando es cero — que es exactamente el caso de IP, cuyo `Umbral` no fija el campo. No hay cambio de firma en `puertos.LimitadorTasa`, no hay migración, no se reabre ADR 0018 (se corrige un parámetro cuyo efecto asimétrico ese ADR no llegó a analizar).

## Alternativas consideradas

- **Dejar el escalado como está y compensar con una lista de "correos protegidos"** o alguna excepción manual: descartado — no escala, requiere mantenimiento humano continuo, y no protege a nadie hasta que alguien lo agregue a la lista después del primer ataque.
- **Quitar el escalado también de la clave de IP**: descartado — ahí el análisis de ADR 0018 sigue siendo correcto: el escalado castiga a quien insiste desde su propia red, exactamente la dirección de incentivo deseada.
- **Bajar el tope general a algo menor que 2h para ambos niveles**: descartado — no resuelve el problema de fondo (que el escalado se aplica a una clave que el atacante no posee), solo lo acorta; y para IP, 2 horas sigue siendo la decisión correcta de ADR 0018.

## Consecuencias

- **Intercambio aceptado explícitamente**: un atacante de credential stuffing distribuido recupera un presupuesto de ~480 intentos/día/cuenta (en vez de decaer a ~60/día con el escalado previo). Se acepta porque, contra contraseñas de al menos 12 caracteres filtradas por HIBP (ADR 0014), 480 intentos diarios siguen sin ser un presupuesto de fuerza bruta significativo — y esta decisión queda explícitamente condicionada a que ADR 0014 siga vigente: si algún día se relajara la política de contraseñas, este intercambio habría que revisar.
- **Riesgo residual que queda vivo**: el DoS dirigido baja de 2 horas a 15 minutos por ráfaga, no a cero — un atacante que sostenga 5 intentos cada 15 minutos mantiene a la víctima en cooldown de forma indefinida. Pero la víctima conserva la salida por captcha en todo momento (si `TURNSTILE_SECRET_KEY` está configurada — ver ADR 0053), el atacante tiene que martillear continuamente en vez de pagar 9 peticiones una sola vez, y cada intento produce una fila auditada. La contención pasa de "barata y silenciosa" a "sostenida y ruidosa".
- `ConBackoffMaximo` (la opción de test del adaptador) sigue existiendo sin cambios.
- Corrección del texto de ADR 0018 §Consecuencias: "bloqueo duro de 15 minutos" pasa a reflejar el comportamiento real anterior a este ADR (hasta 2 horas) y el comportamiento posterior (acotado a la ventana nominal para cuenta).

## Estado

Aceptado.
