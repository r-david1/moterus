package dominio

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func origenDePrueba(t *testing.T) OrigenSolicitud {
	t.Helper()
	o, err := NuevoOrigenSolicitud("203.0.113.10", "agente-de-prueba/1.0", "huella-abc", "req-123")
	if err != nil {
		t.Fatalf("no se pudo construir el OrigenSolicitud de prueba: %v", err)
	}
	return o
}

// politicaCortaDePrueba usa ventanas pequeñas para poder probar expiración
// sin esperar minutos reales, preservando las relaciones exigidas por
// NuevaPoliticaSesion (vidaTokenAcceso ∈ [1,60]min, refresco ≤ inactividad
// ≤ absoluta).
func politicaCortaDePrueba(t *testing.T) PoliticaSesion {
	t.Helper()
	p, err := NuevaPoliticaSesion(10*time.Minute, time.Hour, 2*time.Hour, 3*time.Hour, time.Minute, 10)
	if err != nil {
		t.Fatalf("no se pudo construir la política de prueba: %v", err)
	}
	return p
}

func hashRefrescoDePrueba(t *testing.T, relleno string) HashTokenRefresco {
	t.Helper()
	h, err := NuevoHashTokenRefresco(strings.Repeat(relleno, 64))
	if err != nil {
		t.Fatalf("no se pudo construir el hash de refresco de prueba: %v", err)
	}
	return h
}

func sesionDePrueba(t *testing.T) (*Sesion, time.Time, PoliticaSesion) {
	t.Helper()
	ahora := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	politica := politicaCortaDePrueba(t)
	s, err := IniciarSesion(idSesionDePrueba(t), idUsuarioDePrueba(t), origenDePrueba(t), ahora, politica)
	if err != nil {
		t.Fatalf("no se pudo iniciar la sesión de prueba: %v", err)
	}
	return s, ahora, politica
}

func sesionConRefrescoDePrueba(t *testing.T) (*Sesion, time.Time, PoliticaSesion) {
	t.Helper()
	s, ahora, politica := sesionDePrueba(t)
	if err := s.EmitirPrimerRefresco(hashRefrescoDePrueba(t, "a"), ahora, politica); err != nil {
		t.Fatalf("no se pudo emitir el primer refresco: %v", err)
	}
	s.EventosPendientes() // limpia SesionIniciada para que los tests de eventos empiecen en limpio
	return s, ahora, politica
}

// --- IniciarSesion / INV-ACC-01, INV-ACC-02 --------------------------------

// TestINV_ACC_01_NaceConUsuarioYOrigen verifica que una Sesion siempre nace
// con un IDUsuario no vacío y con el OrigenSolicitud de su creación: no hay
// sesiones anónimas ni sin procedencia forense.
func TestINV_ACC_01_NaceConUsuarioYOrigen(t *testing.T) {
	s, _, _ := sesionDePrueba(t)
	if s.UsuarioID().EsVacio() {
		t.Error("UsuarioID() no debe estar vacío")
	}
	if s.OrigenCreacion().IP().EsVacia() {
		t.Error("OrigenCreacion().IP() no debe estar vacía en este caso de prueba")
	}
	if s.OrigenCreacion().IDSolicitud() != "req-123" {
		t.Errorf("OrigenCreacion().IDSolicitud() = %q, esperado req-123", s.OrigenCreacion().IDSolicitud())
	}
}

func TestIniciarSesion_RechazaIDSesionVacio(t *testing.T) {
	var idVacio IDSesion
	_, err := IniciarSesion(idVacio, idUsuarioDePrueba(t), origenDePrueba(t), time.Now(), politicaCortaDePrueba(t))
	if err == nil {
		t.Fatal("se esperaba error al iniciar sesión con IDSesion vacío")
	}
	var errID *ErrIDSesionInvalido
	if !errors.As(err, &errID) {
		t.Errorf("se esperaba *ErrIDSesionInvalido, obtuvo %T", err)
	}
}

