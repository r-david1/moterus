package aplicacion_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/r-david1/moterus/internal/identidad/aplicacion"
	"github.com/r-david1/moterus/internal/identidad/dominio"
	"github.com/r-david1/moterus/internal/identidad/puertos"
	"github.com/r-david1/moterus/internal/identidad/puertos/mocks"
)

// mocksReenviar agrupa los test doubles de las cinco dependencias de
// ReenviarVerificacionCasoDeUso. Por defecto no hay ningún usuario (el
// repositorio devuelve "no encontrado"): cada test configura el escenario
// que necesita.
type mocksReenviar struct {
	usuarios           *mocks.RepositorioUsuarios
	generadorTokens    *mocks.GeneradorTokens
	tokensVerificacion *mocks.RepositorioTokensVerificacion
	notificadorCorreo  *mocks.NotificadorCorreo
	reloj              *mocks.Reloj
}

func nuevosMocksReenviar(t *testing.T, usuario *dominio.Usuario) *mocksReenviar {
	t.Helper()
	return &mocksReenviar{
		usuarios: &mocks.RepositorioUsuarios{
			FnBuscarPorCorreo: func(ctx context.Context, c dominio.Correo) (*dominio.Usuario, error) {
				if usuario != nil && usuario.Correo().Normalizado() == c.Normalizado() {
					return usuario, nil
				}
				return nil, nil
			},
		},
		generadorTokens:    &mocks.GeneradorTokens{},
		tokensVerificacion: &mocks.RepositorioTokensVerificacion{},
		notificadorCorreo:  &mocks.NotificadorCorreo{},
		reloj:              &mocks.Reloj{Fija: ahoraDePrueba()},
	}
}

func (m *mocksReenviar) casoDeUso() *aplicacion.ReenviarVerificacionCasoDeUso {
	return aplicacion.NuevoReenviarVerificacionCasoDeUso(
		m.usuarios, m.generadorTokens, m.tokensVerificacion, m.notificadorCorreo, m.reloj,
	)
}

func comandoReenviarValido(t *testing.T) aplicacion.ComandoReenviarVerificacion {
	t.Helper()
	return aplicacion.ComandoReenviarVerificacion{
		Correo: correoValidoValor,
		Origen: origenDePrueba(t),
	}
}

func TestReenviarVerificacionCasoDeUso_FlujoFeliz(t *testing.T) {
	id := idDePrueba(t, idUsuarioValido1)
	correo := correoDePrueba(t, correoValidoValor)
	hash := hashDePrueba(t, "hashvigente")
	usuario := usuarioPendienteDePrueba(t, id, correo, hash)

	m := nuevosMocksReenviar(t, usuario)
	caso := m.casoDeUso()

	err := caso.Reenviar(context.Background(), comandoReenviarValido(t))
	if err != nil {
		t.Fatalf("Reenviar() devolvió error inesperado: %v", err)
	}

	if m.generadorTokens.LlamadasGenerar != 1 {
		t.Errorf("se esperaba una llamada a GeneradorTokens.Generar, hubo %d", m.generadorTokens.LlamadasGenerar)
	}
	if len(m.tokensVerificacion.LlamadasGuardar) != 1 {
		t.Fatalf("se esperaba exactamente una llamada a Guardar, hubo %d", len(m.tokensVerificacion.LlamadasGuardar))
	}
	if !m.tokensVerificacion.LlamadasGuardar[0].UsuarioID.EsIgual(id) {
		t.Errorf("Guardar se llamó con el usuario incorrecto: %v", m.tokensVerificacion.LlamadasGuardar[0].UsuarioID)
	}
	if len(m.notificadorCorreo.LlamadasEnviarVerificacion) != 1 {
		t.Fatalf("se esperaba exactamente un envío de correo, hubo %d", len(m.notificadorCorreo.LlamadasEnviarVerificacion))
	}
	if m.notificadorCorreo.LlamadasEnviarVerificacion[0].Correo.Normalizado() != correo.Normalizado() {
		t.Error("el correo notificado no coincide con el del usuario")
	}
	// El token en claro que recibe NotificadorCorreo debe ser el mismo que
	// produjo GeneradorTokens, y el hash guardado no debe ser igual al
	// token en claro (INV-ID-21, aplicado también a este flujo).
	tokenPlano := m.notificadorCorreo.LlamadasEnviarVerificacion[0].TokenPlano
	if tokenPlano == "" {
		t.Error("el token en claro enviado no debe estar vacío")
	}
	if m.tokensVerificacion.LlamadasGuardar[0].HashToken == tokenPlano {
		t.Error("el hash guardado no debe ser igual al token en claro (INV-ID-21)")
	}
}

