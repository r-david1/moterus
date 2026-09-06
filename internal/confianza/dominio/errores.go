package dominio

import "fmt"

// Errores de dominio tipados (§1.8 del diseño). Se implementan como tipos
// propios (no errors.New ad hoc) para que la capa de aplicación pueda
// mapearlos a códigos HTTP inspeccionando el tipo con errors.As.
//
// Nota deliberada sobre por qué acá NO se colapsan los errores de
// resolución de un ticket (ErrTurnoNoAlcanzado, ErrTurnoCaducado,
// ErrTicketDesconocido, ErrTicketConsumido) — a diferencia de INV-ACC-21,
// INV-TEN-24 e INV-MFA-08, que sí colapsan sus respectivos catálogos: esos
// tres casos evitan construir un oráculo sobre un secreto (¿existe esta
// cuenta?, ¿es válido este token?). Un ticket de cola no protege ningún
// secreto — reingresar es gratis, público y sin credenciales, y para ver la
// diferencia entre "desconocido" y "consumido" hay que poseer el ticket. No
// hay oráculo que cerrar, y distinguirlos es la diferencia entre un
// frontend que sabe qué mostrarle al usuario y uno que no (§1.8 del
// diseño).
//
// Este archivo también reúne los errores de construcción de cada value
// object (identificadores, alias, ticket, estados, política...), siguiendo
// el criterio más reciente del repositorio (acceso/dominio, tenencia/dominio)
// de centralizarlos en un único errores.go en vez de declararlos junto a
// cada VO.

// --- errores de construcción de identificadores -----------------------------

// ErrIDSalaDeEsperaInvalido se produce al construir un IDSalaDeEspera con
// formato inválido o con el UUID nulo.
type ErrIDSalaDeEsperaInvalido struct{ Motivo string }

func (e *ErrIDSalaDeEsperaInvalido) Error() string {
	return "identificador de sala de espera inválido: " + e.Motivo
}

// ErrIDOrganizacionInvalido se produce al construir un IDOrganizacion con
// formato inválido o con el UUID nulo.
type ErrIDOrganizacionInvalido struct{ Motivo string }

func (e *ErrIDOrganizacionInvalido) Error() string {
	return "identificador de organización inválido: " + e.Motivo
}

// ErrIDUsuarioInvalido se produce al construir un IDUsuario con formato
// inválido o con el UUID nulo.
type ErrIDUsuarioInvalido struct{ Motivo string }

func (e *ErrIDUsuarioInvalido) Error() string {
	return "identificador de usuario inválido: " + e.Motivo
}

// --- errores de construcción de otros value objects -------------------------

// ErrDireccionIPInvalida se produce al construir una DireccionIP con un
// valor no vacío pero irreconocible.
type ErrDireccionIPInvalida struct{ Motivo string }

func (e *ErrDireccionIPInvalida) Error() string { return "dirección IP inválida: " + e.Motivo }

// ErrAliasSalaInvalido se produce al construir un AliasSala que incumple
// sus reglas estructurales (longitud, alfabeto...).
type ErrAliasSalaInvalido struct{ Motivo string }

func (e *ErrAliasSalaInvalido) Error() string {
	return "alias de sala de espera inválido: " + e.Motivo
}

// ErrAliasSalaYaRegistrado se produce al abrir una sala con un alias ya
// existente. Lo produce el adaptador Postgres al traducir la violación del
// índice único salas_espera_alias_idx (§6.1 del diseño); se declara aquí
// porque el vocabulario de errores pertenece al dominio.
type ErrAliasSalaYaRegistrado struct{ Alias string }

func (e *ErrAliasSalaYaRegistrado) Error() string { return "el alias ya está en uso" }

// ErrAlcanceSalaInvalido se produce al construir un AlcanceSala
// incoherente (p. ej. alcance organizacion sin IDOrganizacion, o alcance
// sistema con uno).
type ErrAlcanceSalaInvalido struct{ Motivo string }

func (e *ErrAlcanceSalaInvalido) Error() string { return "alcance de sala inválido: " + e.Motivo }

