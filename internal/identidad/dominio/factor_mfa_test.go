package dominio

import (
	"encoding/base32"
	"errors"
	"strings"
	"testing"
	"time"
)

func idFactorDePrueba(t *testing.T) IDFactorMFA {
	t.Helper()
	id, err := IDFactorMFADesde("018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c99")
	if err != nil {
		t.Fatalf("no se pudo construir el IDFactorMFA de prueba: %v", err)
	}
	return id
}

// secretoFactorDePrueba devuelve el par (secreto en claro Base32, secreto
// "cifrado" -- aquí solo envuelto como bytes crudos, el dominio no cifra de
// verdad) usado en los tests de FactorMFA. El caso de uso real cifraría el
// secreto con CifradorSecretos; el dominio de FactorMFA nunca ve ese paso,
// solo recibe el secreto ya descifrado en Confirmar/VerificarCodigo.
func secretoFactorDePrueba(t *testing.T) (string, SecretoTOTPCifrado) {
	t.Helper()
	plano := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte("clave-secreta-de-prueba-1234"))
	cifrado, err := NuevoSecretoTOTPCifrado([]byte(plano))
	if err != nil {
		t.Fatalf("no se pudo construir el secreto cifrado de prueba: %v", err)
	}
	return plano, cifrado
}

func codigoValidoParaSecreto(t *testing.T, secreto string, ahora time.Time) CodigoTOTP {
	t.Helper()
	clave, err := decodificarSecretoBase32(secreto)
	if err != nil {
		t.Fatalf("no se pudo decodificar el secreto: %v", err)
	}
	contador := uint64(ahora.Unix() / pasoTOTPSegundos) //nolint:gosec // instante de prueba siempre posterior a 1970; nunca negativo.
	codigo, err := NuevoCodigoTOTP(generarCodigoTOTP(clave, contador))
	if err != nil {
		t.Fatalf("no se pudo construir el código de control: %v", err)
	}
	return codigo
}

func TestHabilitarFactorMFA(t *testing.T) {
	id := idFactorDePrueba(t)
	usuarioID := idDePrueba(t)
	_, cifrado := secretoFactorDePrueba(t)
	ahora := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	f, err := HabilitarFactorMFA(id, usuarioID, TipoFactorTOTP, cifrado, ahora)
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if f.EstaConfirmado() {
		t.Error("un factor recién habilitado no debe estar confirmado (INV-MFA-01)")
	}
	if _, tiene := f.ConfirmadoEn(); tiene {
		t.Error("un factor recién habilitado no debe tener ConfirmadoEn")
	}
	if !f.ID().EsIgual(id) || !f.UsuarioID().EsIgual(usuarioID) || !f.Tipo().EsIgual(TipoFactorTOTP) {
		t.Error("los getters deben reflejar los valores de construcción")
	}
	if len(f.CodigosRespaldo()) != 0 {
		t.Error("un factor recién habilitado no debe tener códigos de respaldo (ADR 0040)")
	}

	eventos := f.EventosPendientes()
	if len(eventos) != 1 {
		t.Fatalf("se esperaba 1 evento, hay %d", len(eventos))
	}
	if eventos[0].NombreEvento() != "FactorMFAHabilitado" {
		t.Errorf("evento = %q, esperado FactorMFAHabilitado", eventos[0].NombreEvento())
	}
	if len(f.EventosPendientes()) != 0 {
		t.Error("EventosPendientes debe vaciar el buffer tras la primera llamada")
	}
}