func TestIniciarSesion_RechazaIDUsuarioVacio(t *testing.T) {
	var idVacio IDUsuario
	_, err := IniciarSesion(idSesionDePrueba(t), idVacio, origenDePrueba(t), time.Now(), politicaCortaDePrueba(t))
	if err == nil {
		t.Fatal("se esperaba error al iniciar sesión con IDUsuario vacío")
	}
}

func TestIniciarSesion_NaceActivaConGeneracionCero(t *testing.T) {
	s, ahora, _ := sesionDePrueba(t)
	if !s.Estado().EsIgual(EstadoSesionActiva) {
		t.Errorf("Estado() = %s, esperado activa", s.Estado())
	}
	if s.Generacion() != 0 {
		t.Errorf("Generacion() = %d, esperado 0", s.Generacion())
	}
	if !s.CreadaEn().Equal(ahora) {
		t.Errorf("CreadaEn() = %v, esperado %v", s.CreadaEn(), ahora)
	}
	if !s.ActualizadaEn().Equal(ahora) {
		t.Errorf("ActualizadaEn() = %v, esperado %v (INV-ACC-10)", s.ActualizadaEn(), ahora)
	}
	if _, tiene := s.RefrescoVigente(); tiene {
		t.Error("una sesión recién iniciada todavía no debe tener refresco vigente")
	}
}

func TestIniciarSesion_CalculaVentanasDeExpiracion(t *testing.T) {
	s, ahora, politica := sesionDePrueba(t)
	esperadoAbsoluto := ahora.Add(politica.VidaAbsolutaSesion())
	if !s.ExpiraAbsolutoEn().Equal(esperadoAbsoluto) {
		t.Errorf("ExpiraAbsolutoEn() = %v, esperado %v", s.ExpiraAbsolutoEn(), esperadoAbsoluto)
	}
	esperadoInactividad := ahora.Add(politica.InactividadMaxima())
	if !s.ExpiraInactividadEn().Equal(esperadoInactividad) {
		t.Errorf("ExpiraInactividadEn() = %v, esperado %v", s.ExpiraInactividadEn(), esperadoInactividad)
	}
}

func TestIniciarSesion_AcumulaSesionIniciada(t *testing.T) {
	s, ahora, politica := sesionDePrueba(t)
	eventos := s.EventosPendientes()
	if len(eventos) != 1 {
		t.Fatalf("se esperaba 1 evento acumulado, hay %d", len(eventos))
	}
	ev, ok := eventos[0].(SesionIniciada)
	if !ok {
		t.Fatalf("se esperaba un evento SesionIniciada, obtuvo %T", eventos[0])
	}
	if ev.IDUsuario != s.UsuarioID().String() {
		t.Errorf("IDUsuario = %q", ev.IDUsuario)
	}
	esperado := ahora.Add(politica.VidaAbsolutaSesion())
	if !ev.ExpiraAbsolutoEn.Equal(esperado) {
		t.Errorf("ExpiraAbsolutoEn = %v, esperado %v", ev.ExpiraAbsolutoEn, esperado)
	}
}

func TestEventosPendientes_Drena(t *testing.T) {
	s, _, _ := sesionDePrueba(t)
	primero := s.EventosPendientes()
	if len(primero) == 0 {
		t.Fatal("se esperaba al menos un evento en la primera llamada")
	}
	segundo := s.EventosPendientes()
	if len(segundo) != 0 {
		t.Errorf("la segunda llamada a EventosPendientes debe devolver un slice vacío, obtuvo %d elementos", len(segundo))
	}
}

// --- EmitirPrimerRefresco / Rotar / INV-ACC-04, INV-ACC-05 -----------------

func TestEmitirPrimerRefresco_FijaElRefrescoVigenteEnGeneracionCero(t *testing.T) {
	s, ahora, politica := sesionDePrueba(t)
	hash := hashRefrescoDePrueba(t, "a")
	if err := s.EmitirPrimerRefresco(hash, ahora, politica); err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	vigente, ok := s.RefrescoVigente()
	if !ok {
		t.Fatal("se esperaba un refresco vigente tras EmitirPrimerRefresco")
	}
	if vigente.Generacion() != 0 {
		t.Errorf("Generacion() = %d, esperado 0", vigente.Generacion())
	}
	if !vigente.Hash().EsIgual(hash) {
		t.Error("el hash del refresco vigente no coincide con el emitido")
	}
	if vigente.EstaConsumido() {
		t.Error("un refresco recién emitido no debe estar consumido")
	}
}

