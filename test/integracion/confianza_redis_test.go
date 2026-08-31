package integracion

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/r-david1/moterus/internal/confianza/adaptadores/redis"
	"github.com/r-david1/moterus/internal/confianza/dominio"
	"github.com/r-david1/moterus/internal/plataforma/cache"
)

// clienteRedis abre un cliente contra REDIS_URL y lo cierra al terminar el
// test. Si REDIS_URL no está definido, el test se salta limpiamente (mismo
// criterio que dsnAplicacion/dsnDueno para Postgres): `go test ./...`
// sigue funcionando sin `make docker-up`.
func clienteRedis(t *testing.T) *redis.LimitadorTasa {
	t.Helper()
	url := os.Getenv("REDIS_URL")
	if url == "" {
		t.Skip("REDIS_URL no está definido: se omiten los tests de integración de Confianza/Redis " +
			"(requieren Redis real levantado, ver deployments/docker-compose.yml)")
	}
	cliente, err := cache.NuevoClienteRedis(url)
	if err != nil {
		t.Fatalf("no se pudo construir el cliente Redis: %v", err)
	}
	if err := cliente.Ping(context.Background()).Err(); err != nil {
		t.Fatalf("no se pudo conectar a Redis en %s: %v", url, err)
	}
	t.Cleanup(func() { _ = cliente.Close() })
	return redis.NuevoLimitadorTasa(cliente, redis.ConBackoffMaximo(2*time.Second))
}

func TestLimitadorTasaRedis_permiteHastaElLimiteYLuegoBloquea(t *testing.T) {
	limitador := clienteRedis(t)
	ctx := context.Background()
	clave := claveUnicaDePrueba(t, "rl-limite")
	umbral := dominio.Umbral{Limite: 3, Ventana: time.Minute}
	t.Cleanup(func() { _ = limitador.Reiniciar(ctx, clave) })

	for i := 1; i <= 3; i++ {
		permitido, restantes, _, err := limitador.Permitir(ctx, clave, umbral)
		if err != nil {
			t.Fatalf("intento %d: error inesperado: %v", i, err)
		}
		if !permitido {
			t.Fatalf("intento %d: se esperaba permitido=true dentro del límite", i)
		}
		if restantes != umbral.Limite-i {
			t.Fatalf("intento %d: restantes = %d, esperado %d", i, restantes, umbral.Limite-i)
		}
	}

	permitido, restantes, reintentarEn, err := limitador.Permitir(ctx, clave, umbral)
	if err != nil {
		t.Fatalf("cuarto intento: error inesperado: %v", err)
	}
	if permitido {
		t.Fatalf("el cuarto intento debía exceder el límite de 3")
	}
	if restantes != 0 {
		t.Fatalf("restantes tras exceder = %d, esperado 0", restantes)
	}
	if reintentarEn <= 0 {
		t.Fatalf("reintentarEn debe ser > 0 tras exceder el límite, fue %v", reintentarEn)
	}
}

func TestLimitadorTasaRedis_reiniciarBorraElContador(t *testing.T) {
	limitador := clienteRedis(t)
	ctx := context.Background()
	clave := claveUnicaDePrueba(t, "rl-reinicio")
	umbral := dominio.Umbral{Limite: 1, Ventana: time.Minute}
	t.Cleanup(func() { _ = limitador.Reiniciar(ctx, clave) })

	if permitido, _, _, err := limitador.Permitir(ctx, clave, umbral); err != nil || !permitido {
		t.Fatalf("primer intento: permitido=%v err=%v", permitido, err)
	}
	if permitido, _, _, err := limitador.Permitir(ctx, clave, umbral); err != nil || permitido {
		t.Fatalf("segundo intento antes de reiniciar debía exceder el límite: permitido=%v err=%v", permitido, err)
	}

	if err := limitador.Reiniciar(ctx, clave); err != nil {
		t.Fatalf("Reiniciar: error inesperado: %v", err)
	}

	if permitido, _, _, err := limitador.Permitir(ctx, clave, umbral); err != nil || !permitido {
		t.Fatalf("tras reiniciar, el primer intento debe volver a estar permitido: permitido=%v err=%v", permitido, err)
	}
}

func TestLimitadorTasaRedis_backoffExponencialCrecienteAlSeguirExcediendo(t *testing.T) {
	limitador := clienteRedis(t)
	ctx := context.Background()
	clave := claveUnicaDePrueba(t, "rl-backoff")
	umbral := dominio.Umbral{Limite: 1, Ventana: 200 * time.Millisecond}
	t.Cleanup(func() { _ = limitador.Reiniciar(ctx, clave) })

	if permitido, _, _, err := limitador.Permitir(ctx, clave, umbral); err != nil || !permitido {
		t.Fatalf("primer intento: permitido=%v err=%v", permitido, err)
	}

	_, _, primerReintento, err := limitador.Permitir(ctx, clave, umbral)
	if err != nil {
		t.Fatalf("segundo intento: error inesperado: %v", err)
	}
	_, _, segundoReintento, err := limitador.Permitir(ctx, clave, umbral)
	if err != nil {
		t.Fatalf("tercer intento: error inesperado: %v", err)
	}

	if segundoReintento <= primerReintento {
		t.Fatalf("el cooldown debe crecer con cada intento adicional que sigue excediendo el límite: primero=%v segundo=%v",
			primerReintento, segundoReintento)
	}
}

// claveUnicaDePrueba evita colisiones entre corridas del test suite contra
// el mismo Redis compartido de desarrollo.
func claveUnicaDePrueba(t *testing.T, sufijo string) string {
	t.Helper()
	return "test:" + t.Name() + ":" + sufijo + ":" + time.Now().Format(time.RFC3339Nano)
}
