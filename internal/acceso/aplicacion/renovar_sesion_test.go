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

type mocksRenovarSesion struct {
	confianza       *mocks.EvaluadorConfianza
	sesiones        *mocks.RepositorioSesiones
	refrescos       *mocks.GeneradorTokensRefresco
	firmador        *mocks.FirmadorTokensAcceso
	consultorEstado *mocks.ConsultorEstadoSujeto
	listaRevocacion *mocks.ListaRevocacion
	auditoria       *mocks.RegistroAuditoria
	reloj           *mocks.Reloj
	ids             *mocks.GeneradorIDs
	uow             *mocks.UnidadDeTrabajo
	politica        dominio.PoliticaSesion
}

// nuevosMocksRenovarSesion configura un camino feliz: Confianza permite,
// el hash presentado resuelve a sesionVigente en RefrescoVigente y el
// sujeto está activo.
func nuevosMocksRenovarSesion(t *testing.T, sesionVigente *dominio.Sesion) *mocksRenovarSesion {
	t.Helper()
	return &mocksRenovarSesion{
		confianza: &mocks.EvaluadorConfianza{},
		sesiones: &mocks.RepositorioSesiones{
			FnBuscarPorHashRefresco: func(ctx context.Context, h dominio.HashTokenRefresco) (*dominio.Sesion, puertos.SituacionRefresco, int, error) {
				if sesionVigente == nil {
					return nil, puertos.RefrescoDesconocido, 0, nil
				}
				return sesionVigente, puertos.RefrescoVigente, sesionVigente.Generacion(), nil
			},
		},
		refrescos: &mocks.GeneradorTokensRefresco{
			FnGenerar: func() (dominio.TokenRefrescoPlano, error) {
				return tokenRefrescoPlanoDePrueba(t, tokenRefrescoValor2), nil
			},
		},
		firmador:        &mocks.FirmadorTokensAcceso{},
		consultorEstado: &mocks.ConsultorEstadoSujeto{},
		listaRevocacion: &mocks.ListaRevocacion{},
		auditoria:       &mocks.RegistroAuditoria{},
		reloj:           &mocks.Reloj{Fija: ahoraDePrueba()},
		ids: &mocks.GeneradorIDs{
			FnNuevoIDTokenAcceso: func() (dominio.IDTokenAcceso, error) {
				return dominio.IDTokenAccesoDesde("123e4567-e89b-42d3-a456-426614174099")
			},
		},
		uow:      &mocks.UnidadDeTrabajo{},
		politica: politicaDePrueba(t),
	}
}

func (m *mocksRenovarSesion) casoDeUso() *aplicacion.RenovarSesionCasoDeUso {
	return aplicacion.NuevoRenovarSesionCasoDeUso(
		m.confianza, m.sesiones, m.refrescos, m.firmador, m.consultorEstado, m.listaRevocacion,
		m.auditoria, m.reloj, m.ids, m.uow, m.politica, emisorDePrueba, audienciaDePrueba,
	)
}

func comandoRenovarSesionValido(t *testing.T) aplicacion.ComandoRenovarSesion {
	t.Helper()
	return aplicacion.ComandoRenovarSesion{
		TokenRefresco: tokenRefrescoValor1,
		Origen:        origenDePrueba(t),
	}
}

