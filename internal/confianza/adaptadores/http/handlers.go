package http

import (
	"context"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/r-david1/moterus/internal/confianza/puertos"
)

// ManejadorConfianza agrupa los casos de uso de colas de acceso virtual de
// Confianza detrás de sus puertos de entrada, nunca de los structs
// concretos de aplicacion (mismo criterio que ManejadorTenencia y
// ManejadorAcceso).
type ManejadorConfianza struct {
	portero   puertos.PorteroDeSala
	gestor    puertos.GestorDeSalasDeEspera
	consultor puertos.ConsultorDeSalas
}

// NuevoManejadorConfianza construye el manejador HTTP con sus puertos de
// entrada inyectados.
func NuevoManejadorConfianza(
	portero puertos.PorteroDeSala,
	gestor puertos.GestorDeSalasDeEspera,
	consultor puertos.ConsultorDeSalas,
) *ManejadorConfianza {
	return &ManejadorConfianza{portero: portero, gestor: gestor, consultor: consultor}
}

// idSujetoDesdeContexto extrae el `sub` del acceso/puertos.Acceso ya
// validado por middlewareAutenticacion (mismo criterio que INV-TEN-12: el
// IDSujeto ejecutor SIEMPRE sale del token, nunca del cuerpo ni de la
// query).
func idSujetoDesdeContexto(ctx context.Context) string {
	acceso, _ := accesoDesdeContexto(ctx)
	return acceso.IDUsuario
}

// --- Endpoints públicos (§7.1 del diseño) ------------------------------------

// IngresarASala implementa el handler Huma de
// POST /confianza/salas-espera/{aliasSala}/tickets.
func (m *ManejadorConfianza) IngresarASala(ctx context.Context, in *IngresarASalaInput) (*IngresarASalaOutput, error) {
	resultado, err := m.portero.Ingresar(ctx, puertos.ComandoIngresarASala{
		Alias:  in.AliasSala,
		Origen: origenSolicitudDesdeContexto(ctx),
	})
	if err != nil {
		return nil, mapearErrorDominio(ctx, err)
	}
	return &IngresarASalaOutput{Body: respuestaTurnoDesde(resultado, true)}, nil
}

// ConsultarTurno implementa el handler Huma de
// GET /confianza/salas-espera/{aliasSala}/turno. Cualquier desenlace del
// ticket (incluidos ticket_desconocido/ticket_consumido/turno_caducado) es
// un 200 con el desenlace en el cuerpo, nunca un error HTTP (§7.1 del
// diseño, §1.8: un ticket de cola no protege ningún secreto).
func (m *ManejadorConfianza) ConsultarTurno(ctx context.Context, in *ConsultarTurnoInput) (*ConsultarTurnoOutput, error) {
	resultado, err := m.portero.ConsultarTurno(ctx, puertos.ConsultaTurno{
		Alias:       in.AliasSala,
		TicketPlano: in.TicketCola,
	})
	if err != nil {
		return nil, mapearErrorDominio(ctx, err)
	}
	return &ConsultarTurnoOutput{
		CacheControl: "no-store",
		Body:         respuestaTurnoDesde(resultado, false),
	}, nil
}

// ObtenerSalaPublica implementa el handler Huma de
// GET /confianza/salas-espera/{aliasSala}: el agregado público y cacheable
// (§7.1 del diseño).
func (m *ManejadorConfianza) ObtenerSalaPublica(ctx context.Context, in *ObtenerSalaPublicaInput) (*ObtenerSalaPublicaOutput, error) {
	vista, err := m.consultor.ObtenerPorAlias(ctx, puertos.ConsultaSalaPorAlias{Alias: in.AliasSala})
	if err != nil {
		return nil, mapearErrorDominio(ctx, err)
	}
	return &ObtenerSalaPublicaOutput{
		CacheControl: "public, max-age=5",
		Body:         respuestaSalaPublicaDesde(vista),
	}, nil
}

// --- Endpoints de administración (§7.2 del diseño) ---------------------------

