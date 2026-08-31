package dominio

import "time"

// EventoDominio es el contrato que implementan los eventos que el agregado
// Usuario acumula durante sus operaciones de negocio y que el caso de uso
// drena (vía Usuario.EventosPendientes) para auditarlos y publicarlos tras
// persistir. Ningún evento transporta ContrasenaPlana, HashContrasena,
// códigos OTP ni tokens (INV-ID-04, INV-ID-14): se verifica con un test de
// dominio que serializa cada evento y busca esos campos.
type EventoDominio interface {
	// NombreEvento identifica el tipo de evento (p. ej. "UsuarioRegistrado").
	NombreEvento() string
	// OcurridoEn indica cuándo ocurrió el evento, siempre con la hora
	// provista por el puerto Reloj (INV-ID-10): el dominio nunca llama a
	// time.Now().
	OcurridoEn() time.Time
	// IDAgregado identifica al Usuario relacionado; va vacío si el evento
	// ocurrió antes de que existiera un agregado persistido (p. ej. un
	// registro o una autenticación rechazados por Confianza).
	IDAgregado() string
}

// UsuarioRegistrado se emite cuando un Usuario nuevo se crea con éxito, en
// estado pendiente_verificacion (accion de auditoría: usuario.registrado).
type UsuarioRegistrado struct {
	IDUsuario         string
	CorreoNormalizado string
	ocurridoEn        time.Time
}

// NuevoUsuarioRegistrado construye el evento UsuarioRegistrado.
func NuevoUsuarioRegistrado(id IDUsuario, correo Correo, ocurridoEn time.Time) UsuarioRegistrado {
	return UsuarioRegistrado{IDUsuario: id.String(), CorreoNormalizado: correo.Normalizado(), ocurridoEn: ocurridoEn}
}

func (e UsuarioRegistrado) NombreEvento() string  { return "UsuarioRegistrado" }
func (e UsuarioRegistrado) OcurridoEn() time.Time { return e.ocurridoEn }
func (e UsuarioRegistrado) IDAgregado() string    { return e.IDUsuario }

// RegistroRechazado se emite cuando Confianza deniega un intento de alta
// antes de crear el agregado; por eso no lleva IDUsuario (accion de
// auditoría: usuario.registro_rechazado).
type RegistroRechazado struct {
	CorreoNormalizado string
	Motivo            string
	ocurridoEn        time.Time
}

// NuevoRegistroRechazado construye el evento RegistroRechazado.
func NuevoRegistroRechazado(correoNormalizado, motivo string, ocurridoEn time.Time) RegistroRechazado {
	return RegistroRechazado{CorreoNormalizado: correoNormalizado, Motivo: motivo, ocurridoEn: ocurridoEn}
}

func (e RegistroRechazado) NombreEvento() string  { return "RegistroRechazado" }
func (e RegistroRechazado) OcurridoEn() time.Time { return e.ocurridoEn }
func (e RegistroRechazado) IDAgregado() string    { return "" }

// AutenticacionExitosa se emite cuando un usuario completa el login con
// contraseña válida y PuedeIniciarSesion() no devuelve error (accion de
// auditoría: usuario.login, resultado exito).
type AutenticacionExitosa struct {
	IDUsuario         string
	CorreoNormalizado string
	ocurridoEn        time.Time
}

// NuevoAutenticacionExitosa construye el evento AutenticacionExitosa.
func NuevoAutenticacionExitosa(id IDUsuario, correo Correo, ocurridoEn time.Time) AutenticacionExitosa {
	return AutenticacionExitosa{IDUsuario: id.String(), CorreoNormalizado: correo.Normalizado(), ocurridoEn: ocurridoEn}
}

func (e AutenticacionExitosa) NombreEvento() string  { return "AutenticacionExitosa" }
func (e AutenticacionExitosa) OcurridoEn() time.Time { return e.ocurridoEn }
func (e AutenticacionExitosa) IDAgregado() string    { return e.IDUsuario }

// AutenticacionFallida se emite cuando la contraseña no coincide o el
// usuario no existe. IDUsuario puede ir vacío (INV-ID-11: correo
// inexistente y contraseña incorrecta son indistinguibles fuera del
// dominio) (accion de auditoría: usuario.login, resultado fallo).
type AutenticacionFallida struct {
	IDUsuario         string
	CorreoNormalizado string
	Motivo            string
	ocurridoEn        time.Time
}

