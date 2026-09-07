// Package aplicacion contiene los casos de uso del bounded context
// Confianza:
//
//   - EvaluarTrustSignalCasoDeUso: rate limiting por IP y por cuenta (vía
//     puertos.LimitadorTasa) y verificación de captcha invisible (vía
//     puertos.VerificadorCaptcha) para producir una dominio.Decision. Ver
//     ADR 0018.
//   - Colas de acceso virtual (docs/design/colas-virtuales.md, §3): un
//     struct por caso de uso de administración (AbrirSalaCasoDeUso,
//     CambiarRitmoDeAdmisionCasoDeUso, CambiarEstadoSalaCasoDeUso —
//     juntos satisfacen puertos.GestorDeSalasDeEspera, compuestos por un
//     adaptador en cmd/api/main.go), PorteroDeSalaCasoDeUso (el puerto de
//     camino caliente, puertos.PorteroDeSala, que nunca depende de
//     puertos.RepositorioSalasDeEspera — INV-COLA-08) y
//     ReconciliarSalasCasoDeUso (el servicio de aplicación de §3.7 que
//     sostiene esa invariante).
//
// Nunca importa internal/confianza/adaptadores/* ni el dominio de otro
// contexto: solo dominio y puertos propios.
package aplicacion
