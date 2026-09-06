package http

import "time"

// Los DTOs de este archivo son la única frontera de serialización HTTP del
// contexto Identidad. Nunca envuelven el agregado dominio.Usuario ni ningún
// value object directamente (VistaUsuario ya es el modelo de lectura que
// garantiza eso a nivel de aplicacion — ver puertos/entrada.go); aquí solo
// se proyectan sus campos primitivos a JSON con las anotaciones de
// validación de Huma v2 (ADR 0006: Huma genera el OpenAPI y valida a partir
// de estos tags, no se mantiene una spec a mano ni se usa
// go-playground/validator).

// --- POST /identidad/usuarios (registro) ------------------------------------

// registroPeticion es el cuerpo de la solicitud de alta de usuario.
type registroPeticion struct {
	Correo string `json:"correo" format:"email" doc:"Correo electrónico del usuario. Se normaliza a minúsculas." example:"ana@ejemplo.com"`
	// La fortaleza de la contraseña (mínimo 12 caracteres, sin reglas de
	// composición, NIST SP 800-63B) la valida dominio.PoliticaContrasena,
	// no Huma: solo se acota aquí la longitud máxima estructural
	// (dominio.ContrasenaPlana, 4096 bytes, previene DoS de hashing) para no
	// duplicar una regla de negocio que puede cambiar sin tocar este DTO.
	Contrasena string `json:"contrasena" minLength:"1" maxLength:"4096" doc:"Contraseña en texto claro. Nunca se registra en logs ni se devuelve en ninguna respuesta." example:"correcto caballo bateria grapa"`
}

// RegistroInput es el input Huma de la operación de registro.
type RegistroInput struct {
	Body registroPeticion
}

// resultadoRegistroRespuesta es el cuerpo de la respuesta de un registro
// exitoso. Proyección 1:1 de puertos.ResultadoRegistro.
type resultadoRegistroRespuesta struct {
	IDUsuario                  string `json:"id_usuario" doc:"Identificador (UUIDv7) del usuario recién creado."`
	Estado                     string `json:"estado" doc:"Estado del ciclo de vida de la cuenta. Un alta siempre nace en pendiente_verificacion (INV-ID-07)."`
	RequiereVerificacionCorreo bool   `json:"requiere_verificacion_correo" doc:"Indica si el flujo de verificación de correo (fuera de alcance del MVP) debe completarse antes de poder iniciar sesión."`
}

// RegistroOutput es el output Huma de la operación de registro.
type RegistroOutput struct {
	Body resultadoRegistroRespuesta
}

// --- POST /identidad/autenticaciones (verificación de credenciales) --------

// autenticarPeticion es el cuerpo de la solicitud de verificación de
// credenciales. Deliberadamente sin format:"email" ni minLength en Correo o
// Contrasena (a diferencia de registroPeticion): cualquier restricción de
// Huma que rechazara la petición ANTES de llegar a
// AutenticadorDeCredenciales.Autenticar devolvería un 422 en vez del 401
// genérico, filtrando información por el canal HTTP que el propio caso de
// uso ya se esfuerza en no filtrar (INV-ID-11: correo malformado y correo
// inexistente deben producir la misma respuesta).
type autenticarPeticion struct {
	Correo     string `json:"correo" doc:"Correo electrónico." example:"ana@ejemplo.com"`
	Contrasena string `json:"contrasena" doc:"Contraseña en texto claro. Nunca se registra en logs." example:"correcto caballo bateria grapa"`
}

// AutenticarInput es el input Huma de la operación de autenticación.
type AutenticarInput struct {
	Body autenticarPeticion
}

// resultadoAutenticacionRespuesta es el cuerpo de la respuesta de una
// autenticación exitosa. Proyección 1:1 de puertos.ResultadoAutenticacion:
// deliberadamente NO incluye ningún token ni dato de sesión (ADR 0009,
// INV-ID-14) — Identidad nunca emite JWT; ese es el contrato que el futuro
// contexto Acceso consumirá para orquestar el login real y devolver un
// token al cliente.
type resultadoAutenticacionRespuesta struct {
	IDUsuario             string  `json:"id_usuario"`
	CorreoNormalizado     string  `json:"correo_normalizado"`
	Estado                string  `json:"estado"`
	RequiereSegundoFactor bool    `json:"requiere_segundo_factor" doc:"Si es true, el cliente debe completar un paso adicional (MFA o step-up de Confianza) antes de que Acceso emita una sesión completa."`
	MotivoStepUp          string  `json:"motivo_step_up,omitempty" doc:"mfa_habilitado | confianza_baja | vacío."`
	PuntajeConfianza      float64 `json:"puntaje_confianza"`
}