func TestRenovarSesionCasoDeUso_FlujoFeliz(t *testing.T) {
	usuarioID := idUsuarioDePrueba(t, idUsuarioValido1)
	sesion := sesionActivaDePrueba(t, idSesionDePrueba(t, idSesionValido1), usuarioID, 0,
		tokenRefrescoValor1, ahoraDePrueba().Add(-1*time.Hour), ahoraDePrueba().Add(23*time.Hour), ahoraDePrueba().Add(47*time.Hour))

	m := nuevosMocksRenovarSesion(t, sesion)
	caso := m.casoDeUso()

	resultado, err := caso.Renovar(context.Background(), comandoRenovarSesionValido(t))
	if err != nil {
		t.Fatalf("Renovar() devolvió error inesperado: %v", err)
	}
	if resultado.TokenRefresco != tokenRefrescoValor2 {
		t.Errorf("TokenRefresco = %q, esperado %q", resultado.TokenRefresco, tokenRefrescoValor2)
	}
	if resultado.IDSesion != idSesionValido1 {
		t.Errorf("IDSesion = %q, esperado %q", resultado.IDSesion, idSesionValido1)
	}
	if len(m.sesiones.LlamadasGuardar) != 1 {
		t.Fatalf("se esperaba una llamada a Guardar, hubo %d", len(m.sesiones.LlamadasGuardar))
	}
	if m.sesiones.LlamadasGuardar[0].Generacion() != 1 {
		t.Errorf("Generacion tras rotar = %d, esperado 1", m.sesiones.LlamadasGuardar[0].Generacion())
	}
	if nombres := m.auditoria.NombresEventos(); len(nombres) != 1 || nombres[0] != "SesionRenovada" {
		t.Errorf("eventos auditados = %v, esperado [SesionRenovada]", nombres)
	}
}

func TestRenovarSesionCasoDeUso_ConfianzaDeniega(t *testing.T) {
	m := nuevosMocksRenovarSesion(t, nil)
	m.confianza.FnEvaluar = func(ctx context.Context, s puertos.SolicitudEvaluacion) (puertos.DecisionConfianza, error) {
		return puertos.DecisionConfianza{Permitido: false, Motivo: "demasiadas_renovaciones", ReintentarEn: 30 * time.Second}, nil
	}
	caso := m.casoDeUso()

	_, err := caso.Renovar(context.Background(), comandoRenovarSesionValido(t))
	var errConfianza *dominio.ErrAccesoDenegadoPorConfianza
	if !errors.As(err, &errConfianza) {
		t.Fatalf("se esperaba *ErrAccesoDenegadoPorConfianza, obtuvo %T: %v", err, err)
	}
	if errConfianza.ReintentarEn != 30*time.Second {
		t.Errorf("ReintentarEn = %v, esperado 30s", errConfianza.ReintentarEn)
	}
	if len(m.sesiones.LlamadasBuscarPorHashRefresco) != 0 {
		t.Error("una renovación denegada por Confianza no debe llegar a resolver el token")
	}
	if len(m.auditoria.LlamadasRegistrar) != 0 {
		t.Error("la denegación de Confianza en renovación no se audita")
	}
}

func TestRenovarSesionCasoDeUso_RefrescoDesconocido(t *testing.T) {
	m := nuevosMocksRenovarSesion(t, nil)
	caso := m.casoDeUso()

	_, err := caso.Renovar(context.Background(), comandoRenovarSesionValido(t))
	var errRefresco *dominio.ErrRefrescoInvalido
	if !errors.As(err, &errRefresco) {
		t.Fatalf("se esperaba *ErrRefrescoInvalido, obtuvo %T: %v", err, err)
	}
	if len(m.sesiones.LlamadasGuardar) != 0 {
		t.Error("un refresco desconocido no debe persistir nada")
	}
	if nombres := m.auditoria.NombresEventos(); len(nombres) != 1 || nombres[0] != "RenovacionRechazada" {
		t.Errorf("eventos auditados = %v, esperado [RenovacionRechazada]", nombres)
	}
}

