package dominio

import "time"

// EventoDominio es el contrato que implementan los eventos que el contexto
// Acceso audita y publica. Mismo contrato que identidad/dominio.EventoDominio
// (NombreEvento, OcurridoEn, IDAgregado), redeclarado aquí a propósito
// (§1.7 del diseño; ADR candidato 0027): acceso/dominio no puede importar
// identidad/dominio (INV-ACC-18).
//
// A diferencia de Usuario en identidad/dominio, no todos estos eventos los
// acumula el agregado Sesion internamente. Solo SesionIniciada (emitido por
// IniciarSesion) y SesionRenovada (emitido por Rotar) representan una
// transición incondicional y autocontenida del agregado, y se drenan con
// Sesion.EventosPendientes tras persistir. Los demás — RenovacionRechazada,
// ReusoRefrescoDetectado, SesionCerrada, SesionRevocada,
// TokenAccesoRechazado — dependen de contexto de orquestación que el
// agregado no tiene (p. ej. si un cierre es individual o masivo, cuántas
// sesiones cerró, o la generación del token presentado en un intento de
// reuso) y los construye acceso/aplicacion directamente con los
// constructores NuevoX de este archivo, exactamente como identidad/aplicacion
// construye AutenticacionExitosa/AutenticacionFallida sin pasar por
// Usuario. Ningún evento transporta el token de refresco en claro, su hash,
// el JWT compacto ni la llave de firma (INV-ACC-11, verificable con un test
// de dominio que serializa cada evento y busca esos campos).
type EventoDominio interface {
	// NombreEvento identifica el tipo de evento (p. ej. "SesionIniciada").
	NombreEvento() string
	// OcurridoEn indica cuándo ocurrió el evento, siempre con la hora
	// provista por el puerto Reloj (INV-ACC-10): el dominio nunca llama a
	// time.Now().
	OcurridoEn() time.Time
	// IDAgregado identifica a la Sesion relacionada; va vacío cuando el
	// evento ocurrió sin que se pudiera resolver una sesión (p. ej. un
	// refresco desconocido).
	IDAgregado() string
}

// SesionIniciada se emite cuando una Sesion nueva nace en estado activa,
// tras una autenticación exitosa en Identidad (accion de auditoría:
// sesion.iniciada).
type SesionIniciada struct {
	IDSesion         string
	IDUsuario        string
	ExpiraAbsolutoEn time.Time
	ocurridoEn       time.Time
}

// NuevoSesionIniciada construye el evento SesionIniciada.
func NuevoSesionIniciada(id IDSesion, usuarioID IDUsuario, expiraAbsolutoEn, ocurridoEn time.Time) SesionIniciada {
	return SesionIniciada{
		IDSesion:         id.String(),
		IDUsuario:        usuarioID.String(),
		ExpiraAbsolutoEn: expiraAbsolutoEn,
		ocurridoEn:       ocurridoEn,
	}
}

func (e SesionIniciada) NombreEvento() string  { return "SesionIniciada" }
func (e SesionIniciada) OcurridoEn() time.Time { return e.ocurridoEn }
func (e SesionIniciada) IDAgregado() string    { return e.IDSesion }

// SesionRenovada se emite cuando una rotación de token de refresco tiene
// éxito (accion de auditoría: sesion.renovada, resultado exito).
type SesionRenovada struct {
	IDSesion   string
	IDUsuario  string
	Generacion int
	ocurridoEn time.Time
}

// NuevoSesionRenovada construye el evento SesionRenovada.
func NuevoSesionRenovada(id IDSesion, usuarioID IDUsuario, generacion int, ocurridoEn time.Time) SesionRenovada {
	return SesionRenovada{
		IDSesion:   id.String(),
		IDUsuario:  usuarioID.String(),
		Generacion: generacion,
		ocurridoEn: ocurridoEn,
	}
}

func (e SesionRenovada) NombreEvento() string  { return "SesionRenovada" }
func (e SesionRenovada) OcurridoEn() time.Time { return e.ocurridoEn }
func (e SesionRenovada) IDAgregado() string    { return e.IDSesion }

// RenovacionRechazada se emite cuando una renovación no prospera: refresco
// desconocido, sesión expirada o sesión revocada (accion de auditoría:
// sesion.renovada, resultado fallo). IDSesion va vacío cuando el refresco
// era desconocido (no hay sesión que nombrar).
type RenovacionRechazada struct {
	IDSesion   string
	Motivo     string
	ocurridoEn time.Time
}

// NuevoRenovacionRechazada construye el evento RenovacionRechazada.
func NuevoRenovacionRechazada(idSesion, motivo string, ocurridoEn time.Time) RenovacionRechazada {
	return RenovacionRechazada{IDSesion: idSesion, Motivo: motivo, ocurridoEn: ocurridoEn}
}

func (e RenovacionRechazada) NombreEvento() string  { return "RenovacionRechazada" }
func (e RenovacionRechazada) OcurridoEn() time.Time { return e.ocurridoEn }
func (e RenovacionRechazada) IDAgregado() string    { return e.IDSesion }

