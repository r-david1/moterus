package puertos

import (
	"context"
	"time"

	"github.com/r-david1/moterus/internal/acceso/dominio"
)

// Nota de ubicación: los tipos ComandoX/ConsultaX/ResultadoX/VistaX se
// definen aquí, junto a las interfaces de entrada que los usan como firma,
// por la misma razón que en identidad/puertos/entrada.go: son el contrato
// del puerto (sección 2.1 del diseño) y aplicacion, que implementa estas
// interfaces, no puede definirlos ella misma sin crear un import circular.
// aplicacion/comandos.go re-expone estos tipos como alias para que el
// código de los casos de uso los use sin calificar con "puertos." (sección
// 5 del diseño, sin violar INV-ACC-19).
//
// Todos los campos son primitivos salvo Origen dominio.OrigenSolicitud: no
// es un dato de negocio a validar, sino el contexto forense que el
// middleware/handler ya construyó.

// ComandoIniciarSesion transporta la entrada del caso de uso IniciarSesion
// (sección 3.1 del diseño). Contrasena se envuelve y se reenvía tal cual a
// Identidad: Acceso nunca la inspecciona ni la hashea (INV-ACC-02).
type ComandoIniciarSesion struct {
	Correo       string
	Contrasena   string
	Origen       dominio.OrigenSolicitud
	TokenCaptcha string
}

// ComandoRenovarSesion transporta la entrada del caso de uso RenovarSesion
// (sección 3.2 del diseño).
type ComandoRenovarSesion struct {
	TokenRefresco string
	Origen        dominio.OrigenSolicitud
}

// ComandoCerrarSesion transporta la entrada de CerradorDeSesiones.Cerrar
// (sección 3.4 del diseño). IDSesion e IDUsuario los resuelve siempre el
// adaptador HTTP a partir del Acceso ya validado (el sujeto del token y,
// para el logout individual sin id explícito, su propia sesión) o de un
// parámetro de ruta — nunca de un campo de cuerpo sin validar (INV-ACC-23).
type ComandoCerrarSesion struct {
	// IDSesion es el identificador de la sesión a cerrar. Lo resuelve el
	// adaptador: para DELETE /acceso/sesiones/actual es el sid del propio
	// token validado; para DELETE /acceso/sesiones/{id} es el parámetro de
	// ruta. Nunca queda vacío al llegar a este caso de uso: si llega
	// vacío, dominio.IDSesionDesde lo rechaza igual que cualquier otro
	// formato inválido.
	IDSesion  string
	IDUsuario string // SIEMPRE del token validado, nunca del cuerpo (INV-ACC-23)
	Origen    dominio.OrigenSolicitud
}

// ComandoCerrarTodasLasSesiones transporta la entrada de
// CerradorDeSesiones.CerrarTodas (sección 3.4 del diseño).
type ComandoCerrarTodasLasSesiones struct {
	IDUsuario          string
	IDSesionAPreservar string // opcional: "cerrar las demás, no la mía"
	Origen             dominio.OrigenSolicitud
}

// ComandoRevocarSesionesDeUsuario transporta la entrada de
// RevocadorDeSesiones.RevocarPorUsuario (sección 3.6 del diseño). Motivo
// debe pertenecer al catálogo cerrado de dominio.MotivoRevocacion.
type ComandoRevocarSesionesDeUsuario struct {
	IDUsuario string
	Motivo    string
	Origen    dominio.OrigenSolicitud
}

// ComandoValidarAcceso transporta la entrada de ValidadorDeAccesos.Validar
// (sección 3.3 del diseño): el camino caliente del sistema.
type ComandoValidarAcceso struct {
	TokenCompacto string
	// ExigirSesionViva fuerza una verificación contra el repositorio de
	// sesiones además de la firma. false en el camino normal (cero
	// consultas a Postgres); true para operaciones de alto valor, donde una
	// ventana de revocación de hasta 10 minutos no es aceptable.
	ExigirSesionViva bool
	Origen           dominio.OrigenSolicitud
}

