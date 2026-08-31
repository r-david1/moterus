package aplicacion

import "github.com/r-david1/moterus/internal/confianza/puertos"

// Solicitud transporta la entrada de EvaluarTrustSignalCasoDeUso.Evaluar.
// Alias de puertos.Solicitud (mismo criterio que identidad/aplicacion/
// comandos.go: se define en puertos/ para evitar un import circular, se
// realiasa aquí para que este paquete no tenga que calificar con
// "puertos.").
type Solicitud = puertos.Solicitud

// ResultadoIntento transporta la entrada de
// EvaluarTrustSignalCasoDeUso.RegistrarResultado.
type ResultadoIntento = puertos.ResultadoIntento