// ReusoRefrescoDetectado se emite cuando se presenta un token de refresco ya
// consumido: robo probable, revoca la sesión completa de inmediato
// (accion de auditoría: sesion.reuso_refresco_detectado, resultado
// denegado). Nunca lleva el hash ni el token, solo las generaciones
// involucradas (INV-ACC-11).
type ReusoRefrescoDetectado struct {
	IDSesion             string
	IDUsuario            string
	GeneracionPresentada int
	GeneracionVigente    int
	ocurridoEn           time.Time
}

// NuevoReusoRefrescoDetectado construye el evento ReusoRefrescoDetectado.
func NuevoReusoRefrescoDetectado(id IDSesion, usuarioID IDUsuario, generacionPresentada, generacionVigente int, ocurridoEn time.Time) ReusoRefrescoDetectado {
	return ReusoRefrescoDetectado{
		IDSesion:             id.String(),
		IDUsuario:            usuarioID.String(),
		GeneracionPresentada: generacionPresentada,
		GeneracionVigente:    generacionVigente,
		ocurridoEn:           ocurridoEn,
	}
}

func (e ReusoRefrescoDetectado) NombreEvento() string  { return "ReusoRefrescoDetectado" }
func (e ReusoRefrescoDetectado) OcurridoEn() time.Time { return e.ocurridoEn }
func (e ReusoRefrescoDetectado) IDAgregado() string    { return e.IDSesion }

// Catálogo cerrado del campo Alcance de SesionCerrada.
const (
	// AlcanceCierreIndividual identifica el cierre de una sola sesión.
	AlcanceCierreIndividual = "individual"
	// AlcanceCierreTodas identifica el cierre de todos los dispositivos.
	AlcanceCierreTodas = "todas"
)

// SesionCerrada se emite por cada sesión que el propio usuario cierra,
// individualmente o como parte de un cierre masivo (accion de auditoría:
// sesion.cerrada, resultado exito). Se emite una fila por sesión revocada,
// nunca una sola fila agregada para un cierre masivo (§3.4 del diseño): la
// bitácora forense necesita poder responder "¿cuándo murió esta sesión
// concreta?".
type SesionCerrada struct {
	IDSesion   string
	IDUsuario  string
	Alcance    string
	Cantidad   int
	ocurridoEn time.Time
}

// NuevoSesionCerrada construye el evento SesionCerrada.
func NuevoSesionCerrada(id IDSesion, usuarioID IDUsuario, alcance string, cantidad int, ocurridoEn time.Time) SesionCerrada {
	return SesionCerrada{
		IDSesion:   id.String(),
		IDUsuario:  usuarioID.String(),
		Alcance:    alcance,
		Cantidad:   cantidad,
		ocurridoEn: ocurridoEn,
	}
}

func (e SesionCerrada) NombreEvento() string  { return "SesionCerrada" }
func (e SesionCerrada) OcurridoEn() time.Time { return e.ocurridoEn }
func (e SesionCerrada) IDAgregado() string    { return e.IDSesion }

// SesionRevocada se emite cuando una sesión se revoca por una razón **no**
// iniciada por el propio usuario: cuenta no operativa, contraseña
// cambiada, límite de sesiones excedido o revocación administrativa
// (accion de auditoría: sesion.revocada, resultado exito). Nunca se usa
// para reuso de refresco: ese caso tiene su propio evento,
// ReusoRefrescoDetectado, con resultado denegado.
type SesionRevocada struct {
	IDSesion   string
	IDUsuario  string
	Motivo     string
	ocurridoEn time.Time
}

// NuevoSesionRevocada construye el evento SesionRevocada.
func NuevoSesionRevocada(id IDSesion, usuarioID IDUsuario, motivo MotivoRevocacion, ocurridoEn time.Time) SesionRevocada {
	return SesionRevocada{
		IDSesion:   id.String(),
		IDUsuario:  usuarioID.String(),
		Motivo:     motivo.Valor(),
		ocurridoEn: ocurridoEn,
	}
}

func (e SesionRevocada) NombreEvento() string  { return "SesionRevocada" }
func (e SesionRevocada) OcurridoEn() time.Time { return e.ocurridoEn }
func (e SesionRevocada) IDAgregado() string    { return e.IDSesion }

// TokenAccesoRechazado se emite **solo** para rechazos de validación con
// valor de señal: firma inválida, kid desconocido, alg/typ inesperado, o
// sid en lista de revocación (accion de auditoría: token_acceso.rechazado,
// resultado denegado). Nunca para un token simplemente expirado
// (INV-ACC-17): es el evento más frecuente del sistema y auditarlo pondría
// el hot path detrás del advisory lock de la cadena de hashes (ADR 0005).
type TokenAccesoRechazado struct {
	IDSesion   string
	Motivo     string
	ocurridoEn time.Time
}

// NuevoTokenAccesoRechazado construye el evento TokenAccesoRechazado.
func NuevoTokenAccesoRechazado(idSesion, motivo string, ocurridoEn time.Time) TokenAccesoRechazado {
	return TokenAccesoRechazado{IDSesion: idSesion, Motivo: motivo, ocurridoEn: ocurridoEn}
}

func (e TokenAccesoRechazado) NombreEvento() string  { return "TokenAccesoRechazado" }
func (e TokenAccesoRechazado) OcurridoEn() time.Time { return e.ocurridoEn }
func (e TokenAccesoRechazado) IDAgregado() string    { return e.IDSesion }
