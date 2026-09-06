package jwt

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jws"
	"github.com/lestrrat-go/jwx/v2/jwt"

	"github.com/r-david1/moterus/internal/acceso/dominio"
	"github.com/r-david1/moterus/internal/acceso/puertos"
)

// tipoTokenStepUp es el valor exigido del header `typ` del token de
// step-up (ADR 0038): deliberadamente distinto de tipoTokenAcceso
// ("at+jwt", firmador.go). Es la pieza central de INV-MFA-03: un token de
// acceso normal, aunque esté firmado con la misma llave y sea perfectamente
// válido como tal, tiene typ="at+jwt" y por lo tanto SIEMPRE es rechazado
// por EmisorTokenStepUp.Validar — y, simétricamente, un token de step-up
// SIEMPRE es rechazado por Firmador.Verificar (que exige typ=="at+jwt"),
// sin que ninguno de los dos validadores necesite conocer al otro. Ningún
// endpoint del sistema salvo POST /acceso/sesiones/segundo-factor debe
// aceptar este typ (ADR 0038, Consecuencias).
const tipoTokenStepUp = "step-up+jwt"

// claveMotivoStepUp es el nombre del único claim privado del token de
// step-up (ADR 0038: "claims mínimos, sub + el motivo del step-up, nunca
// un claim de autorización ni PII").
const claveMotivoStepUp = "motivo_step_up"

// vidaTokenStepUp es el TTL fijo del token de step-up (ADR 0038, INV-MFA-04):
// 5 minutos, la mitad del token de acceso normal (10 min, ADR 0019), sin
// refresco posible. No es configurable: es una decisión de seguridad ya
// cerrada por el ADR, no un parámetro operativo.
const vidaTokenStepUp = 5 * time.Minute

// EmisorTokenStepUp implementa puertos.EmisorTokenStepUp (ADR 0038) sobre
// el MISMO Llavero que Firmador (ADR 0020): ni una llave nueva ni un JWKS
// nuevo, solo un `typ` de cabecera distinto — el propio ADR 0038 lo fija
// así explícitamente ("Mismo firmador, mismo Llavero").
type EmisorTokenStepUp struct {
	llavero    *Llavero
	tolerancia time.Duration
}

var _ puertos.EmisorTokenStepUp = (*EmisorTokenStepUp)(nil)

// NuevoEmisorTokenStepUp construye el adaptador sobre un Llavero ya
// ensamblado (el mismo que usa NuevoFirmador). tolerancia es la misma
// PoliticaSesion.ToleranciaReloj() que ya usa Firmador: el desfase de reloj
// aceptado al validar `exp`, no un valor de configuración nuevo.
func NuevoEmisorTokenStepUp(llavero *Llavero, tolerancia time.Duration) *EmisorTokenStepUp {
	return &EmisorTokenStepUp{llavero: llavero, tolerancia: tolerancia}
}

// Emitir firma un JWT compacto con sub=idUsuario, el claim privado
// motivo_step_up, typ=step-up+jwt (nunca at+jwt) y una expiración de 5
// minutos desde ahora (INV-MFA-04, sin campo `aud`/`iss`: ADR 0038 solo
// exige sub + motivo, y este token no cruza a otro emisor/audiencia que
// necesite verificarlos — el único consumidor es
// POST /acceso/sesiones/segundo-factor, en el mismo proceso que lo emitió).
func (e *EmisorTokenStepUp) Emitir(_ context.Context, idUsuario string, motivoStepUp string, ahora time.Time) (puertos.TokenStepUp, error) {
	if _, err := dominio.IDUsuarioDesde(idUsuario); err != nil {
		return puertos.TokenStepUp{}, fmt.Errorf("jwt: idUsuario inválido para token de step-up: %w", err)
	}

	builder := jwt.NewBuilder().
		Subject(idUsuario).
		IssuedAt(ahora).
		NotBefore(ahora).
		Expiration(ahora.Add(vidaTokenStepUp)).
		Claim(claveMotivoStepUp, motivoStepUp)

	tok, err := builder.Build()
	if err != nil {
		return puertos.TokenStepUp{}, fmt.Errorf("jwt: no se pudo construir el token de step-up: %w", err)
	}

	cabeceras := jws.NewHeaders()
	if err := cabeceras.Set(jws.TypeKey, tipoTokenStepUp); err != nil {
		return puertos.TokenStepUp{}, fmt.Errorf("jwt: no se pudo fijar typ del token de step-up: %w", err)
	}
	if err := cabeceras.Set(jws.KeyIDKey, e.llavero.activa.kid); err != nil {
		return puertos.TokenStepUp{}, fmt.Errorf("jwt: no se pudo fijar kid del token de step-up: %w", err)
	}

	firmado, err := jwt.Sign(tok, jwt.WithKey(jwa.EdDSA, e.llavero.activa.privada, jws.WithProtectedHeaders(cabeceras)))
	if err != nil {
		return puertos.TokenStepUp{}, fmt.Errorf("jwt: no se pudo firmar el token de step-up: %w", err)
	}

	return puertos.NuevoTokenStepUp(string(firmado)), nil
}

