package aplicacion_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/r-david1/moterus/internal/confianza/aplicacion"
	"github.com/r-david1/moterus/internal/confianza/dominio"
	"github.com/r-david1/moterus/internal/confianza/puertos"
	"github.com/r-david1/moterus/internal/confianza/puertos/mocks"
)

const minutoDePrueba = time.Minute

func ritmoDePrueba(t *testing.T, porSegundo int) dominio.RitmoAdmision {
	t.Helper()
	r, err := dominio.NuevoRitmoAdmision(porSegundo)
	if err != nil {
		t.Fatalf("ritmo de prueba inválido: %v", err)
	}
	return r
}

func TestReconciliarSalasCasoDeUso_ActualizaLaInstantaneaYReproyecta(t *testing.T) {
	ahora := ahoraDePrueba()
	sala := salaAbiertaDePrueba(t, idSalaValido1, 50, ahora)
	repo := &mocks.RepositorioSalasDeEspera{
		FnListarVigentes: func(ctx context.Context) ([]*dominio.SalaDeEspera, error) {
			return []*dominio.SalaDeEspera{sala}, nil
		},
	}
	estadoCola := &mocks.EstadoDeCola{
		FnInstantanea: func(ctx context.Context, clave string) (puertos.InstantaneaCola, error) {
			return puertos.InstantaneaCola{LongitudAproximada: 5, Cursor: 3, Ingresos: 8}, nil
		},
	}
	instantanea := aplicacion.NuevaInstantaneaSalasVigentes()
	caso := aplicacion.NuevoReconciliarSalasCasoDeUso(repo, estadoCola, &mocks.Reloj{Fija: ahora}, instantanea)

	if err := caso.Reconciliar(context.Background()); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	vista, ok := instantanea.LeerPorClave(sala.Clave().String())
	if !ok {
		t.Fatal("la instantánea debía quedar poblada con la sala vigente")
	}
	if vista.Alias != sala.Alias().Normalizado() {
		t.Errorf("Alias en la instantánea = %q, esperado %q", vista.Alias, sala.Alias().Normalizado())
	}

	if len(estadoCola.LlamadasProyectar) != 1 {
		t.Fatalf("se esperaba 1 llamada a Proyectar, hubo %d", len(estadoCola.LlamadasProyectar))
	}
	proy := estadoCola.LlamadasProyectar[0]
	if proy.CursorBase != sala.CursorBase() {
		t.Errorf("CursorBase reproyectado = %d, esperado el ancla real de la sala (%d): Redis respondió con éxito, no corresponde reiniciar el reloj", proy.CursorBase, sala.CursorBase())
	}
	if !proy.RelojDesde.Equal(sala.RelojDesde()) {
		t.Errorf("RelojDesde reproyectado = %v, esperado %v", proy.RelojDesde, sala.RelojDesde())
	}
}

// TestReconciliarSalasCasoDeUso_ReiniciaElRelojTrasEncontrarRedisVacio
// verifica INV-COLA-13: si EstadoDeCola.Instantanea no encuentra el estado
// de una sala vigente (Redis lo perdió, p. ej. tras un FLUSHALL a mitad de
// un evento), la reproyección de ESE ciclo usa cursorBase=0 y
// relojDesde=ahora en vez del ancla real (potencialmente grande) que sigue
// en Postgres — para no admitir de golpe a toda una cola vacía.
func TestReconciliarSalasCasoDeUso_ReiniciaElRelojTrasEncontrarRedisVacio(t *testing.T) {
	ahora := ahoraDePrueba()
	// Una sala que llegó a un cursorBase bien avanzado en Postgres (varios
	// cambios de ritmo antes de que Redis perdiera el estado).
	sala := salaAbiertaDePrueba(t, idSalaValido1, 50, ahora.Add(-1*minutoDePrueba))
	if err := sala.CambiarRitmo(ritmoDePrueba(t, 80), 900, ahora); err != nil {
		t.Fatalf("no se pudo cambiar el ritmo de la sala de prueba: %v", err)
	}
	sala.EventosPendientes()
	if sala.CursorBase() == 0 {
		t.Fatal("la sala de prueba debía tener un cursorBase distinto de cero antes del ciclo bajo prueba")
	}

	repo := &mocks.RepositorioSalasDeEspera{
		FnListarVigentes: func(ctx context.Context) ([]*dominio.SalaDeEspera, error) {
			return []*dominio.SalaDeEspera{sala}, nil
		},
	}
	falloInstantanea := errors.New("redis: la clave de configuración no existe")
	estadoCola := &mocks.EstadoDeCola{
		FnInstantanea: func(ctx context.Context, clave string) (puertos.InstantaneaCola, error) {
			return puertos.InstantaneaCola{}, falloInstantanea
		},
	}
	instantanea := aplicacion.NuevaInstantaneaSalasVigentes()
	ahoraDelCiclo := ahora.Add(30 * time.Second)
	caso := aplicacion.NuevoReconciliarSalasCasoDeUso(repo, estadoCola, &mocks.Reloj{Fija: ahoraDelCiclo}, instantanea)

	if err := caso.Reconciliar(context.Background()); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	if len(estadoCola.LlamadasProyectar) != 1 {
		t.Fatalf("se esperaba 1 llamada a Proyectar, hubo %d", len(estadoCola.LlamadasProyectar))
	}
	proy := estadoCola.LlamadasProyectar[0]
	if proy.CursorBase != 0 {
		t.Errorf("CursorBase reproyectado = %d, esperado 0 (reinicio del reloj, INV-COLA-13)", proy.CursorBase)
	}
	if !proy.RelojDesde.Equal(ahoraDelCiclo) {
		t.Errorf("RelojDesde reproyectado = %v, esperado %v (ahora del ciclo)", proy.RelojDesde, ahoraDelCiclo)
	}
}

