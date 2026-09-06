package puertos

import (
	"context"
	"time"

	"github.com/r-david1/moterus/internal/confianza/dominio"
)

// LimitadorTasa es el puerto de salida del limitador de tasa. La
// implementación real (adaptadores/redis) usa Redis con INCR+EXPIRE: un
// contador de ventana fija (fixed window), no una ventana deslizante
// exacta (sliding log) — ver ADR 0018 para la justificación de por qué esa
// aproximación es suficiente aquí. Cualquier adaptador que satisfaga este
// contrato (incluido uno en memoria para tests) es intercambiable sin
// tocar aplicacion.
type LimitadorTasa interface {
	// Permitir incrementa atómicamente el contador de clave y decide si la
	// solicitud actual cabe dentro de umbral.Limite en umbral.Ventana.
	// permitido=false cuando el contador (ya incrementado) supera el
	// límite. reintentarEn es el tiempo restante hasta que la clave
	// expire — cuando el límite ya estaba superado, la implementación
	// real extiende ese TTL de forma exponencial (cooldown creciente,
	// tope documentado en el adaptador) en vez de dejarlo fijo.
	Permitir(ctx context.Context, clave string, umbral dominio.Umbral) (permitido bool, restantes int, reintentarEn time.Duration, err error)
	// Reiniciar borra el contador de clave. Se usa para no penalizar una
	// cuenta tras un intento exitoso (p. ej. login correcto tras un solo
	// error de tipeo previo).
	Reiniciar(ctx context.Context, clave string) error
}

// VerificadorCaptcha es el puerto de salida agnóstico de proveedor
// (Cloudflare Turnstile por defecto, ADR 0003) para validar un token de
// captcha invisible server-side. Firma equivalente a la especificada en el
// encargo original ("CaptchaVerifier.Verify"), renombrada al español por
// ADR 0007 (identificadores de puertos van en español).
type VerificadorCaptcha interface {
	// Verificar valida token contra el proveedor configurado y devuelve un
	// puntaje 0.0-1.0 (accion e ip se reenvían al proveedor para
	// validación adicional, cuando la soporta). Un token vacío es un error
	// de uso del llamador, no de este puerto: EvaluarTrustSignalCasoDeUso
	// nunca debe llamarlo con token="".
	Verificar(ctx context.Context, token string, accion string, ip string) (puntaje float64, err error)
}

// =============================================================================
// Colas de acceso virtual (docs/design/colas-virtuales.md §2.2). Extensión
// aditiva sobre el archivo existente: nada de lo de arriba cambia.
// =============================================================================

// RepositorioSalasDeEspera persiste el agregado SalaDeEspera. Puerto de
// salida frío: fuera del camino caliente de ingreso/consulta/reclamo
// (INV-COLA-08), lo consumen únicamente GestorDeSalasDeEspera y el
// reconciliador (§3.7 del diseño).
type RepositorioSalasDeEspera interface {
	Guardar(ctx context.Context, s *dominio.SalaDeEspera) error
	BuscarPorID(ctx context.Context, id dominio.IDSalaDeEspera) (*dominio.SalaDeEspera, error)
	BuscarPorAlias(ctx context.Context, alias dominio.AliasSala) (*dominio.SalaDeEspera, error)
	// ListarVigentes lo consume el reconciliador (§3.7 del diseño) cada N
	// segundos. Devuelve las salas en estado abierta|drenando.
	ListarVigentes(ctx context.Context) ([]*dominio.SalaDeEspera, error)
}

