// Package porteronoop implementa confianza/puertos.PorteroDeSala en modo
// no-op para cuando REDIS_URL no está configurada — mismo criterio y mismo
// WARN de arranque que EvaluadorConfianzaNoOp (identidad/adaptadores/
// confianza y acceso/adaptadores/confianza): sin Redis no hay motor de
// colas de acceso virtual posible (INV-COLA-08: el camino caliente es
// exclusivamente Redis + CPU local, no tiene un modo "sin Redis" que
// degradar), así que la única postura coherente es "nunca hay sala
// vigente" — MiddlewareSalaDeEspera, montado en Acceso/Identidad/Tenencia
// (§12 del diseño colas-virtuales.md), deja pasar todo el tráfico sin
// costo, exactamente como si la extensión no existiera.
package porteronoop

import (
	"context"
	"log/slog"

	"github.com/r-david1/moterus/internal/confianza/dominio"
	"github.com/r-david1/moterus/internal/confianza/puertos"
)

// PorteroDeSala implementa puertos.PorteroDeSala. SalaVigentePara siempre
// responde "no hay sala vigente" (false): es la única operación que
// MiddlewareSalaDeEspera invoca en el camino feliz (sin sala abierta, sin
// E/S, §7.3 del diseño), así que este es el único método que de verdad
// importa en producción sin Redis. Ingresar/ConsultarTurno/Reclamar existen
// solo para satisfacer la interfaz — un middleware que ya vio
// SalaVigentePara=false nunca los invoca — y devuelven
// ErrEstadoDeColaNoDisponible si alguien los llama igual (p. ej. un
// endpoint propio de Confianza montado por error sin Redis).
type PorteroDeSala struct {
	log *slog.Logger
}

var _ puertos.PorteroDeSala = (*PorteroDeSala)(nil)

// NuevoPorteroDeSala construye el adaptador no-op y emite inmediatamente el
// WARN de arranque. log puede ser nil (usa slog.Default()).
func NuevoPorteroDeSala(log *slog.Logger) *PorteroDeSala {
	if log == nil {
		log = slog.Default()
	}
	log.Warn("confianza/adaptadores/porteronoop: PorteroDeSala en modo no-op — " +
		"sin colas de acceso virtual (nunca hay sala vigente) en acceso.iniciar_sesion, " +
		"identidad.registrar_usuario ni tenencia.aceptar_invitacion. NO USAR EN PRODUCCIÓN durante un evento con pico esperado. " +
		"Definir REDIS_URL para montar el motor real (docs/design/colas-virtuales.md).")
	return &PorteroDeSala{log: log}
}

// SalaVigentePara nunca encuentra una sala vigente.
func (p *PorteroDeSala) SalaVigentePara(_ context.Context, _ puertos.ConsultaSalaVigente) (puertos.VistaSalaVigente, bool) {
	return puertos.VistaSalaVigente{}, false
}

func (p *PorteroDeSala) Ingresar(_ context.Context, _ puertos.ComandoIngresarASala) (puertos.ResultadoTurno, error) {
	return puertos.ResultadoTurno{}, p.errNoDisponible()
}

func (p *PorteroDeSala) ConsultarTurno(_ context.Context, _ puertos.ConsultaTurno) (puertos.ResultadoTurno, error) {
	return puertos.ResultadoTurno{}, p.errNoDisponible()
}

func (p *PorteroDeSala) Reclamar(_ context.Context, _ puertos.ComandoReclamarTurno) (puertos.ResultadoTurno, error) {
	return puertos.ResultadoTurno{}, p.errNoDisponible()
}

func (p *PorteroDeSala) errNoDisponible() error {
	return &dominio.ErrEstadoDeColaNoDisponible{Motivo: "colas de acceso virtual deshabilitadas: REDIS_URL no configurada"}
}
