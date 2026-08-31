package eventos

import (
	"context"
	"log/slog"

	"github.com/r-david1/moterus/internal/identidad/dominio"
	"github.com/r-david1/moterus/internal/identidad/puertos"
)

// PublicadorLog implementa puertos.PublicadorEventos registrando cada
// evento en el logger estructurado, sin entregarlo a ningún message broker.
//
// Limitación documentada explícitamente (pedida por el encargo): el stack
// fijo del proyecto no define todavía una cola/broker para integración
// asíncrona entre bounded contexts (no hay Kafka ni NATS en el stack; Redis
// está reservado para cache/rate-limit/colas virtuales del contexto
// Confianza — agente colas-virtuales, fuera de alcance aquí). Mientras no
// exista ese mecanismo, ningún suscriptor real (p. ej. "enviar correo de
// verificación tras UsuarioRegistrado") puede engancharse a este puerto: los
// eventos publicados aquí solo quedan en el log de la aplicación. Esto es
// aceptable porque PublicadorEventos es best-effort y se invoca fuera de la
// UnidadDeTrabajo (nunca bloquea ni aborta un caso de uso), pero es una
// brecha funcional real que debe resolverse antes de que Identidad dependa
// de un flujo asíncrono para algo que importe (p. ej. verificación de
// correo).
type PublicadorLog struct {
	log *slog.Logger
}

var _ puertos.PublicadorEventos = (*PublicadorLog)(nil)

// NuevoPublicadorLog construye el adaptador. log puede ser nil, en cuyo
// caso se usa slog.Default().
func NuevoPublicadorLog(log *slog.Logger) *PublicadorLog {
	if log == nil {
		log = slog.Default()
	}
	return &PublicadorLog{log: log}
}

// Publicar registra cada evento (nombre, agregado, instante) en el logger.
// Nunca serializa el evento completo: algunos eventos futuros de fase 2
// podrían llevar campos que no deben terminar en logs (p. ej. si algún día
// se añaden metadatos de MFA); solo se registran los campos ya expuestos
// por la interfaz EventoDominio, que el propio dominio garantiza libres de
// secretos (INV-ID-04).
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
