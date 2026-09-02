package aplicacion_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/r-david1/moterus/internal/acceso/aplicacion"
	"github.com/r-david1/moterus/internal/acceso/dominio"
	"github.com/r-david1/moterus/internal/acceso/puertos/mocks"
)

type mocksValidarAcceso struct {
	firmador        *mocks.FirmadorTokensAcceso
	listaRevocacion *mocks.ListaRevocacion
	sesiones        *mocks.RepositorioSesiones
	auditoria       *mocks.RegistroAuditoria
	reloj           *mocks.Reloj
}

func reclamacionesDePrueba(t *testing.T) dominio.ReclamacionesAcceso {
	t.Helper()
	sujeto := idUsuarioDePrueba(t, idUsuarioValido1)
	sesion := idSesionDePrueba(t, idSesionValido1)
	jti, err := dominio.IDTokenAccesoDesde("123e4567-e89b-42d3-a456-426614174099")
	if err != nil {
		t.Fatalf("no se pudo construir el jti de prueba: %v", err)
	}
	r, err := dominio.NuevasReclamacionesAcceso(
		emisorDePrueba, sujeto, audienciaDePrueba, sesion, jti,
		[]string{"pwd"}, ahoraDePrueba(), ahoraDePrueba(), ahoraDePrueba().Add(10*time.Minute), 1,
	)
	if err != nil {
		t.Fatalf("no se pudo construir las reclamaciones de prueba: %v", err)
	}
	return r
}

func nuevosMocksValidarAcceso(t *testing.T) *mocksValidarAcceso {
	t.Helper()
	reclamaciones := reclamacionesDePrueba(t)
	return &mocksValidarAcceso{
		firmador: &mocks.FirmadorTokensAcceso{
			FnVerificar: func(ctx context.Context, tokenCompacto string) (dominio.ReclamacionesAcceso, error) {
				return reclamaciones, nil
			},
		},
		listaRevocacion: &mocks.ListaRevocacion{},
		sesiones:        &mocks.RepositorioSesiones{},
		auditoria:       &mocks.RegistroAuditoria{},
		reloj:           &mocks.Reloj{Fija: ahoraDePrueba()},
	}
}

func (m *mocksValidarAcceso) casoDeUso() *aplicacion.ValidarAccesoCasoDeUso {
	return aplicacion.NuevoValidarAccesoCasoDeUso(m.firmador, m.listaRevocacion, m.sesiones, m.auditoria, m.reloj)
}

func TestValidarAccesoCasoDeUso_FlujoFeliz(t *testing.T) {
	m := nuevosMocksValidarAcceso(t)
	caso := m.casoDeUso()

	acceso, err := caso.Validar(context.Background(), aplicacion.ComandoValidarAcceso{TokenCompacto: "un-jwt", Origen: origenDePrueba(t)})
	if err != nil {
		t.Fatalf("Validar() devolvió error inesperado: %v", err)
	}
	if acceso.IDUsuario != idUsuarioValido1 {
		t.Errorf("IDUsuario = %q, esperado %q", acceso.IDUsuario, idUsuarioValido1)
	}
	if acceso.IDSesion != idSesionValido1 {
		t.Errorf("IDSesion = %q, esperado %q", acceso.IDSesion, idSesionValido1)
	}
	if len(acceso.MetodosAutenticacion) != 1 || acceso.MetodosAutenticacion[0] != "pwd" {
		t.Errorf("MetodosAutenticacion = %v, esperado [pwd]", acceso.MetodosAutenticacion)
	}
	if len(m.sesiones.LlamadasBuscarPorID) != 0 {
		t.Error("el camino feliz sin ExigirSesionViva no debe consultar Postgres")
	}
	if len(m.auditoria.LlamadasRegistrar) != 0 {
		t.Error("una validación exitosa no se audita")
	}
}

func TestValidarAccesoCasoDeUso_TokenExpirado_NoAudita(t *testing.T) {
	m := nuevosMocksValidarAcceso(t)
	m.firmador.FnVerificar = func(ctx context.Context, tokenCompacto string) (dominio.ReclamacionesAcceso, error) {
		return dominio.ReclamacionesAcceso{}, &dominio.ErrTokenAccesoExpirado{}
	}
	caso := m.casoDeUso()

	_, err := caso.Validar(context.Background(), aplicacion.ComandoValidarAcceso{TokenCompacto: "un-jwt", Origen: origenDePrueba(t)})
	var errExpirado *dominio.ErrTokenAccesoExpirado
	if !errors.As(err, &errExpirado) {
		t.Fatalf("se esperaba *ErrTokenAccesoExpirado, obtuvo %T: %v", err, err)
	}
	// INV-ACC-17: el rechazo más frecuente del sistema no se audita.
	if len(m.auditoria.LlamadasRegistrar) != 0 {
		t.Error("un token expirado nunca debe auditarse")
	}
}

