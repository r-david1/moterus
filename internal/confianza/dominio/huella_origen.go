package dominio

import (
	"crypto/sha256"
	"encoding/hex"
	"net"
	"strings"
)

// longitudClaveCuenta es la longitud en caracteres hexadecimales a la que se
// trunca el SHA-256 del correo normalizado (§1.3 del diseño): 32 hex, la
// mitad de un digest completo. No hace falta el digest entero para indexar
// una cuenta dentro de un HASH de Redis, y truncar reduce el tamaño de la
// clave sin reabrir la discusión de colisiones de HashHuella (aquí el
// espacio de entrada, correos válidos, es mucho más disperso que el de un
// identificador de dispositivo).
const longitudClaveCuenta = 32

// ClaveCuenta es el value object que identifica, dentro de Redis, el perfil
// de orígenes de una cuenta (§1.3 del diseño). Es el SHA-256 hex del correo
// ya normalizado por Identidad, truncado a 32 caracteres. INV-RIES-07: el
// correo en claro nunca entra a este subsistema — a diferencia de
// "confianza:rl:cuenta:login:<correo>" (limitador de tasa, TTL de 15
// minutos), este dato vive meses y esa diferencia de duración es
// precisamente la que cambia el cálculo de qué trato darle al correo.
type ClaveCuenta struct {
	valor string
}

// NuevaClaveCuenta calcula la ClaveCuenta de un correo ya normalizado. No
// vuelve a normalizar: quien llama (el caso de uso, que ya recibe
// CorreoNormalizado) es responsable de haber aplicado esa normalización
// antes.
func NuevaClaveCuenta(correoNormalizado string) ClaveCuenta {
	c := strings.TrimSpace(correoNormalizado)
	if c == "" {
		return ClaveCuenta{}
	}
	suma := sha256.Sum256([]byte(c))
	return ClaveCuenta{valor: hex.EncodeToString(suma[:])[:longitudClaveCuenta]}
}

// String devuelve la representación hexadecimal de la clave, tal como la usa
// el adaptador Redis como parte de "confianza:orig:<clave>".
func (c ClaveCuenta) String() string { return c.valor }

// EsVacia indica si el value object nunca fue construido (zero value, o el
// correo de origen estaba vacío).
func (c ClaveCuenta) EsVacia() bool { return c.valor == "" }

// longitudHashHuella es la longitud en caracteres hexadecimales a la que se
// trunca el SHA-256 de una huella de dispositivo (§1.3 del diseño): 16 hex
// (64 bits). Deliberadamente corto: alcanza de sobra para distinguir del
// orden de 20 dispositivos por cuenta (maximoOrigenesRecordados) y acota el
// tamaño del HASH de Redis.
const longitudHashHuella = 16

// HashHuella es el value object que envuelve el hash truncado de una huella
// de dispositivo cruda (§1.3 del diseño). El cero value representa "sin
// huella" y es indistinguible de un hash real solo porque nunca se compara
// contra hashes reales fuera de este paquete: la ausencia se decide con
// traeHuella, no adivinando a partir del hash.
type HashHuella struct {
	valor string
}

// HashearHuella calcula el HashHuella de una huella cruda, previa
// normalización (TrimSpace). Devuelve el cero value si la huella venía
// vacía: una petición sin cabecera X-Device-Fingerprint no tiene nada que
// hashear, y forzar un hash de la cadena vacía confundiría "no mandó
// huella" con "mandó una huella igual a la de otra cuenta que tampoco
// manda".
func HashearHuella(cruda string) HashHuella {
	v := strings.TrimSpace(cruda)
	if v == "" {
		return HashHuella{}
	}
	suma := sha256.Sum256([]byte(v))
	return HashHuella{valor: hex.EncodeToString(suma[:])[:longitudHashHuella]}
}

// String devuelve la representación hexadecimal del hash, o "" si es el
// cero value.
func (h HashHuella) String() string { return h.valor }

// EsVacio indica si no hubo huella que hashear.
func (h HashHuella) EsVacio() bool { return h.valor == "" }

// EsIgual compara dos hashes de huella por su valor. No hace falta
// comparación en tiempo constante (crypto/subtle): a diferencia de
// HashTicket, esto no protege un secreto de alta entropía generado por el
// sistema — es un identificador derivado de un dato que el propio cliente
// controla y puede falsificar (INV-RIES-06).
func (h HashHuella) EsIgual(otro HashHuella) bool { return h.valor == otro.valor }

