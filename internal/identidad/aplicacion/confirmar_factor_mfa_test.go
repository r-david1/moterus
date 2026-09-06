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

type mocksConfirmarFactorMFA struct {
	usuarios         *mocks.RepositorioUsuarios
	factoresMFA      *mocks.RepositorioFactoresMFA
	generadorSecreto *mocks.GeneradorSecretoTOTP
	cifrador         *mocks.CifradorSecretos
	auditoria        *mocks.RegistroAuditoria
	eventos          *mocks.PublicadorEventos
	reloj            *mocks.Reloj
	uow              *mocks.UnidadDeTrabajo
}

func nuevosMocksConfirmarFactorMFA(t *testing.T, usuario *dominio.Usuario, factor *dominio.FactorMFA) *mocksConfirmarFactorMFA {
	t.Helper()
	return &mocksConfirmarFactorMFA{
		usuarios: &mocks.RepositorioUsuarios{
			FnBuscarPorID: func(ctx context.Context, id dominio.IDUsuario) (*dominio.Usuario, error) {
				if usuario != nil && usuario.ID().EsIgual(id) {
					return usuario, nil
				}
				return nil, nil
			},
		},
		factoresMFA: &mocks.RepositorioFactoresMFA{
			FnBuscarPorID: func(ctx context.Context, id dominio.IDFactorMFA) (*dominio.FactorMFA, error) {
				if factor != nil && factor.ID().EsIgual(id) {
					return factor, nil
				}
				return nil, &dominio.ErrFactorMFANoEncontrado{}
			},
		},
		// El default de mocks.GeneradorSecretoTOTP.GenerarCodigosRespaldo
		// genera "ABCDEFGH00".."ABCDEFGH09", que contienen '0'/'1': fuera del
		// alfabeto restringido de dominio.CodigoRespaldoPlano (sin 0/O/1/I/L)
		// y por lo tanto siempre rechazados por NuevoCodigoRespaldoPlano. Es
		// un defecto del mock ya cerrado (identidad/puertos/mocks/mocks.go),
		// reportado en el resumen final; aquí se rodea configurando
		// explícitamente un generador que sí produce códigos válidos.
		generadorSecreto: &mocks.GeneradorSecretoTOTP{
			FnGenerarCodigosRespaldo: codigosRespaldoValidosDePrueba,
		},
		cifrador:  &mocks.CifradorSecretos{},
		auditoria: &mocks.RegistroAuditoria{},
		eventos:   &mocks.PublicadorEventos{},
		reloj:     &mocks.Reloj{Fija: ahoraDePrueba()},
		uow:       &mocks.UnidadDeTrabajo{},
	}
}

func (m *mocksConfirmarFactorMFA) casoDeUso() *aplicacion.ConfirmarFactorMFACasoDeUso {
	return aplicacion.NuevoConfirmarFactorMFACasoDeUso(
		m.usuarios, m.factoresMFA, m.generadorSecreto, m.cifrador,
		m.auditoria, m.eventos, m.reloj, m.uow,
	)
}

func TestConfirmarFactorMFACasoDeUso_FlujoFeliz(t *testing.T) {
	idUsuario := idDePrueba(t, idUsuarioValido1)
	idFactor := idFactorDePrueba(t, idFactorValido1)
	correo := correoDePrueba(t, correoValidoValor)
	hash := hashDePrueba(t, "hashvigente")
	usuario := usuarioActivoDePrueba(t, idUsuario, correo, hash)
	factor := factorSinConfirmarDePrueba(t, idFactor, idUsuario)

	m := nuevosMocksConfirmarFactorMFA(t, usuario, factor)
	caso := m.casoDeUso()

	cmd := puertos.ComandoConfirmarFactorMFA{
		IDSujeto: idUsuarioValido1,
		IDFactor: idFactorValido1,
		Codigo:   codigoTOTPValidoDePrueba(t, ahoraDePrueba()),
	}
	resultado, err := caso.ConfirmarFactor(context.Background(), cmd)
	if err != nil {
		t.Fatalf("ConfirmarFactor() devolvió error inesperado: %v", err)
	}
	if len(resultado.CodigosRespaldo) != 10 {
		t.Fatalf("se esperaban 10 códigos de respaldo, hubo %d", len(resultado.CodigosRespaldo))
	}
	for _, c := range resultado.CodigosRespaldo {
		if c == "" {
			t.Error("ningún código de respaldo debe venir vacío")
		}
	}

	if !factor.EstaConfirmado() {
		t.Error("el factor debe quedar confirmado")
	}

	if len(m.usuarios.LlamadasGuardar) != 1 {
		t.Fatalf("se esperaba 1 llamada a usuarios.Guardar, hubo %d", len(m.usuarios.LlamadasGuardar))
	}
	// El flip de tieneMFA debe ocurrir en la misma unidad de trabajo que la
	// confirmación: el usuario persistido ya debe reflejarlo.
	if !m.usuarios.LlamadasGuardar[0].TieneMFA() {
		t.Error("Usuario.TieneMFA() debe ser true tras confirmar el primer factor (INV-ID-08)")
	}
	if len(m.factoresMFA.LlamadasGuardar) != 1 {
		t.Fatalf("se esperaba 1 llamada a factoresMFA.Guardar, hubo %d", len(m.factoresMFA.LlamadasGuardar))
	}

	nombres := m.auditoria.NombresEventos()
	if len(nombres) != 1 || nombres[0] != "FactorMFAConfirmado" {
		t.Errorf("eventos auditados = %v, esperado [FactorMFAConfirmado]", nombres)
	}
}