// EstadoDeCola es el puerto sobre el estado efímero de una sala. La
// implementación real (adaptadores/redis/estado_cola.go) usa tres scripts
// Lua para que "leer la configuración, calcular el cursor, decidir y
// escribir" sea atómico — mismo criterio y mismo motivo que scriptPermitir
// del limitador de tasa (ADR 0018). Cualquier adaptador que satisfaga este
// contrato (incluido uno en memoria para tests) es intercambiable sin tocar
// aplicacion. La clave que reciben todos sus métodos es el valor textual de
// dominio.ClaveSala (p. ej. "sistema:acceso.iniciar_sesion"); viaja como
// string porque este puerto vive del lado de infraestructura del camino
// caliente, donde la clave ya fue resuelta por el agregado y no necesita
// reconstruirse desde sus partes.
type EstadoDeCola interface {
	// Proyectar escribe/actualiza la configuración de la sala en Redis de
	// forma idempotente. Inicializa el reloj de admisión SOLO si no existía
	// (HSETNX): si Redis perdió el estado, el reloj arranca de cero en vez
	// de admitir de golpe a toda una cola vacía (§6.2, INV-COLA-13).
	Proyectar(ctx context.Context, p ProyeccionSala) error
	Retirar(ctx context.Context, clave string) error

	Ingresar(ctx context.Context, clave string, hash string) (EstadoTicket, error)
	Consultar(ctx context.Context, clave string, hash string) (EstadoTicket, error)
	Reclamar(ctx context.Context, clave string, hash string) (EstadoTicket, error)

	// Instantanea alimenta el endpoint público agregado y las métricas.
	Instantanea(ctx context.Context, clave string) (InstantaneaCola, error)
}

// ProyeccionSala es lo que EstadoDeCola.Proyectar escribe en Redis:
// configuración de una sala más el ancla del reloj de admisión (§1.5 y §6.2
// del diseño). Primitivos, nunca dominio.SalaDeEspera: quien implementa
// este puerto (el adaptador Redis) no necesita ni debe cargar el agregado
// completo para reproyectarlo, solo estos campos ya derivados.
type ProyeccionSala struct {
	Clave               string
	Alias               string
	Estado              string
	RitmoAdmision       int
	CapacidadMaximaCola int64
	VentanaReclamo      time.Duration
	CursorBase          int64
	RelojDesde          time.Time
	Version             int64 // monótona: una proyección vieja nunca pisa a una nueva
}

// EstadoTicket es el struct de transporte que devuelven
// Ingresar/Consultar/Reclamar de EstadoDeCola: el resultado crudo del
// script Lua correspondiente (§6.2 del diseño), con el desenlace ya
// resuelto y los datos para calcular posición/ETA del lado de aplicación.
//
// Nombre deliberadamente igual al VO enum dominio.EstadoTicket
// (esperando|consumido, ticket_cola.go): no colisiona en Go porque vive en
// el paquete puertos, no en dominio, y son conceptos distintos a propósito
// — este es un DTO de transporte de infraestructura hacia aplicación
// (incluye Desenlace, Rango, Cursor...), no el value object de solo dos
// estados que el dominio usa para razonar sobre el ciclo de vida de un
// TicketDeCola. El nombre se mantiene tal cual lo escribe §2.2 del diseño.
type EstadoTicket struct {
	Desenlace       string
	Rango           int64
	Cursor          int64
	LongitudCola    int64
	TurnoEstimadoEn time.Time
}

// InstantaneaCola alimenta el endpoint público agregado y las métricas
// (§7.1 del diseño y EstadoDeCola.Instantanea, arriba).
type InstantaneaCola struct {
	LongitudAproximada int64
	Cursor             int64
	Ingresos           int64
}

// GeneradorTickets: el dominio nunca genera bytes aleatorios por sí mismo
// (mismo criterio que GeneradorSecretoTOTP en identidad/puertos y el
// generador de tokens de invitación en tenencia/puertos).
type GeneradorTickets interface {
	GenerarTicket() (dominio.TicketPlano, error)
}

// --- Puertos que Confianza gana por primera vez con esta extensión ---------
//
// Hasta esta extensión, Confianza no tenía ninguno de los cuatro: nunca tuvo
// estado propio ni eventos de dominio (era "un evaluador puro", §0.1 del
// diseño). Mismo contrato exacto que los tres contextos existentes
// (identidad/puertos, acceso/puertos, tenencia/puertos) — sin variación, para
// que las implementaciones de infraestructura compartidas (p. ej. un reloj
// de sistema, un generador UUIDv7) puedan reutilizarse tal cual.

// Reloj es el puerto de salida para obtener la hora actual. El dominio
// nunca llama a time.Now(); los casos de uso lo hacen a través de este
// puerto para poder fijar el tiempo en los tests. Mismo contrato que
// identidad/puertos.Reloj y tenencia/puertos.Reloj.
type Reloj interface {
	Ahora() time.Time
}

