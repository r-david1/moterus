package bd

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Este archivo implementa el único mecanismo genérico que
// internal/plataforma necesita para que Tenencia pueda sincronizar RLS
// (ADR candidato 0031 del diseño de Tenencia) con la decisión de
// autorización de la aplicación: dos GUC de sesión, `app.usuario_actual` y
// `app.organizacion_actual`, fijados con `SET LOCAL` (vía `set_config` con
// parámetros vinculados, nunca concatenación de strings) al abrir cada
// transacción.
//
// Kernel técnico: este paquete NO importa tipos de tenencia/dominio ni
// tenencia/puertos (seguiría siendo cierto incluso si Tenencia no
// existiera). AlcanceTenencia es un par de strings opaco; quien construye
// el context.Context con esos valores es el adaptador propio de Tenencia
// (internal/tenencia/adaptadores/postgres.AlcanceTenencia, que implementa
// puertos.AlcanceDeTenencia) — igual que TxDesdeContexto ya hace con la
// transacción pgx.
//
// Las transacciones de Identidad y Acceso nunca publican un AlcanceTenencia
// en su ctx, así que EjecutarEnTransaccion no cambia de comportamiento para
// ellos: ConAlcanceTenencia/alcanceTenenciaDesdeContexto son simplemente
// invisibles si nadie las invoca.
type claveAlcanceTenencia struct{}

// AlcanceTenencia es el par (usuario, organización) que las políticas RLS
// de Tenencia comparan contra `app.usuario_actual`/`app.organizacion_actual`.
// Un campo vacío se traduce a NULL en Postgres (vía `nullif` en las
// funciones `tenencia_usuario_actual`/`tenencia_organizacion_actual` de la
// migración 000014), lo que hace que la política correspondiente falle
// cerrada (INV-TEN-30) en vez de comparar contra una cadena vacía.
type AlcanceTenencia struct {
	IDUsuario      string
	IDOrganizacion string
}

// ConAlcanceTenencia publica un AlcanceTenencia en el context.Context. Lo
// invoca el adaptador propio de Tenencia que implementa
// puertos.AlcanceDeTenencia (desde el middleware de autorización HTTP), y
// también, con parámetros más acotados, los propios repositorios de
// Tenencia antes de abrir su propia transacción de una sola consulta (ver
// el comentario de cabecera de internal/tenencia/adaptadores/postgres).
func ConAlcanceTenencia(ctx context.Context, idUsuario, idOrganizacion string) context.Context {
	return context.WithValue(ctx, claveAlcanceTenencia{}, AlcanceTenencia{
		IDUsuario:      idUsuario,
		IDOrganizacion: idOrganizacion,
	})
}

// alcanceTenenciaDesdeContexto recupera el AlcanceTenencia publicado por
// ConAlcanceTenencia, si existe.
func alcanceTenenciaDesdeContexto(ctx context.Context) (AlcanceTenencia, bool) {
	a, ok := ctx.Value(claveAlcanceTenencia{}).(AlcanceTenencia)
	return a, ok
}

// fijarAlcanceTenencia emite los dos SET LOCAL de Tenencia sobre una
// transacción ya abierta, con `set_config(..., true)` — el tercer
// argumento `true` es `is_local`, el equivalente parametrizable de
// `SET LOCAL name = $1` (que la gramática de Postgres no admite con un
// parámetro vinculado). Nunca concatenación de strings.
func fijarAlcanceTenencia(ctx context.Context, tx pgx.Tx, a AlcanceTenencia) error {
	const sql = `SELECT set_config('app.usuario_actual', $1, true), set_config('app.organizacion_actual', $2, true)`
	if _, err := tx.Exec(ctx, sql, a.IDUsuario, a.IDOrganizacion); err != nil {
		return fmt.Errorf("bd: no se pudo fijar el alcance de tenencia: %w", err)
	}
	return nil
}

// ReforzarAlcanceTenencia fija el alcance de Tenencia sobre la transacción
// activa publicada en ctx, pero SOLO cuando ctx no trae ya un
// AlcanceTenencia explícito. Esta comprobación es la que hace que la
// "red de seguridad" de INV-TEN-30/ADR candidato 0031 sea real y no
// circular:
//
//   - Si el middleware de autorización HTTP de Tenencia ya publicó un
//     AlcanceTenencia (con ConAlcanceTenencia, tras verificar el permiso
//     contra el {idOrganizacion} de la ruta), EjecutarEnTransaccion ya lo
//     fijó al abrir la transacción, con un valor INDEPENDIENTE de lo que
//     cualquier repositorio vaya a consultar después. Esta función debe
//     dejarlo intacto: si en cambio lo reemitiera con los parámetros
//     propios de cada consulta puntual (p. ej. el organizacion_id que un
//     caso de uso con un bug futuro pasara por error), la política RLS
//     terminaría comparando ese valor consigo mismo — "organizacion_id =
//     organizacion_id" — siempre verdadero, y dejaría de poder detectar
//     exactamente el error que está pensada para atrapar. Por eso, en este
//     caso, la función es un no-op.
//   - Solo cuando ctx NO trae ningún AlcanceTenencia (operaciones que no
//     pasan por ese middleware: el propio caso de uso Autorizar, que
//     decide si el middleware debe dejar pasar la petición; los ACL de
//     Identidad/Confianza que llaman a Tenencia in-process; AceptarInvitacion,
//     que no cuelga de una ruta con {idOrganizacion} porque el aceptante
//     todavía no es miembro de ninguna organización; CrearOrganizacion, el
//     único caso de uso que no requiere autorización previa de Tenencia)
//     esta función deriva el alcance de los parámetros propios de la
//     consulta, como mejor esfuerzo para operar bajo FORCE ROW LEVEL
//     SECURITY sin un alcance previamente verificado. Aquí sí es
//     estructuralmente equivalente a "sin RLS" para esa consulta puntual
//     —no hay nada más confiable con qué compararla—, pero es
//     exactamente el mismo caso para el que ADR 0031 ya acepta que la
//     frontera real es la decisión de AutorizarCasoDeUso, no RLS.
func ReforzarAlcanceTenencia(ctx context.Context, idUsuario, idOrganizacion string) error {
	if _, yaHayAlcance := alcanceTenenciaDesdeContexto(ctx); yaHayAlcance {
		return nil
	}
	if idUsuario == "" && idOrganizacion == "" {
		return nil
	}
	tx, ok := TxDesdeContexto(ctx)
	if !ok {
		return nil
	}
	return fijarAlcanceTenencia(ctx, tx, AlcanceTenencia{IDUsuario: idUsuario, IDOrganizacion: idOrganizacion})
}
