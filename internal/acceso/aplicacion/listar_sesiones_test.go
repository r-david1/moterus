package aplicacion_test

import (
	"context"
	"testing"
	"time"

	"github.com/r-david1/moterus/internal/acceso/aplicacion"
	"github.com/r-david1/moterus/internal/acceso/dominio"
	"github.com/r-david1/moterus/internal/acceso/puertos/mocks"
)

func TestListarSesionesCasoDeUso_FlujoFeliz(t *testing.T) {
	usuarioID := idUsuarioDePrueba(t, idUsuarioValido1)
	actual := sesionActivaDePrueba(t, idSesionDePrueba(t, idSesionValido1), usuarioID, 0,
		tokenRefrescoValor1, ahoraDePrueba().Add(-1*time.Hour), ahoraDePrueba().Add(23*time.Hour), ahoraDePrueba().Add(47*time.Hour))
	otra := sesionActivaDePrueba(t, idSesionDePrueba(t, idSesionValido2), usuarioID, 0,
		tokenRefrescoValor2, ahoraDePrueba().Add(-2*time.Hour), ahoraDePrueba().Add(22*time.Hour), ahoraDePrueba().Add(46*time.Hour))

	sesiones := &mocks.RepositorioSesiones{
		FnListarActivasDeUsuario: func(ctx context.Context, u dominio.IDUsuario) ([]*dominio.Sesion, error) {
			return []*dominio.Sesion{actual, otra}, nil
		},
	}
	caso := aplicacion.NuevoListarSesionesCasoDeUso(sesiones)

	vistas, err := caso.ListarDeUsuario(context.Background(), aplicacion.ConsultaSesionesDeUsuario{
		IDUsuario: idUsuarioValido1, IDSesionActual: idSesionValido1, Origen: origenDePrueba(t),
	})
	if err != nil {
		t.Fatalf("ListarDeUsuario() devolvió error inesperado: %v", err)
	}
	if len(vistas) != 2 {
		t.Fatalf("se esperaban 2 vistas, hubo %d", len(vistas))
	}
	var marcoActual bool
	for _, v := range vistas {
		if v.ID == idSesionValido1 {
			if !v.EsSesionActual {
				t.Error("la sesión con IDSesionActual debía marcarse EsSesionActual=true")
			}
			marcoActual = true
		} else if v.EsSesionActual {
			t.Error("solo la sesión actual debe marcarse EsSesionActual=true")
		}
	}
	if !marcoActual {
		t.Error("no se encontró la sesión actual entre las vistas devueltas")
	}
}

func TestListarSesionesCasoDeUso_Vacio(t *testing.T) {
	sesiones := &mocks.RepositorioSesiones{
		FnListarActivasDeUsuario: func(ctx context.Context, u dominio.IDUsuario) ([]*dominio.Sesion, error) {
			return nil, nil
		},
	}
	caso := aplicacion.NuevoListarSesionesCasoDeUso(sesiones)

	vistas, err := caso.ListarDeUsuario(context.Background(), aplicacion.ConsultaSesionesDeUsuario{
		IDUsuario: idUsuarioValido1, Origen: origenDePrueba(t),
	})
	if err != nil {
		t.Fatalf("ListarDeUsuario() devolvió error inesperado: %v", err)
	}
	if len(vistas) != 0 {
		t.Errorf("se esperaba una lista vacía, hubo %d elementos", len(vistas))
	}
}
