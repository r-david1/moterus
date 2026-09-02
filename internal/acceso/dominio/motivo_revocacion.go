package dominio

// MotivoRevocacion es el value object enum **cerrado** que documenta por
// qué se revocó una Sesion. A diferencia de MotivoCambioEstado de
// identidad/dominio (texto libre acotado), aquí el catálogo es cerrado
// (tabla 1.3 del diseño): el motivo alimenta la auditoría y las métricas de
// seguridad, y un texto libre haría imposible responder preguntas como
// "¿cuántas sesiones se revocaron por reuso de refresco este mes?" sin
// parsear texto (INV-ACC-08).
type MotivoRevocacion struct {
	valor string
}

var (
	// MotivoCierreUsuario: logout individual iniciado por el propio usuario.
	MotivoCierreUsuario = MotivoRevocacion{valor: "cierre_usuario"}
	// MotivoCierreMasivoUsuario: logout de todos los dispositivos iniciado
	// por el propio usuario.
	MotivoCierreMasivoUsuario = MotivoRevocacion{valor: "cierre_masivo_usuario"}
	// MotivoReusoRefrescoDetectado: se presentó un token de refresco ya
	// consumido (INV-ACC-06); robo probable.
	MotivoReusoRefrescoDetectado = MotivoRevocacion{valor: "reuso_refresco_detectado"}
	// MotivoCuentaNoOperativa: Identidad reportó que el sujeto ya no está en
	// estado activo (revalidación en cada renovación, §3.2 paso 5).
	MotivoCuentaNoOperativa = MotivoRevocacion{valor: "cuenta_no_operativa"}
	// MotivoContrasenaCambiada: revocación reactiva a un cambio de
	// contraseña en Identidad (§11.1, backlog).
	MotivoContrasenaCambiada = MotivoRevocacion{valor: "contrasena_cambiada"}
	// MotivoLimiteSesionesExcedido: se superó PoliticaSesion.MaximoSesionesActivas
	// y se revocó la sesión más antigua (INV-ACC-22).
	MotivoLimiteSesionesExcedido = MotivoRevocacion{valor: "limite_sesiones_excedido"}
	// MotivoRevocacionAdministrativa: revocación decidida por un operador,
	// sin un motivo más específico en el catálogo.
	MotivoRevocacionAdministrativa = MotivoRevocacion{valor: "revocacion_administrativa"}
)

// MotivoRevocacionDesde valida un valor persistido contra el catálogo
// cerrado de motivos.
func MotivoRevocacionDesde(valor string) (MotivoRevocacion, error) {
	switch valor {
	case MotivoCierreUsuario.valor,
		MotivoCierreMasivoUsuario.valor,
		MotivoReusoRefrescoDetectado.valor,
		MotivoCuentaNoOperativa.valor,
		MotivoContrasenaCambiada.valor,
		MotivoLimiteSesionesExcedido.valor,
		MotivoRevocacionAdministrativa.valor:
		return MotivoRevocacion{valor: valor}, nil
	default:
		return MotivoRevocacion{}, &ErrMotivoRevocacionInvalido{Valor: valor}
	}
}

// Valor devuelve el texto canónico del motivo, del catálogo cerrado.
func (m MotivoRevocacion) Valor() string { return m.valor }

// String implementa fmt.Stringer.
func (m MotivoRevocacion) String() string { return m.valor }

// EsVacio indica si el value object nunca fue construido (zero value): una
// sesión sin revocar no tiene motivo (INV-ACC-08 exige motivo únicamente en
// el estado revocada).
func (m MotivoRevocacion) EsVacio() bool { return m.valor == "" }

// EsIgual compara dos motivos por su valor.
func (m MotivoRevocacion) EsIgual(otro MotivoRevocacion) bool { return m.valor == otro.valor }

// EsIniciativaPropia indica si el motivo representa una decisión del propio
// usuario (cierre_usuario, cierre_masivo_usuario) en contraposición a una
// revocación que el sistema le impone (los demás motivos del catálogo). Es
// la misma distinción que separa los eventos SesionCerrada de SesionRevocada
// (tabla 1.6 del diseño).
func (m MotivoRevocacion) EsIniciativaPropia() bool {
	return m.EsIgual(MotivoCierreUsuario) || m.EsIgual(MotivoCierreMasivoUsuario)
}

// ErrMotivoRevocacionInvalido se produce al construir un MotivoRevocacion
// con un valor fuera del catálogo cerrado.
type ErrMotivoRevocacionInvalido struct{ Valor string }

func (e *ErrMotivoRevocacionInvalido) Error() string {
	return "motivo de revocación desconocido: " + e.Valor
}
