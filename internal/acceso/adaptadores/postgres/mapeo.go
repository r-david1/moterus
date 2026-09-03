package postgres

import (
	"net/netip"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/r-david1/moterus/internal/acceso/adaptadores/postgres/sqlc"
	"github.com/r-david1/moterus/internal/acceso/dominio"
)

// Este archivo traduce entre las filas generadas por sqlc (sqlc.Sesione,
// sqlc.TokensRefresco) y el agregado dominio.Sesion / su entidad interna
// TokenRefrescoEmitido, en ambos sentidos. Es la única frontera del paquete
// que conoce ambos vocabularios (INV-ACC-20: ningún otro contexto ve esta
// traducción).

// uuidNulo es el UUID nulo que dominio.IDSesionDesde/IDUsuarioDesde ya
// rechazan como identificador válido (esUUIDNulo). Se usa como sentinela
// para "sin excepción" en RevocarActivasDeUsuario cuando
// dominio.IDSesion.EsVacio() (ver comentario de cabecera de
// db/consultas/acceso.sql): nunca puede coincidir con un IDSesion real
// (UUIDv7).
const uuidNulo = "00000000-0000-0000-0000-000000000000"

// idSesionAPg convierte un dominio.IDSesion ya validado a pgtype.UUID.
func idSesionAPg(id dominio.IDSesion) (pgtype.UUID, error) {
	var v pgtype.UUID
	if err := v.Scan(id.String()); err != nil {
		return pgtype.UUID{}, err
	}
	return v, nil
}

// idSesionExceptoAPg convierte el parámetro "excepto" de
// RevocarActivasDeUsuario: si viene vacío (no preservar ninguna sesión) se
// traduce al UUID nulo, que nunca coincide con un IDSesion real.
func idSesionExceptoAPg(id dominio.IDSesion) (pgtype.UUID, error) {
	if id.EsVacio() {
		var v pgtype.UUID
		if err := v.Scan(uuidNulo); err != nil {
			return pgtype.UUID{}, err
		}
		return v, nil
	}
	return idSesionAPg(id)
}

// idUsuarioAPg convierte un dominio.IDUsuario ya validado a pgtype.UUID.
func idUsuarioAPg(id dominio.IDUsuario) (pgtype.UUID, error) {
	var v pgtype.UUID
	if err := v.Scan(id.String()); err != nil {
		return pgtype.UUID{}, err
	}
	return v, nil
}

// tiempoAPg convierte un time.Time a pgtype.Timestamptz válido.
func tiempoAPg(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

// tiempoOpcionalAPg convierte un *time.Time (posiblemente nil) a
// pgtype.Timestamptz, NULL cuando el puntero es nil.
func tiempoOpcionalAPg(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

// textoOpcionalAPg devuelve NULL para una cadena vacía, en vez de
// persistir "" — motivo_revocacion/agente_usuario/huella_dispositivo/
// hash_sucesor son columnas nullable donde "" y "sin dato" no deben
// confundirse.
func textoOpcionalAPg(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

// ipAPg convierte una dominio.DireccionIP (posiblemente vacía) al puntero
// *netip.Addr que sqlc genera para la columna INET.
func ipAPg(ip dominio.DireccionIP) *netip.Addr {
	if ip.EsVacia() {
		return nil
	}
	addr, err := netip.ParseAddr(ip.String())
	if err != nil {
		return nil
	}
	return &addr
}

func textoDePg(t pgtype.Text) string {
	if !t.Valid {
		return ""
	}
	return t.String
}

func ipDePg(ip *netip.Addr) string {
	if ip == nil {
		return ""
	}
	return ip.String()
}

// sesionDesdeFila reconstruye el agregado dominio.Sesion a partir de una
// fila sqlc.Sesione ya persistida, vía dominio.Reconstituir (no acumula
// eventos: es rehidratación, no una operación de negocio nueva).
// refrescoVigente puede ir nil: los métodos de negocio que lo requieren
// (Rotar) fallan explícitamente si falta, y BuscarPorID/
// ListarActivasDeUsuario/RevocarActivasDeUsuario nunca llaman a Rotar sobre
// el agregado que devuelven (ver comentario de BuscarPorHashRefresco en
// repositorio_sesiones.go para el único caso que sí necesita hidratarlo).
func sesionDesdeFila(fila sqlc.Sesione, refrescoVigente *dominio.TokenRefrescoEmitido) (*dominio.Sesion, error) {
	id, err := dominio.IDSesionDesde(fila.ID.String())
	if err != nil {
		return nil, err
	}
	usuarioID, err := dominio.IDUsuarioDesde(fila.UsuarioID.String())
	if err != nil {
		return nil, err
	}
	estado, err := dominio.EstadoSesionDesde(fila.Estado)
	if err != nil {
		return nil, err
	}
	origen, err := dominio.NuevoOrigenSolicitud(
		ipDePg(fila.IpOrigen),
		textoDePg(fila.AgenteUsuario),
		textoDePg(fila.HuellaDispositivo),
		"",
	)
	if err != nil {
		return nil, err
	}

	var motivo dominio.MotivoRevocacion
	if v := textoDePg(fila.MotivoRevocacion); v != "" {
		motivo, err = dominio.MotivoRevocacionDesde(v)
		if err != nil {
			return nil, err
		}
	}

	var ultimaRenovacionEn *time.Time
	if fila.UltimaRenovacionEn.Valid {
		t := fila.UltimaRenovacionEn.Time
		ultimaRenovacionEn = &t
	}
	var revocadaEn *time.Time
	if fila.RevocadaEn.Valid {
		t := fila.RevocadaEn.Time
		revocadaEn = &t
	}

	return dominio.Reconstituir(
		id,
		usuarioID,
		estado,
		int(fila.Generacion),
		refrescoVigente,
		origen,
		fila.CreadaEn.Time,
		fila.ActualizadaEn.Time,
		ultimaRenovacionEn,
		fila.ExpiraInactividadEn.Time,
		fila.ExpiraAbsolutoEn.Time,
		revocadaEn,
		motivo,
	), nil
}

// tokenRefrescoEmitidoDesdeFila reconstruye un
// dominio.TokenRefrescoEmitido a partir de una fila sqlc.TokensRefresco ya
// persistida.
func tokenRefrescoEmitidoDesdeFila(fila sqlc.TokensRefresco) (dominio.TokenRefrescoEmitido, error) {
	hash, err := dominio.NuevoHashTokenRefresco(fila.HashToken)
	if err != nil {
		return dominio.TokenRefrescoEmitido{}, err
	}

	var consumidoEn *time.Time
	if fila.ConsumidoEn.Valid {
		t := fila.ConsumidoEn.Time
		consumidoEn = &t
	}
	var hashSucesor *dominio.HashTokenRefresco
	if v := textoDePg(fila.HashSucesor); v != "" {
		h, err := dominio.NuevoHashTokenRefresco(v)
		if err != nil {
			return dominio.TokenRefrescoEmitido{}, err
		}
		hashSucesor = &h
	}

	return dominio.ReconstituirTokenRefrescoEmitido(
		hash,
		int(fila.Generacion),
		fila.EmitidoEn.Time,
		fila.ExpiraEn.Time,
		consumidoEn,
		hashSucesor,
	), nil
}
