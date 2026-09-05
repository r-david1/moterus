package dominio

import (
	"fmt"
	"time"
)

// Errores de dominio tipados (tabla 1.5 del diseño del contexto Acceso).
//
// Se implementan como tipos propios (no errors.New ad hoc) para que la capa
// de aplicación pueda mapearlos a códigos HTTP inspeccionando el tipo con
// errors.As. Varios de estos errores no los produce nunca acceso/dominio
// directamente (los produce el ACL de Identidad, el adaptador Postgres o el
// propio caso de uso), pero se declaran aquí porque el vocabulario de
// errores pertenece al dominio — mismo criterio que
// identidad/dominio.ErrCorreoYaRegistrado o ErrAccesoDenegadoPorConfianza.
//
// Los errores de construcción de cada value object (formato de un
// identificador, de un token, de una política, etc.) viven junto a su VO
// (identificadores.go, tokens.go, politica_sesion.go, ...), no aquí: este
// archivo reúne los errores de negocio del agregado Sesion y los que cruzan
// hacia la capa de aplicación.

// ErrIDUsuarioInvalido se produce al construir un IDUsuario con formato
// inválido o con el UUID nulo.
type ErrIDUsuarioInvalido struct{ Motivo string }

func (e *ErrIDUsuarioInvalido) Error() string {
	return "identificador de usuario inválido: " + e.Motivo
}

// ErrIDSesionInvalido se produce al construir un IDSesion con formato
// inválido o con el UUID nulo.
type ErrIDSesionInvalido struct{ Motivo string }

func (e *ErrIDSesionInvalido) Error() string {
	return "identificador de sesión inválido: " + e.Motivo
}

// ErrIDTokenAccesoInvalido se produce al construir un IDTokenAcceso con
// formato inválido o con el UUID nulo.
type ErrIDTokenAccesoInvalido struct{ Motivo string }

func (e *ErrIDTokenAccesoInvalido) Error() string {
	return "identificador de token de acceso inválido: " + e.Motivo
}

// ErrCredencialesRechazadas se produce cuando Identidad rechazó la
// autenticación (traducción, en el ACL de acceso/adaptadores/identidad, de
// identidad/dominio.ErrCredencialesInvalidas — acceso/aplicacion nunca ve el
// tipo de Identidad, INV-ACC-19). Deliberadamente tan genérico como el
// original.
type ErrCredencialesRechazadas struct{}

func (e *ErrCredencialesRechazadas) Error() string { return "credenciales rechazadas" }

// Catálogo cerrado de ErrCuentaNoOperativa.Motivo.
const (
	MotivoCuentaNoOperativaCorreoNoVerificado = "correo_no_verificado"
	MotivoCuentaNoOperativaSuspendida         = "cuenta_suspendida"
	MotivoCuentaNoOperativaBloqueada          = "cuenta_bloqueada"
)

// ErrCuentaNoOperativa se produce cuando Identidad devolvió un estado que
// impide el login: correo no verificado, cuenta suspendida o cuenta
// bloqueada. El ACL de Identidad traduce los tres errores tipados de
// Identidad a este único tipo de Acceso.
type ErrCuentaNoOperativa struct{ Motivo string }

func (e *ErrCuentaNoOperativa) Error() string { return "cuenta no operativa: " + e.Motivo }

// ErrSegundoFactorRequerido se produce cuando Identidad indicó que la
// autenticación exige un segundo factor. No se emite sesión ni token de
// acceso alguno (INV-ACC-03). TokenStepUp lleva el JWT de vida corta (ADR
// 0038) que el cliente debe presentar en POST /acceso/sesiones/segundo-factor
// junto con el código OTP; campo aditivo, no cambia Error() ni rompe a quien
// ya construye este error solo con MotivoStepUp.
type ErrSegundoFactorRequerido struct {
	MotivoStepUp string
	TokenStepUp  string
}

func (e *ErrSegundoFactorRequerido) Error() string { return "se requiere segundo factor" }

// ErrRefrescoInvalido es el error único y deliberadamente indistinguible
// (INV-ACC-21) para las cuatro situaciones de un token de refresco no
// utilizable: desconocido, malformado, expirado o ya consumido. Separarlos
// le diría a un atacante si el token que probó existió alguna vez.
type ErrRefrescoInvalido struct{}

func (e *ErrRefrescoInvalido) Error() string { return "token de refresco inválido" }

// ErrSesionExpirada se produce cuando la ventana de inactividad o la vida
// absoluta de la sesión se agotaron. Solo debe devolverse cuando el token
// presentado sí era el vigente de esa sesión (Sesion.PuedeRenovarse ya
// aplica este orden); si no lo era, gana ErrRefrescoInvalido.
type ErrSesionExpirada struct{}

func (e *ErrSesionExpirada) Error() string { return "sesión expirada" }

// ErrSesionRevocada se produce cuando la sesión está en un estado terminal
// distinto de activa. Motivo va vacío si el estado es expirada (no revocada
// propiamente) — lleva el MotivoRevocacion del catálogo cerrado cuando
// aplica, para que el cliente pueda mostrar un mensaje útil ("tu
// contraseña cambió, ingresá de nuevo").
type ErrSesionRevocada struct{ Motivo string }

func (e *ErrSesionRevocada) Error() string { return "sesión revocada" }

// ErrReusoRefrescoDetectado es un error interno del caso de uso: nunca
// llega al cliente (el adaptador HTTP lo mapea al mismo 401 de
// ErrRefrescoInvalido). Existe como tipo propio porque dispara una acción
// de auditoría distinta (ReusoRefrescoDetectado) y la revocación de toda la
// sesión (INV-ACC-06).
type ErrReusoRefrescoDetectado struct{}