// ErrRutaNoProtegible se produce cuando la ruta pedida está fuera del
// catálogo cerrado de RutaProtegida, o cuando la ruta no admite el alcance
// solicitado (§1.6 del diseño).
type ErrRutaNoProtegible struct{ Motivo string }

func (e *ErrRutaNoProtegible) Error() string { return "ruta no protegible: " + e.Motivo }

// ErrRitmoAdmisionInvalido se produce al construir un RitmoAdmision fuera
// del rango [1, 10000].
type ErrRitmoAdmisionInvalido struct{ Motivo string }

func (e *ErrRitmoAdmisionInvalido) Error() string { return "ritmo de admisión inválido: " + e.Motivo }

// ErrModoDegradadoInvalido se produce al construir un ModoDegradado con un
// valor fuera del catálogo cerrado permitir/rechazar.
type ErrModoDegradadoInvalido struct{ Valor string }

func (e *ErrModoDegradadoInvalido) Error() string { return "modo degradado desconocido: " + e.Valor }

// ErrPoliticaSalaInvalida se produce al construir una PoliticaSala que
// incumple alguna de sus invariantes. Falla al arrancar el proceso (config
// inválida) o en el PATCH de administración (422).
type ErrPoliticaSalaInvalida struct{ Motivo string }

func (e *ErrPoliticaSalaInvalida) Error() string { return "política de sala inválida: " + e.Motivo }

// ErrEstadoSalaInvalido se produce al construir un EstadoSala con un valor
// fuera del catálogo cerrado.
type ErrEstadoSalaInvalido struct{ Valor string }

func (e *ErrEstadoSalaInvalido) Error() string { return "estado de sala desconocido: " + e.Valor }

// ErrDesenlaceDeAdmisionInvalido se produce al construir un
// DesenlaceDeAdmision con un valor fuera del catálogo cerrado de siete
// valores.
type ErrDesenlaceDeAdmisionInvalido struct{ Valor string }

func (e *ErrDesenlaceDeAdmisionInvalido) Error() string {
	return "desenlace de admisión desconocido: " + e.Valor
}

// ErrEstadoTicketInvalido se produce al construir un EstadoTicket con un
// valor fuera del catálogo cerrado esperando/consumido.
type ErrEstadoTicketInvalido struct{ Valor string }

func (e *ErrEstadoTicketInvalido) Error() string { return "estado de ticket desconocido: " + e.Valor }

// ErrTicketPlanoInvalido se produce por restricciones estructurales de
// TicketPlano (prefijo, longitud o alfabeto incorrectos). Nunca incluye el
// valor recibido.
type ErrTicketPlanoInvalido struct{ Motivo string }

func (e *ErrTicketPlanoInvalido) Error() string { return "ticket de cola inválido: " + e.Motivo }

// ErrHashTicketInvalido se produce al construir un HashTicket con un
// formato irreconocible.
type ErrHashTicketInvalido struct{ Motivo string }

func (e *ErrHashTicketInvalido) Error() string { return "hash de ticket inválido: " + e.Motivo }

// ErrRangoEnColaInvalido se produce al construir un RangoEnCola menor a 1.
type ErrRangoEnColaInvalido struct{ Motivo string }

func (e *ErrRangoEnColaInvalido) Error() string { return "rango en cola inválido: " + e.Motivo }

// ErrClaveSalaInvalida se produce al envolver una ClaveSala vacía, p. ej.
// al construir un TicketDeCola sin la clave de su sala.
type ErrClaveSalaInvalida struct{ Motivo string }

func (e *ErrClaveSalaInvalida) Error() string { return "clave de sala inválida: " + e.Motivo }

// --- errores de negocio del agregado SalaDeEspera ---------------------------

// ErrSalaNoEncontrada se produce en consultas por alias o por ID.
type ErrSalaNoEncontrada struct{ Referencia string }

func (e *ErrSalaNoEncontrada) Error() string { return "sala de espera no encontrada" }

// ErrSalaYaAbiertaParaLaRuta se produce al abrir una sala cuando ya existe
// una sala no cerrada para el mismo (alcance, ruta) — INV-COLA-01. Lo
// produce el adaptador Postgres al traducir la violación del índice único
// parcial salas_espera_vigente_idx (§6.1 del diseño); la garantía es
// estructural, no una consulta previa del caso de uso.
type ErrSalaYaAbiertaParaLaRuta struct{ Clave string }

