package aplicacion_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/r-david1/moterus/internal/acceso/aplicacion"
	"github.com/r-david1/moterus/internal/acceso/dominio"
	"github.com/r-david1/moterus/internal/acceso/puertos"
	"github.com/r-david1/moterus/internal/acceso/puertos/mocks"
)

const (
	emisorDePrueba    = "https://acceso.moterus.test"
	audienciaDePrueba = "moterus"
)

type mocksIniciarSesion struct {
	autenticador    *mocks.AutenticadorIdentidad
	sesiones        *mocks.RepositorioSesiones
	refrescos       *mocks.GeneradorTokensRefresco
	firmador        *mocks.FirmadorTokensAcceso
	listaRevocacion *mocks.ListaRevocacion
	auditoria       *mocks.RegistroAuditoria
	eventos         *mocks.PublicadorEventos
	reloj           *mocks.Reloj
	ids             *mocks.GeneradorIDs
	uow             *mocks.UnidadDeTrabajo
	politica        dominio.PoliticaSesion
	emisorStepUp    *mocks.EmisorTokenStepUp
}

func nuevosMocksIniciarSesion(t *testing.T) *mocksIniciarSesion {
	t.Helper()
	return &mocksIniciarSesion{
		autenticador: &mocks.AutenticadorIdentidad{
			FnAutenticar: func(ctx context.Context, c puertos.CredencialesSujeto) (puertos.SujetoAutenticado, error) {
				return puertos.SujetoAutenticado{IDUsuario: idUsuarioValido1, Estado: "activo"}, nil
			},
		},
		sesiones: &mocks.RepositorioSesiones{},
		refrescos: &mocks.GeneradorTokensRefresco{
			FnGenerar: func() (dominio.TokenRefrescoPlano, error) {
				return tokenRefrescoPlanoDePrueba(t, tokenRefrescoValor1), nil
			},
		},
		firmador:        &mocks.FirmadorTokensAcceso{},
		listaRevocacion: &mocks.ListaRevocacion{},
		auditoria:       &mocks.RegistroAuditoria{},
		eventos:         &mocks.PublicadorEventos{},
		reloj:           &mocks.Reloj{Fija: ahoraDePrueba()},
		ids: &mocks.GeneradorIDs{
			FnNuevoIDSesion: func() (dominio.IDSesion, error) { return idSesionDePrueba(t, idSesionValido1), nil },
		},
		uow:          &mocks.UnidadDeTrabajo{},
		politica:     politicaDePrueba(t),
		emisorStepUp: &mocks.EmisorTokenStepUp{},
	}
}

func (m *mocksIniciarSesion) casoDeUso() *aplicacion.IniciarSesionCasoDeUso {
	return aplicacion.NuevoIniciarSesionCasoDeUso(
		m.autenticador, m.sesiones, m.refrescos, m.firmador, m.listaRevocacion,
		m.auditoria, m.eventos, m.reloj, m.ids, m.uow, m.politica, emisorDePrueba, audienciaDePrueba,
		m.emisorStepUp,
	)
}

func comandoIniciarSesionValido(t *testing.T) aplicacion.ComandoIniciarSesion {
	t.Helper()
	return aplicacion.ComandoIniciarSesion{
		Correo:     "ana@ejemplo.com",
		Contrasena: "correcto caballo batería grapa",
		Origen:     origenDePrueba(t),
	}
}

func TestIniciarSesionCasoDeUso_FlujoFeliz(t *testing.T) {
	m := nuevosMocksIniciarSesion(t)
	caso := m.casoDeUso()

	resultado, err := caso.Iniciar(context.Background(), comandoIniciarSesionValido(t))
	if err != nil {
		t.Fatalf("Iniciar() devolvió error inesperado: %v", err)
	}

	if resultado.IDUsuario != idUsuarioValido1 {
		t.Errorf("IDUsuario = %q, esperado %q", resultado.IDUsuario, idUsuarioValido1)
	}
	if resultado.IDSesion != idSesionValido1 {
		t.Errorf("IDSesion = %q, esperado %q", resultado.IDSesion, idSesionValido1)
	}
	if resultado.TipoToken != "Bearer" {
		t.Errorf("TipoToken = %q, esperado Bearer", resultado.TipoToken)
	}
	if resultado.TokenRefresco != tokenRefrescoValor1 {
		t.Errorf("TokenRefresco = %q, esperado %q", resultado.TokenRefresco, tokenRefrescoValor1)
	}
	if resultado.TokenAcceso == "" {
		t.Error("TokenAcceso no debería estar vacío")
	}
	if resultado.ExpiraEnSegundos != int(m.politica.VidaTokenAcceso().Seconds()) {
		t.Errorf("ExpiraEnSegundos = %d, esperado %d", resultado.ExpiraEnSegundos, int(m.politica.VidaTokenAcceso().Seconds()))
	}

	if len(m.sesiones.LlamadasGuardar) != 1 {
		t.Fatalf("se esperaba una llamada a Guardar, hubo %d", len(m.sesiones.LlamadasGuardar))
	}
	if nombres := m.auditoria.NombresEventos(); len(nombres) != 1 || nombres[0] != "SesionIniciada" {
		t.Errorf("eventos auditados = %v, esperado [SesionIniciada]", nombres)
	}
	if len(m.eventos.LlamadasPublicar) != 1 {
		t.Errorf("se esperaba una llamada a PublicadorEventos.Publicar, hubo %d", len(m.eventos.LlamadasPublicar))
	}
	// El token de acceso se firma DESPUÉS de la llamada a la UoW
	// (INV-ACC-24); no hay forma directa de comprobar el orden temporal con
	// este mock, pero si Firmar nunca se hubiera llamado tampoco habría
	// TokenAcceso.
	if len(m.firmador.LlamadasFirmar) != 1 {
		t.Fatalf("se esperaba una llamada a Firmar, hubo %d", len(m.firmador.LlamadasFirmar))
	}
}

