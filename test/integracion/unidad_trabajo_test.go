package integracion

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/r-david1/moterus/internal/identidad/adaptadores/auditoria"
	identidadpostgres "github.com/r-david1/moterus/internal/identidad/adaptadores/postgres"
	"github.com/r-david1/moterus/internal/identidad/dominio"
)

// TestUnidadDeTrabajo_Ejecutar_FalloAMitadDeTransaccion_HaceRollbackCompleto
// verifica contra Postgres real la garantía central de ADR 0005/INV-ID-15:
// negocio + auditoría se persisten en la MISMA transacción, y si cualquier
// paso falla a mitad de camino, NADA queda persistido — ni el usuario ni el
// evento de auditoría. Los mocks de puertos.UnidadDeTrabajo (aplicacion/*)
// ya cubrieron que el caso de uso invoca Ejecutar correctamente; lo que
// nunca se probó hasta este test es que bd.EjecutarEnTransaccion realmente
// hace ROLLBACK en una transacción pgx real, no solo que "el fn recibió el
// ctx correcto".
//
// El fallo se fuerza con un caso 100% real de Postgres (no un error
// inyectado desde Go): un segundo Guardar con el mismo correo dentro de la
// misma transacción que el primer Guardar + el registro de auditoría,
// violando el índice único usuarios_correo_idx (INV-ID-02).
func TestUnidadDeTrabajo_Ejecutar_FalloAMitadDeTransaccion_HaceRollbackCompleto(t *testing.T) {
	pool := poolAplicacion(t)
	duenoPool := poolDueno(t)
	ctx := context.Background()

	repo := identidadpostgres.NuevoRepositorioUsuarios(pool)
	uow := identidadpostgres.NuevaUnidadDeTrabajo(pool)
	registroAuditoria := auditoria.NuevoRegistroAuditoria(pool)
	origen := origenDePrueba(t)

	correoStr := correoUnico(t, "rollback-real")
	usuarioA := nuevoUsuarioDePrueba(t, correoStr)
	t.Cleanup(func() { borrarUsuario(t, duenoPool, usuarioA.ID().String()) })

	eventoA := dominio.NuevoUsuarioRegistrado(usuarioA.ID(), usuarioA.Correo(), time.Now().UTC())

	errTransaccion := uow.Ejecutar(ctx, func(ctxTx context.Context) error {
		// 1. Guarda el usuario A: esta escritura, si la transacción no
		// hiciera rollback, sí sería válida por sí sola.
		if err := repo.Guardar(ctxTx, usuarioA); err != nil {
			return err
		}
		// 2. Audita su alta, en la MISMA transacción (ADR 0005).
		if err := registroAuditoria.Registrar(ctxTx, eventoA, origen); err != nil {
			return err
		}
		// 3. Fuerza un fallo real de Postgres A MITAD de la transacción:
		// un segundo usuario con el correo duplicado.
		usuarioB := nuevoUsuarioDePrueba(t, correoStr)
		return repo.Guardar(ctxTx, usuarioB)
	})

	var errCorreo *dominio.ErrCorreoYaRegistrado
	if !errors.As(errTransaccion, &errCorreo) {
		t.Fatalf("Ejecutar() = %v (%T), se esperaba que propagara *dominio.ErrCorreoYaRegistrado", errTransaccion, errTransaccion)
	}

	// Nada debe haber quedado persistido: ni el usuario A...
	encontrado, err := repo.BuscarPorID(ctx, usuarioA.ID())
	if err != nil {
		t.Fatalf("BuscarPorID() tras el rollback devolvió error inesperado: %v", err)
	}
	if encontrado != nil {
		t.Fatalf("el usuario A quedó persistido pese al rollback de la transacción (ADR 0005 violado)")
	}

	// ...ni el evento de auditoría de su alta. Se consulta con el rol dueño
	// para leer sin las restricciones de rol_login_identidad.
	var cuenta int
	err = duenoPool.QueryRow(ctx,
		`SELECT count(*) FROM auditoria WHERE recurso_id = $1 AND accion = 'usuario.registrado'`,
		usuarioA.ID().String(),
	).Scan(&cuenta)
	if err != nil {
		t.Fatalf("consultando auditoria tras el rollback: %v", err)
	}
	if cuenta != 0 {
		t.Fatalf("quedaron %d fila(s) de auditoría de usuario.registrado pese al rollback (ADR 0005 violado)", cuenta)
	}

	// La cadena de hashes de auditoría sigue íntegra: el rollback no dejó
	// nada a medio confirmar que pudiera romper el encadenamiento.
	assertCadenaAuditoriaIntegra(t, duenoPool)
}
