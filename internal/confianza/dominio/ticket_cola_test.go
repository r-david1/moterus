package dominio

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func idSalaDePrueba(t *testing.T) IDSalaDeEspera {
	t.Helper()
	id, err := IDSalaDeEsperaDesde("018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6d")
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	return id
}

// salaAbiertaDePrueba construye una SalaDeEspera de alcance sistema, con
// ritmo 10/s, y la abre en el instante ahora. Devuelve el agregado ya
// abierto, con EventosPendientes ya drenado.
func salaAbiertaDePrueba(t *testing.T, ritmoPorSegundo int, ahora time.Time) *SalaDeEspera {
	t.Helper()
	alias, err := NuevoAliasSala("inscripciones-2026")
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	ritmo, err := NuevoRitmoAdmision(ritmoPorSegundo)
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	politica, err := NuevaPoliticaSala(ritmo, 500_000, 2*time.Minute, ModoDegradadoPermitir)
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	s, err := NuevaSalaDeEspera(idSalaDePrueba(t), alias, AlcanceSistema(), RutaAccesoIniciarSesion, politica, nil, ahora)
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if err := s.Abrir(ahora); err != nil {
		t.Fatalf("no se esperaba error al abrir: %v", err)
	}
	s.EventosPendientes()
	return s
}

// --- TicketPlano ---------------------------------------------------------

func ticketPlanoValido() string { return prefijoTicket + strings.Repeat("a", 43) }

func TestNuevoTicketPlano_ValidaPrefijoYLongitud(t *testing.T) {
	if _, err := NuevoTicketPlano(""); err == nil {
		t.Error("se esperaba error para un ticket vacío")
	}
	if _, err := NuevoTicketPlano("mot_rt_" + strings.Repeat("a", 43)); err == nil {
		t.Error("se esperaba error para un prefijo desconocido")
	}
	if _, err := NuevoTicketPlano(prefijoTicket + strings.Repeat("a", 10)); err == nil {
		t.Error("se esperaba error por longitud de secreto insuficiente")
	}
	if _, err := NuevoTicketPlano(prefijoTicket + "no-es-base64url!!!" + strings.Repeat("a", 30)); err == nil {
		t.Error("se esperaba error por caracteres fuera del alfabeto")
	}
	tok, err := NuevoTicketPlano(ticketPlanoValido())
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if tok.Valor() != ticketPlanoValido() {
		t.Error("Valor() debe devolver el ticket real")
	}
}

// TestINV_COLA_03_TicketPlano_NoConfundibleConOtrosTokens verifica la mitad
// de dominio de INV-COLA-03: un ticket de cola nunca puede aceptarse donde
// se espera un token de otro contexto, porque el prefijo estructural es
// distinto y NuevoTicketPlano lo rechaza de plano. mot_rt_ (Acceso) y
// mot_inv_ (Tenencia) son los prefijos ya documentados por esos contextos
// (§1.4 del diseño); un JWT (con puntos "." separando sus tres partes)
// tampoco tiene la forma de un ticket opaco.
func TestINV_COLA_03_TicketPlano_NoConfundibleConOtrosTokens(t *testing.T) {
	otrosTokens := []string{
		"mot_rt_" + strings.Repeat("a", 43),            // token de refresco de Acceso
		"mot_inv_" + strings.Repeat("a", 43),           // token de invitación de Tenencia
		"eyJhbGciOiJFZERTQSJ9.eyJzdWIiOiJ1c3IifQ.c2ln", // forma de un JWT
	}
	for _, tok := range otrosTokens {
		if _, err := NuevoTicketPlano(tok); err == nil {
			t.Errorf("NuevoTicketPlano(%q) = nil, un token de otro sistema no debe aceptarse como ticket de cola (INV-COLA-03)", tok)
		}
	}
}

