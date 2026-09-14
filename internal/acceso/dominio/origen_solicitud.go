package dominio

import (
	"net"
	"strings"
	"unicode/utf8"
)

// longitudMaximaAgenteUsuario acota el User-Agent almacenado.
const longitudMaximaAgenteUsuario = 512

// DireccionIP es el value object que envuelve una dirección IPv4 o IPv6 ya
// validada. Tipo propio de acceso/dominio, duplicado a propósito del
// homónimo de identidad/dominio (§1.7 del diseño; ADR 0027).
type DireccionIP struct {
	ip net.IP
}

// NuevaDireccionIP valida que el valor sea una IPv4 o IPv6 reconocible.
func NuevaDireccionIP(valor string) (DireccionIP, error) {
	v := strings.TrimSpace(valor)
	if v == "" {
		return DireccionIP{}, &ErrDireccionIPInvalida{Motivo: "no puede estar vacía"}
	}
	ip := net.ParseIP(v)
	if ip == nil {
		return DireccionIP{}, &ErrDireccionIPInvalida{Motivo: "formato no reconocido"}
	}
	return DireccionIP{ip: ip}, nil
}

// EsVacia indica si el value object nunca fue construido, típicamente
// porque la solicitud es una llamada interna del sistema sin IP de origen
// (p. ej. la revalidación de estado del sujeto en cada renovación, §3.2).
func (d DireccionIP) EsVacia() bool { return d.ip == nil }

// String devuelve la representación textual de la IP, o "" si está vacía.
func (d DireccionIP) String() string {
	if d.ip == nil {
		return ""
	}
	return d.ip.String()
}

// EsPrivada indica si la IP pertenece a un rango privado, de loopback o de
// enlace local.
func (d DireccionIP) EsPrivada() bool {
	if d.ip == nil {
		return false
	}
	return d.ip.IsPrivate() || d.ip.IsLoopback() || d.ip.IsLinkLocalUnicast()
}

// OrigenSolicitud es el value object que transporta el contexto forense de
// quién pidió una operación y desde dónde: IP, agente de usuario, huella de
// dispositivo e id de solicitud. Tipo propio de acceso/dominio, duplicado a
// propósito del homónimo de identidad/dominio (§1.7 del diseño), con la misma
// forma y las mismas reglas. Se llama OrigenSolicitud, y deliberadamente no
// ContextoSolicitud, para que ningún parámetro se confunda con
// ctx context.Context de Go.
type OrigenSolicitud struct {
	ip                DireccionIP
	agenteUsuario     string
	huellaDispositivo string
	idSolicitud       string
}

// NuevoOrigenSolicitud construye un OrigenSolicitud. La IP puede ir vacía
// cuando la llamada es interna del sistema; el agente de usuario se trunca
// a 512 caracteres.
func NuevoOrigenSolicitud(ipCruda, agenteUsuario, huellaDispositivo, idSolicitud string) (OrigenSolicitud, error) {
	var ip DireccionIP
	ipCruda = strings.TrimSpace(ipCruda)
	if ipCruda != "" {
		var err error
		ip, err = NuevaDireccionIP(ipCruda)
		if err != nil {
			return OrigenSolicitud{}, err
		}
	}
	agente := strings.TrimSpace(agenteUsuario)
	if utf8.RuneCountInString(agente) > longitudMaximaAgenteUsuario {
		runas := []rune(agente)
		agente = string(runas[:longitudMaximaAgenteUsuario])
	}
	return OrigenSolicitud{
		ip:                ip,
		agenteUsuario:     agente,
		huellaDispositivo: strings.TrimSpace(huellaDispositivo),
		idSolicitud:       strings.TrimSpace(idSolicitud),
	}, nil
}

// IP devuelve la dirección IP de origen (puede estar vacía).
func (o OrigenSolicitud) IP() DireccionIP { return o.ip }

// AgenteUsuario devuelve el User-Agent, truncado a 512 caracteres.
func (o OrigenSolicitud) AgenteUsuario() string { return o.agenteUsuario }

// HuellaDispositivo devuelve la huella de fingerprinting del cliente.
func (o OrigenSolicitud) HuellaDispositivo() string { return o.huellaDispositivo }

// IDSolicitud devuelve el identificador de correlación de la solicitud.
func (o OrigenSolicitud) IDSolicitud() string { return o.idSolicitud }

// ErrDireccionIPInvalida se produce al construir una DireccionIP con un
// valor no vacío pero irreconocible.
type ErrDireccionIPInvalida struct{ Motivo string }

func (e *ErrDireccionIPInvalida) Error() string { return "dirección IP inválida: " + e.Motivo }