// NuevoAutenticacionFallida construye el evento AutenticacionFallida.
func NuevoAutenticacionFallida(idUsuario, correoNormalizado, motivo string, ocurridoEn time.Time) AutenticacionFallida {
	return AutenticacionFallida{IDUsuario: idUsuario, CorreoNormalizado: correoNormalizado, Motivo: motivo, ocurridoEn: ocurridoEn}
}

func (e AutenticacionFallida) NombreEvento() string  { return "AutenticacionFallida" }
func (e AutenticacionFallida) OcurridoEn() time.Time { return e.ocurridoEn }
func (e AutenticacionFallida) IDAgregado() string    { return e.IDUsuario }

// AutenticacionDenegada se emite cuando Confianza bloquea el intento de
// login antes de tocar el repositorio (accion de auditoría: usuario.login,
// resultado denegado).
type AutenticacionDenegada struct {
	CorreoNormalizado string
	Motivo            string
	ocurridoEn        time.Time
}

// NuevoAutenticacionDenegada construye el evento AutenticacionDenegada.
func NuevoAutenticacionDenegada(correoNormalizado, motivo string, ocurridoEn time.Time) AutenticacionDenegada {
	return AutenticacionDenegada{CorreoNormalizado: correoNormalizado, Motivo: motivo, ocurridoEn: ocurridoEn}
}

func (e AutenticacionDenegada) NombreEvento() string  { return "AutenticacionDenegada" }
func (e AutenticacionDenegada) OcurridoEn() time.Time { return e.ocurridoEn }
func (e AutenticacionDenegada) IDAgregado() string    { return "" }

// SegundoFactorRequerido se emite cuando el login exige un paso adicional,
// ya sea porque el usuario tiene MFA propio o porque Confianza exige
// step-up (accion de auditoría: usuario.step_up_requerido).
type SegundoFactorRequerido struct {
	IDUsuario  string
	Motivo     string
	ocurridoEn time.Time
}

// NuevoSegundoFactorRequerido construye el evento SegundoFactorRequerido.
func NuevoSegundoFactorRequerido(id IDUsuario, motivo string, ocurridoEn time.Time) SegundoFactorRequerido {
	return SegundoFactorRequerido{IDUsuario: id.String(), Motivo: motivo, ocurridoEn: ocurridoEn}
}

func (e SegundoFactorRequerido) NombreEvento() string  { return "SegundoFactorRequerido" }
func (e SegundoFactorRequerido) OcurridoEn() time.Time { return e.ocurridoEn }
func (e SegundoFactorRequerido) IDAgregado() string    { return e.IDUsuario }

// ContrasenaCambiada se emite cuando el usuario cambia su contraseña por
// decisión propia, con la contraseña actual ya verificada (accion de
// auditoría: usuario.contrasena_cambiada).
type ContrasenaCambiada struct {
	IDUsuario  string
	ocurridoEn time.Time
}

// NuevoContrasenaCambiada construye el evento ContrasenaCambiada.
func NuevoContrasenaCambiada(id IDUsuario, ocurridoEn time.Time) ContrasenaCambiada {
	return ContrasenaCambiada{IDUsuario: id.String(), ocurridoEn: ocurridoEn}
}

func (e ContrasenaCambiada) NombreEvento() string  { return "ContrasenaCambiada" }
func (e ContrasenaCambiada) OcurridoEn() time.Time { return e.ocurridoEn }
func (e ContrasenaCambiada) IDAgregado() string    { return e.IDUsuario }

// CredencialRehasheada se emite cuando el rehash oportunista (INV-ID-13)
// reemplaza el hash de un usuario tras un login exitoso, porque el
// adaptador de hashing indicó que el hash vigente ya no cumple los
// parámetros actuales (accion de auditoría: usuario.credencial_rehasheada).
type CredencialRehasheada struct {
	IDUsuario  string
	ocurridoEn time.Time
}

// NuevoCredencialRehasheada construye el evento CredencialRehasheada.
func NuevoCredencialRehasheada(id IDUsuario, ocurridoEn time.Time) CredencialRehasheada {
	return CredencialRehasheada{IDUsuario: id.String(), ocurridoEn: ocurridoEn}
}

func (e CredencialRehasheada) NombreEvento() string  { return "CredencialRehasheada" }
func (e CredencialRehasheada) OcurridoEn() time.Time { return e.ocurridoEn }
func (e CredencialRehasheada) IDAgregado() string    { return e.IDUsuario }

