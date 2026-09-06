package aplicacion_test

import (
	"context"
	"errors"
	"testing"

	"github.com/r-david1/moterus/internal/identidad/aplicacion"
	"github.com/r-david1/moterus/internal/identidad/dominio"
	"github.com/r-david1/moterus/internal/identidad/puertos"
	"github.com/r-david1/moterus/internal/identidad/puertos/mocks"
)

type mocksDeshabilitarMFA struct {
	usuarios    *mocks.RepositorioUsuarios
	factoresMFA *mocks.RepositorioFactoresMFA
	cifrador    *mocks.CifradorSecretos
	auditoria   *mocks.RegistroAuditoria
	eventos     *mocks.PublicadorEventos
	reloj       *mocks.Reloj
	uow         *mocks.UnidadDeTrabajo
}

func nuevosMocksDeshabilitarMFA(t *testing.T, usuario *dominio.Usuario, factoresConfirmados []*dominio.FactorMFA) *mocksDeshabilitarMFA {
	t.Helper()
	return &mocksDeshabilitarMFA{
		usuarios: &mocks.RepositorioUsuarios{
			FnBuscarPorID: func(ctx context.Context, id dominio.IDUsuario) (*dominio.Usuario, error) {
				if usuario != nil && usuario.ID().EsIgual(id) {
					return usuario, nil
				}
				return nil, nil
			},
		},
		factoresMFA: &mocks.RepositorioFactoresMFA{
			FnBuscarConfirmadosDeUsuario: func(ctx context.Context, u dominio.IDUsuario) ([]*dominio.FactorMFA, error) {
				return factoresConfirmados, nil
			},
		},
		cifrador:  &mocks.CifradorSecretos{},
		auditoria: &mocks.RegistroAuditoria{},
		eventos:   &mocks.PublicadorEventos{},
		reloj:     &mocks.Reloj{Fija: ahoraDePrueba()},
		uow:       &mocks.UnidadDeTrabajo{},
	}
}

func (m *mocksDeshabilitarMFA) casoDeUso() *aplicacion.DeshabilitarMFACasoDeUso {
	return aplicacion.NuevoDeshabilitarMFACasoDeUso(
		m.usuarios, m.factoresMFA, m.cifrador, m.auditoria, m.eventos, m.reloj, m.uow,
	)
}

func TestDeshabilitarMFACasoDeUso_FlujoFelizConTOTP(t *testing.T) {
	idUsuario := idDePrueba(t, idUsuarioValido1)
	idFactor := idFactorDePrueba(t, idFactorValido1)
	correo := correoDePrueba(t, correoValidoValor)
	hash := hashDePrueba(t, "hashvigente")
	usuario := usuarioConMFADePrueba(t, idUsuario, correo, hash)
	factor := factorConfirmadoDePrueba(t, idFactor, idUsuario, nil)

	m := nuevosMocksDeshabilitarMFA(t, usuario, []*dominio.FactorMFA{factor})
	caso := m.casoDeUso()

	cmd := puertos.ComandoDeshabilitarMFA{
		IDSujeto: idUsuarioValido1,
		Codigo:   codigoTOTPValidoDePrueba(t, ahoraDePrueba()),
	}
	if err := caso.Deshabilitar(context.Background(), cmd); err != nil {
		t.Fatalf("Deshabilitar() devolvió error inesperado: %v", err)
	}

	if len(m.usuarios.LlamadasGuardar) != 1 {
		t.Fatalf("se esperaba 1 llamada a usuarios.Guardar, hubo %d", len(m.usuarios.LlamadasGuardar))
	}
	if m.usuarios.LlamadasGuardar[0].TieneMFA() {
		t.Error("Usuario.TieneMFA() debe quedar en false tras deshabilitar el único factor")
	}
	if len(m.factoresMFA.LlamadasGuardar) != 1 {
		t.Fatalf("se esperaba 1 llamada a factoresMFA.Guardar, hubo %d", len(m.factoresMFA.LlamadasGuardar))
	}

	nombres := m.auditoria.NombresEventos()
	if len(nombres) != 1 || nombres[0] != "FactorMFADeshabilitado" {
		t.Errorf("eventos auditados = %v, esperado [FactorMFADeshabilitado]", nombres)
	}
}

