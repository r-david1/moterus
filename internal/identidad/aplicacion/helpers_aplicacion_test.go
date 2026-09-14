package aplicacion_test

// Fixtures compartidos entre los tests de los tres casos de uso de
// aplicacion. Viven en package aplicacion_test (caja negra): los tests solo
// ejercen la API pública de aplicacion/puertos/dominio, tal como lo haría un
// consumidor real (p. ej. el contexto Acceso).

import (
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // RFC 6238 exige SHA-1; mismo motivo que dominio/totp.go.
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/r-david1/moterus/internal/identidad/dominio"
)

// contrasenaFuerteValor es una contraseña que cumple PoliticaContrasena
// (>= 12 caracteres, sin reglas de composición, sin secuencias triviales, no
// contiene el correo/dominio de correoValidoValor) — el mismo fixture que
// usa dominio/politica_contrasena_test.go.
const contrasenaFuerteValor = "correcto caballo batería grapa"

// correoValidoValor es el correo de prueba por defecto usado en los tests
// de aplicacion. No debe cambiarse sin revisar que contrasenaFuerteValor
// sigue sin contenerlo.
const correoValidoValor = "ana@ejemplo.com"

func correoDePrueba(t *testing.T, crudo string) dominio.Correo {
	t.Helper()
	c, err := dominio.NuevoCorreo(crudo)
	if err != nil {
		t.Fatalf("no se pudo construir el correo de prueba %q: %v", crudo, err)
	}
	return c
}

func hashDePrueba(t *testing.T, sufijo string) dominio.HashContrasena {
	t.Helper()
	h, err := dominio.NuevoHashContrasena("$argon2id$v=19$m=65536,t=3,p=1$sal$" + sufijo)
	if err != nil {
		t.Fatalf("no se pudo construir el hash de prueba: %v", err)
	}
	return h
}

func idDePrueba(t *testing.T, uuid string) dominio.IDUsuario {
	t.Helper()
	id, err := dominio.IDUsuarioDesde(uuid)
	if err != nil {
		t.Fatalf("no se pudo construir el IDUsuario de prueba %q: %v", uuid, err)
	}
	return id
}

// idUsuarioValido1/2 son dos UUIDs sintácticamente válidos y distintos, para
// distinguir "el usuario correcto" de "otro usuario" en los tests de
// auditoría condicional.
const (
	idUsuarioValido1 = "123e4567-e89b-12d3-a456-426614174000"
	idUsuarioValido2 = "223e4567-e89b-12d3-a456-426614174001"
)

func origenDePrueba(t *testing.T) dominio.OrigenSolicitud {
	t.Helper()
	o, err := dominio.NuevoOrigenSolicitud("203.0.113.5", "agente-test/1.0", "huella-test", "req-123")
	if err != nil {
		t.Fatalf("no se pudo construir el origen de prueba: %v", err)
	}
	return o
}

// ahoraDePrueba es una hora fija y determinista para todos los tests.
func ahoraDePrueba() time.Time {
	return time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
}

// usuarioPendienteDePrueba construye un Usuario recién registrado
// (pendiente_verificacion) y drena sus eventos pendientes, para que los
// tests que lo usan como fixture de entrada (p. ej. vía
// RepositorioUsuarios.FnBuscarPorCorreo) no arrastren el evento
// UsuarioRegistrado de su propia construcción.
func usuarioPendienteDePrueba(t *testing.T, id dominio.IDUsuario, correo dominio.Correo, hash dominio.HashContrasena) *dominio.Usuario {
	t.Helper()
	u, err := dominio.RegistrarUsuario(id, correo, hash, ahoraDePrueba())
	if err != nil {
		t.Fatalf("no se pudo construir el usuario de prueba: %v", err)
	}
	u.EventosPendientes() // drenar
	return u
}

// usuarioActivoDePrueba construye un Usuario en estado activo (correo ya
// confirmado) con la contraseña hasheada indicada, y drena sus eventos
// pendientes.
func usuarioActivoDePrueba(t *testing.T, id dominio.IDUsuario, correo dominio.Correo, hash dominio.HashContrasena) *dominio.Usuario {
	t.Helper()
	u := usuarioPendienteDePrueba(t, id, correo, hash)
	if err := u.ConfirmarCorreo(ahoraDePrueba()); err != nil {
		t.Fatalf("no se pudo confirmar el correo del usuario de prueba: %v", err)
	}
	u.EventosPendientes() // drenar
	return u
}