// ResultadoSesion es la salida común de IniciarSesion y RenovarSesion.
type ResultadoSesion struct {
	TokenAcceso      string // JWT compacto
	ExpiraEnSegundos int    // vida del token de acceso
	TipoToken        string // "Bearer"
	TokenRefresco    string // valor en claro; el caller lo entrega UNA vez y lo olvida
	RefrescoExpiraEn time.Time
	IDSesion         string
	IDUsuario        string
	SesionExpiraEn   time.Time // vida absoluta, para que el cliente sepa cuándo tendrá que reautenticarse
}

// Acceso es el sujeto autenticado que el middleware inyecta en el
// contexto. Es deliberadamente pobre: no lleva correo, ni roles, ni
// organización (INV-ACC-12).
type Acceso struct {
	IDUsuario            string
	IDSesion             string
	MetodosAutenticacion []string  // amr: ["pwd"], luego ["pwd","otp"]
	AutenticadoEn        time.Time // auth_time: para políticas de reautenticación
	TokenExpiraEn        time.Time
}

// ConsultaSesionesDeUsuario transporta la entrada de
// ConsultorDeSesiones.ListarDeUsuario (sección 3.5 del diseño).
type ConsultaSesionesDeUsuario struct {
	IDUsuario      string
	IDSesionActual string // para marcar cuál es "esta"
	Origen         dominio.OrigenSolicitud
}

// VistaSesion es un modelo de LECTURA: nunca expone hashes de refresco ni
// la cadena de rotación. Mismo criterio estructural que VistaUsuario en
// Identidad.
type VistaSesion struct {
	ID                 string
	EsSesionActual     bool
	Estado             string
	CreadaEn           time.Time
	UltimaRenovacionEn *time.Time
	ExpiraAbsolutoEn   time.Time
	IPOrigen           string // la de creación; útil para "cerrar la sesión de Madrid"
	AgenteUsuario      string
}

// ResultadoCierreMasivo es la salida de CerradorDeSesiones.CerrarTodas y de
// RevocadorDeSesiones.RevocarPorUsuario.
type ResultadoCierreMasivo struct{ SesionesRevocadas int }

// IniciadorDeSesion es el puerto de entrada que implementa el caso de uso
// IniciarSesion (sección 3.1 del diseño).
type IniciadorDeSesion interface {
	Iniciar(ctx context.Context, cmd ComandoIniciarSesion) (ResultadoSesion, error)
}

// RenovadorDeSesion es el puerto de entrada que implementa el caso de uso
// RenovarSesion (sección 3.2 del diseño).
type RenovadorDeSesion interface {
	Renovar(ctx context.Context, cmd ComandoRenovarSesion) (ResultadoSesion, error)
}

// CerradorDeSesiones es el puerto de entrada que implementa el caso de uso
// CerrarSesion/CerrarTodasLasSesiones (sección 3.4 del diseño).
type CerradorDeSesiones interface {
	Cerrar(ctx context.Context, cmd ComandoCerrarSesion) error
	CerrarTodas(ctx context.Context, cmd ComandoCerrarTodasLasSesiones) (ResultadoCierreMasivo, error)
}

// ValidadorDeAccesos es el puerto que consume el middleware HTTP de
// CUALQUIER contexto para autenticar una petición (sección 3.3 del
// diseño). Es el contrato más caliente del sistema: se invoca una vez por
// request autenticada.
type ValidadorDeAccesos interface {
	Validar(ctx context.Context, cmd ComandoValidarAcceso) (Acceso, error)
}

// ConsultorDeSesiones es el puerto de entrada que implementa el caso de
// uso ListarSesiones (sección 3.5 del diseño).
type ConsultorDeSesiones interface {
	ListarDeUsuario(ctx context.Context, q ConsultaSesionesDeUsuario) ([]VistaSesion, error)
}

// RevocadorDeSesiones es el puerto que Acceso EXPONE a otros contextos
// (hoy: nadie; mañana: Identidad al suspender una cuenta o cambiar la
// contraseña — ADR candidato 0022). Implementa el caso de uso
// RevocarSesionesDeUsuario (sección 3.6 del diseño): se declara desde
// ahora para que el día que Identidad lo necesite no se invente un acceso
// directo a la tabla sesiones.
type RevocadorDeSesiones interface {
	RevocarPorUsuario(ctx context.Context, cmd ComandoRevocarSesionesDeUsuario) (ResultadoCierreMasivo, error)
}
