package cache

import (
	"fmt"

	"github.com/redis/go-redis/v9"
)

// NuevoClienteRedis construye un cliente Redis a partir de una URL de
// conexión (formato redis://[usuario:password@]host:puerto/db, el mismo
// que acepta `redis-cli -u`). No abre conexión de inmediato (go-redis es
// perezoso): el primer comando real es el que falla si Redis no está
// disponible, igual criterio que bd.NuevoPool con pgxpool.
func NuevoClienteRedis(url string) (*redis.Client, error) {
	if url == "" {
		return nil, fmt.Errorf("cache: URL de Redis vacía")
	}
	opciones, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("cache: URL de Redis inválida: %w", err)
	}
	return redis.NewClient(opciones), nil
}