func TestHabilitarFactorMFA_Invalidaciones(t *testing.T) {
	id := idFactorDePrueba(t)
	usuarioID := idDePrueba(t)
	_, cifrado := secretoFactorDePrueba(t)
	ahora := time.Now()

	if _, err := HabilitarFactorMFA(IDFactorMFA{}, usuarioID, TipoFactorTOTP, cifrado, ahora); err == nil {
		t.Error("un IDFactorMFA vacío debe rechazarse")
	}
	if _, err := HabilitarFactorMFA(id, IDUsuario{}, TipoFactorTOTP, cifrado, ahora); err == nil {
		t.Error("un IDUsuario vacío debe rechazarse")
	}
	if _, err := HabilitarFactorMFA(id, usuarioID, TipoFactor{}, cifrado, ahora); err == nil {
		t.Error("un TipoFactor vacío debe rechazarse")
	}
	if _, err := HabilitarFactorMFA(id, usuarioID, TipoFactorTOTP, SecretoTOTPCifrado{}, ahora); err == nil {
		t.Error("un SecretoTOTPCifrado vacío debe rechazarse")
	}
}

func TestFactorMFA_Confirmar_Exitoso(t *testing.T) {
	id := idFactorDePrueba(t)
	usuarioID := idDePrueba(t)
	secretoPlano, cifrado := secretoFactorDePrueba(t)
	ahora := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	f, _ := HabilitarFactorMFA(id, usuarioID, TipoFactorTOTP, cifrado, ahora)
	f.EventosPendientes() // drena FactorMFAHabilitado, no es objeto de este test

	confirmadoEn := ahora.Add(time.Minute)
	codigo := codigoValidoParaSecreto(t, secretoPlano, confirmadoEn)
	if err := f.Confirmar(codigo, secretoPlano, confirmadoEn); err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if !f.EstaConfirmado() {
		t.Error("el factor debe quedar confirmado (INV-MFA-01)")
	}
	got, tiene := f.ConfirmadoEn()
	if !tiene || !got.Equal(confirmadoEn) {
		t.Errorf("ConfirmadoEn() = %v, %v; esperado %v, true", got, tiene, confirmadoEn)
	}

	eventos := f.EventosPendientes()
	if len(eventos) != 1 || eventos[0].NombreEvento() != "FactorMFAConfirmado" {
		t.Fatalf("se esperaba 1 evento FactorMFAConfirmado, obtuvo %v", eventos)
	}
}

func TestFactorMFA_Confirmar_CodigoIncorrecto(t *testing.T) {
	id := idFactorDePrueba(t)
	usuarioID := idDePrueba(t)
	secretoPlano, cifrado := secretoFactorDePrueba(t)
	ahora := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	f, _ := HabilitarFactorMFA(id, usuarioID, TipoFactorTOTP, cifrado, ahora)
	f.EventosPendientes()

	incorrecto, _ := NuevoCodigoTOTP("000000")
	valido := codigoValidoParaSecreto(t, secretoPlano, ahora)
	if incorrecto == valido {
		t.Skip("colisión improbable con el código de control")
	}

	err := f.Confirmar(incorrecto, secretoPlano, ahora)
	if err == nil {
		t.Fatal("se esperaba ErrCodigoOTPInvalido")
	}
	var errCodigo *ErrCodigoOTPInvalido
	if !errors.As(err, &errCodigo) {
		t.Errorf("se esperaba *ErrCodigoOTPInvalido, obtuvo %T", err)
	}
	if f.EstaConfirmado() {
		t.Error("un código incorrecto no debe confirmar el factor")
	}
	if len(f.EventosPendientes()) != 0 {
		t.Error("un intento fallido de confirmación no debe acumular eventos")
	}
}

func TestFactorMFA_Confirmar_YaConfirmado(t *testing.T) {
	id := idFactorDePrueba(t)
	usuarioID := idDePrueba(t)
	secretoPlano, cifrado := secretoFactorDePrueba(t)
	ahora := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	f, _ := HabilitarFactorMFA(id, usuarioID, TipoFactorTOTP, cifrado, ahora)
	codigo := codigoValidoParaSecreto(t, secretoPlano, ahora)
	if err := f.Confirmar(codigo, secretoPlano, ahora); err != nil {
		t.Fatalf("no se esperaba error en la primera confirmación: %v", err)
	}

	err := f.Confirmar(codigo, secretoPlano, ahora)
	if err == nil {
		t.Fatal("se esperaba ErrFactorMFAYaConfirmado")
	}
	var errYaConfirmado *ErrFactorMFAYaConfirmado
	if !errors.As(err, &errYaConfirmado) {
		t.Errorf("se esperaba *ErrFactorMFAYaConfirmado, obtuvo %T", err)
	}
}

