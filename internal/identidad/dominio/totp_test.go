package dominio

import (
	"encoding/base32"
	"strings"
	"testing"
	"time"
)

func secretoTOTPDePrueba(t *testing.T) string {
	t.Helper()
	// 20 bytes = 160 bits de entropía, codificados en Base32 sin relleno.
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(
		[]byte("supersecretkey12345!"),
	)
}

func TestTipoFactorDesde(t *testing.T) {
	tipo, err := TipoFactorDesde("totp")
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if !tipo.EsIgual(TipoFactorTOTP) {
		t.Error("TipoFactorDesde(\"totp\") debe ser igual a TipoFactorTOTP")
	}
	if tipo.Valor() != "totp" {
		t.Errorf("Valor() = %q, esperado %q", tipo.Valor(), "totp")
	}
}

func TestTipoFactorDesde_Invalido(t *testing.T) {
	casos := []string{"", "email", "sms", "SMS", "webauthn"}
	for _, c := range casos {
		if _, err := TipoFactorDesde(c); err == nil {
			t.Errorf("TipoFactorDesde(%q) debía fallar: solo \"totp\" está en el catálogo del MVP (ADR 0037)", c)
		}
	}
}

func TestTipoFactor_EsVacio(t *testing.T) {
	var vacio TipoFactor
	if !vacio.EsVacio() {
		t.Error("el zero value debe estar vacío")
	}
	if TipoFactorTOTP.EsVacio() {
		t.Error("TipoFactorTOTP no debe estar vacío")
	}
}

func TestNuevoSecretoTOTPPlano(t *testing.T) {
	secretoValido := strings.Repeat("A", 32) // 32 chars Base32 = 160 bits
	casos := []struct {
		nombre   string
		valor    string
		esValido bool
	}{
		{"válido mayúsculas", secretoValido, true},
		{"válido minúsculas se normaliza", strings.ToLower(secretoValido), true},
		{"válido con relleno se recorta", secretoValido + "====", true},
		{"vacío", "", false},
		{"demasiado corto", strings.Repeat("A", 31), false},
		{"caracter fuera del alfabeto base32 (1)", strings.Repeat("A", 31) + "1", false},
		{"caracter fuera del alfabeto base32 (0)", strings.Repeat("A", 31) + "0", false},
		{"caracter fuera del alfabeto base32 (8)", strings.Repeat("A", 31) + "8", false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			s, err := NuevoSecretoTOTPPlano(c.valor)
			if c.esValido {
				if err != nil {
					t.Fatalf("no se esperaba error: %v", err)
				}
				if s.EsVacio() {
					t.Error("no debería resultar vacío")
				}
				return
			}
			if err == nil {
				t.Fatal("se esperaba error")
			}
		})
	}
}

// TestINV_MFA_02_SecretoTOTPPlano_SeRedacta verifica que el secreto en
// claro nunca se filtra por String(), GoString() ni MarshalJSON.
func TestINV_MFA_02_SecretoTOTPPlano_SeRedacta(t *testing.T) {
	s, err := NuevoSecretoTOTPPlano(strings.Repeat("A", 32))
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if s.String() != "[REDACTADO]" {
		t.Errorf("String() = %q, esperado [REDACTADO]", s.String())
	}
	if s.GoString() != "[REDACTADO]" {
		t.Errorf("GoString() = %q, esperado [REDACTADO]", s.GoString())
	}
	b, err := s.MarshalJSON()
	if err != nil {
		t.Fatalf("no se esperaba error de MarshalJSON: %v", err)
	}
	if string(b) != `"[REDACTADO]"` {
		t.Errorf("MarshalJSON() = %s, esperado \"[REDACTADO]\"", b)
	}
	if s.Valor() == "" || s.Valor() == "[REDACTADO]" {
		t.Error("Valor() debe seguir exponiendo el secreto real para HabilitarMFA/CifradorSecretos")
	}
}

func TestSecretoTOTPPlano_URIProvisionamiento(t *testing.T) {
	s, _ := NuevoSecretoTOTPPlano(strings.Repeat("A", 32))
	correo, _ := NuevoCorreo("usuario@ejemplo.com")
	uri := s.URIProvisionamiento(correo, "Moterus")

	if !strings.HasPrefix(uri, "otpauth://totp/") {
		t.Errorf("la URI debe empezar con otpauth://totp/, obtuvo %q", uri)
	}
	if !strings.Contains(uri, "secret="+s.Valor()) {
		t.Error("la URI debe incluir el secreto en claro en el parámetro secret")
	}
	if !strings.Contains(uri, "issuer=Moterus") {
		t.Error("la URI debe incluir el emisor")
	}
	if !strings.Contains(uri, "algorithm=SHA1") || !strings.Contains(uri, "digits=6") || !strings.Contains(uri, "period=30") {
		t.Errorf("la URI debe fijar algorithm=SHA1, digits=6, period=30: %q", uri)
	}
}

