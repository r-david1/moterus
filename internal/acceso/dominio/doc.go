// Package dominio contiene el agregado Sesion, sus value objects, servicios
// de dominio, eventos y errores del bounded context Acceso.
//
// Acceso responde una sola pregunta: este sujeto ya demostró quién es (eso
// lo decide Identidad) — ¿qué credencial de sesión le doy, por cuánto
// tiempo, y sigue siendo válida? El agregado raíz es Sesion (sesion.go):
// estado activa/revocada/expirada, la cadena de rotación de sus tokens de
// refresco (TokenRefrescoEmitido, entidad interna) y las ventanas de
// expiración por inactividad y absoluta. El token de acceso (JWT) no es un
// agregado: es una proyección firmada y de vida corta del estado de la
// sesión (ReclamacionesAcceso, reclamaciones.go), sin estado propio del
// lado del servidor.
//
// Value objects: IDSesion/IDTokenAcceso/IDUsuario (identificadores.go),
// EstadoSesion (estado_sesion.go), MotivoRevocacion (motivo_revocacion.go),
// TokenRefrescoPlano/HashTokenRefresco (tokens.go), OrigenSolicitud
// (origen_solicitud.go), PoliticaSesion (politica_sesion.go). Eventos de
// dominio en eventos.go, errores tipados en errores.go (más los errores de
// construcción propios de cada archivo de value object).
//
// Regla de arquitectura (INV-ACC-18): este paquete no importa nada fuera de
// la stdlib de Go (permitido: time, strings, fmt, net, unicode/utf8,
// crypto/sha256, crypto/subtle, encoding/hex, etc.). Cero dependencias
// externas, cero imports de internal/plataforma o de cualquier otro
// paquete del proyecto — en particular, nunca identidad/dominio
// (INV-ACC-18) ni ninguna biblioteca de JWT: el dominio decide qué dice el
// token, nunca cómo se codifica ni con qué llave se firma (eso vive detrás
// del puerto FirmadorTokensAcceso, en la capa de aplicación/adaptadores).
//
// IDUsuario, OrigenSolicitud y EventoDominio son tipos deliberadamente
// duplicados de sus homónimos en identidad/dominio, no compartidos: ver
// §1.7 del diseño del contexto (docs/design/acceso-bounded-context.md) y el
// ADR candidato 0027.
package dominio
