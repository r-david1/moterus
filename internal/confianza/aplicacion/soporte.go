package aplicacion

// Helpers compartidos entre los casos de uso de colas de acceso virtual
// (docs/design/colas-virtuales.md). Mismo criterio que
// acceso/aplicacion/soporte.go y tenencia/aplicacion/soporte.go: funciones
// puras de mapeo/traducción entre el agregado de dominio y los DTOs
// primitivos de puertos, sin E/S propia.

import (
	"time"

	"github.com/r-david1/moterus/internal/confianza/dominio"
	"github.com/r-david1/moterus/internal/confianza/puertos"
)

// verificarPertenenciaOrganizacion refuerza, del lado del caso de uso, que
// una sala cargada por IDSala pertenece a la organización que ya autorizó
// el endpoint HTTP org-scoped (§7.2 del diseño). idOrganizacionEsperada=""
// significa que el llamador es de confianza para tocar cualquier sala (el
// subcomando de CLI que opera salas de alcance sistema fuera de la API) y
// no se verifica nada.
//
// Sin este chequeo, el middleware de autorización de Tenencia solo
// confirma que el sujeto tiene el permiso sobre el {idOrganizacion} de la
// RUTA — nunca sobre la sala que el comando termina mutando. Un
// administrador autorizado sobre su propia organización podría entonces
// pasar el IDSala de una sala de alcance sistema, o de otra organización,
// y mutarla igual (IDOR clásico: autorización sobre un recurso de la URL,
// operación real sobre un recurso distinto tomado del cuerpo).
//
// Devuelve ErrSalaNoEncontrada, no un error de autorización, en la
// discrepancia: mismo criterio de "no oráculo" que el resto del catálogo
// de errores de esta extensión (§1.8 del diseño) — confirmar que una sala
// existe pero pertenece a otra organización (o al sistema) es información
// que un administrador de una organización ajena no debe poder distinguir
// de "esa sala no existe".
func verificarPertenenciaOrganizacion(sala *dominio.SalaDeEspera, idOrganizacionEsperada, idSalaReferencia string) error {
	if idOrganizacionEsperada == "" {
		return nil
	}
	esperado, err := dominio.IDOrganizacionDesde(idOrganizacionEsperada)
	if err != nil {
		// Un IDOrganizacion mal formado nunca puede coincidir con nada: se
		// trata igual que una discrepancia (denegar), no como un 422 aparte.
		return &dominio.ErrSalaNoEncontrada{Referencia: idSalaReferencia}
	}
	idOrg, esOrgScoped := sala.Alcance().OrganizacionID()
	if !esOrgScoped || !idOrg.EsIgual(esperado) {
		return &dominio.ErrSalaNoEncontrada{Referencia: idSalaReferencia}
	}
	return nil
}

// alcanceSalaDesde construye un dominio.AlcanceSala a partir del
// IDOrganizacion primitivo que transportan los comandos de administración
// (§2.1 del diseño): "" significa alcance sistema.
func alcanceSalaDesde(idOrganizacion string) (dominio.AlcanceSala, error) {
	if idOrganizacion == "" {
		return dominio.AlcanceSistema(), nil
	}
	id, err := dominio.IDOrganizacionDesde(idOrganizacion)
	if err != nil {
		return dominio.AlcanceSala{}, err
	}
	return dominio.AlcanceOrganizacion(id)
}

