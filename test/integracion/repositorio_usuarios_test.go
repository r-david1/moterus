package integracion

import (
	"context"
	"errors"
	"testing"

	identidadpostgres "github.com/r-david1/moterus/internal/identidad/adaptadores/postgres"
	"github.com/r-david1/moterus/internal/identidad/dominio"
)

// Estos tests ejercitan identidad/adaptadores/postgres.RepositorioUsuarios
// contra una base de datos Postgres real, conectados con el mismo rol
// (rol_login_identidad) con el que corre el proceso api en producción. Los
// mocks de puertos.RepositorioUsuarios (internal/identidad/puertos/mocks)
// ya cubrieron el contrato desde la perspectiva de aplicacion; aquí se
// verifica que la implementación concreta cumple ese contrato de verdad,
// incluida la traducción de errores de Postgres a errores de dominio.

func TestRepositorioUsuarios_GuardarYBuscarPorID_PersisteEnPostgresReal(t *testing.T) {
	pool := poolAplicacion(t)
	duenoPool := poolDueno(t)
	repo := identidadpostgres.NuevoRepositorioUsuarios(pool)
	ctx := context.Background()

	usuario := nuevoUsuarioDePrueba(t, correoUnico(t, "guardar-buscar-id"))
	t.Cleanup(func() { borrarUsuario(t, duenoPool, usuario.ID().String()) })

	if err := repo.Guardar(ctx, usuario); err != nil {
		t.Fatalf("Guardar() = %v, se esperaba nil", err)
	}

	encontrado, err := repo.BuscarPorID(ctx, usuario.ID())
	if err != nil {
		t.Fatalf("BuscarPorID() devolvió error inesperado: %v", err)
	}
	if encontrado == nil {
		t.Fatalf("BuscarPorID() = nil, se esperaba el usuario recién guardado")
	}
	if !encontrado.ID().EsIgual(usuario.ID()) {
		t.Errorf("ID = %q, esperado %q", encontrado.ID().String(), usuario.ID().String())
	}
	if encontrado.Correo().Normalizado() != usuario.Correo().Normalizado() {
		t.Errorf("Correo = %q, esperado %q", encontrado.Correo().Normalizado(), usuario.Correo().Normalizado())
	}
	// INV-ID-07: todo alta nace en pendiente_verificacion.
	if !encontrado.Estado().EsIgual(dominio.EstadoPendienteVerificacion) {
		t.Errorf("Estado = %q, esperado %q", encontrado.Estado().String(), dominio.EstadoPendienteVerificacion.String())
	}
	if encontrado.Credencial().Hash().Valor() != usuario.Credencial().Hash().Valor() {
		t.Errorf("el hash persistido no coincide con el guardado")
	}
}

func TestRepositorioUsuarios_BuscarPorCorreo_EncuentraUsuarioPersistido(t *testing.T) {
	pool := poolAplicacion(t)
	duenoPool := poolDueno(t)
	repo := identidadpostgres.NuevoRepositorioUsuarios(pool)
	ctx := context.Background()

	correoStr := correoUnico(t, "buscar-por-correo")
	usuario := nuevoUsuarioDePrueba(t, correoStr)
	t.Cleanup(func() { borrarUsuario(t, duenoPool, usuario.ID().String()) })

	if err := repo.Guardar(ctx, usuario); err != nil {
		t.Fatalf("Guardar() = %v, se esperaba nil", err)
	}

	correo, err := dominio.NuevoCorreo(correoStr)
	if err != nil {
		t.Fatalf("NuevoCorreo(): %v", err)
	}
	encontrado, err := repo.BuscarPorCorreo(ctx, correo)
	if err != nil {
		t.Fatalf("BuscarPorCorreo() devolvió error inesperado: %v", err)
	}
	if encontrado == nil {
		t.Fatalf("BuscarPorCorreo() = nil, se esperaba el usuario recién guardado")
	}
	if !encontrado.ID().EsIgual(usuario.ID()) {
		t.Errorf("ID = %q, esperado %q", encontrado.ID().String(), usuario.ID().String())
	}
}

