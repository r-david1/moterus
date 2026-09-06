package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/r-david1/moterus/internal/identidad/adaptadores/postgres/sqlc"
	"github.com/r-david1/moterus/internal/identidad/dominio"
	"github.com/r-david1/moterus/internal/identidad/puertos"
	"github.com/r-david1/moterus/internal/plataforma/bd"
)

// RepositorioFactoresMFA implementa puertos.RepositorioFactoresMFA sobre
// pgx/sqlc (docs/design/otp-mfa.md §2.2, migración 000015), siguiendo el
// mismo patrón que RepositorioUsuarios: cuando el ctx recibido lleva una
// transacción publicada por UnidadDeTrabajo.Ejecutar, las consultas
// participan en ella; si no, usan el pool directamente.
type RepositorioFactoresMFA struct {
	pool           *pgxpool.Pool
	queriesConPool *sqlc.Queries
}

var _ puertos.RepositorioFactoresMFA = (*RepositorioFactoresMFA)(nil)

// NuevoRepositorioFactoresMFA construye el adaptador sobre un pool pgx ya
// inicializado.
func NuevoRepositorioFactoresMFA(pool *pgxpool.Pool) *RepositorioFactoresMFA {
	return &RepositorioFactoresMFA{pool: pool, queriesConPool: sqlc.New(pool)}
}

// consultas devuelve el conjunto de queries sqlc ligado a la transacción
// activa en ctx (si UnidadDeTrabajo.Ejecutar la publicó) o, en su defecto,
// al pool compartido.
func (r *RepositorioFactoresMFA) consultas(ctx context.Context) *sqlc.Queries {
	if tx, ok := bd.TxDesdeContexto(ctx); ok {
		return sqlc.New(tx)
	}
	return r.queriesConPool
}

// Guardar persiste el estado completo del agregado FactorMFA, siguiendo el
// mismo patrón "intenta UPDATE, si no actualiza ninguna fila hace INSERT"
// que RepositorioUsuarios.Guardar (no hay flag "es nuevo" en el dominio,
// INV-ID-09). Los códigos de respaldo (colección de VOs hijos, §1.2 del
// diseño) se persisten con upsert por hash_codigo (UpsertCodigoRespaldoMFA),
// nunca con un borrado previo: codigos_respaldo_mfa tiene DELETE revocado
// para rol_aplicacion (migración 000015, "un código consumido queda
// marcado con usado_en, no se borra"), así que un patrón
// "borra e inserta de nuevo" fallaría contra la base real. El upsert cubre
// tanto la creación inicial (ConfirmarFactorMFA, 10 filas nuevas) como el
// consumo posterior de un código (VerificarOTP/DeshabilitarMFA, la misma
// fila con usado_en actualizado) con una única query.
func (r *RepositorioFactoresMFA) Guardar(ctx context.Context, f *dominio.FactorMFA) error {
	q := r.consultas(ctx)

	idPg, err := idFactorAPg(f.ID())
	if err != nil {
		return err
	}
	usuarioIDPg, err := idAPg(f.UsuarioID())
	if err != nil {
		return err
	}
	confirmadoEnPg := pgtype.Timestamptz{}
	if confirmadoEn, ok := f.ConfirmadoEn(); ok {
		confirmadoEnPg = tiempoAPg(confirmadoEn)
	}

	_, err = q.ActualizarFactorMFA(ctx, sqlc.ActualizarFactorMFAParams{
		ID:             idPg,
		SecretoCifrado: f.SecretoCifrado().Valor(),
		Confirmado:     f.EstaConfirmado(),
		Activo:         f.EstaActivo(),
		ConfirmadoEn:   confirmadoEnPg,
	})
	switch {
	case err == nil:
		// Fila existente actualizada.
	case errors.Is(err, pgx.ErrNoRows):
		// No existía: se trata de un alta nueva (HabilitarMFA).
		if _, err := q.CrearFactorMFA(ctx, sqlc.CrearFactorMFAParams{
			ID:             idPg,
			UsuarioID:      usuarioIDPg,
			Tipo:           f.Tipo().Valor(),
			SecretoCifrado: f.SecretoCifrado().Valor(),
			Confirmado:     f.EstaConfirmado(),
			Activo:         f.EstaActivo(),
			CreadoEn:       tiempoAPg(f.CreadoEn()),
			ConfirmadoEn:   confirmadoEnPg,
		}); err != nil {
			return traducirError(err)
		}
	default:
		return traducirError(err)
	}

	for _, c := range f.CodigosRespaldo() {
		usadoEnPg := pgtype.Timestamptz{}
		if usadoEn, ok := c.UsadoEn(); ok {
			usadoEnPg = tiempoAPg(usadoEn)
		}
		if err := q.UpsertCodigoRespaldoMFA(ctx, sqlc.UpsertCodigoRespaldoMFAParams{
			FactorID:   idPg,
			HashCodigo: c.HashCodigo().Valor(),
			UsadoEn:    usadoEnPg,
		}); err != nil {
			return traducirError(err)
		}
	}

	return nil
}

