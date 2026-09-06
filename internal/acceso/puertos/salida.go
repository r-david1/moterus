package puertos

import (
	"context"
	"time"

	"github.com/r-david1/moterus/internal/acceso/dominio"
)

// --- Persistencia -----------------------------------------------------------

// RepositorioSesiones es el puerto de salida para la persistencia del
// agregado Sesion. Ningún otro contexto lee la tabla sesiones directamente
// (INV-ACC-20): el acceso siempre pasa por este puerto.
type RepositorioSesiones interface {
	// Guardar persiste el agregado completo: la sesión y las mutaciones
	// pendientes de su cadena de tokens (el token recién emitido y el
	// consumido en la misma rotación, vía Sesion.RefrescoVigente/
	// RefrescoRecienConsumido). Debe ser idempotente respecto a los tokens
	// ya persistidos y sin cambios.
	Guardar(ctx context.Context, s *dominio.Sesion) error

	BuscarPorID(ctx context.Context, id dominio.IDSesion) (*dominio.Sesion, error)

	// BuscarPorHashRefresco resuelve el token presentado SIN cargar la
	// cadena completa. Devuelve la sesión, en qué situación está ese hash
	// concreto (vigente / consumido / desconocido) y la generación exacta
	// del token de refresco que coincidió con el hash buscado.
	//
	// Nota de implementación (desviación deliberada de la firma del §2.2
	// del diseño, que solo devolvía *Sesion y SituacionRefresco):
	// dominio.Sesion.Reconstituir solo transporta el token VIGENTE de la
	// sesión (cuya generación coincide con Sesion.Generacion()), así que
	// cuando la situación es RefrescoConsumido (reuso) el caso de uso
	// RenovarSesion no tiene forma de conocer, a través del agregado
	// reconstruido, la generación del token específico que fue
	// reutilizado — solo la generación vigente. Sin ese dato no se puede
	// construir dominio.NuevoReusoRefrescoDetectado(..., generacionPresentada,
	// generacionVigente, ...) (§1.6 del diseño) con información real. Se
	// añade este cuarto valor de retorno para cerrar ese hueco sin tocar
	// el agregado de dominio; generacionToken no tiene significado cuando
	// situacion == RefrescoDesconocido.
	BuscarPorHashRefresco(ctx context.Context, h dominio.HashTokenRefresco) (sesion *dominio.Sesion, situacion SituacionRefresco, generacionToken int, err error)

	ListarActivasDeUsuario(ctx context.Context, u dominio.IDUsuario) ([]*dominio.Sesion, error)
	ContarActivasDeUsuario(ctx context.Context, u dominio.IDUsuario) (int, error)

	// RevocarActivasDeUsuario es una operación de conjunto: revocar 200
	// sesiones cargando 200 agregados sería absurdo. Devuelve los IDs
	// revocados para que el caso de uso los propague a la lista de
	// revocación y emita un evento de auditoría por sesión. excepto puede
	// ir vacío (dominio.IDSesion{}, EsVacio() == true) para no preservar
	// ninguna.
	RevocarActivasDeUsuario(ctx context.Context, u dominio.IDUsuario, excepto dominio.IDSesion,
		motivo dominio.MotivoRevocacion, ahora time.Time) ([]dominio.IDSesion, error)
}

// SituacionRefresco describe en qué estado está el hash de un token de
// refresco presentado, tal como lo resuelve
// RepositorioSesiones.BuscarPorHashRefresco. Es lo único que el caso de
// uso RenovarSesion necesita para decidir entre rotar, rechazar o
// declarar reuso.
type SituacionRefresco int

const (
	// RefrescoDesconocido indica que el hash no existe en tokens_refresco.
	RefrescoDesconocido SituacionRefresco = iota
	// RefrescoVigente indica que el hash existe y no está consumido.
	RefrescoVigente
	// RefrescoConsumido indica que el hash existe y ya fue rotado: REUSO
	// (INV-ACC-06).
	RefrescoConsumido
)

// --- Criptografía de sesión --------------------------------------------------