func TestRepositorioUsuarios_BuscarPorID_UsuarioInexistente_DevuelveNilSinError(t *testing.T) {
	pool := poolAplicacion(t)
	repo := identidadpostgres.NuevoRepositorioUsuarios(pool)

	idInexistente := nuevoUsuarioDePrueba(t, correoUnico(t, "nunca-persistido")).ID()

	encontrado, err := repo.BuscarPorID(context.Background(), idInexistente)
	if err != nil {
		t.Fatalf("BuscarPorID() devolvió error inesperado para un usuario inexistente: %v", err)
	}
	if encontrado != nil {
		t.Fatalf("BuscarPorID() = %+v, se esperaba nil para un usuario inexistente", encontrado)
	}
}

func TestRepositorioUsuarios_BuscarPorCorreo_CorreoInexistente_DevuelveNilSinError(t *testing.T) {
	pool := poolAplicacion(t)
	repo := identidadpostgres.NuevoRepositorioUsuarios(pool)

	correo, err := dominio.NuevoCorreo(correoUnico(t, "correo-nunca-registrado"))
	if err != nil {
		t.Fatalf("NuevoCorreo(): %v", err)
	}

	encontrado, err := repo.BuscarPorCorreo(context.Background(), correo)
	if err != nil {
		t.Fatalf("BuscarPorCorreo() devolvió error inesperado para un correo inexistente: %v", err)
	}
	if encontrado != nil {
		t.Fatalf("BuscarPorCorreo() = %+v, se esperaba nil para un correo inexistente", encontrado)
	}
}

// TestRepositorioUsuarios_Guardar_CorreoDuplicado_TraduceAErrCorreoYaRegistrado
// verifica INV-ID-02 contra el índice único real de Postgres
// (usuarios_correo_idx, 000001_crear_usuarios.up.sql) y la traducción de
// errores de identidad/adaptadores/postgres/errores.go: la violación de
// unicidad debe llegar a la capa de aplicación como
// *dominio.ErrCorreoYaRegistrado, nunca como un *pgconn.PgError crudo.
func TestRepositorioUsuarios_Guardar_CorreoDuplicado_TraduceAErrCorreoYaRegistrado(t *testing.T) {
	pool := poolAplicacion(t)
	duenoPool := poolDueno(t)
	repo := identidadpostgres.NuevoRepositorioUsuarios(pool)
	ctx := context.Background()

	correoStr := correoUnico(t, "correo-duplicado")
	primero := nuevoUsuarioDePrueba(t, correoStr)
	t.Cleanup(func() { borrarUsuario(t, duenoPool, primero.ID().String()) })

	if err := repo.Guardar(ctx, primero); err != nil {
		t.Fatalf("Guardar() del primer usuario = %v, se esperaba nil", err)
	}

	segundo := nuevoUsuarioDePrueba(t, correoStr) // mismo correo, distinto ID (distinto agregado)
	err := repo.Guardar(ctx, segundo)
	if err == nil {
		t.Fatalf("Guardar() con correo duplicado no devolvió error")
	}

	var errCorreo *dominio.ErrCorreoYaRegistrado
	if !errors.As(err, &errCorreo) {
		t.Fatalf("Guardar() con correo duplicado = %v (%T), se esperaba *dominio.ErrCorreoYaRegistrado", err, err)
	}

	// Defensa en profundidad: el segundo usuario no debe haber quedado
	// persistido bajo ningún ID.
	encontrado, err := repo.BuscarPorID(ctx, segundo.ID())
	if err != nil {
		t.Fatalf("BuscarPorID() del segundo usuario devolvió error inesperado: %v", err)
	}
	if encontrado != nil {
		t.Fatalf("el segundo usuario (correo duplicado) quedó persistido pese al error de unicidad")
	}
}
