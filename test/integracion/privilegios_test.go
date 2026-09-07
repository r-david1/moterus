package integracion

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// codigoInsuficientePrivilegio es el SQLSTATE de Postgres para
// "insufficient_privilege": lo produce un GRANT/REVOKE, nunca el trigger de
// bloqueo de mutación (auditoria_asignar_cadena.go /
// bloquear_mutacion_auditoria, que usa RAISE EXCEPTION con ERRCODE
// 'insufficient_privilege' TAMBIÉN — ver más abajo el matiz) — por eso
// estos tests verifican explícitamente el SQLSTATE, no solo "hubo un
// error".
const codigoInsuficientePrivilegio = "42501"

// Estos tests formalizan lo que ADR 0005 promete y ADR 0017 documenta:
// rol_login_identidad (el rol de login que usa el proceso api en tiempo de
// ejecución, migración 000003) solo puede SELECT/INSERT sobre auditoria, y
// eso se aplica por privilegios de PostgreSQL (GRANT/REVOKE), no solo por
// disciplina de la aplicación ni por el trigger de bloqueo.
//
// Nota sobre el matiz del comentario de arriba: bloquear_mutacion_auditoria
// (000002, sección 6) también usa ERRCODE = 'insufficient_privilege' al
// lanzar su excepción, así que el SQLSTATE por sí solo no basta para
// distinguir "el GRANT lo bloqueó" de "el trigger lo bloqueó" con un
// superusuario. Lo que sí los distingue es el mensaje: el trigger siempre
// dice literalmente "auditoria es append-only"; el rechazo de ACL de
// Postgres dice "permission denied for table auditoria" y ocurre ANTES de
// que el trigger llegue a ejecutarse (el rewrite del planner rechaza el
// UPDATE/DELETE completo por falta de privilegio de tabla, ni siquiera se
// evalúan filas). Se verifica el mensaje además del código por eso mismo.
func TestPrivilegiosRolLoginIdentidad_UpdateSobreAuditoria_PermissionDenied(t *testing.T) {
	pool := poolAplicacion(t)

	// WHERE false: no hace falta que exista ninguna fila que coincida —
	// el rechazo de ACL ocurre en el análisis del statement, antes de
	// evaluar el predicado o cualquier fila.
	_, err := pool.Exec(t.Context(), `UPDATE auditoria SET resultado = 'exito' WHERE false`)
	if err == nil {
		t.Fatalf("UPDATE sobre auditoria con rol_login_identidad no devolvió error; el REVOKE de la migración 000002 no está teniendo efecto")
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("el error no es *pgconn.PgError: %v (%T)", err, err)
	}
	if pgErr.Code != codigoInsuficientePrivilegio {
		t.Errorf("SQLSTATE = %q, esperado %q (insufficient_privilege)", pgErr.Code, codigoInsuficientePrivilegio)
	}
	if pgErr.Message != "permission denied for table auditoria" {
		t.Errorf("mensaje = %q; se esperaba el rechazo de ACL de Postgres ('permission denied for table auditoria'), "+
			"no la excepción del trigger append-only — indicaría que el UPDATE llegó a evaluarse en vez de "+
			"rechazarse por falta de privilegio", pgErr.Message)
	}
}

func TestPrivilegiosRolLoginIdentidad_DeleteSobreAuditoria_PermissionDenied(t *testing.T) {
	pool := poolAplicacion(t)

	_, err := pool.Exec(t.Context(), `DELETE FROM auditoria WHERE false`)
	if err == nil {
		t.Fatalf("DELETE sobre auditoria con rol_login_identidad no devolvió error; el REVOKE de la migración 000002 no está teniendo efecto")
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("el error no es *pgconn.PgError: %v (%T)", err, err)
	}
	if pgErr.Code != codigoInsuficientePrivilegio {
		t.Errorf("SQLSTATE = %q, esperado %q (insufficient_privilege)", pgErr.Code, codigoInsuficientePrivilegio)
	}
	if pgErr.Message != "permission denied for table auditoria" {
		t.Errorf("mensaje = %q; se esperaba el rechazo de ACL de Postgres, no la excepción del trigger append-only", pgErr.Message)
	}
}

