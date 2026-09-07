package aplicacion

import (
	"context"
	"sync/atomic"

	"github.com/r-david1/moterus/internal/confianza/dominio"
	"github.com/r-david1/moterus/internal/confianza/puertos"
)

// InstantaneaSalasVigentes es la instantánea en memoria de §3.7 del diseño
// colas-virtuales.md: el reconciliador la reemplaza por completo cada ~15s
// a partir de RepositorioSalasDeEspera.ListarVigentes(), y
// PorteroDeSalaCasoDeUso la lee sin candado (dos atomic.Pointer, uno por
// cada forma de búsqueda que el camino caliente necesita).
//
// Decisión de diseño (§3.7 del documento menciona el mecanismo pero no dicta
// quién es su dueño): ni PorteroDeSalaCasoDeUso ni ReconciliarSalasCasoDeUso
// son propietarios exclusivos de este estado — ambos lo reciben como un
// colaborador construido aparte (NuevaInstantaneaSalasVigentes) e inyectado
// por constructor a los dos, igual que un cliente Redis compartido.
// Que viva declarada en este archivo (junto a su lector más natural,
// PorteroDeSalaCasoDeUso) es solo organización de código: ReconciliarSalas,
// en el mismo paquete, la usa exactamente igual sin necesitar importarla de
// ningún otro lado.
//
// Dos índices porque el camino caliente necesita resolver la sala vigente
// de DOS formas distintas: por (ruta, idOrganizacion) — lo único que conoce
// el middleware al montarse (§7.3) — y por AliasSala — lo único que traen
// las rutas públicas de ingreso/consulta (§7.1, {aliasSala} en la URL).
// Ambos índices se reconstruyen juntos a partir de la misma lista de salas
// vigentes, así que nunca divergen entre sí.
type InstantaneaSalasVigentes struct {
	porClave atomic.Pointer[map[string]puertos.VistaSalaVigente]
	porAlias atomic.Pointer[map[string]puertos.VistaSalaVigente]
}

// NuevaInstantaneaSalasVigentes construye una instantánea vacía (sin
// ninguna sala vigente): es el estado correcto antes del primer ciclo del
// reconciliador, y también el que corresponde con el proceso recién
// arrancado.
func NuevaInstantaneaSalasVigentes() *InstantaneaSalasVigentes {
	i := &InstantaneaSalasVigentes{}
	vacioClave := map[string]puertos.VistaSalaVigente{}
	vacioAlias := map[string]puertos.VistaSalaVigente{}
	i.porClave.Store(&vacioClave)
	i.porAlias.Store(&vacioAlias)
	return i
}

// LeerPorClave busca una sala vigente por su ClaveSala textual
// ("<alcance>:<ruta>"). Es la operación que respalda
// PorteroDeSala.SalaVigentePara.
func (i *InstantaneaSalasVigentes) LeerPorClave(clave string) (puertos.VistaSalaVigente, bool) {
	m := i.porClave.Load()
	if m == nil {
		return puertos.VistaSalaVigente{}, false
	}
	v, ok := (*m)[clave]
	return v, ok
}

// LeerPorAlias busca una sala vigente por su AliasSala normalizado. La usan
// Ingresar y ConsultarTurno, que reciben el alias de la URL pública.
func (i *InstantaneaSalasVigentes) LeerPorAlias(alias string) (puertos.VistaSalaVigente, bool) {
	m := i.porAlias.Load()
	if m == nil {
		return puertos.VistaSalaVigente{}, false
	}
	v, ok := (*m)[alias]
	return v, ok
}

