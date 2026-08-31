package integracion

import (
	"net/http"
	"testing"
)

// Estos tests ejercitan los tres endpoints HTTP de Identidad
// (internal/identidad/adaptadores/http) contra un servidor real cableado
// exactamente como cmd/api/main.go (ver nuevoServidorIdentidad en
// entorno_test.go), con Postgres real detrás. Los status codes esperados
// se verificaron empíricamente contra el servidor real antes de escribir
// estos tests (curl), no se infirieron leyendo el código.

type registroPeticionPrueba struct {
	Correo     string `json:"correo"`
	Contrasena string `json:"contrasena"`
}

type registroRespuestaPrueba struct {
	IDUsuario                  string `json:"id_usuario"`
	Estado                     string `json:"estado"`
	RequiereVerificacionCorreo bool   `json:"requiere_verificacion_correo"`
}

type autenticarPeticionPrueba struct {
	Correo     string `json:"correo"`
	Contrasena string `json:"contrasena"`
}

type autenticarRespuestaPrueba struct {
	IDUsuario             string  `json:"id_usuario"`
	CorreoNormalizado     string  `json:"correo_normalizado"`
	Estado                string  `json:"estado"`
	RequiereSegundoFactor bool    `json:"requiere_segundo_factor"`
	PuntajeConfianza      float64 `json:"puntaje_confianza"`
}

type vistaUsuarioRespuestaPrueba struct {
	ID       string `json:"id"`
	Correo   string `json:"correo"`
	Estado   string `json:"estado"`
	TieneMFA bool   `json:"tiene_mfa"`
}

func TestHTTP_Registro_Exitoso_QuedaEnUsuariosConEstadoPendienteVerificacion(t *testing.T) {
	pool := poolAplicacion(t)
	duenoPool := poolDueno(t)
	app := nuevoServidorIdentidad(t, pool)

	correo := correoUnico(t, "http-registro-ok")
	req := peticionJSON(t, http.MethodPost, "/identidad/usuarios", registroPeticionPrueba{
		Correo:     correo,
		Contrasena: contrasenaFuerteDePrueba,
	})

	var respuesta registroRespuestaPrueba
	status := respuestaHTTP(t, app, req, &respuesta)
	t.Cleanup(func() { borrarUsuario(t, duenoPool, respuesta.IDUsuario) })

	if status != http.StatusOK {
		t.Fatalf("POST /identidad/usuarios = %d, esperado %d (verificado empíricamente con curl)", status, http.StatusOK)
	}
	if respuesta.IDUsuario == "" {
		t.Fatalf("respuesta sin id_usuario: %+v", respuesta)
	}
	// INV-ID-07: un alta siempre nace en pendiente_verificacion.
	if respuesta.Estado != "pendiente_verificacion" {
		t.Errorf("Estado = %q, esperado %q", respuesta.Estado, "pendiente_verificacion")
	}
	if !respuesta.RequiereVerificacionCorreo {
		t.Errorf("RequiereVerificacionCorreo = false, esperado true")
	}

	// Verificación directa contra la tabla usuarios (no solo la respuesta HTTP).
	var estadoBD string
	var correoBD string
	err := duenoPool.QueryRow(t.Context(),
		`SELECT correo, estado FROM usuarios WHERE id = $1`, respuesta.IDUsuario,
	).Scan(&correoBD, &estadoBD)
	if err != nil {
		t.Fatalf("consultando usuarios tras el registro: %v", err)
	}
	if correoBD != correo {
		t.Errorf("correo persistido = %q, esperado %q", correoBD, correo)
	}
	if estadoBD != "pendiente_verificacion" {
		t.Errorf("estado persistido = %q, esperado %q", estadoBD, "pendiente_verificacion")
	}
}

func TestHTTP_Registro_CorreoDuplicado_Devuelve409(t *testing.T) {
	pool := poolAplicacion(t)
	duenoPool := poolDueno(t)
	app := nuevoServidorIdentidad(t, pool)

	correo := correoUnico(t, "http-registro-duplicado")
	cuerpo := registroPeticionPrueba{Correo: correo, Contrasena: contrasenaFuerteDePrueba}

	var primeraRespuesta registroRespuestaPrueba
	statusPrimero := respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/identidad/usuarios", cuerpo), &primeraRespuesta)
	t.Cleanup(func() { borrarUsuario(t, duenoPool, primeraRespuesta.IDUsuario) })
	if statusPrimero != http.StatusOK {
		t.Fatalf("el primer registro (setup) = %d, esperado %d", statusPrimero, http.StatusOK)
	}

	var errorRespuesta errorHumaRespuesta
	statusSegundo := respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/identidad/usuarios", cuerpo), &errorRespuesta)

	if statusSegundo != http.StatusConflict {
		t.Fatalf("POST /identidad/usuarios con correo duplicado = %d, esperado %d", statusSegundo, http.StatusConflict)
	}
	if errorRespuesta.Status != http.StatusConflict {
		t.Errorf("cuerpo.status = %d, esperado %d", errorRespuesta.Status, http.StatusConflict)
	}
}

