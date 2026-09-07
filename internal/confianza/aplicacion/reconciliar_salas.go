package aplicacion

import (
	"context"
	"log/slog"

	"github.com/r-david1/moterus/internal/confianza/puertos"
)

// ReconciliarSalasCasoDeUso implementa el servicio de aplicación de §3.7 del
// diseño colas-virtuales.md: el que sostiene INV-COLA-08 (el camino caliente
// nunca toca Postgres, porque esta es la única pieza de aplicacion que
// consulta RepositorioSalasDeEspera fuera de la administración) y §8 (el
// modoDegradado se decide desde una instantánea en memoria, no desde Redis).
// No es un adaptador: coordina RepositorioSalasDeEspera y EstadoDeCola, y no
// conoce ni pgx ni go-redis.
//
// Es un servicio de aplicación, no "un caso de uso" en el sentido de una
// transacción de negocio disparada por un sujeto: Reconciliar ejecuta UN
// ciclo (ListarVigentes + actualizar instantánea + reproyectar). El bucle
// con time.Ticker y su cierre ordenado los arma cmd/api/main.go (§3.7 y §12
// del diseño): eso es wiring de infraestructura de proceso, fuera de este
// paquete.
type ReconciliarSalasCasoDeUso struct {
	salas       puertos.RepositorioSalasDeEspera
	estadoCola  puertos.EstadoDeCola
	reloj       puertos.Reloj
	instantanea *InstantaneaSalasVigentes
}

// NuevoReconciliarSalasCasoDeUso construye el servicio con sus dependencias
// inyectadas por puerto. instantanea se comparte con
// PorteroDeSalaCasoDeUso (ver el comentario de InstantaneaSalasVigentes en
// portero_sala.go): este servicio es quien la escribe.
func NuevoReconciliarSalasCasoDeUso(
	salas puertos.RepositorioSalasDeEspera,
	estadoCola puertos.EstadoDeCola,
	reloj puertos.Reloj,
	instantanea *InstantaneaSalasVigentes,
) *ReconciliarSalasCasoDeUso {
	return &ReconciliarSalasCasoDeUso{
		salas:       salas,
		estadoCola:  estadoCola,
		reloj:       reloj,
		instantanea: instantanea,
	}
}

// Reconciliar ejecuta un ciclo (§3.7 del diseño, pasos 1-3):
//
//  1. RepositorioSalasDeEspera.ListarVigentes() — la única consulta a
//     Postgres de todo el ciclo.
//  2. Reemplaza la instantánea en memoria por completo, ANTES de reproyectar
//     a Redis: así, incluso si la proyección de alguna sala falla más
//     abajo, la instantánea (y con ella SalaVigentePara/§8) ya refleja la
//     lista vigente más reciente conocida por Postgres.
//  3. Para cada sala vigente, EstadoDeCola.Proyectar. Antes de proyectar,
//     EstadoDeCola.Instantanea sondea si Redis todavía tiene el estado de
//     esa sala: si no responde (Redis vació o perdió la clave de
//     configuración, p. ej. tras un FLUSHALL a mitad de un evento), este
//     ciclo reinicia el reloj de admisión de la reproyección
//     (cursorBase=0, relojDesde=ahora) en vez de reinstalar el ancla
//     verdadera que sigue en Postgres (INV-COLA-13): si se reinstalara tal
//     cual, el contador `seq` de Redis —perdido junto con el resto del
//     estado— volvería a arrancar en 1 mientras el cursor derivado seguiría
//     en un valor grande, admitiendo de golpe a cualquiera que reingrese.
//     Cuando Redis SÍ tiene el estado, se reproyecta con el ancla real del
//     agregado (continuidad exacta, sin reinicio).
//
// Devuelve el primer error de reproyección encontrado (tras intentar
// reproyectar todas las salas vigentes, registrando cada fallo por slog):
// una sala que no se pudo reproyectar sigue protegida por el valor previo
// que ya tenía en Redis (o, en el peor caso, por el modoDegradado de la
// instantánea, §8), así que un solo fallo no debe abortar el ciclo entero.
func (c *ReconciliarSalasCasoDeUso) Reconciliar(ctx context.Context) error {
	vigentes, err := c.salas.ListarVigentes(ctx)
	if err != nil {
		return err
	}

	c.instantanea.reemplazar(vigentes)

	ahora := c.reloj.Ahora()
	var primerError error
	for _, sala := range vigentes {
		if sala == nil {
			continue
		}
		clave := sala.Clave().String()
		cursorBase, relojDesde := sala.CursorBase(), sala.RelojDesde()

		if _, err := c.estadoCola.Instantanea(ctx, clave); err != nil {
			slog.WarnContext(ctx, "confianza: el reconciliador no encontró el estado de una sala vigente en Redis; reiniciando el reloj de admisión de la reproyección (INV-COLA-13)",
				"clave", clave, "error", err)
			cursorBase, relojDesde = 0, ahora
		}

		proy := puertos.ProyeccionSala{
			Clave:               clave,
			Alias:               sala.Alias().Normalizado(),
			Estado:              sala.Estado().String(),
			RitmoAdmision:       sala.Politica().RitmoAdmision().PorSegundo(),
			CapacidadMaximaCola: sala.Politica().CapacidadMaximaCola(),
			VentanaReclamo:      sala.Politica().VentanaReclamo(),
			CursorBase:          cursorBase,
			RelojDesde:          relojDesde,
			Version:             sala.RelojDesde().UnixNano(),
		}
		if err := c.estadoCola.Proyectar(ctx, proy); err != nil {
			slog.ErrorContext(ctx, "confianza: el reconciliador no pudo reproyectar una sala vigente",
				"clave", clave, "error", err)
			if primerError == nil {
				primerError = err
			}
		}
	}
	return primerError
}