func TestConfirmarFactorMFACasoDeUso_CodigoIncorrectoNoFlipeaNada(t *testing.T) {
	idUsuario := idDePrueba(t, idUsuarioValido1)
	idFactor := idFactorDePrueba(t, idFactorValido1)
	correo := correoDePrueba(t, correoValidoValor)
	hash := hashDePrueba(t, "hashvigente")
	usuario := usuarioActivoDePrueba(t, idUsuario, correo, hash)
	factor := factorSinConfirmarDePrueba(t, idFactor, idUsuario)

	m := nuevosMocksConfirmarFactorMFA(t, usuario, factor)
	caso := m.casoDeUso()

	cmd := puertos.ComandoConfirmarFactorMFA{
		IDSujeto: idUsuarioValido1,
		IDFactor: idFactorValido1,
		Codigo:   "000000",
	}
	// Descarta la colisión improbable con el código de control.
	if cmd.Codigo == codigoTOTPValidoDePrueba(t, ahoraDePrueba()) {
		t.Skip("colisión improbable con el código de control")
	}

	_, err := caso.ConfirmarFactor(context.Background(), cmd)
	if err == nil {
		t.Fatal("se esperaba ErrCodigoOTPInvalido")
	}
	var codigoInvalido *dominio.ErrCodigoOTPInvalido
	if !errors.As(err, &codigoInvalido) {
		t.Errorf("se esperaba *ErrCodigoOTPInvalido, obtuvo %T", err)
	}

	if factor.EstaConfirmado() {
		t.Error("un código incorrecto no debe confirmar el factor")
	}
	if usuario.TieneMFA() {
		t.Error("un código incorrecto no debe activar Usuario.TieneMFA()")
	}
	if len(m.usuarios.LlamadasGuardar) != 0 {
		t.Error("no debe persistirse el usuario cuando el código es incorrecto")
	}
	if len(m.factoresMFA.LlamadasGuardar) != 0 {
		t.Error("no debe persistirse el factor cuando el código es incorrecto")
	}
	if len(m.auditoria.LlamadasRegistrar) != 0 {
		t.Error("no debe auditarse nada cuando el código es incorrecto (el dominio no muta el agregado)")
	}
}

func TestConfirmarFactorMFACasoDeUso_FactorDeOtroUsuario(t *testing.T) {
	idUsuario := idDePrueba(t, idUsuarioValido1)
	idOtro := idDePrueba(t, idUsuarioValido2)
	idFactor := idFactorDePrueba(t, idFactorValido1)
	correo := correoDePrueba(t, correoValidoValor)
	hash := hashDePrueba(t, "hashvigente")
	usuario := usuarioActivoDePrueba(t, idUsuario, correo, hash)
	factorAjeno := factorSinConfirmarDePrueba(t, idFactor, idOtro)

	m := nuevosMocksConfirmarFactorMFA(t, usuario, factorAjeno)
	caso := m.casoDeUso()

	cmd := puertos.ComandoConfirmarFactorMFA{
		IDSujeto: idUsuarioValido1,
		IDFactor: idFactorValido1,
		Codigo:   codigoTOTPValidoDePrueba(t, ahoraDePrueba()),
	}
	_, err := caso.ConfirmarFactor(context.Background(), cmd)
	if err == nil {
		t.Fatal("se esperaba ErrFactorMFANoEncontrado")
	}
	var noEncontrado *dominio.ErrFactorMFANoEncontrado
	if !errors.As(err, &noEncontrado) {
		t.Errorf("se esperaba *ErrFactorMFANoEncontrado, obtuvo %T", err)
	}
}

func TestConfirmarFactorMFACasoDeUso_FactorNoEncontrado(t *testing.T) {
	idUsuario := idDePrueba(t, idUsuarioValido1)
	correo := correoDePrueba(t, correoValidoValor)
	hash := hashDePrueba(t, "hashvigente")
	usuario := usuarioActivoDePrueba(t, idUsuario, correo, hash)

	m := nuevosMocksConfirmarFactorMFA(t, usuario, nil)
	caso := m.casoDeUso()

	cmd := puertos.ComandoConfirmarFactorMFA{
		IDSujeto: idUsuarioValido1,
		IDFactor: idFactorValido1,
		Codigo:   "123456",
	}
	_, err := caso.ConfirmarFactor(context.Background(), cmd)
	if err == nil {
		t.Fatal("se esperaba ErrFactorMFANoEncontrado")
	}
	var noEncontrado *dominio.ErrFactorMFANoEncontrado
	if !errors.As(err, &noEncontrado) {
		t.Errorf("se esperaba *ErrFactorMFANoEncontrado, obtuvo %T", err)
	}
}

func TestConfirmarFactorMFACasoDeUso_FactorYaConfirmado(t *testing.T) {
	idUsuario := idDePrueba(t, idUsuarioValido1)
	idFactor := idFactorDePrueba(t, idFactorValido1)
	correo := correoDePrueba(t, correoValidoValor)
	hash := hashDePrueba(t, "hashvigente")
	usuario := usuarioConMFADePrueba(t, idUsuario, correo, hash)
	factor := factorConfirmadoDePrueba(t, idFactor, idUsuario, nil)

	m := nuevosMocksConfirmarFactorMFA(t, usuario, factor)
	caso := m.casoDeUso()

	cmd := puertos.ComandoConfirmarFactorMFA{
		IDSujeto: idUsuarioValido1,
		IDFactor: idFactorValido1,
		Codigo:   codigoTOTPValidoDePrueba(t, ahoraDePrueba()),
	}
	_, err := caso.ConfirmarFactor(context.Background(), cmd)
	if err == nil {
		t.Fatal("se esperaba ErrFactorMFAYaConfirmado")
	}
	var yaConfirmado *dominio.ErrFactorMFAYaConfirmado
	if !errors.As(err, &yaConfirmado) {
		t.Errorf("se esperaba *ErrFactorMFAYaConfirmado, obtuvo %T", err)
	}
}