// reemplazar sustituye ambos índices atómicamente (cada uno con su propio
// atomic.Pointer.Store; un lector puede, en el peor caso, ver el índice por
// clave ya actualizado y el de alias todavía viejo por una fracción de
// segundo entre los dos Store — inofensivo, porque el siguiente ciclo del
// reconciliador (15s) los vuelve a alinear y ninguno de los dos índices
// aparece nunca en un estado a medio construir). No exportado: solo lo
// invoca ReconciliarSalasCasoDeUso, en este mismo paquete.
func (i *InstantaneaSalasVigentes) reemplazar(salas []*dominio.SalaDeEspera) {
	porClave := make(map[string]puertos.VistaSalaVigente, len(salas))
	porAlias := make(map[string]puertos.VistaSalaVigente, len(salas))
	for _, s := range salas {
		if s == nil {
			continue
		}
		v := vistaSalaVigenteDesde(s)
		porClave[v.Clave] = v
		porAlias[s.Alias().Normalizado()] = v
	}
	i.porClave.Store(&porClave)
	i.porAlias.Store(&porAlias)
}

// PorteroDeSalaCasoDeUso implementa puertos.PorteroDeSala (§3.4, §3.5 y §3.6
// del diseño): el puerto de camino caliente que consume EXCLUSIVAMENTE el
// middleware HTTP. Deliberadamente no tiene ninguna dependencia sobre
// puertos.RepositorioSalasDeEspera ni ningún otro puerto que hable con
// Postgres: la garantía de INV-COLA-08 (el camino caliente nunca toca
// Postgres) es aquí estructural, no solo una promesa de comportamiento — el
// tipo ni siquiera podría hacerlo si quisiera, porque no tiene con qué.
type PorteroDeSalaCasoDeUso struct {
	instantanea *InstantaneaSalasVigentes
	estadoCola  puertos.EstadoDeCola
	tickets     puertos.GeneradorTickets
	reloj       puertos.Reloj
}

var _ puertos.PorteroDeSala = (*PorteroDeSalaCasoDeUso)(nil)

// NuevoPorteroDeSalaCasoDeUso construye el caso de uso con sus dependencias
// inyectadas por puerto. instantanea se comparte con
// ReconciliarSalasCasoDeUso (ver el comentario de InstantaneaSalasVigentes).
func NuevoPorteroDeSalaCasoDeUso(
	instantanea *InstantaneaSalasVigentes,
	estadoCola puertos.EstadoDeCola,
	tickets puertos.GeneradorTickets,
	reloj puertos.Reloj,
) *PorteroDeSalaCasoDeUso {
	return &PorteroDeSalaCasoDeUso{
		instantanea: instantanea,
		estadoCola:  estadoCola,
		tickets:     tickets,
		reloj:       reloj,
	}
}

// SalaVigentePara responde desde la instantánea en memoria, sin E/S (§7.3
// del diseño, paso 1 del middleware): tiene que poder responder incluso con
// Redis caído.
func (c *PorteroDeSalaCasoDeUso) SalaVigentePara(ctx context.Context, q puertos.ConsultaSalaVigente) (puertos.VistaSalaVigente, bool) {
	return c.instantanea.LeerPorClave(claveSalaTextual(q.Ruta, q.IDOrganizacion))
}

