// Package jwt implementa puertos.FirmadorTokensAcceso (ADR 0020) sobre
// github.com/lestrrat-go/jwx/v2: firma EdDSA/Ed25519 primaria, JWKS con
// kid = thumbprint RFC 7638, y verificación que deriva el algoritmo del
// kid presente en el conjunto de llaves, nunca del campo `alg` de la
// cabecera del token que se está validando (defensa contra algorithm
// confusion). Ningún tipo de jwx cruza los puertos hacia aplicacion ni
// dominio (INV-ACC-18): este paquete es el único, junto con
// adaptadores/redis, que sabe qué biblioteca de JWT usa el proyecto.
package jwt

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"strings"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
)

// llaveActiva agrupa una llave privada Ed25519 con su representación JWK
// pública ya completa (kid = thumbprint RFC 7638, alg=EdDSA, use=sig).
type llaveActiva struct {
	privada ed25519.PrivateKey
	publica jwk.Key
	kid     string
}

// nuevaLlaveActiva deriva la llave pública JWK de privada y le asigna kid
// (jwk.AssignKeyID: thumbprint RFC 7638 por defecto, SHA-256 — ADR 0020 §3),
// algoritmo EdDSA y uso "sig".
func nuevaLlaveActiva(privada ed25519.PrivateKey) (llaveActiva, error) {
	pub, ok := privada.Public().(ed25519.PublicKey)
	if !ok {
		return llaveActiva{}, fmt.Errorf("jwt: la llave privada no es Ed25519")
	}
	jwkPub, err := jwk.FromRaw(pub)
	if err != nil {
		return llaveActiva{}, fmt.Errorf("jwt: no se pudo construir la llave pública JWK: %w", err)
	}
	if err := jwkPub.Set(jwk.AlgorithmKey, jwa.EdDSA); err != nil {
		return llaveActiva{}, fmt.Errorf("jwt: no se pudo fijar el algoritmo de la llave: %w", err)
	}
	if err := jwkPub.Set(jwk.KeyUsageKey, "sig"); err != nil {
		return llaveActiva{}, fmt.Errorf("jwt: no se pudo fijar el uso de la llave: %w", err)
	}
	if err := jwk.AssignKeyID(jwkPub); err != nil {
		return llaveActiva{}, fmt.Errorf("jwt: no se pudo calcular el kid (thumbprint RFC 7638): %w", err)
	}
	kidCrudo, ok := jwkPub.Get(jwk.KeyIDKey)
	if !ok {
		return llaveActiva{}, fmt.Errorf("jwt: la llave JWK no tiene kid tras AssignKeyID")
	}
	kid, ok := kidCrudo.(string)
	if !ok || kid == "" {
		return llaveActiva{}, fmt.Errorf("jwt: kid con tipo inesperado: %T", kidCrudo)
	}
	return llaveActiva{privada: privada, publica: jwkPub, kid: kid}, nil
}

// cargarLlaveEd25519Privada parsea el valor de ACCESO_LLAVE_FIRMA o de
// cada elemento de ACCESO_LLAVES_VERIFICACION_PREVIAS (ADR 0020 §4): PEM
// (PKCS8) de una llave Ed25519, o un seed/llave de 32/64 bytes en
// base64 (estándar, sin relleno, o URL-safe — se prueban todas las
// variantes para no exigirle al operador un formato exacto).
//
// Alcance deliberado de este MVP: solo Ed25519. La firma primaria es
// siempre Ed25519 (ADR 0020 §1: "Ed25519 desde el día uno"), y hoy no
// existe ninguna llave RS256 real que cargar — el soporte de RS256 "por el
// verificador" que exige el ADR es estructural (Llavero.verificacion es un
// jwk.Set genérico y ValidadorDeAccesos.Verificar no asume ningún
// algoritmo concreto), pero el parseo de un PEM RSA queda fuera de este
// MVP hasta que haga falta una migración real de algoritmo. Ver el informe
// de la tarea.
func cargarLlaveEd25519Privada(valor string) (ed25519.PrivateKey, error) {
	v := strings.TrimSpace(valor)
	if v == "" {
		return nil, fmt.Errorf("jwt: valor de llave vacío")
	}

	if bloque, _ := pem.Decode([]byte(v)); bloque != nil {
		clave, err := x509.ParsePKCS8PrivateKey(bloque.Bytes)
		if err != nil {
			return nil, fmt.Errorf("jwt: no se pudo parsear la llave PEM (PKCS8): %w", err)
		}
		ed, ok := clave.(ed25519.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("jwt: la llave PEM no es Ed25519 (tipo %T)", clave)
		}
		return ed, nil
	}

	seed, err := decodificarBase64Flexible(v)
	if err != nil {
		return nil, fmt.Errorf("jwt: la llave no es PEM ni base64 válido: %w", err)
	}
	switch len(seed) {
	case ed25519.SeedSize:
		return ed25519.NewKeyFromSeed(seed), nil
	case ed25519.PrivateKeySize:
		return ed25519.PrivateKey(seed), nil
	default:
		return nil, fmt.Errorf("jwt: longitud de llave Ed25519 inesperada: %d bytes (se esperaban %d o %d)",
			len(seed), ed25519.SeedSize, ed25519.PrivateKeySize)
	}
}

