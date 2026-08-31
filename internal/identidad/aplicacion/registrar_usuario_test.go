package aplicacion_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/r-david1/moterus/internal/identidad/aplicacion"
	"github.com/r-david1/moterus/internal/identidad/dominio"
	"github.com/r-david1/moterus/internal/identidad/puertos"
	"github.com/r-david1/moterus/internal/identidad/puertos/mocks"
)

// mocksRegistrar agrupa los test doubles de las doce dependencias de
// RegistrarUsuarioCasoDeUso, ya configurados con un camino feliz razonable.
// Cada test sobreescribe solo lo que necesita.
type mocksRegistrar struct {
	usuarios           *mocks.RepositorioUsuarios
	hasher             *mocks.HasherContrasenas
	filtradas          *mocks.VerificadorContrasenasFiltradas
	confianza          *mocks.EvaluadorConfianza
	auditoria          *mocks.RegistroAuditoria
	eventos            *mocks.PublicadorEventos
	reloj              *mocks.Reloj
	ids                *mocks.GeneradorIDs
	uow                *mocks.UnidadDeTrabajo
	generadorTokens    *mocks.GeneradorTokens
	tokensVerificacion *mocks.RepositorioTokensVerificacion
	notificadorCorreo  *mocks.NotificadorCorreo
}

func nuevosMocksRegistrar(t *testing.T) *mocksRegistrar {
	t.Helper()
	id := idDePrueba(t, idUsuarioValido1)
	hash := hashDePrueba(t, "hashinicial")
	return &mocksRegistrar{
		usuarios: &mocks.RepositorioUsuarios{},
		hasher: &mocks.HasherContrasenas{
			FnHashear: func(ctx context.Context, p dominio.ContrasenaPlana) (dominio.HashContrasena, error) {
				return hash, nil
			},
		},
		filtradas: &mocks.VerificadorContrasenasFiltradas{},
		confianza: &mocks.EvaluadorConfianza{},
		auditoria: &mocks.RegistroAuditoria{},
		eventos:   &mocks.PublicadorEventos{},
		reloj:     &mocks.Reloj{Fija: ahoraDePrueba()},
		ids: &mocks.GeneradorIDs{
			FnNuevoIDUsuario: func() (dominio.IDUsuario, error) { return id, nil },
		},
		uow:                &mocks.UnidadDeTrabajo{},
		generadorTokens:    &mocks.GeneradorTokens{},
		tokensVerificacion: &mocks.RepositorioTokensVerificacion{},
		notificadorCorreo:  &mocks.NotificadorCorreo{},
	}
}

func (m *mocksRegistrar) casoDeUso() *aplicacion.RegistrarUsuarioCasoDeUso {
	return aplicacion.NuevoRegistrarUsuarioCasoDeUso(
		m.usuarios, m.hasher, m.filtradas, m.confianza, m.auditoria, m.eventos, m.reloj, m.ids, m.uow,
		m.generadorTokens, m.tokensVerificacion, m.notificadorCorreo,
	)
}

func comandoRegistrarValido(t *testing.T) aplicacion.ComandoRegistrarUsuario {
	t.Helper()
	return aplicacion.ComandoRegistrarUsuario{
		Correo:     correoValidoValor,
		Contrasena: contrasenaFuerteValor,
		Origen:     origenDePrueba(t),
	}
}

