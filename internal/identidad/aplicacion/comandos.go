package aplicacion

import "github.com/r-david1/moterus/internal/identidad/puertos"

// Los tipos ComandoX/ConsultaX/ResultadoX/VistaX que transportan la entrada
// y salida de los casos de uso se definen en puertos/entrada.go, porque son
// el contrato de los puertos de entrada (RegistradorDeUsuarios,
// AutenticadorDeCredenciales, ConsultorDeUsuarios) y este paquete los
// implementa: definirlos aquí en lugar de allí crearía un import circular
// (aplicacion -> puertos -> aplicacion). Este archivo los re-expone como
// alias de tipo para que el resto de aplicacion/ los use sin calificar con
// "puertos.", tal como pide la sección 5 del diseño (comandos.go vive en
// aplicacion/). Al ser alias (=), son exactamente el mismo tipo: no hay
// conversión ni pérdida de identidad para satisfacer las interfaces de
// puertos.

// ComandoRegistrarUsuario transporta la entrada del caso de uso
// RegistrarUsuario. Solo primitivos salvo Origen (contexto forense, no dato
// de negocio a validar; ADR candidato 0016).
type ComandoRegistrarUsuario = puertos.ComandoRegistrarUsuario

// ResultadoRegistro es la salida del caso de uso RegistrarUsuario.
type ResultadoRegistro = puertos.ResultadoRegistro

// ComandoAutenticar transporta la entrada del caso de uso AutenticarUsuario.
type ComandoAutenticar = puertos.ComandoAutenticar

// ResultadoAutenticacion es la salida del caso de uso AutenticarUsuario.
// Nunca lleva tokens ni sesión (INV-ID-14).
type ResultadoAutenticacion = puertos.ResultadoAutenticacion

// ConsultaUsuarioPorID transporta la entrada del caso de uso ObtenerUsuario.
type ConsultaUsuarioPorID = puertos.ConsultaUsuarioPorID

// VistaUsuario es el modelo de lectura devuelto por ObtenerUsuario.
type VistaUsuario = puertos.VistaUsuario

// ComandoVerificarCorreo transporta la entrada del caso de uso
// VerificarCorreo.
type ComandoVerificarCorreo = puertos.ComandoVerificarCorreo

// ComandoReenviarVerificacion transporta la entrada del caso de uso
// ReenviarVerificacion.
type ComandoReenviarVerificacion = puertos.ComandoReenviarVerificacion
