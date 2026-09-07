package integracion

import (
	"context"
	"os"
	"testing"
	"time"

	confianzaredis "github.com/r-david1/moterus/internal/confianza/adaptadores/redis"
	"github.com/r-david1/moterus/internal/confianza/puertos"
	"github.com/r-david1/moterus/internal/plataforma/cache"
)

// Este archivo cubre el adaptador Redis de la extensión de reconocimiento de
// origen (docs/design/fingerprinting-comportamiento.md, §5.2):
// internal/confianza/adaptadores/redis/perfil_origenes.go. Requiere Redis
// real (mismo criterio que confianza_redis_test.go y
// confianza_cola_redis_test.go): sin REDIS_URL, se salta limpiamente.

// perfilOrigenesRedis abre un PerfilOrigenes contra REDIS_URL, con el reloj
// interno reemplazado por relojFn (ConRelojPerfilOrigenes): así el test
// controla exactamente qué "ahora_ms" ve el script `registrar`, que es lo
// que decide cuál origen queda marcado como "el más antiguo" al podar
// (§5.2 del diseño, INV-RIES-11) — sin ese control, dos invocaciones que
// caen en el mismo milisegundo de reloj real volverían la poda ambigua.
func perfilOrigenesRedis(t *testing.T, relojFn func() time.Time) *confianzaredis.PerfilOrigenes {
	t.Helper()
	url := os.Getenv("REDIS_URL")
	if url == "" {
		t.Skip("REDIS_URL no está definido: se omiten los tests de integración de Confianza/Redis " +
			"(reconocimiento de origen, requieren Redis real levantado, ver deployments/docker-compose.yml)")
	}
	cliente, err := cache.NuevoClienteRedis(url)
	if err != nil {
		t.Fatalf("no se pudo construir el cliente Redis: %v", err)
	}
	if err := cliente.Ping(context.Background()).Err(); err != nil {
		t.Fatalf("no se pudo conectar a Redis en %s: %v", url, err)
	}
	t.Cleanup(func() { _ = cliente.Close() })
	return confianzaredis.NuevoPerfilDeOrigenes(cliente, confianzaredis.ConRelojPerfilOrigenes(relojFn))
}