func TestRegistrarUsuarioCasoDeUso_FlujoFeliz(t *testing.T) {
	m := nuevosMocksRegistrar(t)
	caso := m.casoDeUso()

	resultado, err := caso.Registrar(context.Background(), comandoRegistrarValido(t))
	if err != nil {
		t.Fatalf("Registrar() devolvió error inesperado: %v", err)
	}

	if resultado.IDUsuario != idUsuarioValido1 {
		t.Errorf("IDUsuario = %q, esperado %q", resultado.IDUsuario, idUsuarioValido1)
	}
	// INV-ID-07: nace en pendiente_verificacion, nunca en activo.
	if resultado.Estado != "pendiente_verificacion" {
		t.Errorf("Estado = %q, esperado pendiente_verificacion", resultado.Estado)
	}
	if !resultado.RequiereVerificacionCorreo {
		t.Error("RequiereVerificacionCorreo debía ser true")
	}

	if len(m.usuarios.LlamadasGuardar) != 1 {
		t.Fatalf("se esperaba exactamente una llamada a Guardar, hubo %d", len(m.usuarios.LlamadasGuardar))
	}
	if estado := m.usuarios.LlamadasGuardar[0].Estado().String(); estado != "pendiente_verificacion" {
		t.Errorf("el usuario guardado debía estar pendiente_verificacion, estaba %q", estado)
	}

	if nombres := m.auditoria.NombresEventos(); len(nombres) != 1 || nombres[0] != "UsuarioRegistrado" {
		t.Errorf("eventos auditados = %v, esperado [UsuarioRegistrado]", nombres)
	}
	if len(m.eventos.LlamadasPublicar) != 1 {
		t.Errorf("se esperaba una publicación de eventos tras confirmar el registro, hubo %d", len(m.eventos.LlamadasPublicar))
	}
	if len(m.confianza.LlamadasRegistrarResultado) != 1 || !m.confianza.LlamadasRegistrarResultado[0].Exitoso {
		t.Error("se esperaba EvaluadorConfianza.RegistrarResultado(exitoso=true)")
	}
}

func TestRegistrarUsuarioCasoDeUso_ConfianzaDeniega(t *testing.T) {
	m := nuevosMocksRegistrar(t)
	m.confianza.FnEvaluar = func(ctx context.Context, s puertos.SolicitudEvaluacion) (puertos.DecisionConfianza, error) {
		return puertos.DecisionConfianza{Permitido: false, Motivo: "riesgo_alto"}, nil
	}
	caso := m.casoDeUso()

	_, err := caso.Registrar(context.Background(), comandoRegistrarValido(t))

	var errConfianza *dominio.ErrAccesoDenegadoPorConfianza
	if !errors.As(err, &errConfianza) {
		t.Fatalf("se esperaba *ErrAccesoDenegadoPorConfianza, obtuvo %T: %v", err, err)
	}
	if len(m.usuarios.LlamadasGuardar) != 0 {
		t.Error("un registro denegado por Confianza no debe llegar a persistir el usuario")
	}
	if nombres := m.auditoria.NombresEventos(); len(nombres) != 1 || nombres[0] != "RegistroRechazado" {
		t.Errorf("eventos auditados = %v, esperado [RegistroRechazado]", nombres)
	}
}

// TestRegistrarUsuarioCasoDeUso_ConfianzaDeniega_FalloAuditoriaSePropaga
// cubre INV-ID-15: si no se puede probar lo que pasó, no pasa. El error de
// auditoría se prioriza sobre el de negocio (ErrAccesoDenegadoPorConfianza).
func TestRegistrarUsuarioCasoDeUso_ConfianzaDeniega_FalloAuditoriaSePropaga(t *testing.T) {
	m := nuevosMocksRegistrar(t)
	errAuditoria := errors.New("bd de auditoría caída")
	m.confianza.FnEvaluar = func(ctx context.Context, s puertos.SolicitudEvaluacion) (puertos.DecisionConfianza, error) {
		return puertos.DecisionConfianza{Permitido: false, Motivo: "riesgo_alto"}, nil
	}
	m.auditoria.FnRegistrar = func(ctx context.Context, e dominio.EventoDominio, origen dominio.OrigenSolicitud) error {
		return errAuditoria
	}
	caso := m.casoDeUso()

	_, err := caso.Registrar(context.Background(), comandoRegistrarValido(t))
	if !errors.Is(err, errAuditoria) {
		t.Fatalf("se esperaba que el error de auditoría se propagara tal cual, obtuvo %v", err)
	}
}

