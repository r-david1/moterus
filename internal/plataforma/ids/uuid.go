package ids

import "github.com/google/uuid"

// GenerarUUIDv7 produce un UUID versión 7 (ordenable en el tiempo, mejor
// localidad de índice B-tree que UUIDv4 — ADR candidato 0012 del diseño de
// Identidad). Devuelve el valor en su representación canónica en minúsculas.
//
// Vive en plataforma/ids porque es un generador puramente técnico, sin
// conocimiento de qué agregado de qué bounded context lo va a usar: cada
// contexto envuelve esta función en su propio adaptador para implementar su
// puerto GeneradorIDs (p. ej. identidad/adaptadores/postgres.GeneradorIDs),
// que sí construye el value object de dominio correspondiente.
func GenerarUUIDv7() (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	return id.String(), nil
}
