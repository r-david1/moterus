package cripto_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/r-david1/moterus/internal/identidad/adaptadores/cripto"
)

// contraseñaConocida es "password" — su SHA-1 es
// 5BAA61E4C9B93F3F0682250B6CF8331B7EE68FD8, prefijo 5BAA6, sufijo
// 1E4C9B93F3F0682250B6CF8331B7EE68FD8. Es el ejemplo canónico de la
// documentación de la API de HIBP.
const contrasenaConocida = "password"
const sufijoContrasenaConocida = "1E4C9B93F3F0682250B6CF8331B7EE68FD8"

func TestVerificadorHIBP_ContrasenaFiltrada(t *testing.T) {
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/5BAA6") {
			t.Errorf("se esperaba el prefijo 5BAA6 en la ruta, se recibió %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(sufijoContrasenaConocida + ":3861493\r\nOTROSUFIJO00000000000000000000000:1\r\n"))
	}))
	defer servidor.Close()

	v := cripto.NuevoVerificadorHIBP(cripto.ConBaseURLHIBP(servidor.URL + "/range/"))
	filtrada, err := v.EstaFiltrada(context.Background(), contrasenaDePrueba(t, contrasenaConocida))
	if err != nil {
		t.Fatalf("EstaFiltrada() error = %v", err)
	}
	if !filtrada {
		t.Fatal("EstaFiltrada() = false para una contraseña que aparece en la respuesta simulada de HIBP")
	}
}

func TestVerificadorHIBP_ContrasenaNoFiltrada(t *testing.T) {
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("OTROSUFIJO00000000000000000000000:1\r\n"))
	}))
	defer servidor.Close()

	v := cripto.NuevoVerificadorHIBP(cripto.ConBaseURLHIBP(servidor.URL + "/range/"))
	filtrada, err := v.EstaFiltrada(context.Background(), contrasenaDePrueba(t, "una-contrasena-muy-poco-comun-xyz"))
	if err != nil {
		t.Fatalf("EstaFiltrada() error = %v", err)
	}
	if filtrada {
		t.Fatal("EstaFiltrada() = true para una contraseña ausente de la respuesta simulada de HIBP")
	}
}

// TestVerificadorHIBP_FalloDeRedDevuelveError documenta el contrato
// fail-open: este adaptador NUNCA decide por sí mismo que "no está
// filtrada" ante un error de red — devuelve el error real, y es
// identidad/aplicacion.RegistrarUsuarioCasoDeUso quien decide continuar el
// registro pese al error (ver comentario del paso 4 en registrar_usuario.go).
func TestVerificadorHIBP_FalloDeRedDevuelveError(t *testing.T) {
	cliente := &http.Client{Timeout: 200 * time.Millisecond}
	v := cripto.NuevoVerificadorHIBP(
		cripto.ConBaseURLHIBP("http://127.0.0.1:0/range/"), // puerto 0: nadie escucha ahí.
		cripto.ConClienteHIBP(cliente),
	)
	_, err := v.EstaFiltrada(context.Background(), contrasenaDePrueba(t, "cualquier-cosa"))
	if err == nil {
		t.Fatal("EstaFiltrada() no devolvió error ante un servidor inalcanzable")
	}
}

func TestVerificadorHIBP_RespuestaNoOKDevuelveError(t *testing.T) {
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer servidor.Close()

	v := cripto.NuevoVerificadorHIBP(cripto.ConBaseURLHIBP(servidor.URL + "/range/"))
	_, err := v.EstaFiltrada(context.Background(), contrasenaDePrueba(t, "cualquier-cosa"))
	if err == nil {
		t.Fatal("EstaFiltrada() no devolvió error ante una respuesta 500 de HIBP")
	}
}