func TestIniciarSesionCasoDeUso_CredencialesRechazadas(t *testing.T) {
	m := nuevosMocksIniciarSesion(t)
	m.autenticador.FnAutenticar = func(ctx context.Context, c puertos.CredencialesSujeto) (puertos.SujetoAutenticado, error) {
		return puertos.SujetoAutenticado{}, &dominio.ErrCredencialesRechazadas{}
	}
	caso := m.casoDeUso()

	_, err := caso.Iniciar(context.Background(), comandoIniciarSesionValido(t))
	var errCredenciales *dominio.ErrCredencialesRechazadas
	if !errors.As(err, &errCredenciales) {
		t.Fatalf("se esperaba *ErrCredencialesRechazadas, obtuvo %T: %v", err, err)
	}
	if len(m.sesiones.LlamadasGuardar) != 0 {
		t.Error("un login rechazado no debe persistir ninguna sesión")
	}
	if len(m.auditoria.LlamadasRegistrar) != 0 {
		t.Error("Acceso no debe auditar un rechazo de credenciales: Identidad ya lo hizo")
	}
}

func TestIniciarSesionCasoDeUso_CuentaNoOperativa(t *testing.T) {
	m := nuevosMocksIniciarSesion(t)
	m.autenticador.FnAutenticar = func(ctx context.Context, c puertos.CredencialesSujeto) (puertos.SujetoAutenticado, error) {
		return puertos.SujetoAutenticado{}, &dominio.ErrCuentaNoOperativa{Motivo: dominio.MotivoCuentaNoOperativaSuspendida}
	}
	caso := m.casoDeUso()

	_, err := caso.Iniciar(context.Background(), comandoIniciarSesionValido(t))
	var errCuenta *dominio.ErrCuentaNoOperativa
	if !errors.As(err, &errCuenta) {
		t.Fatalf("se esperaba *ErrCuentaNoOperativa, obtuvo %T: %v", err, err)
	}
}

func TestIniciarSesionCasoDeUso_AccesoDenegadoPorConfianza(t *testing.T) {
	m := nuevosMocksIniciarSesion(t)
	m.autenticador.FnAutenticar = func(ctx context.Context, c puertos.CredencialesSujeto) (puertos.SujetoAutenticado, error) {
		return puertos.SujetoAutenticado{}, &dominio.ErrAccesoDenegadoPorConfianza{Motivo: "riesgo_alto"}
	}
	caso := m.casoDeUso()

	_, err := caso.Iniciar(context.Background(), comandoIniciarSesionValido(t))
	var errConfianza *dominio.ErrAccesoDenegadoPorConfianza
	if !errors.As(err, &errConfianza) {
		t.Fatalf("se esperaba *ErrAccesoDenegadoPorConfianza, obtuvo %T: %v", err, err)
	}
}

