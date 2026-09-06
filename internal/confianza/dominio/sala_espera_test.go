package dominio

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

// --- AliasSala -------------------------------------------------------------

func TestNuevoAliasSala_NormalizaYValida(t *testing.T) {
	a, err := NuevoAliasSala("  Inscripciones-2026  ")
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if a.Normalizado() != "inscripciones-2026" {
		t.Errorf("Normalizado() = %q", a.Normalizado())
	}
}

func TestNuevoAliasSala_RechazaCasosInvalidos(t *testing.T) {
	casos := []string{
		"",
		"ab",
		strings.Repeat("a", 49),
		"-empieza-con-guion",
		"termina-con-guion-",
		"doble--guion",
		"Mayus_culas_no_permitidas",
	}
	for _, c := range casos {
		if _, err := NuevoAliasSala(c); err == nil {
			t.Errorf("NuevoAliasSala(%q) = nil, se esperaba error", c)
		}
	}
}

func TestAliasSala_EsIgualYEsVacio(t *testing.T) {
	var a AliasSala
	if !a.EsVacio() {
		t.Error("el zero value debería estar vacío")
	}
	b, _ := NuevoAliasSala("evento-x")
	c, _ := NuevoAliasSala("EVENTO-X")
	if !b.EsIgual(c) {
		t.Error("misma forma normalizada debería ser igual")
	}
	if b.String() != "evento-x" {
		t.Errorf("String() = %q", b.String())
	}
}

// --- construcción del agregado ----------------------------------------

func aliasDePrueba(t *testing.T) AliasSala {
	t.Helper()
	a, err := NuevoAliasSala("inscripciones-2026")
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	return a
}

func TestNuevaSalaDeEspera_ConstruyeEnProgramada(t *testing.T) {
	ahora := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	s, err := NuevaSalaDeEspera(idSalaDePrueba(t), aliasDePrueba(t), AlcanceSistema(), RutaAccesoIniciarSesion, PoliticaSalaPorDefecto(), nil, ahora)
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if !s.Estado().EsIgual(EstadoSalaProgramada) {
		t.Errorf("Estado() = %v, esperado programada", s.Estado())
	}
	if len(s.EventosPendientes()) != 0 {
		t.Error("la construcción no debería acumular eventos")
	}
	if _, ok := s.CreadaPor(); ok {
		t.Error("una sala de alcance sistema no debería tener CreadaPor")
	}
}

func TestNuevaSalaDeEspera_RutaNoAdmiteAlcance(t *testing.T) {
	idOrg := idOrganizacionDePrueba(t)
	org, _ := AlcanceOrganizacion(idOrg)
	idUsuario, _ := IDUsuarioDesde("018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b6000")
	_, err := NuevaSalaDeEspera(idSalaDePrueba(t), aliasDePrueba(t), org, RutaAccesoIniciarSesion, PoliticaSalaPorDefecto(), &idUsuario, time.Now())
	if err == nil {
		t.Fatal("se esperaba ErrRutaNoProtegible: acceso.iniciar_sesion no admite alcance organizacion")
	}
	var errRuta *ErrRutaNoProtegible
	if !errors.As(err, &errRuta) {
		t.Errorf("se esperaba *ErrRutaNoProtegible, obtuvo %T", err)
	}
}

// TestNuevaSalaDeEspera_ExigeCreadaPorParaAlcanceOrganizacion verifica que
// una sala de alcance organizacion sin creadaPor se rechaza por esa razón
// específica (ErrIDUsuarioInvalido), independientemente de si la ruta
// admite o no ese alcance — mismo criterio que
// Membresia.AgregarMiembro exige otorgadaPor en tenencia/dominio. La
// validación se ejecuta antes que RutaProtegida.AdmiteAlcance a propósito,
// para que ambas reglas sean observables por separado aunque, hoy, ninguna
// ruta del catálogo (§1.6) admita alcance organizacion.
func TestNuevaSalaDeEspera_ExigeCreadaPorParaAlcanceOrganizacion(t *testing.T) {
	idOrg := idOrganizacionDePrueba(t)
	org, _ := AlcanceOrganizacion(idOrg)
	_, err := NuevaSalaDeEspera(idSalaDePrueba(t), aliasDePrueba(t), org, RutaTenenciaAceptarInvitacion, PoliticaSalaPorDefecto(), nil, time.Now())
	if err == nil {
		t.Fatal("se esperaba ErrIDUsuarioInvalido: creadaPor es obligatorio para alcance organizacion")
	}
	var errID *ErrIDUsuarioInvalido
	if !errors.As(err, &errID) {
		t.Errorf("se esperaba *ErrIDUsuarioInvalido, obtuvo %T", err)
	}

	idUsuario, _ := IDUsuarioDesde("018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b6000")
	_, err2 := NuevaSalaDeEspera(idSalaDePrueba(t), aliasDePrueba(t), org, RutaTenenciaAceptarInvitacion, PoliticaSalaPorDefecto(), &idUsuario, time.Now())
	if err2 == nil {
		t.Fatal("se esperaba ErrRutaNoProtegible: ninguna ruta del catálogo actual admite alcance organizacion")
	}
	var errRuta *ErrRutaNoProtegible
	if !errors.As(err2, &errRuta) {
		t.Errorf("se esperaba *ErrRutaNoProtegible, obtuvo %T", err2)
	}
}