func TestRegistrarUsuarioCasoDeUso_ConfianzaEvaluarError_SePropaga(t *testing.T) {
	m := nuevosMocksRegistrar(t)
	errConexion := errors.New("confianza no disponible")
	m.confianza.FnEvaluar = func(ctx context.Context, s puertos.SolicitudEvaluacion) (puertos.DecisionConfianza, error) {
		return puertos.DecisionConfianza{}, errConexion
	}
	caso := m.casoDeUso()

	_, err := caso.Registrar(context.Background(), comandoRegistrarValido(t))
	if !errors.Is(err, errConexion) {
		t.Fatalf("se esperaba que el error de EvaluadorConfianza se propagara, obtuvo %v", err)
	}
	if len(m.usuarios.LlamadasGuardar) != 0 {
		t.Error("no debía llegar a persistir el usuario")
	}
}

func TestRegistrarUsuarioCasoDeUso_CorreoInvalido(t *testing.T) {
	m := nuevosMocksRegistrar(t)
	caso := m.casoDeUso()
	cmd := comandoRegistrarValido(t)
	cmd.Correo = "no-es-un-correo"

	_, err := caso.Registrar(context.Background(), cmd)
	var errCorreo *dominio.ErrCorreoInvalido
	if !errors.As(err, &errCorreo) {
		t.Fatalf("se esperaba *ErrCorreoInvalido, obtuvo %T: %v", err, err)
	}
}

func TestRegistrarUsuarioCasoDeUso_ContrasenaVaciaEsInvalidaEstructuralmente(t *testing.T) {
	m := nuevosMocksRegistrar(t)
	caso := m.casoDeUso()
	cmd := comandoRegistrarValido(t)
	cmd.Contrasena = ""

	_, err := caso.Registrar(context.Background(), cmd)
	var errPlana *dominio.ErrContrasenaPlanaInvalida
	if !errors.As(err, &errPlana) {
		t.Fatalf("se esperaba *ErrContrasenaPlanaInvalida, obtuvo %T: %v", err, err)
	}
}

func TestRegistrarUsuarioCasoDeUso_ContrasenaDebil(t *testing.T) {
	m := nuevosMocksRegistrar(t)
	caso := m.casoDeUso()
	cmd := comandoRegistrarValido(t)
	cmd.Contrasena = "corta12345" // 10 caracteres: incumple el mínimo NIST

	_, err := caso.Registrar(context.Background(), cmd)
	var errDebil *dominio.ErrContrasenaDebil
	if !errors.As(err, &errDebil) {
		t.Fatalf("se esperaba *ErrContrasenaDebil, obtuvo %T: %v", err, err)
	}
}

func TestRegistrarUsuarioCasoDeUso_ContrasenaFiltrada(t *testing.T) {
	m := nuevosMocksRegistrar(t)
	m.filtradas.FnEstaFiltrada = func(ctx context.Context, p dominio.ContrasenaPlana) (bool, error) {
		return true, nil
	}
	caso := m.casoDeUso()

	_, err := caso.Registrar(context.Background(), comandoRegistrarValido(t))
	var errFiltrada *dominio.ErrContrasenaFiltrada
	if !errors.As(err, &errFiltrada) {
		t.Fatalf("se esperaba *ErrContrasenaFiltrada, obtuvo %T: %v", err, err)
	}
	if len(m.usuarios.LlamadasGuardar) != 0 {
		t.Error("una contraseña filtrada no debe llegar a persistir el usuario")
	}
}

