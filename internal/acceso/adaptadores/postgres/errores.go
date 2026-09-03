package postgres

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/r-david1/moterus/internal/acceso/dominio"
)

// codigoViolacionUnicidad es el SQLSTATE de Postgres para "unique_violation".
const codigoViolacionUnicidad = "23505"

// indiceRefrescoVigentePorSesion es el nombre del índice único parcial que
// garantiza INV-ACC-04 a nivel de base de datos: a lo sumo un token de
// refresco vigente (no consumido) por sesión (migración
// 000006_crear_sesiones.up.sql).
const indiceRefrescoVigentePorSesion = "tokens_refresco_vigente_por_sesion_idx"

// traducirError convierte un error del driver pgx en un error de dominio
// tipado cuando corresponde: la violación del índice único parcial de
// refresco vigente se traduce a dominio.ErrConcurrenciaSesion (tabla 1.5
// del diseño: "lo produce el adaptador Postgres al traducir la violación
// del índice único parcial de refresco vigente"). Cualquier otro error se
// envuelve sin tipar, para que el llamador no lo confunda con un error de
// negocio.
func traducirError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == codigoViolacionUnicidad {
		if pgErr.ConstraintName == indiceRefrescoVigentePorSesion {
			return &dominio.ErrConcurrenciaSesion{}
		}
	}
	return fmt.Errorf("postgres: %w", err)
}