// TestINV_ACC_04_NoPuedeEmitirseDosVecesElPrimerRefresco verifica que como
// mucho hay un refresco vigente por sesión: llamar EmitirPrimerRefresco dos
// veces sobre el mismo agregado falla.
func TestINV_ACC_04_NoPuedeEmitirseDosVecesElPrimerRefresco(t *testing.T) {
	s, ahora, politica := sesionDePrueba(t)
	if err := s.EmitirPrimerRefresco(hashRefrescoDePrueba(t, "a"), ahora, politica); err != nil {
		t.Fatalf("no se esperaba error en la primera emisión: %v", err)
	}
	err := s.EmitirPrimerRefresco(hashRefrescoDePrueba(t, "b"), ahora, politica)
	if err == nil {
		t.Fatal("se esperaba error al emitir un segundo refresco inicial (INV-ACC-04)")
	}
	var errYaEmitido *ErrRefrescoYaEmitido
	if !errors.As(err, &errYaEmitido) {
		t.Errorf("se esperaba *ErrRefrescoYaEmitido, obtuvo %T", err)
	}
}

func TestEmitirPrimerRefresco_RechazaHashVacio(t *testing.T) {
	s, ahora, politica := sesionDePrueba(t)
	var hashVacio HashTokenRefresco
	err := s.EmitirPrimerRefresco(hashVacio, ahora, politica)
	if err == nil {
		t.Fatal("se esperaba error al emitir con hash vacío")
	}
	var errHash *ErrHashTokenRefrescoInvalido
	if !errors.As(err, &errHash) {
		t.Errorf("se esperaba *ErrHashTokenRefrescoInvalido, obtuvo %T", err)
	}
}

func TestRotar_RechazaHashVacio(t *testing.T) {
	s, ahora, politica := sesionConRefrescoDePrueba(t)
	var hashVacio HashTokenRefresco
	err := s.Rotar(hashVacio, ahora.Add(time.Minute), politica)
	if err == nil {
		t.Fatal("se esperaba error al rotar con hash vacío")
	}
	var errHash *ErrHashTokenRefrescoInvalido
	if !errors.As(err, &errHash) {
		t.Errorf("se esperaba *ErrHashTokenRefrescoInvalido, obtuvo %T", err)
	}
}

func TestRotar_FallaSiTodaviaNoHayRefrescoEmitido(t *testing.T) {
	s, ahora, politica := sesionDePrueba(t)
	err := s.Rotar(hashRefrescoDePrueba(t, "b"), ahora, politica)
	if err == nil {
		t.Fatal("se esperaba error al rotar sin refresco previo")
	}
	var errNoEmitido *ErrRefrescoNoEmitido
	if !errors.As(err, &errNoEmitido) {
		t.Errorf("se esperaba *ErrRefrescoNoEmitido, obtuvo %T", err)
	}
}