// AutenticarOutput es el output Huma de la operación de autenticación.
type AutenticarOutput struct {
	Body resultadoAutenticacionRespuesta
}

// --- GET /identidad/usuarios/{id} (consulta) --------------------------------

// ConsultaUsuarioInput es el input Huma de la consulta de usuario por ID.
//
// Cambio de contrato (§11.2 del diseño de Tenencia,
// docs/design/tenencia-bounded-context.md): el parámetro de query
// `solicitante_id` DEJA DE TENER EFECTO. Quién pregunta ya no lo dice el
// cliente: sale siempre del token Bearer ya validado
// (middlewareAutenticacionAcceso), exactamente lo que INV-TEN-12/
// INV-ACC-23 exigen. Ver internal/identidad/README.md.
type ConsultaUsuarioInput struct {
	ID string `path:"id" format:"uuid" doc:"Identificador (UUID) del usuario a consultar."`
	// OrganizacionID es la organización desde la que se pregunta, cuando el
	// solicitante consulta a un tercero (no su propio perfil). Sin ella, un
	// solicitante distinto del objetivo recibe siempre 404: no hay ningún
	// contexto en el que Tenencia pueda autorizar la consulta. Con ella, se
	// exige miembro.ver del solicitante Y que el objetivo sea miembro de
	// esa misma organización.
	OrganizacionID string `query:"organizacion_id" format:"uuid" doc:"Organización desde la que se consulta a un tercero (irrelevante al consultar el propio perfil)."`
}

// vistaUsuarioRespuesta es el cuerpo de la respuesta de la consulta.
// Proyección 1:1 de puertos.VistaUsuario: nunca el agregado dominio.Usuario,
// así que el hash de contraseña es estructuralmente imposible de filtrar
// aquí (sección 3.3 del diseño).
type vistaUsuarioRespuesta struct {
	ID             string     `json:"id"`
	Correo         string     `json:"correo"`
	Estado         string     `json:"estado"`
	TieneMFA       bool       `json:"tiene_mfa"`
	CreadoEn       time.Time  `json:"creado_en"`
	UltimoAccesoEn *time.Time `json:"ultimo_acceso_en,omitempty"`
}

// ConsultaUsuarioOutput es el output Huma de la consulta de usuario.
type ConsultaUsuarioOutput struct {
	Body vistaUsuarioRespuesta
}

// --- POST /identidad/verificaciones-correo (sección 3.4 del diseño) --------

// verificarCorreoPeticion es el cuerpo de la solicitud de verificación de
// correo: solo el token en claro recibido por el usuario (por el canal que
// NotificadorCorreo haya usado). Nunca se valida su forma aquí (longitud,
// charset): el caso de uso lo hashea y busca por hash, así que cualquier
// valor produce el mismo camino (token inválido) sin filtrar información
// estructural por el canal HTTP.
type verificarCorreoPeticion struct {
	Token string `json:"token" minLength:"1" doc:"Token de verificación de correo recibido por el usuario."`
}

// VerificarCorreoInput es el input Huma de la operación de verificación.
type VerificarCorreoInput struct {
	Body verificarCorreoPeticion
}

// VerificarCorreoOutput es el output Huma de la operación de verificación.
// Cuerpo vacío a propósito: éxito es únicamente "la transición ocurrió"
// (INV-ID-21: el token nunca viaja de vuelta en ninguna respuesta).
type VerificarCorreoOutput struct{}

// --- POST /identidad/verificaciones-correo/reenvios (sección 3.4) ----------

// reenviarVerificacionPeticion es el cuerpo de la solicitud de reenvío.
// Deliberadamente sin format:"email" (mismo criterio que
// autenticarPeticion): un correo malformado no debe distinguirse de uno
// inexistente ni siquiera por el status de validación de Huma (INV-ID-22,
// mismo criterio anti-enumeración que INV-ID-11).
type reenviarVerificacionPeticion struct {
	Correo string `json:"correo" doc:"Correo electrónico al que reenviar el token de verificación, si corresponde." example:"ana@ejemplo.com"`
}

// ReenviarVerificacionInput es el input Huma de la operación de reenvío.
type ReenviarVerificacionInput struct {
	Body reenviarVerificacionPeticion
}

