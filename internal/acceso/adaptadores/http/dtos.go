package http

import "time"

// Los DTOs de este archivo son la única frontera de serialización HTTP del
// contexto Acceso. Nunca envuelven el agregado dominio.Sesion ni ningún
// value object directamente: se proyectan sus campos primitivos a JSON con
// las anotaciones de validación de Huma v2 (ADR 0006).

// --- POST /acceso/sesiones (login) ------------------------------------------

type iniciarSesionPeticion struct {
	Correo     string `json:"correo" doc:"Correo electrónico." example:"ana@ejemplo.com"`
	Contrasena string `json:"contrasena" doc:"Contraseña en texto claro. Nunca se registra en logs." example:"correcto caballo bateria grapa"`
	// TokenCaptcha se reenvía a Identidad, que es quien evalúa Confianza en
	// el flujo de login (§0 del diseño: Acceso no evalúa Confianza en
	// IniciarSesion).
	TokenCaptcha string `json:"token_captcha,omitempty" doc:"Token de captcha invisible (Cloudflare Turnstile), si el cliente lo resolvió."`
}

// IniciarSesionInput es el input Huma del login.
type IniciarSesionInput struct {
	Body iniciarSesionPeticion
}

// resultadoSesionRespuesta es la proyección 1:1 de puertos.ResultadoSesion
// (§2.1 del diseño), común a login y renovación. TokenRefresco viaja en el
// cuerpo JSON (transporte por cookie es ADR candidato 0021, sin resolver
// — ver el informe de la tarea): el caller lo recibe UNA vez y lo olvida.
type resultadoSesionRespuesta struct {
	TokenAcceso      string    `json:"token_acceso"`
	TipoToken        string    `json:"tipo_token" example:"Bearer"`
	ExpiraEnSegundos int       `json:"expira_en_segundos"`
	TokenRefresco    string    `json:"token_refresco"`
	RefrescoExpiraEn time.Time `json:"refresco_expira_en"`
	IDSesion         string    `json:"id_sesion"`
	IDUsuario        string    `json:"id_usuario"`
	SesionExpiraEn   time.Time `json:"sesion_expira_en"`
}

// IniciarSesionOutput es el output Huma del login. 201: nace un recurso
// nuevo (la sesión).
type IniciarSesionOutput struct {
	Body resultadoSesionRespuesta
}

// --- POST /acceso/sesiones/segundo-factor (completar login con MFA) --------

// completarSegundoFactorPeticion es el cuerpo de la solicitud (§3.6 del
// diseño otp-mfa.md): el token de step-up recibido en el 401 de
// POST /acceso/sesiones y el código OTP (TOTP o de respaldo) presentado por
// el usuario. Sin credencial Bearer: el propio token de step-up es la
// credencial de este endpoint (INV-MFA-03, distinto de un token de acceso
// normal).
type completarSegundoFactorPeticion struct {
	TokenStepUp string `json:"token_step_up" minLength:"1" doc:"Token de step-up devuelto por POST /acceso/sesiones cuando exige un segundo factor."`
	Codigo      string `json:"codigo" minLength:"6" maxLength:"10" doc:"Código TOTP (6 dígitos) o de respaldo (10 caracteres)." example:"123456"`
}

// CompletarSegundoFactorInput es el input Huma del paso 2 del login con MFA.
type CompletarSegundoFactorInput struct {
	Body completarSegundoFactorPeticion
}

// CompletarSegundoFactorOutput es el output Huma: la misma sesión completa
// que emite el login normal, con amr=["pwd","otp"] (§3.6 del diseño).
type CompletarSegundoFactorOutput struct {
	Body resultadoSesionRespuesta
}

// --- POST /acceso/sesiones/renovaciones (rotación de refresco) -------------

type renovarSesionPeticion struct {
	TokenRefresco string `json:"token_refresco" minLength:"1" doc:"Token de refresco opaco (prefijo mot_rt_) recibido en el login o en la renovación anterior."`
}