// CorreoVerificado se emite cuando un Usuario en pendiente_verificacion
// transiciona a activo mediante Usuario.ConfirmarCorreo (accion de
// auditoría: usuario.correo_verificado).
type CorreoVerificado struct {
	IDUsuario         string
	CorreoNormalizado string
	ocurridoEn        time.Time
}

// NuevoCorreoVerificado construye el evento CorreoVerificado.
func NuevoCorreoVerificado(id IDUsuario, correo Correo, ocurridoEn time.Time) CorreoVerificado {
	return CorreoVerificado{IDUsuario: id.String(), CorreoNormalizado: correo.Normalizado(), ocurridoEn: ocurridoEn}
}

func (e CorreoVerificado) NombreEvento() string  { return "CorreoVerificado" }
func (e CorreoVerificado) OcurridoEn() time.Time { return e.ocurridoEn }
func (e CorreoVerificado) IDAgregado() string    { return e.IDUsuario }

// VerificacionCorreoFallida se emite cuando un intento de verificación de
// correo no prospera porque el token es inválido o expiró (sección 3.4 del
// diseño del contexto). IDUsuario puede ir vacío si el token no se pudo
// resolver a ningún usuario (accion de auditoría: usuario.correo_verificado,
// resultado fallo — misma acción de catálogo que cubre el éxito con
// CorreoVerificado, no una acción nueva).
type VerificacionCorreoFallida struct {
	IDUsuario  string
	Motivo     string
	ocurridoEn time.Time
}

// NuevoVerificacionCorreoFallida construye el evento VerificacionCorreoFallida.
func NuevoVerificacionCorreoFallida(idUsuario, motivo string, ocurridoEn time.Time) VerificacionCorreoFallida {
	return VerificacionCorreoFallida{IDUsuario: idUsuario, Motivo: motivo, ocurridoEn: ocurridoEn}
}

func (e VerificacionCorreoFallida) NombreEvento() string  { return "VerificacionCorreoFallida" }
func (e VerificacionCorreoFallida) OcurridoEn() time.Time { return e.ocurridoEn }
func (e VerificacionCorreoFallida) IDAgregado() string    { return e.IDUsuario }

// EstadoUsuarioCambiado se emite en las transiciones de EstadoUsuario que
// no tienen su propio evento dedicado: Suspender, Bloquear y Reactivar
// (accion de auditoría: usuario.estado_cambiado).
type EstadoUsuarioCambiado struct {
	IDUsuario      string
	EstadoAnterior string
	EstadoNuevo    string
	Motivo         string
	ocurridoEn     time.Time
}

// NuevoEstadoUsuarioCambiado construye el evento EstadoUsuarioCambiado.
func NuevoEstadoUsuarioCambiado(id IDUsuario, anterior, nuevo EstadoUsuario, motivo string, ocurridoEn time.Time) EstadoUsuarioCambiado {
	return EstadoUsuarioCambiado{
		IDUsuario:      id.String(),
		EstadoAnterior: anterior.String(),
		EstadoNuevo:    nuevo.String(),
		Motivo:         motivo,
		ocurridoEn:     ocurridoEn,
	}
}

func (e EstadoUsuarioCambiado) NombreEvento() string  { return "EstadoUsuarioCambiado" }
func (e EstadoUsuarioCambiado) OcurridoEn() time.Time { return e.ocurridoEn }
func (e EstadoUsuarioCambiado) IDAgregado() string    { return e.IDUsuario }

// UsuarioConsultado se emite cuando un tercero (no el propio usuario)
// consulta el perfil de un Usuario. Consultar el perfil propio no se
// audita (accion de auditoría: usuario.consultado).
type UsuarioConsultado struct {
	IDUsuario     string
	IDSolicitante string
	ocurridoEn    time.Time
}

// NuevoUsuarioConsultado construye el evento UsuarioConsultado.
func NuevoUsuarioConsultado(idUsuario, idSolicitante string, ocurridoEn time.Time) UsuarioConsultado {
	return UsuarioConsultado{IDUsuario: idUsuario, IDSolicitante: idSolicitante, ocurridoEn: ocurridoEn}
}

func (e UsuarioConsultado) NombreEvento() string  { return "UsuarioConsultado" }
func (e UsuarioConsultado) OcurridoEn() time.Time { return e.ocurridoEn }
func (e UsuarioConsultado) IDAgregado() string    { return e.IDUsuario }
