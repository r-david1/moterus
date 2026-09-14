package puertos

import (
	"context"
	"time"

	"github.com/r-david1/moterus/internal/identidad/dominio"
)

// --- Persistencia -----------------------------------------------------------

// RepositorioUsuarios es el puerto de salida para la persistencia del
// agregado Usuario. Ningún otro contexto lee la tabla usuarios directamente
// (INV-ID-20): el acceso siempre pasa por este puerto.
type RepositorioUsuarios interface {
	Guardar(ctx context.Context, u *dominio.Usuario) error
	BuscarPorID(ctx context.Context, id dominio.IDUsuario) (*dominio.Usuario, error)
	BuscarPorCorreo(ctx context.Context, c dominio.Correo) (*dominio.Usuario, error)
}

// --- Criptografía de credenciales -------------------------------------------

// HasherContrasenas es el puerto de salida para el hashing y verificación
// criptográfica de contraseñas (ADR candidato 0008: Argon2id).
type HasherContrasenas interface {
	Hashear(ctx context.Context, p dominio.ContrasenaPlana) (dominio.HashContrasena, error)
	Verificar(ctx context.Context, h dominio.HashContrasena, p dominio.ContrasenaPlana) (bool, error)
	NecesitaRehash(h dominio.HashContrasena) bool
	// ConsumirTiempoEquivalente ejecuta un hash señuelo con el mismo coste
	// que Verificar. Se invoca cuando el correo no existe, para que el
	// tiempo de respuesta no revele la existencia de la cuenta (INV-ID-11).
	ConsumirTiempoEquivalente(ctx context.Context)
}

// VerificadorContrasenasFiltradas es el puerto de salida para consultar
// brechas de contraseñas conocidas (p. ej. HIBP). Requiere red, por eso no
// vive en PoliticaContrasena (servicio de dominio puro).
type VerificadorContrasenasFiltradas interface {
	EstaFiltrada(ctx context.Context, p dominio.ContrasenaPlana) (bool, error)
}

// --- Infraestructura neutra --------------------------------------------------

// Reloj es el puerto de salida para obtener la hora actual. El dominio
// nunca llama a time.Now() (INV-ID-10); los casos de uso lo hacen a través
// de este puerto para poder fijar el tiempo en los tests.
type Reloj interface {
	Ahora() time.Time
}

// GeneradorIDs es el puerto de salida para generar identificadores nuevos
// de usuario y de factor MFA. El dominio nunca genera UUIDs por sí mismo.
//
// NuevoIDFactorMFA se agregó junto con la extensión OTP/MFA
// (docs/design/otp-mfa.md §1.4): dominio/identificadores.go ya documentaba,
// desde antes de que este método existiera, que "la generación de IDs
// nuevos es responsabilidad del puerto GeneradorIDs de infraestructura,
// igual que IDUsuario" — es decir, el propio dominio daba por hecho que
// este método iba a existir. HabilitarMFACasoDeUso (aplicacion/
// habilitar_mfa.go) lo consume directamente; antes de que este método
// existiera usaba un shim temporal sobre NuevoIDUsuario, ya eliminado.
type GeneradorIDs interface {
	NuevoIDUsuario() (dominio.IDUsuario, error)
	NuevoIDFactorMFA() (dominio.IDFactorMFA, error)
}

// UnidadDeTrabajo es el puerto de salida que agrupa la escritura de negocio
// y el registro de auditoría en una sola transacción (ADR 0005, ADR
// candidato 0010).
type UnidadDeTrabajo interface {
	Ejecutar(ctx context.Context, fn func(ctx context.Context) error) error
}

// --- Cruce de bounded contexts (anticorrupción) ------------------------------

// EvaluadorConfianza es el puerto de salida implementado sobre el contexto
// Confianza. Ningún intento de autenticación llega al repositorio sin
// haber consultado antes a este puerto (INV-ID-12).
type EvaluadorConfianza interface {
	Evaluar(ctx context.Context, s SolicitudEvaluacion) (DecisionConfianza, error)
	RegistrarResultado(ctx context.Context, r ResultadoIntento) error
}