func TestReenviarVerificacionCasoDeUso_CorreoInexistente_RespuestaNeutra(t *testing.T) {
	m := nuevosMocksReenviar(t, nil)
	caso := m.casoDeUso()

	err := caso.Reenviar(context.Background(), comandoReenviarValido(t))
	if err != nil {
		t.Fatalf("se esperaba respuesta neutra (nil), obtuvo %v", err)
	}
	if m.generadorTokens.LlamadasGenerar != 0 {
		t.Error("un correo inexistente no debe generar ningún token")
	}
	if len(m.tokensVerificacion.LlamadasGuardar) != 0 {
		t.Error("un correo inexistente no debe guardar ningún token")
	}
	if len(m.notificadorCorreo.LlamadasEnviarVerificacion) != 0 {
		t.Error("un correo inexistente no debe disparar ningún envío")
	}
}

func TestReenviarVerificacionCasoDeUso_UsuarioNoEncontradoComoError_RespuestaNeutra(t *testing.T) {
	m := nuevosMocksReenviar(t, nil)
	m.usuarios.FnBuscarPorCorreo = func(ctx context.Context, c dominio.Correo) (*dominio.Usuario, error) {
		return nil, &dominio.ErrUsuarioNoEncontrado{IDUsuario: ""}
	}
	caso := m.casoDeUso()

	err := caso.Reenviar(context.Background(), comandoReenviarValido(t))
	if err != nil {
		t.Fatalf("se esperaba respuesta neutra (nil), obtuvo %v", err)
	}
}

func TestReenviarVerificacionCasoDeUso_CorreoInvalido_RespuestaNeutra(t *testing.T) {
	m := nuevosMocksReenviar(t, nil)
	caso := m.casoDeUso()
	cmd := comandoReenviarValido(t)
	cmd.Correo = "no-es-un-correo"

	err := caso.Reenviar(context.Background(), cmd)
	if err != nil {
		t.Fatalf("se esperaba respuesta neutra (nil), obtuvo %v", err)
	}
	if len(m.usuarios.LlamadasBuscarPorCorreo) != 0 {
		t.Error("un correo estructuralmente inválido no debe llegar a consultar el repositorio")
	}
}

func TestReenviarVerificacionCasoDeUso_UsuarioActivo_RespuestaNeutra(t *testing.T) {
	id := idDePrueba(t, idUsuarioValido1)
	correo := correoDePrueba(t, correoValidoValor)
	hash := hashDePrueba(t, "hashvigente")
	usuario := usuarioActivoDePrueba(t, id, correo, hash)

	m := nuevosMocksReenviar(t, usuario)
	caso := m.casoDeUso()

	err := caso.Reenviar(context.Background(), comandoReenviarValido(t))
	if err != nil {
		t.Fatalf("se esperaba respuesta neutra (nil), obtuvo %v", err)
	}
	if m.generadorTokens.LlamadasGenerar != 0 {
		t.Error("un usuario ya activo no debe generar ningún token nuevo")
	}
}

func TestReenviarVerificacionCasoDeUso_UsuarioSuspendido_RespuestaNeutra(t *testing.T) {
	id := idDePrueba(t, idUsuarioValido1)
	correo := correoDePrueba(t, correoValidoValor)
	hash := hashDePrueba(t, "hashvigente")
	usuario := usuarioSuspendidoDePrueba(t, id, correo, hash)

	m := nuevosMocksReenviar(t, usuario)
	caso := m.casoDeUso()

	err := caso.Reenviar(context.Background(), comandoReenviarValido(t))
	if err != nil {
		t.Fatalf("se esperaba respuesta neutra (nil), obtuvo %v", err)
	}
	if m.generadorTokens.LlamadasGenerar != 0 {
		t.Error("un usuario suspendido no debe generar ningún token nuevo")
	}
}

