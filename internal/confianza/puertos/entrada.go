package puertos

import (
	"context"
	"time"

	"github.com/r-david1/moterus/internal/confianza/dominio"
)

// Solicitud transporta lo que EvaluadorDeRiesgo necesita para decidir. Es
// deliberadamente más ancho que identidad/puertos.SolicitudEvaluacion (que
// solo tiene Accion/CorreoNormalizado/Origen/TokenCaptcha): aquí IP y
// TenantID viajan como primitivos propios en vez de depender del VO
// dominio.OrigenSolicitud de Identidad — Confianza no debe importar el
// paquete dominio de Identidad (cada contexto tiene su propio lenguaje
// ubicuo, sección 5.3 del diseño de Identidad).
type Solicitud struct {
	Accion            dominio.Accion
	IPOrigen          string
	CorreoNormalizado string
	TokenCaptcha      string
	// TenantID ya no está vacío siempre: el contexto Tenencia lo puebla
	// desde su middleware de autorización HTTP (§11.3 de
	// docs/design/tenencia-bounded-context.md), una vez que el
	// {idOrganizacion} de la ruta ya fue autorizado. Sigue sin usarse para
	// un límite de rate limiting POR TENANT (ese alcance queda para cuando
	// algún consumidor lo necesite; ver ADR 0018, "Alcance no cubierto")
	// pero el hueco de "no hay una clave real que limitar" ya se cerró.
	TenantID string
}

// ResultadoIntento informa el desenlace real de la acción evaluada, para
// que EvaluadorDeRiesgo pueda ajustar el estado que guarda entre
// solicitudes (p. ej. resetear el contador de una cuenta tras un login
// exitoso, para no penalizar a un usuario legítimo que erró la contraseña
// una vez).
type ResultadoIntento struct {
	Accion            dominio.Accion
	IPOrigen          string
	CorreoNormalizado string
	Exitoso           bool
}

// EvaluadorDeRiesgo es el puerto de entrada del bounded context Confianza:
// lo implementa aplicacion.EvaluarTrustSignalCasoDeUso y lo consumen (a)
// el ACL identidad/adaptadores/confianza, que lo envuelve detrás de
// identidad/puertos.EvaluadorConfianza para login/registro, y (b) el
// guardián de perímetro de identidad/adaptadores/http para el endpoint de
// reenvío de verificación (que la aplicación de Identidad no evalúa hoy —
// ver ADR 0018).
type EvaluadorDeRiesgo interface {
	Evaluar(ctx context.Context, s Solicitud) (dominio.Decision, error)
	RegistrarResultado(ctx context.Context, r ResultadoIntento) error
}

// =============================================================================
// Colas de acceso virtual (docs/design/colas-virtuales.md §2.1). Extensión
// aditiva sobre el archivo existente: nada de lo de arriba cambia.
// =============================================================================

// PorteroDeSala es el puerto de camino caliente de la sala de espera: lo
// implementa aplicacion.PorteroDeSalaCasoDeUso y lo consume EXCLUSIVAMENTE
// el middleware HTTP MiddlewareSalaDeEspera (§7.3 del diseño) — nunca un
// caso de uso de otro contexto bounded. Sus tres operaciones tocan
// exclusivamente Redis y CPU local, jamás Postgres (INV-COLA-08); por eso
// sus DTOs de entrada/salida son primitivos, mismo criterio que Solicitud
// más arriba: nunca se filtra dominio.SalaDeEspera ni dominio.TicketDeCola
// hacia el borde HTTP.
type PorteroDeSala interface {
	// SalaVigentePara responde, desde la instantánea en memoria del
	// reconciliador (§3.7 del diseño), si la ruta+alcance de esta petición
	// está protegida ahora mismo. Es la primera pregunta del middleware y no
	// hace E/S: tiene que poder responderse incluso con Redis caído, que es
	// precisamente cuando hay que saber el ModoDegradado (§8). El booleano
	// de retorno indica si hay una sala vigente para esa clave.
	SalaVigentePara(ctx context.Context, q ConsultaSalaVigente) (VistaSalaVigente, bool)

	// Ingresar emite un ticket y devuelve el turno asignado (§3.4 del
	// diseño). Es la operación pública, sin autenticación, detrás de
	// POST /confianza/salas-espera/{aliasSala}/tickets.
	Ingresar(ctx context.Context, cmd ComandoIngresarASala) (ResultadoTurno, error)

	// ConsultarTurno es de solo lectura: no muta nada salvo refrescar el TTL
	// del ticket (§3.5 y §6.2 del diseño). Nunca toca Postgres ni revela
	// nada de la organización dueña o de la ruta protegida (INV-COLA-15).
	ConsultarTurno(ctx context.Context, q ConsultaTurno) (ResultadoTurno, error)

	// Reclamar consume el turno (§3.6 del diseño). Es la única operación
	// que puede dejar pasar la petición real, y es de un solo uso
	// (INV-COLA-06). Lo invoca el middleware, nunca un endpoint propio.
	Reclamar(ctx context.Context, cmd ComandoReclamarTurno) (ResultadoTurno, error)
}

