// Package bd contiene el pool de conexiones pgx compartido y los
// helpers de transacción (incluida la futura UnidadDeTrabajo). Kernel
// técnico: no conoce reglas de negocio de ningún bounded context.
package bd

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NuevoPool crea un pool de conexiones pgx a partir de un DSN. No
// ejecuta ninguna consulta: la resolución de tenant (SET LOCAL
// app.current_tenant) es responsabilidad de cada adaptador de
// repositorio, dentro de su propia transacción.
func NuevoPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	if dsn == "" {
		return nil, fmt.Errorf("bd: DSN vacio")
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("bd: no se pudo crear el pool: %w", err)
	}
	return pool, nil
}