// TestINV_ACC_05_RotarConsumeElAnteriorYEmiteUnoNuevo verifica que cada uso
// de un token de refresco lo consume: no existe la reutilización legítima.
func TestINV_ACC_05_RotarConsumeElAnteriorYEmiteUnoNuevo(t *testing.T) {
	s, ahora, politica := sesionConRefrescoDePrueba(t)
	primerHash, _ := s.RefrescoVigente()

	momentoRotacion := ahora.Add(time.Minute)
	nuevoHash := hashRefrescoDePrueba(t, "b")
	if err := s.Rotar(nuevoHash, momentoRotacion, politica); err != nil {
		t.Fatalf("no se esperaba error al rotar: %v", err)
	}

	consumido, ok := s.RefrescoRecienConsumido()
	if !ok {
		t.Fatal("se esperaba un refresco recién consumido tras Rotar")
	}
	if !consumido.Hash().EsIgual(primerHash.Hash()) {
		t.Error("el refresco consumido debe ser el que era vigente antes de rotar")
	}
	if !consumido.EstaConsumido() {
		t.Error("el refresco anterior debe reportarse consumido tras la rotación")
	}
	consumidoEn, tieneConsumidoEn := consumido.ConsumidoEn()
	if !tieneConsumidoEn || !consumidoEn.Equal(momentoRotacion) {
		t.Errorf("ConsumidoEn() = %v, %v; esperado %v, true", consumidoEn, tieneConsumidoEn, momentoRotacion)
	}
	sucesor, tieneSucesor := consumido.HashSucesor()
	if !tieneSucesor || !sucesor.EsIgual(nuevoHash) {
		t.Error("HashSucesor() debe apuntar al nuevo hash vigente")
	}

	nuevoVigente, ok := s.RefrescoVigente()
	if !ok {
		t.Fatal("se esperaba un nuevo refresco vigente tras rotar")
	}
	if !nuevoVigente.Hash().EsIgual(nuevoHash) {
		t.Error("el nuevo refresco vigente debe tener el hash recién generado")
	}
	if nuevoVigente.Generacion() != 1 {
		t.Errorf("Generacion() del nuevo refresco = %d, esperado 1", nuevoVigente.Generacion())
	}
	if s.Generacion() != 1 {
		t.Errorf("Sesion.Generacion() = %d, esperado 1", s.Generacion())
	}
	renovadaEn, tiene := s.UltimaRenovacionEn()
	if !tiene || !renovadaEn.Equal(momentoRotacion) {
		t.Errorf("UltimaRenovacionEn() = %v, %v; esperado %v, true", renovadaEn, tiene, momentoRotacion)
	}
}

func TestRotar_AcumulaSesionRenovada(t *testing.T) {
	s, ahora, politica := sesionConRefrescoDePrueba(t)
	momento := ahora.Add(time.Minute)
	if err := s.Rotar(hashRefrescoDePrueba(t, "b"), momento, politica); err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	eventos := s.EventosPendientes()
	if len(eventos) != 1 {
		t.Fatalf("se esperaba 1 evento acumulado, hay %d", len(eventos))
	}
	ev, ok := eventos[0].(SesionRenovada)
	if !ok {
		t.Fatalf("se esperaba un evento SesionRenovada, obtuvo %T", eventos[0])
	}
	if ev.Generacion != 1 {
		t.Errorf("Generacion = %d, esperado 1", ev.Generacion)
	}
}

// TestINV_ACC_16_RotarNuncaExtiendeLaVidaAbsoluta verifica que, al rotar
// cerca del límite de la vida absoluta, la nueva ventana de inactividad se
// acota a expiraAbsolutoEn en vez de extenderla.
func TestINV_ACC_16_RotarNuncaExtiendeLaVidaAbsoluta(t *testing.T) {
	s, ahora, politica := sesionConRefrescoDePrueba(t)
	cercaDelLimite := s.ExpiraAbsolutoEn().Add(-time.Minute)
	if err := s.Rotar(hashRefrescoDePrueba(t, "b"), cercaDelLimite, politica); err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if !s.ExpiraInactividadEn().Equal(s.ExpiraAbsolutoEn()) {
		t.Errorf("ExpiraInactividadEn() = %v, esperado que se acote a ExpiraAbsolutoEn() = %v (INV-ACC-16)", s.ExpiraInactividadEn(), s.ExpiraAbsolutoEn())
	}
	if s.ExpiraInactividadEn().After(s.ExpiraAbsolutoEn()) {
		t.Error("ExpiraInactividadEn() nunca debe superar ExpiraAbsolutoEn() (INV-ACC-16)")
	}
	_ = ahora
}

func TestRotar_FallaSiLaSesionNoEstaActiva(t *testing.T) {
	s, ahora, politica := sesionConRefrescoDePrueba(t)
	if err := s.Revocar(MotivoCierreUsuario, ahora); err != nil {
		t.Fatalf("no se esperaba error al revocar: %v", err)
	}
	err := s.Rotar(hashRefrescoDePrueba(t, "b"), ahora.Add(time.Minute), politica)
	if err == nil {
		t.Fatal("se esperaba error al rotar una sesión no activa")
	}
	var errTransicion *ErrTransicionEstadoSesionInvalida
	if !errors.As(err, &errTransicion) {
		t.Errorf("se esperaba *ErrTransicionEstadoSesionInvalida, obtuvo %T", err)
	}
}