// FirmadorTokensAcceso traduce entre las ReclamacionesAcceso del dominio y
// el formato de transporte firmado (JWT). El dominio decide QUÉ dice el
// token; este puerto decide CÓMO se codifica y con qué llave.
type FirmadorTokensAcceso interface {
	Firmar(ctx context.Context, r dominio.ReclamacionesAcceso) (string, error)
	// Verificar valida firma, alg, typ, kid, iss, aud y ventanas
	// temporales, y devuelve las reclamaciones ya tipadas. Un token que no
	// supere CUALQUIERA de esas comprobaciones produce error: no hay
	// verificación parcial. Un exp vencido (con tolerancia ya aplicada)
	// produce *dominio.ErrTokenAccesoExpirado; cualquier otro motivo
	// produce *dominio.ErrTokenAccesoInvalido{Motivo}.
	Verificar(ctx context.Context, tokenCompacto string) (dominio.ReclamacionesAcceso, error)
	LlavesPublicas(ctx context.Context) ([]LlavePublica, error)
}

// GeneradorTokensRefresco es el puerto de salida que genera tokens de
// refresco opacos.
type GeneradorTokensRefresco interface {
	// Generar devuelve un token opaco con ≥32 bytes de entropía de
	// crypto/rand, prefijado con "mot_rt_". Nunca se persiste en claro
	// (INV-ACC-11): solo su hash (TokenRefrescoPlano.Hash()) llega a la
	// base de datos.
	Generar() (dominio.TokenRefrescoPlano, error)
}

// ListaRevocacion es el ACELERADOR de la revocación, no su frontera de
// seguridad (INV-ACC-15). Respaldado por Redis (ADR 0018 ya lo trajo al
// stack). Si Redis no está disponible, Disponible() devuelve false y la
// revocación sigue siendo correcta pero tarda hasta vidaTokenAcceso.
type ListaRevocacion interface {
	RevocarSesion(ctx context.Context, idSesion dominio.IDSesion, hasta time.Time) error
	SesionRevocada(ctx context.Context, idSesion dominio.IDSesion) (bool, error)
	Disponible() bool
}

// --- Infraestructura neutra --------------------------------------------------

// Reloj es el puerto de salida para obtener la hora actual. El dominio
// nunca llama a time.Now() (INV-ACC-10); los casos de uso lo hacen a
// través de este puerto para poder fijar el tiempo en los tests.
type Reloj interface{ Ahora() time.Time }

// GeneradorIDs es el puerto de salida para generar identificadores nuevos.
// El dominio nunca genera UUIDs por sí mismo.
type GeneradorIDs interface {
	NuevoIDSesion() (dominio.IDSesion, error)           // UUIDv7
	NuevoIDTokenAcceso() (dominio.IDTokenAcceso, error) // UUIDv4
}

// UnidadDeTrabajo es el puerto de salida que agrupa la escritura de
// negocio y el registro de auditoría en una sola transacción (ADR 0005).
type UnidadDeTrabajo interface {
	Ejecutar(ctx context.Context, fn func(ctx context.Context) error) error
}

// --- Cruce de bounded contexts (anticorrupción) ------------------------------

// AutenticadorIdentidad es el ACL sobre
// identidad/puertos.AutenticadorDeCredenciales. Tipos propios de Acceso a
// ambos lados: acceso/aplicacion nunca ve un tipo de Identidad
// (INV-ACC-19). El adaptador que implemente este puerto es también quien
// traduce los errores tipados de Identidad a los de acceso/dominio
// (ErrCredencialesRechazadas, ErrCuentaNoOperativa, ErrAccesoDenegadoPorConfianza):
// por eso Autenticar aquí puede devolver directamente esos tipos.
type AutenticadorIdentidad interface {
	Autenticar(ctx context.Context, c CredencialesSujeto) (SujetoAutenticado, error)
}

