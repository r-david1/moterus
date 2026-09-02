package dominio

import "time"

// ReclamacionesAcceso es el value object que representa lo que el dominio
// decide que debe decir el token de acceso (perfil RFC 9068, `typ: at+jwt`;
// tabla del §7 del diseño). El dominio decide QUÉ va en el token; el
// adaptador FirmadorTokensAcceso decide CÓMO se codifica y con qué llave, y
// lo serializa a JWT compacto. Deliberadamente no lleva roles, permisos,
// org_id/tenant_id ni PII (correo, IP, huella de dispositivo) — INV-ACC-12.
type ReclamacionesAcceso struct {
	emisor               string
	sujeto               IDUsuario
	audiencia            string
	idSesion             IDSesion
	jti                  IDTokenAcceso
	metodosAutenticacion []string
	autenticadoEn        time.Time
	emitidoEn            time.Time
	expiraEn             time.Time
	version              int
}

// NuevasReclamacionesAcceso valida y construye un ReclamacionesAcceso.
// Exige emisor, audiencia, sujeto, idSesion, jti y al menos un método de
// autenticación presentes, y expiraEn estrictamente posterior a emitidoEn.
func NuevasReclamacionesAcceso(
	emisor string,
	sujeto IDUsuario,
	audiencia string,
	idSesion IDSesion,
	jti IDTokenAcceso,
	metodosAutenticacion []string,
	autenticadoEn time.Time,
	emitidoEn time.Time,
	expiraEn time.Time,
	version int,
) (ReclamacionesAcceso, error) {
	if emisor == "" {
		return ReclamacionesAcceso{}, &ErrReclamacionesAccesoInvalidas{Motivo: "el emisor no puede estar vacío"}
	}
	if sujeto.EsVacio() {
		return ReclamacionesAcceso{}, &ErrReclamacionesAccesoInvalidas{Motivo: "el sujeto no puede estar vacío"}
	}
	if audiencia == "" {
		return ReclamacionesAcceso{}, &ErrReclamacionesAccesoInvalidas{Motivo: "la audiencia no puede estar vacía"}
	}
	if idSesion.EsVacio() {
		return ReclamacionesAcceso{}, &ErrReclamacionesAccesoInvalidas{Motivo: "el id de sesión no puede estar vacío"}
	}
	if jti.EsVacio() {
		return ReclamacionesAcceso{}, &ErrReclamacionesAccesoInvalidas{Motivo: "el jti no puede estar vacío"}
	}
	if len(metodosAutenticacion) == 0 {
		return ReclamacionesAcceso{}, &ErrReclamacionesAccesoInvalidas{Motivo: "debe haber al menos un método de autenticación"}
	}
	if !expiraEn.After(emitidoEn) {
		return ReclamacionesAcceso{}, &ErrReclamacionesAccesoInvalidas{Motivo: "expiraEn debe ser posterior a emitidoEn"}
	}
	if version < 1 {
		return ReclamacionesAcceso{}, &ErrReclamacionesAccesoInvalidas{Motivo: "la versión debe ser al menos 1"}
	}
	amr := make([]string, len(metodosAutenticacion))
	copy(amr, metodosAutenticacion)
	return ReclamacionesAcceso{
		emisor:               emisor,
		sujeto:               sujeto,
		audiencia:            audiencia,
		idSesion:             idSesion,
		jti:                  jti,
		metodosAutenticacion: amr,
		autenticadoEn:        autenticadoEn,
		emitidoEn:            emitidoEn,
		expiraEn:             expiraEn,
		version:              version,
	}, nil
}

// Emisor devuelve el claim `iss`.
func (r ReclamacionesAcceso) Emisor() string { return r.emisor }

// Sujeto devuelve el claim `sub`.
func (r ReclamacionesAcceso) Sujeto() IDUsuario { return r.sujeto }

// Audiencia devuelve el claim `aud`.
func (r ReclamacionesAcceso) Audiencia() string { return r.audiencia }

// IDSesion devuelve el claim `sid`: la clave de revocación y de correlación
// con la tabla sesiones.
func (r ReclamacionesAcceso) IDSesion() IDSesion { return r.idSesion }

// JTI devuelve el claim `jti`.
func (r ReclamacionesAcceso) JTI() IDTokenAcceso { return r.jti }

// MetodosAutenticacion devuelve una copia del claim `amr`.
func (r ReclamacionesAcceso) MetodosAutenticacion() []string {
	amr := make([]string, len(r.metodosAutenticacion))
	copy(amr, r.metodosAutenticacion)
	return amr
}

// AutenticadoEn devuelve el claim `auth_time`: el instante de la
// autenticación original de la sesión (no se actualiza en cada renovación).
func (r ReclamacionesAcceso) AutenticadoEn() time.Time { return r.autenticadoEn }

// EmitidoEn devuelve el claim `iat`.
func (r ReclamacionesAcceso) EmitidoEn() time.Time { return r.emitidoEn }

// ExpiraEn devuelve el claim `exp`.
func (r ReclamacionesAcceso) ExpiraEn() time.Time { return r.expiraEn }

// Version devuelve el claim `ver`: la versión del formato de claims.
func (r ReclamacionesAcceso) Version() int { return r.version }

// ErrReclamacionesAccesoInvalidas se produce al construir un
// ReclamacionesAcceso con campos obligatorios ausentes o con una ventana
// temporal incoherente.
type ErrReclamacionesAccesoInvalidas struct{ Motivo string }

func (e *ErrReclamacionesAccesoInvalidas) Error() string {
	return "reclamaciones de acceso inválidas: " + e.Motivo
}
