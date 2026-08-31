package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/r-david1/moterus/internal/identidad/adaptadores/postgres/sqlc"
	"github.com/r-david1/moterus/internal/identidad/dominio"
	"github.com/r-david1/moterus/internal/identidad/puertos"
	"github.com/r-david1/moterus/internal/plataforma/bd"
)

// RepositorioUsuarios implementa puertos.RepositorioUsuarios sobre pgx/sqlc.
// Cuando el ctx recibido lleva una transacción publicada por
// UnidadDeTrabajo.Ejecutar (ADR candidato 0010), las consultas participan en
// ella; si no, usan el pool directamente.
type RepositorioUsuarios struct {
	pool           *pgxpool.Pool
	queriesConPool *sqlc.Queries
}

var _ puertos.RepositorioUsuarios = (*RepositorioUsuarios)(nil)

// NuevoRepositorioUsuarios construye el adaptador sobre un pool pgx ya
// inicializado (internal/plataforma/bd.NuevoPool).
func NuevoRepositorioUsuarios(pool *pgxpool.Pool) *RepositorioUsuarios {
	return &RepositorioUsuarios{pool: pool, queriesConPool: sqlc.New(pool)}
}

// consultas devuelve el conjunto de queries sqlc ligado a la transacción
// activa en ctx (si UnidadDeTrabajo.Ejecutar la publicó) o, en su defecto,
// al pool compartido.
func (r *RepositorioUsuarios) consultas(ctx context.Context) *sqlc.Queries {
	if tx, ok := bd.TxDesdeContexto(ctx); ok {
		return sqlc.New(tx)
	}
	return r.queriesConPool
}

// Guardar persiste el estado completo del agregado Usuario. No hay un flag
// explícito "es nuevo" en el dominio (INV-ID-09: sin campos exportados), así
// que el adaptador intenta primero UPDATE (ActualizarUsuario); si no
// actualiza ninguna fila (pgx.ErrNoRows en un :one con RETURNING) es porque
// el usuario todavía no existe, y entonces hace el INSERT (CrearUsuario). Si
// el agregado tiene ultimo_acceso_en marcado, se sincroniza aparte con
// RegistrarAccesoUsuario (esa columna no la toca ActualizarUsuario/
// CrearUsuario, ver comentario del query sqlc).
func (r *RepositorioUsuarios) Guardar(ctx context.Context, u *dominio.Usuario) error {
	q := r.consultas(ctx)

	idPg, err := idAPg(u.ID())
	if err != nil {
		return err
	}

	_, err = q.ActualizarUsuario(ctx, sqlc.ActualizarUsuarioParams{
		ID:             idPg,
		ContrasenaHash: u.Credencial().Hash().Valor(),
		Estado:         u.Estado().String(),
		TieneMfa:       u.TieneMFA(),
		ActualizadoEn:  tiempoAPg(u.ActualizadoEn()),
	})
	switch {
	case err == nil:
		// Fila existente actualizada.
	case errors.Is(err, pgx.ErrNoRows):
		// No existía: se trata de un alta nueva.
		if _, err := q.CrearUsuario(ctx, sqlc.CrearUsuarioParams{
			ID:             idPg,
			Correo:         u.Correo().Normalizado(),
			ContrasenaHash: u.Credencial().Hash().Valor(),
			Estado:         u.Estado().String(),
			TieneMfa:       u.TieneMFA(),
			CreadoEn:       tiempoAPg(u.CreadoEn()),
		}); err != nil {
			return traducirError(err)
		}
	default:
		return traducirError(err)
	}

	if ultimoAcceso, ok := u.UltimoAccesoEn(); ok {
		if _, err := q.RegistrarAccesoUsuario(ctx, sqlc.RegistrarAccesoUsuarioParams{
			ID:             idPg,
			UltimoAccesoEn: tiempoAPg(ultimoAcceso),
		}); err != nil {
			return traducirError(err)
		}
	}

	return nil
}

// BuscarPorID devuelve (nil, nil) cuando no existe el usuario: el contrato
// que espera identidad/aplicacion (ver ObtenerUsuarioCasoDeUso, que
// construye su propio dominio.ErrUsuarioNoEncontrado a partir de un
// resultado nil sin error).
func (r *RepositorioUsuarios) BuscarPorID(ctx context.Context, id dominio.IDUsuario) (*dominio.Usuario, error) {
	idPg, err := idAPg(id)
	if err != nil {
		return nil, err
	}
	fila, err := r.consultas(ctx).ObtenerUsuarioPorID(ctx, idPg)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, traducirError(err)
	}
	return usuarioDesdeFila(fila)
}

// BuscarPorCorreo devuelve (nil, nil) cuando no existe el usuario, por el
// mismo motivo que BuscarPorID: el caso de uso AutenticarUsuario también
// tolera un dominio.ErrUsuarioNoEncontrado devuelto por el repositorio (vía
// errors.As), pero el camino normativo es (nil, nil) sin error.
func (r *RepositorioUsuarios) BuscarPorCorreo(ctx context.Context, c dominio.Correo) (*dominio.Usuario, error) {
	fila, err := r.consultas(ctx).ObtenerUsuarioPorCorreo(ctx, c.Normalizado())
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, traducirError(err)
	}
	return usuarioDesdeFila(fila)
}
