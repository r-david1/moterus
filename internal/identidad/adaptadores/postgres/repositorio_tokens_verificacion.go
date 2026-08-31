package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/r-david1/moterus/internal/identidad/adaptadores/postgres/sqlc"
	"github.com/r-david1/moterus/internal/identidad/dominio"
	"github.com/r-david1/moterus/internal/identidad/puertos"
	"github.com/r-david1/moterus/internal/plataforma/bd"
)

// RepositorioTokensVerificacion implementa
// puertos.RepositorioTokensVerificacion sobre pgx/sqlc, siguiendo el mismo
// patrón que RepositorioUsuarios: cuando el ctx recibido lleva una
// transacción publicada por UnidadDeTrabajo.Ejecutar, las consultas
// participan en ella; si no, usan el pool directamente.
type RepositorioTokensVerificacion struct {
	pool           *pgxpool.Pool
	queriesConPool *sqlc.Queries
}

var _ puertos.RepositorioTokensVerificacion = (*RepositorioTokensVerificacion)(nil)

// NuevoRepositorioTokensVerificacion construye el adaptador sobre un pool
// pgx ya inicializado.
func NuevoRepositorioTokensVerificacion(pool *pgxpool.Pool) *RepositorioTokensVerificacion {
	return &RepositorioTokensVerificacion{pool: pool, queriesConPool: sqlc.New(pool)}
}

// consultas devuelve el conjunto de queries sqlc ligado a la transacción
// activa en ctx (si UnidadDeTrabajo.Ejecutar la publicó) o, en su defecto,
// al pool compartido.
func (r *RepositorioTokensVerificacion) consultas(ctx context.Context) *sqlc.Queries {
	if tx, ok := bd.TxDesdeContexto(ctx); ok {
		return sqlc.New(tx)
	}
	return r.queriesConPool
}

// Guardar hace upsert real (INSERT ... ON CONFLICT (usuario_id) DO UPDATE,
// query UpsertTokenVerificacionCorreo) por usuarioID: un reenvío invalida el
// token anterior sin un paso de borrado previo (sección 3.4 del diseño).
func (r *RepositorioTokensVerificacion) Guardar(ctx context.Context, usuarioID dominio.IDUsuario, hashToken string, expiraEn time.Time) error {
	idPg, err := idAPg(usuarioID)
	if err != nil {
		return err
	}
	return r.consultas(ctx).UpsertTokenVerificacionCorreo(ctx, sqlc.UpsertTokenVerificacionCorreoParams{
		HashToken: hashToken,
		UsuarioID: idPg,
		ExpiraEn:  tiempoAPg(expiraEn),
	})
}

// BuscarPorHash devuelve encontrado=false (sin error) cuando no existe
// ningún token con ese hash — el token no encontrado es un resultado de
// negocio normal (token inventado o ya consumido), no una condición de
// error de infraestructura.
func (r *RepositorioTokensVerificacion) BuscarPorHash(ctx context.Context, hashToken string) (dominio.IDUsuario, time.Time, bool, error) {
	fila, err := r.consultas(ctx).ObtenerTokenVerificacionCorreoPorHash(ctx, hashToken)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return dominio.IDUsuario{}, time.Time{}, false, nil
		}
		return dominio.IDUsuario{}, time.Time{}, false, traducirError(err)
	}

	usuarioID, err := dominio.IDUsuarioDesde(fila.UsuarioID.String())
	if err != nil {
		return dominio.IDUsuario{}, time.Time{}, false, err
	}
	return usuarioID, fila.ExpiraEn.Time, true, nil
}

// Eliminar borra el token activo del usuario (si existe). No es un error
// que no exista ninguna fila que borrar (p. ej. un token que ya expiró y
// fue eliminado por otro camino): DELETE sin filas afectadas no falla.
func (r *RepositorioTokensVerificacion) Eliminar(ctx context.Context, usuarioID dominio.IDUsuario) error {
	idPg, err := idAPg(usuarioID)
	if err != nil {
		return err
	}
	if err := r.consultas(ctx).EliminarTokenVerificacionCorreoPorUsuario(ctx, idPg); err != nil {
		return traducirError(err)
	}
	return nil
}