// TestRegistrarUsuarioCasoDeUso_VerificadorFiltradasFalla_FailOpen cubre el
// diseño explícito: una caída del puerto de brechas conocidas no puede
// bloquear todas las altas (sección 3.1, paso 4).
func TestRegistrarUsuarioCasoDeUso_VerificadorFiltradasFalla_FailOpen(t *testing.T) {
	m := nuevosMocksRegistrar(t)
	m.filtradas.FnEstaFiltrada = func(ctx context.Context, p dominio.ContrasenaPlana) (bool, error) {
		return false, errors.New("hibp no disponible")
	}
	caso := m.casoDeUso()

	resultado, err := caso.Registrar(context.Background(), comandoRegistrarValido(t))
	if err != nil {
		t.Fatalf("un fallo del verificador de filtradas no debe abortar el registro (fail-open): %v", err)
	}
	if resultado.IDUsuario == "" {
		t.Error("se esperaba un registro exitoso pese al fallo fail-open")
	}
	if len(m.usuarios.LlamadasGuardar) != 1 {
		t.Error("el registro debía completarse y persistir el usuario")
	}
}

func TestRegistrarUsuarioCasoDeUso_HashearFalla(t *testing.T) {
	m := nuevosMocksRegistrar(t)
	errHash := errors.New("hasher no disponible")
	m.hasher.FnHashear = func(ctx context.Context, p dominio.ContrasenaPlana) (dominio.HashContrasena, error) {
		return dominio.HashContrasena{}, errHash
	}
	caso := m.casoDeUso()

	_, err := caso.Registrar(context.Background(), comandoRegistrarValido(t))
	if !errors.Is(err, errHash) {
		t.Fatalf("se esperaba que el error del hasher se propagara, obtuvo %v", err)
	}
	if len(m.usuarios.LlamadasGuardar) != 0 {
		t.Error("no debía llegar a persistir el usuario")
	}
}

func TestRegistrarUsuarioCasoDeUso_GeneradorIDsFalla(t *testing.T) {
	m := nuevosMocksRegistrar(t)
	errIDs := errors.New("generador de ids no disponible")
	m.ids.FnNuevoIDUsuario = func() (dominio.IDUsuario, error) { return dominio.IDUsuario{}, errIDs }
	caso := m.casoDeUso()

	_, err := caso.Registrar(context.Background(), comandoRegistrarValido(t))
	if !errors.Is(err, errIDs) {
		t.Fatalf("se esperaba que el error del generador de ids se propagara, obtuvo %v", err)
	}
}

// TestRegistrarUsuarioCasoDeUso_GeneradorIDsDevuelveVacio ejerce la
// validación defensiva de dominio.RegistrarUsuario cuando el puerto
// GeneradorIDs devuelve un IDUsuario vacío sin error.
func TestRegistrarUsuarioCasoDeUso_GeneradorIDsDevuelveVacio(t *testing.T) {
	m := nuevosMocksRegistrar(t)
	m.ids.FnNuevoIDUsuario = func() (dominio.IDUsuario, error) { return dominio.IDUsuario{}, nil }
	caso := m.casoDeUso()

	_, err := caso.Registrar(context.Background(), comandoRegistrarValido(t))
	var errID *dominio.ErrIDUsuarioInvalido
	if !errors.As(err, &errID) {
		t.Fatalf("se esperaba *ErrIDUsuarioInvalido, obtuvo %T: %v", err, err)
	}
}

// TestRegistrarUsuarioCasoDeUso_FalloDePersistenciaAbortaElRegistro cubre el
// mecanismo real de INV-ID-15 en este caso de uso: la escritura de negocio y
// la auditoría viven en la misma UnidadDeTrabajo, así que un fallo del
// repositorio aborta la transacción completa. Nada aguas abajo (publicación
// de eventos, aviso a Confianza) debe ejecutarse.
func TestRegistrarUsuarioCasoDeUso_FalloDePersistenciaAbortaElRegistro(t *testing.T) {
	m := nuevosMocksRegistrar(t)
	errGuardar := errors.New("violación de restricción en la base de datos")
	m.usuarios.FnGuardar = func(ctx context.Context, u *dominio.Usuario) error { return errGuardar }
	caso := m.casoDeUso()

	_, err := caso.Registrar(context.Background(), comandoRegistrarValido(t))
	if !errors.Is(err, errGuardar) {
		t.Fatalf("se esperaba que el error de persistencia se propagara, obtuvo %v", err)
	}
	if len(m.auditoria.LlamadasRegistrar) != 0 {
		t.Error("si Guardar falla dentro de la UnidadDeTrabajo, no debe llegar a auditarse UsuarioRegistrado")
	}
	if len(m.eventos.LlamadasPublicar) != 0 {
		t.Error("un registro abortado no debe publicar eventos")
	}
	if len(m.confianza.LlamadasRegistrarResultado) != 0 {
		t.Error("un registro abortado no debe avisar a Confianza del resultado")
	}
}

