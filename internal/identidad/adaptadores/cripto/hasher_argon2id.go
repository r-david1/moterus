package cripto

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"

	"github.com/r-david1/moterus/internal/identidad/dominio"
	"github.com/r-david1/moterus/internal/identidad/puertos"
)

// Parámetros de Argon2id fijados por ADR 0008
// (docs/adr/0008-argon2id-parametros.md). No son configurables por entorno a
// propósito: subir memoria/iteraciones en el futuro se hace cambiando estas
// constantes y dejando que NecesitaRehash + ReemplazarHash (INV-ID-13)
// migren los hashes existentes de forma oportunista, sin migración masiva.
const (
	argonMemoriaKiB   uint32 = 64 * 1024 // 64 MiB, piso RFC 9106 para uso interactivo.
	argonIteraciones  uint32 = 3
	argonParalelismo  uint8  = 1
	argonTamSalBytes  int    = 16
	argonTamHashBytes uint32 = 32
	argonIDAlgoritmo  string = "argon2id"
)

// HasherArgon2id implementa puertos.HasherContrasenas con Argon2id
// (golang.org/x/crypto/argon2), almacenando el hash en formato PHC
// autocontenido: $argon2id$v=19$m=65536,t=3,p=1$<sal>$<hash>. Vive en
// adaptadores (no en dominio) porque depende de una biblioteca externa de
// criptografía, fuera de la excepción única que INV-ID-18 concede a
// golang.org/x/text en el paquete dominio (ver ADR 0008).
type HasherArgon2id struct{}

var _ puertos.HasherContrasenas = (*HasherArgon2id)(nil)

// NuevoHasherArgon2id construye el adaptador. No tiene estado ni
// dependencias externas más allá de crypto/rand.
func NuevoHasherArgon2id() *HasherArgon2id { return &HasherArgon2id{} }

// Hashear calcula un hash Argon2id con los parámetros vigentes de ADR 0008 y
// una sal aleatoria de 16 bytes (crypto/rand), nunca reutilizada.
func (HasherArgon2id) Hashear(_ context.Context, p dominio.ContrasenaPlana) (dominio.HashContrasena, error) {
	sal := make([]byte, argonTamSalBytes)
	if _, err := rand.Read(sal); err != nil {
		return dominio.HashContrasena{}, fmt.Errorf("cripto: no se pudo generar la sal: %w", err)
	}
	return dominio.NuevoHashContrasena(codificarPHC(sal, calcularHash(p.Valor(), sal, argonMemoriaKiB, argonIteraciones, argonParalelismo)))
}

// Verificar recalcula el hash Argon2id de p con los parámetros embebidos en
// h (no los parámetros vigentes: un hash antiguo con memoria menor sigue
// siendo verificable; NecesitaRehash es quien decide si conviene
// actualizarlo) y lo compara en tiempo constante.
func (HasherArgon2id) Verificar(_ context.Context, h dominio.HashContrasena, p dominio.ContrasenaPlana) (bool, error) {
	parametros, sal, hashAlmacenado, err := analizarPHC(h.Valor())
	if err != nil {
		return false, err
	}
	calculado := calcularHash(p.Valor(), sal, parametros.memoria, parametros.iteraciones, parametros.paralelismo)
	if len(calculado) != len(hashAlmacenado) {
		return false, nil
	}
	return subtle.ConstantTimeCompare(calculado, hashAlmacenado) == 1, nil
}

// NecesitaRehash compara los parámetros embebidos en h contra los
// parámetros vigentes del adaptador (ADR 0008). Un hash con otro algoritmo,
// u otros valores de memoria/iteraciones/paralelismo, o que ni siquiera se
// pueda analizar, se marca para rehash.
func (HasherArgon2id) NecesitaRehash(h dominio.HashContrasena) bool {
	if h.Algoritmo() != argonIDAlgoritmo {
		return true
	}
	parametros, _, _, err := analizarPHC(h.Valor())
	if err != nil {
		return true
	}
	return parametros.memoria != argonMemoriaKiB ||
		parametros.iteraciones != argonIteraciones ||
		parametros.paralelismo != argonParalelismo
}

