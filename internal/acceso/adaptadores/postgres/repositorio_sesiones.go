package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/r-david1/moterus/internal/acceso/adaptadores/postgres/sqlc"
	"github.com/r-david1/moterus/internal/acceso/dominio"
	"github.com/r-david1/moterus/internal/acceso/puertos"
	"github.com/r-david1/moterus/internal/plataforma/bd"
)

// RepositorioSesiones implementa puertos.RepositorioSesiones sobre
// pgx/sqlc. Cuando el ctx recibido lleva una transacción publicada por
// UnidadDeTrabajo.Ejecutar, las consultas participan en ella; si no, usan
// el pool directamente (mismo patrón que
// identidad/adaptadores/postgres.RepositorioUsuarios).
type RepositorioSesiones struct {
	pool           *pgxpool.Pool
	queriesConPool *sqlc.Queries
}

var _ puertos.RepositorioSesiones = (*RepositorioSesiones)(nil)

// NuevoRepositorioSesiones construye el adaptador sobre un pool pgx ya
// inicializado.
func NuevoRepositorioSesiones(pool *pgxpool.Pool) *RepositorioSesiones {
	return &RepositorioSesiones{pool: pool, queriesConPool: sqlc.New(pool)}
}

// consultas devuelve el conjunto de queries sqlc ligado a la transacción
// activa en ctx (si UnidadDeTrabajo.Ejecutar la publicó) o, en su defecto,
// al pool compartido.
func (r *RepositorioSesiones) consultas(ctx context.Context) *sqlc.Queries {
	if tx, ok := bd.TxDesdeContexto(ctx); ok {
		return sqlc.New(tx)
	}
	return r.queriesConPool
}

// Guardar persiste el agregado completo: la sesión (INSERT si es nueva,
// UPDATE si ya existía — sin un flag "es nuevo" explícito, INV-ACC-09, se
// intenta primero el UPDATE y se cae al INSERT si no afectó ninguna fila,
// mismo patrón que RepositorioUsuarios.Guardar de Identidad) y las
// mutaciones pendientes de su cadena de tokens.
//
// Orden normativo dentro de la cadena de tokens (ver el comentario de
// ConsumirTokenRefresco en db/consultas/acceso.sql): PRIMERO se consume el
// token recién rotado (si lo hay), DESPUÉS se inserta el nuevo vigente. Si
// se invirtiera el orden, habría un instante con dos tokens no consumidos
// para la misma sesión y el índice único parcial fallaría incluso sin
// ninguna concurrencia real. Bajo concurrencia real (dos rotaciones
// simultáneas de la misma sesión), es precisamente esta secuencia la que
// hace que el INSERT del segundo transactor viole la unicidad y produzca
// dominio.ErrConcurrenciaSesion (§6 del diseño).
func (r *RepositorioSesiones) Guardar(ctx context.Context, s *dominio.Sesion) error {
	q := r.consultas(ctx)

	idPg, err := idSesionAPg(s.ID())
	if err != nil {
		return err
	}

	var motivoRevocacionTexto string
	if m, ok := s.MotivoRevocacion(); ok {
		motivoRevocacionTexto = m.Valor()
	}
	var ultimaRenovacionEn *time.Time
	if t, ok := s.UltimaRenovacionEn(); ok {
		copia := t
		ultimaRenovacionEn = &copia
	}
	var revocadaEn *time.Time
	if t, ok := s.RevocadaEn(); ok {
		copia := t
		revocadaEn = &copia
	}

	_, err = q.ActualizarSesion(ctx, sqlc.ActualizarSesionParams{
		ID:                  idPg,
		Estado:              s.Estado().String(),
		Generacion:          int32(s.Generacion()),
		ActualizadaEn:       tiempoAPg(s.ActualizadaEn()),
		UltimaRenovacionEn:  tiempoOpcionalAPg(ultimaRenovacionEn),
		ExpiraInactividadEn: tiempoAPg(s.ExpiraInactividadEn()),
		RevocadaEn:          tiempoOpcionalAPg(revocadaEn),
		MotivoRevocacion:    textoOpcionalAPg(motivoRevocacionTexto),
	})
	switch {
	case err == nil:
		// Fila existente actualizada.
	case errors.Is(err, pgx.ErrNoRows):
		// No existía: se trata de un alta nueva (IniciarSesion).
		usuarioIDPg, errID := idUsuarioAPg(s.UsuarioID())
		if errID != nil {
			return errID
		}
		if _, err := q.CrearSesion(ctx, sqlc.CrearSesionParams{
			ID:                  idPg,
			UsuarioID:           usuarioIDPg,
			Estado:              s.Estado().String(),
			Generacion:          int32(s.Generacion()),
			CreadaEn:            tiempoAPg(s.CreadaEn()),
			ExpiraInactividadEn: tiempoAPg(s.ExpiraInactividadEn()),
			ExpiraAbsolutoEn:    tiempoAPg(s.ExpiraAbsolutoEn()),
			IpOrigen:            ipAPg(s.OrigenCreacion().IP()),
			AgenteUsuario:       textoOpcionalAPg(s.OrigenCreacion().AgenteUsuario()),
			HuellaDispositivo:   textoOpcionalAPg(s.OrigenCreacion().HuellaDispositivo()),
		}); err != nil {
			return traducirError(err)
		}
	default:
		return traducirError(err)
	}

	// PRIMERO: consumir el token recién rotado (si lo hay). Ver el
	// comentario de cabecera sobre el orden normativo.
	if consumido, ok := s.RefrescoRecienConsumido(); ok {
		var hashSucesorTexto string
		if hs, okHS := consumido.HashSucesor(); okHS {
			hashSucesorTexto = hs.Valor()
		}
		consumidoEn, _ := consumido.ConsumidoEn()
		if err := q.ConsumirTokenRefresco(ctx, sqlc.ConsumirTokenRefrescoParams{
			HashToken:   consumido.Hash().Valor(),
			ConsumidoEn: tiempoAPg(consumidoEn),
			HashSucesor: textoOpcionalAPg(hashSucesorTexto),
		}); err != nil {
			return traducirError(err)
		}
	}

	// DESPUÉS: insertar el nuevo token vigente (idempotente vía ON
	// CONFLICT DO NOTHING).
	if vigente, ok := s.RefrescoVigente(); ok {
		if err := q.CrearTokenRefresco(ctx, sqlc.CrearTokenRefrescoParams{
			HashToken:  vigente.Hash().Valor(),
			SesionID:   idPg,
			Generacion: int32(vigente.Generacion()),
			EmitidoEn:  tiempoAPg(vigente.EmitidoEn()),
			ExpiraEn:   tiempoAPg(vigente.ExpiraEn()),
		}); err != nil {
			return traducirError(err)
		}
	}

	return nil
}