func TestNuevoSecretoTOTPCifrado(t *testing.T) {
	if _, err := NuevoSecretoTOTPCifrado(nil); err == nil {
		t.Error("un valor vacío debe ser inválido")
	}
	if _, err := NuevoSecretoTOTPCifrado([]byte{}); err == nil {
		t.Error("un valor vacío debe ser inválido")
	}
	c, err := NuevoSecretoTOTPCifrado([]byte{0x01, 0x02, 0x03})
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if c.EsVacio() {
		t.Error("no debería resultar vacío")
	}
}

func TestSecretoTOTPCifrado_ValorDevuelveCopia(t *testing.T) {
	original := []byte{0x01, 0x02, 0x03}
	c, _ := NuevoSecretoTOTPCifrado(original)
	obtenido := c.Valor()
	obtenido[0] = 0xff
	if c.Valor()[0] != 0x01 {
		t.Error("Valor() debe devolver una copia; mutarla no debe afectar al value object interno")
	}
}

func TestNuevoCodigoTOTP(t *testing.T) {
	casos := []struct {
		valor    string
		esValido bool
	}{
		{"123456", true},
		{"000000", true},
		{"", false},
		{"12345", false},   // corto
		{"1234567", false}, // largo
		{"12345a", false},  // no numérico
		{"123 456", false}, // espacio interno
	}
	for _, c := range casos {
		_, err := NuevoCodigoTOTP(c.valor)
		if c.esValido && err != nil {
			t.Errorf("NuevoCodigoTOTP(%q) no debía fallar: %v", c.valor, err)
		}
		if !c.esValido && err == nil {
			t.Errorf("NuevoCodigoTOTP(%q) debía fallar", c.valor)
		}
	}
}

func TestCodigoTOTP_NoSeRedacta(t *testing.T) {
	c, _ := NuevoCodigoTOTP("123456")
	if c.String() != "123456" {
		t.Errorf("CodigoTOTP no se redacta (vida corta, baja entropía): String() = %q", c.String())
	}
}

// --- VerificarCodigo (RFC 6238) ---------------------------------------------

// TestVerificarCodigo_VectoresOficialesRFC6238 usa los vectores de prueba
// oficiales del Apéndice B de RFC 6238 (secreto ASCII "12345678901234567890"
// para SHA1, truncado a los 6 dígitos menos significativos de cada valor de
// 8 dígitos del RFC, ya que mod 10^6 de un entero decimal son exactamente
// sus últimos 6 dígitos). El secreto se re-expresa en Base32 sin relleno
// porque esa es la representación que maneja SecretoTOTPPlano/VerificarCodigo
// en este dominio; codificar y decodificar Base32 sin relleno es una
// operación exacta que no altera los bytes originales del vector oficial.
func TestVerificarCodigo_VectoresOficialesRFC6238(t *testing.T) {
	secretoASCII := "12345678901234567890"
	secretoB32 := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte(secretoASCII))

	casos := []struct {
		nombre     string
		tiempoUnix int64
		codigoOcho string
		codigoSeis string
	}{
		{"1970-01-01 00:00:59", 59, "94287082", "287082"},
		{"2005-03-18 01:58:29", 1111111109, "07081804", "081804"},
		{"2005-03-18 01:58:31", 1111111111, "14050471", "050471"},
		{"2009-02-13 23:31:30", 1234567890, "89005924", "005924"},
		{"2033-05-18 03:33:20", 2000000000, "69279037", "279037"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			codigo, err := NuevoCodigoTOTP(c.codigoSeis)
			if err != nil {
				t.Fatalf("no se esperaba error construyendo el código: %v", err)
			}
			ahora := time.Unix(c.tiempoUnix, 0).UTC()
			if !VerificarCodigo(secretoB32, codigo, ahora) {
				t.Errorf("VerificarCodigo debía aceptar el código %s (derivado de %s del RFC) en t=%d", c.codigoSeis, c.codigoOcho, c.tiempoUnix)
			}
		})
	}
}

