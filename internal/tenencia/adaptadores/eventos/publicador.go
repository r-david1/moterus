// Package eventos implementa puertos.PublicadorEventos registrando cada
// evento en el logger estructurado, sin entregarlo a ningún message
// broker. Mismo criterio y misma limitación documentada que
// identidad/adaptadores/eventos.PublicadorLog y
// acceso/adaptadores/eventos.PublicadorLog.
package eventos

import (
	"context"
	"log/slog"

	"github.com/r-david1/moterus/internal/tenencia/dominio"
	"github.com/r-david1/moterus/internal/tenencia/puertos"
)

// PublicadorLog implementa puertos.PublicadorEventos.
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
// secretos (INV-TEN-23).
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
