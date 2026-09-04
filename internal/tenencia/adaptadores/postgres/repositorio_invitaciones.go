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

// RepositorioInvitaciones implementa puertos.RepositorioInvitaciones sobre
// pgx/sqlc.
type RepositorioInvitaciones struct {
	pool *pgxpool.Pool
}

var _ puertos.RepositorioInvitaciones = (*RepositorioInvitaciones)(nil)

// NuevoRepositorioInvitaciones construye el adaptador sobre un pool pgx ya
// inicializado.
func NuevoRepositorioInvitaciones(pool *pgxpool.Pool) *RepositorioInvitaciones {
	return &RepositorioInvitaciones{pool: pool}
}

// Guardar persiste el agregado completo: intenta primero el UPDATE
// (ActualizarInvitacion); si no afecta ninguna fila es porque la invitación
// todavía no existe, y se hace el INSERT (CrearInvitacion). El alcance de
// RLS se auto-acota a la organización del propio agregado.
func (r *RepositorioInvitaciones) Guardar(ctx context.Context, i *dominio.Invitacion) error {
	idPg, err := uuidAPg(i.ID().String())
	if err != nil {
		return err
	}
	organizacionIDPg, err := uuidAPg(i.OrganizacionID().String())
	if err != nil {
		return err
	}
	invitadaPorPg, err := uuidAPg(i.InvitadaPor().String())
	if err != nil {
		return err
	}
	return ejecutar(ctx, r.pool, "", i.OrganizacionID().String(), func(ctx context.Context, q *sqlc.Queries) error {
		resuelta := tiempoOpcionalAPg(resueltaEnAPuntero(i))
		_, err := q.ActualizarInvitacion(ctx, sqlc.ActualizarInvitacionParams{
			ID:         idPg,
			Estado:     i.Estado().String(),
			ResueltaEn: resuelta,
		})
		switch {
		case err == nil:
			return nil
		case errors.Is(err, pgx.ErrNoRows):
			_, err := q.CrearInvitacion(ctx, sqlc.CrearInvitacionParams{
				ID:                 idPg,
				OrganizacionID:     organizacionIDPg,
				CorreoDestinatario: i.Destinatario().Normalizado(),
				RolPropuesto:       i.RolPropuesto().Valor(),
				Estado:             i.Estado().String(),
				HashToken:          i.HashToken().Valor(),
				InvitadaPor:        invitadaPorPg,
				CreadaEn:           tiempoAPg(i.CreadaEn()),
				ExpiraEn:           tiempoAPg(i.ExpiraEn()),
				ResueltaEn:         resuelta,
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

func resueltaEnAPuntero(i *dominio.Invitacion) *time.Time {
	if t, ok := i.ResueltaEn(); ok {
		v := t
		return &v
	}
	return nil
}

// BuscarPorID no conoce la organización de antemano (el identificador
// administrativo no la revela); solo devuelve resultados dentro de una
// transacción que ya trae un alcance vigente (típicamente el publicado por
// el middleware de autorización HTTP, org-scoped, para la petición en
// curso — ver RevocarInvitacion, §3.5 del diseño).
func (r *RepositorioInvitaciones) BuscarPorID(ctx context.Context, id dominio.IDInvitacion) (*dominio.Invitacion, error) {
	idPg, err := uuidAPg(id.String())
	if err != nil {
		return nil, err
	}
	var resultado *dominio.Invitacion
	if err := ejecutar(ctx, r.pool, "", "", func(ctx context.Context, q *sqlc.Queries) error {
		fila, err := q.ObtenerInvitacionPorID(ctx, idPg)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return traducirError(err)
		}
		resultado, err = invitacionDesdeFila(fila)
		return err
	}); err != nil {
		return nil, err
	}
	return resultado, nil
}

// BuscarPorHash es la única consulta que atraviesa el aislamiento por
// organización (§2.2 del diseño): se implementa sobre la función SECURITY
// DEFINER tenencia_resolver_invitacion (000014), la ÚNICA vía de escape de
// RLS del contexto — nunca una consulta directa a la tabla. No hace falta
// abrir ninguna transacción con alcance propio: la función ya bypassa RLS
// internamente.
func (r *RepositorioInvitaciones) BuscarPorHash(ctx context.Context, h dominio.HashTokenInvitacion) (*dominio.Invitacion, error) {
	var resultado *dominio.Invitacion
	if err := ejecutar(ctx, r.pool, "", "", func(ctx context.Context, q *sqlc.Queries) error {
		fila, err := q.ObtenerInvitacionPorHash(ctx, h.Valor())
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return traducirError(err)
		}
		resultado, err = invitacionDesdeFilaHash(fila)
		return err
	}); err != nil {
		return nil, err
	}
	return resultado, nil
}

// BuscarPendiente resuelve la invitación pendiente (si existe) para el par
// (organización, correo) — INV-TEN-22.
func (r *RepositorioInvitaciones) BuscarPendiente(ctx context.Context, o dominio.IDOrganizacion, c dominio.CorreoDestinatario) (*dominio.Invitacion, error) {
	organizacionIDPg, err := uuidAPg(o.String())
	if err != nil {
		return nil, err
	}
	var resultado *dominio.Invitacion
	if err := ejecutar(ctx, r.pool, "", o.String(), func(ctx context.Context, q *sqlc.Queries) error {
		fila, err := q.ObtenerInvitacionPendiente(ctx, sqlc.ObtenerInvitacionPendienteParams{
			OrganizacionID:     organizacionIDPg,
			CorreoDestinatario: c.Normalizado(),
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return traducirError(err)
		}
		resultado, err = invitacionDesdeFila(fila)
		return err
	}); err != nil {
		return nil, err
	}
	return resultado, nil
}

// ListarPendientesDeOrganizacion devuelve las invitaciones pendientes de
// una organización, más recientes primero.
func (r *RepositorioInvitaciones) ListarPendientesDeOrganizacion(ctx context.Context, o dominio.IDOrganizacion) ([]*dominio.Invitacion, error) {
	organizacionIDPg, err := uuidAPg(o.String())
	if err != nil {
		return nil, err
	}
	var resultado []*dominio.Invitacion
	if err := ejecutar(ctx, r.pool, "", o.String(), func(ctx context.Context, q *sqlc.Queries) error {
		filas, err := q.ListarInvitacionesPendientesDeOrganizacion(ctx, organizacionIDPg)
		if err != nil {
			return traducirError(err)
		}
		resultado = make([]*dominio.Invitacion, 0, len(filas))
		for _, fila := range filas {
			inv, err := invitacionDesdeFila(fila)
			if err != nil {
				return err
			}
			resultado = append(resultado, inv)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return resultado, nil
}

// ContarPendientesDeOrganizacion cuenta las invitaciones pendientes de una
// organización (PoliticaOrganizacion.MaximoInvitacionesPendientes).
func (r *RepositorioInvitaciones) ContarPendientesDeOrganizacion(ctx context.Context, o dominio.IDOrganizacion) (int, error) {
	organizacionIDPg, err := uuidAPg(o.String())
	if err != nil {
		return 0, err
	}
	var total int64
	if err := ejecutar(ctx, r.pool, "", o.String(), func(ctx context.Context, q *sqlc.Queries) error {
		var err error
		total, err = q.ContarInvitacionesPendientesDeOrganizacion(ctx, organizacionIDPg)
		return traducirError(err)
	}); err != nil {
		return 0, err
	}
	return int(total), nil
}
