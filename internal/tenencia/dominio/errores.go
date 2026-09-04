package dominio

import (
	"fmt"
	"time"
)

// Errores de dominio tipados (tabla 1.5 del diseño del contexto Tenencia).
//
// Se implementan como tipos propios (no errors.New ad hoc) para que la capa
// de aplicación pueda mapearlos a códigos HTTP inspeccionando el tipo con
// errors.As. Varios de estos errores no los produce nunca tenencia/dominio
// directamente (los produce el ACL de Identidad, el adaptador Postgres o el
// propio caso de uso), pero se declaran aquí porque el vocabulario de
// errores pertenece al dominio — mismo criterio que
// acceso/dominio.ErrAccesoDenegadoPorConfianza.
//
// Este archivo también reúne los errores de construcción de cada value
// object (identificadores, alias, tokens, estados, roles, permisos...): a
// diferencia de identidad/dominio (que los declara junto a cada VO),
// tenencia/dominio sigue el criterio más reciente de acceso/dominio de
// centralizarlos en un único archivo de errores, que es lo que exige la
// estructura de carpetas del §5 del diseño (un solo errores.go).

// --- errores de construcción de identificadores -----------------------------

// ErrIDOrganizacionInvalido se produce al construir un IDOrganizacion con
// formato inválido o con el UUID nulo.
type ErrIDOrganizacionInvalido struct{ Motivo string }

func (e *ErrIDOrganizacionInvalido) Error() string {
	return "identificador de organización inválido: " + e.Motivo
}

// ErrIDMembresiaInvalido se produce al construir un IDMembresia con formato
// inválido o con el UUID nulo.
type ErrIDMembresiaInvalido struct{ Motivo string }

func (e *ErrIDMembresiaInvalido) Error() string {
	return "identificador de membresía inválido: " + e.Motivo
}

// ErrIDInvitacionInvalido se produce al construir un IDInvitacion con
// formato inválido o con el UUID nulo.
type ErrIDInvitacionInvalido struct{ Motivo string }

