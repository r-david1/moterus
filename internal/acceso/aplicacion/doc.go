// Package aplicacion contiene los casos de uso del bounded context Acceso:
// IniciarSesion, RenovarSesion, ValidarAcceso, CerrarSesion/CerrarTodas,
// ListarSesiones y RevocarSesionesDeUsuario (sección 3 de
// docs/design/acceso-bounded-context.md). Cada caso de uso es un struct
// con sus dependencias inyectadas por puerto (nunca implementaciones
// concretas de adaptadores/) e implementa la interfaz de entrada
// correspondiente de acceso/puertos. Importa dominio y puertos; nunca
// adaptadores ni ningún paquete de Identidad salvo el ACL declarado como
// puerto (INV-ACC-19).
package aplicacion
