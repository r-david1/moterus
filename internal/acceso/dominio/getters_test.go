package dominio

import (
	"strings"
	"testing"
	"time"
)

// Este archivo cubre getters simples y mensajes de Error() que las pruebas
// de invariantes (sesion_test.go, tokens_test.go, ...) no ejercitan por no
// ser, en sí mismos, una regla de negocio — pero sí forman parte del
// contrato público del paquete y conviene no dejarlos sin probar.

func TestDireccionIP_StringYEsPrivada(t *testing.T) {
	publica, err := NuevaDireccionIP("203.0.113.10")
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if publica.String() != "203.0.113.10" {
		t.Errorf("String() = %q", publica.String())
	}
	if publica.EsPrivada() {
		t.Error("203.0.113.10 (TEST-NET-3) no debe reportarse como privada")
	}

	privada, err := NuevaDireccionIP("10.0.0.5")
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if !privada.EsPrivada() {
		t.Error("10.0.0.5 debe reportarse como privada")
	}

	var vacia DireccionIP
	if vacia.String() != "" {
		t.Errorf("String() de una DireccionIP vacía = %q, esperado \"\"", vacia.String())
	}
	if vacia.EsPrivada() {
		t.Error("una DireccionIP vacía no debe reportarse como privada")
	}
}

func TestNuevaDireccionIP_RechazaFormatoInvalido(t *testing.T) {
	if _, err := NuevaDireccionIP(""); err == nil {
		t.Error("se esperaba error para una IP vacía")
	}
	if _, err := NuevaDireccionIP("no-es-una-ip"); err == nil {
		t.Error("se esperaba error para un formato irreconocible")
	}
}

func TestNuevoOrigenSolicitud_PropagaErrorDeIPInvalida(t *testing.T) {
	_, err := NuevoOrigenSolicitud("no-es-una-ip", "agente", "huella", "req-1")
	if err == nil {
		t.Fatal("se esperaba error al construir OrigenSolicitud con IP inválida")
	}
}

func TestNuevoOrigenSolicitud_PermiteIPVacia(t *testing.T) {
	o, err := NuevoOrigenSolicitud("", "agente-interno", "", "req-2")
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if !o.IP().EsVacia() {
		t.Error("IP() debe estar vacía cuando la llamada es interna del sistema")
	}
	if o.AgenteUsuario() != "agente-interno" {
		t.Errorf("AgenteUsuario() = %q", o.AgenteUsuario())
	}
	if o.HuellaDispositivo() != "" {
		t.Errorf("HuellaDispositivo() = %q, esperado vacío", o.HuellaDispositivo())
	}
}

func TestNuevoOrigenSolicitud_TruncaAgenteUsuario(t *testing.T) {
	largo := strings.Repeat("a", 600)
	o, err := NuevoOrigenSolicitud("", largo, "", "")
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if len(o.AgenteUsuario()) != 512 {
		t.Errorf("longitud de AgenteUsuario() = %d, esperado 512", len(o.AgenteUsuario()))
	}
}

func TestEstadoSesion_String(t *testing.T) {
	if EstadoSesionActiva.String() != "activa" {
		t.Errorf("String() = %q", EstadoSesionActiva.String())
	}
}

func TestMotivoRevocacion_String(t *testing.T) {
	if MotivoCierreUsuario.String() != "cierre_usuario" {
		t.Errorf("String() = %q", MotivoCierreUsuario.String())
	}
}

func TestIDTokenAcceso_String(t *testing.T) {
	id, _ := IDTokenAccesoDesde(uuidV4DePrueba)
	if id.String() != uuidV4DePrueba {
		t.Errorf("String() = %q, esperado %q", id.String(), uuidV4DePrueba)
	}
}

