package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/r-david1/moterus/internal/plataforma/bd"
	"github.com/r-david1/moterus/internal/tenencia/puertos"
)

// UnidadDeTrabajo implementa puertos.UnidadDeTrabajo envolviendo el helper
// técnico internal/plataforma/bd.EjecutarEnTransaccion — mismo patrón que
// identidad/adaptadores/postgres.UnidadDeTrabajo y
// acceso/adaptadores/postgres.UnidadDeTrabajo: abre una transacción pgx, la
// publica en el context.Context para que los tres repositorios y el ACL de
// auditoría la usen, y hace commit/rollback según el resultado de fn.
//
// La diferencia con los otros dos contextos es que, cuando el ctx que
// recibe Ejecutar ya trae un AlcanceTenencia publicado (por el middleware
// de autorización HTTP de Tenencia, vía AlcanceTenencia.ConAlcance de este
// mismo paquete), bd.EjecutarEnTransaccion emite el SET LOCAL
// correspondiente al abrir la transacción — ver
// internal/plataforma/bd/alcance_tenencia.go. Esta implementación NO
// necesita saber nada de eso: delega íntegramente en el helper genérico,
// que ya lo resuelve leyendo el ctx.
type UnidadDeTrabajo struct {
	pool *pgxpool.Pool
}

var _ puertos.UnidadDeTrabajo = (*UnidadDeTrabajo)(nil)

// NuevaUnidadDeTrabajo construye el adaptador sobre un pool pgx ya
// inicializado.
func NuevaUnidadDeTrabajo(pool *pgxpool.Pool) *UnidadDeTrabajo {
	return &UnidadDeTrabajo{pool: pool}
}

// Ejecutar delega en bd.EjecutarEnTransaccion.
func (u *UnidadDeTrabajo) Ejecutar(ctx context.Context, fn func(ctx context.Context) error) error {
	return bd.EjecutarEnTransaccion(ctx, u.pool, fn)
}
