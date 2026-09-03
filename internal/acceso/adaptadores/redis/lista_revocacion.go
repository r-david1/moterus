// Package redis implementa puertos.ListaRevocacion (ADR 0019 §4, INV-ACC-15)
// sobre Redis (go-redis/v9), respaldado por el cliente compartido de
// internal/plataforma/cache. Es el ACELERADOR de la revocación, no su
// frontera de seguridad: si Redis no está disponible, ListaRevocacionNoOp
// hace que Disponible() devuelva false y la revocación sigue siendo
// correcta (la sesión en Postgres es la autoridad), solo se degrada al
// peor caso conocido — hasta PoliticaSesion.VidaTokenAcceso() (10 minutos
// por defecto).
package redis

import (
	"context"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/r-david1/moterus/internal/acceso/dominio"
	"github.com/r-david1/moterus/internal/acceso/puertos"
)

// prefijoClave namespacea las claves de este contexto dentro de Redis
// (mismo criterio que confianza/adaptadores/redis: "confianza:rl:...").
const prefijoClave = "acceso:revocadas:"

// ListaRevocacion implementa puertos.ListaRevocacion contra un
// *redis.Client (go-redis/v9) ya conectado.
type ListaRevocacion struct {
	cliente *goredis.Client
}

var _ puertos.ListaRevocacion = (*ListaRevocacion)(nil)

// NuevaListaRevocacion construye el adaptador sobre un cliente Redis ya
// creado. No valida la conexión aquí (mismo criterio que bd.NuevoPool): la
// primera llamada real es la que falla si Redis no responde, y ese fallo
// se trata como fail-open en ValidarAcceso/los casos de uso que la
// consumen (INV-ACC-15), nunca como un error que bloquee el negocio.
func NuevaListaRevocacion(cliente *goredis.Client) *ListaRevocacion {
	return &ListaRevocacion{cliente: cliente}
}

func clave(idSesion dominio.IDSesion) string {
	return prefijoClave + idSesion.String()
}

// RevocarSesion publica el sid en la lista con TTL igual al tiempo que
// falta hasta hasta (nunca negativo: si hasta ya pasó, se usa 1 segundo
// para que la clave siga existiendo el tiempo mínimo necesario para que
// una lectura concurrente la vea).
func (l *ListaRevocacion) RevocarSesion(ctx context.Context, idSesion dominio.IDSesion, hasta time.Time) error {
	ttl := time.Until(hasta)
	if ttl <= 0 {
		ttl = time.Second
	}
	if err := l.cliente.Set(ctx, clave(idSesion), "1", ttl).Err(); err != nil {
		return fmt.Errorf("acceso/redis: no se pudo publicar la revocación: %w", err)
	}
	return nil
}

// SesionRevocada consulta si el sid está en la lista de revocación.
func (l *ListaRevocacion) SesionRevocada(ctx context.Context, idSesion dominio.IDSesion) (bool, error) {
	n, err := l.cliente.Exists(ctx, clave(idSesion)).Result()
	if err != nil && !errors.Is(err, goredis.Nil) {
		return false, fmt.Errorf("acceso/redis: no se pudo consultar la lista de revocación: %w", err)
	}
	return n > 0, nil
}

// Disponible siempre devuelve true: este adaptador solo se monta cuando
// REDIS_URL está configurado (ver cmd/api/main.go).
func (l *ListaRevocacion) Disponible() bool { return true }

// ListaRevocacionNoOp implementa puertos.ListaRevocacion cuando Redis no
// está configurado (REDIS_URL vacío). Disponible() siempre false: los
// casos de uso que la consumen ya saben que deben omitir el paso
// (INV-ACC-15), así que RevocarSesion/SesionRevocada no deberían
// invocarse nunca en la práctica, pero se implementan como no-ops seguros
// por si acaso.
type ListaRevocacionNoOp struct{}

var _ puertos.ListaRevocacion = ListaRevocacionNoOp{}

// NuevaListaRevocacionNoOp construye el adaptador no-op.
func NuevaListaRevocacionNoOp() ListaRevocacionNoOp { return ListaRevocacionNoOp{} }

// RevocarSesion no hace nada.
func (ListaRevocacionNoOp) RevocarSesion(context.Context, dominio.IDSesion, time.Time) error {
	return nil
}

// SesionRevocada siempre informa que no está revocada: el llamador nunca
// debería preguntarlo porque Disponible() es false.
func (ListaRevocacionNoOp) SesionRevocada(context.Context, dominio.IDSesion) (bool, error) {
	return false, nil
}

// Disponible siempre false.
func (ListaRevocacionNoOp) Disponible() bool { return false }