func hashesDeCodigosDePrueba(t *testing.T, n int) []HashCodigoRespaldo {
	t.Helper()
	alfabeto := "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
	hashes := make([]HashCodigoRespaldo, 0, n)
	for i := 0; i < n; i++ {
		// Genera un código de 10 caracteres del alfabeto permitido,
		// determinístico y distinto por índice.
		var sb strings.Builder
		for j := 0; j < 10; j++ {
			sb.WriteByte(alfabeto[(i*7+j*3)%len(alfabeto)])
		}
		plano, err := NuevoCodigoRespaldoPlano(sb.String())
		if err != nil {
			t.Fatalf("no se pudo construir el código de respaldo de prueba: %v", err)
		}
		hashes = append(hashes, plano.Hash())
	}
	return hashes
}

func factorConfirmadoDePrueba(t *testing.T) (*FactorMFA, string, time.Time) {
	t.Helper()
	id := idFactorDePrueba(t)
	usuarioID := idDePrueba(t)
	secretoPlano, cifrado := secretoFactorDePrueba(t)
	ahora := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	f, _ := HabilitarFactorMFA(id, usuarioID, TipoFactorTOTP, cifrado, ahora)
	codigo := codigoValidoParaSecreto(t, secretoPlano, ahora)
	if err := f.Confirmar(codigo, secretoPlano, ahora); err != nil {
		t.Fatalf("no se esperaba error al confirmar: %v", err)
	}
	f.EventosPendientes() // drena Habilitado+Confirmado, no son objeto de estos tests
	return f, secretoPlano, ahora
}

func TestFactorMFA_AsignarCodigosRespaldo(t *testing.T) {
	f, _, _ := factorConfirmadoDePrueba(t)
	hashes := hashesDeCodigosDePrueba(t, 10)
	f.AsignarCodigosRespaldo(hashes)

	if len(f.CodigosRespaldo()) != 10 {
		t.Fatalf("se esperaban 10 códigos de respaldo, hay %d", len(f.CodigosRespaldo()))
	}
	if f.CodigosRespaldoDisponibles() != 10 {
		t.Errorf("los 10 códigos recién asignados deben estar disponibles, hay %d", f.CodigosRespaldoDisponibles())
	}
}

func TestFactorMFA_VerificarCodigo_TOTP_Exitoso(t *testing.T) {
	f, secretoPlano, ahora := factorConfirmadoDePrueba(t)
	codigo := codigoValidoParaSecreto(t, secretoPlano, ahora)

	if !f.VerificarCodigo(codigo.Valor(), secretoPlano, ahora) {
		t.Error("un código TOTP válido debe verificarse con éxito")
	}
	// El éxito puntual no se audita a este nivel (§3.4 del diseño): el
	// evento de auditoría relevante lo dispara Acceso al completar el login.
	if len(f.EventosPendientes()) != 0 {
		t.Error("una verificación TOTP exitosa no debe acumular eventos propios")
	}
}