// Ingresar ejecuta el flujo de §3.4 del diseño: valida el alias, confirma
// que hay una sala vigente para él, genera un ticket y lo ingresa de forma
// atómica en Redis a través de EstadoDeCola.Ingresar.
func (c *PorteroDeSalaCasoDeUso) Ingresar(ctx context.Context, cmd puertos.ComandoIngresarASala) (puertos.ResultadoTurno, error) {
	alias, err := dominio.NuevoAliasSala(cmd.Alias)
	if err != nil {
		return puertos.ResultadoTurno{}, err
	}
	vista, ok := c.instantanea.LeerPorAlias(alias.Normalizado())
	if !ok {
		return puertos.ResultadoTurno{}, &dominio.ErrSalaNoEncontrada{Referencia: cmd.Alias}
	}

	// TODO(§12): evaluar con dominio.AccionIngresoASala cuando exista en el
	// catálogo cerrado de acciones (dominio/accion.go) — es el único freno
	// contra el farming de tickets (§3.4 paso 2 y §4 INV-COLA-04 del
	// diseño). No se invoca todavía: la acción es un cambio aditivo
	// posterior (§12) y este caso de uso no debe depender de un tipo que
	// esta extensión todavía no declara.

	plano, err := c.tickets.GenerarTicket()
	if err != nil {
		return puertos.ResultadoTurno{}, err
	}
	hash := plano.Hash()

	estado, err := c.estadoCola.Ingresar(ctx, vista.Clave, hash.Valor())
	if err != nil {
		return puertos.ResultadoTurno{}, err
	}
	desenlace, err := dominio.DesenlaceDeAdmisionDesde(estado.Desenlace)
	if err != nil {
		return puertos.ResultadoTurno{}, err
	}
	if err := errorDesdeDesenlaceIngreso(desenlace); err != nil {
		return puertos.ResultadoTurno{}, err
	}

	posicion := posicionDesde(estado)
	return puertos.ResultadoTurno{
		TicketPlano:     plano.Valor(),
		Desenlace:       desenlace.String(),
		Posicion:        posicion,
		LongitudCola:    estado.LongitudCola,
		EsperaEstimada:  esperaEstimadaDesde(estado.TurnoEstimadoEn, c.reloj.Ahora()),
		TurnoEstimadoEn: estado.TurnoEstimadoEn,
		ReconsultarEn:   reconsultarEnDesdePosicion(posicion),
	}, nil
}

// ConsultarTurno ejecuta el flujo de §3.5 del diseño: solo lectura (más el
// refresco de TTL que ya hace el adaptador de Redis dentro del script
// `consultar`, §6.2). A diferencia de Reclamar, cualquier desenlace —
// incluidos ticket_desconocido/ticket_consumido/turno_caducado/sala_cerrada
// — es un resultado válido para el llamador (§7.1: la ruta pública responde
// 200 con el desenlace en el cuerpo), nunca un error de Go: el ticket no
// protege ningún secreto (§1.8 del diseño) y el frontend necesita saber qué
// pasó para decidir si debe reingresar.
func (c *PorteroDeSalaCasoDeUso) ConsultarTurno(ctx context.Context, q puertos.ConsultaTurno) (puertos.ResultadoTurno, error) {
	alias, err := dominio.NuevoAliasSala(q.Alias)
	if err != nil {
		return puertos.ResultadoTurno{}, err
	}
	vista, ok := c.instantanea.LeerPorAlias(alias.Normalizado())
	if !ok {
		return puertos.ResultadoTurno{}, &dominio.ErrSalaNoEncontrada{Referencia: q.Alias}
	}
	plano, err := dominio.NuevoTicketPlano(q.TicketPlano)
	if err != nil {
		return puertos.ResultadoTurno{}, err
	}
	hash := plano.Hash()

	estado, err := c.estadoCola.Consultar(ctx, vista.Clave, hash.Valor())
	if err != nil {
		return puertos.ResultadoTurno{}, err
	}
	desenlace, err := dominio.DesenlaceDeAdmisionDesde(estado.Desenlace)
	if err != nil {
		return puertos.ResultadoTurno{}, err
	}

	posicion := posicionDesde(estado)
	return puertos.ResultadoTurno{
		// TicketPlano deliberadamente vacío: solo Ingresar lo revela, la
		// única vez que sale del proceso (INV-COLA-07).
		Desenlace:       desenlace.String(),
		Posicion:        posicion,
		LongitudCola:    estado.LongitudCola,
		EsperaEstimada:  esperaEstimadaDesde(estado.TurnoEstimadoEn, c.reloj.Ahora()),
		TurnoEstimadoEn: estado.TurnoEstimadoEn,
		ReconsultarEn:   reconsultarEnDesdePosicion(posicion),
	}, nil
}