// TestPerfilOrigenesRedis_CicloCompleto ejercita el ciclo completo que pide
// la verificación de la extensión: Registrar dos orígenes distintos para la
// misma cuenta -> Consultar los ve como conocidos -> Registrar un origen
// nuevo por encima de un techo bajo (maximoOrigenesRecordados=2, para que la
// poda se dispare con pocas escrituras) -> el más antiguo se poda -> Olvidar
// -> Consultar vuelve a ver todo como desconocido.
func TestPerfilOrigenesRedis_CicloCompleto(t *testing.T) {
	ahora := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	perfiles := perfilOrigenesRedis(t, func() time.Time { return ahora })
	ctx := context.Background()

	clave := claveUnicaDePrueba(t, "perfil-origen-ciclo")
	t.Cleanup(func() { _ = perfiles.Olvidar(context.Background(), clave) })

	const (
		hash1     = "aaaaaaaaaaaaaaaa"
		hash2     = "bbbbbbbbbbbbbbbb"
		hash3     = "cccccccccccccccc"
		techoBajo = 2 // deliberadamente bajo para este test, ver INV-RIES-11
	)

	registrar := func(t *testing.T, hash string, avance time.Duration) {
		t.Helper()
		ahora = ahora.Add(avance)
		if err := perfiles.Registrar(ctx, puertos.RegistrarOrigenObservado{
			Clave:                    clave,
			HashHuella:               hash,
			VidaPerfil:               time.Hour,
			MaximoOrigenesRecordados: techoBajo,
		}); err != nil {
			t.Fatalf("Registrar(%s): %v", hash, err)
		}
	}
	consultar := func(t *testing.T, hash string) puertos.VistaPerfilOrigen {
		t.Helper()
		vista, err := perfiles.Consultar(ctx, puertos.ConsultaPerfilOrigen{Clave: clave, HashHuella: hash})
		if err != nil {
			t.Fatalf("Consultar(%s): %v", hash, err)
		}
		return vista
	}

	// Dos orígenes distintos para la misma cuenta.
	registrar(t, hash1, 0)
	registrar(t, hash2, time.Millisecond)

	if !consultar(t, hash1).DispositivoConocido {
		t.Fatal("hash1 debería ser un dispositivo conocido tras Registrar")
	}
	vista2 := consultar(t, hash2)
	if !vista2.DispositivoConocido {
		t.Fatal("hash2 debería ser un dispositivo conocido tras Registrar")
	}
	if vista2.OrigenesConocidos != 2 {
		t.Fatalf("OrigenesConocidos = %d, esperado 2", vista2.OrigenesConocidos)
	}
	if vista2.Exitos != 2 {
		t.Fatalf("Exitos = %d, esperado 2", vista2.Exitos)
	}
	if vista2.ExitosConHuella != 2 {
		t.Fatalf("ExitosConHuella = %d, esperado 2", vista2.ExitosConHuella)
	}

	// Un tercer origen por encima del techo (2): el más antiguo (hash1) debe
	// podarse dentro del mismo script Lua que escribe (INV-RIES-11).
	registrar(t, hash3, time.Millisecond)

	if consultar(t, hash1).DispositivoConocido {
		t.Fatal("hash1 debería haber sido podado por ser el más antiguo de la familia 'd:'")
	}
	if !consultar(t, hash2).DispositivoConocido {
		t.Fatal("hash2 no debería haberse podado: no es el más antiguo")
	}
	vista3 := consultar(t, hash3)
	if !vista3.DispositivoConocido {
		t.Fatal("hash3 debería ser un dispositivo conocido tras Registrar")
	}
	if vista3.OrigenesConocidos != techoBajo {
		t.Fatalf("OrigenesConocidos = %d, esperado %d (el techo se respeta tras podar)", vista3.OrigenesConocidos, techoBajo)
	}

	// Olvidar borra el perfil completo: todo vuelve a "sin historial".
	if err := perfiles.Olvidar(ctx, clave); err != nil {
		t.Fatalf("Olvidar: %v", err)
	}
	vistaOlvidada := consultar(t, hash2)
	if vistaOlvidada.DispositivoConocido {
		t.Fatal("tras Olvidar, ningún dispositivo debería seguir siendo conocido")
	}
	if vistaOlvidada.Exitos != 0 || vistaOlvidada.ExitosConHuella != 0 {
		t.Fatalf("tras Olvidar, los contadores deberían estar en cero, obtuve Exitos=%d ExitosConHuella=%d",
			vistaOlvidada.Exitos, vistaOlvidada.ExitosConHuella)
	}
	if vistaOlvidada.OrigenesConocidos != 0 {
		t.Fatalf("tras Olvidar, OrigenesConocidos = %d, esperado 0", vistaOlvidada.OrigenesConocidos)
	}
}