func TestNuevaSalaDeEspera_ValidaCamposObligatorios(t *testing.T) {
	ahora := time.Now()
	politica := PoliticaSalaPorDefecto()
	if _, err := NuevaSalaDeEspera(IDSalaDeEspera{}, aliasDePrueba(t), AlcanceSistema(), RutaAccesoIniciarSesion, politica, nil, ahora); err == nil {
		t.Error("se esperaba error con id vacío")
	}
	if _, err := NuevaSalaDeEspera(idSalaDePrueba(t), AliasSala{}, AlcanceSistema(), RutaAccesoIniciarSesion, politica, nil, ahora); err == nil {
		t.Error("se esperaba error con alias vacío")
	}
	if _, err := NuevaSalaDeEspera(idSalaDePrueba(t), aliasDePrueba(t), AlcanceSala{}, RutaAccesoIniciarSesion, politica, nil, ahora); err == nil {
		t.Error("se esperaba error con alcance vacío")
	}
	if _, err := NuevaSalaDeEspera(idSalaDePrueba(t), aliasDePrueba(t), AlcanceSistema(), RutaProtegida{}, politica, nil, ahora); err == nil {
		t.Error("se esperaba error con ruta vacía")
	}
	if _, err := NuevaSalaDeEspera(idSalaDePrueba(t), aliasDePrueba(t), AlcanceSistema(), RutaAccesoIniciarSesion, PoliticaSala{}, nil, ahora); err == nil {
		t.Error("se esperaba error con política vacía")
	}
}

// --- Abrir / máquina de estados del agregado -------------------------------

func TestSalaDeEspera_Abrir_PrimeraVez(t *testing.T) {
	ahora := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	s, _ := NuevaSalaDeEspera(idSalaDePrueba(t), aliasDePrueba(t), AlcanceSistema(), RutaAccesoIniciarSesion, PoliticaSalaPorDefecto(), nil, ahora)

	if err := s.Abrir(ahora); err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if !s.Estado().EsIgual(EstadoSalaAbierta) {
		t.Errorf("Estado() = %v, esperado abierta", s.Estado())
	}
	if s.CursorBase() != 0 {
		t.Errorf("CursorBase() = %d, esperado 0", s.CursorBase())
	}
	if !s.RelojDesde().Equal(ahora) {
		t.Errorf("RelojDesde() = %v, esperado %v", s.RelojDesde(), ahora)
	}
	abiertaEn, ok := s.AbiertaEn()
	if !ok || !abiertaEn.Equal(ahora) {
		t.Errorf("AbiertaEn() = %v, %v", abiertaEn, ok)
	}

	eventos := s.EventosPendientes()
	if len(eventos) != 1 {
		t.Fatalf("se esperaba 1 evento, hay %d", len(eventos))
	}
	ev, ok := eventos[0].(SalaDeEsperaAbierta)
	if !ok {
		t.Fatalf("se esperaba SalaDeEsperaAbierta, obtuvo %T", eventos[0])
	}
	if ev.Alias != "inscripciones-2026" || ev.Alcance != "sistema" || ev.Ruta != RutaAccesoIniciarSesion.String() {
		t.Errorf("evento con datos incorrectos: %+v", ev)
	}
}

