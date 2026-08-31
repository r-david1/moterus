package dominio

import (
	"strings"
	"time"
	"unicode/utf8"
)

// longitudMaximaContrasenaPlana evita el DoS de hashing con entradas
// arbitrariamente largas (tabla 1.3).
const longitudMaximaContrasenaPlana = 4096

// HashContrasena envuelve un hash de contraseña ya calculado en formato PHC
// (p. ej. "$argon2id$v=19$m=65536,t=3,p=1$sal$hash"). Nunca contiene la
// contraseña en claro (INV-ID-04).
type HashContrasena struct {
	valor string
}

// NuevoHashContrasena valida que el hash tenga un formato PHC reconocible.
func NuevoHashContrasena(valor string) (HashContrasena, error) {
	v := strings.TrimSpace(valor)
	if v == "" {
		return HashContrasena{}, &ErrHashContrasenaInvalido{Motivo: "no puede estar vacío"}
	}
	if !strings.HasPrefix(v, "$") {
		return HashContrasena{}, &ErrHashContrasenaInvalido{Motivo: "no tiene formato PHC reconocible"}
	}
	partes := strings.Split(v, "$")
	if len(partes) < 3 || partes[1] == "" {
		return HashContrasena{}, &ErrHashContrasenaInvalido{Motivo: "no tiene formato PHC reconocible"}
	}
	return HashContrasena{valor: v}, nil
}

// Algoritmo devuelve el identificador del algoritmo (p. ej. "argon2id"),
// tomado del formato PHC, para que el adaptador criptográfico decida si
// necesita rehash.
func (h HashContrasena) Algoritmo() string {
	partes := strings.Split(h.valor, "$")
	if len(partes) < 2 {
		return ""
	}
	return partes[1]
}

// Valor expone el hash crudo para que los adaptadores de persistencia lo
// almacenen. No debe usarse para logging: String()/GoString()/MarshalJSON
// redactan deliberadamente el valor (tabla 1.3: "nunca se serializa en logs
// ni respuestas").
func (h HashContrasena) Valor() string { return h.valor }

// EsVacio indica si el value object nunca fue construido (zero value).
func (h HashContrasena) EsVacio() bool { return h.valor == "" }

// String redacta el hash.
func (h HashContrasena) String() string { return "[REDACTADO]" }

// GoString redacta el hash en el formato %#v.
func (h HashContrasena) GoString() string { return "[REDACTADO]" }

// MarshalJSON redacta el hash si el value object se serializa por
// descuido.
func (h HashContrasena) MarshalJSON() ([]byte, error) {
	return []byte(`"[REDACTADO]"`), nil
}

// Credencial agrupa el hash de contraseña vigente de un Usuario y la marca
// de tiempo de su última actualización.
type Credencial struct {
	hash          HashContrasena
	actualizadaEn time.Time
}

// NuevaCredencial construye una Credencial a partir de un hash ya validado.
func NuevaCredencial(hash HashContrasena, actualizadaEn time.Time) (Credencial, error) {
	if hash.EsVacio() {
		return Credencial{}, &ErrCredencialInvalida{Motivo: "el hash no puede estar vacío"}
	}
	return Credencial{hash: hash, actualizadaEn: actualizadaEn}, nil
}

// Hash devuelve el hash de contraseña vigente.
func (c Credencial) Hash() HashContrasena { return c.hash }

// ActualizadaEn devuelve cuándo se estableció este hash por última vez.
func (c Credencial) ActualizadaEn() time.Time { return c.actualizadaEn }

// ContrasenaPlana es un value object efímero que envuelve una contraseña en
// texto claro solo durante el tiempo mínimo necesario para hashearla o
// verificarla. Nunca se persiste, ni se registra en logs, ni viaja en un
// evento de dominio, ni aparece en un mensaje de error (INV-ID-04).
type ContrasenaPlana struct {
	valor string
}

// NuevaContrasenaPlana valida solo restricciones estructurales (no vacía,
// longitud acotada); la fortaleza de la contraseña la evalúa el servicio de
// dominio PoliticaContrasena.
func NuevaContrasenaPlana(valor string) (ContrasenaPlana, error) {
	if valor == "" {
		return ContrasenaPlana{}, &ErrContrasenaPlanaInvalida{Motivo: "no puede estar vacía"}
	}
	if len(valor) > longitudMaximaContrasenaPlana {
		return ContrasenaPlana{}, &ErrContrasenaPlanaInvalida{Motivo: "supera la longitud máxima permitida"}
	}
	return ContrasenaPlana{valor: valor}, nil
}

// Longitud devuelve el número de caracteres (runas) de la contraseña, sin
// exponer su contenido.
func (p ContrasenaPlana) Longitud() int { return utf8.RuneCountInString(p.valor) }

// Valor expone la contraseña en claro. Solo debe usarlo un adaptador
// criptográfico (p. ej. la implementación de HasherContrasenas) para
// hashear o verificar; nunca debe registrarse ni persistirse.
func (p ContrasenaPlana) Valor() string { return p.valor }

// String redacta la contraseña (INV-ID-04).
func (p ContrasenaPlana) String() string { return "[REDACTADO]" }

// GoString redacta la contraseña en el formato %#v.
func (p ContrasenaPlana) GoString() string { return "[REDACTADO]" }

// MarshalJSON redacta la contraseña si el value object se serializa por
// descuido.
func (p ContrasenaPlana) MarshalJSON() ([]byte, error) {
	return []byte(`"[REDACTADO]"`), nil
}

// --- errores de construcción propios de este archivo ------------------------
// No forman parte de la tabla 1.5 (errores.go): son errores de validación
// de value object, más finos que los de negocio.

// ErrHashContrasenaInvalido se produce al construir un HashContrasena con
// formato irreconocible.
type ErrHashContrasenaInvalido struct{ Motivo string }

func (e *ErrHashContrasenaInvalido) Error() string {
	return "hash de contraseña inválido: " + e.Motivo
}

// ErrCredencialInvalida se produce al construir una Credencial sin hash.
type ErrCredencialInvalida struct{ Motivo string }

func (e *ErrCredencialInvalida) Error() string { return "credencial inválida: " + e.Motivo }

// ErrContrasenaPlanaInvalida se produce por restricciones estructurales de
// ContrasenaPlana (vacía o excesivamente larga). Nunca incluye el valor.
type ErrContrasenaPlanaInvalida struct{ Motivo string }

func (e *ErrContrasenaPlanaInvalida) Error() string { return "contraseña inválida: " + e.Motivo }
