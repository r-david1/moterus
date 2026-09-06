package aplicacion_test

import (
	"context"
	"testing"

	"github.com/r-david1/moterus/internal/identidad/aplicacion"
	"github.com/r-david1/moterus/internal/identidad/dominio"
	"github.com/r-david1/moterus/internal/identidad/puertos"
	"github.com/r-david1/moterus/internal/identidad/puertos/mocks"
)

type mocksVerificarOTP struct {
	factoresMFA *mocks.RepositorioFactoresMFA
	cifrador    *mocks.CifradorSecretos
	auditoria   *mocks.RegistroAuditoria
	reloj       *mocks.Reloj
	uow         *mocks.UnidadDeTrabajo
}

func nuevosMocksVerificarOTP(t *testing.T, factoresConfirmados []*dominio.FactorMFA) *mocksVerificarOTP {
	t.Helper()
	return &mocksVerificarOTP{
		factoresMFA: &mocks.RepositorioFactoresMFA{
			FnBuscarConfirmadosDeUsuario: func(ctx context.Context, u dominio.IDUsuario) ([]*dominio.FactorMFA, error) {
				return factoresConfirmados, nil
			},
		},
		cifrador:  &mocks.CifradorSecretos{},
		auditoria: &mocks.RegistroAuditoria{},
		reloj:     &mocks.Reloj{Fija: ahoraDePrueba()},
		uow:       &mocks.UnidadDeTrabajo{},
	}
}

func (m *mocksVerificarOTP) casoDeUso() *aplicacion.VerificarOTPCasoDeUso {
	return aplicacion.NuevoVerificarOTPCasoDeUso(m.factoresMFA, m.cifrador, m.auditoria, m.reloj, m.uow)
}

func TestVerificarOTPCasoDeUso_FlujoFelizConTOTP(t *testing.T) {
	idUsuario := idDePrueba(t, idUsuarioValido1)
	idFactor := idFactorDePrueba(t, idFactorValido1)
	factor := factorConfirmadoDePrueba(t, idFactor, idUsuario, nil)

	m := nuevosMocksVerificarOTP(t, []*dominio.FactorMFA{factor})
	caso := m.casoDeUso()

	ok, err := caso.Verificar(context.Background(), puertos.ConsultaVerificarOTP{
		IDUsuario: idUsuarioValido1,
		Codigo:    codigoTOTPValidoDePrueba(t, ahoraDePrueba()),
		Origen:    origenDePrueba(t),
	})
	if err != nil {
		t.Fatalf("Verificar() devolvió error inesperado: %v", err)
	}
	if !ok {
		t.Error("se esperaba true con un código TOTP válido")
	}
	if len(m.auditoria.LlamadasRegistrar) != 0 {
		t.Errorf("el éxito no debe auditarse: %v", m.auditoria.NombresEventos())
	}
	if len(m.factoresMFA.LlamadasGuardar) != 1 {
		t.Errorf("se esperaba 1 llamada a Guardar tras el éxito, hubo %d", len(m.factoresMFA.LlamadasGuardar))
	}
}

func TestVerificarOTPCasoDeUso_FlujoFelizConCodigoDeRespaldo_AuditaConsumo(t *testing.T) {
	idUsuario := idDePrueba(t, idUsuarioValido1)
	idFactor := idFactorDePrueba(t, idFactorValido1)
	factor := factorConfirmadoDePrueba(t, idFactor, idUsuario, []dominio.CodigoRespaldoMFA{codigoRespaldoDisponibleDePrueba(t)})

	m := nuevosMocksVerificarOTP(t, []*dominio.FactorMFA{factor})
	caso := m.casoDeUso()

	ok, err := caso.Verificar(context.Background(), puertos.ConsultaVerificarOTP{
		IDUsuario: idUsuarioValido1,
		Codigo:    codigoRespaldoPlanoDePrueba,
		Origen:    origenDePrueba(t),
	})
	if err != nil {
		t.Fatalf("Verificar() devolvió error inesperado: %v", err)
	}
	if !ok {
		t.Error("se esperaba true con un código de respaldo disponible")
	}
	nombres := m.auditoria.NombresEventos()
	if len(nombres) != 1 || nombres[0] != "CodigoRespaldoConsumido" {
		t.Errorf("eventos auditados = %v, esperado [CodigoRespaldoConsumido]", nombres)
	}
}

func TestVerificarOTPCasoDeUso_CodigoIncorrecto_Audita(t *testing.T) {
	idUsuario := idDePrueba(t, idUsuarioValido1)
	idFactor := idFactorDePrueba(t, idFactorValido1)
	factor := factorConfirmadoDePrueba(t, idFactor, idUsuario, nil)

	m := nuevosMocksVerificarOTP(t, []*dominio.FactorMFA{factor})
	caso := m.casoDeUso()

	codigo := "000000"
	if codigo == codigoTOTPValidoDePrueba(t, ahoraDePrueba()) {
		t.Skip("colisión improbable con el código de control")
	}

	ok, err := caso.Verificar(context.Background(), puertos.ConsultaVerificarOTP{
		IDUsuario: idUsuarioValido1,
		Codigo:    codigo,
		Origen:    origenDePrueba(t),
	})
	if err != nil {
		t.Fatalf("Verificar() devolvió error inesperado: %v", err)
	}
	if ok {
		t.Error("se esperaba false con un código incorrecto")
	}
	nombres := m.auditoria.NombresEventos()
	if len(nombres) != 1 || nombres[0] != "VerificacionOTPFallida" {
		t.Errorf("eventos auditados = %v, esperado [VerificacionOTPFallida]", nombres)
	}
	if len(m.factoresMFA.LlamadasGuardar) != 0 {
		t.Error("no debe persistirse nada cuando la verificación falla")
	}
}

func TestVerificarOTPCasoDeUso_SinFactorConfirmado_InconsistenciaInterna(t *testing.T) {
	m := nuevosMocksVerificarOTP(t, nil)
	caso := m.casoDeUso()

	ok, err := caso.Verificar(context.Background(), puertos.ConsultaVerificarOTP{
		IDUsuario: idUsuarioValido1,
		Codigo:    "123456",
		Origen:    origenDePrueba(t),
	})
	if err != nil {
		t.Fatalf("Verificar() devolvió error inesperado: %v", err)
	}
	if ok {
		t.Error("se esperaba false cuando no hay ningún factor confirmado")
	}
	if len(m.auditoria.LlamadasRegistrar) != 0 {
		t.Error("una inconsistencia interna no debe auditarse como fallo del usuario (INV-MFA-06)")
	}
}
