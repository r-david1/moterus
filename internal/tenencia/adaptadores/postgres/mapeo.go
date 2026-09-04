package postgres

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/r-david1/moterus/internal/tenencia/adaptadores/postgres/sqlc"
	"github.com/r-david1/moterus/internal/tenencia/dominio"
)

// Este archivo traduce entre las filas generadas por sqlc
// (sqlc.Organizacione, sqlc.Membresia, sqlc.Invitacione) y los agregados
// dominio.Organizacion / dominio.Membresia / dominio.Invitacion, en ambos
// sentidos. Es la única frontera del paquete que conoce ambos vocabularios
// (INV-TEN-29: ninguna consulta de db/consultas/tenencia.sql incluye la
// tabla `usuarios`).

func uuidAPg(valor string) (pgtype.UUID, error) {
	var v pgtype.UUID
	if err := v.Scan(valor); err != nil {
		return pgtype.UUID{}, err
	}
	return v, nil
}

func uuidOpcionalAPg(valor string) pgtype.UUID {
	if valor == "" {
		return pgtype.UUID{}
	}
	var v pgtype.UUID
	if err := v.Scan(valor); err != nil {
		return pgtype.UUID{}
	}
	return v
}

func tiempoAPg(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func tiempoOpcionalAPg(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

func textoOpcionalAPg(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func textoDePg(t pgtype.Text) string {
	if !t.Valid {
		return ""
	}
	return t.String
}

func tiempoOpcionalDePg(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	copia := t.Time
	return &copia
}

// tiempoArchivadaAPg extrae ArchivadaEn() de un agregado Organizacion como
// pgtype.Timestamptz, NULL si todavía no está archivada.
func tiempoArchivadaAPg(o *dominio.Organizacion) pgtype.Timestamptz {
	if t, ok := o.ArchivadaEn(); ok {
		return tiempoAPg(t)
	}
	return pgtype.Timestamptz{}
}

// --- Organizacion -------------------------------------------------------------

func organizacionDesdeFila(fila sqlc.Organizacione) (*dominio.Organizacion, error) {
	id, err := dominio.IDOrganizacionDesde(fila.ID.String())
	if err != nil {
		return nil, err
	}
	alias, err := dominio.NuevoAlias(fila.Alias)
	if err != nil {
		return nil, err
	}
	nombre, err := dominio.NuevoNombre(fila.Nombre)
	if err != nil {
		return nil, err
	}
	estado, err := dominio.EstadoOrganizacionDesde(fila.Estado)
	if err != nil {
		return nil, err
	}
	creadaPor, err := dominio.IDUsuarioDesde(fila.CreadaPor.String())
	if err != nil {
		return nil, err
	}
	var motivo dominio.MotivoCambioEstado
	if v := textoDePg(fila.MotivoEstado); v != "" {
		motivo, err = dominio.NuevoMotivo(v)
		if err != nil {
			return nil, err
		}
	}
	return dominio.ReconstituirOrganizacion(
		id, alias, nombre, estado, creadaPor,
		fila.CreadaEn.Time, fila.ActualizadaEn.Time,
		tiempoOpcionalDePg(fila.ArchivadaEn), motivo,
	), nil
}

// --- Membresia ------------------------------------------------------------

func membresiaDesdeFila(fila sqlc.Membresia) (*dominio.Membresia, error) {
	id, err := dominio.IDMembresiaDesde(fila.ID.String())
	if err != nil {
		return nil, err
	}
	organizacionID, err := dominio.IDOrganizacionDesde(fila.OrganizacionID.String())
	if err != nil {
		return nil, err
	}
	usuarioID, err := dominio.IDUsuarioDesde(fila.UsuarioID.String())
	if err != nil {
		return nil, err
	}
	rol, err := dominio.RolDesde(fila.Rol)
	if err != nil {
		return nil, err
	}
	estado, err := dominio.EstadoMembresiaDesde(fila.Estado)
	if err != nil {
		return nil, err
	}
	var otorgadaPor *dominio.IDUsuario
	if fila.OtorgadaPor.Valid {
		v, err := dominio.IDUsuarioDesde(fila.OtorgadaPor.String())
		if err != nil {
			return nil, err
		}
		otorgadaPor = &v
	}
	return dominio.ReconstituirMembresia(
		id, organizacionID, usuarioID, rol, estado, otorgadaPor,
		fila.CreadaEn.Time, fila.ActualizadaEn.Time,
		tiempoOpcionalDePg(fila.RemovidaEn),
	), nil
}

// --- Invitacion -----------------------------------------------------------

func invitacionDesdeFila(fila sqlc.Invitacione) (*dominio.Invitacion, error) {
	return invitacionDesde(
		fila.ID, fila.OrganizacionID, fila.CorreoDestinatario, fila.RolPropuesto,
		fila.Estado, fila.HashToken, fila.InvitadaPor,
		fila.CreadaEn, fila.ExpiraEn, fila.ResueltaEn,
	)
}

func invitacionDesdeFilaHash(fila sqlc.ObtenerInvitacionPorHashRow) (*dominio.Invitacion, error) {
	return invitacionDesde(
		fila.ID, fila.OrganizacionID, fila.CorreoDestinatario, fila.RolPropuesto,
		fila.Estado, fila.HashToken, fila.InvitadaPor,
		fila.CreadaEn, fila.ExpiraEn, fila.ResueltaEn,
	)
}

func invitacionDesde(
	idPg, organizacionIDPg pgtype.UUID,
	correo, rolPropuesto, estadoTexto, hashToken string,
	invitadaPorPg pgtype.UUID,
	creadaEn, expiraEn, resueltaEn pgtype.Timestamptz,
) (*dominio.Invitacion, error) {
	id, err := dominio.IDInvitacionDesde(idPg.String())
	if err != nil {
		return nil, err
	}
	organizacionID, err := dominio.IDOrganizacionDesde(organizacionIDPg.String())
	if err != nil {
		return nil, err
	}
	destinatario, err := dominio.NuevoCorreoDestinatario(correo)
	if err != nil {
		return nil, err
	}
	rol, err := dominio.RolDesde(rolPropuesto)
	if err != nil {
		return nil, err
	}
	estado, err := dominio.EstadoInvitacionDesde(estadoTexto)
	if err != nil {
		return nil, err
	}
	hash, err := dominio.NuevoHashTokenInvitacion(hashToken)
	if err != nil {
		return nil, err
	}
	invitadaPor, err := dominio.IDUsuarioDesde(invitadaPorPg.String())
	if err != nil {
		return nil, err
	}
	return dominio.ReconstituirInvitacion(
		id, organizacionID, destinatario, rol, estado, hash, invitadaPor,
		creadaEn.Time, expiraEn.Time, tiempoOpcionalDePg(resueltaEn),
	), nil
}
