package dominio

import "time"

// EventoDominio es el contrato que implementan los tres eventos que el
// contexto Confianza audita (§1.7 del diseño). Mismo contrato que
// identidad/dominio.EventoDominio, acceso/dominio.EventoDominio y
// tenencia/dominio.EventoDominio (NombreEvento, OcurridoEn, IDAgregado),
// redeclarado aquí a propósito: confianza/dominio no puede importar el
// dominio de otro contexto.
//
// Los tres eventos (SalaDeEsperaAbierta, RitmoDeAdmisionCambiado,
// SalaDeEsperaCerrada) los acumula el agregado SalaDeEspera y se drenan con
// EventosPendientes tras persistir, en la misma unidad de trabajo que la
// auditoría (INV-COLA-11). No hay un cuarto evento: ni los ingresos, ni las
// consultas de turno, ni los reclamos se auditan (§1.7 del diseño, mismo
// razonamiento que INV-TEN-25 en tenencia/dominio) — TicketDeCola no
// acumula eventos, ni siquiera tiene un campo para hacerlo. Ningún evento
// transporta un TicketPlano ni su HashTicket, verificable con un test de
// reflexión (mismo patrón que INV-TEN-23 en tenencia/dominio).
type EventoDominio interface {
	// NombreEvento identifica el tipo de evento (p. ej. "SalaDeEsperaAbierta").
	NombreEvento() string
	// OcurridoEn indica cuándo ocurrió el evento, siempre con la hora
	// provista como parámetro por el caso de uso (el dominio nunca llama a
	// time.Now()).
	OcurridoEn() time.Time
	// IDAgregado identifica a la SalaDeEspera relacionada.
	IDAgregado() string
}

// SalaDeEsperaAbierta se emite cada vez que una sala transiciona a
// EstadoSalaAbierta, tanto en su primera apertura como en una reapertura
// durante el drenaje (accion de auditoría: sala_espera.abierta).
type SalaDeEsperaAbierta struct {
	IDSalaDeEspera  string
	Alias           string
	Alcance         string
	Ruta            string
	RitmoAdmision   int
	CapacidadMaxima int64
	ModoDegradado   string
	ocurridoEn      time.Time
}

// NuevoSalaDeEsperaAbierta construye el evento SalaDeEsperaAbierta.
func NuevoSalaDeEsperaAbierta(id IDSalaDeEspera, alias AliasSala, alcance AlcanceSala, ruta RutaProtegida, politica PoliticaSala, ocurridoEn time.Time) SalaDeEsperaAbierta {
	return SalaDeEsperaAbierta{
		IDSalaDeEspera:  id.String(),
		Alias:           alias.Normalizado(),
		Alcance:         alcance.Clave(),
		Ruta:            ruta.String(),
		RitmoAdmision:   politica.RitmoAdmision().PorSegundo(),
		CapacidadMaxima: politica.CapacidadMaximaCola(),
		ModoDegradado:   politica.ModoDegradado().String(),
		ocurridoEn:      ocurridoEn,
	}
}

func (e SalaDeEsperaAbierta) NombreEvento() string  { return "SalaDeEsperaAbierta" }
func (e SalaDeEsperaAbierta) OcurridoEn() time.Time { return e.ocurridoEn }
func (e SalaDeEsperaAbierta) IDAgregado() string    { return e.IDSalaDeEspera }

// RitmoDeAdmisionCambiado se emite al cambiar el ritmo de admisión de una
// sala vigente (accion de auditoría: sala_espera.ritmo_cambiado). Lleva el
// cursor y la longitud de la cola en el momento del cambio: es el dato que
// un post-mortem del evento va a pedir primero (§3.2 del diseño).
type RitmoDeAdmisionCambiado struct {
	IDSalaDeEspera  string
	RitmoAnterior   int
	RitmoNuevo      int
	CursorAlCambiar int64
	LongitudCola    int64
	ocurridoEn      time.Time
}

// NuevoRitmoDeAdmisionCambiado construye el evento RitmoDeAdmisionCambiado.
func NuevoRitmoDeAdmisionCambiado(id IDSalaDeEspera, ritmoAnterior, ritmoNuevo RitmoAdmision, cursorAlCambiar, longitudCola int64, ocurridoEn time.Time) RitmoDeAdmisionCambiado {
	return RitmoDeAdmisionCambiado{
		IDSalaDeEspera:  id.String(),
		RitmoAnterior:   ritmoAnterior.PorSegundo(),
		RitmoNuevo:      ritmoNuevo.PorSegundo(),
		CursorAlCambiar: cursorAlCambiar,
		LongitudCola:    longitudCola,
		ocurridoEn:      ocurridoEn,
	}
}

func (e RitmoDeAdmisionCambiado) NombreEvento() string  { return "RitmoDeAdmisionCambiado" }
func (e RitmoDeAdmisionCambiado) OcurridoEn() time.Time { return e.ocurridoEn }
func (e RitmoDeAdmisionCambiado) IDAgregado() string    { return e.IDSalaDeEspera }

// SalaDeEsperaCerrada se emite tanto al pasar a EstadoSalaDrenando como al
// pasar a EstadoSalaCerrada, distinguidos por Destino (accion de
// auditoría: sala_espera.cerrada). Lleva los totales agregados del evento:
// son los únicos números del camino caliente que llegan a la auditoría, una
// sola vez (§1.7 del diseño).
type SalaDeEsperaCerrada struct {
	IDSalaDeEspera   string
	Destino          string
	IngresosTotales  int64
	AdmitidosTotales int64
	ocurridoEn       time.Time
}

// NuevoSalaDeEsperaCerrada construye el evento SalaDeEsperaCerrada.
func NuevoSalaDeEsperaCerrada(id IDSalaDeEspera, destino EstadoSala, ingresosTotales, admitidosTotales int64, ocurridoEn time.Time) SalaDeEsperaCerrada {
	return SalaDeEsperaCerrada{
		IDSalaDeEspera:   id.String(),
		Destino:          destino.String(),
		IngresosTotales:  ingresosTotales,
		AdmitidosTotales: admitidosTotales,
		ocurridoEn:       ocurridoEn,
	}
}

func (e SalaDeEsperaCerrada) NombreEvento() string  { return "SalaDeEsperaCerrada" }
func (e SalaDeEsperaCerrada) OcurridoEn() time.Time { return e.ocurridoEn }
func (e SalaDeEsperaCerrada) IDAgregado() string    { return e.IDSalaDeEspera }