// ReenviarVerificacionOutput es el output Huma de la operación de reenvío.
// Cuerpo vacío a propósito (INV-ID-22): la respuesta es siempre 202
// Accepted sin ningún cuerpo distintivo, exista o no el correo.
type ReenviarVerificacionOutput struct{}

// --- POST /identidad/usuarios/actual/factores-mfa (habilitar MFA) ----------
// --- POST .../factores-mfa/confirmacion (confirmar)                  ------
// --- DELETE /identidad/usuarios/actual/factores-mfa (deshabilitar)   ------
//
// Los tres endpoints (§7 del diseño otp-mfa.md) actúan siempre sobre el
// propio sujeto autenticado: ninguno lleva un ID en la ruta ni en el
// cuerpo, IDSujeto sale siempre de accesoDesdeContexto (el token Bearer ya
// validado), nunca de un parámetro que el cliente controle.

// HabilitarMFAInput es el input Huma de habilitar MFA. Sin cuerpo: no hay
// ningún parámetro que el cliente deba enviar (§3.1 del diseño).
type HabilitarMFAInput struct{}

// resultadoHabilitarMFARespuesta es el cuerpo de la respuesta de habilitar
// MFA. Proyección 1:1 de puertos.ResultadoHabilitarMFA: lleva el secreto EN
// CLARO y la URI de provisionamiento, la única vez que salen del proceso
// (INV-MFA-02).
type resultadoHabilitarMFARespuesta struct {
	IDFactor            string `json:"id_factor" doc:"Identificador del factor MFA recién creado, sin confirmar todavía."`
	SecretoEnClaro      string `json:"secreto_en_claro" doc:"Secreto TOTP en Base32, para mostrar como texto además/en vez del QR. Nunca vuelve a estar disponible tras esta respuesta."`
	URIProvisionamiento string `json:"uri_provisionamiento" doc:"URI otpauth://totp/... para generar el código QR que la app autenticadora del usuario escanea."`
}

// HabilitarMFAOutput es el output Huma de habilitar MFA.
type HabilitarMFAOutput struct {
	Body resultadoHabilitarMFARespuesta
}

// confirmarFactorMFAPeticion es el cuerpo de la solicitud de confirmación:
// el ID del factor recién habilitado y el primer código TOTP leído de la
// app autenticadora.
type confirmarFactorMFAPeticion struct {
	IDFactor string `json:"id_factor" format:"uuid" doc:"Identificador del factor MFA devuelto por el endpoint de habilitación."`
	Codigo   string `json:"codigo" minLength:"6" maxLength:"6" doc:"Código TOTP de 6 dígitos mostrado por la app autenticadora." example:"123456"`
}

// ConfirmarFactorMFAInput es el input Huma de la confirmación de un factor.
type ConfirmarFactorMFAInput struct {
	Body confirmarFactorMFAPeticion
}

// resultadoConfirmarMFARespuesta es el cuerpo de la respuesta de una
// confirmación exitosa. Proyección 1:1 de puertos.ResultadoConfirmarMFA:
// lleva los 10 códigos de respaldo EN CLARO, la única vez (ADR 0040).
type resultadoConfirmarMFARespuesta struct {
	CodigosRespaldo []string `json:"codigos_respaldo" doc:"Los 10 códigos de respaldo de un solo uso, en claro. Guárdalos ahora: no se vuelven a mostrar (solo su hash queda en el servidor)."`
}

// ConfirmarFactorMFAOutput es el output Huma de la confirmación.
type ConfirmarFactorMFAOutput struct {
	Body resultadoConfirmarMFARespuesta
}

// deshabilitarMFAPeticion es el cuerpo de la solicitud de deshabilitación:
// exige un código propio del factor (TOTP vigente o de respaldo no usado),
// no basta con el Bearer (ADR 0039, INV-MFA-05).
type deshabilitarMFAPeticion struct {
	Codigo string `json:"codigo" minLength:"6" maxLength:"10" doc:"Código TOTP (6 dígitos) o de respaldo (10 caracteres) del factor que se va a deshabilitar."`
}

// DeshabilitarMFAInput es el input Huma de deshabilitar MFA.
type DeshabilitarMFAInput struct {
	Body deshabilitarMFAPeticion
}

// DeshabilitarMFAOutput es el output Huma de deshabilitar MFA. Cuerpo vacío
// (204): no hay nada más que confirmar que la baja ocurrió.
type DeshabilitarMFAOutput struct{}