// TestINV_COLA_11_CicloDeVida_AcumulaExactamenteUnEventoPorMutacion
// verifica INV-COLA-11: toda mutación del ciclo de vida de una sala
// (abrir, cambiar ritmo, drenar, cerrar) acumula exactamente un evento de
// dominio, listo para auditarse en la misma unidad de trabajo que la
// escritura; ingresos, consultas y reclamos —que no pasan por
// SalaDeEspera, sino por TicketDeCola— no acumulan ninguno (§1.7 del
// diseño): TicketDeCola ni siquiera tiene un método EventosPendientes.
func TestINV_COLA_11_CicloDeVida_AcumulaExactamenteUnEventoPorMutacion(t *testing.T) {
	ahora := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	s, _ := NuevaSalaDeEspera(idSalaDePrueba(t), aliasDePrueba(t), AlcanceSistema(), RutaAccesoIniciarSesion, PoliticaSalaPorDefecto(), nil, ahora)

	if err := s.Abrir(ahora); err != nil {
		t.Fatalf("Abrir: no se esperaba error: %v", err)
	}
	if eventos := s.EventosPendientes(); len(eventos) != 1 {
		t.Fatalf("Abrir: se esperaba 1 evento, hay %d", len(eventos))
	}

	nuevoRitmo, _ := NuevoRitmoAdmision(120)
	if err := s.CambiarRitmo(nuevoRitmo, 4200, ahora.Add(time.Minute)); err != nil {
		t.Fatalf("CambiarRitmo: no se esperaba error: %v", err)
	}
	eventosRitmo := s.EventosPendientes()
	if len(eventosRitmo) != 1 {
		t.Fatalf("CambiarRitmo: se esperaba 1 evento, hay %d", len(eventosRitmo))
	}
	if _, ok := eventosRitmo[0].(RitmoDeAdmisionCambiado); !ok {
		t.Fatalf("se esperaba RitmoDeAdmisionCambiado, obtuvo %T", eventosRitmo[0])
	}

	if err := s.Drenar(10000, 8000, ahora.Add(2*time.Minute)); err != nil {
		t.Fatalf("Drenar: no se esperaba error: %v", err)
	}
	eventosDrenar := s.EventosPendientes()
	if len(eventosDrenar) != 1 {
		t.Fatalf("Drenar: se esperaba 1 evento, hay %d", len(eventosDrenar))
	}
	evDrenar, ok := eventosDrenar[0].(SalaDeEsperaCerrada)
	if !ok || evDrenar.Destino != EstadoSalaDrenando.String() {
		t.Fatalf("se esperaba SalaDeEsperaCerrada{Destino: drenando}, obtuvo %+v", eventosDrenar[0])
	}

	if err := s.Cerrar(10000, 9500, ahora.Add(3*time.Minute)); err != nil {
		t.Fatalf("Cerrar: no se esperaba error: %v", err)
	}
	eventosCerrar := s.EventosPendientes()
	if len(eventosCerrar) != 1 {
		t.Fatalf("Cerrar: se esperaba 1 evento, hay %d", len(eventosCerrar))
	}
	evCerrar, ok := eventosCerrar[0].(SalaDeEsperaCerrada)
	if !ok || evCerrar.Destino != EstadoSalaCerrada.String() {
		t.Fatalf("se esperaba SalaDeEsperaCerrada{Destino: cerrada}, obtuvo %+v", eventosCerrar[0])
	}
	if evCerrar.IngresosTotales != 10000 || evCerrar.AdmitidosTotales != 9500 {
		t.Errorf("totales incorrectos: %+v", evCerrar)
	}
}

func TestSalaDeEspera_Abrir_TransicionInvalidaDesdeAbierta(t *testing.T) {
	ahora := time.Now()
	s := salaAbiertaDePrueba(t, 50, ahora)
	err := s.Abrir(ahora)
	if err == nil {
		t.Fatal("se esperaba ErrTransicionEstadoSalaInvalida: abierta no puede volver a abrirse")
	}
	var errTrans *ErrTransicionEstadoSalaInvalida
	if !errors.As(err, &errTrans) {
		t.Errorf("se esperaba *ErrTransicionEstadoSalaInvalida, obtuvo %T", err)
	}
}

func TestSalaDeEspera_Cerrar_EsTerminal(t *testing.T) {
	ahora := time.Now()
	s := salaAbiertaDePrueba(t, 50, ahora)
	if _, ok := s.CerradaEn(); ok {
		t.Error("una sala abierta no debería tener CerradaEn")
	}
	if err := s.Cerrar(0, 0, ahora); err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if cerradaEn, ok := s.CerradaEn(); !ok || !cerradaEn.Equal(ahora) {
		t.Errorf("CerradaEn() = %v, %v; esperado %v, true", cerradaEn, ok, ahora)
	}
	if err := s.Abrir(ahora); err == nil {
		t.Error("una sala cerrada no debería poder reabrirse")
	}
	if err := s.Cerrar(0, 0, ahora); err == nil {
		t.Error("una sala cerrada no debería poder volver a cerrarse")
	}
	if err := s.Drenar(0, 0, ahora); err == nil {
		t.Error("una sala cerrada no debería poder drenarse")
	}
}