// creadaPorDesde construye el puntero *dominio.IDUsuario que exige
// dominio.NuevaSalaDeEspera: nil para una sala de alcance sistema, operada
// fuera de la API (§7.2 del diseño).
func creadaPorDesde(idSujeto string) (*dominio.IDUsuario, error) {
	if idSujeto == "" {
		return nil, nil
	}
	id, err := dominio.IDUsuarioDesde(idSujeto)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

// claveSalaTextual reconstruye el valor textual de dominio.ClaveSala
// ("<alcance>:<ruta>") a partir de los primitivos que transporta
// ConsultaSalaVigente. El middleware HTTP no conoce el dominio (comentario
// de ConsultaSalaVigente en puertos/entrada.go), así que este helper replica
// la forma de AlcanceSala.Clave()+":"+RutaProtegida.String() sin pasar por
// el agregado.
func claveSalaTextual(ruta, idOrganizacion string) string {
	if idOrganizacion == "" {
		return dominio.TipoAlcanceSistema.String() + ":" + ruta
	}
	return "org:" + idOrganizacion + ":" + ruta
}

// proyeccionDesde traduce el estado vigente del agregado a la forma
// primitiva que EstadoDeCola.Proyectar necesita escribir en Redis
// (puertos.ProyeccionSala, §2.2 del diseño).
//
// Decisión de diseño no especificada por el documento (necesaria para
// completar la firma del puerto, que exige un Version int64 "monótono: una
// proyección vieja nunca pisa a una nueva"): se usa
// sala.RelojDesde().UnixNano() en vez de agregar un campo de versión nuevo
// al agregado (que no lo expone y está fuera de alcance de este cambio).
// RelojDesde solo cambia en SalaDeEspera.Abrir/CambiarRitmo — nunca en una
// reproyección rutinaria del reconciliador (§3.7) sobre la misma
// configuración — así que dos proyecciones sucesivas sin mutación de por
// medio comparten Version, y cualquier mutación real produce una Version
// mayor. Satisface el contrato sin inventar estado nuevo en dominio.
func proyeccionDesde(sala *dominio.SalaDeEspera) puertos.ProyeccionSala {
	return puertos.ProyeccionSala{
		Clave:               sala.Clave().String(),
		Alias:               sala.Alias().Normalizado(),
		Estado:              sala.Estado().String(),
		RitmoAdmision:       sala.Politica().RitmoAdmision().PorSegundo(),
		CapacidadMaximaCola: sala.Politica().CapacidadMaximaCola(),
		VentanaReclamo:      sala.Politica().VentanaReclamo(),
		CursorBase:          sala.CursorBase(),
		RelojDesde:          sala.RelojDesde(),
		Version:             sala.RelojDesde().UnixNano(),
	}
}

// vistaSalaVigenteDesde proyecta la sala a la vista minimalista que consume
// la instantánea en memoria del reconciliador y, a través de ella,
// PorteroDeSala.SalaVigentePara (§3.7 y §7.3 del diseño).
func vistaSalaVigenteDesde(sala *dominio.SalaDeEspera) puertos.VistaSalaVigente {
	return puertos.VistaSalaVigente{
		Alias:         sala.Alias().Normalizado(),
		Clave:         sala.Clave().String(),
		Estado:        sala.Estado().String(),
		ModoDegradado: sala.Politica().ModoDegradado().String(),
		RitmoAdmision: sala.Politica().RitmoAdmision().PorSegundo(),
	}
}

// vistaSalaDesde proyecta el agregado a la vista completa que devuelven los
// endpoints de administración (§2.1 del diseño, VistaSala): a diferencia de
// VistaSalaPublica, esta vista sí puede revelar el dueño y la ruta
// protegida.
func vistaSalaDesde(sala *dominio.SalaDeEspera) puertos.VistaSala {
	creadaPor, _ := sala.CreadaPor()
	idOrganizacion := ""
	if id, ok := sala.Alcance().OrganizacionID(); ok {
		idOrganizacion = id.String()
	}
	var abiertaEn *time.Time
	if t, ok := sala.AbiertaEn(); ok {
		copia := t
		abiertaEn = &copia
	}
	var cerradaEn *time.Time
	if t, ok := sala.CerradaEn(); ok {
		copia := t
		cerradaEn = &copia
	}
	return puertos.VistaSala{
		ID:                  sala.ID().String(),
		Alias:               sala.Alias().Normalizado(),
		AlcanceTipo:         sala.Alcance().Tipo().String(),
		IDOrganizacion:      idOrganizacion,
		Ruta:                sala.Ruta().String(),
		Estado:              sala.Estado().String(),
		RitmoAdmision:       sala.Politica().RitmoAdmision().PorSegundo(),
		CapacidadMaximaCola: sala.Politica().CapacidadMaximaCola(),
		VentanaReclamo:      sala.Politica().VentanaReclamo(),
		ModoDegradado:       sala.Politica().ModoDegradado().String(),
		CreadaPor:           creadaPor.String(),
		CreadaEn:            sala.CreadaEn(),
		AbiertaEn:           abiertaEn,
		CerradaEn:           cerradaEn,
	}
}

// posicionDesde calcula la posición en cola (max(0, rango-cursor), tabla 1.3
// del diseño) a partir del EstadoTicket que devuelve EstadoDeCola: réplica
// deliberada de dominio.PosicionEnCola sobre el DTO de puertos, porque
// EstadoTicket (puertos) transporta enteros crudos, no un
// dominio.RangoEnCola ya construido.
func posicionDesde(e puertos.EstadoTicket) int64 {
	pos := e.Rango - e.Cursor
	if pos < 0 {
		return 0
	}
	return pos
}

// esperaEstimadaDesde calcula cuánto falta para el turno estimado, nunca
// negativo (un turno ya alcanzado espera 0, no un valor negativo).
func esperaEstimadaDesde(turnoEstimadoEn, ahora time.Time) time.Duration {
	if turnoEstimadoEn.After(ahora) {
		return turnoEstimadoEn.Sub(ahora)
	}
	return 0
}

// Umbrales de reconsultarEnDesdePosicion (§7.1 del diseño: "un intervalo
// dictado por el servidor y proporcional a la posición, p. ej. 15s si faltan
// miles de turnos, 2s si el turno es inminente"). El documento da esos dos
// puntos como ejemplo, no como tabla cerrada; se agrega un escalón
// intermedio razonable para no saltar de 2s a 15s de golpe.
const (
	reconsultaInmediata  = 2 * time.Second
	reconsultaIntermedia = 5 * time.Second
	reconsultaLejana     = 15 * time.Second

	umbralPosicionInmediata  = 50
	umbralPosicionIntermedia = 1000
)

// reconsultarEnDesdePosicion decide el intervalo de sondeo que el servidor
// dicta al cliente (ResultadoTurno.ReconsultarEn, §7.1 del diseño).
func reconsultarEnDesdePosicion(posicion int64) time.Duration {
	switch {
	case posicion <= umbralPosicionInmediata:
		return reconsultaInmediata
	case posicion <= umbralPosicionIntermedia:
		return reconsultaIntermedia
	default:
		return reconsultaLejana
	}
}
