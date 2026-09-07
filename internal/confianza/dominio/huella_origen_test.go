package dominio

import (
	"strings"
	"testing"
)

// --- ClaveCuenta -----------------------------------------------------------

func TestNuevaClaveCuenta_ProduceHexTruncadoA32(t *testing.T) {
	c := NuevaClaveCuenta("usuario@example.com")
	if c.EsVacia() {
		t.Fatal("no se esperaba clave vacía")
	}
	if len(c.String()) != 32 {
		t.Errorf("longitud de ClaveCuenta = %d, esperado 32", len(c.String()))
	}
	for _, r := range c.String() {
		if !strings.ContainsRune("0123456789abcdef", r) {
			t.Errorf("ClaveCuenta contiene un carácter no hexadecimal: %q", r)
		}
	}
}

func TestNuevaClaveCuenta_EsDeterministaYSensibleAlCorreo(t *testing.T) {
	a := NuevaClaveCuenta("usuario@example.com")
	b := NuevaClaveCuenta("usuario@example.com")
	c := NuevaClaveCuenta("otro@example.com")
	if a.String() != b.String() {
		t.Error("el mismo correo debe producir la misma ClaveCuenta")
	}
	if a.String() == c.String() {
		t.Error("correos distintos no deberían producir la misma ClaveCuenta")
	}
}

func TestNuevaClaveCuenta_VaciaProduceCeroValue(t *testing.T) {
	if c := NuevaClaveCuenta(""); !c.EsVacia() {
		t.Error("un correo vacío debe producir el cero value")
	}
	if c := NuevaClaveCuenta("   "); !c.EsVacia() {
		t.Error("un correo con solo espacios debe producir el cero value")
	}
}

// TestINV_RIES_07_ClaveCuenta_NuncaContieneElCorreoEnClaro verifica la
// mitad de ClaveCuenta de INV-RIES-07: el correo en claro nunca entra a
// este subsistema. La clave hexadecimal resultante no puede contener el
// correo como subcadena (una comprobación necesaria, aunque no suficiente
// por sí sola: la garantía real es que el valor es un digest SHA-256, no
// una codificación reversible).
func TestINV_RIES_07_ClaveCuenta_NuncaContieneElCorreoEnClaro(t *testing.T) {
	correo := "persona.identificable@example.com"
	c := NuevaClaveCuenta(correo)
	if strings.Contains(c.String(), correo) {
		t.Error("ClaveCuenta no debe contener el correo en claro")
	}
	if len(c.String()) >= len(correo) {
		t.Error("ClaveCuenta truncada a 32 hex no debería ser más larga que un correo típico de prueba")
	}
}

// --- HashHuella --------------------------------------------------------

func TestHashearHuella_ProduceHexTruncadoA16(t *testing.T) {
	h := HashearHuella("abc123-fingerprint-de-un-navegador")
	if h.EsVacio() {
		t.Fatal("no se esperaba hash vacío")
	}
	if len(h.String()) != 16 {
		t.Errorf("longitud de HashHuella = %d, esperado 16", len(h.String()))
	}
}

func TestHashearHuella_VaciaProduceCeroValue(t *testing.T) {
	if h := HashearHuella(""); !h.EsVacio() {
		t.Error("una huella vacía debe producir el cero value")
	}
	if h := HashearHuella("   "); !h.EsVacio() {
		t.Error("una huella con solo espacios (tras TrimSpace) debe producir el cero value")
	}
}

func TestHashearHuella_EsDeterministaYSensibleAlValor(t *testing.T) {
	a := HashearHuella("dispositivo-A")
	b := HashearHuella("dispositivo-A")
	c := HashearHuella("dispositivo-B")
	if !a.EsIgual(b) {
		t.Error("la misma huella cruda debe producir el mismo HashHuella")
	}
	if a.EsIgual(c) {
		t.Error("huellas distintas no deberían producir el mismo HashHuella")
	}
}

// --- HashRed -------------------------------------------------------------

func ipDePrueba(t *testing.T, valor string) DireccionIP {
	t.Helper()
	ip, err := NuevaDireccionIP(valor)
	if err != nil {
		t.Fatalf("no se esperaba error construyendo la IP %q: %v", valor, err)
	}
	return ip
}

func TestHashearRed_IPPublicaProduceHash(t *testing.T) {
	h := HashearRed(ipDePrueba(t, "203.0.113.42"))
	if h.EsVacio() {
		t.Fatal("una IP pública debe producir un HashRed no vacío")
	}
}

func TestHashearRed_MismoPrefijo24ProduceElMismoHashParaIPv4(t *testing.T) {
	a := HashearRed(ipDePrueba(t, "203.0.113.5"))
	b := HashearRed(ipDePrueba(t, "203.0.113.250"))
	c := HashearRed(ipDePrueba(t, "203.0.114.5"))
	if !a.EsIgual(b) {
		t.Error("dos IPv4 en el mismo /24 deben producir el mismo HashRed")
	}
	if a.EsIgual(c) {
		t.Error("dos IPv4 en /24 distintos no deberían producir el mismo HashRed")
	}
}