// usuarioSuspendidoDePrueba construye un Usuario activo y lo suspende.
func usuarioSuspendidoDePrueba(t *testing.T, id dominio.IDUsuario, correo dominio.Correo, hash dominio.HashContrasena) *dominio.Usuario {
	t.Helper()
	u := usuarioActivoDePrueba(t, id, correo, hash)
	motivo, err := dominio.NuevoMotivoCambioEstado("comportamiento sospechoso reportado por soporte")
	if err != nil {
		t.Fatalf("no se pudo construir el motivo de prueba: %v", err)
	}
	if err := u.Suspender(motivo, ahoraDePrueba()); err != nil {
		t.Fatalf("no se pudo suspender el usuario de prueba: %v", err)
	}
	u.EventosPendientes()
	return u
}

// usuarioBloqueadoDePrueba construye un Usuario pendiente de verificación y
// lo bloquea.
func usuarioBloqueadoDePrueba(t *testing.T, id dominio.IDUsuario, correo dominio.Correo, hash dominio.HashContrasena) *dominio.Usuario {
	t.Helper()
	u := usuarioPendienteDePrueba(t, id, correo, hash)
	motivo, err := dominio.NuevoMotivoCambioEstado("fraude confirmado por el equipo de riesgo")
	if err != nil {
		t.Fatalf("no se pudo construir el motivo de prueba: %v", err)
	}
	if err := u.Bloquear(motivo, ahoraDePrueba()); err != nil {
		t.Fatalf("no se pudo bloquear el usuario de prueba: %v", err)
	}
	u.EventosPendientes()
	return u
}

// usuarioConMFADePrueba construye, vía dominio.Reconstituir, un Usuario
// activo con tieneMFA=true. Es la única forma de obtener un usuario con MFA
// habilitado sin el agregado FactorMFA (fase 2, fuera de alcance): el
// dominio no expone ningún setter de negocio para activar tieneMFA sin
// confirmar un factor.
func usuarioConMFADePrueba(t *testing.T, id dominio.IDUsuario, correo dominio.Correo, hash dominio.HashContrasena) *dominio.Usuario {
	t.Helper()
	credencial, err := dominio.NuevaCredencial(hash, ahoraDePrueba())
	if err != nil {
		t.Fatalf("no se pudo construir la credencial de prueba: %v", err)
	}
	return dominio.Reconstituir(id, correo, credencial, dominio.EstadoActivo, true, ahoraDePrueba(), ahoraDePrueba(), nil)
}

// --- Fixtures de MFA/OTP (docs/design/otp-mfa.md) --------------------------
//
// idFactorValido1/2 son dos UUIDs sintácticamente válidos y distintos, mismo
// criterio que idUsuarioValido1/2.
const (
	idFactorValido1 = "018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c99"
	idFactorValido2 = "018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c98"
)

// secretoBase32DePrueba es un secreto TOTP en claro con forma válida (32
// caracteres Base32, ADR 0037/§1.4 del diseño): 160 bits de entropía no son
// necesarios para el test, solo la forma estructural que exige
// dominio.NuevoSecretoTOTPPlano.
const secretoBase32DePrueba = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

func idFactorDePrueba(t *testing.T, uuid string) dominio.IDFactorMFA {
	t.Helper()
	id, err := dominio.IDFactorMFADesde(uuid)
	if err != nil {
		t.Fatalf("no se pudo construir el IDFactorMFA de prueba %q: %v", uuid, err)
	}
	return id
}

// secretoCifradoDePrueba envuelve secretoBase32DePrueba como
// SecretoTOTPCifrado. Coincide con el comportamiento por defecto de
// mocks.CifradorSecretos (Cifrar/Descifrar hacen un roundtrip sin
// transformar el valor), así que Descifrar(secretoCifradoDePrueba(t))
// devuelve de vuelta secretoBase32DePrueba.
func secretoCifradoDePrueba(t *testing.T) dominio.SecretoTOTPCifrado {
	t.Helper()
	c, err := dominio.NuevoSecretoTOTPCifrado([]byte(secretoBase32DePrueba))
	if err != nil {
		t.Fatalf("no se pudo construir el secreto cifrado de prueba: %v", err)
	}
	return c
}

