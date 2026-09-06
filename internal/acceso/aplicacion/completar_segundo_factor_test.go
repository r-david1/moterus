package aplicacion_test

import (
	"context"
	"errors"
	"testing"

	"github.com/r-david1/moterus/internal/acceso/aplicacion"
	"github.com/r-david1/moterus/internal/acceso/dominio"
	"github.com/r-david1/moterus/internal/acceso/puertos"
	"github.com/r-david1/moterus/internal/acceso/puertos/mocks"
)

// verificadorSegundoFactorDePrueba es un test double manual de
// puertos.VerificadorSegundoFactor (movida a acceso/puertos/salida.go,
// junto a AutenticadorIdentidad/ConsultorEstadoSujeto).
type verificadorSegundoFactorDePrueba struct {
	fnVerificar func(ctx context.Context, idUsuario, codigo string, origen dominio.OrigenSolicitud) (bool, error)

	llamadas []string // códigos presentados, en orden
}

func (v *verificadorSegundoFactorDePrueba) Verificar(ctx context.Context, idUsuario, codigo string, origen dominio.OrigenSolicitud) (bool, error) {
	v.llamadas = append(v.llamadas, codigo)
	if v.fnVerificar != nil {
		return v.fnVerificar(ctx, idUsuario, codigo, origen)
	}
	return true, nil
}

type mocksCompletarSegundoFactor struct {
	emisorStepUp  *mocks.EmisorTokenStepUp
	confianza     *mocks.EvaluadorConfianza
	verificador   *verificadorSegundoFactorDePrueba
	iniciarSesion *mocksIniciarSesion
}

func nuevosMocksCompletarSegundoFactor(t *testing.T) *mocksCompletarSegundoFactor {
	t.Helper()
	return &mocksCompletarSegundoFactor{
		emisorStepUp: &mocks.EmisorTokenStepUp{
			FnValidar: func(ctx context.Context, tokenCompacto string) (puertos.ClaimsStepUp, error) {
				return puertos.ClaimsStepUp{IDUsuario: idUsuarioValido1, MotivoStepUp: "mfa_habilitado"}, nil
			},
		},
		confianza:     &mocks.EvaluadorConfianza{},
		verificador:   &verificadorSegundoFactorDePrueba{},
		iniciarSesion: nuevosMocksIniciarSesion(t),
	}
}

func (m *mocksCompletarSegundoFactor) casoDeUso() *aplicacion.CompletarSegundoFactorCasoDeUso {
	return aplicacion.NuevoCompletarSegundoFactorCasoDeUso(
		m.emisorStepUp, m.confianza, m.verificador, m.iniciarSesion.casoDeUso(),
	)
}

func comandoCompletarSegundoFactorValido(t *testing.T) aplicacion.ComandoCompletarSegundoFactor {
	t.Helper()
	return aplicacion.ComandoCompletarSegundoFactor{
		TokenStepUp: "token-step-up-de-prueba",
		Codigo:      "123456",
		Origen:      origenDePrueba(t),
	}
}

func TestCompletarSegundoFactorCasoDeUso_FlujoFeliz(t *testing.T) {
	m := nuevosMocksCompletarSegundoFactor(t)
	caso := m.casoDeUso()

	resultado, err := caso.CompletarSegundoFactor(context.Background(), comandoCompletarSegundoFactorValido(t))
	if err != nil {
		t.Fatalf("CompletarSegundoFactor() devolvió error inesperado: %v", err)
	}
	if resultado.IDUsuario != idUsuarioValido1 {
		t.Errorf("IDUsuario = %q, esperado %q", resultado.IDUsuario, idUsuarioValido1)
	}
	if resultado.TokenAcceso == "" {
		t.Error("TokenAcceso no debe estar vacío")
	}

	if len(m.iniciarSesion.firmador.LlamadasFirmar) != 1 {
		t.Fatalf("se esperaba 1 llamada a Firmar, hubo %d", len(m.iniciarSesion.firmador.LlamadasFirmar))
	}
	amr := m.iniciarSesion.firmador.LlamadasFirmar[0].MetodosAutenticacion()
	if len(amr) != 2 || amr[0] != "pwd" || amr[1] != "otp" {
		t.Errorf("amr = %v, esperado [pwd otp]", amr)
	}

	if len(m.iniciarSesion.sesiones.LlamadasGuardar) != 1 {
		t.Errorf("se esperaba 1 sesión persistida, hubo %d", len(m.iniciarSesion.sesiones.LlamadasGuardar))
	}
}