// RenovarSesionInput es el input Huma de la renovación.
type RenovarSesionInput struct {
	Body renovarSesionPeticion
}

// RenovarSesionOutput es el output Huma de la renovación.
type RenovarSesionOutput struct {
	Body resultadoSesionRespuesta
}

// --- DELETE /acceso/sesiones/actual (logout individual, sesión propia) -----

// CerrarSesionActualInput es el input Huma del logout de la sesión propia.
// Sin campos: el IDSesion se resuelve del token Bearer ya validado
// (INV-ACC-23).
type CerrarSesionActualInput struct{}

// CerrarSesionActualOutput es el output Huma. Cuerpo vacío (204).
type CerrarSesionActualOutput struct{}

// --- DELETE /acceso/sesiones/{id} (logout de una sesión concreta) ----------

// CerrarSesionInput es el input Huma del logout de una sesión por id.
type CerrarSesionInput struct {
	ID string `path:"id" format:"uuid" doc:"Identificador (UUID) de la sesión a cerrar."`
}

// CerrarSesionOutput es el output Huma. Cuerpo vacío (204).
type CerrarSesionOutput struct{}

// --- DELETE /acceso/sesiones (logout de todos los dispositivos) ------------

// CerrarTodasLasSesionesInput es el input Huma del logout masivo. Sin
// campos de cuerpo: cierra TODAS las sesiones activas del usuario,
// incluida la que hizo la petición (decisión de diseño — ver el informe de
// la tarea, la sección 7 del documento de diseño no especifica un
// mecanismo de "preservar la actual" para esta ruta).
type CerrarTodasLasSesionesInput struct{}

// CerrarTodasLasSesionesOutput es el output Huma. Cuerpo vacío (204).
type CerrarTodasLasSesionesOutput struct{}

// --- GET /acceso/sesiones (listar sesiones activas) -------------------------

// ListarSesionesInput es el input Huma de la consulta de sesiones activas
// del usuario autenticado. Sin parámetros: siempre "mis sesiones".
type ListarSesionesInput struct{}

// vistaSesionRespuesta es la proyección 1:1 de puertos.VistaSesion: nunca
// expone hashes de refresco ni la cadena de rotación (estructural, no
// disciplinario).
type vistaSesionRespuesta struct {
	ID                 string     `json:"id"`
	EsSesionActual     bool       `json:"es_sesion_actual"`
	Estado             string     `json:"estado"`
	CreadaEn           time.Time  `json:"creada_en"`
	UltimaRenovacionEn *time.Time `json:"ultima_renovacion_en,omitempty"`
	ExpiraAbsolutoEn   time.Time  `json:"expira_absoluto_en"`
	IPOrigen           string     `json:"ip_origen,omitempty"`
	AgenteUsuario      string     `json:"agente_usuario,omitempty"`
}

// ListarSesionesOutput es el output Huma de la consulta.
type ListarSesionesOutput struct {
	Body []vistaSesionRespuesta
}

// --- GET /.well-known/jwks.json ---------------------------------------------

// JWKSInput es el input Huma del endpoint JWKS. Sin parámetros.
type JWKSInput struct{}

// jwkRespuesta es una llave del conjunto, en el formato RFC 7517. Solo se
// completan los campos aplicables al tipo de llave (OKP/Ed25519 hoy; RSA
// queda declarado para cuando el verificador cargue una llave RS256real,
// ADR 0020).
type jwkRespuesta struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	Crv string `json:"crv,omitempty"`
	X   string `json:"x,omitempty"`
	N   string `json:"n,omitempty"`
	E   string `json:"e,omitempty"`
}

type jwksRespuesta struct {
	Keys []jwkRespuesta `json:"keys"`
}

// JWKSOutput es el output Huma del endpoint JWKS, con Cache-Control
// explícito (§7 del diseño: "cacheable... pero corta para que una llave
// nueva se propague rápido").
type JWKSOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         jwksRespuesta
}