// BuscarPorID devuelve (nil, nil) cuando no existe el factor, mismo
// contrato que RepositorioUsuarios.BuscarPorID.
func (r *RepositorioFactoresMFA) BuscarPorID(ctx context.Context, id dominio.IDFactorMFA) (*dominio.FactorMFA, error) {
	idPg, err := idFactorAPg(id)
	if err != nil {
		return nil, err
	}
	fila, err := r.consultas(ctx).ObtenerFactorMFAPorID(ctx, idPg)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, traducirError(err)
	}
	codigos, err := r.codigosRespaldoDeFactor(ctx, idPg)
	if err != nil {
		return nil, err
	}
	return factorMFADesdeFila(fila, codigos)
}

// BuscarConfirmadosDeUsuario es el camino caliente de VerificarOTP en cada
// login (§3.4 del diseño): "confirmados" significa SIEMPRE confirmado=true
// AND activo=true (ver el comentario del puerto en puertos/salida.go) — la
// query ObtenerFactoresMFAConfirmadosActivosDeUsuario ya filtra por ambas
// columnas, no solo por confirmado.
func (r *RepositorioFactoresMFA) BuscarConfirmadosDeUsuario(ctx context.Context, u dominio.IDUsuario) ([]*dominio.FactorMFA, error) {
	usuarioIDPg, err := idAPg(u)
	if err != nil {
		return nil, err
	}
	filas, err := r.consultas(ctx).ObtenerFactoresMFAConfirmadosActivosDeUsuario(ctx, usuarioIDPg)
	if err != nil {
		return nil, traducirError(err)
	}
	factores := make([]*dominio.FactorMFA, 0, len(filas))
	for _, fila := range filas {
		codigos, err := r.codigosRespaldoDeFactor(ctx, fila.ID)
		if err != nil {
			return nil, err
		}
		factor, err := factorMFADesdeFila(fila, codigos)
		if err != nil {
			return nil, err
		}
		factores = append(factores, factor)
	}
	return factores, nil
}

// ContarConfirmadosDeUsuario respeta el mismo filtro
// confirmado=true AND activo=true que BuscarConfirmadosDeUsuario (ver
// comentario de arriba): es lo que permite un HabilitarMFA posterior tras
// deshabilitar el factor anterior (ADR 0037/commit 1e06146).
func (r *RepositorioFactoresMFA) ContarConfirmadosDeUsuario(ctx context.Context, u dominio.IDUsuario) (int, error) {
	usuarioIDPg, err := idAPg(u)
	if err != nil {
		return 0, err
	}
	total, err := r.consultas(ctx).ContarFactoresMFAConfirmadosActivosDeUsuario(ctx, usuarioIDPg)
	if err != nil {
		return 0, traducirError(err)
	}
	return int(total), nil
}

// codigosRespaldoDeFactor carga la colección completa de códigos de
// respaldo (VO hijo, §1.2 del diseño) de un factor ya persistido.
func (r *RepositorioFactoresMFA) codigosRespaldoDeFactor(ctx context.Context, idFactorPg pgtype.UUID) ([]sqlc.CodigosRespaldoMfa, error) {
	filas, err := r.consultas(ctx).ObtenerCodigosRespaldoDeFactor(ctx, idFactorPg)
	if err != nil {
		return nil, traducirError(err)
	}
	return filas, nil
}

// factorMFADesdeFila reconstruye el agregado dominio.FactorMFA a partir de
// una fila sqlc.FactoresMfa y su colección de códigos de respaldo, vía
// dominio.ReconstituirFactorMFA (no acumula eventos: es rehidratación, no
// una operación de negocio nueva).
func factorMFADesdeFila(fila sqlc.FactoresMfa, codigosFilas []sqlc.CodigosRespaldoMfa) (*dominio.FactorMFA, error) {
	id, err := dominio.IDFactorMFADesde(fila.ID.String())
	if err != nil {
		return nil, err
	}
	usuarioID, err := dominio.IDUsuarioDesde(fila.UsuarioID.String())
	if err != nil {
		return nil, err
	}
	tipo, err := dominio.TipoFactorDesde(fila.Tipo)
	if err != nil {
		return nil, err
	}
	secretoCifrado, err := dominio.NuevoSecretoTOTPCifrado(fila.SecretoCifrado)
	if err != nil {
		return nil, err
	}

	var confirmadoEn *time.Time
	if fila.ConfirmadoEn.Valid {
		t := fila.ConfirmadoEn.Time
		confirmadoEn = &t
	}

	codigos := make([]dominio.CodigoRespaldoMFA, 0, len(codigosFilas))
	for _, cf := range codigosFilas {
		hash, err := dominio.NuevoHashCodigoRespaldo(cf.HashCodigo)
		if err != nil {
			return nil, err
		}
		var usadoEn *time.Time
		if cf.UsadoEn.Valid {
			t := cf.UsadoEn.Time
			usadoEn = &t
		}
		codigos = append(codigos, dominio.ReconstituirCodigoRespaldoMFA(hash, usadoEn))
	}

	return dominio.ReconstituirFactorMFA(
		id,
		usuarioID,
		tipo,
		secretoCifrado,
		fila.Confirmado,
		fila.Activo,
		fila.CreadoEn.Time,
		confirmadoEn,
		codigos,
	), nil
}
