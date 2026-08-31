package cripto_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/r-david1/moterus/internal/identidad/adaptadores/cripto"
	"github.com/r-david1/moterus/internal/identidad/dominio"
)

func contrasenaDePrueba(t *testing.T, valor string) dominio.ContrasenaPlana {
	t.Helper()
	p, err := dominio.NuevaContrasenaPlana(valor)
	if err != nil {
		t.Fatalf("no se pudo construir la contraseña de prueba: %v", err)
	}
	return p
}

func TestHasherArgon2id_HashearYVerificar(t *testing.T) {
	h := cripto.NuevoHasherArgon2id()
	ctx := context.Background()
	plana := contrasenaDePrueba(t, "correcto caballo batería grapa")

	hash, err := h.Hashear(ctx, plana)
	if err != nil {
		t.Fatalf("Hashear() error = %v", err)
	}
	if !strings.HasPrefix(hash.Valor(), "$argon2id$v=") {
		t.Fatalf("formato PHC inesperado: %q", hash.Valor())
	}
	if hash.Algoritmo() != "argon2id" {
		t.Fatalf("Algoritmo() = %q, se esperaba argon2id", hash.Algoritmo())
	}

	ok, err := h.Verificar(ctx, hash, plana)
	if err != nil {
		t.Fatalf("Verificar() error = %v", err)
	}
	if !ok {
		t.Fatal("Verificar() = false para la contraseña correcta")
	}
}

func TestHasherArgon2id_VerificarRechazaContrasenaIncorrecta(t *testing.T) {
	h := cripto.NuevoHasherArgon2id()
	ctx := context.Background()
	hash, err := h.Hashear(ctx, contrasenaDePrueba(t, "correcto caballo batería grapa"))
	if err != nil {
		t.Fatalf("Hashear() error = %v", err)
	}

	ok, err := h.Verificar(ctx, hash, contrasenaDePrueba(t, "otra contraseña totalmente distinta"))
	if err != nil {
		t.Fatalf("Verificar() error = %v", err)
	}
	if ok {
		t.Fatal("Verificar() = true para una contraseña incorrecta")
	}
}

func TestHasherArgon2id_HashearProduceSalesDistintas(t *testing.T) {
	h := cripto.NuevoHasherArgon2id()
	ctx := context.Background()
	plana := contrasenaDePrueba(t, "correcto caballo batería grapa")

	hash1, err := h.Hashear(ctx, plana)
	if err != nil {
		t.Fatalf("Hashear() error = %v", err)
	}
	hash2, err := h.Hashear(ctx, plana)
	if err != nil {
		t.Fatalf("Hashear() error = %v", err)
	}
	if hash1.Valor() == hash2.Valor() {
		t.Fatal("dos llamadas a Hashear con la misma contraseña produjeron el mismo hash: la sal no es aleatoria")
	}
}

func TestHasherArgon2id_NecesitaRehash(t *testing.T) {
	h := cripto.NuevoHasherArgon2id()
	ctx := context.Background()

	hashVigente, err := h.Hashear(ctx, contrasenaDePrueba(t, "correcto caballo batería grapa"))
	if err != nil {
		t.Fatalf("Hashear() error = %v", err)
	}
	if h.NecesitaRehash(hashVigente) {
		t.Fatal("NecesitaRehash() = true para un hash recién calculado con los parámetros vigentes")
	}

	hashParametrosViejos, err := dominio.NuevoHashContrasena("$argon2id$v=19$m=4096,t=1,p=1$c2FsZXNhbHNhbA$aGFzaGhhc2hoYXNo")
	if err != nil {
		t.Fatalf("no se pudo construir el hash de prueba: %v", err)
	}
	if !h.NecesitaRehash(hashParametrosViejos) {
		t.Fatal("NecesitaRehash() = false para un hash con parámetros distintos a los vigentes")
	}

	hashOtroAlgoritmo, err := dominio.NuevoHashContrasena("$2a$10$abcdefghijklmnopqrstuv")
	if err != nil {
		t.Fatalf("no se pudo construir el hash de prueba: %v", err)
	}
	if !h.NecesitaRehash(hashOtroAlgoritmo) {
		t.Fatal("NecesitaRehash() = false para un hash de otro algoritmo (bcrypt)")
	}
}

func TestHasherArgon2id_ConsumirTiempoEquivalente(t *testing.T) {
	h := cripto.NuevoHasherArgon2id()
	ctx := context.Background()
	plana := contrasenaDePrueba(t, "correcto caballo batería grapa")

	hash, err := h.Hashear(ctx, plana)
	if err != nil {
		t.Fatalf("Hashear() error = %v", err)
	}

	inicioVerificar := time.Now()
	if _, err := h.Verificar(ctx, hash, plana); err != nil {
		t.Fatalf("Verificar() error = %v", err)
	}
	duracionVerificar := time.Since(inicioVerificar)

	inicioSenuelo := time.Now()
	h.ConsumirTiempoEquivalente(ctx)
	duracionSenuelo := time.Since(inicioSenuelo)

	// No se compara con igualdad estricta (el costo de un Argon2id real
	// varía por scheduling de CPU), pero el señuelo no debe ser
	// órdenes de magnitud más barato que una verificación real: eso
	// delataría la existencia de la cuenta por temporización (INV-ID-11).
	proporcion := float64(duracionSenuelo) / float64(duracionVerificar)
	if proporcion < 0.3 {
		t.Fatalf("ConsumirTiempoEquivalente() tardó %v, Verificar() tardó %v: demasiado más rápido para ser un costo equivalente", duracionSenuelo, duracionVerificar)
	}
}
