// Package dominio contiene los agregados, value objects, servicios de
// dominio, eventos y errores tipados del bounded context Tenencia:
// Organizacion (el tenant, ADR 0002), Membresia (Usuario × Organizacion ×
// Rol) e Invitacion, junto con el catálogo cerrado de roles y permisos, la
// regla de dominancia (INV-TEN-20) y el evaluador de autorización que
// consume el resto del sistema (VerificadorDeAutorizacion, §1.4 y §3.7 de
// docs/design/tenencia-bounded-context.md).
//
// Cero dependencias externas salvo la excepción documentada y explícita
// golang.org/x/text/unicode/norm para la normalización NFC de
// AliasOrganizacion, NombreOrganizacion y CorreoDestinatario (mismo
// criterio que identidad/dominio.Correo). En particular, este paquete no
// importa nada de internal/identidad ni de internal/acceso (INV-TEN-27,
// verificado en arquitectura_test.go): los tipos que en apariencia se
// solapan con esos contextos (IDUsuario, OrigenSolicitud, EventoDominio, la
// normalización de correo) están deliberadamente duplicados aquí — ver
// §1.7 del diseño.
package dominio