// Reclamar ejecuta el flujo de §3.6 del diseño: la única operación que
// puede dejar pasar la petición real. A diferencia de ConsultarTurno,
// cualquier desenlace que no sea "admitido" es un error de dominio (§1.8),
// porque lo invoca el middleware para decidir si corta la petición con 503
// (§7.4) — nunca un endpoint propio.
func (c *PorteroDeSalaCasoDeUso) Reclamar(ctx context.Context, cmd puertos.ComandoReclamarTurno) (puertos.ResultadoTurno, error) {
	plano, err := dominio.NuevoTicketPlano(cmd.TicketPlano)
	if err != nil {
		return puertos.ResultadoTurno{}, err
	}
	hash := plano.Hash()

	estado, err := c.estadoCola.Reclamar(ctx, cmd.Clave, hash.Valor())
	if err != nil {
		return puertos.ResultadoTurno{}, err
	}
	desenlace, err := dominio.DesenlaceDeAdmisionDesde(estado.Desenlace)
	if err != nil {
		return puertos.ResultadoTurno{}, err
	}

	posicion := posicionDesde(estado)
	resultado := puertos.ResultadoTurno{
		Desenlace:       desenlace.String(),
		Posicion:        posicion,
		LongitudCola:    estado.LongitudCola,
		EsperaEstimada:  esperaEstimadaDesde(estado.TurnoEstimadoEn, c.reloj.Ahora()),
		TurnoEstimadoEn: estado.TurnoEstimadoEn,
		ReconsultarEn:   reconsultarEnDesdePosicion(posicion),
	}
	// El resultado se devuelve incluso en el camino de error: el middleware
	// necesita posicion/espera_estimada/reconsultar_en para construir el
	// cuerpo del 503 de §7.4 (p. ej. "esperando" con la posición actual).
	if err := errorDesdeDesenlaceReclamo(desenlace, posicion); err != nil {
		return resultado, err
	}
	return resultado, nil
}

// errorDesdeDesenlaceIngreso traduce el desenlace de EstadoDeCola.Ingresar a
// un error de dominio cuando el ingreso no llegó a emitir un ticket: solo
// "sala_cerrada" (la sala dejó de admitir entre el chequeo de
// SalaVigentePara/la instantánea y el script Lua de Redis) y "cola_llena"
// son desenlaces de fallo del ingreso (§6.2 del diseño, script `ingresar`);
// "esperando" es el camino feliz — el ticket se emitió y su turno todavía no
// llegó.
func errorDesdeDesenlaceIngreso(d dominio.DesenlaceDeAdmision) error {
	switch {
	case d.EsIgual(dominio.DesenlaceSalaCerrada):
		return &dominio.ErrSalaNoAbierta{}
	case d.EsIgual(dominio.DesenlaceColaLlena):
		return &dominio.ErrColaLlena{}
	default:
		return nil
	}
}

// errorDesdeDesenlaceReclamo traduce el desenlace de EstadoDeCola.Reclamar a
// un error de dominio para cualquier resultado que no sea "admitido": es la
// única operación que puede dejar pasar la petición real (INV-COLA-06), así
// que todo lo demás debe frenarla (§1.8 del diseño).
func errorDesdeDesenlaceReclamo(d dominio.DesenlaceDeAdmision, posicion int64) error {
	switch {
	case d.EsIgual(dominio.DesenlaceAdmitido):
		return nil
	case d.EsIgual(dominio.DesenlaceEsperando):
		return &dominio.ErrTurnoNoAlcanzado{Posicion: posicion}
	case d.EsIgual(dominio.DesenlaceTurnoCaducado):
		return &dominio.ErrTurnoCaducado{}
	case d.EsIgual(dominio.DesenlaceTicketDesconocido):
		return &dominio.ErrTicketDesconocido{}
	case d.EsIgual(dominio.DesenlaceTicketConsumido):
		return &dominio.ErrTicketConsumido{}
	case d.EsIgual(dominio.DesenlaceSalaCerrada):
		return &dominio.ErrSalaNoAbierta{}
	default:
		// DesenlaceColaLlena es un desenlace de Ingresar, no de Reclamar; el
		// script `reclamar` (§6.2) nunca debería producirlo. Guarda
		// defensiva: tratarlo como fallo antes que como admisión silenciosa
		// (INV-COLA-04, la misma dirección de error segura).
		return &dominio.ErrColaLlena{}
	}
}
