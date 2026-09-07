// Package http contiene los adaptadores HTTP (Huma v2 sobre Fiber) del
// bounded context Confianza: los endpoints públicos y de administración de
// colas de acceso virtual (§7 del diseño docs/design/colas-virtuales.md) y
// el middleware MiddlewareSalaDeEspera que Acceso, Identidad y Tenencia
// montan sobre sus propias rutas protegidas (§7.3 del diseño). El montaje
// real (§12) está hecho: cmd/api/main.go lo inyecta en las tres, junto con
// el cableado del resto del motor (Redis, Postgres, reconciliador).
package http