// GestorDeSalasDeEspera es el puerto de administración (frío, §3.1-§3.3 del
// diseño): muta el agregado SalaDeEspera en Postgres y reproyecta a Redis en
// la misma unidad de trabajo lógica. Lo consumen los endpoints de
// administración org-scoped (§7.2) — las salas de alcance sistema se abren
// por un subcomando de CLI que invoca el mismo puerto con IDSujeto vacío,
// nunca por HTTP (este producto no tiene rol de administrador de
// plataforma, §7.2 del diseño).
type GestorDeSalasDeEspera interface {
	Abrir(ctx context.Context, cmd ComandoAbrirSala) (VistaSala, error)
	CambiarRitmo(ctx context.Context, cmd ComandoCambiarRitmoAdmision) (VistaSala, error)
	CambiarEstado(ctx context.Context, cmd ComandoCambiarEstadoSala) (VistaSala, error)
}

// ConsultorDeSalas alimenta el endpoint público agregado
// (GET /confianza/salas-espera/{aliasSala}, §7.1 del diseño): datos de la
// sala pensados para un tercero anónimo (longitud aproximada, estado, ETA),
// nunca de un ticket concreto ni de la organización dueña (INV-COLA-15).
type ConsultorDeSalas interface {
	ObtenerPorAlias(ctx context.Context, q ConsultaSalaPorAlias) (VistaSalaPublica, error)
}

// --- comandos, consultas y resultados (primitivos, nunca DTOs HTTP) --------

// ConsultaSalaVigente transporta lo que PorteroDeSala.SalaVigentePara
// necesita para resolver la clave de sala sin depender de dominio.ClaveSala
// (el middleware no conoce el dominio, solo el valor textual de la ruta que
// declaró al montarse, §7.3 del diseño).
type ConsultaSalaVigente struct {
	Ruta           string // valor del catálogo cerrado dominio.RutaProtegida
	IDOrganizacion string // "" para alcance sistema
}

// VistaSalaVigente es lo que el middleware necesita para decidir sin tocar
// Redis todavía: si hay sala, su alias (para armar el 503 de §7.4), su
// estado (abierta admite ingresos nuevos; drenando no, pero sigue
// reclamando turnos vivos) y el modo degradado a aplicar si Redis no
// responde (§8 del diseño).
type VistaSalaVigente struct {
	Alias         string
	Clave         string
	Estado        string // "abierta" | "drenando"
	ModoDegradado string // "permitir" | "rechazar"
}

// ComandoIngresarASala transporta la entrada de PorteroDeSala.Ingresar.
type ComandoIngresarASala struct {
	Alias  string
	Origen dominio.OrigenSolicitud
}

// ConsultaTurno transporta la entrada de PorteroDeSala.ConsultarTurno.
type ConsultaTurno struct {
	Alias       string
	TicketPlano string
}

// ComandoReclamarTurno transporta la entrada de PorteroDeSala.Reclamar. La
// Clave ya viene resuelta por el middleware desde VistaSalaVigente: el
// caso de uso no vuelve a calcularla.
type ComandoReclamarTurno struct {
	Clave       string // resuelta por el middleware desde VistaSalaVigente
	TicketPlano string
}

// ResultadoTurno es lo que ve el cliente (§7.1 del diseño). TicketPlano
// viene poblado SOLO en la respuesta de Ingresar (única vez que el ticket
// sale del proceso, INV-COLA-07), igual que el secreto TOTP en
// ResultadoHabilitarMFA de Identidad.
type ResultadoTurno struct {
	TicketPlano     string
	Desenlace       string // catálogo cerrado dominio.DesenlaceDeAdmision
	Posicion        int64
	LongitudCola    int64
	EsperaEstimada  time.Duration
	TurnoEstimadoEn time.Time
	ReconsultarEn   time.Duration // intervalo de sondeo dictado por el servidor (§7.1)
}

