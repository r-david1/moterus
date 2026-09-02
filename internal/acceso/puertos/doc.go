// Package puertos define los contratos de entrada (driving, en
// entrada.go) y salida (driven, en salida.go) del bounded context Acceso.
// Los tipos ComandoX/ConsultaX/ResultadoX/VistaX de entrada.go son el
// contrato de los puertos de entrada que acceso/aplicacion implementa; los
// tipos de apoyo de salida.go (CredencialesSujeto, SujetoAutenticado,
// EstadoSujeto, SolicitudEvaluacion, DecisionConfianza, ResultadoIntento,
// LlavePublica) son el contrato de cruce con Identidad y Confianza. El
// subpaquete mocks contiene test doubles escritos a mano de los puertos de
// salida. Ver docs/design/acceso-bounded-context.md, sección 2.
package puertos
