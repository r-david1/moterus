package aplicacion

import (
	"context"

	"github.com/r-david1/moterus/internal/confianza/dominio"
	"github.com/r-david1/moterus/internal/confianza/puertos"
)

// AbrirSalaCasoDeUso implementa puertos.GestorDeSalasDeEspera.Abrir (§3.1
// del diseño colas-virtuales.md). Es el único caso de uso de administración
// que crea el agregado: los otros dos (CambiarRitmoDeAdmisionCasoDeUso,
// CambiarEstadoSalaCasoDeUso) operan sobre una sala ya existente. Los tres
// son casos de uso separados (una transacción de negocio cada uno), aunque
// juntos satisfacen puertos.GestorDeSalasDeEspera: mismo criterio de
// composición que identidad/aplicacion.HabilitarMFACasoDeUso/
// ConfirmarFactorMFACasoDeUso/DeshabilitarMFACasoDeUso frente a
// puertos.GestorDeMFA — el adaptador que los compone en una sola interfaz de
// puerto vive en cmd/api/main.go (§12 del diseño, fuera de este paquete).
type AbrirSalaCasoDeUso struct {
	salas      puertos.RepositorioSalasDeEspera
	estadoCola puertos.EstadoDeCola
	auditoria  puertos.RegistroAuditoria
	reloj      puertos.Reloj
	ids        puertos.GeneradorIDs
	uow        puertos.UnidadDeTrabajo
}

// NuevoAbrirSalaCasoDeUso construye el caso de uso con sus dependencias
// inyectadas por puerto.
func NuevoAbrirSalaCasoDeUso(
	salas puertos.RepositorioSalasDeEspera,
	estadoCola puertos.EstadoDeCola,
	auditoria puertos.RegistroAuditoria,
	reloj puertos.Reloj,
	ids puertos.GeneradorIDs,
	uow puertos.UnidadDeTrabajo,
) *AbrirSalaCasoDeUso {
	return &AbrirSalaCasoDeUso{
		salas:      salas,
		estadoCola: estadoCola,
		auditoria:  auditoria,
		reloj:      reloj,
		ids:        ids,
		uow:        uow,
	}
}

// Abrir ejecuta el flujo normativo de §3.1 del diseño:
//
//  1. Construye los VOs (AliasSala, RutaProtegida, AlcanceSala,
//     PoliticaSala) — cualquier fallo es un error de dominio antes de tocar
//     nada (422 en el adaptador HTTP).
//  2. dominio.NuevaSalaDeEspera ya verifica que la ruta admite el alcance
//     (RutaProtegida.AdmiteAlcance) como parte de sus invariantes de
//     construcción: no hace falta repetir el chequeo aquí.
//  3. Si el alcance es organizacion, la autorización ya la resolvió el
//     middleware (§7.2 del diseño): este caso de uso confía en el IDSujeto
//     que recibe, mismo criterio que INV-TEN-12.
//  4. Persiste la sala recién construida en estado "programada": la
//     unicidad de (alcance, ruta) entre salas no cerradas la garantiza el
//     índice único parcial de §6.1 (INV-COLA-01), no una consulta previa.
//  5. Abrir(ahora) muta el agregado EN MEMORIA a "abierta".
//  6. Proyecta a Redis. Si la proyección falla, la apertura falla: como el
//     paso 4 ya persistió la sala en "programada" y este método no vuelve a
//     invocar Guardar hasta después de este punto, la fila persistida queda
//     "programada" — nunca "abierta" en Postgres sin estar reflejada en
//     Redis (§3.1 del diseño: "una sala 'abierta' en Postgres pero
//     invisible en Redis es una sala que no protege nada"; fail-closed en
//     la administración, que es el camino frío).
//  7. Solo si la proyección tuvo éxito: persiste la sala ya abierta y
//     audita SalaDeEsperaAbierta en la MISMA unidad de trabajo (INV-COLA-11).
func (c *AbrirSalaCasoDeUso) Abrir(ctx context.Context, cmd puertos.ComandoAbrirSala) (puertos.VistaSala, error) {
	alias, err := dominio.NuevoAliasSala(cmd.Alias)
	if err != nil {
		return puertos.VistaSala{}, err
	}
	ruta, err := dominio.RutaProtegidaDesde(cmd.Ruta)
	if err != nil {
		return puertos.VistaSala{}, err
	}
	alcance, err := alcanceSalaDesde(cmd.IDOrganizacion)
	if err != nil {
		return puertos.VistaSala{}, err
	}
	ritmo, err := dominio.NuevoRitmoAdmision(cmd.RitmoAdmision)
	if err != nil {
		return puertos.VistaSala{}, err
	}
	modo, err := dominio.ModoDegradadoDesde(cmd.ModoDegradado)
	if err != nil {
		return puertos.VistaSala{}, err
	}
	politica, err := dominio.NuevaPoliticaSala(ritmo, cmd.CapacidadMaximaCola, cmd.VentanaReclamo, modo)
	if err != nil {
		return puertos.VistaSala{}, err
	}
	creadaPor, err := creadaPorDesde(cmd.IDSujeto)
	if err != nil {
		return puertos.VistaSala{}, err
	}

	id, err := c.ids.NuevoIDSalaDeEspera()
	if err != nil {
		return puertos.VistaSala{}, err
	}
	ahora := c.reloj.Ahora()

	sala, err := dominio.NuevaSalaDeEspera(id, alias, alcance, ruta, politica, creadaPor, ahora)
	if err != nil {
		return puertos.VistaSala{}, err
	}

	// Paso 4: persistir en "programada" primero.
	if err := c.salas.Guardar(ctx, sala); err != nil {
		return puertos.VistaSala{}, err
	}

	// Paso 5: mutar en memoria a "abierta".
	if err := sala.Abrir(ahora); err != nil {
		return puertos.VistaSala{}, err
	}

	// Paso 6: proyectar a Redis. Fail-closed: si esto falla, no se vuelve a
	// llamar a Guardar y la fila persistida en el paso 4 queda "programada".
	if err := c.estadoCola.Proyectar(ctx, proyeccionDesde(sala)); err != nil {
		return puertos.VistaSala{}, err
	}

	// Paso 7: persistir la sala ya abierta y auditar en la misma UoW.
	eventos := sala.EventosPendientes()
	if err := c.uow.Ejecutar(ctx, func(ctx context.Context) error {
		if err := c.salas.Guardar(ctx, sala); err != nil {
			return err
		}
		for _, e := range eventos {
			if err := c.auditoria.Registrar(ctx, e, cmd.Origen); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return puertos.VistaSala{}, err
	}

	return vistaSalaDesde(sala), nil
}