// ConsultorEstadoSujeto es el ACL sobre
// identidad/puertos.ConsultorDeUsuarios. Se invoca en CADA renovación: es
// el mecanismo por el que Acceso se entera de que Identidad suspendió o
// bloqueó una cuenta sin necesidad de un broker de eventos (ADR candidato
// 0022). El adaptador debe invocar a Identidad con IDSolicitante vacío
// (llamada interna del sistema) para que Identidad no audite
// usuario.consultado en cada renovación.
type ConsultorEstadoSujeto interface {
	EstadoDe(ctx context.Context, idUsuario string) (EstadoSujeto, error)
}

// EvaluadorConfianza es el puerto de salida implementado sobre el
// contexto Confianza. Acceso lo consulta en renovación y en cierre
// masivo (§0 del diseño: en login no, porque Identidad ya lo hizo dentro
// de AutenticarUsuario).
type EvaluadorConfianza interface {
	Evaluar(ctx context.Context, s SolicitudEvaluacion) (DecisionConfianza, error)
	RegistrarResultado(ctx context.Context, r ResultadoIntento) error
}

// RegistroAuditoria es el puerto de salida implementado sobre el contexto
// Auditoría. Un fallo al registrar la auditoría en una acción crítica
// aborta la transacción de negocio (INV-ACC-17).
type RegistroAuditoria interface {
	Registrar(ctx context.Context, e dominio.EventoDominio, origen dominio.OrigenSolicitud) error
}

// PublicadorEventos es el puerto de salida para la integración asíncrona.
// Se invoca fuera de la UnidadDeTrabajo: es best-effort, no transaccional.
type PublicadorEventos interface {
	Publicar(ctx context.Context, eventos ...dominio.EventoDominio) error
}

// --- Tipos de apoyo de los puertos de cruce ---------------------------------
//
// Viven en puertos, no en dominio, porque son el contrato con otro
// contexto (o con Confianza) y no lenguaje ubicuo de Acceso.

// CredencialesSujeto es la entrada de AutenticadorIdentidad.Autenticar.
type CredencialesSujeto struct {
	Correo       string
	Contrasena   string
	TokenCaptcha string
	Origen       dominio.OrigenSolicitud
}

// SujetoAutenticado es la proyección de
// identidad/puertos.ResultadoAutenticacion que Acceso necesita. No
// incluye PuntajeConfianza como dato de negocio: solo se usa para decidir
// step-up, y eso ya viene resuelto en RequiereSegundoFactor.
type SujetoAutenticado struct {
	IDUsuario             string
	Estado                string
	RequiereSegundoFactor bool
	MotivoStepUp          string
}

// EstadoSujeto es la salida de ConsultorEstadoSujeto.EstadoDe.
type EstadoSujeto struct {
	Existe bool
	Estado string // "activo" | "suspendido" | "bloqueado" | "pendiente_verificacion" | "anonimizado"
	// CredencialActualizadaEn habilita la revocación por cambio de
	// contraseña sin broker (§11.1 del diseño). HOY NO EXISTE con la
	// semántica correcta en Identidad: queda declarado y en cero hasta que
	// se implemente esa parte. Ningún caso de uso de este hito lo lee.
	CredencialActualizadaEn time.Time
}

// SolicitudEvaluacion es la entrada de EvaluadorConfianza.Evaluar.
type SolicitudEvaluacion struct {
	Accion       string // "renovacion_sesion" | "cierre_masivo_sesiones"
	ClaveCuenta  string // "sesion:<id>" o "usuario:<id>": Acceso no conoce el correo
	Origen       dominio.OrigenSolicitud
	TokenCaptcha string
}

// DecisionConfianza es la salida de EvaluadorConfianza.Evaluar.
type DecisionConfianza struct {
	Permitido      bool
	RequiereStepUp bool
	Puntaje        float64
	Motivo         string
	ReintentarEn   time.Duration
}

// ResultadoIntento es la entrada de EvaluadorConfianza.RegistrarResultado.
type ResultadoIntento struct {
	Accion      string
	ClaveCuenta string
	Origen      dominio.OrigenSolicitud
	Exitoso     bool
	IDUsuario   string
}

// LlavePublica es un elemento del conjunto que devuelve
// FirmadorTokensAcceso.LlavesPublicas, insumo del endpoint JWKS.
type LlavePublica struct {
	KID, TipoLlave, Curva, Algoritmo string
	Material                         []byte
}

