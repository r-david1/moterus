// Package http contiene los adaptadores HTTP (Huma v2 sobre Fiber) del
// bounded context Confianza: los endpoints públicos y de administración de
// colas de acceso virtual (§7 del diseño docs/design/colas-virtuales.md) y
// el middleware MiddlewareSalaDeEspera que otros contextos (Acceso,
// Identidad, Tenencia) montan sobre sus propias rutas protegidas (§7.3 del
// diseño; ese montaje es una tarea posterior, §12).
package http