func TestSalaDeEspera_Reapertura_DesdeDrenando_PreservaContinuidad(t *testing.T) {
	ahora := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	s := salaAbiertaDePrueba(t, 10, ahora) // 10/s

	momentoDrenar := ahora.Add(10 * time.Second)
	cursorAlDrenar := s.CursorEn(momentoDrenar) // 100
	if err := s.Drenar(0, 0, momentoDrenar); err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	s.EventosPendientes()

	// Reabrir 5s más tarde: el cursor NO debe saltar hacia atrás ni
	// reiniciarse a 0; debe continuar desde donde el reloj lo hubiera
	// llevado durante el drenaje (mismo criterio de continuidad que
	// CambiarRitmo, §1.5 del diseño).
	momentoReabrir := momentoDrenar.Add(5 * time.Second)
	cursorEsperado := cursorAlDrenar + 50 // 5s * 10/s
	if err := s.Abrir(momentoReabrir); err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if s.CursorBase() != cursorEsperado {
		t.Errorf("CursorBase() tras reabrir = %d, esperado %d (continuidad)", s.CursorBase(), cursorEsperado)
	}
	if s.CursorEn(momentoReabrir) != cursorEsperado {
		t.Errorf("CursorEn(momentoReabrir) = %d, esperado %d", s.CursorEn(momentoReabrir), cursorEsperado)
	}
}

// --- CambiarRitmo -----------------------------------------------------

func TestSalaDeEspera_CambiarRitmo_NoOpIdempotente(t *testing.T) {
	ahora := time.Now()
	s := salaAbiertaDePrueba(t, 50, ahora)
	mismoRitmo, _ := NuevoRitmoAdmision(50)
	if err := s.CambiarRitmo(mismoRitmo, 1000, ahora); err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if eventos := s.EventosPendientes(); len(eventos) != 0 {
		t.Errorf("cambiar al mismo ritmo no debería acumular eventos, hay %d", len(eventos))
	}
}

func TestSalaDeEspera_CambiarRitmo_RequiereSalaVigente(t *testing.T) {
	ahora := time.Now()
	s, _ := NuevaSalaDeEspera(idSalaDePrueba(t), aliasDePrueba(t), AlcanceSistema(), RutaAccesoIniciarSesion, PoliticaSalaPorDefecto(), nil, ahora)
	nuevoRitmo, _ := NuevoRitmoAdmision(80)
	err := s.CambiarRitmo(nuevoRitmo, 0, ahora)
	if err == nil {
		t.Fatal("se esperaba ErrSalaNoVigente: una sala programada no puede cambiar de ritmo")
	}
	var errVig *ErrSalaNoVigente
	if !errors.As(err, &errVig) {
		t.Errorf("se esperaba *ErrSalaNoVigente, obtuvo %T", err)
	}
}

func TestSalaDeEspera_CambiarRitmo_PermitidoDurranteElDrenaje(t *testing.T) {
	ahora := time.Now()
	s := salaAbiertaDePrueba(t, 50, ahora)
	if err := s.Drenar(0, 0, ahora); err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	s.EventosPendientes()
	nuevoRitmo, _ := NuevoRitmoAdmision(20)
	if err := s.CambiarRitmo(nuevoRitmo, 500, ahora.Add(time.Second)); err != nil {
		t.Fatalf("no se esperaba error: cambiar el ritmo durante el drenaje es válido (los tickets vivos siguen avanzando): %v", err)
	}
}

// --- AdmiteIngreso ------------------------------------------------------

func TestSalaDeEspera_AdmiteIngreso_SoloEnAbierta(t *testing.T) {
	ahora := time.Now()
	s, _ := NuevaSalaDeEspera(idSalaDePrueba(t), aliasDePrueba(t), AlcanceSistema(), RutaAccesoIniciarSesion, PoliticaSalaPorDefecto(), nil, ahora)
	if err := s.AdmiteIngreso(0, ahora); err == nil {
		t.Error("una sala programada no debería admitir ingresos")
	}
	if err := s.Abrir(ahora); err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if err := s.AdmiteIngreso(0, ahora); err != nil {
		t.Errorf("una sala recién abierta y vacía debería admitir ingresos: %v", err)
	}
	if err := s.Drenar(0, 0, ahora); err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if err := s.AdmiteIngreso(0, ahora); err == nil {
		t.Error("una sala drenando no debería admitir ingresos nuevos")
	}
}