// ComandoAbrirSala transporta la entrada de GestorDeSalasDeEspera.Abrir
// (§3.1 del diseño).
type ComandoAbrirSala struct {
	Alias               string
	Ruta                string
	IDOrganizacion      string // "" ⇒ alcance sistema
	RitmoAdmision       int
	CapacidadMaximaCola int64
	VentanaReclamo      time.Duration
	ModoDegradado       string
	IDSujeto            string // "" para el alcance sistema, operado fuera de la API (§7.2)
	Origen              dominio.OrigenSolicitud
}

// ComandoCambiarRitmoAdmision transporta la entrada de
// GestorDeSalasDeEspera.CambiarRitmo (§3.2 del diseño).
//
// IDOrganizacion: "" cuando el llamador es de confianza para tocar
// cualquier sala (el subcomando de CLI que opera salas de alcance sistema
// fuera de la API, §7.2); no vacío cuando el llamador es el endpoint HTTP
// org-scoped (§7.2), que ya autorizó al sujeto sobre ESA organización y
// exige que el caso de uso confirme que la sala cargada por IDSala
// pertenece exactamente a ella — sin este campo, un administrador
// autorizado sobre su propia organización podría pasar el IDSala de una
// sala de alcance sistema o de otra organización y mutarla igual, porque
// nada más en el flujo verifica la pertenencia (IDOR: la autorización del
// middleware es sobre el {idOrganizacion} de la ruta, no sobre el
// recurso que el comando termina tocando).
type ComandoCambiarRitmoAdmision struct {
	IDSala         string
	IDOrganizacion string
	RitmoAdmision  int
	IDSujeto       string
	Origen         dominio.OrigenSolicitud
}

// ComandoCambiarEstadoSala transporta la entrada de
// GestorDeSalasDeEspera.CambiarEstado (drenar/cerrar/reabrir, §3.3 del
// diseño).
//
// IDOrganizacion: mismo criterio y misma razón que en
// ComandoCambiarRitmoAdmision — "" para el llamador de confianza (CLI,
// alcance sistema), no vacío para el endpoint HTTP org-scoped, que exige
// verificar que la sala cargada por IDSala pertenezca a esa organización.
type ComandoCambiarEstadoSala struct {
	IDSala         string
	IDOrganizacion string
	Destino        string // "abierta" | "drenando" | "cerrada"
	IDSujeto       string
	Origen         dominio.OrigenSolicitud
}

// VistaSala es la proyección completa de una SalaDeEspera para los
// endpoints de administración (§7.2 del diseño: POST/PATCH/GET devuelven
// VistaSala). No está definida explícitamente en el diseño; se infiere de
// dos fuentes: la tabla de endpoints de §7.2 (que exige una vista rica,
// a diferencia de la pública y minimalista VistaSalaPublica de §7.1/INV-
// COLA-15) y los getters ya existentes del agregado
// confianza/dominio.SalaDeEspera. A diferencia de VistaSalaPublica, esta
// vista SÍ puede revelar el dueño y la ruta protegida: solo la ve un
// operador ya autorizado con "organizacion.editar"/"organizacion.ver"
// sobre esa misma organización (§7.2), nunca un tercero anónimo.
type VistaSala struct {
	ID                  string
	Alias               string
	AlcanceTipo         string // "sistema" | "organizacion"
	IDOrganizacion      string // "" para alcance sistema
	Ruta                string
	Estado              string
	RitmoAdmision       int
	CapacidadMaximaCola int64
	VentanaReclamo      time.Duration
	ModoDegradado       string
	CreadaPor           string // "" para alcance sistema (§7.2)
	CreadaEn            time.Time
	AbiertaEn           *time.Time // nil si la sala nunca se abrió
	CerradaEn           *time.Time // nil si la sala no está cerrada
}

// ConsultaSalaPorAlias transporta la entrada de
// ConsultorDeSalas.ObtenerPorAlias. No está definida explícitamente en el
// diseño; es análoga a ConsultaTurno (arriba), pero sin ticket: el endpoint
// público agregado (GET /confianza/salas-espera/{aliasSala}, §7.1) solo
// necesita el alias.
type ConsultaSalaPorAlias struct {
	Alias string
}

// VistaSalaPublica es la proyección minimalista para el endpoint público
// agregado (§7.1 del diseño, "Cache-Control: public, max-age=5"): nunca
// revela la organización dueña, la ruta protegida ni la existencia de otras
// salas (INV-COLA-15).
type VistaSalaPublica struct {
	Alias              string
	Estado             string
	LongitudAproximada int64
	EsperaEstimada     time.Duration
	ReconsultarEn      time.Duration
}
