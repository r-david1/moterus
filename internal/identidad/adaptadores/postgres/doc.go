// Package postgres implementa los puertos de salida de Identidad que
// requieren persistencia: puertos.RepositorioUsuarios y
// puertos.UnidadDeTrabajo (envolviendo internal/plataforma/bd), además del
// puerto puertos.GeneradorIDs (envolviendo internal/plataforma/ids).
//
// La tabla usuarios no tiene tenant_id (ADR 0002: Identidad no tiene
// multi-tenancy), así que este adaptador NO ejecuta
// SET LOCAL app.current_tenant — esa defensa en profundidad de RLS aplica a
// los contextos con aislamiento por tenant (p. ej. Tenencia), no a
// Identidad. La plantilla original de este comentario asumía RLS por
// tenant en todos los adaptadores Postgres; se corrige aquí porque no
// aplica a este contexto.
package postgres