// --- Revocar / INV-ACC-07, INV-ACC-08 --------------------------------------

func TestRevocar_TransicionaAEstadoRevocadaConMotivoYMomento(t *testing.T) {
	s, ahora, _ := sesionDePrueba(t)
	momento := ahora.Add(time.Hour)
	if err := s.Revocar(MotivoCuentaNoOperativa, momento); err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if !s.Estado().EsIgual(EstadoSesionRevocada) {
		t.Errorf("Estado() = %s, esperado revocada", s.Estado())
	}
	revocadaEn, tiene := s.RevocadaEn()
	if !tiene || !revocadaEn.Equal(momento) {
		t.Errorf("RevocadaEn() = %v, %v; esperado %v, true", revocadaEn, tiene, momento)
	}
	motivo, tiene := s.MotivoRevocacion()
	if !tiene || !motivo.EsIgual(MotivoCuentaNoOperativa) {
		t.Errorf("MotivoRevocacion() = %v, %v; esperado cuenta_no_operativa, true", motivo, tiene)
	}
	if !s.ActualizadaEn().Equal(momento) {
		t.Errorf("ActualizadaEn() = %v, esperado %v (INV-ACC-10)", s.ActualizadaEn(), momento)
	}
}

// TestINV_ACC_08_RevocarExigeMotivo verifica que toda revocación lleva un
// MotivoRevocacion del catálogo cerrado: no hay revocaciones sin motivo.
func TestINV_ACC_08_RevocarExigeMotivo(t *testing.T) {
	s, ahora, _ := sesionDePrueba(t)
	var motivoVacio MotivoRevocacion
	err := s.Revocar(motivoVacio, ahora)
	if err == nil {
		t.Fatal("se esperaba error al revocar sin motivo (INV-ACC-08)")
	}
	var errMotivo *ErrMotivoRevocacionRequerido
	if !errors.As(err, &errMotivo) {
		t.Errorf("se esperaba *ErrMotivoRevocacionRequerido, obtuvo %T", err)
	}
}

// TestINV_ACC_07_RevocarEsTerminal verifica que revocada es terminal: una
// segunda revocación (o cualquier otra mutación de transición) falla.
func TestINV_ACC_07_RevocarEsTerminal(t *testing.T) {
	s, ahora, _ := sesionDePrueba(t)
	if err := s.Revocar(MotivoCierreUsuario, ahora); err != nil {
		t.Fatalf("no se esperaba error en la primera revocación: %v", err)
	}
	err := s.Revocar(MotivoCierreUsuario, ahora.Add(time.Minute))
	if err == nil {
		t.Fatal("se esperaba error al revocar una sesión ya revocada (INV-ACC-07)")
	}
	if err := s.MarcarExpirada(ahora.Add(time.Minute)); err == nil {
		t.Error("se esperaba error al expirar una sesión ya revocada (INV-ACC-07)")
	}
}

func TestRevocar_NoAcumulaEventoPropio(t *testing.T) {
	// Decisión de diseño documentada en el doc comment de Sesion.Revocar: el
	// agregado no decide por sí mismo entre SesionCerrada, SesionRevocada o
	// ReusoRefrescoDetectado (depende de contexto de orquestación que no
	// tiene), así que no acumula ningún evento propio.
	s, ahora, _ := sesionDePrueba(t)
	s.EventosPendientes() // drena SesionIniciada
	if err := s.Revocar(MotivoCierreUsuario, ahora); err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if eventos := s.EventosPendientes(); len(eventos) != 0 {
		t.Errorf("Revocar no debe acumular eventos propios, se acumularon %d", len(eventos))
	}
}

// --- MarcarExpirada / INV-ACC-07 -------------------------------------------

func TestMarcarExpirada_TransicionaAEstadoExpirada(t *testing.T) {
	s, ahora, _ := sesionDePrueba(t)
	momento := ahora.Add(3 * time.Hour)
	if err := s.MarcarExpirada(momento); err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if !s.Estado().EsIgual(EstadoSesionExpirada) {
		t.Errorf("Estado() = %s, esperado expirada", s.Estado())
	}
	if !s.ActualizadaEn().Equal(momento) {
		t.Errorf("ActualizadaEn() = %v, esperado %v", s.ActualizadaEn(), momento)
	}
}