func TestValidarAccesoCasoDeUso_TokenInvalido_Audita(t *testing.T) {
	m := nuevosMocksValidarAcceso(t)
	m.firmador.FnVerificar = func(ctx context.Context, tokenCompacto string) (dominio.ReclamacionesAcceso, error) {
		return dominio.ReclamacionesAcceso{}, &dominio.ErrTokenAccesoInvalido{Motivo: dominio.MotivoTokenAccesoFirmaInvalida}
	}
	caso := m.casoDeUso()

	_, err := caso.Validar(context.Background(), aplicacion.ComandoValidarAcceso{TokenCompacto: "un-jwt", Origen: origenDePrueba(t)})
	var errInvalido *dominio.ErrTokenAccesoInvalido
	if !errors.As(err, &errInvalido) {
		t.Fatalf("se esperaba *ErrTokenAccesoInvalido, obtuvo %T: %v", err, err)
	}
	if nombres := m.auditoria.NombresEventos(); len(nombres) != 1 || nombres[0] != "TokenAccesoRechazado" {
		t.Errorf("eventos auditados = %v, esperado [TokenAccesoRechazado]", nombres)
	}
}

func TestValidarAccesoCasoDeUso_SesionRevocadaEnLista(t *testing.T) {
	m := nuevosMocksValidarAcceso(t)
	m.listaRevocacion.FnSesionRevocada = func(ctx context.Context, idSesion dominio.IDSesion) (bool, error) {
		return true, nil
	}
	caso := m.casoDeUso()

	_, err := caso.Validar(context.Background(), aplicacion.ComandoValidarAcceso{TokenCompacto: "un-jwt", Origen: origenDePrueba(t)})
	var errRevocada *dominio.ErrSesionRevocadaEnLista
	if !errors.As(err, &errRevocada) {
		t.Fatalf("se esperaba *ErrSesionRevocadaEnLista, obtuvo %T: %v", err, err)
	}
	if nombres := m.auditoria.NombresEventos(); len(nombres) != 1 || nombres[0] != "TokenAccesoRechazado" {
		t.Errorf("eventos auditados = %v, esperado [TokenAccesoRechazado]", nombres)
	}
}

func TestValidarAccesoCasoDeUso_ListaRevocacionNoDisponible_FailOpen(t *testing.T) {
	m := nuevosMocksValidarAcceso(t)
	m.listaRevocacion.FnDisponible = func() bool { return false }
	caso := m.casoDeUso()

	_, err := caso.Validar(context.Background(), aplicacion.ComandoValidarAcceso{TokenCompacto: "un-jwt", Origen: origenDePrueba(t)})
	if err != nil {
		t.Fatalf("Validar() devolvió error inesperado: %v", err)
	}
	if len(m.listaRevocacion.LlamadasSesionRevocada) != 0 {
		t.Error("si Disponible()==false no debe consultarse la lista de revocación")
	}
}

func TestValidarAccesoCasoDeUso_ExigirSesionViva_SesionExpirada(t *testing.T) {
	m := nuevosMocksValidarAcceso(t)
	usuarioID := idUsuarioDePrueba(t, idUsuarioValido1)
	sesion := sesionActivaDePrueba(t, idSesionDePrueba(t, idSesionValido1), usuarioID, 0,
		tokenRefrescoValor1, ahoraDePrueba().Add(-25*time.Hour), ahoraDePrueba().Add(-1*time.Hour), ahoraDePrueba().Add(47*time.Hour))
	m.sesiones.FnBuscarPorID = func(ctx context.Context, id dominio.IDSesion) (*dominio.Sesion, error) {
		return sesion, nil
	}
	caso := m.casoDeUso()

	_, err := caso.Validar(context.Background(), aplicacion.ComandoValidarAcceso{
		TokenCompacto: "un-jwt", ExigirSesionViva: true, Origen: origenDePrueba(t),
	})
	var errExpirada *dominio.ErrSesionExpirada
	if !errors.As(err, &errExpirada) {
		t.Fatalf("se esperaba *ErrSesionExpirada, obtuvo %T: %v", err, err)
	}
	if len(m.sesiones.LlamadasBuscarPorID) != 1 {
		t.Error("ExigirSesionViva debe consultar el repositorio de sesiones exactamente una vez")
	}
}

func TestValidarAccesoCasoDeUso_ExigirSesionViva_SesionViva(t *testing.T) {
	m := nuevosMocksValidarAcceso(t)
	usuarioID := idUsuarioDePrueba(t, idUsuarioValido1)
	sesion := sesionActivaDePrueba(t, idSesionDePrueba(t, idSesionValido1), usuarioID, 0,
		tokenRefrescoValor1, ahoraDePrueba().Add(-1*time.Hour), ahoraDePrueba().Add(23*time.Hour), ahoraDePrueba().Add(47*time.Hour))
	m.sesiones.FnBuscarPorID = func(ctx context.Context, id dominio.IDSesion) (*dominio.Sesion, error) {
		return sesion, nil
	}
	caso := m.casoDeUso()

	acceso, err := caso.Validar(context.Background(), aplicacion.ComandoValidarAcceso{
		TokenCompacto: "un-jwt", ExigirSesionViva: true, Origen: origenDePrueba(t),
	})
	if err != nil {
		t.Fatalf("Validar() devolvió error inesperado: %v", err)
	}
	if acceso.IDSesion != idSesionValido1 {
		t.Errorf("IDSesion = %q, esperado %q", acceso.IDSesion, idSesionValido1)
	}
}