// TestINV_COLA_07_TicketPlano_SeRedacta verifica que el ticket en claro
// nunca se expone accidentalmente por String/GoString/MarshalJSON, mismo
// patrón que TokenInvitacionPlano en tenencia/dominio y SecretoTOTPPlano en
// identidad/dominio.
func TestINV_COLA_07_TicketPlano_SeRedacta(t *testing.T) {
	tok, err := NuevoTicketPlano(ticketPlanoValido())
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if tok.String() != "[REDACTADO]" {
		t.Errorf("String() = %q, esperado [REDACTADO]", tok.String())
	}
	if tok.GoString() != "[REDACTADO]" {
		t.Errorf("GoString() = %q, esperado [REDACTADO]", tok.GoString())
	}
	b, err := tok.MarshalJSON()
	if err != nil {
		t.Fatalf("no se esperaba error de MarshalJSON: %v", err)
	}
	if string(b) != `"[REDACTADO]"` {
		t.Errorf("MarshalJSON() = %s, esperado \"[REDACTADO]\"", b)
	}
	if !strings.HasPrefix(tok.Valor(), prefijoTicket) {
		t.Error("Valor() debe seguir exponiendo el ticket real (única vía legítima: la respuesta de IngresarASala)")
	}
}

func TestTicketPlano_Hash(t *testing.T) {
	tok, _ := NuevoTicketPlano(ticketPlanoValido())
	h1 := tok.Hash()
	h2 := HashearTicket(tok)
	if !h1.EsIgual(h2) {
		t.Error("Hash() y HashearTicket() deben producir el mismo resultado")
	}
	if h1.EsVacio() {
		t.Error("el hash no debería estar vacío")
	}
}

func TestNuevoHashTicket_ValidaFormato(t *testing.T) {
	if _, err := NuevoHashTicket("no-es-hex"); err == nil {
		t.Error("se esperaba error por longitud/formato inválidos")
	}
	if _, err := NuevoHashTicket(strings.Repeat("A", 64)); err == nil {
		t.Error("se esperaba error por mayúsculas (debe ser hex minúscula)")
	}
	if _, err := NuevoHashTicket(strings.Repeat("a", 64)); err != nil {
		t.Errorf("no se esperaba error: %v", err)
	}
}

// --- RangoEnCola / PosicionEnCola ----------------------------------------

// TestINV_COLA_14_RangoEnCola_EsPositivoPorConstruccion verifica la mitad
// de dominio de INV-COLA-14 ("el rango es monótono y se asigna exactamente
// una vez"): el VO nunca acepta un valor menor a 1, que es la precondición
// que hace que una secuencia asignada por INCR de Redis (arranca en 1 y
// nunca repite) sea representable. La atomicidad de la asignación en sí es
// una garantía de Redis, fuera del alcance de un test de dominio puro.
func TestINV_COLA_14_RangoEnCola_EsPositivoPorConstruccion(t *testing.T) {
	for _, v := range []int64{-1, 0} {
		if _, err := NuevoRango(v); err == nil {
			t.Errorf("NuevoRango(%d) = nil, se esperaba error", v)
		}
	}
	secuencia := []int64{1, 2, 3, 4, 5}
	var anterior RangoEnCola
	for i, v := range secuencia {
		r, err := NuevoRango(v)
		if err != nil {
			t.Fatalf("no se esperaba error: %v", err)
		}
		if i > 0 && r.Valor() <= anterior.Valor() {
			t.Errorf("la secuencia debe ser estrictamente creciente: %d no es mayor que %d", r.Valor(), anterior.Valor())
		}
		anterior = r
	}
}

func TestRangoEnCola_EsVacio(t *testing.T) {
	var r RangoEnCola
	if !r.EsVacio() {
		t.Error("el zero value debería estar vacío")
	}
	valido, _ := NuevoRango(1)
	if valido.EsVacio() {
		t.Error("un rango válido no debería estar vacío")
	}
}