func TestMarcarExpirada_FallaSiNoEstaActiva(t *testing.T) {
	s, ahora, _ := sesionDePrueba(t)
	if err := s.MarcarExpirada(ahora); err != nil {
		t.Fatalf("no se esperaba error en la primera llamada: %v", err)
	}
	if err := s.MarcarExpirada(ahora.Add(time.Minute)); err == nil {
		t.Error("se esperaba error al expirar una sesión ya expirada (INV-ACC-07)")
	}
}

func TestMarcarExpirada_NoAcumulaEvento(t *testing.T) {
	s, ahora, _ := sesionDePrueba(t)
	s.EventosPendientes()
	if err := s.MarcarExpirada(ahora); err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if eventos := s.EventosPendientes(); len(eventos) != 0 {
		t.Errorf("MarcarExpirada no debe acumular eventos, se acumularon %d", len(eventos))
	}
}

// --- PuedeRenovarse / EstaViva (EvaluadorVentanas, §1.4 del diseño) --------

func TestPuedeRenovarse_PermiteDentroDeLasVentanas(t *testing.T) {
	s, ahora, _ := sesionDePrueba(t)
	if err := s.PuedeRenovarse(ahora.Add(time.Minute)); err != nil {
		t.Errorf("no se esperaba error dentro de las ventanas: %v", err)
	}
	if !s.EstaViva(ahora.Add(time.Minute)) {
		t.Error("EstaViva() debe ser true dentro de las ventanas")
	}
}

// TestPuedeRenovarse_OrdenDeComprobaciones verifica el orden normativo del
// servicio EvaluadorVentanas (§1.4 del diseño): estado ≠ activa gana
// siempre sobre las comprobaciones de tiempo.
func TestPuedeRenovarse_OrdenDeComprobaciones(t *testing.T) {
	s, ahora, _ := sesionDePrueba(t)
	if err := s.Revocar(MotivoCierreUsuario, ahora); err != nil {
		t.Fatalf("no se esperaba error al revocar: %v", err)
	}
	// Aunque además esté fuera de sus ventanas de tiempo, una sesión no
	// activa siempre reporta ErrSesionRevocada, nunca ErrSesionExpirada.
	err := s.PuedeRenovarse(s.ExpiraAbsolutoEn().Add(time.Hour))
	var errRevocada *ErrSesionRevocada
	if !errors.As(err, &errRevocada) {
		t.Errorf("se esperaba *ErrSesionRevocada, obtuvo %T (%v)", err, err)
	}
}

func TestPuedeRenovarse_ExpiraPorVidaAbsoluta(t *testing.T) {
	s, _, _ := sesionDePrueba(t)
	err := s.PuedeRenovarse(s.ExpiraAbsolutoEn().Add(time.Second))
	var errExpirada *ErrSesionExpirada
	if !errors.As(err, &errExpirada) {
		t.Errorf("se esperaba *ErrSesionExpirada, obtuvo %T (%v)", err, err)
	}
	if s.EstaViva(s.ExpiraAbsolutoEn().Add(time.Second)) {
		t.Error("EstaViva() debe ser false tras superar la vida absoluta")
	}
}

func TestPuedeRenovarse_ExpiraPorInactividad(t *testing.T) {
	s, _, _ := sesionDePrueba(t)
	err := s.PuedeRenovarse(s.ExpiraInactividadEn().Add(time.Second))
	var errExpirada *ErrSesionExpirada
	if !errors.As(err, &errExpirada) {
		t.Errorf("se esperaba *ErrSesionExpirada, obtuvo %T (%v)", err, err)
	}
}

