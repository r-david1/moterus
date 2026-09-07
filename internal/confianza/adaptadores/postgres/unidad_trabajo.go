package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/r-david1/moterus/internal/confianza/puertos"
	"github.com/r-david1/moterus/internal/plataforma/bd"
)

// UnidadDeTrabajo implementa puertos.UnidadDeTrabajo envolviendo el helper
// técnico internal/plataforma/bd.EjecutarEnTransaccion — mismo patrón exacto
// que identidad/adaptadores/postgres.UnidadDeTrabajo y
// acceso/adaptadores/postgres.UnidadDeTrabajo: abre una transacción pgx, la
// publica en el context.Context para que RepositorioSalasDeEspera y el ACL
// de auditoría la usen, y hace commit/rollback según el resultado de fn.
// Sin RLS (§6.1 del diseño: `salas_espera` no es multi-tenant), a
// diferencia de tenencia/adaptadores/postgres.UnidadDeTrabajo: no hay
// SET LOCAL app.current_tenant que emitir.
//
// Esto es lo que permite cumplir INV-COLA-11: AbrirSalaDeEspera/
// CambiarRitmoDeAdmision/CambiarEstadoSala invocan UnidadDeTrabajo.Ejecutar
// con un fn que guarda el agregado Y audita en la misma transacción.
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