func TestReconciliarSalasCasoDeUso_UnaSalaQueFallaAlProyectarNoDetieneElCiclo(t *testing.T) {
	ahora := ahoraDePrueba()
	sala1 := salaAbiertaDePrueba(t, idSalaValido1, 50, ahora)
	// sala2 protege una ruta distinta a propósito: dos salas con la misma
	// (alcance, ruta) violarían INV-COLA-01 y, más relevante para este
	// test, colisionarían en la misma ClaveSala, haciendo indistinguibles
	// sus proyecciones.
	sala2, err := dominio.NuevaSalaDeEspera(
		idSalaDePrueba(t, idSalaValido2),
		aliasSalaDePrueba(t, "evento-"+idSalaValido2),
		dominio.AlcanceSistema(),
		dominio.RutaIdentidadRegistrarUsuario,
		politicaSalaDePrueba(t, 80),
		nil,
		ahora,
	)
	if err != nil {
		t.Fatalf("no se pudo construir sala2: %v", err)
	}
	if err := sala2.Abrir(ahora); err != nil {
		t.Fatalf("no se pudo abrir sala2: %v", err)
	}
	sala2.EventosPendientes()
	repo := &mocks.RepositorioSalasDeEspera{
		FnListarVigentes: func(ctx context.Context) ([]*dominio.SalaDeEspera, error) {
			return []*dominio.SalaDeEspera{sala1, sala2}, nil
		},
	}
	falloProyeccion := errors.New("redis no disponible")
	estadoCola := &mocks.EstadoDeCola{
		FnProyectar: func(ctx context.Context, p puertos.ProyeccionSala) error {
			if p.Clave == sala1.Clave().String() {
				return falloProyeccion
			}
			return nil
		},
	}
	instantanea := aplicacion.NuevaInstantaneaSalasVigentes()
	caso := aplicacion.NuevoReconciliarSalasCasoDeUso(repo, estadoCola, &mocks.Reloj{Fija: ahora}, instantanea)

	err = caso.Reconciliar(context.Background())
	if !errors.Is(err, falloProyeccion) {
		t.Fatalf("se esperaba el error de proyección de sala1, obtuvo %v", err)
	}
	if len(estadoCola.LlamadasProyectar) != 2 {
		t.Fatalf("el fallo de una sala no debía impedir que se intentara reproyectar la otra, hubo %d llamadas", len(estadoCola.LlamadasProyectar))
	}
	// La instantánea igual queda actualizada con AMBAS salas: se reemplaza
	// antes de reproyectar, así que un fallo de Redis no la deja a medias.
	if _, ok := instantanea.LeerPorClave(sala1.Clave().String()); !ok {
		t.Error("la instantánea debía incluir sala1 pese al fallo de Proyectar")
	}
	if _, ok := instantanea.LeerPorClave(sala2.Clave().String()); !ok {
		t.Error("la instantánea debía incluir sala2")
	}
}

func TestReconciliarSalasCasoDeUso_ListarVigentesFallido(t *testing.T) {
	falloListar := errors.New("postgres no disponible")
	repo := &mocks.RepositorioSalasDeEspera{
		FnListarVigentes: func(ctx context.Context) ([]*dominio.SalaDeEspera, error) {
			return nil, falloListar
		},
	}
	estadoCola := &mocks.EstadoDeCola{}
	instantanea := aplicacion.NuevaInstantaneaSalasVigentes()
	caso := aplicacion.NuevoReconciliarSalasCasoDeUso(repo, estadoCola, &mocks.Reloj{Fija: ahoraDePrueba()}, instantanea)

	err := caso.Reconciliar(context.Background())
	if !errors.Is(err, falloListar) {
		t.Fatalf("se esperaba el error de ListarVigentes, obtuvo %v", err)
	}
	if len(estadoCola.LlamadasProyectar) != 0 {
		t.Error("no debía intentarse ninguna proyección si ListarVigentes falló")
	}
}