func TestTicketDeCola_Posicion(t *testing.T) {
	rango, _ := NuevoRango(100)
	clave := nuevaClaveSala(AlcanceSistema(), RutaAccesoIniciarSesion)
	hash, _ := NuevoHashTicket(strings.Repeat("a", 64))
	tk, err := NuevoTicketDeCola(hash, clave, rango, EstadoTicketEsperando, time.Now())
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if tk.Posicion(40).Valor() != 60 {
		t.Errorf("Posicion(40) = %d, esperado 60", tk.Posicion(40).Valor())
	}
	if tk.Posicion(150).Valor() != 0 {
		t.Errorf("Posicion(150) = %d, esperado 0 (max(0, rango-cursor))", tk.Posicion(150).Valor())
	}
}

func TestNuevoTicketDeCola_ValidaCampos(t *testing.T) {
	rango, _ := NuevoRango(1)
	clave := nuevaClaveSala(AlcanceSistema(), RutaAccesoIniciarSesion)
	hash, _ := NuevoHashTicket(strings.Repeat("a", 64))
	ahora := time.Now()

	if _, err := NuevoTicketDeCola(HashTicket{}, clave, rango, EstadoTicketEsperando, ahora); err == nil {
		t.Error("se esperaba error con hash vacío")
	}
	if _, err := NuevoTicketDeCola(hash, ClaveSala{}, rango, EstadoTicketEsperando, ahora); err == nil {
		t.Error("se esperaba error con clave vacía")
	}
	if _, err := NuevoTicketDeCola(hash, clave, RangoEnCola{}, EstadoTicketEsperando, ahora); err == nil {
		t.Error("se esperaba error con rango vacío")
	}
	if _, err := NuevoTicketDeCola(hash, clave, rango, EstadoTicket{}, ahora); err == nil {
		t.Error("se esperaba error con estado vacío")
	}
}

// --- EstadoTicket ---------------------------------------------------------

func TestEstadoTicketDesde_CatalogoCerrado(t *testing.T) {
	if _, err := EstadoTicketDesde("inexistente"); err == nil {
		t.Error("se esperaba error para un valor fuera del catálogo")
	}
	for _, v := range []EstadoTicket{EstadoTicketEsperando, EstadoTicketConsumido} {
		e, err := EstadoTicketDesde(v.String())
		if err != nil || !e.EsIgual(v) {
			t.Errorf("EstadoTicketDesde(%q) = %v, %v", v.String(), e, err)
		}
	}
}

// TestINV_COLA_06_EstadoTicket_ConsumidoEsTerminal cubre la máquina de
// estados de EstadoTicket: esperando puede avanzar únicamente a consumido,
// y consumido no admite ninguna transición de salida (INV-COLA-06).
func TestINV_COLA_06_EstadoTicket_ConsumidoEsTerminal(t *testing.T) {
	if !EstadoTicketEsperando.PuedeTransicionarA(EstadoTicketConsumido) {
		t.Error("esperando debería poder transicionar a consumido")
	}
	if EstadoTicketConsumido.PuedeTransicionarA(EstadoTicketEsperando) {
		t.Error("consumido es terminal: no debería poder volver a esperando")
	}
	if !EstadoTicketConsumido.EsTerminal() {
		t.Error("consumido debería ser terminal")
	}
}

// TestINV_COLA_06_Desenlace_TicketConsumidoSiempreGanaAlRango verifica que,
// una vez consumido, el desenlace de un ticket es SIEMPRE ticket_consumido
// sin importar su rango ni el instante consultado: un ticket se reclama
// exactamente una vez y esa condición domina cualquier otra (INV-COLA-06).
func TestINV_COLA_06_Desenlace_TicketConsumidoSiempreGanaAlRango(t *testing.T) {
	ahora := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	sala := salaAbiertaDePrueba(t, 10, ahora)
	clave := sala.Clave()
	hash, _ := NuevoHashTicket(strings.Repeat("a", 64))
	rango, _ := NuevoRango(1) // ya admitido: 1 <= cursor en cualquier instante posterior

	tk, err := NuevoTicketDeCola(hash, clave, rango, EstadoTicketConsumido, ahora)
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	desenlace := tk.Desenlace(sala, ahora.Add(time.Hour))
	if !desenlace.EsIgual(DesenlaceTicketConsumido) {
		t.Errorf("Desenlace() = %v, esperado ticket_consumido", desenlace)
	}
}