// TestHTTP_Login_AntesDeVerificarCorreo_Devuelve403 verifica qué status
// devuelve realmente hoy un intento de login contra una cuenta recién
// registrada (pendiente_verificacion, INV-ID-06): 403 Forbidden con detalle
// "correo no verificado" (dominio.ErrCorreoNoVerificado vía
// mapearErrorDominio), no un 401 genérico — a diferencia de credenciales
// incorrectas, aquí SÍ se distingue el motivo porque ya se verificó la
// contraseña (ver autenticar_usuario.go, paso 5).
func TestHTTP_Login_AntesDeVerificarCorreo_Devuelve403(t *testing.T) {
	pool := poolAplicacion(t)
	duenoPool := poolDueno(t)
	app := nuevoServidorIdentidad(t, pool)

	correo := correoUnico(t, "http-login-no-verificado")
	var registro registroRespuestaPrueba
	statusRegistro := respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/identidad/usuarios", registroPeticionPrueba{
		Correo: correo, Contrasena: contrasenaFuerteDePrueba,
	}), &registro)
	t.Cleanup(func() { borrarUsuario(t, duenoPool, registro.IDUsuario) })
	if statusRegistro != http.StatusOK {
		t.Fatalf("el registro (setup) = %d, esperado %d", statusRegistro, http.StatusOK)
	}

	var errorRespuesta errorHumaRespuesta
	status := respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/identidad/autenticaciones", autenticarPeticionPrueba{
		Correo: correo, Contrasena: contrasenaFuerteDePrueba,
	}), &errorRespuesta)

	if status != http.StatusForbidden {
		t.Fatalf("POST /identidad/autenticaciones antes de verificar correo = %d, esperado %d", status, http.StatusForbidden)
	}
	if errorRespuesta.Detail != "correo no verificado" {
		t.Errorf("detail = %q, esperado %q", errorRespuesta.Detail, "correo no verificado")
	}
}

// TestHTTP_Login_ContrasenaIncorrectaYCorreoInexistente_MismoStatusYCuerpo
// formaliza como test automatizado INV-ID-11 (ya verificado a mano con
// curl): un login con contraseña incorrecta contra una cuenta activa real y
// un login contra un correo que no existe deben producir exactamente el
// mismo status HTTP y el mismo cuerpo — nunca deben distinguirse por el
// canal HTTP (mitigación de enumeración de cuentas / timing attack a nivel
// de aplicación).
func TestHTTP_Login_ContrasenaIncorrectaYCorreoInexistente_MismoStatusYCuerpo(t *testing.T) {
	pool := poolAplicacion(t)
	duenoPool := poolDueno(t)
	app := nuevoServidorIdentidad(t, pool)

	correoActivo := correoUnico(t, "http-login-contrasena-incorrecta")
	var registro registroRespuestaPrueba
	statusRegistro := respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/identidad/usuarios", registroPeticionPrueba{
		Correo: correoActivo, Contrasena: contrasenaFuerteDePrueba,
	}), &registro)
	t.Cleanup(func() { borrarUsuario(t, duenoPool, registro.IDUsuario) })
	if statusRegistro != http.StatusOK {
		t.Fatalf("el registro (setup) = %d, esperado %d", statusRegistro, http.StatusOK)
	}
	activarUsuario(t, duenoPool, registro.IDUsuario)

	var respuestaContrasenaIncorrecta errorHumaRespuesta
	statusContrasenaIncorrecta := respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/identidad/autenticaciones", autenticarPeticionPrueba{
		Correo: correoActivo, Contrasena: "esta-no-es-la-contrasena-correcta",
	}), &respuestaContrasenaIncorrecta)

	correoInexistente := correoUnico(t, "http-login-correo-inexistente")
	var respuestaCorreoInexistente errorHumaRespuesta
	statusCorreoInexistente := respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/identidad/autenticaciones", autenticarPeticionPrueba{
		Correo: correoInexistente, Contrasena: "cualquier-contrasena-arbitraria",
	}), &respuestaCorreoInexistente)

	if statusContrasenaIncorrecta != http.StatusUnauthorized {
		t.Errorf("status (contraseña incorrecta) = %d, esperado %d", statusContrasenaIncorrecta, http.StatusUnauthorized)
	}
	if statusCorreoInexistente != http.StatusUnauthorized {
		t.Errorf("status (correo inexistente) = %d, esperado %d", statusCorreoInexistente, http.StatusUnauthorized)
	}
	if statusContrasenaIncorrecta != statusCorreoInexistente {
		t.Fatalf("INV-ID-11 violada: status distinto entre contraseña incorrecta (%d) y correo inexistente (%d)",
			statusContrasenaIncorrecta, statusCorreoInexistente)
	}
	if respuestaContrasenaIncorrecta != respuestaCorreoInexistente {
		t.Fatalf("INV-ID-11 violada: cuerpo distinto entre contraseña incorrecta (%+v) y correo inexistente (%+v)",
			respuestaContrasenaIncorrecta, respuestaCorreoInexistente)
	}
	if respuestaContrasenaIncorrecta.Detail != "credenciales inválidas" {
		t.Errorf("detail = %q, esperado el mensaje genérico %q", respuestaContrasenaIncorrecta.Detail, "credenciales inválidas")
	}
}

