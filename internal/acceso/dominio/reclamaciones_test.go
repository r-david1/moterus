package dominio

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func reclamacionesValidasDePrueba(t *testing.T) (IDUsuario, IDSesion, IDTokenAcceso, time.Time, time.Time, time.Time) {
	t.Helper()
	ahora := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	return idUsuarioDePrueba(t), idSesionDePrueba(t), idTokenAccesoDePrueba(t), ahora, ahora, ahora.Add(10 * time.Minute)
}

func TestNuevasReclamacionesAcceso_ConstruyeConCamposValidos(t *testing.T) {
	sujeto, idSesion, jti, autenticadoEn, emitidoEn, expiraEn := reclamacionesValidasDePrueba(t)
	r, err := NuevasReclamacionesAcceso("https://acceso.ejemplo.com", sujeto, "moterus", idSesion, jti, []string{"pwd"}, autenticadoEn, emitidoEn, expiraEn, 1)
	if err != nil {
		t.Fatalf("no se esperaba error, obtuvo %v", err)
	}
	if r.Emisor() != "https://acceso.ejemplo.com" {
		t.Errorf("Emisor() = %q", r.Emisor())
	}
	if !r.Sujeto().EsIgual(sujeto) {
		t.Error("Sujeto() no coincide")
	}
	if r.Audiencia() != "moterus" {
		t.Errorf("Audiencia() = %q", r.Audiencia())
	}
	if !r.IDSesion().EsIgual(idSesion) {
		t.Error("IDSesion() no coincide")
	}
	if !r.JTI().EsIgual(jti) {
		t.Error("JTI() no coincide")
	}
	if got := r.MetodosAutenticacion(); len(got) != 1 || got[0] != "pwd" {
		t.Errorf("MetodosAutenticacion() = %v, esperado [pwd]", got)
	}
	if r.Version() != 1 {
		t.Errorf("Version() = %d, esperado 1", r.Version())
	}
}

// TestNuevasReclamacionesAcceso_ExigeCamposObligatorios recorre cada campo
// obligatorio de la tabla 1.3 del diseño y verifica que su ausencia rechaza
// la construcción.
func TestNuevasReclamacionesAcceso_ExigeCamposObligatorios(t *testing.T) {
	sujeto, idSesion, jti, autenticadoEn, emitidoEn, expiraEn := reclamacionesValidasDePrueba(t)

	casos := []struct {
		nombre string
		llamar func() (ReclamacionesAcceso, error)
	}{
		{"emisor vacío", func() (ReclamacionesAcceso, error) {
			return NuevasReclamacionesAcceso("", sujeto, "moterus", idSesion, jti, []string{"pwd"}, autenticadoEn, emitidoEn, expiraEn, 1)
		}},
		{"sujeto vacío", func() (ReclamacionesAcceso, error) {
			return NuevasReclamacionesAcceso("iss", IDUsuario{}, "moterus", idSesion, jti, []string{"pwd"}, autenticadoEn, emitidoEn, expiraEn, 1)
		}},
		{"audiencia vacía", func() (ReclamacionesAcceso, error) {
			return NuevasReclamacionesAcceso("iss", sujeto, "", idSesion, jti, []string{"pwd"}, autenticadoEn, emitidoEn, expiraEn, 1)
		}},
		{"idSesion vacío", func() (ReclamacionesAcceso, error) {
			return NuevasReclamacionesAcceso("iss", sujeto, "moterus", IDSesion{}, jti, []string{"pwd"}, autenticadoEn, emitidoEn, expiraEn, 1)
		}},
		{"jti vacío", func() (ReclamacionesAcceso, error) {
			return NuevasReclamacionesAcceso("iss", sujeto, "moterus", idSesion, IDTokenAcceso{}, []string{"pwd"}, autenticadoEn, emitidoEn, expiraEn, 1)
		}},
		{"sin métodos de autenticación", func() (ReclamacionesAcceso, error) {
			return NuevasReclamacionesAcceso("iss", sujeto, "moterus", idSesion, jti, nil, autenticadoEn, emitidoEn, expiraEn, 1)
		}},
		{"expiraEn no posterior a emitidoEn", func() (ReclamacionesAcceso, error) {
			return NuevasReclamacionesAcceso("iss", sujeto, "moterus", idSesion, jti, []string{"pwd"}, autenticadoEn, emitidoEn, emitidoEn, 1)
		}},
		{"version menor a 1", func() (ReclamacionesAcceso, error) {
			return NuevasReclamacionesAcceso("iss", sujeto, "moterus", idSesion, jti, []string{"pwd"}, autenticadoEn, emitidoEn, expiraEn, 0)
		}},
	}
	for _, c := range casos {
		if _, err := c.llamar(); err == nil {
			t.Errorf("%s: se esperaba error", c.nombre)
		}
	}
}

func TestReclamacionesAcceso_MetodosAutenticacion_DevuelveCopia(t *testing.T) {
	sujeto, idSesion, jti, autenticadoEn, emitidoEn, expiraEn := reclamacionesValidasDePrueba(t)
	r, err := NuevasReclamacionesAcceso("iss", sujeto, "moterus", idSesion, jti, []string{"pwd"}, autenticadoEn, emitidoEn, expiraEn, 1)
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	amr := r.MetodosAutenticacion()
	amr[0] = "mutado"
	if r.MetodosAutenticacion()[0] != "pwd" {
		t.Error("MetodosAutenticacion() debe devolver una copia, no el slice interno")
	}
}

// TestINV_ACC_12_ReclamacionesAccesoSinAutorizacionNiPII verifica, por
// reflexión, que ReclamacionesAcceso no tiene ningún campo cuyo nombre
// sugiera roles, permisos, organización/tenant o PII (correo, IP, huella de
// dispositivo) — tabla del §7 del diseño: "claims que deliberadamente NO
// existen".
func TestINV_ACC_12_ReclamacionesAccesoSinAutorizacionNiPII(t *testing.T) {
	prohibidos := []string{"rol", "permiso", "org", "tenant", "correo", "email", "ip", "huella"}
	tipo := reflect.TypeOf(ReclamacionesAcceso{})
	for i := 0; i < tipo.NumField(); i++ {
		nombre := tipo.Field(i).Name
		nombreMin := strings.ToLower(nombre)
		for _, p := range prohibidos {
			if strings.Contains(nombreMin, p) {
				t.Errorf("ReclamacionesAcceso.%s: el nombre sugiere un claim de autorización o PII prohibido (INV-ACC-12)", nombre)
			}
		}
	}
}