// decodificarBase64Flexible prueba las variantes usuales de base64 en
// orden, para no obligar al operador a un formato exacto de codificación.
func decodificarBase64Flexible(v string) ([]byte, error) {
	if b, err := base64.StdEncoding.DecodeString(v); err == nil {
		return b, nil
	}
	if b, err := base64.RawStdEncoding.DecodeString(v); err == nil {
		return b, nil
	}
	if b, err := base64.URLEncoding.DecodeString(v); err == nil {
		return b, nil
	}
	return base64.RawURLEncoding.DecodeString(v)
}

// Llavero agrupa la llave de firma activa y el conjunto de llaves públicas
// de verificación (la activa + cualquier previa configurada, ADR 0020 §4).
type Llavero struct {
	activa       llaveActiva
	verificacion jwk.Set
}

// NuevoLlavero construye el llavero a partir de los valores crudos de
// configuración: llaveFirmaCruda es ACCESO_LLAVE_FIRMA (obligatoria);
// llavesVerificacionPreviasCrudas son los elementos ya separados por comas
// de ACCESO_LLAVES_VERIFICACION_PREVIAS (puede ir vacío). Cada llave previa
// se parsea con el mismo formato que la activa (ADR 0020 §4: rotar es
// "mover" el valor de ACCESO_LLAVE_FIRMA a la lista de previas) y solo se
// conserva su mitad pública en el conjunto de verificación.
func NuevoLlavero(llaveFirmaCruda string, llavesVerificacionPreviasCrudas []string) (*Llavero, error) {
	privada, err := cargarLlaveEd25519Privada(llaveFirmaCruda)
	if err != nil {
		return nil, fmt.Errorf("jwt: ACCESO_LLAVE_FIRMA inválida: %w", err)
	}
	activa, err := nuevaLlaveActiva(privada)
	if err != nil {
		return nil, err
	}

	conjunto := jwk.NewSet()
	if err := conjunto.AddKey(activa.publica); err != nil {
		return nil, fmt.Errorf("jwt: no se pudo agregar la llave activa al conjunto de verificación: %w", err)
	}

	for _, cruda := range llavesVerificacionPreviasCrudas {
		cruda = strings.TrimSpace(cruda)
		if cruda == "" {
			continue
		}
		prevPrivada, err := cargarLlaveEd25519Privada(cruda)
		if err != nil {
			return nil, fmt.Errorf("jwt: ACCESO_LLAVES_VERIFICACION_PREVIAS inválida: %w", err)
		}
		prevActiva, err := nuevaLlaveActiva(prevPrivada)
		if err != nil {
			return nil, err
		}
		if prevActiva.kid == activa.kid {
			// La misma llave ya está en el conjunto (p. ej. quedó duplicada
			// en la configuración durante una rotación): AddKey volvería a
			// insertarla sin daño, pero se salta explícitamente para dejar
			// claro que no es un error de configuración.
			continue
		}
		if err := conjunto.AddKey(prevActiva.publica); err != nil {
			return nil, fmt.Errorf("jwt: no se pudo agregar una llave previa al conjunto de verificación: %w", err)
		}
	}

	return &Llavero{activa: activa, verificacion: conjunto}, nil
}

// KIDActivo devuelve el kid (thumbprint RFC 7638) de la llave de firma
// activa. Útil para logging de arranque (cmd/api/main.go): confirma en el
// log qué llave concreta está firmando, sin exponer ningún material
// privado.
func (l *Llavero) KIDActivo() string { return l.activa.kid }

// GenerarLlaveEfimera produce un seed Ed25519 nuevo de crypto/rand,
// codificado en base64 estándar, en el mismo formato que acepta
// ACCESO_LLAVE_FIRMA. Es lo que cmd/api/main.go usa cuando falta esa
// variable fuera de APP_ENV=production (ADR 0020 §4: "en desarrollo se
// genera una llave efímera en memoria con WARN explícito de que todos los
// tokens mueren al reiniciar"). El llamador es responsable de emitir ese
// WARN; esta función solo genera la llave.
func GenerarLlaveEfimera() (string, error) {
	seed := make([]byte, ed25519.SeedSize)
	if _, err := rand.Read(seed); err != nil {
		return "", fmt.Errorf("jwt: no se pudo generar la llave efímera: %w", err)
	}
	return base64.StdEncoding.EncodeToString(seed), nil
}