func TestTicketDeCola_Desenlace_Esperando(t *testing.T) {
	ahora := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	sala := salaAbiertaDePrueba(t, 10, ahora) // 10 admisiones/segundo
	clave := sala.Clave()
	hash, _ := NuevoHashTicket(strings.Repeat("a", 64))
	rango, _ := NuevoRango(1000) // muy por delante del cursor inicial (0)

	tk, err := NuevoTicketDeCola(hash, clave, rango, EstadoTicketEsperando, ahora)
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if d := tk.Desenlace(sala, ahora); !d.EsIgual(DesenlaceEsperando) {
		t.Errorf("Desenlace() = %v, esperado esperando", d)
	}
}

func TestTicketDeCola_Desenlace_AdmitidoYTurnoCaducado(t *testing.T) {
	ahora := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	sala := salaAbiertaDePrueba(t, 10, ahora) // 10/s, ventana de reclamo 2 min
	clave := sala.Clave()
	hash, _ := NuevoHashTicket(strings.Repeat("a", 64))
	rango, _ := NuevoRango(50) // turno a los 5s (50/10)

	tk, err := NuevoTicketDeCola(hash, clave, rango, EstadoTicketEsperando, ahora)
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}

	// Justo en el turno: admitido.
	turno := sala.TurnoDe(rango)
	if d := tk.Desenlace(sala, turno); !d.EsIgual(DesenlaceAdmitido) {
		t.Errorf("Desenlace() en el turno = %v, esperado admitido", d)
	}

	// Dentro de la ventana de reclamo: sigue admitido.
	if d := tk.Desenlace(sala, turno.Add(90*time.Second)); !d.EsIgual(DesenlaceAdmitido) {
		t.Errorf("Desenlace() dentro de la ventana = %v, esperado admitido", d)
	}

	// Fuera de la ventana de reclamo: turno caducado.
	if d := tk.Desenlace(sala, turno.Add(3*time.Minute)); !d.EsIgual(DesenlaceTurnoCaducado) {
		t.Errorf("Desenlace() fuera de la ventana = %v, esperado turno_caducado", d)
	}
}

func TestErrores_ImplementanError(t *testing.T) {
	var errs []error = []error{
		&ErrTicketPlanoInvalido{Motivo: "x"},
		&ErrHashTicketInvalido{Motivo: "x"},
		&ErrRangoEnColaInvalido{Motivo: "x"},
		&ErrEstadoTicketInvalido{Valor: "x"},
		&ErrClaveSalaInvalida{Motivo: "x"},
	}
	for _, e := range errs {
		if e.Error() == "" {
			t.Errorf("%T.Error() no debería estar vacío", e)
		}
	}
	var target *ErrTicketPlanoInvalido
	if !errors.As(errs[0], &target) {
		t.Error("errors.As debería reconocer *ErrTicketPlanoInvalido")
	}
}