// Validar verifica, EN ESTE ORDEN (mismo criterio defensivo que
// Firmador.Verificar, INV-ACC-14/INV-MFA-03):
//  1. Estructura del JWS.
//  2. `alg: none` rechazado.
//  3. `typ` debe ser EXACTAMENTE "step-up+jwt" — un token de acceso normal
//     (typ="at+jwt"), aunque esté firmado con la misma llave y sea
//     perfectamente válido como token de acceso, se rechaza aquí sin
//     evaluar nada más. Es la defensa central de INV-MFA-03.
//  4. `kid` presente y conocido en el conjunto de verificación del mismo
//     Llavero (activa + previas) — el algoritmo de verificación lo fija la
//     llave encontrada por kid, nunca el `alg` de la cabecera.
//  5. Firma y expiración (con tolerancia de reloj); sin `iss`/`aud`
//     (Emitir no los fija).
//
// Un único error genérico envuelve cualquier motivo de rechazo: el
// llamador (CompletarSegundoFactorCasoDeUso) ya lo colapsa en
// ErrCredencialesRechazadas sin distinguir "expirado" de "typ incorrecto"
// de "firma inválida" (INV-MFA-08).
func (e *EmisorTokenStepUp) Validar(_ context.Context, tokenCompacto string) (puertos.ClaimsStepUp, error) {
	crudo := []byte(tokenCompacto)

	mensaje, err := jws.Parse(crudo)
	if err != nil || len(mensaje.Signatures()) != 1 {
		return puertos.ClaimsStepUp{}, errors.New("jwt: token de step-up con estructura inválida")
	}
	cabecera := mensaje.Signatures()[0].ProtectedHeaders()

	alg := cabecera.Algorithm()
	if alg == "" || alg == jwa.NoSignature {
		return puertos.ClaimsStepUp{}, errors.New("jwt: token de step-up con algoritmo inesperado")
	}
	if cabecera.Type() != tipoTokenStepUp {
		// INV-MFA-03: incluye deliberadamente el caso de un token de acceso
		// normal (typ="at+jwt") válido en todo lo demás — ese es exactamente
		// el ataque de type confusion que este chequeo previene.
		return puertos.ClaimsStepUp{}, errors.New("jwt: typ no es el distintivo de step-up")
	}
	kid := cabecera.KeyID()
	if kid == "" {
		return puertos.ClaimsStepUp{}, errors.New("jwt: token de step-up sin kid")
	}
	if _, ok := e.llavero.verificacion.LookupKeyID(kid); !ok {
		return puertos.ClaimsStepUp{}, errors.New("jwt: kid desconocido en el token de step-up")
	}

	tok, err := jwt.Parse(crudo,
		jwt.WithKeySet(e.llavero.verificacion, jws.WithRequireKid(true)),
		jwt.WithValidate(true),
		jwt.WithAcceptableSkew(e.tolerancia),
	)
	if err != nil {
		return puertos.ClaimsStepUp{}, fmt.Errorf("jwt: token de step-up inválido o expirado: %w", err)
	}

	idUsuario, err := dominio.IDUsuarioDesde(tok.Subject())
	if err != nil {
		return puertos.ClaimsStepUp{}, fmt.Errorf("jwt: sub inválido en el token de step-up: %w", err)
	}

	motivo, _ := tok.Get(claveMotivoStepUp)
	motivoStr, _ := motivo.(string)

	return puertos.ClaimsStepUp{IDUsuario: idUsuario.String(), MotivoStepUp: motivoStr}, nil
}