func TestReenviarVerificacionCasoDeUso_UsuarioBloqueado_RespuestaNeutra(t *testing.T) {
	id := idDePrueba(t, idUsuarioValido1)
	correo := correoDePrueba(t, correoValidoValor)
	hash := hashDePrueba(t, "hashvigente")
	usuario := usuarioBloqueadoDePrueba(t, id, correo, hash)

	m := nuevosMocksReenviar(t, usuario)
	caso := m.casoDeUso()

	err := caso.Reenviar(context.Background(), comandoReenviarValido(t))
	if err != nil {
		t.Fatalf("se esperaba respuesta neutra (nil), obtuvo %v", err)
	}
	if m.generadorTokens.LlamadasGenerar != 0 {
		t.Error("un usuario bloqueado no debe generar ningún token nuevo")
	}
}

func TestReenviarVerificacionCasoDeUso_FalloAlBuscarUsuario_RespuestaNeutra(t *testing.T) {
	m := nuevosMocksReenviar(t, nil)
	m.usuarios.FnBuscarPorCorreo = func(ctx context.Context, c dominio.Correo) (*dominio.Usuario, error) {
		return nil, errors.New("bd de usuarios caída")
	}
	caso := m.casoDeUso()

	err := caso.Reenviar(context.Background(), comandoReenviarValido(t))
	if err != nil {
		t.Fatalf("un fallo interno no debe filtrarse como error distinto al camino feliz (INV-ID-22), obtuvo %v", err)
	}
}

func TestReenviarVerificacionCasoDeUso_FalloAlGenerarToken_RespuestaNeutra(t *testing.T) {
	id := idDePrueba(t, idUsuarioValido1)
	correo := correoDePrueba(t, correoValidoValor)
	hash := hashDePrueba(t, "hashvigente")
	usuario := usuarioPendienteDePrueba(t, id, correo, hash)

	m := nuevosMocksReenviar(t, usuario)
	m.generadorTokens.FnGenerar = func() (string, error) { return "", errors.New("generador no disponible") }
	caso := m.casoDeUso()

	err := caso.Reenviar(context.Background(), comandoReenviarValido(t))
	if err != nil {
		t.Fatalf("un fallo interno no debe filtrarse como error distinto al camino feliz (INV-ID-22), obtuvo %v", err)
	}
	if len(m.tokensVerificacion.LlamadasGuardar) != 0 {
		t.Error("si el generador falla, no debe intentar guardar ningún token")
	}
}

func TestReenviarVerificacionCasoDeUso_FalloAlGuardarToken_RespuestaNeutra(t *testing.T) {
	id := idDePrueba(t, idUsuarioValido1)
	correo := correoDePrueba(t, correoValidoValor)
	hash := hashDePrueba(t, "hashvigente")
	usuario := usuarioPendienteDePrueba(t, id, correo, hash)

	m := nuevosMocksReenviar(t, usuario)
	m.tokensVerificacion.FnGuardar = func(ctx context.Context, usuarioID dominio.IDUsuario, hashToken string, expiraEn time.Time) error {
		return errors.New("bd de tokens caída al guardar")
	}
	caso := m.casoDeUso()

	err := caso.Reenviar(context.Background(), comandoReenviarValido(t))
	if err != nil {
		t.Fatalf("un fallo interno no debe filtrarse como error distinto al camino feliz (INV-ID-22), obtuvo %v", err)
	}
	if len(m.notificadorCorreo.LlamadasEnviarVerificacion) != 0 {
		t.Error("si el token nuevo no se pudo guardar, no debe intentar enviarse el correo")
	}
}

func TestReenviarVerificacionCasoDeUso_FalloAlEnviarCorreo_RespuestaNeutra(t *testing.T) {
	id := idDePrueba(t, idUsuarioValido1)
	correo := correoDePrueba(t, correoValidoValor)
	hash := hashDePrueba(t, "hashvigente")
	usuario := usuarioPendienteDePrueba(t, id, correo, hash)

	m := nuevosMocksReenviar(t, usuario)
	m.notificadorCorreo.FnEnviarVerificacion = func(ctx context.Context, correo dominio.Correo, tokenPlano string) error {
		return errors.New("proveedor de correo no disponible")
	}
	caso := m.casoDeUso()

	err := caso.Reenviar(context.Background(), comandoReenviarValido(t))
	if err != nil {
		t.Fatalf("un fallo al enviar el correo no debe filtrarse como error distinto al camino feliz (INV-ID-22), obtuvo %v", err)
	}
	if len(m.tokensVerificacion.LlamadasGuardar) != 1 {
		t.Error("el token ya se guardó antes del intento de envío; eso no debe deshacerse")
	}
}