// prefijoRedIPv4 es la longitud, en bits, del prefijo de red que se hashea
// para una IPv4 (§1.3 del diseño: /24). Una IP móvil cambia de /24 varias
// veces por día — por diseño, deliberadamente grueso (§1.5: peso bajo de
// red_desconocida).
const prefijoRedIPv4 = 24

// prefijoRedIPv6 es la longitud, en bits, del prefijo de red que se hashea
// para una IPv6 (§1.3 del diseño: /48), el tamaño típico de asignación a un
// sitio/hogar en IPv6.
const prefijoRedIPv6 = 48

// HashRed es el value object que envuelve el hash truncado del prefijo de
// red de una IP (§1.3 del diseño). Nunca la IP completa: se hashea /24 para
// IPv4 y /48 para IPv6. El cero value representa "sin red que evaluar" —
// IP privada, de loopback, o ausente (INV-RIES-07: en desarrollo y detrás de
// un proxy mal configurado, "todos vienen de 10.0.0.x" no es una señal, es
// ruido).
type HashRed struct {
	valor string
}

// HashearRed calcula el HashRed del prefijo de red de una DireccionIP.
// Devuelve el cero value si la IP está vacía o es privada/loopback/enlace
// local (DireccionIP.EsPrivada()).
func HashearRed(ip DireccionIP) HashRed {
	if ip.EsVacia() || ip.EsPrivada() {
		return HashRed{}
	}
	prefijo := prefijoDeRed(ip)
	if prefijo == nil {
		return HashRed{}
	}
	suma := sha256.Sum256(prefijo)
	return HashRed{valor: hex.EncodeToString(suma[:])[:longitudHashHuella]}
}

// prefijoDeRed devuelve los bytes crudos del prefijo de red de una IP (los
// primeros /24 bits de una IPv4, o los primeros /48 bits de una IPv6), o nil
// si la IP no es reconocible como IPv4 ni IPv6. Accede al campo no exportado
// de DireccionIP directamente porque este archivo vive en el mismo paquete
// (dominio); no hace falta exponer un getter de net.IP fuera de este
// archivo.
func prefijoDeRed(ip DireccionIP) []byte {
	if v4 := ip.ip.To4(); v4 != nil {
		return v4.Mask(net.CIDRMask(prefijoRedIPv4, 32))
	}
	if v6 := ip.ip.To16(); v6 != nil {
		return v6.Mask(net.CIDRMask(prefijoRedIPv6, 128))
	}
	return nil
}

// String devuelve la representación hexadecimal del hash, o "" si es el
// cero value.
func (h HashRed) String() string { return h.valor }

// EsVacio indica si no hubo prefijo de red que hashear.
func (h HashRed) EsVacio() bool { return h.valor == "" }

// EsIgual compara dos hashes de red por su valor.
func (h HashRed) EsIgual(otro HashRed) bool { return h.valor == otro.valor }

// HuellaDeOrigen es el value object que agrupa todo lo que el dominio ve de
// una petición de login (§1.3 del diseño): el hash de la huella de
// dispositivo, el hash del prefijo de red, y si la petición traía huella o
// no. Es lo único que PerfilDeOrigen.Senales recibe como "lo observado en
// esta petición".
type HuellaDeOrigen struct {
	huella     HashHuella
	red        HashRed
	traeHuella bool
}

// HuellaDeOrigenDesde construye una HuellaDeOrigen a partir del
// OrigenSolicitud de la petición actual: hashea la huella cruda y el
// prefijo de red, y registra si la petición traía huella (la cabecera
// venía no vacía), que es un dato distinto de "el hash quedó vacío" —
// aunque hoy ambos coincidan, mantenerlos separados es lo que le da nombre
// propio a la señal huella_ausente en vez de inferirla del hash.
func HuellaDeOrigenDesde(obs OrigenSolicitud) HuellaDeOrigen {
	cruda := strings.TrimSpace(obs.HuellaDispositivo())
	return HuellaDeOrigen{
		huella:     HashearHuella(cruda),
		red:        HashearRed(obs.IP()),
		traeHuella: cruda != "",
	}
}

// Huella devuelve el hash de la huella de dispositivo observada.
func (h HuellaDeOrigen) Huella() HashHuella { return h.huella }

// Red devuelve el hash del prefijo de red observado.
func (h HuellaDeOrigen) Red() HashRed { return h.red }

// TraeHuella indica si la petición traía una huella de dispositivo no
// vacía.
func (h HuellaDeOrigen) TraeHuella() bool { return h.traeHuella }
