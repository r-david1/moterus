package puertos

import (
	"context"
	"time"

	"github.com/r-david1/moterus/internal/identidad/dominio"
)

// Nota de ubicación: los tipos ComandoX/ConsultaX/ResultadoX/VistaX se
// definen aquí, junto a las interfaces de entrada que los usan como firma,
// porque son el contrato del puerto (sección 2.1 del diseño). La capa
// aplicacion, que implementa estas interfaces, no puede definirlos ella
// misma sin crear un import circular (aplicacion ya importa puertos para
// implementar RegistradorDeUsuarios/AutenticadorDeCredenciales/
// ConsultorDeUsuarios). aplicacion/comandos.go re-expone estos tipos como
// alias para que el código de los casos de uso los use sin calificar con
// "puertos." (sección 3 y sección 5 del diseño, sin violar INV-ID-19).
//
// Todos los campos son primitivos salvo Origen dominio.OrigenSolicitud: no
// es un dato de negocio a validar (correo, contraseña) sino el contexto
// forense que el middleware/handler ya construyó (ADR candidato 0016 no
// aplica a este campo por diseño explícito de la sección 3).

// ComandoRegistrarUsuario transporta la entrada del caso de uso
// RegistrarUsuario.
type ComandoRegistrarUsuario struct {
	Correo       string
	Contrasena   string // se envuelve en dominio.ContrasenaPlana de inmediato
	Origen       dominio.OrigenSolicitud
	TokenCaptcha string
}

// ResultadoRegistro es la salida del caso de uso RegistrarUsuario.
type ResultadoRegistro struct {
	IDUsuario                  string
	Estado                     string // "pendiente_verificacion"
	RequiereVerificacionCorreo bool
}

// ComandoAutenticar transporta la entrada del caso de uso AutenticarUsuario.
type ComandoAutenticar struct {
	Correo       string
	Contrasena   string
	Origen       dominio.OrigenSolicitud
	TokenCaptcha string
}

// ResultadoAutenticacion es la salida del caso de uso AutenticarUsuario. No
// transporta tokens ni sesión (INV-ID-14): eso es responsabilidad del
// contexto Acceso.
type ResultadoAutenticacion struct {
	IDUsuario             string
	CorreoNormalizado     string
	Estado                string
	RequiereSegundoFactor bool
	MotivoStepUp          string // "mfa_habilitado" | "confianza_baja" | ""
	PuntajeConfianza      float64
}

// ConsultaUsuarioPorID transporta la entrada del caso de uso ObtenerUsuario.
type ConsultaUsuarioPorID struct {
	IDUsuario     string
	IDSolicitante string // quién pregunta; vacío = llamada interna del sistema
	Origen        dominio.OrigenSolicitud
}

// VistaUsuario es el modelo de lectura devuelto por ObtenerUsuario. Nunca es
// el agregado dominio.Usuario: así el filtrado de campos sensibles (p. ej.
// el hash de contraseña) es estructural, no disciplinario.
type VistaUsuario struct {
	ID             string
	Correo         string
	Estado         string
	TieneMFA       bool
	CreadoEn       time.Time
	UltimoAccesoEn *time.Time
}

// RegistradorDeUsuarios es el puerto de entrada que implementa el caso de
// uso RegistrarUsuario (sección 3.1 del diseño).
type RegistradorDeUsuarios interface {
	Registrar(ctx context.Context, cmd ComandoRegistrarUsuario) (ResultadoRegistro, error)
}

// AutenticadorDeCredenciales es el puerto de entrada que implementa el caso
// de uso AutenticarUsuario (sección 3.2 del diseño). Es exactamente el
// contrato que el contexto Acceso consumirá para su orquestación
// IniciarSesion: si no existiera esta interfaz, Acceso terminaría
// importando el struct concreto de identidad/aplicacion y el acoplamiento
// entre contextos dejaría de ser inspeccionable.
type AutenticadorDeCredenciales interface {
	Autenticar(ctx context.Context, cmd ComandoAutenticar) (ResultadoAutenticacion, error)
}

// ConsultorDeUsuarios es el puerto de entrada que implementa el caso de uso
// ObtenerUsuario (sección 3.3 del diseño).
type ConsultorDeUsuarios interface {
	ObtenerPorID(ctx context.Context, q ConsultaUsuarioPorID) (VistaUsuario, error)
}

// ComandoVerificarCorreo transporta la entrada del caso de uso
// VerificarCorreo (sección 3.4 del diseño).
type ComandoVerificarCorreo struct {
	TokenPlano string
	Origen     dominio.OrigenSolicitud
}

// ComandoReenviarVerificacion transporta la entrada del caso de uso
// ReenviarVerificacion (sección 3.4 del diseño).
type ComandoReenviarVerificacion struct {
	Correo string
	Origen dominio.OrigenSolicitud
}

// VerificadorDeCorreo es el puerto de entrada que implementa el caso de uso
// VerificarCorreo (sección 3.4 del diseño). No hay ningún estado de negocio
// que reportar más allá de éxito/fallo, así que el resultado es solo el
// error: éxito es nil, y cada fallo es un error de dominio tipado
// (ErrTokenVerificacionInvalido / ErrTokenVerificacionExpirado) que el
// adaptador HTTP mapea a su status correspondiente.
type VerificadorDeCorreo interface {
	Verificar(ctx context.Context, cmd ComandoVerificarCorreo) error
}

// ReenviadorDeVerificacion es el puerto de entrada que implementa el caso
// de uso ReenviarVerificacion (sección 3.4 del diseño). Devuelve siempre
// nil (INV-ID-22): la respuesta es deliberadamente neutra, así que ni
// siquiera el tipo de retorno puede distinguir si el correo existía, si ya
// estaba verificado o si hubo un fallo interno al reenviar.
type ReenviadorDeVerificacion interface {
	Reenviar(ctx context.Context, cmd ComandoReenviarVerificacion) error
}