func TestCompletarSegundoFactorCasoDeUso_TokenStepUpInvalido(t *testing.T) {
	m := nuevosMocksCompletarSegundoFactor(t)
	m.emisorStepUp.FnValidar = func(ctx context.Context, tokenCompacto string) (puertos.ClaimsStepUp, error) {
		return puertos.ClaimsStepUp{}, errors.New("token de step-up expirado")
	}
	caso := m.casoDeUso()

	_, err := caso.CompletarSegundoFactor(context.Background(), comandoCompletarSegundoFactorValido(t))
	var rechazadas *dominio.ErrCredencialesRechazadas
	if !errors.As(err, &rechazadas) {
		t.Fatalf("se esperaba *ErrCredencialesRechazadas, obtuvo %T: %v", err, err)
	}
	if len(m.verificador.llamadas) != 0 {
		t.Error("no debe llegar a verificar el código si el token de step-up es inválido")
	}
	if len(m.iniciarSesion.sesiones.LlamadasGuardar) != 0 {
		t.Error("no debe emitirse ninguna sesión")
	}
}

func TestCompletarSegundoFactorCasoDeUso_CodigoIncorrecto(t *testing.T) {
	m := nuevosMocksCompletarSegundoFactor(t)
	m.verificador.fnVerificar = func(ctx context.Context, idUsuario, codigo string, origen dominio.OrigenSolicitud) (bool, error) {
		return false, nil
	}
	caso := m.casoDeUso()

	_, err := caso.CompletarSegundoFactor(context.Background(), comandoCompletarSegundoFactorValido(t))
	var rechazadas *dominio.ErrCredencialesRechazadas
	if !errors.As(err, &rechazadas) {
		t.Fatalf("se esperaba *ErrCredencialesRechazadas, obtuvo %T: %v", err, err)
	}
	if len(m.iniciarSesion.sesiones.LlamadasGuardar) != 0 {
		t.Error("no debe emitirse ninguna sesión con un código incorrecto")
	}
}

func TestCompletarSegundoFactorCasoDeUso_ConfianzaDeniega(t *testing.T) {
	m := nuevosMocksCompletarSegundoFactor(t)
	m.confianza.FnEvaluar = func(ctx context.Context, s puertos.SolicitudEvaluacion) (puertos.DecisionConfianza, error) {
		return puertos.DecisionConfianza{Permitido: false, Motivo: "demasiados_intentos"}, nil
	}
	caso := m.casoDeUso()

	_, err := caso.CompletarSegundoFactor(context.Background(), comandoCompletarSegundoFactorValido(t))
	var denegado *dominio.ErrAccesoDenegadoPorConfianza
	if !errors.As(err, &denegado) {
		t.Fatalf("se esperaba *ErrAccesoDenegadoPorConfianza, obtuvo %T: %v", err, err)
	}
	if len(m.verificador.llamadas) != 0 {
		t.Error("no debe llegar a verificar el código si Confianza deniega")
	}
}

func TestCompletarSegundoFactorCasoDeUso_ConfianzaEvaluaConAccionYClaveCorrectas(t *testing.T) {
	m := nuevosMocksCompletarSegundoFactor(t)
	caso := m.casoDeUso()

	if _, err := caso.CompletarSegundoFactor(context.Background(), comandoCompletarSegundoFactorValido(t)); err != nil {
		t.Fatalf("CompletarSegundoFactor() devolvió error inesperado: %v", err)
	}
	if len(m.confianza.LlamadasEvaluar) != 1 {
		t.Fatalf("se esperaba 1 llamada a Confianza.Evaluar, hubo %d", len(m.confianza.LlamadasEvaluar))
	}
	solicitud := m.confianza.LlamadasEvaluar[0]
	if solicitud.Accion != "verificar_otp" {
		t.Errorf("Accion = %q, esperado verificar_otp", solicitud.Accion)
	}
	if solicitud.ClaveCuenta != "usuario:"+idUsuarioValido1 {
		t.Errorf("ClaveCuenta = %q, esperado usuario:%s", solicitud.ClaveCuenta, idUsuarioValido1)
	}
}