func TestDeshabilitarMFACasoDeUso_FlujoFelizConCodigoDeRespaldo(t *testing.T) {
	idUsuario := idDePrueba(t, idUsuarioValido1)
	idFactor := idFactorDePrueba(t, idFactorValido1)
	correo := correoDePrueba(t, correoValidoValor)
	hash := hashDePrueba(t, "hashvigente")
	usuario := usuarioConMFADePrueba(t, idUsuario, correo, hash)
	factor := factorConfirmadoDePrueba(t, idFactor, idUsuario, []dominio.CodigoRespaldoMFA{codigoRespaldoDisponibleDePrueba(t)})

	m := nuevosMocksDeshabilitarMFA(t, usuario, []*dominio.FactorMFA{factor})
	caso := m.casoDeUso()

	cmd := puertos.ComandoDeshabilitarMFA{
		IDSujeto: idUsuarioValido1,
		Codigo:   codigoRespaldoPlanoDePrueba,
	}
	if err := caso.Deshabilitar(context.Background(), cmd); err != nil {
		t.Fatalf("Deshabilitar() devolvió error inesperado: %v", err)
	}

	nombres := m.auditoria.NombresEventos()
	tieneConsumido, tieneDeshabilitado := false, false
	for _, n := range nombres {
		if n == "CodigoRespaldoConsumido" {
			tieneConsumido = true
		}
		if n == "FactorMFADeshabilitado" {
			tieneDeshabilitado = true
		}
	}
	if !tieneConsumido || !tieneDeshabilitado {
		t.Errorf("eventos auditados = %v, se esperaban CodigoRespaldoConsumido y FactorMFADeshabilitado", nombres)
	}
}

func TestDeshabilitarMFACasoDeUso_CodigoIncorrectoNoDeshabilitaNada(t *testing.T) {
	idUsuario := idDePrueba(t, idUsuarioValido1)
	idFactor := idFactorDePrueba(t, idFactorValido1)
	correo := correoDePrueba(t, correoValidoValor)
	hash := hashDePrueba(t, "hashvigente")
	usuario := usuarioConMFADePrueba(t, idUsuario, correo, hash)
	factor := factorConfirmadoDePrueba(t, idFactor, idUsuario, nil)

	m := nuevosMocksDeshabilitarMFA(t, usuario, []*dominio.FactorMFA{factor})
	caso := m.casoDeUso()

	cmd := puertos.ComandoDeshabilitarMFA{IDSujeto: idUsuarioValido1, Codigo: "000000"}
	if cmd.Codigo == codigoTOTPValidoDePrueba(t, ahoraDePrueba()) {
		t.Skip("colisión improbable con el código de control")
	}

	err := caso.Deshabilitar(context.Background(), cmd)
	if err == nil {
		t.Fatal("se esperaba ErrCodigoOTPInvalido")
	}
	var codigoInvalido *dominio.ErrCodigoOTPInvalido
	if !errors.As(err, &codigoInvalido) {
		t.Errorf("se esperaba *ErrCodigoOTPInvalido, obtuvo %T", err)
	}

	if len(m.usuarios.LlamadasGuardar) != 0 {
		t.Error("no debe persistirse el usuario cuando el código es incorrecto")
	}
	if len(m.factoresMFA.LlamadasGuardar) != 0 {
		t.Error("no debe persistirse el factor cuando el código es incorrecto")
	}
	if !usuario.TieneMFA() {
		t.Error("un código incorrecto no debe desactivar Usuario.TieneMFA()")
	}

	nombres := m.auditoria.NombresEventos()
	if len(nombres) != 1 || nombres[0] != "VerificacionOTPFallida" {
		t.Errorf("eventos auditados = %v, esperado [VerificacionOTPFallida]", nombres)
	}
}

func TestDeshabilitarMFACasoDeUso_SinFactorConfirmado(t *testing.T) {
	idUsuario := idDePrueba(t, idUsuarioValido1)
	correo := correoDePrueba(t, correoValidoValor)
	hash := hashDePrueba(t, "hashvigente")
	usuario := usuarioActivoDePrueba(t, idUsuario, correo, hash)

	m := nuevosMocksDeshabilitarMFA(t, usuario, nil)
	caso := m.casoDeUso()

	cmd := puertos.ComandoDeshabilitarMFA{IDSujeto: idUsuarioValido1, Codigo: "123456"}
	err := caso.Deshabilitar(context.Background(), cmd)
	if err == nil {
		t.Fatal("se esperaba ErrCodigoOTPInvalido")
	}
	var codigoInvalido *dominio.ErrCodigoOTPInvalido
	if !errors.As(err, &codigoInvalido) {
		t.Errorf("se esperaba *ErrCodigoOTPInvalido, obtuvo %T", err)
	}
	if len(m.auditoria.LlamadasRegistrar) != 0 {
		t.Error("no debe auditarse nada cuando el usuario nunca tuvo un factor confirmado")
	}
}
