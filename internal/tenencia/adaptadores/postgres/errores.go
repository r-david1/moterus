package postgres

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/r-david1/moterus/internal/tenencia/dominio"
)

// codigoViolacionUnicidad es el SQLSTATE de Postgres para "unique_violation".
const codigoViolacionUnicidad = "23505"

// codigoViolacionCheck es el SQLSTATE de Postgres para "check_violation" —
// el mismo código que usa el CONSTRAINT TRIGGER diferido de INV-TEN-06 (ver
// migración 000009_crear_organizaciones_membresias.up.sql,
// tenencia_verificar_propietario: `RAISE EXCEPTION ... USING ERRCODE =
// '23514'`).
const codigoViolacionCheck = "23514"

// Nombres de los índices únicos y del constraint trigger que este adaptador
// traduce a errores de dominio tipados (mismo patrón que
// acceso/adaptadores/postgres/errores.go y
// identidad/adaptadores/postgres.traducirError).
const (
	indiceAliasOrganizacion      = "organizaciones_alias_idx"
	indiceMembresiaVigente       = "membresias_vigente_idx"
	indiceInvitacionPendiente    = "invitaciones_pendiente_idx"
	constraintAlMenosPropietario = "membresias_al_menos_un_propietario"
)

// traducirError convierte un error del driver pgx en un error de dominio
// tipado cuando corresponde (tabla 1.5 del diseño):
//   - violación del índice único de alias -> ErrAliasYaRegistrado (409)
//   - violación del índice único parcial de membresía vigente ->
//     ErrMembresiaDuplicada (409)
//   - violación del índice único parcial de invitación pendiente ->
//     ErrInvitacionDuplicada (409, caso de carrera: el flujo normal de
//     reinvitación ya revoca la anterior antes de insertar)
//   - violación del CONSTRAINT TRIGGER diferido de INV-TEN-06 ->
//     ErrUltimoPropietario (409)
//
// Cualquier otro error se envuelve sin tipar, para que el llamador no lo
// confunda con un error de negocio.
func traducirError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case codigoViolacionUnicidad:
			switch pgErr.ConstraintName {
			case indiceAliasOrganizacion:
				return &dominio.ErrAliasYaRegistrado{}
			case indiceMembresiaVigente:
				return &dominio.ErrMembresiaDuplicada{}
			case indiceInvitacionPendiente:
				return &dominio.ErrInvitacionDuplicada{}
			}
		case codigoViolacionCheck:
			if pgErr.ConstraintName == constraintAlMenosPropietario {
				return &dominio.ErrUltimoPropietario{}
			}
		}
	}
	return fmt.Errorf("postgres: %w", err)
}
