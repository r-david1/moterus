package jwt

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jws"
	"github.com/lestrrat-go/jwx/v2/jwt"

	"github.com/r-david1/moterus/internal/acceso/dominio"
	"github.com/r-david1/moterus/internal/acceso/puertos"
)

// tipoTokenAcceso es el valor exigido del header `typ` del token de acceso
// (RFC 9068, perfil "access token" — sección 7 del diseño, ADR 0020 §2).
// Cualquier otro valor (incluida su ausencia) se rechaza sin evaluar la
// firma.
const tipoTokenAcceso = "at+jwt"

// claveSID / claveAMR / claveAuthTime / claveVersion son los nombres de los
// claims privados del perfil de token de Acceso (sección 7 del diseño):
// jwx solo reconoce por nombre los claims registrados de RFC 7519
// (iss/sub/aud/exp/iat/nbf/jti); estos cuatro se transportan como claims
// privados con Get/Claim.
const (
	claveSID      = "sid"
	claveAMR      = "amr"
	claveAuthTime = "auth_time"
	claveVersion  = "ver"
)

// Firmador implementa puertos.FirmadorTokensAcceso sobre jwx/v2 (ADR 0020).
type Firmador struct {
	llavero    *Llavero
	emisor     string
	audiencia  string
	tolerancia time.Duration
}

var _ puertos.FirmadorTokensAcceso = (*Firmador)(nil)

// NuevoFirmador construye el adaptador. tolerancia es
// PoliticaSesion.ToleranciaReloj() (60s por defecto, tabla 1.3 del
// diseño): el desfase aceptado al validar exp/nbf.
func NuevoFirmador(llavero *Llavero, emisor, audiencia string, tolerancia time.Duration) *Firmador {
	return &Firmador{llavero: llavero, emisor: emisor, audiencia: audiencia, tolerancia: tolerancia}
}

// Firmar serializa r a un JWT compacto firmado con la llave activa
// (siempre Ed25519, ADR 0020 §1), con typ=at+jwt y kid=thumbprint RFC 7638
// de la llave activa.
func (f *Firmador) Firmar(_ context.Context, r dominio.ReclamacionesAcceso) (string, error) {
	builder := jwt.NewBuilder().
		Issuer(r.Emisor()).
		Subject(r.Sujeto().String()).
		Audience([]string{r.Audiencia()}).
		JwtID(r.JTI().String()).
		IssuedAt(r.EmitidoEn()).
		NotBefore(r.EmitidoEn()).
		Expiration(r.ExpiraEn()).
		Claim(claveSID, r.IDSesion().String()).
		Claim(claveAMR, r.MetodosAutenticacion()).
		Claim(claveAuthTime, r.AutenticadoEn().Unix()).
		Claim(claveVersion, r.Version())

	tok, err := builder.Build()
	if err != nil {
		return "", fmt.Errorf("jwt: no se pudo construir el token de acceso: %w", err)
	}

	cabeceras := jws.NewHeaders()
	if err := cabeceras.Set(jws.TypeKey, tipoTokenAcceso); err != nil {
		return "", fmt.Errorf("jwt: no se pudo fijar typ: %w", err)
	}
	if err := cabeceras.Set(jws.KeyIDKey, f.llavero.activa.kid); err != nil {
		return "", fmt.Errorf("jwt: no se pudo fijar kid: %w", err)
	}

	firmado, err := jwt.Sign(tok, jwt.WithKey(jwa.EdDSA, f.llavero.activa.privada, jws.WithProtectedHeaders(cabeceras)))
	if err != nil {
		return "", fmt.Errorf("jwt: no se pudo firmar el token de acceso: %w", err)
	}
	return string(firmado), nil
}