// TestRegistrarUsuarioCasoDeUso_FalloDeAuditoriaDentroDeLaTransaccionAborta
// cubre INV-ID-15 en el otro sentido: si la escritura de negocio tuvo éxito
// pero la auditoría no pudo registrarse en la misma transacción, la
// UnidadDeTrabajo debe abortar (nunca hay negocio confirmado sin su
// auditoría).
func TestRegistrarUsuarioCasoDeUso_FalloDeAuditoriaDentroDeLaTransaccionAborta(t *testing.T) {
	m := nuevosMocksRegistrar(t)
	errAuditoria := errors.New("bd de auditoría caída")
	m.auditoria.FnRegistrar = func(ctx context.Context, e dominio.EventoDominio, origen dominio.OrigenSolicitud) error {
		return errAuditoria
	}
	caso := m.casoDeUso()

	_, err := caso.Registrar(context.Background(), comandoRegistrarValido(t))
	if !errors.Is(err, errAuditoria) {
		t.Fatalf("se esperaba que el error de auditoría se propagara, obtuvo %v", err)
	}
	if len(m.eventos.LlamadasPublicar) != 0 {
		t.Error("un registro abortado no debe publicar eventos")
	}
}

// TestRegistrarUsuarioCasoDeUso_FalloAlPublicarEventos_NoBloqueaElRegistro
// cubre que la publicación asíncrona (fuera de la UnidadDeTrabajo) es
// best-effort: su fallo no debe convertir un registro ya persistido en un
// error.
func TestRegistrarUsuarioCasoDeUso_FalloAlPublicarEventos_NoBloqueaElRegistro(t *testing.T) {
	m := nuevosMocksRegistrar(t)
	m.eventos.FnPublicar = func(ctx context.Context, eventos ...dominio.EventoDominio) error {
		return errors.New("cola de eventos no disponible")
	}
	caso := m.casoDeUso()

	resultado, err := caso.Registrar(context.Background(), comandoRegistrarValido(t))
	if err != nil {
		t.Fatalf("un fallo al publicar eventos no debe abortar el registro ya persistido: %v", err)
	}
	if resultado.IDUsuario == "" {
		t.Error("se esperaba un resultado de registro exitoso")
	}
}

// TestRegistrarUsuarioCasoDeUso_FalloAlRegistrarResultadoEnConfianza_NoBloquea
// cubre que el aviso final a Confianza también es best-effort.
func TestRegistrarUsuarioCasoDeUso_FalloAlRegistrarResultadoEnConfianza_NoBloquea(t *testing.T) {
	m := nuevosMocksRegistrar(t)
	m.confianza.FnRegistrarResultado = func(ctx context.Context, r puertos.ResultadoIntento) error {
		return errors.New("confianza no disponible")
	}
	caso := m.casoDeUso()

	resultado, err := caso.Registrar(context.Background(), comandoRegistrarValido(t))
	if err != nil {
		t.Fatalf("un fallo al avisar a Confianza no debe abortar el registro ya persistido: %v", err)
	}
	if resultado.IDUsuario == "" {
		t.Error("se esperaba un resultado de registro exitoso")
	}
}

// --- sección 3.4 del diseño: token de verificación de correo ---------------

