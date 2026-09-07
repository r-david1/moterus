package postgres

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/r-david1/moterus/internal/confianza/adaptadores/postgres/sqlc"
	"github.com/r-david1/moterus/internal/confianza/dominio"
)

// Este archivo traduce entre la fila generada por sqlc (sqlc.SalasEspera) y
// el agregado dominio.SalaDeEspera, en ambos sentidos. Es la única frontera
// del paquete que conoce ambos vocabularios (mismo criterio que
// tenencia/adaptadores/postgres/mapeo.go e
// identidad/adaptadores/postgres/mapeo.go).

func uuidAPg(valor string) (pgtype.UUID, error) {
	var v pgtype.UUID
	if err := v.Scan(valor); err != nil {
		return pgtype.UUID{}, err
	}
	return v, nil
}

// uuidOpcionalAPg construye un pgtype.UUID inválido (NULL) cuando valor es
// "", o el UUID escaneado en caso contrario. Se usa para
// alcance_organizacion_id (NULL en alcance sistema) y creada_por (NULL en
// una sala de alcance sistema, operada fuera de la API).
func uuidOpcionalAPg(valor string) (pgtype.UUID, error) {
	if valor == "" {
		return pgtype.UUID{}, nil
	}
	return uuidAPg(valor)
}

func tiempoAPg(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func tiempoOpcionalAPg(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return tiempoAPg(*t)
}

// pgTimestampOpcional adapta el patrón de getter "(time.Time, bool)" que
// usa SalaDeEspera (AbiertaEn/CerradaEn) al pgtype.Timestamptz que espera
// sqlc.
func pgTimestampOpcional(t time.Time, ok bool) pgtype.Timestamptz {
	if !ok {
		return pgtype.Timestamptz{}
	}
	return tiempoAPg(t)
}

func tiempoOpcionalDePg(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	copia := t.Time
	return &copia
}

// alcanceOrganizacionIDDeSala devuelve el IDOrganizacion en su forma
// primitiva ("" para alcance sistema), listo para uuidOpcionalAPg.
func alcanceOrganizacionIDDeSala(s *dominio.SalaDeEspera) string {
	if id, ok := s.Alcance().OrganizacionID(); ok {
		return id.String()
	}
	return ""
}

// creadaPorIDDeSala devuelve el IDUsuario en su forma primitiva ("" para
// una sala de alcance sistema).
func creadaPorIDDeSala(s *dominio.SalaDeEspera) string {
	if id, ok := s.CreadaPor(); ok {
		return id.String()
	}
	return ""
}

// salaDeEsperaDesdeFila reconstruye el agregado dominio.SalaDeEspera a
// partir de una fila sqlc.SalasEspera, vía dominio.ReconstituirSalaDeEspera
// (no acumula eventos: es rehidratación, no una operación de negocio
// nueva). version_config no se traduce: el agregado de dominio no la
// expone (ver el comentario de GuardarSalaDeEspera en
// db/consultas/confianza.sql).
func salaDeEsperaDesdeFila(fila sqlc.SalasEspera) (*dominio.SalaDeEspera, error) {
	id, err := dominio.IDSalaDeEsperaDesde(fila.ID.String())
	if err != nil {
		return nil, err
	}
	alias, err := dominio.NuevoAliasSala(fila.Alias)
	if err != nil {
		return nil, err
	}
	organizacionID := ""
	if fila.AlcanceOrganizacionID.Valid {
		organizacionID = fila.AlcanceOrganizacionID.String()
	}
	alcance, err := dominio.ReconstituirAlcanceSala(fila.AlcanceTipo, organizacionID)
	if err != nil {
		return nil, err
	}
	ruta, err := dominio.RutaProtegidaDesde(fila.RutaProtegida)
	if err != nil {
		return nil, err
	}
	estado, err := dominio.EstadoSalaDesde(fila.Estado)
	if err != nil {
		return nil, err
	}
	ritmo, err := dominio.NuevoRitmoAdmision(int(fila.RitmoAdmision))
	if err != nil {
		return nil, err
	}
	modo, err := dominio.ModoDegradadoDesde(fila.ModoDegradado)
	if err != nil {
		return nil, err
	}
	politica, err := dominio.NuevaPoliticaSala(
		ritmo,
		fila.CapacidadMaximaCola,
		time.Duration(fila.VentanaReclamoMs)*time.Millisecond,
		modo,
	)
	if err != nil {
		return nil, err
	}
	var creadaPor *dominio.IDUsuario
	if fila.CreadaPor.Valid {
		v, err := dominio.IDUsuarioDesde(fila.CreadaPor.String())
		if err != nil {
			return nil, err
		}
		creadaPor = &v
	}

	return dominio.ReconstituirSalaDeEspera(
		id, alias, alcance, ruta, estado, politica,
		fila.CursorBase, fila.RelojDesde.Time,
		creadaPor, fila.CreadaEn.Time,
		tiempoOpcionalDePg(fila.AbiertaEn),
		tiempoOpcionalDePg(fila.CerradaEn),
	), nil
}