// TestRenovarSesionCasoDeUso_ReusoDetectado verifica INV-ACC-06: presentar
// un refresco ya consumido revoca TODA la sesión y devuelve el mismo error
// que un refresco desconocido (INV-ACC-21).
func TestRenovarSesionCasoDeUso_ReusoDetectado(t *testing.T) {
	usuarioID := idUsuarioDePrueba(t, idUsuarioValido1)
	// La sesión ya rotó una vez (generación vigente = 1); el token
	// presentado en este intento es de la generación 0, ya consumida.
	sesion := sesionActivaDePrueba(t, idSesionDePrueba(t, idSesionValido1), usuarioID, 1,
		tokenRefrescoValor2, ahoraDePrueba().Add(-1*time.Hour), ahoraDePrueba().Add(23*time.Hour), ahoraDePrueba().Add(47*time.Hour))

	m := nuevosMocksRenovarSesion(t, nil)
	m.sesiones.FnBuscarPorHashRefresco = func(ctx context.Context, h dominio.HashTokenRefresco) (*dominio.Sesion, puertos.SituacionRefresco, int, error) {
		return sesion, puertos.RefrescoConsumido, 0, nil
	}
	caso := m.casoDeUso()

	_, err := caso.Renovar(context.Background(), comandoRenovarSesionValido(t))
	var errRefresco *dominio.ErrRefrescoInvalido
	if !errors.As(err, &errRefresco) {
		t.Fatalf("se esperaba *ErrRefrescoInvalido, obtuvo %T: %v", err, err)
	}

	if len(m.sesiones.LlamadasGuardar) != 1 {
		t.Fatalf("se esperaba una llamada a Guardar (la sesión revocada), hubo %d", len(m.sesiones.LlamadasGuardar))
	}
	revocada := m.sesiones.LlamadasGuardar[0]
	if !revocada.Estado().EsIgual(dominio.EstadoSesionRevocada) {
		t.Error("toda la sesión debía quedar revocada tras detectar el reuso")
	}
	if motivo, ok := revocada.MotivoRevocacion(); !ok || !motivo.EsIgual(dominio.MotivoReusoRefrescoDetectado) {
		t.Errorf("MotivoRevocacion = %v, esperado reuso_refresco_detectado", motivo)
	}
	if nombres := m.auditoria.NombresEventos(); len(nombres) != 1 || nombres[0] != "ReusoRefrescoDetectado" {
		t.Errorf("eventos auditados = %v, esperado [ReusoRefrescoDetectado]", nombres)
	}
	if len(m.listaRevocacion.LlamadasRevocarSesion) != 1 {
		t.Error("se esperaba propagar la revocación a la lista tras un reuso detectado")
	}
}

// TestRenovarSesionCasoDeUso_SesionExpirada verifica que una sesión cuya
// ventana de inactividad ya venció (pero seguía en estado activa) se
// marca expirada y devuelve ErrSesionExpirada.
func TestRenovarSesionCasoDeUso_SesionExpirada(t *testing.T) {
	usuarioID := idUsuarioDePrueba(t, idUsuarioValido1)
	sesion := sesionActivaDePrueba(t, idSesionDePrueba(t, idSesionValido1), usuarioID, 0,
		tokenRefrescoValor1, ahoraDePrueba().Add(-25*time.Hour), ahoraDePrueba().Add(-1*time.Hour), ahoraDePrueba().Add(47*time.Hour))

	m := nuevosMocksRenovarSesion(t, sesion)
	caso := m.casoDeUso()

	_, err := caso.Renovar(context.Background(), comandoRenovarSesionValido(t))
	var errExpirada *dominio.ErrSesionExpirada
	if !errors.As(err, &errExpirada) {
		t.Fatalf("se esperaba *ErrSesionExpirada, obtuvo %T: %v", err, err)
	}
	if len(m.sesiones.LlamadasGuardar) != 1 {
		t.Fatalf("se esperaba una llamada a Guardar (MarcarExpirada), hubo %d", len(m.sesiones.LlamadasGuardar))
	}
	if !m.sesiones.LlamadasGuardar[0].Estado().EsIgual(dominio.EstadoSesionExpirada) {
		t.Error("la sesión debía quedar marcada como expirada")
	}
	if nombres := m.auditoria.NombresEventos(); len(nombres) != 1 || nombres[0] != "RenovacionRechazada" {
		t.Errorf("eventos auditados = %v, esperado [RenovacionRechazada]", nombres)
	}
}