// TestRegistrarUsuarioCasoDeUso_EmiteYEnviaTokenDeVerificacion cubre el
// flujo nuevo de la sección 3.4: se genera un token, se guarda su hash junto
// con el usuario en la misma transacción, y se entrega el token en claro
// únicamente al NotificadorCorreo.
func TestRegistrarUsuarioCasoDeUso_EmiteYEnviaTokenDeVerificacion(t *testing.T) {
	m := nuevosMocksRegistrar(t)
	m.generadorTokens.FnGenerar = func() (string, error) { return "token-plano-de-prueba", nil }
	caso := m.casoDeUso()

	resultado, err := caso.Registrar(context.Background(), comandoRegistrarValido(t))
	if err != nil {
		t.Fatalf("Registrar() devolvió error inesperado: %v", err)
	}

	if m.generadorTokens.LlamadasGenerar != 1 {
		t.Errorf("se esperaba una llamada a GeneradorTokens.Generar, hubo %d", m.generadorTokens.LlamadasGenerar)
	}
	if len(m.tokensVerificacion.LlamadasGuardar) != 1 {
		t.Fatalf("se esperaba exactamente una llamada a RepositorioTokensVerificacion.Guardar, hubo %d", len(m.tokensVerificacion.LlamadasGuardar))
	}
	guardado := m.tokensVerificacion.LlamadasGuardar[0]
	if guardado.UsuarioID.String() != resultado.IDUsuario {
		t.Errorf("el token se guardó para el usuario %q, esperado %q", guardado.UsuarioID.String(), resultado.IDUsuario)
	}
	// INV-ID-21: el hash guardado nunca es el token en claro.
	if guardado.HashToken == "token-plano-de-prueba" {
		t.Error("el hash guardado no debe ser igual al token en claro (INV-ID-21)")
	}
	if !guardado.ExpiraEn.After(ahoraDePrueba()) {
		t.Errorf("ExpiraEn = %v, se esperaba una fecha futura respecto de %v", guardado.ExpiraEn, ahoraDePrueba())
	}

	if len(m.notificadorCorreo.LlamadasEnviarVerificacion) != 1 {
		t.Fatalf("se esperaba exactamente un envío de correo de verificación, hubo %d", len(m.notificadorCorreo.LlamadasEnviarVerificacion))
	}
	notificacion := m.notificadorCorreo.LlamadasEnviarVerificacion[0]
	if notificacion.TokenPlano != "token-plano-de-prueba" {
		t.Errorf("TokenPlano notificado = %q, esperado %q", notificacion.TokenPlano, "token-plano-de-prueba")
	}
	if notificacion.Correo.Normalizado() != correoValidoValor {
		t.Errorf("Correo notificado = %q, esperado %q", notificacion.Correo.Normalizado(), correoValidoValor)
	}
}

// TestRegistrarUsuarioCasoDeUso_INVID21_ResultadoNuncaExponeElToken es la
// salvaguarda explícita de INV-ID-21: el token en claro que recibe
// NotificadorCorreo nunca debe coincidir con ningún valor de string
// exportado por ResultadoRegistro (recorrido por reflexión), y el propio
// tipo no debe tener campos cuyo nombre sugiera "token" (complementa
// TestResultados_NoExponenCredenciales, que ya cubre el nombre del campo).
func TestRegistrarUsuarioCasoDeUso_INVID21_ResultadoNuncaExponeElToken(t *testing.T) {
	m := nuevosMocksRegistrar(t)
	const tokenSecreto = "token-plano-secreto-que-no-debe-salir"
	m.generadorTokens.FnGenerar = func() (string, error) { return tokenSecreto, nil }
	caso := m.casoDeUso()

	resultado, err := caso.Registrar(context.Background(), comandoRegistrarValido(t))
	if err != nil {
		t.Fatalf("Registrar() devolvió error inesperado: %v", err)
	}

	valor := reflect.ValueOf(resultado)
	tipo := valor.Type()
	for i := 0; i < tipo.NumField(); i++ {
		campo := tipo.Field(i)
		if strings.Contains(strings.ToLower(campo.Name), "token") {
			t.Errorf("ResultadoRegistro.%s: el nombre del campo sugiere que transporta el token (INV-ID-21)", campo.Name)
		}
		if valor.Field(i).Kind() == reflect.String && valor.Field(i).String() == tokenSecreto {
			t.Errorf("ResultadoRegistro.%s contiene el token en claro (INV-ID-21)", campo.Name)
		}
	}
}