// Verificar implementa el flujo normativo de la sección 3.3 del diseño y
// las verificaciones obligatorias de ADR 0020 §2, en este orden:
//
//  1. Estructura del JWS (rechazo genérico si no es ni siquiera un JWS
//     bien formado).
//  2. `alg: none` rechazado explícitamente.
//  3. `typ` debe ser exactamente "at+jwt".
//  4. `kid` presente y conocido en el conjunto de verificación (activa +
//     previas). El algoritmo de verificación usado es el que trae la
//     LLAVE encontrada por kid (fijado por nuevaLlaveActiva:
//     jwa.EdDSA/jwk.AlgorithmKey), nunca el `alg` que declara la cabecera
//     del token (defensa contra algorithm confusion, INV-ACC-14).
//  5. Firma, `iss`, `aud`, `exp`/`nbf` con tolerancia de reloj.
//
// Un `exp` vencido (con tolerancia ya aplicada) produce
// *dominio.ErrTokenAccesoExpirado; cualquier otro motivo produce
// *dominio.ErrTokenAccesoInvalido{Motivo} del catálogo cerrado.
func (f *Firmador) Verificar(_ context.Context, tokenCompacto string) (dominio.ReclamacionesAcceso, error) {
	crudo := []byte(tokenCompacto)

	mensaje, err := jws.Parse(crudo)
	if err != nil || len(mensaje.Signatures()) != 1 {
		return dominio.ReclamacionesAcceso{}, &dominio.ErrTokenAccesoInvalido{Motivo: dominio.MotivoTokenAccesoEstructuraCorrupta}
	}
	cabecera := mensaje.Signatures()[0].ProtectedHeaders()

	alg := cabecera.Algorithm()
	if alg == "" || alg == jwa.NoSignature {
		return dominio.ReclamacionesAcceso{}, &dominio.ErrTokenAccesoInvalido{Motivo: dominio.MotivoTokenAccesoAlgoritmoInesperado}
	}
	if cabecera.Type() != tipoTokenAcceso {
		return dominio.ReclamacionesAcceso{}, &dominio.ErrTokenAccesoInvalido{Motivo: dominio.MotivoTokenAccesoTipoIncorrecto}
	}
	kid := cabecera.KeyID()
	if kid == "" {
		return dominio.ReclamacionesAcceso{}, &dominio.ErrTokenAccesoInvalido{Motivo: dominio.MotivoTokenAccesoKIDDesconocido}
	}
	if _, ok := f.llavero.verificacion.LookupKeyID(kid); !ok {
		return dominio.ReclamacionesAcceso{}, &dominio.ErrTokenAccesoInvalido{Motivo: dominio.MotivoTokenAccesoKIDDesconocido}
	}

	tok, err := jwt.Parse(crudo,
		jwt.WithKeySet(f.llavero.verificacion, jws.WithRequireKid(true)),
		jwt.WithValidate(true),
		jwt.WithIssuer(f.emisor),
		jwt.WithAudience(f.audiencia),
		jwt.WithAcceptableSkew(f.tolerancia),
	)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired()) {
			return dominio.ReclamacionesAcceso{}, &dominio.ErrTokenAccesoExpirado{}
		}
		if errors.Is(err, jwt.ErrInvalidIssuer()) {
			return dominio.ReclamacionesAcceso{}, &dominio.ErrTokenAccesoInvalido{Motivo: dominio.MotivoTokenAccesoEmisorInesperado}
		}
		if errors.Is(err, jwt.ErrInvalidAudience()) {
			return dominio.ReclamacionesAcceso{}, &dominio.ErrTokenAccesoInvalido{Motivo: dominio.MotivoTokenAccesoAudienciaInesperada}
		}
		return dominio.ReclamacionesAcceso{}, &dominio.ErrTokenAccesoInvalido{Motivo: dominio.MotivoTokenAccesoFirmaInvalida}
	}

	return reclamacionesDesdeToken(tok)
}

// LlavesPublicas devuelve el conjunto completo de llaves públicas de
// verificación (activa + previas), insumo del endpoint JWKS.
func (f *Firmador) LlavesPublicas(_ context.Context) ([]puertos.LlavePublica, error) {
	llaves := make([]puertos.LlavePublica, 0, f.llavero.verificacion.Len())
	for i := range f.llavero.verificacion.Len() {
		clave, ok := f.llavero.verificacion.Key(i)
		if !ok {
			continue
		}
		lp, err := llavePublicaDesdeJWK(clave)
		if err != nil {
			return nil, err
		}
		llaves = append(llaves, lp)
	}
	return llaves, nil
}

