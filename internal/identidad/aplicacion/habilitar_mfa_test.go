package aplicacion_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/r-david1/moterus/internal/identidad/aplicacion"
	"github.com/r-david1/moterus/internal/identidad/dominio"
	"github.com/r-david1/moterus/internal/identidad/puertos"
	"github.com/r-david1/moterus/internal/identidad/puertos/mocks"
)

type mocksHabilitarMFA struct {
	usuarios         *mocks.RepositorioUsuarios
	factoresMFA      *mocks.RepositorioFactoresMFA
	generadorSecreto *mocks.GeneradorSecretoTOTP
	cifrador         *mocks.CifradorSecretos
	auditoria        *mocks.RegistroAuditoria
	eventos          *mocks.PublicadorEventos
	reloj            *mocks.Reloj
	ids              *mocks.GeneradorIDs
	uow              *mocks.UnidadDeTrabajo
}

func nuevosMocksHabilitarMFA(t *testing.T, usuario *dominio.Usuario, confirmados int) *mocksHabilitarMFA {
	t.Helper()
	return &mocksHabilitarMFA{
		usuarios: &mocks.RepositorioUsuarios{
			FnBuscarPorID: func(ctx context.Context, id dominio.IDUsuario) (*dominio.Usuario, error) {
				if usuario != nil && usuario.ID().EsIgual(id) {
					return usuario, nil
				}
				return nil, nil
			},
		},
		factoresMFA: &mocks.RepositorioFactoresMFA{
			FnContarConfirmadosDeUsuario: func(ctx context.Context, u dominio.IDUsuario) (int, error) {
				return confirmados, nil
			},
		},
		generadorSecreto: &mocks.GeneradorSecretoTOTP{},
		cifrador:         &mocks.CifradorSecretos{},
		auditoria:        &mocks.RegistroAuditoria{},
		eventos:          &mocks.PublicadorEventos{},
		reloj:            &mocks.Reloj{Fija: ahoraDePrueba()},
		ids:              &mocks.GeneradorIDs{FnNuevoIDUsuario: func() (dominio.IDUsuario, error) { return idDePrueba(t, idFactorValido1), nil }},
		uow:              &mocks.UnidadDeTrabajo{},
	}
}

func (m *mocksHabilitarMFA) casoDeUso() *aplicacion.HabilitarMFACasoDeUso {
	return aplicacion.NuevoHabilitarMFACasoDeUso(
		m.usuarios, m.factoresMFA, m.generadorSecreto, m.cifrador,
		m.auditoria, m.eventos, m.reloj, m.ids, m.uow,
	)
}

func TestHabilitarMFACasoDeUso_FlujoFeliz(t *testing.T) {
	id := idDePrueba(t, idUsuarioValido1)
	correo := correoDePrueba(t, correoValidoValor)
	hash := hashDePrueba(t, "hashvigente")
	usuario := usuarioActivoDePrueba(t, id, correo, hash)

	m := nuevosMocksHabilitarMFA(t, usuario, 0)
	caso := m.casoDeUso()

	resultado, err := caso.Habilitar(context.Background(), puertos.ComandoHabilitarMFA{IDSujeto: idUsuarioValido1})
	if err != nil {
		t.Fatalf("Habilitar() devolvió error inesperado: %v", err)
	}
	if resultado.SecretoEnClaro == "" {
		t.Error("SecretoEnClaro no debe estar vacío")
	}
	if !strings.HasPrefix(resultado.URIProvisionamiento, "otpauth://totp/") {
		t.Errorf("URIProvisionamiento = %q, se esperaba prefijo otpauth://totp/", resultado.URIProvisionamiento)
	}
	if resultado.IDFactor == "" {
		t.Error("IDFactor no debe estar vacío")
	}

	if len(m.factoresMFA.LlamadasGuardar) != 1 {
		t.Fatalf("se esperaba 1 llamada a Guardar, hubo %d", len(m.factoresMFA.LlamadasGuardar))
	}
	factorGuardado := m.factoresMFA.LlamadasGuardar[0]
	if factorGuardado.EstaConfirmado() {
		t.Error("el factor recién habilitado no debe quedar confirmado (INV-MFA-01)")
	}

	nombres := m.auditoria.NombresEventos()
	if len(nombres) != 1 || nombres[0] != "FactorMFAHabilitado" {
		t.Errorf("eventos auditados = %v, esperado [FactorMFAHabilitado]", nombres)
	}
}

func TestHabilitarMFACasoDeUso_RechazaSiYaHayFactorConfirmado(t *testing.T) {
	id := idDePrueba(t, idUsuarioValido1)
	correo := correoDePrueba(t, correoValidoValor)
	hash := hashDePrueba(t, "hashvigente")
	usuario := usuarioActivoDePrueba(t, id, correo, hash)

	m := nuevosMocksHabilitarMFA(t, usuario, 1)
	caso := m.casoDeUso()

	_, err := caso.Habilitar(context.Background(), puertos.ComandoHabilitarMFA{IDSujeto: idUsuarioValido1})
	if err == nil {
		t.Fatal("se esperaba ErrLimiteFactoresMFAExcedido")
	}
	var limite *dominio.ErrLimiteFactoresMFAExcedido
	if !errors.As(err, &limite) {
		t.Errorf("se esperaba *ErrLimiteFactoresMFAExcedido, obtuvo %T", err)
	}
	if len(m.factoresMFA.LlamadasGuardar) != 0 {
		t.Error("no debe guardarse ningún factor cuando ya existe uno confirmado")
	}
}

func TestHabilitarMFACasoDeUso_RechazaSiUsuarioNoActivo(t *testing.T) {
	id := idDePrueba(t, idUsuarioValido1)
	correo := correoDePrueba(t, correoValidoValor)
	hash := hashDePrueba(t, "hashvigente")
	usuario := usuarioPendienteDePrueba(t, id, correo, hash)

	m := nuevosMocksHabilitarMFA(t, usuario, 0)
	caso := m.casoDeUso()

	_, err := caso.Habilitar(context.Background(), puertos.ComandoHabilitarMFA{IDSujeto: idUsuarioValido1})
	if err == nil {
		t.Fatal("se esperaba un error de estado no operativo")
	}
	var noVerificado *dominio.ErrCorreoNoVerificado
	if !errors.As(err, &noVerificado) {
		t.Errorf("se esperaba *ErrCorreoNoVerificado, obtuvo %T", err)
	}
	if len(m.factoresMFA.LlamadasGuardar) != 0 {
		t.Error("no debe guardarse ningún factor si el usuario no está activo")
	}
}

func TestHabilitarMFACasoDeUso_RechazaSiUsuarioNoEncontrado(t *testing.T) {
	m := nuevosMocksHabilitarMFA(t, nil, 0)
	caso := m.casoDeUso()

	_, err := caso.Habilitar(context.Background(), puertos.ComandoHabilitarMFA{IDSujeto: idUsuarioValido1})
	if err == nil {
		t.Fatal("se esperaba ErrUsuarioNoEncontrado")
	}
	var noEncontrado *dominio.ErrUsuarioNoEncontrado
	if !errors.As(err, &noEncontrado) {
		t.Errorf("se esperaba *ErrUsuarioNoEncontrado, obtuvo %T", err)
	}
}