// BuscarPorID devuelve (nil, nil) cuando no existe la sesión. El agregado
// se reconstruye SIN el token de refresco vigente hidratado: ni
// ValidarAcceso (EstaViva/PuedeRenovarse) ni CerrarSesion (Revocar) tocan
// RefrescoVigente, así que no vale la pena una consulta adicional para un
// dato que ningún llamador de BuscarPorID necesita hoy.
func (r *RepositorioSesiones) BuscarPorID(ctx context.Context, id dominio.IDSesion) (*dominio.Sesion, error) {
	idPg, err := idSesionAPg(id)
	if err != nil {
		return nil, err
	}
	fila, err := r.consultas(ctx).ObtenerSesionPorID(ctx, idPg)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, traducirError(err)
	}
	return sesionDesdeFila(fila, nil)
}

// BuscarPorHashRefresco resuelve el token presentado (§2.2 del diseño,
// nota de implementación de puertos.RepositorioSesiones): busca primero la
// fila de tokens_refresco por hash, y solo si existe carga la sesión
// dueña.
//
//   - Hash desconocido: (nil, RefrescoDesconocido, 0, nil).
//   - Hash consumido (REUSO): se reconstruye la sesión SIN el token
//     vigente hidratado — RenovarSesion.manejarReuso solo llama a
//     sesion.Revocar (no toca RefrescoVigente) y usa sesion.Generacion()
//     (columna sesiones.generacion, no el token) para el evento de reuso.
//   - Hash vigente: la fila encontrada ES el token vigente, así que se
//     reconstruye la sesión CON ese token hidratado (lo que
//     Sesion.Rotar necesita).
func (r *RepositorioSesiones) BuscarPorHashRefresco(ctx context.Context, h dominio.HashTokenRefresco) (*dominio.Sesion, puertos.SituacionRefresco, int, error) {
	q := r.consultas(ctx)

	filaToken, err := q.ObtenerTokenRefrescoPorHash(ctx, h.Valor())
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, puertos.RefrescoDesconocido, 0, nil
		}
		return nil, puertos.RefrescoDesconocido, 0, traducirError(err)
	}

	filaSesion, err := q.ObtenerSesionPorID(ctx, filaToken.SesionID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// FK sesion_id -> sesiones(id) garantiza que esto no debería
			// ocurrir; se trata como error de infraestructura, no como
			// "sesión no encontrada" de negocio.
			return nil, puertos.RefrescoDesconocido, 0, traducirError(err)
		}
		return nil, puertos.RefrescoDesconocido, 0, traducirError(err)
	}

	generacionToken := int(filaToken.Generacion)

	if filaToken.ConsumidoEn.Valid {
		sesion, errSesion := sesionDesdeFila(filaSesion, nil)
		if errSesion != nil {
			return nil, puertos.RefrescoDesconocido, 0, errSesion
		}
		return sesion, puertos.RefrescoConsumido, generacionToken, nil
	}

	tokenVigente, err := tokenRefrescoEmitidoDesdeFila(filaToken)
	if err != nil {
		return nil, puertos.RefrescoDesconocido, 0, err
	}
	sesion, err := sesionDesdeFila(filaSesion, &tokenVigente)
	if err != nil {
		return nil, puertos.RefrescoDesconocido, 0, err
	}
	return sesion, puertos.RefrescoVigente, generacionToken, nil
}