func TestSalaDeEspera_AdmiteIngreso_ColaLlena(t *testing.T) {
	ahora := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	ritmo, _ := NuevoRitmoAdmision(1)
	politica, _ := NuevaPoliticaSala(ritmo, 100, 2*time.Minute, ModoDegradadoPermitir)
	s, _ := NuevaSalaDeEspera(idSalaDePrueba(t), aliasDePrueba(t), AlcanceSistema(), RutaAccesoIniciarSesion, politica, nil, ahora)
	if err := s.Abrir(ahora); err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	s.EventosPendientes()

	// cursor(ahora) = 0. longitud=100 significa 100 asignados y 0 admitidos
	// todavía: longitud - cursor = 100 >= capacidad (100) => cola llena.
	err := s.AdmiteIngreso(100, ahora)
	if err == nil {
		t.Fatal("se esperaba ErrColaLlena")
	}
	var errLlena *ErrColaLlena
	if !errors.As(err, &errLlena) {
		t.Errorf("se esperaba *ErrColaLlena, obtuvo %T", err)
	}
	// Con longitud=99, todavía hay cupo.
	if err := s.AdmiteIngreso(99, ahora); err != nil {
		t.Errorf("no se esperaba error con longitud=99: %v", err)
	}
}

// --- aritmética del cursor / turno (§1.5 del diseño) -----------------------

// TestINV_COLA_04_CursorEn_NuncaExcedeElRitmoAdmision verifica INV-COLA-04:
// el cursor jamás avanza más rápido que ritmoAdmision admisiones por
// segundo. Se pasa "ahora" como parámetro, mismo patrón que INV-TEN-06 se
// prueba pasando el conteo de propietarios (§1.5 y §11 del diseño).
func TestINV_COLA_04_CursorEn_NuncaExcedeElRitmoAdmision(t *testing.T) {
	ahora := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	s := salaAbiertaDePrueba(t, 50, ahora) // 50 admisiones/segundo

	casos := []struct {
		transcurrido time.Duration
		esperado     int64
	}{
		{0, 0},
		{time.Second, 50},
		{10 * time.Second, 500},
		{1500 * time.Millisecond, 75},
		{999 * time.Millisecond, 49}, // floor(999*50/1000) = floor(49.95) = 49
	}
	for _, c := range casos {
		obtenido := s.CursorEn(ahora.Add(c.transcurrido))
		if obtenido != c.esperado {
			t.Errorf("CursorEn(+%v) = %d, esperado %d", c.transcurrido, obtenido, c.esperado)
		}
		// El cursor nunca debe exceder ritmo * segundos transcurridos.
		limiteSuperior := int64(c.transcurrido.Seconds() * 50)
		if obtenido > limiteSuperior {
			t.Errorf("CursorEn(+%v) = %d excede el límite de %d admisiones/segundo (INV-COLA-04)", c.transcurrido, obtenido, limiteSuperior)
		}
	}
}

// TestINV_COLA_04_CursorEn_EsMonotonicoNoDecreciente refuerza INV-COLA-04
// desde otro ángulo: el cursor nunca retrocede a medida que pasa el tiempo,
// así que el sistema protegido nunca "recupera" cupo ya consumido.
func TestINV_COLA_04_CursorEn_EsMonotonicoNoDecreciente(t *testing.T) {
	ahora := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	s := salaAbiertaDePrueba(t, 7, ahora)
	anterior := s.CursorEn(ahora)
	for i := 1; i <= 100; i++ {
		actual := s.CursorEn(ahora.Add(time.Duration(i) * 137 * time.Millisecond))
		if actual < anterior {
			t.Fatalf("el cursor retrocedió: %d -> %d", anterior, actual)
		}
		anterior = actual
	}
}

// TestSalaDeEspera_TurnoDe_RangoYaAlcanzado cubre la rama delta<=0 de
// TurnoDe: un rango que ya está por detrás (o justo en) cursorBase tiene su
// turno en relojDesde, sin necesidad de calcular nada más — ya está en
// condiciones de admitirse.
func TestSalaDeEspera_TurnoDe_RangoYaAlcanzado(t *testing.T) {
	ahora := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	s := salaAbiertaDePrueba(t, 10, ahora)
	momentoDrenar := ahora.Add(10 * time.Second) // cursorBase pasa a 100 al reabrir
	if err := s.Drenar(0, 0, momentoDrenar); err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	s.EventosPendientes()
	if err := s.Abrir(momentoDrenar); err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	s.EventosPendientes()

	rangoYaAlcanzado, _ := NuevoRango(50) // 50 < cursorBase (100)
	turno := s.TurnoDe(rangoYaAlcanzado)
	if !turno.Equal(s.RelojDesde()) {
		t.Errorf("TurnoDe(rango ya alcanzado) = %v, esperado RelojDesde() (%v)", turno, s.RelojDesde())
	}
}