// RegistroAuditoria es el puerto de salida implementado sobre el contexto
// Auditoría. Un fallo al registrar la auditoría en una acción crítica
// aborta la transacción de negocio (INV-ID-15).
type RegistroAuditoria interface {
	Registrar(ctx context.Context, e dominio.EventoDominio, origen dominio.OrigenSolicitud) error
}

// --- Integración asíncrona ---------------------------------------------------

// PublicadorEventos es el puerto de salida para la integración asíncrona
// (p. ej. el envío del correo de verificación tras UsuarioRegistrado). Se
// invoca fuera de la UnidadDeTrabajo: es best-effort, no transaccional.
type PublicadorEventos interface {
	Publicar(ctx context.Context, eventos ...dominio.EventoDominio) error
}

// --- Verificación de correo (sección 3.4 del diseño) -------------------------

// GeneradorTokens genera secretos aleatorios de alta entropía para flujos
// de un solo uso (verificación de correo). No es GeneradorIDs: los IDs son
// UUIDv7 (ordenables, no secretos); estos tokens son opacos y no deben ser
// predecibles ni ordenables.
type GeneradorTokens interface {
	// Generar devuelve un token opaco, base64url, con al menos 32 bytes de
	// entropía (crypto/rand del lado del adaptador). Nunca se persiste en
	// claro (INV-ID-21): el caso de uso lo hashea antes de guardarlo.
	Generar() (string, error)
}

// RepositorioTokensVerificacion persiste el hash (nunca el token plano) del
// token de verificación de correo activo por usuario. Guardar hace upsert
// por usuarioID: un reenvío invalida el token anterior sin necesidad de un
// paso de borrado previo.
type RepositorioTokensVerificacion interface {
	Guardar(ctx context.Context, usuarioID dominio.IDUsuario, hashToken string, expiraEn time.Time) error
	BuscarPorHash(ctx context.Context, hashToken string) (usuarioID dominio.IDUsuario, expiraEn time.Time, encontrado bool, err error)
	Eliminar(ctx context.Context, usuarioID dominio.IDUsuario) error
}

// NotificadorCorreo envía el enlace/token de verificación al usuario.
// Implementación de producción: NotificadorCorreoTransaccional, vía SMTP
// genérico (ADR 0054/0055); NotificadorCorreoLog es el fallback de
// desarrollo.
type NotificadorCorreo interface {
	EnviarVerificacion(ctx context.Context, correo dominio.Correo, tokenPlano string) error
}

// --- Tipos de apoyo de los puertos de cruce ---------------------------------
//
// Viven en puertos, no en dominio, porque son el contrato con otro contexto
// y no lenguaje ubicuo de Identidad.

// SolicitudEvaluacion es la entrada de EvaluadorConfianza.Evaluar.
type SolicitudEvaluacion struct {
	Accion            string // "login" | "registro" | "reset_contrasena"
	CorreoNormalizado string // clave de rate limit por cuenta, incluso si no existe
	Origen            dominio.OrigenSolicitud
	TokenCaptcha      string // opcional; vacío si el cliente no envió
}

// DecisionConfianza es la salida de EvaluadorConfianza.Evaluar.
type DecisionConfianza struct {
	Permitido       bool
	RequiereStepUp  bool
	RequiereCaptcha bool
	Puntaje         float64
	Motivo          string
	ReintentarEn    time.Duration
}

// ResultadoIntento es la entrada de EvaluadorConfianza.RegistrarResultado.
type ResultadoIntento struct {
	Accion            string
	CorreoNormalizado string
	Origen            dominio.OrigenSolicitud
	Exitoso           bool
	UsuarioID         string // vacío si no se resolvió el usuario
}