// TestReenviarVerificacionCasoDeUso_INVID22_RespuestaIdenticaEnTodosLosEscenarios
// es la salvaguarda explícita de INV-ID-22: recorre un conjunto amplio de
// escenarios (cuenta inexistente, correo malformado, cada estado no
// elegible, y varios fallos internos) y confirma que el único resultado
// observable posible — el valor de error devuelto — es siempre nil, sin
// excepción. Como puertos.ReenviadorDeVerificacion.Reenviar no expone
// ningún otro dato en su firma, esto agota la superficie observable del
// puerto de entrada.
func TestReenviarVerificacionCasoDeUso_INVID22_RespuestaIdenticaEnTodosLosEscenarios(t *testing.T) {
	id := idDePrueba(t, idUsuarioValido1)
	correo := correoDePrueba(t, correoValidoValor)
	hash := hashDePrueba(t, "hashvigente")

	casos := map[string]func() *mocksReenviar{
		"cuenta inexistente": func() *mocksReenviar {
			return nuevosMocksReenviar(t, nil)
		},
		"correo malformado": func() *mocksReenviar {
			return nuevosMocksReenviar(t, nil)
		},
		"usuario activo": func() *mocksReenviar {
			return nuevosMocksReenviar(t, usuarioActivoDePrueba(t, id, correo, hash))
		},
		"usuario suspendido": func() *mocksReenviar {
			return nuevosMocksReenviar(t, usuarioSuspendidoDePrueba(t, id, correo, hash))
		},
		"usuario bloqueado": func() *mocksReenviar {
			return nuevosMocksReenviar(t, usuarioBloqueadoDePrueba(t, id, correo, hash))
		},
		"usuario pendiente, camino feliz": func() *mocksReenviar {
			return nuevosMocksReenviar(t, usuarioPendienteDePrueba(t, id, correo, hash))
		},
		"usuario pendiente, fallo interno al generar token": func() *mocksReenviar {
			m := nuevosMocksReenviar(t, usuarioPendienteDePrueba(t, id, correo, hash))
			m.generadorTokens.FnGenerar = func() (string, error) { return "", errors.New("falla") }
			return m
		},
		"usuario pendiente, fallo interno al guardar token": func() *mocksReenviar {
			m := nuevosMocksReenviar(t, usuarioPendienteDePrueba(t, id, correo, hash))
			m.tokensVerificacion.FnGuardar = func(ctx context.Context, usuarioID dominio.IDUsuario, hashToken string, expiraEn time.Time) error {
				return errors.New("falla")
			}
			return m
		},
		"usuario pendiente, fallo interno al notificar": func() *mocksReenviar {
			m := nuevosMocksReenviar(t, usuarioPendienteDePrueba(t, id, correo, hash))
			m.notificadorCorreo.FnEnviarVerificacion = func(ctx context.Context, correo dominio.Correo, tokenPlano string) error {
				return errors.New("falla")
			}
			return m
		},
	}

	for nombre, construir := range casos {
		t.Run(nombre, func(t *testing.T) {
			m := construir()
			caso := m.casoDeUso()
			cmd := comandoReenviarValido(t)
			if nombre == "correo malformado" {
				cmd.Correo = "no-es-un-correo"
			}
			err := caso.Reenviar(context.Background(), cmd)
			if err != nil {
				t.Errorf("INV-ID-22: la respuesta debe ser siempre neutra (nil), obtuvo %v", err)
			}
		})
	}
}

// TestReenviadorDeVerificacion_ImplementaPuertoEntrada es una salvaguarda
// de compilación: el caso de uso debe satisfacer la interfaz de entrada del
// diseño (sección 3.4).
func TestReenviadorDeVerificacion_ImplementaPuertoEntrada(t *testing.T) {
	var _ puertos.ReenviadorDeVerificacion = (*aplicacion.ReenviarVerificacionCasoDeUso)(nil)
}