// TestPerfilOrigenesRedis_FamiliasDispositivoYRedSePodanPorSeparado cubre que
// el script `registrar` poda cada familia ('d:' y 'n:') de forma
// independiente (§5.2 del diseño: "por cada familia de campos"): agotar el
// techo de dispositivos no debe afectar a las redes ya conocidas.
func TestPerfilOrigenesRedis_FamiliasDispositivoYRedSePodanPorSeparado(t *testing.T) {
	ahora := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	perfiles := perfilOrigenesRedis(t, func() time.Time { return ahora })
	ctx := context.Background()

	clave := claveUnicaDePrueba(t, "perfil-origen-familias")
	t.Cleanup(func() { _ = perfiles.Olvidar(context.Background(), clave) })

	const (
		red1      = "1111111111111111"
		dispA     = "aaaa111111111111"
		dispB     = "bbbb222222222222"
		techoBajo = 1
	)

	// Una red conocida...
	ahora = ahora.Add(time.Millisecond)
	if err := perfiles.Registrar(ctx, puertos.RegistrarOrigenObservado{
		Clave: clave, HashRed: red1, VidaPerfil: time.Hour, MaximoOrigenesRecordados: techoBajo,
	}); err != nil {
		t.Fatalf("Registrar(red1): %v", err)
	}

	// ...y luego dos dispositivos distintos, con el techo de dispositivos en 1.
	ahora = ahora.Add(time.Millisecond)
	if err := perfiles.Registrar(ctx, puertos.RegistrarOrigenObservado{
		Clave: clave, HashHuella: dispA, VidaPerfil: time.Hour, MaximoOrigenesRecordados: techoBajo,
	}); err != nil {
		t.Fatalf("Registrar(dispA): %v", err)
	}
	ahora = ahora.Add(time.Millisecond)
	if err := perfiles.Registrar(ctx, puertos.RegistrarOrigenObservado{
		Clave: clave, HashHuella: dispB, VidaPerfil: time.Hour, MaximoOrigenesRecordados: techoBajo,
	}); err != nil {
		t.Fatalf("Registrar(dispB): %v", err)
	}

	vistaRed, err := perfiles.Consultar(ctx, puertos.ConsultaPerfilOrigen{Clave: clave, HashRed: red1})
	if err != nil {
		t.Fatalf("Consultar(red1): %v", err)
	}
	if !vistaRed.RedConocida {
		t.Fatal("red1 no debería haberse podado: la poda de dispositivos no debe tocar la familia de redes")
	}

	vistaA, err := perfiles.Consultar(ctx, puertos.ConsultaPerfilOrigen{Clave: clave, HashHuella: dispA})
	if err != nil {
		t.Fatalf("Consultar(dispA): %v", err)
	}
	if vistaA.DispositivoConocido {
		t.Fatal("dispA debería haberse podado por ser el más antiguo de la familia 'd:'")
	}
	vistaB, err := perfiles.Consultar(ctx, puertos.ConsultaPerfilOrigen{Clave: clave, HashHuella: dispB})
	if err != nil {
		t.Fatalf("Consultar(dispB): %v", err)
	}
	if !vistaB.DispositivoConocido {
		t.Fatal("dispB no debería haberse podado")
	}
}

// TestPerfilOrigenesRedis_CamposVaciosNuncaCoincidenConUnCampoReal cubre la
// decisión de diseño de Consultar: si la petición no trae huella o la IP era
// privada (HashHuella/HashRed == ""), el HMGET pide un nombre de campo vacío
// que Registrar nunca escribe, así que jamás debe leerse como "conocido".
func TestPerfilOrigenesRedis_CamposVaciosNuncaCoincidenConUnCampoReal(t *testing.T) {
	ahora := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	perfiles := perfilOrigenesRedis(t, func() time.Time { return ahora })
	ctx := context.Background()

	clave := claveUnicaDePrueba(t, "perfil-origen-vacio")
	t.Cleanup(func() { _ = perfiles.Olvidar(context.Background(), clave) })

	// Una cuenta sin ningún origen registrado: Consultar con hashes vacíos
	// (equivalente a "la petición no traía huella ni IP pública") no debe
	// fallar ni marcar nada como conocido.
	vista, err := perfiles.Consultar(ctx, puertos.ConsultaPerfilOrigen{Clave: clave})
	if err != nil {
		t.Fatalf("Consultar con hashes vacíos no debería fallar: %v", err)
	}
	if vista.DispositivoConocido || vista.RedConocida {
		t.Fatalf("un perfil vacío consultado sin hashes no debe marcar nada como conocido, obtuve %+v", vista)
	}
	if vista.OrigenesConocidos != 0 {
		t.Fatalf("OrigenesConocidos = %d, esperado 0 para un perfil que nunca se registró", vista.OrigenesConocidos)
	}
}