// --- Step-up MFA (ADR 0038, docs/design/otp-mfa.md §2.4) --------------------

// TokenStepUp es el token de vida corta que EmisorTokenStepUp produce.
// Deliberadamente NO es una dominio.Sesion: no tiene fila en `sesiones`, no
// tiene refresco, y jamás debe aceptarse donde se espera un token de
// acceso normal (INV-MFA-03). El único dato observable desde fuera de este
// paquete es su representación compacta ya firmada; quien necesite
// inspeccionar sus claims debe volver a pasarlo por
// EmisorTokenStepUp.Validar, nunca decodificarlo a mano.
type TokenStepUp struct {
	compacto string
}

// NuevoTokenStepUp envuelve la representación JWT compacta que el
// adaptador de infraestructura (jwx/v2, ADR 0038) ya firmó. Este paquete no
// firma nada por sí mismo: solo transporta el resultado que la
// infraestructura produjo.
func NuevoTokenStepUp(compacto string) TokenStepUp { return TokenStepUp{compacto: compacto} }

// Compacto devuelve la representación JWT compacta firmada, la que el
// cliente reenvía en POST /acceso/sesiones/segundo-factor.
func (t TokenStepUp) Compacto() string { return t.compacto }

// ClaimsStepUp es la salida de EmisorTokenStepUp.Validar. Deliberadamente
// primitivos (no VOs de Identidad): acceso/puertos no importa nada de
// identidad/dominio ni identidad/puertos (INV-ACC-19), y estos claims no
// son lenguaje ubicuo de Identidad sino el contrato propio, mínimo, de
// este token de un solo propósito.
type ClaimsStepUp struct {
	IDUsuario    string
	MotivoStepUp string
}

// EmisorTokenStepUp emite y valida el token de step-up (ADR 0038): un JWT
// propio de Acceso, de vida corta (INV-MFA-04: 5 minutos, sin refresco
// posible), que representa "credenciales OK, falta el segundo factor".
// Nunca lo emite Identidad (ADR 0009: Acceso nunca deja que Identidad
// emita tokens) — Identidad solo decide, vía ResultadoAutenticacion, que
// hace falta un segundo factor; emitir el token que lo representa es
// trabajo de Acceso. Validar debe verificar, además de firma y expiración,
// que el `typ` de cabecera sea el distintivo de step-up y nunca el de un
// token de acceso normal (INV-MFA-03, mismo criterio de defensa contra
// *algorithm/type confusion* que ya aplica FirmadorTokensAcceso.Verificar).
type EmisorTokenStepUp interface {
	Emitir(ctx context.Context, idUsuario string, motivoStepUp string, ahora time.Time) (TokenStepUp, error)
	Validar(ctx context.Context, tokenCompacto string) (ClaimsStepUp, error)
}

// VerificadorSegundoFactor es el puerto de salida que
// CompletarSegundoFactorCasoDeUso (acceso/aplicacion) consume para
// verificar el código OTP presentado contra Identidad (§3.6 del diseño
// otp-mfa.md). Implementado por el ACL
// acceso/adaptadores/identidad.VerificadorOTP sobre
// identidad/puertos.VerificadorOTP — mismo patrón que
// AutenticadorIdentidad/ConsultorEstadoSujeto: tipos propios de Acceso a
// ambos lados, acceso/aplicacion nunca ve un tipo de Identidad
// (INV-ACC-19).
//
// Nota de reubicación: esta interfaz vivió temporalmente declarada dentro
// de acceso/aplicacion/completar_segundo_factor.go (con la misma forma que
// tiene aquí) mientras acceso/puertos estaba cerrado para ese encargo. Se
// trasladó aquí, junto a AutenticadorIdentidad/ConsultorEstadoSujeto, sin
// cambiar su forma.
type VerificadorSegundoFactor interface {
	Verificar(ctx context.Context, idUsuario string, codigo string, origen dominio.OrigenSolicitud) (bool, error)
}
