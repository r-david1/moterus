package cripto

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"math/big"

	"github.com/r-david1/moterus/internal/identidad/dominio"
	"github.com/r-david1/moterus/internal/identidad/puertos"
)

// bytesSecretoTOTP es la cantidad de bytes de entropía del secreto TOTP:
// 160 bits (§1.4 del diseño otp-mfa.md — RFC 4226 recomienda ≥128 bits;
// 160 es el tamaño nativo de salida de SHA-1, el algoritmo que usa TOTP
// clásico).
const bytesSecretoTOTP = 20 // 160 bits / 8

// longitudCodigoRespaldoGenerado es la longitud del código de respaldo que
// este generador produce (dominio.CodigoRespaldoPlano exige exactamente 10
// caracteres, §1.4 del diseño / ADR 0040).
const longitudCodigoRespaldoGenerado = 10

// alfabetoCodigoRespaldoGenerado es EXACTAMENTE el alfabeto que
// dominio.NuevoCodigoRespaldoPlano acepta (mayúsculas + dígitos, sin
// 0/O/1/I/L): generar fuera de este alfabeto produciría un
// ErrCodigoRespaldoPlanoInvalido en tiempo de confirmación.
const alfabetoCodigoRespaldoGenerado = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

// GeneradorTOTP implementa puertos.GeneradorSecretoTOTP vía crypto/rand: el
// dominio nunca genera bytes aleatorios por sí mismo (§2.2 del diseño
// otp-mfa.md).
type GeneradorTOTP struct{}

var _ puertos.GeneradorSecretoTOTP = (*GeneradorTOTP)(nil)

// NuevoGeneradorTOTP construye el adaptador.
func NuevoGeneradorTOTP() *GeneradorTOTP { return &GeneradorTOTP{} }

// GenerarSecreto produce 160 bits de crypto/rand codificados en Base32 sin
// relleno (mayúsculas) — exactamente el formato que
// dominio.NuevoSecretoTOTPPlano exige.
func (GeneradorTOTP) GenerarSecreto() (dominio.SecretoTOTPPlano, error) {
	buf := make([]byte, bytesSecretoTOTP)
	if _, err := rand.Read(buf); err != nil {
		return dominio.SecretoTOTPPlano{}, fmt.Errorf("cripto: no se pudo generar el secreto TOTP: %w", err)
	}
	codificado := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf)
	return dominio.NuevoSecretoTOTPPlano(codificado)
}

// GenerarCodigosRespaldo produce n códigos de respaldo de un solo uso
// (ADR 0040: n=10 en la práctica, fijado por el caso de uso
// ConfirmarFactorMFA), cada uno de 10 caracteres del alfabeto restringido
// sin caracteres ambiguos, vía crypto/rand (rechazo por muestreo/módulo
// sobre big.Int para no introducir sesgo de módulo con un alfabeto de 32
// símbolos que no es potencia de 2... en realidad 32 SÍ es potencia de 2,
// así que un byte aleatorio & 0x1F ya es uniforme; se usa
// crypto/rand.Int con un límite exacto de len(alfabeto) para que el
// código siga siendo correcto aunque el alfabeto cambie de tamaño en el
// futuro).
func (GeneradorTOTP) GenerarCodigosRespaldo(n int) ([]dominio.CodigoRespaldoPlano, error) {
	if n <= 0 {
		return nil, fmt.Errorf("cripto: la cantidad de códigos de respaldo a generar debe ser positiva, recibido %d", n)
	}
	limite := big.NewInt(int64(len(alfabetoCodigoRespaldoGenerado)))
	codigos := make([]dominio.CodigoRespaldoPlano, 0, n)
	for i := 0; i < n; i++ {
		buf := make([]byte, longitudCodigoRespaldoGenerado)
		for j := range buf {
			idx, err := rand.Int(rand.Reader, limite)
			if err != nil {
				return nil, fmt.Errorf("cripto: no se pudo generar un código de respaldo: %w", err)
			}
			buf[j] = alfabetoCodigoRespaldoGenerado[idx.Int64()]
		}
		codigo, err := dominio.NuevoCodigoRespaldoPlano(string(buf))
		if err != nil {
			return nil, fmt.Errorf("cripto: código de respaldo generado inválido (bug en el generador): %w", err)
		}
		codigos = append(codigos, codigo)
	}
	return codigos, nil
}