func TestVerificarCodigo_ToleranciaUnPasoEnAmbosSentidos(t *testing.T) {
	secreto := secretoTOTPDePrueba(t)
	// t=59 -> paso 1. El código válido para el paso 1 debe aceptarse también
	// 29s antes (todavía paso 0->no, pero dentro de ±1 paso de 30s) y 29s
	// después del propio instante usado para generarlo, siempre que el
	// contador de 30s se mantenga a 1 paso de distancia.
	base := time.Unix(1_700_000_000, 0).UTC()
	// Redondear al inicio exacto de un paso de 30s para tener control fino.
	base = time.Unix((base.Unix()/pasoTOTPSegundos)*pasoTOTPSegundos, 0).UTC()

	codigoActual := generarCodigoTOTP(decodificarSecretoBase32Test(t, secreto), uint64(base.Unix()/pasoTOTPSegundos)) //nolint:gosec // instante de prueba siempre posterior a 1970; nunca negativo.
	codigo, err := NuevoCodigoTOTP(codigoActual)
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}

	// Dentro de la tolerancia: -1 paso (30s antes) y +1 paso (30s después).
	if !VerificarCodigo(secreto, codigo, base.Add(-pasoTOTPSegundos*time.Second)) {
		t.Error("un desfase de -1 paso (30s antes) debe aceptarse")
	}
	if !VerificarCodigo(secreto, codigo, base.Add(pasoTOTPSegundos*time.Second)) {
		t.Error("un desfase de +1 paso (30s después) debe aceptarse")
	}
	// Fuera de la tolerancia: ±2 pasos.
	if VerificarCodigo(secreto, codigo, base.Add(-2*pasoTOTPSegundos*time.Second)) {
		t.Error("un desfase de -2 pasos no debe aceptarse")
	}
	if VerificarCodigo(secreto, codigo, base.Add(2*pasoTOTPSegundos*time.Second)) {
		t.Error("un desfase de +2 pasos no debe aceptarse")
	}
}

func decodificarSecretoBase32Test(t *testing.T, secreto string) []byte {
	t.Helper()
	clave, err := decodificarSecretoBase32(secreto)
	if err != nil {
		t.Fatalf("no se pudo decodificar el secreto de prueba: %v", err)
	}
	return clave
}

func TestVerificarCodigo_CodigoIncorrectoFalla(t *testing.T) {
	secreto := secretoTOTPDePrueba(t)
	codigo, _ := NuevoCodigoTOTP("000000")
	ahora := time.Unix(1_700_000_000, 0).UTC()
	// Es extremadamente improbable (1 en un millón) que "000000" coincida
	// por azar; si el generador cambia y esto empieza a fallar de forma
	// intermitente, hay que fijar el instante para que nunca coincida.
	esperado := generarCodigoTOTP(decodificarSecretoBase32Test(t, secreto), uint64(ahora.Unix()/pasoTOTPSegundos)) //nolint:gosec // instante de prueba siempre posterior a 1970; nunca negativo.
	if esperado == "000000" {
		t.Skip("colisión improbable con el código de control, no representativa")
	}
	if VerificarCodigo(secreto, codigo, ahora) {
		t.Error("un código incorrecto no debe verificarse como válido")
	}
}

func TestVerificarCodigo_SecretoInvalidoNoPanica(t *testing.T) {
	codigo, _ := NuevoCodigoTOTP("123456")
	if VerificarCodigo("no es base32 válido!!", codigo, time.Now()) {
		t.Error("un secreto malformado debe producir false, no un pánico")
	}
	if VerificarCodigo("", codigo, time.Now()) {
		t.Error("un secreto vacío debe producir false")
	}
}

func TestVerificarCodigo_CodigoVacioFalla(t *testing.T) {
	secreto := secretoTOTPDePrueba(t)
	if VerificarCodigo(secreto, CodigoTOTP{}, time.Now()) {
		t.Error("un CodigoTOTP zero-value nunca debe verificarse como válido")
	}
}

func TestErrores_TOTP_MensajesNoVacios(t *testing.T) {
	errores := []error{
		&ErrTipoFactorInvalido{Valor: "x"},
		&ErrSecretoTOTPPlanoInvalido{Motivo: "x"},
		&ErrSecretoTOTPCifradoInvalido{Motivo: "x"},
		&ErrCodigoTOTPInvalido{Motivo: "x"},
	}
	for _, err := range errores {
		if err.Error() == "" {
			t.Errorf("%T.Error() no debe estar vacío", err)
		}
	}
}