func TestHTTP_Login_Exitoso_DevuelveResultadoSinToken(t *testing.T) {
	pool := poolAplicacion(t)
	duenoPool := poolDueno(t)
	app := nuevoServidorIdentidad(t, pool)

	correo := correoUnico(t, "http-login-exitoso")
	var registro registroRespuestaPrueba
	statusRegistro := respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/identidad/usuarios", registroPeticionPrueba{
		Correo: correo, Contrasena: contrasenaFuerteDePrueba,
	}), &registro)
	t.Cleanup(func() { borrarUsuario(t, duenoPool, registro.IDUsuario) })
	if statusRegistro != http.StatusOK {
		t.Fatalf("el registro (setup) = %d, esperado %d", statusRegistro, http.StatusOK)
	}
	activarUsuario(t, duenoPool, registro.IDUsuario)

	var respuesta autenticarRespuestaPrueba
	status := respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/identidad/autenticaciones", autenticarPeticionPrueba{
		Correo: correo, Contrasena: contrasenaFuerteDePrueba,
	}), &respuesta)

	if status != http.StatusOK {
		t.Fatalf("POST /identidad/autenticaciones (login exitoso) = %d, esperado %d", status, http.StatusOK)
	}
	if respuesta.IDUsuario != registro.IDUsuario {
		t.Errorf("IDUsuario = %q, esperado %q", respuesta.IDUsuario, registro.IDUsuario)
	}
	if respuesta.Estado != "activo" {
		t.Errorf("Estado = %q, esperado %q", respuesta.Estado, "activo")
	}
	if respuesta.RequiereSegundoFactor {
		t.Errorf("RequiereSegundoFactor = true, esperado false (usuario sin MFA y sin step-up de Confianza no-op)")
	}
}

func TestHTTP_ConsultaUsuario_PorIDExistente_Devuelve200(t *testing.T) {
	pool := poolAplicacion(t)
	duenoPool := poolDueno(t)
	app := nuevoServidorIdentidad(t, pool)

	correo := correoUnico(t, "http-consulta-existente")
	var registro registroRespuestaPrueba
	statusRegistro := respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/identidad/usuarios", registroPeticionPrueba{
		Correo: correo, Contrasena: contrasenaFuerteDePrueba,
	}), &registro)
	t.Cleanup(func() { borrarUsuario(t, duenoPool, registro.IDUsuario) })
	if statusRegistro != http.StatusOK {
		t.Fatalf("el registro (setup) = %d, esperado %d", statusRegistro, http.StatusOK)
	}

	var vista vistaUsuarioRespuestaPrueba
	status := respuestaHTTP(t, app, peticionJSON(t, http.MethodGet, "/identidad/usuarios/"+registro.IDUsuario, nil), &vista)

	if status != http.StatusOK {
		t.Fatalf("GET /identidad/usuarios/{id} existente = %d, esperado %d", status, http.StatusOK)
	}
	if vista.ID != registro.IDUsuario {
		t.Errorf("ID = %q, esperado %q", vista.ID, registro.IDUsuario)
	}
	if vista.Correo != correo {
		t.Errorf("Correo = %q, esperado %q", vista.Correo, correo)
	}
}

func TestHTTP_ConsultaUsuario_PorIDInexistente_Devuelve404(t *testing.T) {
	pool := poolAplicacion(t)
	app := nuevoServidorIdentidad(t, pool)

	// UUID sintácticamente válido pero que nunca se persistió.
	idInexistente := nuevoUsuarioDePrueba(t, correoUnico(t, "consulta-404")).ID().String()

	var errorRespuesta errorHumaRespuesta
	status := respuestaHTTP(t, app, peticionJSON(t, http.MethodGet, "/identidad/usuarios/"+idInexistente, nil), &errorRespuesta)

	if status != http.StatusNotFound {
		t.Fatalf("GET /identidad/usuarios/{id} inexistente = %d, esperado %d", status, http.StatusNotFound)
	}
}

func TestHTTP_ConsultaUsuario_PorIDConFormatoInvalido_Devuelve422(t *testing.T) {
	pool := poolAplicacion(t)
	app := nuevoServidorIdentidad(t, pool)

	var errorRespuesta errorHumaRespuesta
	status := respuestaHTTP(t, app, peticionJSON(t, http.MethodGet, "/identidad/usuarios/esto-no-es-un-uuid", nil), &errorRespuesta)

	if status != http.StatusUnprocessableEntity {
		t.Fatalf("GET /identidad/usuarios/{id} con formato inválido = %d, esperado %d", status, http.StatusUnprocessableEntity)
	}
}
