package aplicacion_test

// Fixtures compartidos entre los tests de los casos de uso de aplicacion.
// Viven en package aplicacion_test (caja negra): los tests solo ejercen
// la API pública de aplicacion/puertos/dominio, tal como lo haría un
// consumidor real (p. ej. el adaptador HTTP).

import (
	"testing"
	"time"

	"github.com/r-david1/moterus/internal/acceso/dominio"
)

// idUsuarioValido1/2 son dos UUIDs sintácticamente válidos y distintos.
const (
	idUsuarioValido1 = "123e4567-e89b-12d3-a456-426614174000"
	idUsuarioValido2 = "223e4567-e89b-12d3-a456-426614174001"
)

// idSesionValido1/2/3 son tres UUIDs sintácticamente válidos y distintos.
const (
	idSesionValido1 = "018e7e6a-0000-7000-8000-000000000001"
	idSesionValido2 = "018e7e6a-0000-7000-8000-000000000002"
	idSesionValido3 = "018e7e6a-0000-7000-8000-000000000003"
)

// tokenRefrescoValor1/2 son dos tokens de refresco en claro con forma
// válida (prefijo mot_rt_ + 43 caracteres base64url) y distintos.
const (
	tokenRefrescoValor1 = "mot_rt_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	tokenRefrescoValor2 = "mot_rt_BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"
)

func ahoraDePrueba() time.Time {
	return time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
}

func idUsuarioDePrueba(t *testing.T, uuid string) dominio.IDUsuario {
	t.Helper()
	id, err := dominio.IDUsuarioDesde(uuid)
	if err != nil {
		t.Fatalf("no se pudo construir el IDUsuario de prueba %q: %v", uuid, err)
	}
	return id
}

func idSesionDePrueba(t *testing.T, uuid string) dominio.IDSesion {
	t.Helper()
	id, err := dominio.IDSesionDesde(uuid)
	if err != nil {
		t.Fatalf("no se pudo construir el IDSesion de prueba %q: %v", uuid, err)
	}
	return id
}

func origenDePrueba(t *testing.T) dominio.OrigenSolicitud {
	t.Helper()
	o, err := dominio.NuevoOrigenSolicitud("203.0.113.5", "agente-test/1.0", "huella-test", "req-123")
	if err != nil {
		t.Fatalf("no se pudo construir el origen de prueba: %v", err)
	}
	return o
}

// politicaDePrueba es una PoliticaSesion válida con ventanas cómodas para
// manipular en los tests: vidaTokenAcceso corta (10 min, el mínimo
// estructural real), ventanas de refresco/inactividad/absoluta separadas
// para poder construir sesiones "todavía vigentes" y "ya expiradas" sin
// rozar los límites, y un máximo de 2 sesiones activas para poder ejercer
// INV-ACC-22 sin fixtures larguísimos.
func politicaDePrueba(t *testing.T) dominio.PoliticaSesion {
	t.Helper()
	p, err := dominio.NuevaPoliticaSesion(
		10*time.Minute,
		24*time.Hour,
		24*time.Hour,
		48*time.Hour,
		60*time.Second,
		2,
	)
	if err != nil {
		t.Fatalf("no se pudo construir la política de sesión de prueba: %v", err)
	}
	return p
}

func tokenRefrescoPlanoDePrueba(t *testing.T, valor string) dominio.TokenRefrescoPlano {
	t.Helper()
	tok, err := dominio.NuevoTokenRefrescoPlano(valor)
	if err != nil {
		t.Fatalf("no se pudo construir el token de refresco de prueba: %v", err)
	}
	return tok
}

// sesionActivaDePrueba construye, vía dominio.Reconstituir, una Sesion
// activa con un único token de refresco vigente cuyo hash es el de
// hashVigenteValor. No acumula eventos (Reconstituir nunca lo hace): es
// la rehidratación de un episodio ya persistido, tal como lo devolvería
// RepositorioSesiones en un test.
func sesionActivaDePrueba(
	t *testing.T,
	id dominio.IDSesion,
	usuarioID dominio.IDUsuario,
	generacion int,
	hashVigenteValor string,
	creadaEn time.Time,
	expiraInactividadEn time.Time,
	expiraAbsolutoEn time.Time,
) *dominio.Sesion {
	t.Helper()
	hash := tokenRefrescoPlanoDePrueba(t, hashVigenteValor).Hash()
	tok := dominio.ReconstituirTokenRefrescoEmitido(hash, generacion, creadaEn, expiraInactividadEn, nil, nil)
	return dominio.Reconstituir(
		id, usuarioID, dominio.EstadoSesionActiva, generacion, &tok,
		origenDePrueba(t), creadaEn, creadaEn, nil,
		expiraInactividadEn, expiraAbsolutoEn, nil, dominio.MotivoRevocacion{},
	)
}

// sesionRevocadaDePrueba construye una Sesion ya revocada, con un motivo
// del catálogo cerrado, opcionalmente con un token de refresco todavía no
// consumido (el escenario en el que se revocó la sesión sin que su
// refresco vigente llegara a rotarse, p. ej. un logout individual).
func sesionRevocadaDePrueba(
	t *testing.T,
	id dominio.IDSesion,
	usuarioID dominio.IDUsuario,
	hashVigenteValor string,
	motivo dominio.MotivoRevocacion,
	creadaEn time.Time,
	revocadaEn time.Time,
	expiraAbsolutoEn time.Time,
) *dominio.Sesion {
	t.Helper()
	hash := tokenRefrescoPlanoDePrueba(t, hashVigenteValor).Hash()
	tok := dominio.ReconstituirTokenRefrescoEmitido(hash, 0, creadaEn, expiraAbsolutoEn, nil, nil)
	return dominio.Reconstituir(
		id, usuarioID, dominio.EstadoSesionRevocada, 0, &tok,
		origenDePrueba(t), creadaEn, revocadaEn, nil,
		expiraAbsolutoEn, expiraAbsolutoEn, &revocadaEn, motivo,
	)
}
