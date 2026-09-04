package bd

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// claveTx es la clave no exportada bajo la que EjecutarEnTransaccion guarda
// la transacción pgx activa dentro del context.Context (ADR candidato 0010
// del diseño de Identidad: la transacción viaja implícita en el ctx, para
// que los puertos de salida no se contaminen con tipos del driver pgx).
type claveTx struct{}

// EjecutarEnTransaccion abre una transacción pgx sobre pool, la publica en
// el context.Context que recibe fn, y hace commit/rollback según el
// resultado de fn. Es el helper técnico genérico que cada
// bounded-context envuelve en su propia implementación del puerto
// UnidadDeTrabajo (p. ej. identidad/adaptadores/postgres.UnidadDeTrabajo):
// este paquete no conoce puertos ni tipos de negocio de ningún contexto.
func EjecutarEnTransaccion(ctx context.Context, pool *pgxpool.Pool, fn func(ctx context.Context) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("bd: no se pudo iniciar la transacción: %w", err)
	}

	// Si el ctx trae un AlcanceTenencia publicado (por el middleware de
	// autorización de Tenencia, o por uno de sus propios repositorios antes
	// de abrir su propia transacción de una sola consulta), se fija AQUÍ,
	// como parte de abrir la transacción — antes de que cualquier query de
	// negocio se ejecute contra ella (ADR candidato 0031 del diseño de
	// Tenencia). Ningún otro contexto (Identidad, Acceso) publica jamás un
	// AlcanceTenencia, así que esto es un no-op invisible para ellos.
	if alcance, ok := alcanceTenenciaDesdeContexto(ctx); ok {
		if err := fijarAlcanceTenencia(ctx, tx, alcance); err != nil {
			if errRollback := tx.Rollback(ctx); errRollback != nil && errRollback != pgx.ErrTxClosed {
				return fmt.Errorf("bd: fallo al fijar el alcance de tenencia %w; además falló el rollback: %v", err, errRollback)
			}
			return err
		}
	}

	ctxConTx := context.WithValue(ctx, claveTx{}, tx)

	if err := fn(ctxConTx); err != nil {
		if errRollback := tx.Rollback(ctx); errRollback != nil && errRollback != pgx.ErrTxClosed {
			return fmt.Errorf("bd: fallo original %w; además falló el rollback: %v", err, errRollback)
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("bd: no se pudo confirmar la transacción: %w", err)
	}
	return nil
}

// TxDesdeContexto recupera la transacción pgx activa publicada por
// EjecutarEnTransaccion, si existe. Los adaptadores de repositorio la usan
// para participar en la transacción de negocio+auditoría (ADR 0005); si no
// hay transacción en el ctx (p. ej. una lectura fuera de UnidadDeTrabajo),
// el segundo valor devuelto es false y el llamador debe usar el pool
// directamente.
func TxDesdeContexto(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(claveTx{}).(pgx.Tx)
	return tx, ok
}