func (e *ErrIDInvitacionInvalido) Error() string {
	return "identificador de invitación inválido: " + e.Motivo
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

// ErrAliasInvalido se produce al construir un AliasOrganizacion que
// incumple sus reglas estructurales (longitud, alfabeto, reservado...).
type ErrAliasInvalido struct{ Motivo string }

func (e *ErrAliasInvalido) Error() string { return "alias de organización inválido: " + e.Motivo }

// ErrAliasYaRegistrado se produce al dar de alta o renombrar una
// organización con un alias ya existente. Lo produce el adaptador Postgres
// al traducir la violación del índice único (mismo patrón que
// identidad/dominio.ErrCorreoYaRegistrado); se declara en el dominio porque
// el vocabulario de errores pertenece aquí.
type ErrAliasYaRegistrado struct{ Alias string }

func (e *ErrAliasYaRegistrado) Error() string { return "el alias ya está en uso" }

// ErrNombreOrganizacionInvalido se produce al construir un
// NombreOrganizacion vacío, con caracteres de control o demasiado largo.
type ErrNombreOrganizacionInvalido struct{ Motivo string }

func (e *ErrNombreOrganizacionInvalido) Error() string {
	return "nombre de organización inválido: " + e.Motivo
}

// ErrCorreoDestinatarioInvalido se produce al construir un
// CorreoDestinatario que incumple sus reglas estructurales.
type ErrCorreoDestinatarioInvalido struct{ Motivo string }

func (e *ErrCorreoDestinatarioInvalido) Error() string {
	return "correo de destinatario inválido: " + e.Motivo
}

// ErrTokenInvitacionPlanoInvalido se produce por restricciones
// estructurales de TokenInvitacionPlano (prefijo, longitud o alfabeto
// incorrectos). Nunca incluye el valor recibido.
type ErrTokenInvitacionPlanoInvalido struct{ Motivo string }

func (e *ErrTokenInvitacionPlanoInvalido) Error() string {
	return "token de invitación inválido: " + e.Motivo
}

// ErrHashTokenInvitacionInvalido se produce al construir un
// HashTokenInvitacion con un formato irreconocible.
type ErrHashTokenInvitacionInvalido struct{ Motivo string }

func (e *ErrHashTokenInvitacionInvalido) Error() string {
	return "hash de token de invitación inválido: " + e.Motivo
}

// ErrMotivoCambioEstadoInvalido se produce al construir un
// MotivoCambioEstado vacío o demasiado largo.
type ErrMotivoCambioEstadoInvalido struct{ Motivo string }

func (e *ErrMotivoCambioEstadoInvalido) Error() string { return "motivo inválido: " + e.Motivo }

// ErrEstadoOrganizacionInvalido se produce al construir un
// EstadoOrganizacion con un valor fuera del catálogo cerrado.
type ErrEstadoOrganizacionInvalido struct{ Valor string }

func (e *ErrEstadoOrganizacionInvalido) Error() string {
	return "estado de organización desconocido: " + e.Valor
}

// ErrEstadoMembresiaInvalido se produce al construir un EstadoMembresia con
// un valor fuera del catálogo cerrado.
type ErrEstadoMembresiaInvalido struct{ Valor string }

func (e *ErrEstadoMembresiaInvalido) Error() string {
	return "estado de membresía desconocido: " + e.Valor
}

// ErrEstadoInvitacionInvalido se produce al construir un EstadoInvitacion
// con un valor fuera del catálogo cerrado.
type ErrEstadoInvitacionInvalido struct{ Valor string }

func (e *ErrEstadoInvitacionInvalido) Error() string {
	return "estado de invitación desconocido: " + e.Valor
}

// ErrRolInvalido se produce al construir un Rol con un valor fuera del
// catálogo cerrado propietario/administrador/miembro.
type ErrRolInvalido struct{ Valor string }

func (e *ErrRolInvalido) Error() string { return "rol desconocido: " + e.Valor }

// ErrPermisoDesconocido se produce cuando el permiso pedido no pertenece al
// catálogo cerrado de 8 valores. A diferencia de una denegación de
// autorización, esto es un bug del llamador (típicamente otro contexto
// pidiendo autorización con un string mal escrito); el adaptador HTTP lo
// mapea a 500, no a 403 — enmascararlo como denegación escondería un error
// de programación detrás de un comportamiento plausible.
type ErrPermisoDesconocido struct{ Valor string }

func (e *ErrPermisoDesconocido) Error() string { return "permiso desconocido: " + e.Valor }

// ErrMotivoDenegacionInvalido se produce al construir un MotivoDenegacion
// con un valor fuera del catálogo cerrado de cuatro motivos.
type ErrMotivoDenegacionInvalido struct{ Valor string }

func (e *ErrMotivoDenegacionInvalido) Error() string {
	return "motivo de denegación desconocido: " + e.Valor
}

// ErrPoliticaOrganizacionInvalida se produce al construir una
// PoliticaOrganizacion que incumple alguna de sus invariantes. Falla al
// arrancar el proceso (config inválida), no en caliente.
type ErrPoliticaOrganizacionInvalida struct{ Motivo string }

func (e *ErrPoliticaOrganizacionInvalida) Error() string {
	return "política de organización inválida: " + e.Motivo
}

// --- errores de negocio de los agregados ------------------------------------

// ErrOrganizacionNoEncontrada se produce en consultas por ID. El adaptador
// HTTP lo colapsa con ErrNoEsMiembro en un mismo 404 (INV-TEN-17).
type ErrOrganizacionNoEncontrada struct{ ID string }

func (e *ErrOrganizacionNoEncontrada) Error() string { return "organización no encontrada" }

// ErrOrganizacionNoOperativa se produce al operar sobre una organización
// suspendida o archivada.
type ErrOrganizacionNoOperativa struct{ Estado string }

func (e *ErrOrganizacionNoOperativa) Error() string {
	return "la organización no está operativa: " + e.Estado
}

// ErrTransicionEstadoOrganizacionInvalida se produce cuando se intenta una
// transición de EstadoOrganizacion no permitida por la máquina de estados.
type ErrTransicionEstadoOrganizacionInvalida struct{ Origen, Destino EstadoOrganizacion }

func (e *ErrTransicionEstadoOrganizacionInvalida) Error() string {
	return fmt.Sprintf("transición de estado de organización inválida: %s -> %s", e.Origen.String(), e.Destino.String())
}

// ErrMembresiaNoEncontrada se produce en consultas por (organización,
// usuario) sin membresía vigente.
type ErrMembresiaNoEncontrada struct{}

func (e *ErrMembresiaNoEncontrada) Error() string { return "membresía no encontrada" }

// ErrMembresiaDuplicada se produce al agregar a alguien que ya es miembro
// no removido. Lo produce el adaptador al traducir el índice único parcial
// (INV-TEN-09).
type ErrMembresiaDuplicada struct{}

func (e *ErrMembresiaDuplicada) Error() string { return "el usuario ya es miembro de la organización" }

// ErrNoEsMiembro se produce cuando el sujeto no tiene membresía en la
// organización. Se mapea a 404, no a 403: un 403 confirmaría que esa
// organización existe (mismo criterio que ErrSesionAjena en acceso/dominio).
type ErrNoEsMiembro struct{}

func (e *ErrNoEsMiembro) Error() string { return "el sujeto no es miembro de la organización" }

// ErrNoAutorizado se produce cuando la membresía existe pero no alcanza
// para el permiso pedido. Lleva el MotivoDenegacion y el rol actual del
// sujeto.
type ErrNoAutorizado struct {
	Motivo    MotivoDenegacion
	RolActual string
}

func (e *ErrNoAutorizado) Error() string { return "no autorizado: " + e.Motivo.Valor() }

// ErrRolSuperiorAlPropio se produce por el primer paso de la regla de
// dominancia: nadie puede otorgar un rol estrictamente superior al propio
// (INV-TEN-20).
type ErrRolSuperiorAlPropio struct{ Ejecutor, Nuevo string }

func (e *ErrRolSuperiorAlPropio) Error() string {
	return fmt.Sprintf("no se puede otorgar el rol %s siendo %s", e.Nuevo, e.Ejecutor)
}

// ErrMembresiaDominante se produce por el segundo paso de la regla de
// dominancia: nadie puede modificar ni remover una membresía cuyo rol sea
// estrictamente superior al propio (INV-TEN-20).
type ErrMembresiaDominante struct{ Ejecutor, Objetivo string }

func (e *ErrMembresiaDominante) Error() string {
	return fmt.Sprintf("no se puede actuar sobre una membresía %s siendo %s", e.Objetivo, e.Ejecutor)
}

// ErrUltimoPropietario se produce cuando la operación dejaría la
// organización sin propietario activo (INV-TEN-06). Mensaje accionable: es
// el callejón sin salida más probable del producto (§3.2 del diseño).
type ErrUltimoPropietario struct{}

func (e *ErrUltimoPropietario) Error() string {
	return "la organización quedaría sin propietario activo: transferí la propiedad antes de salir"
}

// ErrSujetoNoElegible se produce cuando el usuario_id no existe o no está
// activo en Identidad. Traducción en el ACL: tenencia/aplicacion nunca ve
// un tipo de Identidad (INV-TEN-28).
type ErrSujetoNoElegible struct{}

func (e *ErrSujetoNoElegible) Error() string { return "el sujeto no es elegible" }

// ErrTransicionEstadoInvitacionInvalida se produce cuando se intenta una
// transición de EstadoInvitacion no permitida por la máquina de estados
// (p. ej. revocar o marcar expirada una invitación que ya se resolvió).
// Distinto de ErrInvitacionInvalida: este error lo produce una acción
// administrativa (RevocarInvitacion), no un intento de redención por un
// tercero, así que no aplica el criterio de indistinguibilidad de
// INV-TEN-24.
type ErrTransicionEstadoInvitacionInvalida struct{ Origen, Destino EstadoInvitacion }

func (e *ErrTransicionEstadoInvitacionInvalida) Error() string {
	return fmt.Sprintf("transición de estado de invitación inválida: %s -> %s", e.Origen.String(), e.Destino.String())
}

// ErrInvitacionInvalida se produce por token desconocido, malformado,
// revocado, ya aceptado o expirado — deliberadamente único para los cinco
// casos (INV-TEN-24), igual que ErrRefrescoInvalido en acceso/dominio.
type ErrInvitacionInvalida struct{}

func (e *ErrInvitacionInvalida) Error() string { return "invitación inválida" }

// ErrInvitacionAjena se produce cuando el correo del sujeto autenticado no
// coincide con el destinatario de la invitación. Mismo tratamiento
// observable que ErrInvitacionInvalida (INV-TEN-24); existe como tipo
// propio porque dispara una acción de auditoría distinta.
type ErrInvitacionAjena struct{}

func (e *ErrInvitacionAjena) Error() string { return "invitación inválida" }

// ErrInvitacionDuplicada se produce en el caso de carrera de una invitación
// pendiente concurrente para el mismo (organización, correo): reinvitar
// revoca la anterior en vez de fallar (§3.5 del diseño); este error queda
// para cuando dos solicitudes de invitación colisionan.
type ErrInvitacionDuplicada struct{}

func (e *ErrInvitacionDuplicada) Error() string {
	return "ya existe una invitación pendiente para ese destinatario"
}

// ErrLimiteMiembrosExcedido se produce al superar
// PoliticaOrganizacion.MaximoMiembrosActivos.
type ErrLimiteMiembrosExcedido struct{ Limite int }

func (e *ErrLimiteMiembrosExcedido) Error() string {
	return fmt.Sprintf("se alcanzó el máximo de miembros activos (%d)", e.Limite)
}

// ErrLimiteInvitacionesExcedido se produce al superar
// PoliticaOrganizacion.MaximoInvitacionesPendientes.
type ErrLimiteInvitacionesExcedido struct{ Limite int }

func (e *ErrLimiteInvitacionesExcedido) Error() string {
	return fmt.Sprintf("se alcanzó el máximo de invitaciones pendientes (%d)", e.Limite)
}

// ErrLimiteOrganizacionesExcedido se produce al superar
// PoliticaOrganizacion.MaximoOrganizacionesPorUsuario.
type ErrLimiteOrganizacionesExcedido struct{ Limite int }

func (e *ErrLimiteOrganizacionesExcedido) Error() string {
	return fmt.Sprintf("se alcanzó el máximo de organizaciones propias (%d)", e.Limite)
}

// ErrTransicionEstadoMembresiaInvalida se produce cuando se intenta una
// transición de EstadoMembresia no permitida por la máquina de estados.
type ErrTransicionEstadoMembresiaInvalida struct{ Origen, Destino EstadoMembresia }

func (e *ErrTransicionEstadoMembresiaInvalida) Error() string {
	return fmt.Sprintf("transición de estado de membresía inválida: %s -> %s", e.Origen.String(), e.Destino.String())
}

// ErrAccesoDenegadoPorConfianza se produce cuando el contexto Confianza
// bloquea una operación (crear organización, invitar, aceptar invitación).
// Mismo nombre y misma forma que el homónimo de identidad/dominio y
// acceso/dominio, pero tipo propio de tenencia/dominio (§1.7 del diseño):
// el adaptador HTTP lo mapea a 429 con Retry-After.
type ErrAccesoDenegadoPorConfianza struct {
	Motivo       string
	ReintentarEn time.Duration
}

func (e *ErrAccesoDenegadoPorConfianza) Error() string {
	return "acceso denegado por evaluación de confianza"
}

// ErrConcurrenciaMembresia se produce ante un conflicto de versión
// optimista o una violación del CONSTRAINT TRIGGER diferido de INV-TEN-06.
// Es reintentable.
type ErrConcurrenciaMembresia struct{}

func (e *ErrConcurrenciaMembresia) Error() string {
	return "conflicto de concurrencia sobre la membresía"
}