func (e *ErrReusoRefrescoDetectado) Error() string { return "reuso de token de refresco detectado" }

// ErrSesionNoEncontrada se produce en consultas o cierres por IDSesion
// inexistente. Solo debe usarse en flujos autenticados.
type ErrSesionNoEncontrada struct{ IDSesion string }

func (e *ErrSesionNoEncontrada) Error() string { return "sesión no encontrada" }

// ErrSesionAjena se produce cuando el sujeto del token intenta cerrar una
// sesión de otro usuario. Se mapea a 404, no a 403: un 403 confirmaría que
// ese IDSesion existe.
type ErrSesionAjena struct{}

func (e *ErrSesionAjena) Error() string { return "la sesión no pertenece al sujeto autenticado" }

// Catálogo cerrado de ErrTokenAccesoInvalido.Motivo.
const (
	MotivoTokenAccesoFirmaInvalida       = "firma_invalida"
	MotivoTokenAccesoKIDDesconocido      = "kid_desconocido"
	MotivoTokenAccesoAlgoritmoInesperado = "algoritmo_inesperado"
	MotivoTokenAccesoTipoIncorrecto      = "tipo_incorrecto"
	MotivoTokenAccesoEmisorInesperado    = "emisor_inesperado"
	MotivoTokenAccesoAudienciaInesperada = "audiencia_inesperada"
	MotivoTokenAccesoEstructuraCorrupta  = "estructura_corrupta"
)

// ErrTokenAccesoInvalido se produce cuando la validación de un token de
// acceso falla por firma inválida, kid desconocido, alg/typ inesperado,
// iss/aud que no coinciden, o estructura corrupta. Lleva Motivo del
// catálogo cerrado para métricas y auditoría; el cliente recibe solo
// "invalid_token".
type ErrTokenAccesoInvalido struct{ Motivo string }

func (e *ErrTokenAccesoInvalido) Error() string { return "token de acceso inválido: " + e.Motivo }

// ErrTokenAccesoExpirado se produce cuando el claim exp venció (con la
// tolerancia de reloj ya aplicada). Separado deliberadamente de
// ErrTokenAccesoInvalido: es el 401 esperable y de altísima frecuencia, y
// no se audita (INV-ACC-17).
type ErrTokenAccesoExpirado struct{}

func (e *ErrTokenAccesoExpirado) Error() string { return "token de acceso expirado" }

// ErrSesionRevocadaEnLista se produce cuando el sid del token está en la
// lista de revocación (Redis). Se distingue de ErrTokenAccesoExpirado
// porque sí es señal: alguien sigue usando un token de una sesión matada.
type ErrSesionRevocadaEnLista struct{}

func (e *ErrSesionRevocadaEnLista) Error() string { return "la sesión del token fue revocada" }

// ErrTransicionEstadoSesionInvalida se produce cuando se intenta una
// transición de EstadoSesion no permitida por la máquina de estados.
// Incluye el estado origen y el destino solicitado.
type ErrTransicionEstadoSesionInvalida struct{ Origen, Destino EstadoSesion }

func (e *ErrTransicionEstadoSesionInvalida) Error() string {
	return fmt.Sprintf("transición de estado de sesión inválida: %s -> %s", e.Origen.String(), e.Destino.String())
}

// ErrAccesoDenegadoPorConfianza se produce cuando el contexto Confianza
// bloquea una renovación o un cierre masivo. Mismo nombre y misma forma que
// el homónimo de identidad/dominio, pero tipo propio de acceso/dominio
// (§1.7 del diseño): el adaptador HTTP lo mapea a 429 con Retry-After.
type ErrAccesoDenegadoPorConfianza struct {
	Motivo       string
	ReintentarEn time.Duration
}

func (e *ErrAccesoDenegadoPorConfianza) Error() string {
	return "acceso denegado por evaluación de confianza"
}

// ErrConcurrenciaSesion se produce ante un conflicto al rotar dos veces la
// misma sesión en paralelo. Lo produce el adaptador Postgres al traducir la
// violación del índice único parcial de refresco vigente (§6 del diseño).
// Es reintentable.
type ErrConcurrenciaSesion struct{}

func (e *ErrConcurrenciaSesion) Error() string {
	return "conflicto de concurrencia al rotar la sesión"
}

// --- errores internos de invariantes del agregado (no forman parte de la
// tabla 1.5, son salvaguardas defensivas de Sesion; ver sesion.go) --------

// ErrRefrescoYaEmitido se produce si Sesion.EmitirPrimerRefresco se invoca
// más de una vez sobre el mismo agregado (INV-ACC-04: como mucho un
// refresco vigente por sesión). No debería ser alcanzable desde un caso de
// uso correcto; existe como salvaguarda.
type ErrRefrescoYaEmitido struct{}

func (e *ErrRefrescoYaEmitido) Error() string { return "la sesión ya tiene un refresco emitido" }

// ErrRefrescoNoEmitido se produce si Sesion.Rotar se invoca antes de
// Sesion.EmitirPrimerRefresco. No debería ser alcanzable desde un caso de
// uso correcto; existe como salvaguarda.
type ErrRefrescoNoEmitido struct{}

func (e *ErrRefrescoNoEmitido) Error() string {
	return "la sesión todavía no tiene un refresco emitido"
}

// ErrMotivoRevocacionRequerido se produce si Sesion.Revocar se invoca con
// un MotivoRevocacion vacío (zero value): toda revocación exige un motivo
// del catálogo cerrado (INV-ACC-08).
type ErrMotivoRevocacionRequerido struct{}

func (e *ErrMotivoRevocacionRequerido) Error() string { return "toda revocación exige un motivo" }
