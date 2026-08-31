package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/r-david1/moterus/internal/identidad/puertos"
	"github.com/r-david1/moterus/internal/plataforma/bd"
)

// UnidadDeTrabajo implementa puertos.UnidadDeTrabajo envolviendo el helper
// técnico internal/plataforma/bd.EjecutarEnTransaccion: abre una transacción
// pgx, la publica en el context.Context (ADR candidato 0010) para que
// RepositorioUsuarios y el ACL de auditoría la usen sin que el puerto exponga
// tipos de pgx, y hace commit/rollback según el resultado de fn.
//
// Esto es lo que permite cumplir ADR 0005: RegistrarUsuario/AutenticarUsuario
// invocan UnidadDeTrabajo.Ejecutar con un fn que guarda el agregado Y audita
// en la misma transacción.
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
