package aplicacion

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// vigenciaTokenVerificacionCorreo es la ventana de validez del token de
// verificación de correo (sección 3.4 del diseño): 24 horas. Vive como
// constante en esta capa, no en el esquema de base de datos, para poder
// ajustarla sin migración.
const vigenciaTokenVerificacionCorreo = 24 * time.Hour

// hashTokenVerificacion calcula el hash SHA-256 (hex) de un token de
// verificación de correo en claro.
//
// Por qué SHA-256 y no Argon2id aquí (esta capa no tiene la restricción
// INV-ID-18 de dominio, que solo aplica a identidad/dominio): Argon2id
// (ADR 0008) defiende contra fuerza bruta sobre secretos de baja entropía
// elegidos por humanos (contraseñas). Un token de verificación es un
// secreto aleatorio de alta entropía generado por el sistema (vía el
// puerto GeneradorTokens, crypto/rand del lado del adaptador) — no hay
// ataque de diccionario posible, así que el costo computacional de
// Argon2id no aporta nada y sí penalizaría innecesariamente cada
// verificación. SHA-256 (stdlib) es el criterio estándar para este tipo de
// token de un solo uso.
func hashTokenVerificacion(tokenPlano string) string {
	suma := sha256.Sum256([]byte(tokenPlano))
	return hex.EncodeToString(suma[:])
}
