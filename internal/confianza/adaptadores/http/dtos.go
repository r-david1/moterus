package http

import "time"

// Los DTOs de este archivo son la única frontera de serialización HTTP del
// contexto Confianza para la extensión de colas de acceso virtual (§7 del
// diseño docs/design/colas-virtuales.md). Nunca envuelven
// dominio.SalaDeEspera ni dominio.TicketDeCola directamente: se proyectan
// los tipos primitivos de confianza/puertos (ADR 0006: Huma genera el
// OpenAPI y valida a partir de estos tags).

// --- POST /confianza/salas-espera/{aliasSala}/tickets -----------------------

// IngresarASalaInput es el input Huma del ingreso a una sala de espera.
type IngresarASalaInput struct {
	AliasSala string `path:"aliasSala" minLength:"1" doc:"Alias público de la sala de espera."`
}

// respuestaTurno es la proyección de puertos.ResultadoTurno. Ticket viaja
// SOLO en la respuesta de IngresarASala (INV-COLA-07); ConsultarTurno lo
// deja vacío y `json:"ticket,omitempty"` lo omite del cuerpo.
type respuestaTurno struct {
	Ticket                 string    `json:"ticket,omitempty" doc:"El ticket en claro. Solo presente en la respuesta de ingreso: única vez que sale del proceso."`
	Desenlace              string    `json:"desenlace" doc:"admitido | esperando | turno_caducado | ticket_desconocido | ticket_consumido | sala_cerrada | cola_llena."`
	Posicion               int64     `json:"posicion"`
	LongitudCola           int64     `json:"longitud_cola"`
	EsperaEstimadaSegundos int64     `json:"espera_estimada_segundos"`
	TurnoEstimadoEn        time.Time `json:"turno_estimado_en"`
	ReconsultarEnMs        int64     `json:"reconsultar_en_ms" doc:"Intervalo de sondeo dictado por el servidor, proporcional a la posición."`
}

// IngresarASalaOutput es el output Huma del ingreso (201, con el ticket).
type IngresarASalaOutput struct {
	Body respuestaTurno
}

// --- GET /confianza/salas-espera/{aliasSala}/turno ---------------------------

// ConsultarTurnoInput es el input Huma de la consulta de turno. El ticket
// viaja en la cabecera X-Ticket-Cola, nunca en la ruta ni en la query (§7.1
// del diseño: "un secreto en el path termina en los logs de acceso").
type ConsultarTurnoInput struct {
	AliasSala  string `path:"aliasSala" minLength:"1"`
	TicketCola string `header:"X-Ticket-Cola" doc:"El ticket de cola en claro, obtenido de POST .../tickets."`
}

// ConsultarTurnoOutput es el output Huma de la consulta de turno. Sin
// Ticket (INV-COLA-07) y sin caché (§7.1 del diseño: "el estado por ticket
// no es cacheable: es privado y cambia por segundo").
type ConsultarTurnoOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         respuestaTurno
}

// --- GET /confianza/salas-espera/{aliasSala} ---------------------------------

// ObtenerSalaPublicaInput es el input Huma del agregado público.
type ObtenerSalaPublicaInput struct {
	AliasSala string `path:"aliasSala" minLength:"1"`
}

// respuestaSalaPublica es la proyección de puertos.VistaSalaPublica: nunca
// revela la organización dueña ni la ruta protegida (INV-COLA-15).
type respuestaSalaPublica struct {
	Alias                  string `json:"alias"`
	Estado                 string `json:"estado"`
	LongitudAproximada     int64  `json:"longitud_aproximada"`
	EsperaEstimadaSegundos int64  `json:"espera_estimada_segundos"`
	ReconsultarEnMs        int64  `json:"reconsultar_en_ms"`
}

// ObtenerSalaPublicaOutput es el output Huma del agregado público, cacheable
// (§7.1 del diseño: "Cache-Control: public, max-age=5", apto para CDN).
type ObtenerSalaPublicaOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         respuestaSalaPublica
}

// --- POST /confianza/organizaciones/{idOrganizacion}/salas-espera -----------

