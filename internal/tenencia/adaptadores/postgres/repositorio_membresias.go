package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/r-david1/moterus/internal/tenencia/adaptadores/postgres/sqlc"
	"github.com/r-david1/moterus/internal/tenencia/dominio"
	"github.com/r-david1/moterus/internal/tenencia/puertos"
)

// RepositorioMembresias implementa puertos.RepositorioMembresias sobre
// pgx/sqlc.
type RepositorioMembresias struct {
	pool *pgxpool.Pool
}

var _ puertos.RepositorioMembresias = (*RepositorioMembresias)(nil)

// NuevoRepositorioMembresias construye el adaptador sobre un pool pgx ya
// inicializado.
func NuevoRepositorioMembresias(pool *pgxpool.Pool) *RepositorioMembresias {
	return &RepositorioMembresias{pool: pool}
}

// Guardar persiste el agregado completo: intenta primero el UPDATE
// (ActualizarMembresia); si no afecta ninguna fila es porque la membresía
// todavía no existe, y se hace el INSERT (CrearMembresia). El alcance de
// RLS se auto-acota a (usuarioID, organizacionID) del propio agregado.
func (r *RepositorioMembresias) Guardar(ctx context.Context, m *dominio.Membresia) error {
	idPg, err := uuidAPg(m.ID().String())
	if err != nil {
		return err
	}
	organizacionIDPg, err := uuidAPg(m.OrganizacionID().String())
	if err != nil {
		return err
	}
	usuarioIDPg, err := uuidAPg(m.UsuarioID().String())
	if err != nil {
		return err
	}
	var otorgadaPorTexto string
	if otorgadaPor, ok := m.OtorgadaPor(); ok {
		otorgadaPorTexto = otorgadaPor.String()
	}

	return ejecutar(ctx, r.pool, m.UsuarioID().String(), m.OrganizacionID().String(), func(ctx context.Context, q *sqlc.Queries) error {
		_, err := q.ActualizarMembresia(ctx, sqlc.ActualizarMembresiaParams{
			ID:            idPg,
			Rol:           m.Rol().Valor(),
			Estado:        m.Estado().String(),
			ActualizadaEn: tiempoAPg(m.ActualizadaEn()),
			RemovidaEn:    tiempoOpcionalAPg(removidaEnAPuntero(m)),
		})
		switch {
		case err == nil:
			return nil
		case errors.Is(err, pgx.ErrNoRows):
			_, err := q.CrearMembresia(ctx, sqlc.CrearMembresiaParams{
				ID:             idPg,
				OrganizacionID: organizacionIDPg,
				UsuarioID:      usuarioIDPg,
				Rol:            m.Rol().Valor(),
				Estado:         m.Estado().String(),
				OtorgadaPor:    uuidOpcionalAPg(otorgadaPorTexto),
				CreadaEn:       tiempoAPg(m.CreadaEn()),
				RemovidaEn:     tiempoOpcionalAPg(removidaEnAPuntero(m)),
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

// removidaEnAPuntero extrae RemovidaEn() de un agregado Membresia como
// *time.Time, nil si todavía no está removida.
func removidaEnAPuntero(m *dominio.Membresia) *time.Time {
	if t, ok := m.RemovidaEn(); ok {
		v := t
		return &v
	}
	return nil
}

// BuscarPorID no se invoca hoy desde ningún caso de uso. Sin un
// IDOrganizacion propio con el que acotar el alcance de RLS de antemano
// (una IDMembresia por sí sola no revela su organización), solo devuelve
// resultados dentro de una transacción que ya trae un alcance vigente.
func (r *RepositorioMembresias) BuscarPorID(ctx context.Context, id dominio.IDMembresia) (*dominio.Membresia, error) {
	idPg, err := uuidAPg(id.String())
	if err != nil {
		return nil, err
	}
	var resultado *dominio.Membresia
	if err := ejecutar(ctx, r.pool, "", "", func(ctx context.Context, q *sqlc.Queries) error {
		fila, err := q.ObtenerMembresiaPorID(ctx, idPg)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return traducirError(err)
		}
		resultado, err = membresiaDesdeFila(fila)
		return err
	}); err != nil {
		return nil, err
	}
	return resultado, nil
}

// BuscarVigente resuelve la membresía NO removida del par (usuario,
// organización): el camino caliente de autorización (§3.7 del diseño).
func (r *RepositorioMembresias) BuscarVigente(ctx context.Context, u dominio.IDUsuario, o dominio.IDOrganizacion) (*dominio.Membresia, error) {
	organizacionIDPg, err := uuidAPg(o.String())
	if err != nil {
		return nil, err
	}
	usuarioIDPg, err := uuidAPg(u.String())
	if err != nil {
		return nil, err
	}
	var resultado *dominio.Membresia
	if err := ejecutar(ctx, r.pool, u.String(), o.String(), func(ctx context.Context, q *sqlc.Queries) error {
		fila, err := q.ObtenerMembresiaVigente(ctx, sqlc.ObtenerMembresiaVigenteParams{
			OrganizacionID: organizacionIDPg,
			UsuarioID:      usuarioIDPg,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return traducirError(err)
		}
		resultado, err = membresiaDesdeFila(fila)
		return err
	}); err != nil {
		return nil, err
	}
	return resultado, nil
}

// ListarDeOrganizacion devuelve todas las membresías (cualquier estado) de
// una organización.
func (r *RepositorioMembresias) ListarDeOrganizacion(ctx context.Context, o dominio.IDOrganizacion) ([]*dominio.Membresia, error) {
	organizacionIDPg, err := uuidAPg(o.String())
	if err != nil {
		return nil, err
	}
	var resultado []*dominio.Membresia
	if err := ejecutar(ctx, r.pool, "", o.String(), func(ctx context.Context, q *sqlc.Queries) error {
		filas, err := q.ListarMembresiasDeOrganizacion(ctx, organizacionIDPg)
		if err != nil {
			return traducirError(err)
		}
		resultado = make([]*dominio.Membresia, 0, len(filas))
		for _, fila := range filas {
			m, err := membresiaDesdeFila(fila)
			if err != nil {
				return err
			}
			resultado = append(resultado, m)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return resultado, nil
}

// ListarDeUsuario devuelve las membresías NO removidas de un usuario, en
// cualquier organización (base de ListarMisOrganizaciones, §3.6 del
// diseño).
func (r *RepositorioMembresias) ListarDeUsuario(ctx context.Context, u dominio.IDUsuario) ([]*dominio.Membresia, error) {
	usuarioIDPg, err := uuidAPg(u.String())
	if err != nil {
		return nil, err
	}
	var resultado []*dominio.Membresia
	if err := ejecutar(ctx, r.pool, u.String(), "", func(ctx context.Context, q *sqlc.Queries) error {
		filas, err := q.ListarMembresiasDeUsuario(ctx, usuarioIDPg)
		if err != nil {
			return traducirError(err)
		}
		resultado = make([]*dominio.Membresia, 0, len(filas))
		for _, fila := range filas {
			m, err := membresiaDesdeFila(fila)
			if err != nil {
				return err
			}
			resultado = append(resultado, m)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return resultado, nil
}

// ContarPropietariosActivos cuenta las membresías activas con rol
// propietario de una organización (INV-TEN-06).
func (r *RepositorioMembresias) ContarPropietariosActivos(ctx context.Context, o dominio.IDOrganizacion) (int, error) {
	organizacionIDPg, err := uuidAPg(o.String())
	if err != nil {
		return 0, err
	}
	var total int64
	if err := ejecutar(ctx, r.pool, "", o.String(), func(ctx context.Context, q *sqlc.Queries) error {
		var err error
		total, err = q.ContarPropietariosActivos(ctx, organizacionIDPg)
		return traducirError(err)
	}); err != nil {
		return 0, err
	}
	return int(total), nil
}

// ContarActivasDeOrganizacion cuenta las membresías activas (cualquier rol)
// de una organización.
func (r *RepositorioMembresias) ContarActivasDeOrganizacion(ctx context.Context, o dominio.IDOrganizacion) (int, error) {
	organizacionIDPg, err := uuidAPg(o.String())
	if err != nil {
		return 0, err
	}
	var total int64
	if err := ejecutar(ctx, r.pool, "", o.String(), func(ctx context.Context, q *sqlc.Queries) error {
		var err error
		total, err = q.ContarActivasDeOrganizacion(ctx, organizacionIDPg)
		return traducirError(err)
	}); err != nil {
		return 0, err
	}
	return int(total), nil
}

// ContarOrganizacionesPropiasDeUsuario cuenta las membresías activas con
// rol propietario de un usuario, en cualquier organización
// (PoliticaOrganizacion.MaximoOrganizacionesPorUsuario).
func (r *RepositorioMembresias) ContarOrganizacionesPropiasDeUsuario(ctx context.Context, u dominio.IDUsuario) (int, error) {
	usuarioIDPg, err := uuidAPg(u.String())
	if err != nil {
		return 0, err
	}
	var total int64
	if err := ejecutar(ctx, r.pool, u.String(), "", func(ctx context.Context, q *sqlc.Queries) error {
		var err error
		total, err = q.ContarOrganizacionesPropiasDeUsuario(ctx, usuarioIDPg)
		return traducirError(err)
	}); err != nil {
		return 0, err
	}
	return int(total), nil
}
