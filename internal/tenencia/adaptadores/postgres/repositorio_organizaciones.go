package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/r-david1/moterus/internal/tenencia/adaptadores/postgres/sqlc"
	"github.com/r-david1/moterus/internal/tenencia/dominio"
	"github.com/r-david1/moterus/internal/tenencia/puertos"
)

// RepositorioOrganizaciones implementa puertos.RepositorioOrganizaciones
// sobre pgx/sqlc.
type RepositorioOrganizaciones struct {
	pool *pgxpool.Pool
}

var _ puertos.RepositorioOrganizaciones = (*RepositorioOrganizaciones)(nil)

// NuevoRepositorioOrganizaciones construye el adaptador sobre un pool pgx
// ya inicializado.
func NuevoRepositorioOrganizaciones(pool *pgxpool.Pool) *RepositorioOrganizaciones {
	return &RepositorioOrganizaciones{pool: pool}
}

// Guardar persiste el agregado completo: intenta primero el UPDATE
// (ActualizarOrganizacion); si no afecta ninguna fila (pgx.ErrNoRows con
// :one + RETURNING) es porque la organización todavía no existe, y se hace
// el INSERT (CrearOrganizacion) — mismo patrón que
// acceso/adaptadores/postgres.RepositorioSesiones.Guardar. El alcance de
// RLS se auto-acota a (creadaPor, ID) del propio agregado: cubre tanto el
// alta (organizaciones_aislamiento.WITH CHECK admite `creada_por =
// usuario_actual`) como la actualización (`id = organizacion_actual`).
func (r *RepositorioOrganizaciones) Guardar(ctx context.Context, o *dominio.Organizacion) error {
	idPg, err := uuidAPg(o.ID().String())
	if err != nil {
		return err
	}
	creadaPorPg, err := uuidAPg(o.CreadaPor().String())
	if err != nil {
		return err
	}
	var motivoTexto string
	if m, ok := o.MotivoEstado(); ok {
		motivoTexto = m.Valor()
	}

	return ejecutar(ctx, r.pool, o.CreadaPor().String(), o.ID().String(), func(ctx context.Context, q *sqlc.Queries) error {
		_, err := q.ActualizarOrganizacion(ctx, sqlc.ActualizarOrganizacionParams{
			ID:            idPg,
			Alias:         o.Alias().Normalizado(),
			Nombre:        o.Nombre().Valor(),
			Estado:        o.Estado().String(),
			ActualizadaEn: tiempoAPg(o.ActualizadaEn()),
			ArchivadaEn:   tiempoArchivadaAPg(o),
			MotivoEstado:  textoOpcionalAPg(motivoTexto),
		})
		switch {
		case err == nil:
			return nil
		case errors.Is(err, pgx.ErrNoRows):
			_, err := q.CrearOrganizacion(ctx, sqlc.CrearOrganizacionParams{
				ID:           idPg,
				Alias:        o.Alias().Normalizado(),
				Nombre:       o.Nombre().Valor(),
				Estado:       o.Estado().String(),
				CreadaPor:    creadaPorPg,
				CreadaEn:     tiempoAPg(o.CreadaEn()),
				ArchivadaEn:  tiempoArchivadaAPg(o),
				MotivoEstado: textoOpcionalAPg(motivoTexto),
			})
			if err != nil {
				return traducirError(err)
			}
			return nil
		default:
			return traducirError(err)
		}
	})
}

// BuscarPorID devuelve (nil, nil) cuando no existe la organización (o
// cuando RLS la oculta: el llamador no puede distinguir un caso del otro,
// y no debe hacerlo — INV-TEN-17).
func (r *RepositorioOrganizaciones) BuscarPorID(ctx context.Context, id dominio.IDOrganizacion) (*dominio.Organizacion, error) {
	idPg, err := uuidAPg(id.String())
	if err != nil {
		return nil, err
	}
	var resultado *dominio.Organizacion
	if err := ejecutar(ctx, r.pool, "", id.String(), func(ctx context.Context, q *sqlc.Queries) error {
		fila, err := q.ObtenerOrganizacionPorID(ctx, idPg)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return traducirError(err)
		}
		resultado, err = organizacionDesdeFila(fila)
		return err
	}); err != nil {
		return nil, err
	}
	return resultado, nil
}

// BuscarPorAlias devuelve (nil, nil) cuando no existe una organización con
// ese alias. Nota: no se invoca hoy desde ningún caso de uso (INV-TEN-02 se
// garantiza por el índice único, no por una consulta previa de la
// aplicación); se implementa por completitud del puerto. Sin un
// IDOrganizacion/IDUsuario propio con el que acotar el alcance de RLS de
// antemano, solo devuelve resultados dentro de una transacción que ya trae
// un alcance vigente (p. ej. publicado por el middleware HTTP para la
// petición en curso).
func (r *RepositorioOrganizaciones) BuscarPorAlias(ctx context.Context, a dominio.AliasOrganizacion) (*dominio.Organizacion, error) {
	var resultado *dominio.Organizacion
	if err := ejecutar(ctx, r.pool, "", "", func(ctx context.Context, q *sqlc.Queries) error {
		fila, err := q.ObtenerOrganizacionPorAlias(ctx, a.Normalizado())
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return traducirError(err)
		}
		resultado, err = organizacionDesdeFila(fila)
		return err
	}); err != nil {
		return nil, err
	}
	return resultado, nil
}

// CargarParaActualizar toma el candado de la fila raíz (SELECT ... FOR
// UPDATE, §1.2/INV-TEN-06 del diseño). Solo tiene sentido dentro de una
// UnidadDeTrabujo ya abierta: todos los casos de uso que la invocan lo
// hacen desde dentro de uow.Ejecutar, así que ejecutar() reutiliza esa
// transacción ambiental (reforzando el alcance a esta organización) en vez
// de abrir una nueva.
func (r *RepositorioOrganizaciones) CargarParaActualizar(ctx context.Context, id dominio.IDOrganizacion) (*dominio.Organizacion, error) {
	idPg, err := uuidAPg(id.String())
	if err != nil {
		return nil, err
	}
	var resultado *dominio.Organizacion
	if err := ejecutar(ctx, r.pool, "", id.String(), func(ctx context.Context, q *sqlc.Queries) error {
		fila, err := q.ObtenerOrganizacionParaActualizar(ctx, idPg)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return traducirError(err)
		}
		resultado, err = organizacionDesdeFila(fila)
		return err
	}); err != nil {
		return nil, err
	}
	return resultado, nil
}