func TestHashearRed_MismoPrefijo48ProduceElMismoHashParaIPv6(t *testing.T) {
	a := HashearRed(ipDePrueba(t, "2001:db8:1234::1"))
	b := HashearRed(ipDePrueba(t, "2001:db8:1234::ffff"))
	c := HashearRed(ipDePrueba(t, "2001:db8:9999::1"))
	if !a.EsIgual(b) {
		t.Error("dos IPv6 en el mismo /48 deben producir el mismo HashRed")
	}
	if a.EsIgual(c) {
		t.Error("dos IPv6 en /48 distintos no deberían producir el mismo HashRed")
	}
}

// TestINV_RIES_07_HashearRed_IPPrivadaLoopbackOAusenteProduceCeroValue
// verifica la mitad de HashRed de INV-RIES-07: la IP nunca se guarda, solo
// el SHA-256 de su prefijo, y ni siquiera eso cuando la IP es privada, de
// loopback o está ausente — en desarrollo y detrás de un proxy mal
// configurado, "todos vienen de 10.0.0.x" no es una señal, es ruido.
func TestINV_RIES_07_HashearRed_IPPrivadaLoopbackOAusenteProduceCeroValue(t *testing.T) {
	privada := HashearRed(ipDePrueba(t, "10.1.2.3"))
	loopback := HashearRed(ipDePrueba(t, "127.0.0.1"))
	ausente := HashearRed(DireccionIP{})

	if !privada.EsVacio() {
		t.Error("una IP privada debe producir el cero value")
	}
	if !loopback.EsVacio() {
		t.Error("una IP de loopback debe producir el cero value")
	}
	if !ausente.EsVacio() {
		t.Error("una IP ausente debe producir el cero value")
	}
}

// TestINV_RIES_07_HashRed_NuncaEsLaIPCompleta verifica que el HashRed de dos
// IPs distintas dentro del mismo /24 (o /48) es indistinguible: la
// información que sobrevive es el prefijo de red, nunca el host completo,
// así que un volcado de Redis no puede reconstruir la IP original de la
// petición.
func TestINV_RIES_07_HashRed_NuncaEsLaIPCompleta(t *testing.T) {
	a := HashearRed(ipDePrueba(t, "198.51.100.7"))
	b := HashearRed(ipDePrueba(t, "198.51.100.250"))
	if a.String() == "" || b.String() == "" {
		t.Fatal("se esperaban hashes no vacíos")
	}
	if a.String() != b.String() {
		t.Error("dos hosts distintos del mismo /24 deben colapsar al mismo HashRed (solo el prefijo sobrevive)")
	}
}

// --- HuellaDeOrigen --------------------------------------------------------

func origenDePrueba(t *testing.T, ip, huella string) OrigenSolicitud {
	t.Helper()
	o, err := NuevoOrigenSolicitud(ip, "agente-de-prueba", huella, "id-solicitud-1")
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	return o
}

func TestHuellaDeOrigenDesde_ConHuellaYRed(t *testing.T) {
	obs := HuellaDeOrigenDesde(origenDePrueba(t, "203.0.113.42", "huella-cruda-1"))
	if !obs.TraeHuella() {
		t.Error("TraeHuella() debe ser true cuando la petición trae una huella no vacía")
	}
	if obs.Huella().EsVacio() {
		t.Error("Huella() no debería estar vacía")
	}
	if obs.Red().EsVacio() {
		t.Error("Red() no debería estar vacía para una IP pública")
	}
}

func TestHuellaDeOrigenDesde_SinHuella(t *testing.T) {
	obs := HuellaDeOrigenDesde(origenDePrueba(t, "203.0.113.42", ""))
	if obs.TraeHuella() {
		t.Error("TraeHuella() debe ser false cuando la petición no trae huella")
	}
	if !obs.Huella().EsVacio() {
		t.Error("Huella() debe estar vacía cuando la petición no trae huella")
	}
}

func TestHuellaDeOrigenDesde_IPPrivadaProduceRedVacia(t *testing.T) {
	obs := HuellaDeOrigenDesde(origenDePrueba(t, "10.0.0.5", "huella-cruda-1"))
	if !obs.Red().EsVacio() {
		t.Error("Red() debe estar vacía para una IP privada")
	}
}

func TestHuellaDeOrigenDesde_SinIP(t *testing.T) {
	obs := HuellaDeOrigenDesde(origenDePrueba(t, "", "huella-cruda-1"))
	if !obs.Red().EsVacio() {
		t.Error("Red() debe estar vacía cuando no hay IP")
	}
}