// ListarActivasDeUsuario devuelve las sesiones activas del usuario, sin el
// token de refresco vigente hidratado: ni ListarSesionesCasoDeUso (modelo
// de lectura) ni IniciarSesionCasoDeUso.aplicarLimiteSesiones (que solo
// llama a Revocar sobre la más antigua) necesitan la cadena de rotación.
func (r *RepositorioSesiones) ListarActivasDeUsuario(ctx context.Context, u dominio.IDUsuario) ([]*dominio.Sesion, error) {
	usuarioIDPg, err := idUsuarioAPg(u)
	if err != nil {
		return nil, err
	}
	filas, err := r.consultas(ctx).ListarSesionesActivasDeUsuario(ctx, usuarioIDPg)
	if err != nil {
		return nil, traducirError(err)
	}
	sesiones := make([]*dominio.Sesion, 0, len(filas))
	for _, fila := range filas {
		sesion, err := sesionDesdeFila(fila, nil)
		if err != nil {
			return nil, err
		}
		sesiones = append(sesiones, sesion)
	}
	return sesiones, nil
}

// ContarActivasDeUsuario cuenta las sesiones en estado activa del usuario.
func (r *RepositorioSesiones) ContarActivasDeUsuario(ctx context.Context, u dominio.IDUsuario) (int, error) {
	usuarioIDPg, err := idUsuarioAPg(u)
	if err != nil {
		return 0, err
	}
	total, err := r.consultas(ctx).ContarSesionesActivasDeUsuario(ctx, usuarioIDPg)
	if err != nil {
		return 0, traducirError(err)
	}
	return int(total), nil
}

// RevocarActivasDeUsuario revoca en una sola sentencia todas las sesiones
// activas del usuario, salvo excepto (que puede ir vacío para no preservar
// ninguna — ver idSesionExceptoAPg), y devuelve los ids revocados.
func (r *RepositorioSesiones) RevocarActivasDeUsuario(ctx context.Context, u dominio.IDUsuario, excepto dominio.IDSesion,
	motivo dominio.MotivoRevocacion, ahora time.Time) ([]dominio.IDSesion, error) {
	usuarioIDPg, err := idUsuarioAPg(u)
	if err != nil {
		return nil, err
	}
	exceptoPg, err := idSesionExceptoAPg(excepto)
	if err != nil {
		return nil, err
	}

	filas, err := r.consultas(ctx).RevocarSesionesActivasDeUsuario(ctx, sqlc.RevocarSesionesActivasDeUsuarioParams{
		UsuarioID:        usuarioIDPg,
		RevocadaEn:       tiempoAPg(ahora),
		MotivoRevocacion: textoOpcionalAPg(motivo.Valor()),
		ID:               exceptoPg,
	})
	if err != nil {
		return nil, traducirError(err)
	}

	ids := make([]dominio.IDSesion, 0, len(filas))
	for _, filaID := range filas {
		id, err := dominio.IDSesionDesde(filaID.String())
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}
