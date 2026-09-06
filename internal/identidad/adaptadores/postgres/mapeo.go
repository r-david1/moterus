package postgres

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/r-david1/moterus/internal/identidad/adaptadores/postgres/sqlc"
	"github.com/r-david1/moterus/internal/identidad/dominio"
)

// Este archivo traduce entre las filas generadas por sqlc (sqlc.Usuario) y
// el agregado dominio.Usuario, en ambos sentidos. Es la única frontera del
// paquete que conoce ambos vocabularios (INV-ID-20: ningún otro contexto ve
// esta traducción).

// idAPg convierte un dominio.IDUsuario ya validado a pgtype.UUID.
func idAPg(id dominio.IDUsuario) (pgtype.UUID, error) {
	var v pgtype.UUID
	if err := v.Scan(id.String()); err != nil {
		return pgtype.UUID{}, err
	}
	return v, nil
}

// idFactorAPg convierte un dominio.IDFactorMFA ya validado a pgtype.UUID
// (docs/design/otp-mfa.md §2.2, migración 000015).
func idFactorAPg(id dominio.IDFactorMFA) (pgtype.UUID, error) {
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

// usuarioDesdeFila reconstruye el agregado dominio.Usuario a partir de una
// fila sqlc.Usuario ya persistida, vía dominio.Reconstituir (no acumula
// eventos: es rehidratación, no una operación de negocio nueva).
func usuarioDesdeFila(fila sqlc.Usuario) (*dominio.Usuario, error) {
	id, err := dominio.IDUsuarioDesde(fila.ID.String())
	if err != nil {
		return nil, err
	}
	correo, err := dominio.NuevoCorreo(fila.Correo)
	if err != nil {
		return nil, err
	}
	hash, err := dominio.NuevoHashContrasena(fila.ContrasenaHash)
	if err != nil {
		return nil, err
	}
	credencial, err := dominio.NuevaCredencial(hash, fila.ActualizadoEn.Time)
	if err != nil {
		return nil, err
	}
	estado, err := dominio.EstadoUsuarioDesde(fila.Estado)
	if err != nil {
		return nil, err
	}

	var ultimoAcceso *time.Time
	if fila.UltimoAccesoEn.Valid {
		t := fila.UltimoAccesoEn.Time
		ultimoAcceso = &t
	}

	return dominio.Reconstituir(
		id,
		correo,
		credencial,
		estado,
		fila.TieneMfa,
		fila.CreadoEn.Time,
		fila.ActualizadoEn.Time,
		ultimoAcceso,
	), nil
}