// ConsumirTiempoEquivalente ejecuta un hash Argon2id señuelo con los mismos
// parámetros vigentes que Hashear/Verificar, sobre un valor descartable y
// una sal aleatoria, y descarta el resultado. Se invoca cuando el correo no
// existe (INV-ID-11), para que el tiempo de respuesta de un login con
// cuenta inexistente sea comparable al de un login con contraseña
// incorrecta — nunca un time.Sleep fijo arbitrario, que no reproduce el
// mismo perfil de coste de CPU/memoria que un cómputo Argon2id real.
func (HasherArgon2id) ConsumirTiempoEquivalente(_ context.Context) {
	sal := make([]byte, argonTamSalBytes)
	_, _ = rand.Read(sal) // best-effort: incluso con sal predecible, el coste computacional es el mismo.
	_ = calcularHash("señuelo-tiempo-equivalente", sal, argonMemoriaKiB, argonIteraciones, argonParalelismo)
}

func calcularHash(plana string, sal []byte, memoria, iteraciones uint32, paralelismo uint8) []byte {
	return argon2.IDKey([]byte(plana), sal, iteraciones, memoria, paralelismo, argonTamHashBytes)
}

// parametrosArgon2 son los parámetros de coste embebidos en un hash PHC.
type parametrosArgon2 struct {
	memoria     uint32
	iteraciones uint32
	paralelismo uint8
}

// codificarPHC arma la cadena de almacenamiento
// $argon2id$v=<version>$m=<memoria>,t=<iteraciones>,p=<paralelismo>$<sal>$<hash>
// con sal y hash en base64 sin padding (RawStdEncoding), el formato habitual
// de las implementaciones de referencia de Argon2.
func codificarPHC(sal, hash []byte) string {
	return fmt.Sprintf(
		"$%s$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argonIDAlgoritmo,
		argon2.Version,
		argonMemoriaKiB,
		argonIteraciones,
		argonParalelismo,
		base64.RawStdEncoding.EncodeToString(sal),
		base64.RawStdEncoding.EncodeToString(hash),
	)
}

// analizarPHC descompone una cadena PHC en sus parámetros de coste, la sal y
// el hash. dominio.HashContrasena ya garantiza el formato mínimo ("$alg$...
// con al menos 3 partes"); aquí se exige la forma completa de 6 partes que
// produce codificarPHC.
func analizarPHC(valor string) (parametrosArgon2, []byte, []byte, error) {
	partes := strings.Split(valor, "$")
	// ["", "argon2id", "v=19", "m=...,t=...,p=...", "<sal>", "<hash>"]
	if len(partes) != 6 || partes[1] != argonIDAlgoritmo {
		return parametrosArgon2{}, nil, nil, fmt.Errorf("cripto: formato de hash argon2id irreconocible")
	}

	var version int
	if _, err := fmt.Sscanf(partes[2], "v=%d", &version); err != nil {
		return parametrosArgon2{}, nil, nil, fmt.Errorf("cripto: version de argon2id irreconocible: %w", err)
	}

	var p parametrosArgon2
	if _, err := fmt.Sscanf(partes[3], "m=%d,t=%d,p=%d", &p.memoria, &p.iteraciones, &p.paralelismo); err != nil {
		return parametrosArgon2{}, nil, nil, fmt.Errorf("cripto: parametros de argon2id irreconocibles: %w", err)
	}

	sal, err := base64.RawStdEncoding.DecodeString(partes[4])
	if err != nil {
		return parametrosArgon2{}, nil, nil, fmt.Errorf("cripto: sal de argon2id irreconocible: %w", err)
	}
	hash, err := base64.RawStdEncoding.DecodeString(partes[5])
	if err != nil {
		return parametrosArgon2{}, nil, nil, fmt.Errorf("cripto: hash de argon2id irreconocible: %w", err)
	}

	return p, sal, hash, nil
}