func TestIniciarSesionCasoDeUso_SegundoFactorRequerido(t *testing.T) {
	m := nuevosMocksIniciarSesion(t)
	m.autenticador.FnAutenticar = func(ctx context.Context, c puertos.CredencialesSujeto) (puertos.SujetoAutenticado, error) {
		return puertos.SujetoAutenticado{IDUsuario: idUsuarioValido1, RequiereSegundoFactor: true, MotivoStepUp: "mfa_habilitado"}, nil
	}
	caso := m.casoDeUso()

	_, err := caso.Iniciar(context.Background(), comandoIniciarSesionValido(t))
	var errStepUp *dominio.ErrSegundoFactorRequerido
	if !errors.As(err, &errStepUp) {
		t.Fatalf("se esperaba *ErrSegundoFactorRequerido, obtuvo %T: %v", err, err)
	}
	if errStepUp.MotivoStepUp != "mfa_habilitado" {
		t.Errorf("MotivoStepUp = %q, esperado mfa_habilitado", errStepUp.MotivoStepUp)
	}
	if errStepUp.TokenStepUp == "" {
		t.Error("TokenStepUp no debe estar vacío cuando se requiere segundo factor (ADR 0038)")
	}
	if len(m.emisorStepUp.LlamadasEmitir) != 1 {
		t.Fatalf("se esperaba 1 llamada a EmisorTokenStepUp.Emitir, hubo %d", len(m.emisorStepUp.LlamadasEmitir))
	}
	if m.emisorStepUp.LlamadasEmitir[0].IDUsuario != idUsuarioValido1 {
		t.Errorf("Emitir() IDUsuario = %q, esperado %q", m.emisorStepUp.LlamadasEmitir[0].IDUsuario, idUsuarioValido1)
	}
	if m.emisorStepUp.LlamadasEmitir[0].MotivoStepUp != "mfa_habilitado" {
		t.Errorf("Emitir() MotivoStepUp = %q, esperado mfa_habilitado", m.emisorStepUp.LlamadasEmitir[0].MotivoStepUp)
	}
	// INV-ACC-03: no se emite sesión ni se persiste nada.
	if len(m.sesiones.LlamadasGuardar) != 0 {
		t.Error("no debe persistirse ninguna sesión cuando se requiere segundo factor")
	}
	if len(m.firmador.LlamadasFirmar) != 0 {
		t.Error("no debe firmarse ningún token cuando se requiere segundo factor")
	}
	if len(m.auditoria.LlamadasRegistrar) != 0 {
		t.Error("Acceso no debe auditar el step-up: Identidad ya lo hizo")
	}
}

// TestIniciarSesionCasoDeUso_LimiteSesionesExcedido verifica INV-ACC-22: al
// superar el máximo de sesiones activas, se revoca la más antigua y NUNCA
// se rechaza el login nuevo.
func TestIniciarSesionCasoDeUso_LimiteSesionesExcedido(t *testing.T) {
	m := nuevosMocksIniciarSesion(t)
	usuarioID := idUsuarioDePrueba(t, idUsuarioValido1)

	antigua := sesionActivaDePrueba(t, idSesionDePrueba(t, idSesionValido2), usuarioID, 0,
		tokenRefrescoValor1, ahoraDePrueba().Add(-2*time.Hour), ahoraDePrueba().Add(22*time.Hour), ahoraDePrueba().Add(46*time.Hour))
	reciente := sesionActivaDePrueba(t, idSesionDePrueba(t, idSesionValido3), usuarioID, 0,
		tokenRefrescoValor2, ahoraDePrueba().Add(-1*time.Hour), ahoraDePrueba().Add(23*time.Hour), ahoraDePrueba().Add(47*time.Hour))

	m.sesiones.FnContarActivasDeUsuario = func(ctx context.Context, u dominio.IDUsuario) (int, error) {
		return 3, nil // supera MaximoSesionesActivas=2 de politicaDePrueba
	}
	m.sesiones.FnListarActivasDeUsuario = func(ctx context.Context, u dominio.IDUsuario) ([]*dominio.Sesion, error) {
		return []*dominio.Sesion{reciente, antigua}, nil
	}

	caso := m.casoDeUso()
	_, err := caso.Iniciar(context.Background(), comandoIniciarSesionValido(t))
	if err != nil {
		t.Fatalf("Iniciar() devolvió error inesperado: %v", err)
	}

	if len(m.sesiones.LlamadasGuardar) != 2 {
		t.Fatalf("se esperaban 2 llamadas a Guardar (la nueva sesión + la revocada), hubo %d", len(m.sesiones.LlamadasGuardar))
	}
	revocada := m.sesiones.LlamadasGuardar[1]
	if revocada.ID().String() != idSesionValido2 {
		t.Errorf("se revocó la sesión %q, esperada la más antigua (%q)", revocada.ID().String(), idSesionValido2)
	}
	if !revocada.Estado().EsIgual(dominio.EstadoSesionRevocada) {
		t.Error("la sesión más antigua debía quedar revocada")
	}
	if motivo, ok := revocada.MotivoRevocacion(); !ok || !motivo.EsIgual(dominio.MotivoLimiteSesionesExcedido) {
		t.Errorf("MotivoRevocacion = %v, esperado limite_sesiones_excedido", motivo)
	}

	nombres := m.auditoria.NombresEventos()
	if len(nombres) != 2 || nombres[0] != "SesionIniciada" || nombres[1] != "SesionRevocada" {
		t.Errorf("eventos auditados = %v, esperado [SesionIniciada SesionRevocada]", nombres)
	}
	if len(m.listaRevocacion.LlamadasRevocarSesion) != 1 || m.listaRevocacion.LlamadasRevocarSesion[0].IDSesion.String() != idSesionValido2 {
		t.Error("se esperaba propagar la revocación de la sesión más antigua a la lista de revocación")
	}
}
