package aplicacion

import "github.com/r-david1/moterus/internal/acceso/puertos"

// Los tipos ComandoX/ConsultaX/ResultadoX/VistaX que transportan la
// entrada y salida de los casos de uso se definen en puertos/entrada.go,
// porque son el contrato de los puertos de entrada (IniciadorDeSesion,
// RenovadorDeSesion, CerradorDeSesiones, ValidadorDeAccesos,
// ConsultorDeSesiones, RevocadorDeSesiones) y este paquete los implementa:
// definirlos aquí en lugar de allí crearía un import circular (aplicacion
// -> puertos -> aplicacion). Este archivo los re-expone como alias de
// tipo para que el resto de aplicacion/ los use sin calificar con
// "puertos.", igual que identidad/aplicacion/comandos.go. Al ser alias
// (=), son exactamente el mismo tipo: no hay conversión ni pérdida de
// identidad para satisfacer las interfaces de puertos.

// ComandoIniciarSesion transporta la entrada del caso de uso IniciarSesion.
type ComandoIniciarSesion = puertos.ComandoIniciarSesion

// ComandoRenovarSesion transporta la entrada del caso de uso RenovarSesion.
type ComandoRenovarSesion = puertos.ComandoRenovarSesion

// ComandoCerrarSesion transporta la entrada de Cerrar.
type ComandoCerrarSesion = puertos.ComandoCerrarSesion

// ComandoCerrarTodasLasSesiones transporta la entrada de CerrarTodas.
type ComandoCerrarTodasLasSesiones = puertos.ComandoCerrarTodasLasSesiones

// ComandoRevocarSesionesDeUsuario transporta la entrada de
// RevocarPorUsuario.
type ComandoRevocarSesionesDeUsuario = puertos.ComandoRevocarSesionesDeUsuario

// ComandoValidarAcceso transporta la entrada del caso de uso ValidarAcceso.
type ComandoValidarAcceso = puertos.ComandoValidarAcceso

// ResultadoSesion es la salida común de IniciarSesion y RenovarSesion.
type ResultadoSesion = puertos.ResultadoSesion

// Acceso es el sujeto autenticado que devuelve ValidarAcceso.
type Acceso = puertos.Acceso

// ConsultaSesionesDeUsuario transporta la entrada del caso de uso
// ListarSesiones.
type ConsultaSesionesDeUsuario = puertos.ConsultaSesionesDeUsuario

// VistaSesion es el modelo de lectura devuelto por ListarSesiones.
type VistaSesion = puertos.VistaSesion

// ResultadoCierreMasivo es la salida de CerrarTodas y de RevocarPorUsuario.
type ResultadoCierreMasivo = puertos.ResultadoCierreMasivo
