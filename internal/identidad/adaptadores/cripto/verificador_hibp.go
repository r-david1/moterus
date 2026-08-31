package cripto

import (
	"bufio"
	"context"
	"crypto/sha1" //nolint:gosec // SHA-1 es el algoritmo que exige la API k-anonymity de HIBP, no se usa con fines de integridad propios.
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/r-david1/moterus/internal/identidad/dominio"
	"github.com/r-david1/moterus/internal/identidad/puertos"
)

// hibpBaseURLPorDefecto es el endpoint público de k-anonymity de Have I
// Been Pwned: solo se envían los primeros 5 caracteres del hash SHA-1 de la
// contraseña, nunca la contraseña ni el hash completo (INV-ID-04 aplica por
// extensión: tampoco se filtra la contraseña a un tercero).
const hibpBaseURLPorDefecto = "https://api.pwnedpasswords.com/range/"

// hibpTimeoutPorDefecto es deliberadamente corto: el puerto
// VerificadorContrasenasFiltradas es fail-open (ver
// identidad/aplicacion/registrar_usuario.go, paso 4 — "una caída de HIBP no
// puede bloquear todas las altas"), así que una llamada lenta no debe
// retrasar el registro más de lo estrictamente necesario.
const hibpTimeoutPorDefecto = 3 * time.Second

// VerificadorHIBP implementa puertos.VerificadorContrasenasFiltradas contra
// la API k-anonymity de Have I Been Pwned (rango de hash SHA-1, RFC de la
// propia HIBP). Importante: este adaptador SIEMPRE devuelve el error real
// cuando la llamada de red falla; es la capa de aplicación
// (RegistrarUsuarioCasoDeUso) la que decide fail-open y continúa el
// registro pese al error — este adaptador no debe silenciar el error ni
// "fingir" que la contraseña no está filtrada, porque el llamador necesita
// distinguir "no filtrada" de "no se pudo comprobar" para poder loguearlo.
type VerificadorHIBP struct {
	cliente *http.Client
	baseURL string
}

var _ puertos.VerificadorContrasenasFiltradas = (*VerificadorHIBP)(nil)

// OpcionHIBP configura VerificadorHIBP en su construcción.
type OpcionHIBP func(*VerificadorHIBP)

// ConClienteHIBP inyecta un *http.Client propio (p. ej. para tests con un
// httptest.Server, o para ajustar el timeout en producción).
func ConClienteHIBP(cliente *http.Client) OpcionHIBP {
	return func(v *VerificadorHIBP) { v.cliente = cliente }
}

// ConBaseURLHIBP inyecta una URL base propia (p. ej. la de un
// httptest.Server en tests). Debe terminar sin barra final; el prefijo de 5
// caracteres se concatena directamente.
func ConBaseURLHIBP(baseURL string) OpcionHIBP {
	return func(v *VerificadorHIBP) { v.baseURL = baseURL }
}

// NuevoVerificadorHIBP construye el adaptador con un cliente HTTP de
// timeout corto por defecto (hibpTimeoutPorDefecto) apuntando a la API
// pública real de HIBP.
func NuevoVerificadorHIBP(opciones ...OpcionHIBP) *VerificadorHIBP {
	v := &VerificadorHIBP{
		cliente: &http.Client{Timeout: hibpTimeoutPorDefecto},
		baseURL: hibpBaseURLPorDefecto,
	}
	for _, opcion := range opciones {
		opcion(v)
	}
	return v
}

// EstaFiltrada calcula el SHA-1 de la contraseña, envía solo el prefijo de 5
// caracteres a HIBP (k-anonymity: el servidor nunca ve el hash completo ni
// la contraseña) y busca el sufijo restante en la lista de sufijos
// devuelta.
func (v *VerificadorHIBP) EstaFiltrada(ctx context.Context, p dominio.ContrasenaPlana) (bool, error) {
	suma := sha1.Sum([]byte(p.Valor())) //nolint:gosec // ver comentario del import.
	hexSuma := strings.ToUpper(hex.EncodeToString(suma[:]))
	prefijo, sufijoBuscado := hexSuma[:5], hexSuma[5:]

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.baseURL+prefijo, nil)
	if err != nil {
		return false, fmt.Errorf("cripto: no se pudo construir la solicitud a HIBP: %w", err)
	}
	req.Header.Set("User-Agent", "moterus-auth-service")
	// Add-Padding: el servidor añade líneas de relleno para que el tamaño de
	// la respuesta no filtre si la contraseña es común o rara.
	req.Header.Set("Add-Padding", "true")

	resp, err := v.cliente.Do(req)
	if err != nil {
		return false, fmt.Errorf("cripto: verificador de contraseñas filtradas no disponible: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("cripto: verificador de contraseñas filtradas respondió %d", resp.StatusCode)
	}

	lector := bufio.NewScanner(resp.Body)
	for lector.Scan() {
		sufijo, cuentaCruda, encontrado := strings.Cut(strings.TrimSpace(lector.Text()), ":")
		if !encontrado {
			continue
		}
		if !strings.EqualFold(sufijo, sufijoBuscado) {
			continue
		}
		cuenta, err := strconv.Atoi(strings.TrimSpace(cuentaCruda))
		if err != nil {
			return false, fmt.Errorf("cripto: respuesta de HIBP con formato inesperado: %w", err)
		}
		return cuenta > 0, nil
	}
	if err := lector.Err(); err != nil {
		return false, fmt.Errorf("cripto: error leyendo la respuesta de HIBP: %w", err)
	}

	return false, nil
}