func TestPuedeRenovarse_ErrSesionRevocadaLlevaMotivoCuandoAplica(t *testing.T) {
	s, ahora, _ := sesionDePrueba(t)
	if err := s.Revocar(MotivoContrasenaCambiada, ahora); err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	err := s.PuedeRenovarse(ahora.Add(time.Minute))
	var errRevocada *ErrSesionRevocada
	if !errors.As(err, &errRevocada) {
		t.Fatalf("se esperaba *ErrSesionRevocada, obtuvo %T", err)
	}
	if errRevocada.Motivo != "contrasena_cambiada" {
		t.Errorf("Motivo = %q, esperado contrasena_cambiada", errRevocada.Motivo)
	}
}

// --- ReclamacionesParaToken --------------------------------------------------

func TestReclamacionesParaToken_UsaCreadaEnComoAuthTime(t *testing.T) {
	s, ahora, politica := sesionConRefrescoDePrueba(t)
	momentoEmision := ahora.Add(time.Hour)
	r, err := s.ReclamacionesParaToken(idTokenAccesoDePrueba(t), "https://acceso.ejemplo.com", "moterus", momentoEmision, politica)
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if !r.AutenticadoEn().Equal(ahora) {
		t.Errorf("AutenticadoEn() = %v, esperado CreadaEn() = %v", r.AutenticadoEn(), ahora)
	}
	if !r.EmitidoEn().Equal(momentoEmision) {
		t.Errorf("EmitidoEn() = %v, esperado %v", r.EmitidoEn(), momentoEmision)
	}
	esperadoExpira := momentoEmision.Add(politica.VidaTokenAcceso())
	if !r.ExpiraEn().Equal(esperadoExpira) {
		t.Errorf("ExpiraEn() = %v, esperado %v", r.ExpiraEn(), esperadoExpira)
	}
	if !r.IDSesion().EsIgual(s.ID()) {
		t.Error("IDSesion() debe coincidir con la sesión")
	}
	if !r.Sujeto().EsIgual(s.UsuarioID()) {
		t.Error("Sujeto() debe coincidir con el usuario de la sesión")
	}
	if len(r.MetodosAutenticacion()) == 0 {
		t.Error("MetodosAutenticacion() no debe estar vacío")
	}
}

// --- INV-ACC-09: getters devuelven copias, nunca punteros internos --------

func TestINV_ACC_09_GettersDevuelvenCopias(t *testing.T) {
	s, ahora, _ := sesionConRefrescoDePrueba(t)

	// Mutar la copia devuelta por RefrescoVigente() no debe afectar al
	// agregado: el getter debe devolver una copia de valor, nunca un
	// puntero al campo interno.
	vigente, ok := s.RefrescoVigente()
	if !ok {
		t.Fatal("se esperaba un refresco vigente")
	}
	vigente.consumidoEn = &ahora // campo no exportado, accesible por ser white-box test del mismo paquete
	vigenteDeNuevo, _ := s.RefrescoVigente()
	if vigenteDeNuevo.EstaConsumido() {
		t.Error("mutar la copia devuelta por RefrescoVigente() no debe afectar al agregado (INV-ACC-09)")
	}
}

// --- Reconstituir -------------------------------------------------------

func TestReconstituir_NoAcumulaEventos(t *testing.T) {
	ahora := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	hash := hashRefrescoDePrueba(t, "a")
	vigente := ReconstituirTokenRefrescoEmitido(hash, 0, ahora, ahora.Add(time.Hour), nil, nil)
	s := Reconstituir(
		idSesionDePrueba(t),
		idUsuarioDePrueba(t),
		EstadoSesionActiva,
		0,
		&vigente,
		origenDePrueba(t),
		ahora,
		ahora,
		nil,
		ahora.Add(2*time.Hour),
		ahora.Add(90*24*time.Hour),
		nil,
		MotivoRevocacion{},
	)
	if eventos := s.EventosPendientes(); len(eventos) != 0 {
		t.Errorf("Reconstituir no debe acumular eventos, se acumularon %d", len(eventos))
	}
	got, ok := s.RefrescoVigente()
	if !ok || !got.Hash().EsIgual(hash) {
		t.Error("Reconstituir debe preservar el refresco vigente provisto")
	}
	if _, tiene := s.MotivoRevocacion(); tiene {
		t.Error("una sesión reconstituida sin motivo de revocación no debe reportar uno")
	}
}