func TestFactorMFA_VerificarCodigo_CodigoRespaldo_ExitosoYConsumeUnaVez(t *testing.T) {
	f, secretoPlano, ahora := factorConfirmadoDePrueba(t)
	hashes := hashesDeCodigosDePrueba(t, 3)
	f.AsignarCodigosRespaldo(hashes)

	// Reconstruye el código en claro correspondiente al primer hash (mismo
	// algoritmo determinístico que hashesDeCodigosDePrueba).
	alfabeto := "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
	var sb strings.Builder
	for j := 0; j < 10; j++ {
		sb.WriteByte(alfabeto[(0*7+j*3)%len(alfabeto)])
	}
	codigoRespaldo := sb.String()

	// El propio secreto de una CodigoTOTP nunca coincidiría estructuralmente
	// (6 dígitos vs 10 alfanuméricos), así que forzamos que el TOTP falle
	// pasando un secreto que no coincide, para aislar el camino de backup.
	if !f.VerificarCodigo(codigoRespaldo, secretoPlano, ahora) {
		t.Fatal("el código de respaldo válido debe verificarse con éxito")
	}
	if f.CodigosRespaldoDisponibles() != 2 {
		t.Errorf("deben quedar 2 códigos disponibles tras consumir uno, hay %d", f.CodigosRespaldoDisponibles())
	}

	eventos := f.EventosPendientes()
	if len(eventos) != 1 || eventos[0].NombreEvento() != "CodigoRespaldoConsumido" {
		t.Fatalf("se esperaba 1 evento CodigoRespaldoConsumido, obtuvo %v", eventos)
	}
	consumido, ok := eventos[0].(CodigoRespaldoConsumido)
	if !ok {
		t.Fatalf("tipo de evento inesperado: %T", eventos[0])
	}
	if consumido.CodigosRestantes != 2 {
		t.Errorf("CodigosRestantes = %d, esperado 2", consumido.CodigosRestantes)
	}

	// INV-MFA-06: el mismo código de respaldo no puede reutilizarse.
	if f.VerificarCodigo(codigoRespaldo, secretoPlano, ahora) {
		t.Error("un código de respaldo ya consumido no debe volver a verificarse como válido")
	}
}

func TestFactorMFA_VerificarCodigo_FalloTotalEmiteEventoYNoConsumeNada(t *testing.T) {
	f, secretoPlano, ahora := factorConfirmadoDePrueba(t)
	hashes := hashesDeCodigosDePrueba(t, 2)
	f.AsignarCodigosRespaldo(hashes)

	if f.VerificarCodigo("000000", secretoPlano, ahora.Add(365*24*time.Hour)) {
		t.Fatal("un código incorrecto, muy alejado en el tiempo, no debe verificarse como válido")
	}
	if f.CodigosRespaldoDisponibles() != 2 {
		t.Error("un fallo total no debe consumir ningún código de respaldo")
	}
	eventos := f.EventosPendientes()
	if len(eventos) != 1 || eventos[0].NombreEvento() != "VerificacionOTPFallida" {
		t.Fatalf("se esperaba 1 evento VerificacionOTPFallida, obtuvo %v", eventos)
	}
}

// TestINV_MFA_08_VerificarCodigo_RespuestaIndistinguible verifica que la
// forma observable (bool) de VerificarCodigo no distingue entre "código TOTP
// incorrecto", "código de respaldo inexistente" o "sin códigos de respaldo
// en absoluto": todos devuelven exactamente false, sin ningún dato adicional
// en la respuesta.
func TestINV_MFA_08_VerificarCodigo_RespuestaIndistinguible(t *testing.T) {
	f, secretoPlano, ahora := factorConfirmadoDePrueba(t)
	futuro := ahora.Add(365 * 24 * time.Hour)

	// Sin códigos de respaldo asignados todavía.
	r1 := f.VerificarCodigo("000000", secretoPlano, futuro)
	f.EventosPendientes()

	f.AsignarCodigosRespaldo(hashesDeCodigosDePrueba(t, 1))
	// Con códigos de respaldo, pero el presentado no coincide con ninguno.
	r2 := f.VerificarCodigo("ZZZZZZZZZZ", secretoPlano, futuro)
	f.EventosPendientes()

	if r1 != false || r2 != false {
		t.Fatal("ambos casos deben resultar en false")
	}
}