func TestSalaDeEspera_CursorEn_AhoraAntesDeRelojDesde_NoEsNegativo(t *testing.T) {
	ahora := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	s := salaAbiertaDePrueba(t, 50, ahora)
	obtenido := s.CursorEn(ahora.Add(-time.Hour))
	if obtenido != s.CursorBase() {
		t.Errorf("CursorEn con ahora anterior a relojDesde = %d, esperado cursorBase (%d)", obtenido, s.CursorBase())
	}
}

// TestINV_COLA_05_TurnoDe_EsDeterministaYNoDependeDeAhora verifica
// INV-COLA-05: el turno de un ticket es función determinista de su rango y
// de la configuración vigente — llamar TurnoDe múltiples veces con
// distintos "ahora" (que ni siquiera es un parámetro de la función) siempre
// da el mismo resultado mientras la configuración de la sala no cambie.
func TestINV_COLA_05_TurnoDe_EsDeterministaYNoDependeDeAhora(t *testing.T) {
	ahora := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	s := salaAbiertaDePrueba(t, 25, ahora)
	rango, _ := NuevoRango(1000)

	primero := s.TurnoDe(rango)
	segundo := s.TurnoDe(rango)
	if !primero.Equal(segundo) {
		t.Errorf("TurnoDe debería ser determinista: %v != %v", primero, segundo)
	}

	esperado := ahora.Add(40 * time.Second) // 1000/25 = 40s
	if !primero.Equal(esperado) {
		t.Errorf("TurnoDe(1000) = %v, esperado %v", primero, esperado)
	}
}

// TestINV_COLA_05_EtaNuncaEmpeora_SalvoReduccionExplicitaDelRitmo verifica
// la consecuencia observable de INV-COLA-05: aumentar el ritmo de admisión
// (o mantenerlo) nunca empeora la ETA de un ticket pendiente; solo una
// reducción explícita y auditada del ritmo (§3.2 del diseño) puede
// empeorarla, y ese caso se documenta como la única excepción.
func TestINV_COLA_05_EtaNuncaEmpeora_SalvoReduccionExplicitaDelRitmo(t *testing.T) {
	ahora := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	s := salaAbiertaDePrueba(t, 10, ahora)
	rango, _ := NuevoRango(10000) // muy por delante del cursor

	etaAntes := s.TurnoDe(rango)

	// Subir el ritmo: la ETA debe mejorar (o mantenerse), nunca empeorar.
	ritmoMayor, _ := NuevoRitmoAdmision(50)
	momentoCambio := ahora.Add(5 * time.Second)
	if err := s.CambiarRitmo(ritmoMayor, 1000, momentoCambio); err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	s.EventosPendientes()
	etaDespuesDeSubir := s.TurnoDe(rango)
	if etaDespuesDeSubir.After(etaAntes) {
		t.Errorf("subir el ritmo no debería empeorar la ETA: antes=%v, después=%v", etaAntes, etaDespuesDeSubir)
	}

	// Bajar el ritmo: es la única excepción documentada por la que la ETA
	// puede empeorar, y debe quedar auditada en el evento correspondiente
	// (verificado en TestINV_COLA_11).
	ritmoMenor, _ := NuevoRitmoAdmision(5)
	momentoCambio2 := momentoCambio.Add(time.Second)
	if err := s.CambiarRitmo(ritmoMenor, 1000, momentoCambio2); err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	eventos := s.EventosPendientes()
	if len(eventos) != 1 {
		t.Fatalf("se esperaba 1 evento auditable por el cambio de ritmo, hay %d", len(eventos))
	}
	etaDespuesDeBajar := s.TurnoDe(rango)
	if !etaDespuesDeBajar.After(etaDespuesDeSubir) {
		t.Errorf("bajar el ritmo debería empeorar la ETA en este escenario: antes=%v, después=%v", etaDespuesDeSubir, etaDespuesDeBajar)
	}
}

