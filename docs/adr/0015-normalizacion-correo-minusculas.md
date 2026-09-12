# ADR 0015 — Normalización de correo a minúsculas completas

## Contexto

Un correo electrónico identifica de forma única a una cuenta en Identidad. RFC 5321 permite, técnicamente, que la parte local de un correo (todo lo que va antes de la `@`) sea sensible a mayúsculas — `Ana@dominio.com` y `ana@dominio.com` podrían, en teoría, ser dos buzones distintos. En la práctica, ningún proveedor de correo real (Gmail, Outlook, iCloud, la inmensa mayoría de los servidores SMTP corporativos) implementa esa distinción: todos tratan la parte local como insensible a mayúsculas.

`docs/design/identidad-bounded-context.md` §7 nombró esta decisión como candidata (0015); se implementó de facto (`internal/identidad/dominio/correo.go`) sin disputarse durante la implementación.

## Decisión

**`NuevoCorreo` normaliza el correo completo (parte local y dominio) a minúsculas**, además de aplicar `TrimSpace` y normalización Unicode NFC. `ana@dominio.com` y `Ana@Dominio.Com` producen el mismo `Correo` normalizado, y por lo tanto la misma cuenta.

Es una **desviación consciente del RFC**, no un descuido: seguir la letra del RFC (tratar `Ana@x.com` y `ana@x.com` como potencialmente distintos) abriría la puerta a cuentas duplicadas involuntarias — un usuario que se registra una vez con `Ana@x.com` y, meses después, intenta loguear escribiendo `ana@x.com` (el caso más común, dado que casi ningún cliente de correo distingue mayúsculas al mostrar o autocompletar direcciones) terminaría creyendo que no tiene cuenta, o peor, creando una segunda cuenta con el mismo buzón real.

## Alternativas consideradas

- **Seguir el RFC al pie de la letra** (case-sensitive en la parte local): descartado por el riesgo de duplicados y confusión de usuario descrito arriba, sin ningún beneficio real dado que ningún proveedor lo explota.
- **Normalizar solo el dominio, preservar mayúsculas en la parte local** (un punto intermedio, ya que el dominio SÍ es case-insensitive por especificación DNS): descartado por simplicidad — no resuelve el problema real (la confusión ocurre igual en la parte local, que es la que el usuario más frecuentemente escribe con mayúscula inicial) y agrega una regla más para explicar.

## Consecuencias

- La unicidad de cuentas por correo (INV-ID-02, el índice único de la tabla `usuarios`) se apoya en que dos formas del "mismo" correo humano siempre convergen al mismo valor normalizado antes de llegar a la base — sin esto, la restricción de unicidad de Postgres no cerraría el caso `Ana@x.com` vs `ana@x.com`.
- La normalización Unicode NFC (que también hace `NuevoCorreo`) resuelve un problema análogo para caracteres acentuados representados de dos formas distintas (`é` precompuesto vs. `e´` descompuesto) — mismo espíritu, misma función.
- Cualquier integración externa que envíe un correo a Identidad debe asumir que la forma de mayúsculas/minúsculas no se preserva tal cual — el correo que la cuenta ve en pantalla es siempre la forma normalizada, nunca la que el usuario tecleó originalmente.

## Estado

Aceptado (implementado de facto; documentado retroactivamente).