// TestRenovarSesionCasoDeUso_SesionYaRevocada verifica el caso en el que
// el hash presentado sigue siendo el "vigente" en tokens_refresco (nunca
// se consumió) pero la sesión ya está revocada por otra vía (p. ej. un
// logout individual previo). No hay nada que volver a persistir.
func TestRenovarSesionCasoDeUso_SesionYaRevocada(t *testing.T) {
	usuarioID := idUsuarioDePrueba(t, idUsuarioValido1)
	sesion := sesionRevocadaDePrueba(t, idSesionDePrueba(t, idSesionValido1), usuarioID,
		tokenRefrescoValor1, dominio.MotivoCierreUsuario,
		ahoraDePrueba().Add(-2*time.Hour), ahoraDePrueba().Add(-1*time.Hour), ahoraDePrueba().Add(46*time.Hour))

	m := nuevosMocksRenovarSesion(t, sesion)
	caso := m.casoDeUso()

	_, err := caso.Renovar(context.Background(), comandoRenovarSesionValido(t))
	var errRevocada *dominio.ErrSesionRevocada
	if !errors.As(err, &errRevocada) {
		t.Fatalf("se esperaba *ErrSesionRevocada, obtuvo %T: %v", err, err)
	}
	if len(m.sesiones.LlamadasGuardar) != 0 {
		t.Error("una sesión ya revocada no debe volver a persistirse")
	}
	if nombres := m.auditoria.NombresEventos(); len(nombres) != 1 || nombres[0] != "RenovacionRechazada" {
		t.Errorf("eventos auditados = %v, esperado [RenovacionRechazada]", nombres)
	}
}

// TestRenovarSesionCasoDeUso_CuentaNoOperativa verifica el paso 5 de la
// sección 3.2 del diseño: la revalidación del estado del sujeto contra
// Identidad en cada renovación.
func TestRenovarSesionCasoDeUso_CuentaNoOperativa(t *testing.T) {
	usuarioID := idUsuarioDePrueba(t, idUsuarioValido1)
	sesion := sesionActivaDePrueba(t, idSesionDePrueba(t, idSesionValido1), usuarioID, 0,
		tokenRefrescoValor1, ahoraDePrueba().Add(-1*time.Hour), ahoraDePrueba().Add(23*time.Hour), ahoraDePrueba().Add(47*time.Hour))

	m := nuevosMocksRenovarSesion(t, sesion)
	m.consultorEstado.FnEstadoDe = func(ctx context.Context, idUsuario string) (puertos.EstadoSujeto, error) {
		return puertos.EstadoSujeto{Existe: true, Estado: "suspendido"}, nil
	}
	caso := m.casoDeUso()

	_, err := caso.Renovar(context.Background(), comandoRenovarSesionValido(t))
	var errRevocada *dominio.ErrSesionRevocada
	if !errors.As(err, &errRevocada) {
		t.Fatalf("se esperaba *ErrSesionRevocada, obtuvo %T: %v", err, err)
	}
	if len(m.sesiones.LlamadasGuardar) != 1 {
		t.Fatalf("se esperaba una llamada a Guardar (la sesión revocada), hubo %d", len(m.sesiones.LlamadasGuardar))
	}
	if motivo, ok := m.sesiones.LlamadasGuardar[0].MotivoRevocacion(); !ok || !motivo.EsIgual(dominio.MotivoCuentaNoOperativa) {
		t.Errorf("MotivoRevocacion = %v, esperado cuenta_no_operativa", motivo)
	}
	if nombres := m.auditoria.NombresEventos(); len(nombres) != 1 || nombres[0] != "SesionRevocada" {
		t.Errorf("eventos auditados = %v, esperado [SesionRevocada]", nombres)
	}
}