// codigoTOTPValidoDePrueba calcula, con el mismo algoritmo que
// dominio.VerificarCodigo (RFC 6238/RFC 4226: HMAC-SHA1, paso de 30s, 6
// dígitos), el código TOTP correcto para secretoBase32DePrueba en el
// instante ahora. Se reimplementa aquí (en lugar de importar un símbolo no
// exportado de dominio) porque este paquete de test es de caja negra
// (aplicacion_test) y solo puede depender de la API pública.
func codigoTOTPValidoDePrueba(t *testing.T, ahora time.Time) string {
	t.Helper()
	limpio := strings.ToUpper(strings.TrimRight(secretoBase32DePrueba, "="))
	clave, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(limpio)
	if err != nil {
		t.Fatalf("no se pudo decodificar el secreto de prueba: %v", err)
	}
	contador := uint64(ahora.Unix() / 30) //nolint:gosec // instante de prueba siempre posterior a 1970; nunca negativo.
	var contadorBytes [8]byte
	binary.BigEndian.PutUint64(contadorBytes[:], contador)
	mac := hmac.New(sha1.New, clave)
	mac.Write(contadorBytes[:])
	suma := mac.Sum(nil)
	offset := suma[len(suma)-1] & 0x0f
	codigoBinario := (uint32(suma[offset]&0x7f) << 24) |
		(uint32(suma[offset+1]) << 16) |
		(uint32(suma[offset+2]) << 8) |
		uint32(suma[offset+3])
	return fmt.Sprintf("%06d", codigoBinario%1000000)
}

// factorSinConfirmarDePrueba construye, vía dominio.HabilitarFactorMFA, un
// FactorMFA recién habilitado (sin confirmar) con secretoCifradoDePrueba, y
// drena su evento FactorMFAHabilitado.
func factorSinConfirmarDePrueba(t *testing.T, idFactor dominio.IDFactorMFA, idUsuario dominio.IDUsuario) *dominio.FactorMFA {
	t.Helper()
	f, err := dominio.HabilitarFactorMFA(idFactor, idUsuario, dominio.TipoFactorTOTP, secretoCifradoDePrueba(t), ahoraDePrueba())
	if err != nil {
		t.Fatalf("no se pudo construir el factor de prueba: %v", err)
	}
	f.EventosPendientes() // drenar
	return f
}

// codigoRespaldoPlanoDePrueba es un código de respaldo con forma válida
// (10 caracteres del alfabeto restringido de dominio.CodigoRespaldoPlano:
// sin 0/O/1/I/L).
const codigoRespaldoPlanoDePrueba = "ABCDEFGH23"

// codigoRespaldoDisponibleDePrueba construye un dominio.CodigoRespaldoMFA
// disponible cuyo hash corresponde a codigoRespaldoPlanoDePrueba, para
// fixtures de factores confirmados con códigos de respaldo.
func codigoRespaldoDisponibleDePrueba(t *testing.T) dominio.CodigoRespaldoMFA {
	t.Helper()
	plano, err := dominio.NuevoCodigoRespaldoPlano(codigoRespaldoPlanoDePrueba)
	if err != nil {
		t.Fatalf("no se pudo construir el código de respaldo de prueba: %v", err)
	}
	return dominio.ReconstituirCodigoRespaldoMFA(plano.Hash(), nil)
}

// codigosRespaldoValidosDePrueba genera n dominio.CodigoRespaldoPlano con
// forma válida. Se usa como puertos.GeneradorSecretoTOTP.FnGenerarCodigosRespaldo
// en lugar del comportamiento por defecto de mocks.GeneradorSecretoTOTP, que
// produce códigos con '0'/'1' (fuera del alfabeto restringido de
// dominio.CodigoRespaldoPlano) y siempre falla.
func codigosRespaldoValidosDePrueba(n int) ([]dominio.CodigoRespaldoPlano, error) {
	// Alfabeto sin 0/O/1/I/L, mismo criterio que dominio.CodigoRespaldoPlano.
	const sufijos = "23456789JK"
	codigos := make([]dominio.CodigoRespaldoPlano, 0, n)
	for i := 0; i < n; i++ {
		valor := "ABCDEFGH" + string(sufijos[i%len(sufijos)]) + string(sufijos[(i/len(sufijos))%len(sufijos)])
		c, err := dominio.NuevoCodigoRespaldoPlano(valor)
		if err != nil {
			return nil, err
		}
		codigos = append(codigos, c)
	}
	return codigos, nil
}

// factorConfirmadoDePrueba construye, vía dominio.ReconstituirFactorMFA, un
// FactorMFA ya confirmado con secretoCifradoDePrueba y los códigos de
// respaldo indicados (puede ir vacío).
func factorConfirmadoDePrueba(t *testing.T, idFactor dominio.IDFactorMFA, idUsuario dominio.IDUsuario, codigosRespaldo []dominio.CodigoRespaldoMFA) *dominio.FactorMFA {
	t.Helper()
	confirmadoEn := ahoraDePrueba()
	return dominio.ReconstituirFactorMFA(
		idFactor, idUsuario, dominio.TipoFactorTOTP, secretoCifradoDePrueba(t),
		true, true, ahoraDePrueba(), &confirmadoEn, codigosRespaldo,
	)
}