// llavePublicaDesdeJWK proyecta un jwk.Key (siempre Ed25519/OKP en este
// MVP, ver llavero.go) al tipo de puerto puertos.LlavePublica.
func llavePublicaDesdeJWK(clave jwk.Key) (puertos.LlavePublica, error) {
	var pub ed25519.PublicKey
	if err := clave.Raw(&pub); err != nil {
		return puertos.LlavePublica{}, fmt.Errorf("jwt: no se pudo extraer el material público de la llave: %w", err)
	}
	kid, _ := clave.Get(jwk.KeyIDKey)
	kidStr, _ := kid.(string)
	return puertos.LlavePublica{
		KID:       kidStr,
		TipoLlave: "OKP",
		Curva:     "Ed25519",
		Algoritmo: "EdDSA",
		Material:  []byte(pub),
	}, nil
}

// reclamacionesDesdeToken traduce un jwt.Token ya verificado y validado al
// value object de dominio ReclamacionesAcceso.
func reclamacionesDesdeToken(tok jwt.Token) (dominio.ReclamacionesAcceso, error) {
	sujeto, err := dominio.IDUsuarioDesde(tok.Subject())
	if err != nil {
		return dominio.ReclamacionesAcceso{}, &dominio.ErrTokenAccesoInvalido{Motivo: dominio.MotivoTokenAccesoEstructuraCorrupta}
	}

	sidCrudo, ok := tok.Get(claveSID)
	if !ok {
		return dominio.ReclamacionesAcceso{}, &dominio.ErrTokenAccesoInvalido{Motivo: dominio.MotivoTokenAccesoEstructuraCorrupta}
	}
	sidStr, ok := sidCrudo.(string)
	if !ok {
		return dominio.ReclamacionesAcceso{}, &dominio.ErrTokenAccesoInvalido{Motivo: dominio.MotivoTokenAccesoEstructuraCorrupta}
	}
	idSesion, err := dominio.IDSesionDesde(sidStr)
	if err != nil {
		return dominio.ReclamacionesAcceso{}, &dominio.ErrTokenAccesoInvalido{Motivo: dominio.MotivoTokenAccesoEstructuraCorrupta}
	}

	jti, err := dominio.IDTokenAccesoDesde(tok.JwtID())
	if err != nil {
		return dominio.ReclamacionesAcceso{}, &dominio.ErrTokenAccesoInvalido{Motivo: dominio.MotivoTokenAccesoEstructuraCorrupta}
	}

	amr := extraerAMR(tok)
	authTime := extraerAuthTime(tok)
	version := extraerVersion(tok)

	audiencia := ""
	if aud := tok.Audience(); len(aud) > 0 {
		audiencia = aud[0]
	}

	return dominio.NuevasReclamacionesAcceso(
		tok.Issuer(),
		sujeto,
		audiencia,
		idSesion,
		jti,
		amr,
		authTime,
		tok.IssuedAt(),
		tok.Expiration(),
		version,
	)
}

func extraerAMR(tok jwt.Token) []string {
	crudo, ok := tok.Get(claveAMR)
	if !ok {
		return nil
	}
	switch v := crudo.(type) {
	case []string:
		return v
	case []interface{}:
		amr := make([]string, 0, len(v))
		for _, elem := range v {
			if s, ok := elem.(string); ok {
				amr = append(amr, s)
			}
		}
		return amr
	default:
		return nil
	}
}

func extraerAuthTime(tok jwt.Token) time.Time {
	crudo, ok := tok.Get(claveAuthTime)
	if !ok {
		return time.Time{}
	}
	switch v := crudo.(type) {
	case float64:
		return time.Unix(int64(v), 0).UTC()
	case int64:
		return time.Unix(v, 0).UTC()
	case json.Number:
		n, err := v.Int64()
		if err != nil {
			return time.Time{}
		}
		return time.Unix(n, 0).UTC()
	default:
		return time.Time{}
	}
}

func extraerVersion(tok jwt.Token) int {
	crudo, ok := tok.Get(claveVersion)
	if !ok {
		return 0
	}
	switch v := crudo.(type) {
	case float64:
		return int(v)
	case int64:
		return int(v)
	case json.Number:
		n, err := v.Int64()
		if err != nil {
			return 0
		}
		return int(n)
	default:
		return 0
	}
}