type abrirSalaPeticion struct {
	Alias                  string `json:"alias" minLength:"3" maxLength:"48" doc:"Identificador único de la sala; aparece en la URL de ingreso." example:"inscripciones-2026"`
	Ruta                   string `json:"ruta" enum:"acceso.iniciar_sesion,identidad.registrar_usuario,tenencia.aceptar_invitacion" doc:"Ruta protegida, del catálogo cerrado (§1.6 del diseño)."`
	RitmoAdmision          int    `json:"ritmo_admision" minimum:"1" maximum:"10000" doc:"Admisiones por segundo."`
	CapacidadMaximaCola    int64  `json:"capacidad_maxima_cola" minimum:"100" maximum:"5000000"`
	VentanaReclamoSegundos int    `json:"ventana_reclamo_segundos" minimum:"30" maximum:"900" doc:"Ventana durante la cual un turno admitido puede reclamarse antes de caducar."`
	ModoDegradado          string `json:"modo_degradado" enum:"permitir,rechazar" doc:"Comportamiento ante una caída de Redis (§8 del diseño)."`
}

// AbrirSalaInput es el input Huma de la apertura de una sala org-scoped.
type AbrirSalaInput struct {
	IDOrganizacion string `path:"idOrganizacion" format:"uuid"`
	Body           abrirSalaPeticion
}

// vistaSalaRespuesta es la proyección 1:1 de puertos.VistaSala (§7.2 del
// diseño): a diferencia de respuestaSalaPublica, esta vista sí revela el
// dueño y la ruta protegida — solo la ve un operador ya autorizado.
type vistaSalaRespuesta struct {
	ID                     string     `json:"id"`
	Alias                  string     `json:"alias"`
	AlcanceTipo            string     `json:"alcance_tipo"`
	IDOrganizacion         string     `json:"id_organizacion,omitempty"`
	Ruta                   string     `json:"ruta"`
	Estado                 string     `json:"estado"`
	RitmoAdmision          int        `json:"ritmo_admision"`
	CapacidadMaximaCola    int64      `json:"capacidad_maxima_cola"`
	VentanaReclamoSegundos int64      `json:"ventana_reclamo_segundos"`
	ModoDegradado          string     `json:"modo_degradado"`
	CreadaPor              string     `json:"creada_por,omitempty"`
	CreadaEn               time.Time  `json:"creada_en"`
	AbiertaEn              *time.Time `json:"abierta_en,omitempty"`
	CerradaEn              *time.Time `json:"cerrada_en,omitempty"`
}

// AbrirSalaOutput es el output Huma de la apertura.
type AbrirSalaOutput struct {
	Body vistaSalaRespuesta
}

// --- PATCH /confianza/organizaciones/{idOrganizacion}/salas-espera/{idSala} -

// actualizarSalaPeticion cubre las dos operaciones administrativas que
// GestorDeSalasDeEspera expone además de Abrir (§3.2/§3.3 del diseño:
// CambiarRitmo durante el pico, CambiarEstado para drenar/cerrar/reabrir).
// El diseño no especifica dos rutas separadas para ellas y ambas comparten
// el mismo verbo/recurso en la tabla de §7.2, así que este único PATCH
// despacha según qué campo venga presente en el cuerpo — al menos uno es
// obligatorio.
type actualizarSalaPeticion struct {
	RitmoAdmision *int    `json:"ritmo_admision,omitempty" minimum:"1" maximum:"10000" doc:"Presente para cambiar el ritmo de admisión de una sala vigente (§3.2 del diseño)."`
	Estado        *string `json:"estado,omitempty" enum:"abierta,drenando,cerrada" doc:"Presente para drenar, cerrar o reabrir la sala (§3.3 del diseño)."`
}

// ActualizarSalaInput es el input Huma de la actualización.
type ActualizarSalaInput struct {
	IDOrganizacion string `path:"idOrganizacion" format:"uuid"`
	IDSala         string `path:"idSala" format:"uuid"`
	Body           actualizarSalaPeticion
}

// ActualizarSalaOutput es el output Huma de la actualización.
type ActualizarSalaOutput struct {
	Body vistaSalaRespuesta
}
