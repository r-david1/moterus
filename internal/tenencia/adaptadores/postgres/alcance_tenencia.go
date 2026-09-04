package postgres

import (
	"context"

	"github.com/r-david1/moterus/internal/plataforma/bd"
	"github.com/r-david1/moterus/internal/tenencia/puertos"
)

// AlcanceTenencia implementa puertos.AlcanceDeTenencia: publica en el ctx
// el par (usuario_actual, organizacion_actual) que la UnidadDeTrabajo de
// este mismo paquete lee al abrir cada transacción para emitir
// `SET LOCAL app.usuario_actual` / `SET LOCAL app.organizacion_actual`
// (§6.3 del diseño, ADR candidato 0031). Implementación trivial a
// propósito: toda la lógica de emisión del SET LOCAL vive en
// internal/plataforma/bd (compartida entre "publicar para toda la
// petición" desde el middleware HTTP y "reforzar para una consulta
// puntual" desde cada repositorio de este paquete).
type AlcanceTenencia struct{}

var _ puertos.AlcanceDeTenencia = AlcanceTenencia{}

// NuevaAlcanceTenencia construye el adaptador. Sin estado: no necesita el
// pool ni ninguna otra dependencia.
func NuevaAlcanceTenencia() AlcanceTenencia { return AlcanceTenencia{} }

// ConAlcance publica el alcance en el context.Context.
func (AlcanceTenencia) ConAlcance(ctx context.Context, idUsuario, idOrganizacion string) context.Context {
	return bd.ConAlcanceTenencia(ctx, idUsuario, idOrganizacion)
}
