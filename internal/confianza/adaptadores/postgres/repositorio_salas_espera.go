// Package postgres implementa los adaptadores de persistencia duradera del
// bounded context Confianza (docs/design/colas-virtuales.md §6.1): la
// configuración operativa del agregado SalaDeEspera, migración
// 000017_crear_salas_espera. El estado efímero de una sala (cursor,
// secuencia, tickets) NO vive acá — eso es
// confianza/adaptadores/redis.EstadoDeCola (INV-COLA-12: Postgres es la
// fuente de verdad de la configuración, Redis del estado efímero).
package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/r-david1/moterus/internal/confianza/adaptadores/postgres/sqlc"
	"github.com/r-david1/moterus/internal/confianza/dominio"
	"github.com/r-david1/moterus/internal/confianza/puertos"
	"github.com/r-david1/moterus/internal/plataforma/bd"
)

// RepositorioSalasDeEspera implementa puertos.RepositorioSalasDeEspera
// sobre pgx/sqlc. No hay RLS sobre `salas_espera` (§6.1 del diseño:
// alcance_organizacion_id es una clave opaca para Confianza, no un tenant
// sobre esta tabla), así que, a diferencia de los repositorios de Tenencia,
// este adaptador nunca publica un alcance de RLS: solo participa en la
// transacción activa del ctx si UnidadDeTrabajo.Ejecutar ya la abrió (mismo
// patrón que identidad/adaptadores/postgres.RepositorioFactoresMFA, que
// tampoco es multi-tenant), o usa el pool directamente si no.
type RepositorioSalasDeEspera struct {
	pool           *pgxpool.Pool
	queriesConPool *sqlc.Queries
}

var _ puertos.RepositorioSalasDeEspera = (*RepositorioSalasDeEspera)(nil)

// NuevoRepositorioSalasDeEspera construye el adaptador sobre un pool pgx ya
// inicializado.
func NuevoRepositorioSalasDeEspera(pool *pgxpool.Pool) *RepositorioSalasDeEspera {
	return &RepositorioSalasDeEspera{pool: pool, queriesConPool: sqlc.New(pool)}
}

// consultas devuelve el conjunto de queries sqlc ligado a la transacción
// activa en ctx (si UnidadDeTrabajo.Ejecutar la publicó) o, en su defecto,
// al pool compartido.
func (r *RepositorioSalasDeEspera) consultas(ctx context.Context) *sqlc.Queries {
	if tx, ok := bd.TxDesdeContexto(ctx); ok {
		return sqlc.New(tx)
	}
	return r.queriesConPool
}

// Guardar persiste el agregado completo con un upsert (INSERT ... ON
// CONFLICT (id) DO UPDATE, ver db/consultas/confianza.sql): a diferencia
// del patrón "intenta UPDATE, si no afecta filas hace INSERT" de otros
// repositorios de este repo, acá el propio ON CONFLICT resuelve ambos
// casos en una sola ida a la base, y es el criterio que pide esta
// extensión. La unicidad de (alcance, ruta) entre salas no cerradas
// (INV-COLA-01) y la del alias (§6.1) las garantiza Postgres mediante sus
// índices únicos: cualquier violación la traduce traducirError a
// dominio.ErrSalaYaAbiertaParaLaRuta / dominio.ErrAliasSalaYaRegistrado.
func (r *RepositorioSalasDeEspera) Guardar(ctx context.Context, s *dominio.SalaDeEspera) error {
	idPg, err := uuidAPg(s.ID().String())
	if err != nil {
		return err
	}
	organizacionIDPg, err := uuidOpcionalAPg(alcanceOrganizacionIDDeSala(s))
	if err != nil {
		return err
	}
	creadaPorPg, err := uuidOpcionalAPg(creadaPorIDDeSala(s))
	if err != nil {
		return err
	}
	abiertaEnPg := pgTimestampOpcional(s.AbiertaEn())
	cerradaEnPg := pgTimestampOpcional(s.CerradaEn())

	err = r.consultas(ctx).GuardarSalaDeEspera(ctx, sqlc.GuardarSalaDeEsperaParams{
		ID:                    idPg,
		Alias:                 s.Alias().Normalizado(),
		AlcanceTipo:           s.Alcance().Tipo().String(),
		AlcanceOrganizacionID: organizacionIDPg,
		RutaProtegida:         s.Ruta().String(),
		Estado:                s.Estado().String(),
		RitmoAdmision:         int32(s.Politica().RitmoAdmision().PorSegundo()),
		CapacidadMaximaCola:   s.Politica().CapacidadMaximaCola(),
		VentanaReclamoMs:      int32(s.Politica().VentanaReclamo().Milliseconds()),
		ModoDegradado:         s.Politica().ModoDegradado().String(),
		CursorBase:            s.CursorBase(),
		RelojDesde:            tiempoAPg(s.RelojDesde()),
		CreadaPor:             creadaPorPg,
		CreadaEn:              tiempoAPg(s.CreadaEn()),
		AbiertaEn:             abiertaEnPg,
		CerradaEn:             cerradaEnPg,
	})
	if err != nil {
		return traducirError(err)
	}
	return nil
}

// BuscarPorID devuelve (nil, nil) cuando no existe la sala, mismo contrato
// que el resto de los repositorios de este repositorio (p. ej.
// identidad/adaptadores/postgres.RepositorioFactoresMFA.BuscarPorID).
func (r *RepositorioSalasDeEspera) BuscarPorID(ctx context.Context, id dominio.IDSalaDeEspera) (*dominio.SalaDeEspera, error) {
	idPg, err := uuidAPg(id.String())
	if err != nil {
		return nil, err
	}
	fila, err := r.consultas(ctx).ObtenerSalaDeEsperaPorID(ctx, idPg)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, traducirError(err)
	}
	return salaDeEsperaDesdeFila(fila)
}

// BuscarPorAlias devuelve (nil, nil) cuando no existe la sala.
func (r *RepositorioSalasDeEspera) BuscarPorAlias(ctx context.Context, alias dominio.AliasSala) (*dominio.SalaDeEspera, error) {
	fila, err := r.consultas(ctx).ObtenerSalaDeEsperaPorAlias(ctx, alias.Normalizado())
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, traducirError(err)
	}
	return salaDeEsperaDesdeFila(fila)
}

// ListarVigentes devuelve las salas en estado abierta|drenando (§3.7 del
// diseño: lo consume el reconciliador cada ~15s por réplica).
func (r *RepositorioSalasDeEspera) ListarVigentes(ctx context.Context) ([]*dominio.SalaDeEspera, error) {
	filas, err := r.consultas(ctx).ListarSalasDeEsperaVigentes(ctx)
	if err != nil {
		return nil, traducirError(err)
	}
	resultado := make([]*dominio.SalaDeEspera, 0, len(filas))
	for _, fila := range filas {
		sala, err := salaDeEsperaDesdeFila(fila)
		if err != nil {
			return nil, err
		}
		resultado = append(resultado, sala)
	}
	return resultado, nil
}
