# ADR 0051 — La escalada del reconocimiento de origen es solo captcha, nunca step-up; modo `observar` por defecto

## Contexto

`Decision.RequiereStepUp` existe desde ADR 0018, se traduce en los tres ACL de Confianza, y `identidad/aplicacion/autenticar_usuario.go` ya calcula `requiereSegundoFactor := usuario.TieneMFA() || decision.RequiereStepUp` con `MotivoStepUp = "confianza_baja"` — pero hoy nada produce `RequiereStepUp: true`, así que ese camino nunca se ejercita en producción. Al diseñar el reconocimiento de origen (`docs/design/fingerprinting-comportamiento.md`), la respuesta obvia a "¿qué hacemos cuando el riesgo es alto?" sería "exigimos segundo factor, para eso está el campo" — y esa respuesta obvia es incorrecta.

## El hallazgo

La cadena, verificada en el código real:

1. `Decision.RequiereStepUp` llega a `identidad/aplicacion/autenticar_usuario.go`, que decide `requiereSegundoFactor` y, si el usuario no tiene MFA propio, fija `MotivoStepUp = "confianza_baja"`.
2. `acceso/aplicacion/iniciar_sesion.go` no emite sesión; devuelve `ErrSegundoFactorRequerido` con un token de step-up de 5 minutos (ADR 0038).
3. El cliente llama a `POST /acceso/sesiones/segundo-factor` con un código.
4. `identidad/aplicacion/verificar_otp.go` busca los factores MFA confirmados del usuario. Si no tiene ninguno (exactamente el caso de un usuario sin MFA al que se le exigió step-up por `confianza_baja`):

```go
if len(factores) == 0 {
    // INV-MFA-06: esto no debería poder pasar nunca (quien invoca este
    // puerto ya decidió que hacía falta un segundo factor).
    slog.ErrorContext(ctx, "VerificarOTP invocado sin ningún factor MFA confirmado: inconsistencia interna", ...)
    return false, nil
}
```

El comentario dice *"esto no debería poder pasar nunca"*, y es correcto hoy — porque nada produce `RequiereStepUp`. En el instante en que un mecanismo lo produjera para un usuario sin MFA, ese `return false, nil` se convierte en un **bloqueo permanente**: el usuario recibe `ErrCredencialesRechazadas` genérico (INV-MFA-08 colapsa el motivo), reintenta el login, vuelve a puntuar riesgoso porque su dispositivo sigue siendo el mismo "nuevo", y entra en un bucle sin salida por sí mismo. El mecanismo que existiría para proteger la cuenta sería el que se la quita a su dueño legítimo.

## Decisión

**El reconocimiento de origen no produce `Decision.RequiereStepUp` en ningún caso. Su única escalada posible es `RequiereCaptcha`, resoluble por el usuario en el mismo intento, sin nada preinscrito. Además, el modo por defecto del mecanismo es `observar` (no cambia el desenlace de ningún login) y pasar a `exigir_captcha` es una decisión operativa explícita.**

Esto no es un descuido de este diseño: es una precondición que se documenta y se respeta. `Decision.RequiereStepUp` queda intacto — no se elimina, no se deprecia — para el día en que exista un segundo factor universal (por correo, u otro mecanismo que no dependa de que el usuario ya tenga MFA confirmado). Cerrar ese hueco (decidir entre un segundo factor de respaldo, una verificación de dispositivo por enlace de correo, o exigir que solo usuarios con MFA reciban `RequiereStepUp`) es un hito con su propio ADR, no un renglón de éste.

**Por qué `observar` es el default, con dos motivos independientes**:

1. **Calibración sin datos**: los pesos y umbrales del mecanismo (ver `docs/design/fingerprinting-comportamiento.md` §1.5) no están calibrados contra tráfico real, porque el producto no lo tiene (ADR 0002). El modo observación es el instrumento que produce esos datos antes de que el mecanismo pueda afectar a un solo usuario real.
2. **Una dependencia operativa que hay que decir en voz alta**: ADR 0018 ya fijó que, en producción sin `TURNSTILE_SECRET_KEY`, el captcha es fail-closed. Combinado con `exigir_captcha` activo, un despliegue sin esa llave convertiría "dispositivo nuevo + red nueva" en un bloqueo duro sin salida — el mismo tipo de trampa que este ADR existe para evitar, pero por la puerta de al lado.

## Alternativas consideradas

- **Producir `RequiereStepUp` para riesgo alto, tal como sugiere el nombre del campo**: descartada por el hallazgo de arriba — bloquearía permanentemente a cualquier usuario sin MFA que dispare la señal.
- **Producir `RequiereStepUp` solo si el usuario ya tiene MFA confirmado**: es una opción legítima para un hito futuro, pero exige verificar `tieneMFA` desde el propio evaluador de riesgo (una consulta que hoy no tiene y que acoplaría Confianza a un dato de Identidad) — queda nombrada como una de las tres formas de cerrar el hueco, no decidida acá.
- **`exigir_captcha` como default**: descartada por los dos motivos de calibración y dependencia operativa de arriba.

## Consecuencias

- El único desenlace observable de este mecanismo es un `429` con `Motivo: "riesgo_de_origen_requiere_captcha"`, nunca un `503`/step-up.
- Ningún usuario puede quedar bloqueado sin salida por este mecanismo: siempre puede resolver un captcha en el mismo intento.
- Habilitar `RequiereStepUp` desde el reconocimiento de origen en el futuro requiere primero un ADR que cierre el hueco de `verificar_otp.go`, y ese ADR debe referenciar este documento como la razón por la que la puerta estuvo cerrada hasta entonces.
- `exigir_captcha` en producción exige verificar `TURNSTILE_SECRET_KEY` como parte del checklist de habilitación, igual que ADR 0044 exige decidir `modoDegradado` al abrir una sala de espera.

## Estado

Aceptado.