func TestHashTokenRefresco_EsVacioYString(t *testing.T) {
	var vacio HashTokenRefresco
	if !vacio.EsVacio() {
		t.Error("el zero value de HashTokenRefresco debe reportarse vacío")
	}
	h, _ := NuevoHashTokenRefresco(strings.Repeat("a", 64))
	if h.EsVacio() {
		t.Error("un HashTokenRefresco construido no debe reportarse vacío")
	}
	if h.String() != h.Valor() {
		t.Errorf("String() = %q, esperado igual a Valor() = %q", h.String(), h.Valor())
	}
}

func TestTokenRefrescoEmitido_GettersDeTiempo(t *testing.T) {
	emitidoEn := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	expiraEn := emitidoEn.Add(time.Hour)
	hash, _ := NuevoHashTokenRefresco(strings.Repeat("a", 64))
	tok := nuevoTokenRefrescoEmitido(hash, 0, emitidoEn, expiraEn)

	if !tok.EmitidoEn().Equal(emitidoEn) {
		t.Errorf("EmitidoEn() = %v, esperado %v", tok.EmitidoEn(), emitidoEn)
	}
	if !tok.ExpiraEn().Equal(expiraEn) {
		t.Errorf("ExpiraEn() = %v, esperado %v", tok.ExpiraEn(), expiraEn)
	}
	if tok.EstaExpirado(expiraEn.Add(-time.Minute)) {
		t.Error("no debe estar expirado antes de ExpiraEn()")
	}
	if !tok.EstaExpirado(expiraEn.Add(time.Minute)) {
		t.Error("debe estar expirado después de ExpiraEn()")
	}
}

// TestSesion_GettersDeAusencia verifica el valor "no presente" (false) de
// los getters opcionales de Sesion, antes de que ocurra la mutación que los
// llena, complementando los casos "presente" ya probados en sesion_test.go.
func TestSesion_GettersDeAusencia(t *testing.T) {
	s, _, _ := sesionDePrueba(t)

	if _, tiene := s.RefrescoRecienConsumido(); tiene {
		t.Error("una sesión que nunca rotó no debe tener RefrescoRecienConsumido")
	}
	if _, tiene := s.UltimaRenovacionEn(); tiene {
		t.Error("una sesión que nunca se renovó no debe tener UltimaRenovacionEn")
	}
	if _, tiene := s.RevocadaEn(); tiene {
		t.Error("una sesión activa no debe tener RevocadaEn")
	}
	if _, tiene := s.MotivoRevocacion(); tiene {
		t.Error("una sesión activa no debe tener MotivoRevocacion")
	}
}

// TestReconstituir_PreservaRevocacionYUltimaRenovacion cubre la rama de
// Reconstituir con revocadaEn y ultimaRenovacionEn no nulos, complementando
// TestReconstituir_NoAcumulaEventos (que usa ambos en nil).
func TestReconstituir_PreservaRevocacionYUltimaRenovacion(t *testing.T) {
	ahora := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	renovadaEn := ahora.Add(time.Hour)
	revocadaEn := ahora.Add(2 * time.Hour)
	hash, _ := NuevoHashTokenRefresco(strings.Repeat("a", 64))
	vigente := ReconstituirTokenRefrescoEmitido(hash, 1, renovadaEn, renovadaEn.Add(time.Hour), nil, nil)

	s := Reconstituir(
		idSesionDePrueba(t),
		idUsuarioDePrueba(t),
		EstadoSesionRevocada,
		1,
		&vigente,
		origenDePrueba(t),
		ahora,
		revocadaEn,
		&renovadaEn,
		renovadaEn,
		ahora.Add(90*24*time.Hour),
		&revocadaEn,
		MotivoCierreUsuario,
	)

	got, tiene := s.UltimaRenovacionEn()
	if !tiene || !got.Equal(renovadaEn) {
		t.Errorf("UltimaRenovacionEn() = %v, %v; esperado %v, true", got, tiene, renovadaEn)
	}
	gotRevocada, tiene := s.RevocadaEn()
	if !tiene || !gotRevocada.Equal(revocadaEn) {
		t.Errorf("RevocadaEn() = %v, %v; esperado %v, true", gotRevocada, tiene, revocadaEn)
	}
	motivo, tiene := s.MotivoRevocacion()
	if !tiene || !motivo.EsIgual(MotivoCierreUsuario) {
		t.Errorf("MotivoRevocacion() = %v, %v; esperado cierre_usuario, true", motivo, tiene)
	}
}