// TestINV_COLA_13_ReconstituirConCursorBaseCero_ArrancaSinAdmitirDeGolpe
// verifica la mitad de dominio de INV-COLA-13: cuando Redis perdió el
// estado y el reconciliador reproyecta con cursorBase=0 y relojDesde=ahora
// (mismos valores que produce una primera apertura), el cursor arranca en
// cero — no en un valor gigante que admitiría de golpe a toda una cola que
// Redis ya no recuerda. La reproyección real (HSETNX en el adaptador Redis,
// §3.7 y §6.2 del diseño) es responsabilidad de la capa de aplicación e
// infraestructura; lo que el dominio garantiza es que, dados esos dos
// valores, CursorEn(ahora) es 0 y crece únicamente al ritmo configurado a
// partir de ese instante.
func TestINV_COLA_13_ReconstituirConCursorBaseCero_ArrancaSinAdmitirDeGolpe(t *testing.T) {
	ahora := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	politica := PoliticaSalaPorDefecto() // 50/s
	s := ReconstituirSalaDeEspera(
		idSalaDePrueba(t), aliasDePrueba(t), AlcanceSistema(), RutaAccesoIniciarSesion,
		EstadoSalaAbierta, politica,
		0, ahora, // cursorBase=0, relojDesde=ahora: el reinicio de INV-COLA-13
		nil, ahora, nil, nil,
	)
	if s.CursorEn(ahora) != 0 {
		t.Errorf("CursorEn(ahora) tras el reinicio = %d, esperado 0: no debe admitir de golpe una cola que Redis ya no recuerda", s.CursorEn(ahora))
	}
	// Un ticket con rango muy alto (de la cola perdida) NO debería quedar
	// admitido instantáneamente: su turno se recalcula desde cero.
	rangoAlto, _ := NuevoRango(1_000_000)
	turno := s.TurnoDe(rangoAlto)
	if !turno.After(ahora) {
		t.Errorf("TurnoDe(rango alto) tras el reinicio = %v, debería quedar en el futuro respecto de %v", turno, ahora)
	}
	if s.CursorEn(ahora.Add(time.Second)) != 50 {
		t.Errorf("el cursor debe seguir creciendo al ritmo configurado tras el reinicio: CursorEn(+1s) = %d, esperado 50", s.CursorEn(ahora.Add(time.Second)))
	}
}

// TestINV_COLA_12_ReconstituirSalaDeEspera_EsFielAPostgres verifica la
// mitad de dominio de INV-COLA-12 ("Postgres es la fuente de verdad de la
// configuración"): reconstruir el agregado a partir únicamente de los
// campos de la fila persistida reproduce el mismo comportamiento
// observable (mismo cursor, mismo turno) que el agregado original, sin
// ninguna dependencia de Redis.
func TestINV_COLA_12_ReconstituirSalaDeEspera_EsFielAPostgres(t *testing.T) {
	ahora := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	original := salaAbiertaDePrueba(t, 33, ahora)
	momentoCambio := ahora.Add(7 * time.Second)
	nuevoRitmo, _ := NuevoRitmoAdmision(60)
	if err := original.CambiarRitmo(nuevoRitmo, 200, momentoCambio); err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	original.EventosPendientes()

	abiertaEn, _ := original.AbiertaEn()
	reconstituida := ReconstituirSalaDeEspera(
		original.ID(), original.Alias(), original.Alcance(), original.Ruta(),
		original.Estado(), original.Politica(), original.CursorBase(), original.RelojDesde(),
		nil, original.CreadaEn(), &abiertaEn, nil,
	)

	momentoConsulta := momentoCambio.Add(3 * time.Second)
	if reconstituida.CursorEn(momentoConsulta) != original.CursorEn(momentoConsulta) {
		t.Errorf("CursorEn diverge tras reconstituir: %d != %d", reconstituida.CursorEn(momentoConsulta), original.CursorEn(momentoConsulta))
	}
	rango, _ := NuevoRango(5000)
	if !reconstituida.TurnoDe(rango).Equal(original.TurnoDe(rango)) {
		t.Errorf("TurnoDe diverge tras reconstituir: %v != %v", reconstituida.TurnoDe(rango), original.TurnoDe(rango))
	}
}

// TestReconstituirSalaDeEspera_ConTodosLosPunterosNoNulos cubre las tres
// ramas de copia defensiva de ReconstituirSalaDeEspera (creadaPor,
// abiertaEn, cerradaEn) y los getters correspondientes en su forma "true".
func TestReconstituirSalaDeEspera_ConTodosLosPunterosNoNulos(t *testing.T) {
	idUsuario, _ := IDUsuarioDesde("018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b6000")
	idOrg := idOrganizacionDePrueba(t)
	org, _ := AlcanceOrganizacion(idOrg)
	ahora := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	abierta := ahora.Add(time.Minute)
	cerrada := ahora.Add(time.Hour)

	s := ReconstituirSalaDeEspera(
		idSalaDePrueba(t), aliasDePrueba(t), org, RutaTenenciaAceptarInvitacion,
		EstadoSalaCerrada, PoliticaSalaPorDefecto(), 100, ahora,
		&idUsuario, ahora, &abierta, &cerrada,
	)
	creadaPor, ok := s.CreadaPor()
	if !ok || !creadaPor.EsIgual(idUsuario) {
		t.Errorf("CreadaPor() = %v, %v", creadaPor, ok)
	}
	abiertaEn, ok := s.AbiertaEn()
	if !ok || !abiertaEn.Equal(abierta) {
		t.Errorf("AbiertaEn() = %v, %v", abiertaEn, ok)
	}
	cerradaEn, ok := s.CerradaEn()
	if !ok || !cerradaEn.Equal(cerrada) {
		t.Errorf("CerradaEn() = %v, %v", cerradaEn, ok)
	}
}