// AutorizadorDeConsultas es el puerto de salida ESTRECHO sobre Tenencia que
// GET /identidad/usuarios/{id} consume para cerrar la autorización cruzada
// del §11.2 del diseño de Tenencia (docs/design/tenencia-bounded-context.md):
// "¿puede el solicitante ver el perfil de un usuario distinto de sí mismo,
// dentro de la organización que declara?". Implementado por
// identidad/adaptadores/tenencia — el ÚNICO paquete de Identidad autorizado
// a importar tenencia/puertos (mismo criterio que INV-ACC-19/INV-TEN-28: la
// capa anticorrupción la posee quien depende, no quien es dependido).
//
// Deliberadamente devuelve un solo bool, no una Autorizacion completa: el
// adaptador HTTP de Identidad solo necesita "sí" o "no" (ambos casos de
// fallo se colapsan en el mismo 404, §11.2) y nunca debe inspeccionar el
// motivo de denegación de otro contexto para tomar una decisión propia.
type AutorizadorDeConsultas interface {
	// PuedeConsultar responde true solo si AMBAS condiciones se cumplen:
	// (a) idSolicitante tiene el permiso "miembro.ver" en idOrganizacion, y
	// (b) idObjetivo es miembro (cualquier rol, cualquier estado de
	// membresía) de esa misma organización. Cualquier error de
	// infraestructura o de Tenencia se propaga tal cual: el llamador debe
	// tratarlo como "no se pudo autorizar" (404), nunca como "sí
	// autorizado" por defecto.
	PuedeConsultar(ctx context.Context, idSolicitante, idObjetivo, idOrganizacion string) (bool, error)
}

// --- MFA / OTP (sección 2.2 de docs/design/otp-mfa.md) ----------------------

// RepositorioFactoresMFA es el puerto de salida para la persistencia del
// agregado FactorMFA. "Confirmados", en los dos métodos de abajo, significa
// SIEMPRE "confirmado Y activo" (dominio.FactorMFA.EstaConfirmado() &&
// EstaActivo()) — un factor deshabilitado sigue con EstaConfirmado()=true
// para siempre (es un hecho histórico) pero deja de contar aquí, que es
// justamente lo que permite un HabilitarMFA posterior tras deshabilitar el
// anterior. La migración que implemente este puerto necesita una columna
// "activo" (o equivalente) además de "confirmado", no solo esta última.
type RepositorioFactoresMFA interface {
	Guardar(ctx context.Context, f *dominio.FactorMFA) error
	BuscarPorID(ctx context.Context, id dominio.IDFactorMFA) (*dominio.FactorMFA, error)
	// BuscarConfirmadosDeUsuario es el camino caliente de VerificarOTP en
	// cada login — normalmente devuelve 0 o 1 fila (el MVP no ofrece
	// múltiples factores simultáneos, aunque el esquema no lo impide para
	// el futuro).
	BuscarConfirmadosDeUsuario(ctx context.Context, u dominio.IDUsuario) ([]*dominio.FactorMFA, error)
	ContarConfirmadosDeUsuario(ctx context.Context, u dominio.IDUsuario) (int, error)
}

// GeneradorSecretoTOTP produce el secreto de alta entropía y los códigos de
// respaldo de un factor MFA. El dominio nunca genera bytes aleatorios por
// sí mismo.
type GeneradorSecretoTOTP interface {
	GenerarSecreto() (dominio.SecretoTOTPPlano, error)
	GenerarCodigosRespaldo(n int) ([]dominio.CodigoRespaldoPlano, error)
}

// CifradorSecretos cifra/descifra el secreto TOTP en reposo. A diferencia
// de HasherContrasenas (Argon2id, de un solo sentido), esto es cifrado
// SIMÉTRICO REVERSIBLE: el servidor tiene que poder leer el secreto en
// claro para computar, en cada verificación, el código TOTP esperado a
// partir de él — un hash de un solo sentido no serviría para eso. La
// implementación de infraestructura futura es AES-256-GCM, con la llave
// desde configuración (mismo patrón de gestión que ACCESO_LLAVE_FIRMA — ver
// ADR 0038 sobre dónde vive esa llave y el comportamiento fail-closed en
// producción).
type CifradorSecretos interface {
	Cifrar(secreto dominio.SecretoTOTPPlano) (dominio.SecretoTOTPCifrado, error)
	Descifrar(cifrado dominio.SecretoTOTPCifrado) (dominio.SecretoTOTPPlano, error)
}
