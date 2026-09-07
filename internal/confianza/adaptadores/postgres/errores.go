package postgres

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/r-david1/moterus/internal/confianza/dominio"
)

// codigoViolacionUnicidad es el SQLSTATE de Postgres para "unique_violation".
// Mismo criterio que identidad/adaptadores/postgres/errores.go y
// tenencia/adaptadores/postgres/errores.go.
const codigoViolacionUnicidad = "23505"

// Nombres de los índices únicos de la migración 000017_crear_salas_espera
// que este adaptador traduce a errores de dominio tipados.
const (
	indiceAliasSala   = "salas_espera_alias_idx"
	indiceSalaVigente = "salas_espera_vigente_idx"
)

// traducirError convierte un error del driver pgx en un error de dominio
// tipado cuando corresponde:
//   - violación del índice único de alias -> ErrAliasSalaYaRegistrado (409)
//   - violación del índice único parcial de sala vigente por (alcance,
//     ruta) -> ErrSalaYaAbiertaParaLaRuta (409, INV-COLA-01: la garantía es
//     estructural, no una consulta previa del caso de uso)
//
// Cualquier otro error se envuelve sin tipar, para que el llamador no lo
// confunda con un error de negocio.
func traducirError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == codigoViolacionUnicidad {
		switch pgErr.ConstraintName {
		case indiceAliasSala:
			return &dominio.ErrAliasSalaYaRegistrado{}
		case indiceSalaVigente:
			return &dominio.ErrSalaYaAbiertaParaLaRuta{}
		}
	}
	return fmt.Errorf("confianza/postgres: %w", err)
}
