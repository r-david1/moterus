// Package aplicacion contiene los casos de uso de Identidad
// (RegistrarUsuario, AutenticarUsuario, ObtenerUsuario) y sus comandos,
// consultas y resultados. Importa dominio y puertos; nunca adaptadores
// (INV-ID-19). El subpaquete noop contiene implementaciones stub de
// EvaluadorConfianza y RegistroAuditoria mientras los contextos Confianza y
// Auditoría no existen (sección 8, paso 6 del diseño).
package aplicacion