// TestRegistrarUsuarioCasoDeUso_GeneradorTokensFalla_AbortaElRegistro cubre
// que, sin token de verificación, no tiene sentido dejar completar el
// registro: el usuario quedaría en pendiente_verificacion sin forma de
// activarse.
func TestRegistrarUsuarioCasoDeUso_GeneradorTokensFalla_AbortaElRegistro(t *testing.T) {
	m := nuevosMocksRegistrar(t)
	errToken := errors.New("generador de tokens no disponible")
	m.generadorTokens.FnGenerar = func() (string, error) { return "", errToken }
	caso := m.casoDeUso()

	_, err := caso.Registrar(context.Background(), comandoRegistrarValido(t))
	if !errors.Is(err, errToken) {
		t.Fatalf("se esperaba que el error del generador de tokens se propagara, obtuvo %v", err)
	}
	if len(m.usuarios.LlamadasGuardar) != 0 {
		t.Error("no debía llegar a persistir el usuario")
	}
	if len(m.notificadorCorreo.LlamadasEnviarVerificacion) != 0 {
		t.Error("no debía llegar a enviar ningún correo")
	}
}

// TestRegistrarUsuarioCasoDeUso_FalloAlGuardarTokenAbortaElRegistro cubre
// que el guardado del token vive en la misma UnidadDeTrabajo que el usuario
// y su auditoría (ADR 0005): si falla, toda la transacción se revierte.
func TestRegistrarUsuarioCasoDeUso_FalloAlGuardarTokenAbortaElRegistro(t *testing.T) {
	m := nuevosMocksRegistrar(t)
	errGuardarToken := errors.New("bd de tokens caída")
	m.tokensVerificacion.FnGuardar = func(ctx context.Context, usuarioID dominio.IDUsuario, hashToken string, expiraEn time.Time) error {
		return errGuardarToken
	}
	caso := m.casoDeUso()

	_, err := caso.Registrar(context.Background(), comandoRegistrarValido(t))
	if !errors.Is(err, errGuardarToken) {
		t.Fatalf("se esperaba que el error de RepositorioTokensVerificacion.Guardar se propagara, obtuvo %v", err)
	}
	if len(m.auditoria.LlamadasRegistrar) != 0 {
		t.Error("si el token no se pudo guardar dentro de la UnidadDeTrabajo, no debe llegar a auditarse UsuarioRegistrado")
	}
	if len(m.notificadorCorreo.LlamadasEnviarVerificacion) != 0 {
		t.Error("un registro abortado no debe enviar el correo de verificación")
	}
}

// TestRegistrarUsuarioCasoDeUso_FalloAlNotificarCorreo_NoBloqueaElRegistro
// cubre que el envío del correo de verificación es best-effort, igual
// criterio que la publicación de eventos: su fallo no invalida un registro
// ya persistido.
func TestRegistrarUsuarioCasoDeUso_FalloAlNotificarCorreo_NoBloqueaElRegistro(t *testing.T) {
	m := nuevosMocksRegistrar(t)
	m.notificadorCorreo.FnEnviarVerificacion = func(ctx context.Context, correo dominio.Correo, tokenPlano string) error {
		return errors.New("proveedor de correo no disponible")
	}
	caso := m.casoDeUso()

	resultado, err := caso.Registrar(context.Background(), comandoRegistrarValido(t))
	if err != nil {
		t.Fatalf("un fallo al enviar el correo de verificación no debe abortar el registro ya persistido: %v", err)
	}
	if resultado.IDUsuario == "" {
		t.Error("se esperaba un resultado de registro exitoso")
	}
}
