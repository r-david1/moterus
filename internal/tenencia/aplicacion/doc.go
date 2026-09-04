// Package aplicacion contiene los casos de uso del bounded context
// Tenencia (§3 del diseño): un struct por caso de uso (o un pequeño grupo
// cohesivo de casos de uso que comparten dependencias, cuando el puerto de
// entrada los agrupa en una sola interfaz — GestorDeOrganizaciones,
// GestorDeMembresias, GestorDeInvitaciones), con sus puertos de salida
// inyectados por el constructor NuevoX. Nunca importa
// internal/*/adaptadores/*: solo dominio y puertos (propios, y —a través de
// los ACL declarados en puertos/salida.go— de Identidad y Confianza vía sus
// propios paquetes puertos, nunca sus adaptadores).
package aplicacion
