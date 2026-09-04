package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/r-david1/moterus/internal/plataforma/bd"
	"github.com/r-david1/moterus/internal/tenencia/adaptadores/postgres/sqlc"
)

// ejecutar es el helper compartido por los tres repositorios de este
// paquete para resolver dos cosas a la vez: (1) qué conjunto de queries
// sqlc usar (la transacción activa en ctx si UnidadDeTrabajo.Ejecutar ya la
// abrió, o una transacción propia de una sola consulta sobre el pool si
// no), y (2) qué alcance de RLS aplicar (§6.3 del diseño, ADR candidato
// 0031).
//
// Por qué CADA llamada (lectura o escritura) fija su propio alcance, no
// solo las que abren la UnidadDeTrabujo: el middleware de autorización
// HTTP de Tenencia publica el AlcanceDeTenencia UNA vez, para toda la
// petición, pero varias operaciones legítimas del contexto ocurren fuera
// de ese flujo:
//   - AutorizarCasoDeUso.Autorizar es quien DECIDE si el middleware debe
//     dejar pasar la petición: su propia lectura de membresias/organizaciones
//     ocurre ANTES de que exista ningún AlcanceDeTenencia publicado.
//   - Los ACL de Identidad/Confianza llaman a Tenencia (VerificadorDeAutorizacion,
//     ConsultorDeMembresias) directamente en proceso, sin pasar nunca por
//     el HTTP de Tenencia.
//   - AceptarInvitacion no cuelga de una ruta con {idOrganizacion} (§7 del
//     diseño): quien acepta no es miembro de ninguna organización todavía,
//     así que el middleware de autorización no tiene nada que autorizar, y
//     la organización de la invitación solo se conoce a mitad del flujo.
//
// Por eso cada método de repositorio pasa aquí el (idUsuario, idOrganizacion)
// que YA son parámetros propios de esa consulta o campos del propio
// agregado que persiste, como mejor esfuerzo para las llamadas que ocurren
// SIN un alcance ya establecido. Cuando SÍ existe uno (el middleware de
// autorización HTTP ya lo publicó para toda la petición, con el
// {idOrganizacion} de la ruta ya verificado contra el permiso requerido),
// bd.ReforzarAlcanceTenencia es un no-op: NUNCA lo sobrescribe con los
// parámetros de esta consulta puntual. Ese matiz es lo que hace que RLS siga
// siendo una red de seguridad real y no una tautología — si se reemitiera
// siempre con los propios parámetros de cada consulta, la política
// terminaría comparando ese valor consigo mismo ("organizacion_id =
// organizacion_id") y nunca podría atrapar el caso que está pensada para
// atrapar: un caso de uso futuro que, por un error, opera sobre una
// organización distinta de la que el middleware autorizó. Ver el
// comentario de bd.ReforzarAlcanceTenencia para el razonamiento completo.
// Cuando ninguno de los dos identificadores es conocido de antemano (p. ej.
// RepositorioInvitaciones.BuscarPorID, que resuelve por el identificador
// administrativo sin conocer su organización), tampoco se fija nada nuevo:
// se conserva el alcance que ya tuviera la transacción activa.
func ejecutar(ctx context.Context, pool *pgxpool.Pool, idUsuario, idOrganizacion string, fn func(ctx context.Context, q *sqlc.Queries) error) error {
	if tx, ok := bd.TxDesdeContexto(ctx); ok {
		if err := bd.ReforzarAlcanceTenencia(ctx, idUsuario, idOrganizacion); err != nil {
			return err
		}
		return fn(ctx, sqlc.New(tx))
	}

	ctxConAlcance := ctx
	if idUsuario != "" || idOrganizacion != "" {
		ctxConAlcance = bd.ConAlcanceTenencia(ctx, idUsuario, idOrganizacion)
	}
	return bd.EjecutarEnTransaccion(ctxConAlcance, pool, func(ctxTx context.Context) error {
		tx, _ := bd.TxDesdeContexto(ctxTx)
		return fn(ctxTx, sqlc.New(tx))
	})
}