func TestFactorMFA_Deshabilitar_EmiteEvento(t *testing.T) {
	f, _, ahora := factorConfirmadoDePrueba(t)
	if !f.EstaActivo() {
		t.Fatal("precondición: un factor recién confirmado debe estar activo")
	}
	f.Deshabilitar(ahora)

	eventos := f.EventosPendientes()
	if len(eventos) != 1 || eventos[0].NombreEvento() != "FactorMFADeshabilitado" {
		t.Fatalf("se esperaba 1 evento FactorMFADeshabilitado, obtuvo %v", eventos)
	}
	if f.EstaActivo() {
		t.Error("Deshabilitar debe poner EstaActivo() en false")
	}
	if !f.EstaConfirmado() {
		t.Error("Deshabilitar no debe afectar EstaConfirmado (hecho histórico)")
	}
}

func TestReconstituirFactorMFA(t *testing.T) {
	id := idFactorDePrueba(t)
	usuarioID := idDePrueba(t)
	_, cifrado := secretoFactorDePrueba(t)
	creadoEn := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	confirmadoEn := creadoEn.Add(time.Minute)
	hash, _ := NuevoHashCodigoRespaldo(strings.Repeat("a", 64))
	codigos := []CodigoRespaldoMFA{ReconstituirCodigoRespaldoMFA(hash, nil)}

	f := ReconstituirFactorMFA(id, usuarioID, TipoFactorTOTP, cifrado, true, true, creadoEn, &confirmadoEn, codigos)

	if !f.EstaConfirmado() {
		t.Error("debe reconstituirse como confirmado")
	}
	if !f.EstaActivo() {
		t.Error("debe reconstituirse como activo cuando el parámetro activo es true")
	}
	got, tiene := f.ConfirmadoEn()
	if !tiene || !got.Equal(confirmadoEn) {
		t.Errorf("ConfirmadoEn() = %v, %v; esperado %v, true", got, tiene, confirmadoEn)
	}
	if len(f.CodigosRespaldo()) != 1 {
		t.Errorf("se esperaba 1 código de respaldo reconstituido, hay %d", len(f.CodigosRespaldo()))
	}
	if len(f.EventosPendientes()) != 0 {
		t.Error("ReconstituirFactorMFA no debe acumular eventos: no es una operación de negocio nueva")
	}
}

// TestReconstituirFactorMFA_ActivoFalse cubre el caso de un factor
// confirmado alguna vez pero ya deshabilitado (INV-MFA-01 §EstaActivo):
// EstaConfirmado sigue en true (hecho histórico), EstaActivo en false.
func TestReconstituirFactorMFA_ActivoFalse(t *testing.T) {
	id := idFactorDePrueba(t)
	usuarioID := idDePrueba(t)
	_, cifrado := secretoFactorDePrueba(t)
	creadoEn := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	confirmadoEn := creadoEn.Add(time.Minute)

	f := ReconstituirFactorMFA(id, usuarioID, TipoFactorTOTP, cifrado, true, false, creadoEn, &confirmadoEn, nil)

	if !f.EstaConfirmado() {
		t.Error("EstaConfirmado debe seguir en true: es un hecho histórico, no lo revierte Deshabilitar")
	}
	if f.EstaActivo() {
		t.Error("EstaActivo debe ser false cuando se reconstituye como deshabilitado")
	}
}

func TestErrores_FactorMFA_MensajesNoVacios(t *testing.T) {
	errores := []error{
		&ErrIDFactorMFAInvalido{Motivo: "x"},
		&ErrFactorMFANoEncontrado{},
		&ErrFactorMFAYaConfirmado{},
		&ErrCodigoOTPInvalido{},
		&ErrLimiteFactoresMFAExcedido{},
	}
	for _, err := range errores {
		if err.Error() == "" {
			t.Errorf("%T.Error() no debe estar vacío", err)
		}
	}
}
