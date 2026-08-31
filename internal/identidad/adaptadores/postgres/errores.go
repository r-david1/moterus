package postgres

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/r-david1/moterus/internal/identidad/dominio"
)

// codigoViolacionUnicidad es el SQLSTATE de Postgres para "unique_violation".
const codigoViolacionUnicidad = "23505"

// indiceCorreoUnico es el nombre del índice único que garantiza INV-ID-02 a
// nivel de base de datos (000001_crear_usuarios.up.sql).
const indiceCorreoUnico = "usuarios_correo_idx"

// traducirError convierte un error del driver pgx en un error de dominio
// tipado cuando corresponde (p. ej. la violación del índice único de correo
// se traduce a dominio.ErrCorreoYaRegistrado, tal como exige la tabla 1.5
// del diseño: "lo produce el adaptador Postgres al traducir la violación de
// unicidad"). Cualquier otro error se envuelve sin tipar, para que el
// llamador no lo confunda con un error de negocio.
func traducirError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == codigoViolacionUnicidad {
		if pgErr.ConstraintName == indiceCorreoUnico {
			return &dominio.ErrCorreoYaRegistrado{}
		}
	}
	return fmt.Errorf("postgres: %w", err)
}