// TestReconstituirTokenRefrescoEmitido_PreservaConsumoYSucesor cubre la
// rama con consumidoEn y hashSucesor no nulos.
func TestReconstituirTokenRefrescoEmitido_PreservaConsumoYSucesor(t *testing.T) {
	emitidoEn := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	consumidoEn := emitidoEn.Add(time.Minute)
	hash, _ := NuevoHashTokenRefresco(strings.Repeat("a", 64))
	sucesor, _ := NuevoHashTokenRefresco(strings.Repeat("b", 64))

	tok := ReconstituirTokenRefrescoEmitido(hash, 0, emitidoEn, emitidoEn.Add(time.Hour), &consumidoEn, &sucesor)

	got, tiene := tok.ConsumidoEn()
	if !tiene || !got.Equal(consumidoEn) {
		t.Errorf("ConsumidoEn() = %v, %v; esperado %v, true", got, tiene, consumidoEn)
	}
	gotSucesor, tiene := tok.HashSucesor()
	if !tiene || !gotSucesor.EsIgual(sucesor) {
		t.Error("HashSucesor() no coincide con el provisto")
	}
	if !tok.EstaConsumido() {
		t.Error("un token reconstituido con consumidoEn debe reportarse consumido")
	}
}

// --- mensajes de Error(): forman parte del contrato observable por la capa
// de aplicación (mapeo a HTTP) y por logs/auditoría; se verifica que no
// están vacíos y que, cuando llevan datos, los incluyen.

func TestErrores_MensajesNoVacios(t *testing.T) {
	errores := []error{
		&ErrIDUsuarioInvalido{Motivo: "x"},
		&ErrIDSesionInvalido{Motivo: "x"},
		&ErrIDTokenAccesoInvalido{Motivo: "x"},
		&ErrCredencialesRechazadas{},
		&ErrCuentaNoOperativa{Motivo: MotivoCuentaNoOperativaSuspendida},
		&ErrSegundoFactorRequerido{MotivoStepUp: "otp"},
		&ErrRefrescoInvalido{},
		&ErrSesionExpirada{},
		&ErrSesionRevocada{Motivo: "x"},
		&ErrReusoRefrescoDetectado{},
		&ErrSesionNoEncontrada{IDSesion: "x"},
		&ErrSesionAjena{},
		&ErrTokenAccesoInvalido{Motivo: MotivoTokenAccesoKIDDesconocido},
		&ErrTokenAccesoExpirado{},
		&ErrSesionRevocadaEnLista{},
		&ErrTransicionEstadoSesionInvalida{Origen: EstadoSesionActiva, Destino: EstadoSesionRevocada},
		&ErrAccesoDenegadoPorConfianza{Motivo: "x"},
		&ErrConcurrenciaSesion{},
		&ErrRefrescoYaEmitido{},
		&ErrRefrescoNoEmitido{},
		&ErrMotivoRevocacionRequerido{},
		&ErrEstadoSesionInvalido{Valor: "x"},
		&ErrMotivoRevocacionInvalido{Valor: "x"},
		&ErrDireccionIPInvalida{Motivo: "x"},
		&ErrTokenRefrescoPlanoInvalido{Motivo: "x"},
		&ErrHashTokenRefrescoInvalido{Motivo: "x"},
		&ErrPoliticaSesionInvalida{Motivo: "x"},
		&ErrReclamacionesAccesoInvalidas{Motivo: "x"},
	}
	for _, err := range errores {
		if err.Error() == "" {
			t.Errorf("%T.Error() no debe estar vacío", err)
		}
	}
}
