package eventos

import (
	"context"
	"log/slog"

	"github.com/r-david1/moterus/internal/acceso/dominio"
	"github.com/r-david1/moterus/internal/acceso/puertos"
)

// PublicadorLog implementa puertos.PublicadorEventos registrando cada
// evento en el logger estructurado, sin entregarlo a ningún message
// broker. Mismo criterio y misma limitación documentada que
// identidad/adaptadores/eventos.PublicadorLog: el stack fijo del proyecto
// no define todavía una cola/broker para integración asíncrona entre
// bounded contexts. Es aceptable porque PublicadorEventos es best-effort y
// se invoca fuera de la UnidadDeTrabajo (nunca bloquea ni aborta un caso
// de uso de Acceso).
type PublicadorLog struct {
	log *slog.Logger
}

var _ puertos.PublicadorEventos = (*PublicadorLog)(nil)

// NuevoPublicadorLog construye el adaptador. log puede ser nil (usa
// slog.Default()).
func NuevoPublicadorLog(log *slog.Logger) *PublicadorLog {
	if log == nil {
		log = slog.Default()
	}
	return &PublicadorLog{log: log}
}

// Publicar registra cada evento (nombre, agregado, instante) en el logger.
// Nunca serializa el evento completo: solo los campos ya expuestos por la
// interfaz EventoDominio, que el propio dominio garantiza libres de
// secretos (INV-ACC-11).
func (p *PublicadorLog) Publicar(ctx context.Context, eventosDominio ...dominio.EventoDominio) error {
	for _, e := range eventosDominio {
		p.log.InfoContext(ctx, "evento de dominio publicado (solo log; sin broker configurado)",
			"evento", e.NombreEvento(),
			"agregado_id", e.IDAgregado(),
			"ocurrido_en", e.OcurridoEn(),
		)
	}
	return nil
}