// GeneradorIDs es el puerto de salida para generar identificadores nuevos
// del agregado SalaDeEspera. El dominio nunca genera UUIDs por sí mismo:
// NuevoIDSalaDeEspera produce un UUIDv7, mismo criterio que el resto del
// sistema (tabla 1.3 del diseño).
type GeneradorIDs interface {
	NuevoIDSalaDeEspera() (dominio.IDSalaDeEspera, error)
}

// RegistroAuditoria es el puerto de salida implementado sobre el contexto
// Auditoría, vía el ACL confianza/adaptadores/auditoria (mismo criterio que
// INV-TEN-28: la capa anticorrupción la posee quien depende, no quien es
// dependido). Mismo contrato que identidad/puertos.RegistroAuditoria: sin
// IDOrganizacion propio, porque una sala de alcance sistema no tiene una
// organización que poblar en auditoria.organizacion_id (a diferencia de
// tenencia/puertos.RegistroAuditoria, que sí la lleva porque TODA escritura
// de Tenencia es org-scoped). Cubre las tres mutaciones de ciclo de vida de
// una SalaDeEspera (INV-COLA-11): abrir, cambiar ritmo, drenar/cerrar. Un
// fallo al registrar la auditoría en una de esas mutaciones aborta la
// transacción de negocio, mismo criterio que INV-ID-15.
type RegistroAuditoria interface {
	Registrar(ctx context.Context, evento dominio.EventoDominio, origen dominio.OrigenSolicitud) error
}

// VerificadorDeAutorizacion es el puerto de salida ESTRECHO sobre Tenencia
// que los endpoints de administración org-scoped (§7.2 del diseño) y/o su
// middleware consumen para confirmar "¿este sujeto puede administrar la
// sala de espera de esta organización?" (permiso "organizacion.editar" o
// "organizacion.ver" del catálogo cerrado de ADR 0029, reutilizado tal cual
// en vez de agregar un noveno permiso — §7.2 del diseño). Implementado por
// confianza/adaptadores/tenencia — el ÚNICO paquete de Confianza autorizado
// a importar tenencia/puertos (mismo criterio de frontera que
// identidad/puertos.AutorizadorDeConsultas e INV-TEN-28). Deliberadamente
// primitivo y sin el vocabulario de tenencia/dominio.Permiso: Confianza no
// debe importar el dominio de Tenencia.
type VerificadorDeAutorizacion interface {
	// Autorizar responde permitido=true solo si idSujeto tiene permiso en
	// idOrganizacion, según la matriz de permisos de Tenencia (ADR
	// 0029/0030). Cualquier error de infraestructura o de Tenencia se
	// propaga tal cual: el llamador debe tratarlo como "no se pudo
	// autorizar", nunca como "sí autorizado" por defecto.
	Autorizar(ctx context.Context, q ConsultaAutorizacionOrganizacion) (permitido bool, err error)
}

// ConsultaAutorizacionOrganizacion transporta la entrada de
// VerificadorDeAutorizacion.Autorizar.
type ConsultaAutorizacionOrganizacion struct {
	IDSujeto       string
	IDOrganizacion string
	Permiso        string // p. ej. "organizacion.editar" (catálogo cerrado de Tenencia)
}

// --- Puertos aplazados (§2.3 del diseño) ------------------------------------
//
// Ya nombrados para que nadie los reinvente con otro nombre; no se
// implementan en esta extensión:
//
//   - PrioridadDeCola / PoliticaDeEquidad: colas con prioridad (usuarios
//     premium, reintentos de quien caducó su turno). Hoy la cola es
//     estrictamente FIFO (ver el riesgo residual de ticket-farming, §4,
//     INV-COLA-04, y ADR candidato 0043).
//   - EmisorSalaProgramada: abrir/cerrar una sala por horario
//     (programada -> abierta automática a una hora fija). El estado
//     EstadoSalaProgramada ya existe para que agregarlo sea aditivo; hoy la
//     transición la dispara siempre un operador.
//   - NotificadorDeTurno: avisar por push/websocket en vez de sondeo. El
//     sondeo dictado por el servidor (ResultadoTurno.ReconsultarEn) es el
//     MVP.