// --- INV-COLA-02, 08, 15: invariantes arquitectónicas -----------------

// TestINV_COLA_02_TicketDeCola_NoTransportaAutorizacion documenta y
// verifica, por reflexión, que TicketDeCola y DesenlaceDeAdmision no
// exponen ningún campo o valor que sugiera transportar un permiso, un rol o
// una autorización: un ticket de cola solo decide CUÁNDO una petición
// continúa, nunca SI está autorizada. La petición admitida sigue
// atravesando, sin excepción, los mismos controles que atravesaría sin
// sala (INV-COLA-02) — eso lo garantiza la composición del middleware con
// los demás controles (aplicacion/adaptadores, fuera del alcance de este
// paquete), no un campo del ticket.
func TestINV_COLA_02_TicketDeCola_NoTransportaAutorizacion(t *testing.T) {
	tipo := reflect.TypeOf(TicketDeCola{})
	prohibidos := []string{"permiso", "rol", "autoriz", "scope", "claim"}
	for i := 0; i < tipo.NumField(); i++ {
		nombre := strings.ToLower(tipo.Field(i).Name)
		for _, p := range prohibidos {
			if strings.Contains(nombre, p) {
				t.Errorf("TicketDeCola.%s: el nombre sugiere una autorización (INV-COLA-02)", tipo.Field(i).Name)
			}
		}
	}
}

// TestINV_COLA_08_CaminoCalienteEsPuroYRapido ejercita en un bucle grande
// las tres funciones puras que forman el camino caliente de una sala
// (CursorEn, TurnoDe, AdmiteIngreso) para evidenciar que no hacen E/S:
// sin ningún adaptador de Postgres o Redis presente en este paquete
// (verificado además por TestDominioConfianza_SoloImportaStdlibYExcepciones),
// cien mil evaluaciones deberían completarse en milisegundos, no en
// segundos — cualquier acceso a disco o red las haría órdenes de magnitud
// más lentas (INV-COLA-08: "el camino caliente de la sala es exclusivamente
// Redis + CPU local", y aquí ni siquiera hay Redis).
func TestINV_COLA_08_CaminoCalienteEsPuroYRapido(t *testing.T) {
	ahora := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	s := salaAbiertaDePrueba(t, 50, ahora)
	rango, _ := NuevoRango(12345)

	inicio := time.Now()
	for i := 0; i < 100_000; i++ {
		instante := ahora.Add(time.Duration(i) * time.Millisecond)
		_ = s.CursorEn(instante)
		_ = s.TurnoDe(rango)
		_ = s.AdmiteIngreso(int64(i), instante)
	}
	transcurrido := time.Since(inicio)
	if transcurrido > 2*time.Second {
		t.Errorf("100000 evaluaciones del camino caliente tardaron %v; se esperaba un cómputo puro en memoria, sin E/S (INV-COLA-08)", transcurrido)
	}
}

// TestINV_COLA_15_TicketDeCola_NoExponeAlcanceNiRuta verifica la mitad de
// dominio de INV-COLA-15: TicketDeCola solo expone su hash, su clave
// opaca (un string, no el AlcanceSala/RutaProtegida de la sala), su rango y
// su estado — nunca un campo tipado AlcanceSala o RutaProtegida que un
// adaptador HTTP pudiera serializar por descuido y filtrar la organización
// dueña o la ruta protegida. La ClaveSala en sí es opaca: quien la posee no
// puede reconstruir el alcance ni la ruta sin conocer el formato interno
// documentado en este mismo paquete.
func TestINV_COLA_15_TicketDeCola_NoExponeAlcanceNiRuta(t *testing.T) {
	tipo := reflect.TypeOf(TicketDeCola{})
	tipoAlcance := reflect.TypeOf(AlcanceSala{})
	tipoRuta := reflect.TypeOf(RutaProtegida{})
	for i := 0; i < tipo.NumField(); i++ {
		campo := tipo.Field(i)
		if campo.Type == tipoAlcance || campo.Type == tipoRuta {
			t.Errorf("TicketDeCola.%s tiene tipo %s: un ticket no debe exponer directamente el alcance ni la ruta de su sala (INV-COLA-15)", campo.Name, campo.Type)
		}
	}
}
