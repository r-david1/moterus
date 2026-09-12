# ADR 0016 — Los comandos de la capa de aplicación transportan primitivos, no value objects

## Contexto

Cada caso de uso de la capa de aplicación (`ComandoRegistrarUsuario`, `ComandoAutenticar`, `ComandoVerificarCorreo`, y su equivalente en cada contexto posterior) necesita una forma de entrada. Había que decidir si esa forma usa los value objects del dominio (`dominio.Correo`, `dominio.ContrasenaPlana`) directamente en su firma, o tipos primitivos de Go (`string`, `int`) que el propio caso de uso valida y convierte.

`docs/design/identidad-bounded-context.md` §7 nombró esta decisión como candidata (0016); se implementó de facto en todos los contextos desde el primero (Identidad) sin disputarse durante la implementación.

## Decisión

**Todo `ComandoX`/`ConsultaX` de `puertos/entrada.go`, en cualquier contexto, transporta únicamente tipos primitivos.** La construcción del value object correspondiente (`dominio.NuevoCorreo(cmd.Correo)`, `dominio.NuevaContrasenaPlana(cmd.Contrasena)`, etc.) ocurre **dentro** del caso de uso, como su primer paso, nunca antes de cruzar el puerto de entrada.

Esto garantiza que **la validación de negocio ocurre siempre dentro del hexágono**, sin importar qué adaptador de entrada la invoque. Un handler HTTP, un futuro comando de CLI, o un consumidor de gRPC pueden construir el mismo `ComandoRegistrarUsuario{Correo: "...", Contrasena: "..."}` con los datos crudos que recibieron de su medio de transporte respectivo, sin necesitar saber nada sobre cómo se valida un correo — esa responsabilidad vive en un solo lugar (el dominio), y cualquier input inválido produce el mismo error de dominio tipado (`ErrCorreoInvalido`, etc.) sin importar el adaptador que lo originó.

## Alternativas consideradas

- **El adaptador de entrada (HTTP) construye los value objects antes de invocar el caso de uso**, pasando `dominio.Correo` ya validado dentro del comando: descartado — duplicaría la validación en cada adaptador de entrada presente y futuro, o peor, dejaría la validación real solo en el primer adaptador que se escribió, con los demás confiando en datos no verificados.
- **Los comandos usan `interface{}`/`any` para máxima flexibilidad**: descartado de plano — pierde el chequeo de tipos de Go en tiempo de compilación sin ninguna ganancia real.

## Consecuencias

- Cada caso de uso empieza con un bloque de construcción/validación de VOs a partir de los primitivos del comando, antes de tocar ningún puerto de salida — patrón visible y consistente en todos los `aplicacion/*.go` de los cinco contextos del sistema.
- Los mocks y tests de aplicación pueden construir comandos con strings arbitrarios sin necesitar importar ni entender los constructores de dominio, lo que simplifica la escritura de casos de prueba con entradas inválidas (el propio test verifica que el error de dominio correcto se propaga).
- Este mismo criterio es el que exigió, en la extensión de colas de acceso virtual y reconocimiento de origen (Confianza), que los puertos de entrada (`ConsultaSalaVigente`, `ComandoIngresarASala`, `Solicitud`, `ResultadoIntento`) transporten primitivos y nunca `dominio.OrigenSolicitud` u otro VO — la regla se aplicó de forma consistente en todo el proyecto, no solo en Identidad.

## Estado

Aceptado (implementado de facto; documentado retroactivamente).