// TestPrivilegiosRolLoginIdentidad_DeleteSobreUsuarios_PermissionDenied
// verifica el comentario explícito de la migración 000003: rol_aplicacion
// tiene SELECT/INSERT/UPDATE sobre usuarios, pero deliberadamente NO
// DELETE ("los usuarios nunca se borran físicamente, solo cambian de
// EstadoUsuario"). borrarUsuario (helper de limpieza de estos tests) usa el
// rol dueño precisamente porque esto es cierto.
func TestPrivilegiosRolLoginIdentidad_DeleteSobreUsuarios_PermissionDenied(t *testing.T) {
	pool := poolAplicacion(t)

	_, err := pool.Exec(t.Context(), `DELETE FROM usuarios WHERE false`)
	if err == nil {
		t.Fatalf("DELETE sobre usuarios con rol_login_identidad no devolvió error; " +
			"la migración 000003 no otorgó DELETE explícitamente, así que esto no debería poder ejecutarse")
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("el error no es *pgconn.PgError: %v (%T)", err, err)
	}
	if pgErr.Code != codigoInsuficientePrivilegio {
		t.Errorf("SQLSTATE = %q, esperado %q (insufficient_privilege)", pgErr.Code, codigoInsuficientePrivilegio)
	}
}

// TestPrivilegiosRolLoginIdentidad_DeleteSobreSalasEspera_PermissionDenied
// verifica el comentario explícito de la migración 000017 (contexto
// Confianza, docs/design/colas-virtuales.md §6.1): rol_aplicacion tiene
// SELECT/INSERT/UPDATE sobre salas_espera, pero deliberadamente NO DELETE
// ("una sala cerrada es evidencia de un evento operativo, se conserva con
// estado terminal — mismo criterio que invitaciones y sesiones"). Un GRANT
// DELETE agregado por error a esta tabla lo detecta este test, no una
// revisión manual del SQL de la migración.
func TestPrivilegiosRolLoginIdentidad_DeleteSobreSalasEspera_PermissionDenied(t *testing.T) {
	pool := poolAplicacion(t)

	_, err := pool.Exec(t.Context(), `DELETE FROM salas_espera WHERE false`)
	if err == nil {
		t.Fatalf("DELETE sobre salas_espera con rol_login_identidad no devolvió error; " +
			"la migración 000017 no otorgó DELETE explícitamente, así que esto no debería poder ejecutarse")
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("el error no es *pgconn.PgError: %v (%T)", err, err)
	}
	if pgErr.Code != codigoInsuficientePrivilegio {
		t.Errorf("SQLSTATE = %q, esperado %q (insufficient_privilege)", pgErr.Code, codigoInsuficientePrivilegio)
	}
}

// TestPrivilegiosRolLoginIdentidad_TruncateSobreSalasEspera_PermissionDenied
// cubre el otro REVOKE explícito de 000017 (TRUNCATE, además de DELETE):
// un TRUNCATE evita cualquier trigger o RLS a nivel de fila, así que merece
// su propia verificación en vez de asumir que el rechazo de DELETE ya lo
// cubre.
func TestPrivilegiosRolLoginIdentidad_TruncateSobreSalasEspera_PermissionDenied(t *testing.T) {
	pool := poolAplicacion(t)

	_, err := pool.Exec(t.Context(), `TRUNCATE salas_espera`)
	if err == nil {
		t.Fatalf("TRUNCATE sobre salas_espera con rol_login_identidad no devolvió error; " +
			"la migración 000017 no otorgó TRUNCATE explícitamente, así que esto no debería poder ejecutarse")
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("el error no es *pgconn.PgError: %v (%T)", err, err)
	}
	if pgErr.Code != codigoInsuficientePrivilegio {
		t.Errorf("SQLSTATE = %q, esperado %q (insufficient_privilege)", pgErr.Code, codigoInsuficientePrivilegio)
	}
}