// TestErrores_TodosImplementanErrorConMensajeNoVacio recorre el catálogo
// completo de errores tipados de §1.8 del diseño (más los de construcción
// de cada VO) y verifica que ninguno tiene un mensaje vacío: es la garantía
// mínima de que la capa de aplicación puede loguear cualquiera de ellos sin
// mapear primero.
func TestErrores_TodosImplementanErrorConMensajeNoVacio(t *testing.T) {
	todos := []error{
		&ErrIDSalaDeEsperaInvalido{Motivo: "x"},
		&ErrIDOrganizacionInvalido{Motivo: "x"},
		&ErrIDUsuarioInvalido{Motivo: "x"},
		&ErrDireccionIPInvalida{Motivo: "x"},
		&ErrAliasSalaInvalido{Motivo: "x"},
		&ErrAliasSalaYaRegistrado{Alias: "x"},
		&ErrAlcanceSalaInvalido{Motivo: "x"},
		&ErrRutaNoProtegible{Motivo: "x"},
		&ErrRitmoAdmisionInvalido{Motivo: "x"},
		&ErrModoDegradadoInvalido{Valor: "x"},
		&ErrPoliticaSalaInvalida{Motivo: "x"},
		&ErrEstadoSalaInvalido{Valor: "x"},
		&ErrDesenlaceDeAdmisionInvalido{Valor: "x"},
		&ErrEstadoTicketInvalido{Valor: "x"},
		&ErrTicketPlanoInvalido{Motivo: "x"},
		&ErrHashTicketInvalido{Motivo: "x"},
		&ErrRangoEnColaInvalido{Motivo: "x"},
		&ErrClaveSalaInvalida{Motivo: "x"},
		&ErrSalaNoEncontrada{Referencia: "x"},
		&ErrSalaYaAbiertaParaLaRuta{Clave: "x"},
		&ErrTransicionEstadoSalaInvalida{Origen: EstadoSalaAbierta, Destino: EstadoSalaCerrada},
		&ErrSalaNoVigente{Estado: EstadoSalaProgramada},
		&ErrSalaNoAbierta{Estado: EstadoSalaDrenando},
		&ErrColaLlena{},
		&ErrTurnoNoAlcanzado{Posicion: 10},
		&ErrTurnoCaducado{},
		&ErrTicketDesconocido{},
		&ErrTicketConsumido{},
		&ErrEstadoDeColaNoDisponible{Motivo: "x"},
		&ErrPoliticaRiesgoInvalida{Motivo: "x"},
		&ErrSenalRiesgoDesconocida{Valor: "x"},
	}
	for _, e := range todos {
		if e.Error() == "" {
			t.Errorf("%T.Error() no debería estar vacío", e)
		}
	}
}

// TestTicketDeCola_Getters cubre los getters simples de TicketDeCola.
func TestTicketDeCola_Getters(t *testing.T) {
	rango, _ := NuevoRango(7)
	clave := nuevaClaveSala(AlcanceSistema(), RutaAccesoIniciarSesion)
	hash, _ := NuevoHashTicket(strings.Repeat("a", 64))
	ahora := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tk, err := NuevoTicketDeCola(hash, clave, rango, EstadoTicketEsperando, ahora)
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if !tk.Hash().EsIgual(hash) {
		t.Error("Hash() no coincide")
	}
	if !tk.Clave().EsIgual(clave) {
		t.Error("Clave() no coincide")
	}
	if tk.Rango().Valor() != 7 {
		t.Errorf("Rango().Valor() = %d, esperado 7", tk.Rango().Valor())
	}
	if !tk.Estado().EsIgual(EstadoTicketEsperando) {
		t.Error("Estado() no coincide")
	}
	if !tk.EmitidoEn().Equal(ahora) {
		t.Error("EmitidoEn() no coincide")
	}
}

func TestHashTicket_StringYValor(t *testing.T) {
	h, _ := NuevoHashTicket(strings.Repeat("a", 64))
	if h.String() != strings.Repeat("a", 64) {
		t.Errorf("String() = %q", h.String())
	}
	if h.Valor() != h.String() {
		t.Error("Valor() y String() deberían coincidir")
	}
	if h.EsVacio() {
		t.Error("no debería estar vacío")
	}
	var vacio HashTicket
	if !vacio.EsVacio() {
		t.Error("el zero value debería estar vacío")
	}
}