// AbrirSala implementa el handler Huma de
// POST /confianza/organizaciones/{idOrganizacion}/salas-espera.
func (m *ManejadorConfianza) AbrirSala(ctx context.Context, in *AbrirSalaInput) (*AbrirSalaOutput, error) {
	vista, err := m.gestor.Abrir(ctx, puertos.ComandoAbrirSala{
		Alias:               in.Body.Alias,
		Ruta:                in.Body.Ruta,
		IDOrganizacion:      in.IDOrganizacion,
		RitmoAdmision:       in.Body.RitmoAdmision,
		CapacidadMaximaCola: in.Body.CapacidadMaximaCola,
		VentanaReclamo:      time.Duration(in.Body.VentanaReclamoSegundos) * time.Second,
		ModoDegradado:       in.Body.ModoDegradado,
		IDSujeto:            idSujetoDesdeContexto(ctx),
		Origen:              origenSolicitudDesdeContexto(ctx),
	})
	if err != nil {
		return nil, mapearErrorDominio(ctx, err)
	}
	return &AbrirSalaOutput{Body: vistaSalaRespuestaDesde(vista)}, nil
}

// ActualizarSala implementa el handler Huma de
// PATCH /confianza/organizaciones/{idOrganizacion}/salas-espera/{idSala}.
// Despacha a GestorDeSalasDeEspera.CambiarRitmo y/o CambiarEstado según qué
// campo venga presente en el cuerpo (comentario de actualizarSalaPeticion
// en dtos.go); exige al menos uno.
func (m *ManejadorConfianza) ActualizarSala(ctx context.Context, in *ActualizarSalaInput) (*ActualizarSalaOutput, error) {
	if in.Body.RitmoAdmision == nil && in.Body.Estado == nil {
		return nil, huma.Error422UnprocessableEntity("debe indicarse ritmo_admision o estado")
	}

	var vista puertos.VistaSala
	if in.Body.RitmoAdmision != nil {
		v, err := m.gestor.CambiarRitmo(ctx, puertos.ComandoCambiarRitmoAdmision{
			IDSala:         in.IDSala,
			IDOrganizacion: in.IDOrganizacion,
			RitmoAdmision:  *in.Body.RitmoAdmision,
			IDSujeto:       idSujetoDesdeContexto(ctx),
			Origen:         origenSolicitudDesdeContexto(ctx),
		})
		if err != nil {
			return nil, mapearErrorDominio(ctx, err)
		}
		vista = v
	}
	if in.Body.Estado != nil {
		v, err := m.gestor.CambiarEstado(ctx, puertos.ComandoCambiarEstadoSala{
			IDSala:         in.IDSala,
			IDOrganizacion: in.IDOrganizacion,
			Destino:        *in.Body.Estado,
			IDSujeto:       idSujetoDesdeContexto(ctx),
			Origen:         origenSolicitudDesdeContexto(ctx),
		})
		if err != nil {
			return nil, mapearErrorDominio(ctx, err)
		}
		vista = v
	}

	return &ActualizarSalaOutput{Body: vistaSalaRespuestaDesde(vista)}, nil
}

// --- Proyecciones VistaX -> DTO de respuesta ---------------------------------

func respuestaTurnoDesde(r puertos.ResultadoTurno, conTicket bool) respuestaTurno {
	resp := respuestaTurno{
		Desenlace:              r.Desenlace,
		Posicion:               r.Posicion,
		LongitudCola:           r.LongitudCola,
		EsperaEstimadaSegundos: segundosCeil(r.EsperaEstimada),
		TurnoEstimadoEn:        r.TurnoEstimadoEn,
		ReconsultarEnMs:        r.ReconsultarEn.Milliseconds(),
	}
	if conTicket {
		resp.Ticket = r.TicketPlano
	}
	return resp
}

func respuestaSalaPublicaDesde(v puertos.VistaSalaPublica) respuestaSalaPublica {
	return respuestaSalaPublica{
		Alias:                  v.Alias,
		Estado:                 v.Estado,
		LongitudAproximada:     v.LongitudAproximada,
		EsperaEstimadaSegundos: segundosCeil(v.EsperaEstimada),
		ReconsultarEnMs:        v.ReconsultarEn.Milliseconds(),
	}
}

func vistaSalaRespuestaDesde(v puertos.VistaSala) vistaSalaRespuesta {
	return vistaSalaRespuesta{
		ID:                     v.ID,
		Alias:                  v.Alias,
		AlcanceTipo:            v.AlcanceTipo,
		IDOrganizacion:         v.IDOrganizacion,
		Ruta:                   v.Ruta,
		Estado:                 v.Estado,
		RitmoAdmision:          v.RitmoAdmision,
		CapacidadMaximaCola:    v.CapacidadMaximaCola,
		VentanaReclamoSegundos: int64(v.VentanaReclamo.Seconds()),
		ModoDegradado:          v.ModoDegradado,
		CreadaPor:              v.CreadaPor,
		CreadaEn:               v.CreadaEn,
		AbiertaEn:              v.AbiertaEn,
		CerradaEn:              v.CerradaEn,
	}
}