func (e *ErrSalaYaAbiertaParaLaRuta) Error() string {
	return "ya existe una sala vigente para esa ruta y alcance: " + e.Clave
}

// ErrTransicionEstadoSalaInvalida se produce cuando se intenta una
// transición de EstadoSala no permitida por la máquina de estados (§1.6 del
// diseño).
type ErrTransicionEstadoSalaInvalida struct{ Origen, Destino EstadoSala }

func (e *ErrTransicionEstadoSalaInvalida) Error() string {
	return fmt.Sprintf("transición de estado de sala inválida: %s -> %s", e.Origen.String(), e.Destino.String())
}

// ErrSalaNoVigente se produce al intentar CambiarRitmo sobre una sala que
// no está en EstadoSalaAbierta ni EstadoSalaDrenando (programada o
// cerrada): cambiar el ritmo de admisión solo tiene sentido mientras la
// sala está viva.
type ErrSalaNoVigente struct{ Estado EstadoSala }

func (e *ErrSalaNoVigente) Error() string {
	return "la sala no está vigente (estado actual: " + e.Estado.String() + ")"
}

// ErrSalaNoAbierta se produce al intentar AdmiteIngreso sobre una sala que
// no está en EstadoSalaAbierta (ni programada, ni drenando —que ya no
// admite ingresos nuevos—, ni cerrada). Es el equivalente en dominio puro
// del desenlace DesenlaceSalaCerrada que produce el script Lua `ingresar`
// cuando la configuración no existe o el estado no es "abierta" (§6.2 del
// diseño).
type ErrSalaNoAbierta struct{ Estado EstadoSala }

func (e *ErrSalaNoAbierta) Error() string {
	return "la sala no admite ingresos nuevos (estado actual: " + e.Estado.String() + ")"
}

// ErrColaLlena se produce cuando capacidadMaximaCola ya se alcanzó
// (longitud - cursor ≥ capacidad).
type ErrColaLlena struct{}

func (e *ErrColaLlena) Error() string { return "la cola de la sala de espera está llena" }

// --- errores de resolución de un ticket -------------------------------------
//
// Los cuatro que siguen se producen al reclamar un turno (§1.8 del diseño).
// No se colapsan entre sí: ver la nota al inicio de este archivo.

// ErrTurnoNoAlcanzado se produce al reclamar un ticket cuyo rango todavía
// es mayor que el cursor de admisión (DesenlaceEsperando).
type ErrTurnoNoAlcanzado struct{ Posicion int64 }

func (e *ErrTurnoNoAlcanzado) Error() string { return "el turno todavía no llegó" }

// ErrTurnoCaducado se produce al reclamar un ticket cuyo turno ya llegó
// pero cuya ventana de reclamo ya se agotó (DesenlaceTurnoCaducado).
type ErrTurnoCaducado struct{}

func (e *ErrTurnoCaducado) Error() string { return "el turno caducó: hay que reingresar" }

// ErrTicketDesconocido se produce cuando el hash presentado no tiene
// estado en Redis: nunca existió, o ya expiró (DesenlaceTicketDesconocido).
type ErrTicketDesconocido struct{}

func (e *ErrTicketDesconocido) Error() string { return "ticket desconocido: hay que reingresar" }

// ErrTicketConsumido se produce al reclamar un ticket que ya se había
// reclamado antes (DesenlaceTicketConsumido, INV-COLA-06).
type ErrTicketConsumido struct{}

func (e *ErrTicketConsumido) Error() string { return "el ticket ya fue consumido: hay que reingresar" }

// ErrEstadoDeColaNoDisponible se produce cuando el puerto EstadoDeCola
// (Redis) no responde. El adaptador HTTP lo trata según ModoDegradado
// (§8 del diseño): fail-open o fail-closed, nunca leyendo el modo desde el
// propio Redis caído (INV-COLA-13, tercer punto de la decisión de §8).
type ErrEstadoDeColaNoDisponible struct{ Motivo string }

func (e *ErrEstadoDeColaNoDisponible) Error() string {
	return "el estado de la cola no está disponible: " + e.Motivo
}
