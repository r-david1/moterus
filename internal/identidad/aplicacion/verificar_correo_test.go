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

const tokenVerificacionValor = "token-plano-de-prueba-alta-entropia"

// mocksVerificar agrupa los test doubles de las seis dependencias de
// VerificarCorreoCasoDeUso. Por defecto simula un token válido y vigente
// que resuelve a un usuario en pendiente_verificacion.
type mocksVerificar struct {
	usuarios           *mocks.RepositorioUsuarios
	tokensVerificacion *mocks.RepositorioTokensVerificacion
	auditoria          *mocks.RegistroAuditoria
	eventos            *mocks.PublicadorEventos
	reloj              *mocks.Reloj
	uow                *mocks.UnidadDeTrabajo
}

func nuevosMocksVerificar(t *testing.T, usuario *dominio.Usuario) *mocksVerificar {
	t.Helper()
	id := idDePrueba(t, idUsuarioValido1)
	return &mocksVerificar{
		usuarios: &mocks.RepositorioUsuarios{
			FnBuscarPorID: func(ctx context.Context, buscado dominio.IDUsuario) (*dominio.Usuario, error) {
				if usuario != nil && usuario.ID().EsIgual(buscado) {
					return usuario, nil
				}
				return nil, nil
			},
		},
		tokensVerificacion: &mocks.RepositorioTokensVerificacion{
			FnBuscarPorHash: func(ctx context.Context, hashToken string) (dominio.IDUsuario, time.Time, bool, error) {
				return id, ahoraDePrueba().Add(vigenciaTokenDePruebaFutura), true, nil
			},
		},
		auditoria: &mocks.RegistroAuditoria{},
		eventos:   &mocks.PublicadorEventos{},
		reloj:     &mocks.Reloj{Fija: ahoraDePrueba()},
		uow:       &mocks.UnidadDeTrabajo{},
	}
}

// vigenciaTokenDePruebaFutura mantiene el token de los fixtures vigente
// respecto de ahoraDePrueba().
const vigenciaTokenDePruebaFutura = 24 * time.Hour

func (m *mocksVerificar) casoDeUso() *aplicacion.VerificarCorreoCasoDeUso {
	return aplicacion.NuevoVerificarCorreoCasoDeUso(
		m.usuarios, m.tokensVerificacion, m.auditoria, m.eventos, m.reloj, m.uow,
	)
}

func comandoVerificarValido(t *testing.T) aplicacion.ComandoVerificarCorreo {
	t.Helper()
	return aplicacion.ComandoVerificarCorreo{
		TokenPlano: tokenVerificacionValor,
		Origen:     origenDePrueba(t),
	}
}

func TestVerificarCorreoCasoDeUso_FlujoFeliz(t *testing.T) {
	id := idDePrueba(t, idUsuarioValido1)
	correo := correoDePrueba(t, correoValidoValor)
	hash := hashDePrueba(t, "hashvigente")
	usuario := usuarioPendienteDePrueba(t, id, correo, hash)

	m := nuevosMocksVerificar(t, usuario)
	caso := m.casoDeUso()

	err := caso.Verificar(context.Background(), comandoVerificarValido(t))
	if err != nil {
		t.Fatalf("Verificar() devolvió error inesperado: %v", err)
	}

	if usuario.Estado().String() != "activo" {
		t.Errorf("Estado tras verificar = %q, esperado activo", usuario.Estado().String())
	}
	if len(m.usuarios.LlamadasGuardar) != 1 {
		t.Fatalf("se esperaba exactamente una llamada a Guardar, hubo %d", len(m.usuarios.LlamadasGuardar))
	}
	if len(m.tokensVerificacion.LlamadasEliminar) != 1 || !m.tokensVerificacion.LlamadasEliminar[0].EsIgual(id) {
		t.Errorf("se esperaba Eliminar(usuarioID) exactamente una vez, llamadas: %v", m.tokensVerificacion.LlamadasEliminar)
	}
	if nombres := m.auditoria.NombresEventos(); len(nombres) != 1 || nombres[0] != "CorreoVerificado" {
		t.Errorf("eventos auditados = %v, esperado [CorreoVerificado]", nombres)
	}
	if len(m.eventos.LlamadasPublicar) != 1 {
		t.Errorf("se esperaba una publicación de eventos, hubo %d", len(m.eventos.LlamadasPublicar))
	}

	// INV-ID-21 (indirecto): el hash buscado nunca es igual al token plano
	// recibido — si lo fuera, el token estaría viajando sin hashear hasta
	// el puerto de salida.
	if len(m.tokensVerificacion.LlamadasBuscarPorHash) != 1 {
		t.Fatalf("se esperaba una llamada a BuscarPorHash, hubo %d", len(m.tokensVerificacion.LlamadasBuscarPorHash))
	}
	if m.tokensVerificacion.LlamadasBuscarPorHash[0] == tokenVerificacionValor {
		t.Error("el hash buscado no debe ser igual al token en claro (INV-ID-21)")
	}
}

func TestVerificarCorreoCasoDeUso_TokenNoEncontrado(t *testing.T) {
	m := nuevosMocksVerificar(t, nil)
	m.tokensVerificacion.FnBuscarPorHash = func(ctx context.Context, hashToken string) (dominio.IDUsuario, time.Time, bool, error) {
		return dominio.IDUsuario{}, time.Time{}, false, nil
	}
	caso := m.casoDeUso()

	err := caso.Verificar(context.Background(), comandoVerificarValido(t))

	var errInvalido *dominio.ErrTokenVerificacionInvalido
	if !errors.As(err, &errInvalido) {
		t.Fatalf("se esperaba *ErrTokenVerificacionInvalido, obtuvo %T: %v", err, err)
	}
	if len(m.usuarios.LlamadasGuardar) != 0 {
		t.Error("un token no encontrado no debe llegar a persistir ningún usuario")
	}
	if nombres := m.auditoria.NombresEventos(); len(nombres) != 1 || nombres[0] != "VerificacionCorreoFallida" {
		t.Errorf("eventos auditados = %v, esperado [VerificacionCorreoFallida]", nombres)
	}
	if len(m.auditoria.LlamadasRegistrar) == 1 {
		evento, ok := m.auditoria.LlamadasRegistrar[0].Evento.(dominio.VerificacionCorreoFallida)
		if !ok {
			t.Fatalf("evento auditado no es VerificacionCorreoFallida: %T", m.auditoria.LlamadasRegistrar[0].Evento)
		}
		if evento.IDUsuario != "" {
			t.Errorf("IDUsuario del evento = %q, esperado vacío (token no resuelto a ningún usuario)", evento.IDUsuario)
		}
		if evento.Motivo != "token_invalido" {
			t.Errorf("Motivo = %q, esperado token_invalido", evento.Motivo)
		}
	}
}

func TestVerificarCorreoCasoDeUso_TokenExpirado(t *testing.T) {
	id := idDePrueba(t, idUsuarioValido1)
	m := nuevosMocksVerificar(t, nil)
	m.tokensVerificacion.FnBuscarPorHash = func(ctx context.Context, hashToken string) (dominio.IDUsuario, time.Time, bool, error) {
		return id, ahoraDePrueba().Add(-1 * time.Hour), true, nil
	}
	caso := m.casoDeUso()

	err := caso.Verificar(context.Background(), comandoVerificarValido(t))

	var errExpirado *dominio.ErrTokenVerificacionExpirado
	if !errors.As(err, &errExpirado) {
		t.Fatalf("se esperaba *ErrTokenVerificacionExpirado, obtuvo %T: %v", err, err)
	}
	if len(m.tokensVerificacion.LlamadasEliminar) != 1 || !m.tokensVerificacion.LlamadasEliminar[0].EsIgual(id) {
		t.Errorf("un token expirado debe eliminarse, llamadas: %v", m.tokensVerificacion.LlamadasEliminar)
	}
	if len(m.usuarios.LlamadasGuardar) != 0 {
		t.Error("un token expirado no debe mutar el agregado Usuario")
	}
	if nombres := m.auditoria.NombresEventos(); len(nombres) != 1 || nombres[0] != "VerificacionCorreoFallida" {
		t.Errorf("eventos auditados = %v, esperado [VerificacionCorreoFallida]", nombres)
	}
	evento, ok := m.auditoria.LlamadasRegistrar[0].Evento.(dominio.VerificacionCorreoFallida)
	if !ok {
		t.Fatalf("evento auditado no es VerificacionCorreoFallida: %T", m.auditoria.LlamadasRegistrar[0].Evento)
	}
	if evento.IDUsuario != id.String() {
		t.Errorf("IDUsuario del evento = %q, esperado %q", evento.IDUsuario, id.String())
	}
	if evento.Motivo != "token_expirado" {
		t.Errorf("Motivo = %q, esperado token_expirado", evento.Motivo)
	}
}

func TestVerificarCorreoCasoDeUso_BuscarPorHashError_SePropaga(t *testing.T) {
	m := nuevosMocksVerificar(t, nil)
	errRepo := errors.New("bd de tokens caída")
	m.tokensVerificacion.FnBuscarPorHash = func(ctx context.Context, hashToken string) (dominio.IDUsuario, time.Time, bool, error) {
		return dominio.IDUsuario{}, time.Time{}, false, errRepo
	}
	caso := m.casoDeUso()

	err := caso.Verificar(context.Background(), comandoVerificarValido(t))
	if !errors.Is(err, errRepo) {
		t.Fatalf("se esperaba que el error del repositorio se propagara, obtuvo %v", err)
	}
}

func TestVerificarCorreoCasoDeUso_UsuarioNoEncontrado(t *testing.T) {
	// El token es válido y vigente pero el usuario que referencia no
	// existe (token huérfano): se trata como token inválido.
	m := nuevosMocksVerificar(t, nil)
	caso := m.casoDeUso()

	err := caso.Verificar(context.Background(), comandoVerificarValido(t))

	var errInvalido *dominio.ErrTokenVerificacionInvalido
	if !errors.As(err, &errInvalido) {
		t.Fatalf("se esperaba *ErrTokenVerificacionInvalido, obtuvo %T: %v", err, err)
	}
	if nombres := m.auditoria.NombresEventos(); len(nombres) != 1 || nombres[0] != "VerificacionCorreoFallida" {
		t.Errorf("eventos auditados = %v, esperado [VerificacionCorreoFallida]", nombres)
	}
}

func TestVerificarCorreoCasoDeUso_BuscarPorIDError_SePropaga(t *testing.T) {
	m := nuevosMocksVerificar(t, nil)
	errRepo := errors.New("bd de usuarios caída")
	m.usuarios.FnBuscarPorID = func(ctx context.Context, id dominio.IDUsuario) (*dominio.Usuario, error) {
		return nil, errRepo
	}
	caso := m.casoDeUso()

	err := caso.Verificar(context.Background(), comandoVerificarValido(t))
	if !errors.Is(err, errRepo) {
		t.Fatalf("se esperaba que el error del repositorio se propagara, obtuvo %v", err)
	}
}

func TestVerificarCorreoCasoDeUso_TransicionInvalida(t *testing.T) {
	id := idDePrueba(t, idUsuarioValido1)
	correo := correoDePrueba(t, correoValidoValor)
	hash := hashDePrueba(t, "hashvigente")
	usuario := usuarioActivoDePrueba(t, id, correo, hash) // ya activo: ConfirmarCorreo fallará

	m := nuevosMocksVerificar(t, usuario)
	caso := m.casoDeUso()

	err := caso.Verificar(context.Background(), comandoVerificarValido(t))

	var errTransicion *dominio.ErrTransicionEstadoInvalida
	if !errors.As(err, &errTransicion) {
		t.Fatalf("se esperaba *ErrTransicionEstadoInvalida, obtuvo %T: %v", err, err)
	}
	if len(m.usuarios.LlamadasGuardar) != 0 {
		t.Error("una transición inválida no debe persistir el usuario")
	}
	if nombres := m.auditoria.NombresEventos(); len(nombres) != 1 || nombres[0] != "VerificacionCorreoFallida" {
		t.Errorf("eventos auditados = %v, esperado [VerificacionCorreoFallida]", nombres)
	}
}

func TestVerificarCorreoCasoDeUso_FalloDeAuditoriaTrasNoEncontrado_SePropaga(t *testing.T) {
	m := nuevosMocksVerificar(t, nil)
	m.tokensVerificacion.FnBuscarPorHash = func(ctx context.Context, hashToken string) (dominio.IDUsuario, time.Time, bool, error) {
		return dominio.IDUsuario{}, time.Time{}, false, nil
	}
	errAuditoria := errors.New("bd de auditoría caída")
	m.auditoria.FnRegistrar = func(ctx context.Context, e dominio.EventoDominio, origen dominio.OrigenSolicitud) error {
		return errAuditoria
	}
	caso := m.casoDeUso()

	err := caso.Verificar(context.Background(), comandoVerificarValido(t))
	if !errors.Is(err, errAuditoria) {
		t.Fatalf("se esperaba que el error de auditoría se propagara, obtuvo %v", err)
	}
}

func TestVerificarCorreoCasoDeUso_FalloDePersistenciaAbortaLaVerificacion(t *testing.T) {
	id := idDePrueba(t, idUsuarioValido1)
	correo := correoDePrueba(t, correoValidoValor)
	hash := hashDePrueba(t, "hashvigente")
	usuario := usuarioPendienteDePrueba(t, id, correo, hash)

	m := nuevosMocksVerificar(t, usuario)
	errGuardar := errors.New("violación de restricción en la base de datos")
	m.usuarios.FnGuardar = func(ctx context.Context, u *dominio.Usuario) error { return errGuardar }
	caso := m.casoDeUso()

	err := caso.Verificar(context.Background(), comandoVerificarValido(t))
	if !errors.Is(err, errGuardar) {
		t.Fatalf("se esperaba que el error de persistencia se propagara, obtuvo %v", err)
	}
	if len(m.tokensVerificacion.LlamadasEliminar) != 0 {
		t.Error("si Guardar falla dentro de la UnidadDeTrabajo, no debe llegar a eliminarse el token")
	}
	if len(m.auditoria.LlamadasRegistrar) != 0 {
		t.Error("si Guardar falla dentro de la UnidadDeTrabajo, no debe llegar a auditarse CorreoVerificado")
	}
	if len(m.eventos.LlamadasPublicar) != 0 {
		t.Error("una verificación abortada no debe publicar eventos")
	}
}

// TestVerificarCorreoCasoDeUso_FalloAlEliminarTokenTrasConfirmar_SePropaga
// cubre el paso de limpieza del camino feliz: si el token consumido no se
// pudo eliminar dentro de la misma transacción que confirmó el correo, la
// UnidadDeTrabajo debe abortar (ADR 0005): no puede quedar un usuario
// activo con un token de un solo uso todavía reutilizable.
func TestVerificarCorreoCasoDeUso_FalloAlEliminarTokenTrasConfirmar_SePropaga(t *testing.T) {
	id := idDePrueba(t, idUsuarioValido1)
	correo := correoDePrueba(t, correoValidoValor)
	hash := hashDePrueba(t, "hashvigente")
	usuario := usuarioPendienteDePrueba(t, id, correo, hash)

	m := nuevosMocksVerificar(t, usuario)
	errEliminar := errors.New("bd de tokens caída al eliminar")
	m.tokensVerificacion.FnEliminar = func(ctx context.Context, usuarioID dominio.IDUsuario) error { return errEliminar }
	caso := m.casoDeUso()

	err := caso.Verificar(context.Background(), comandoVerificarValido(t))
	if !errors.Is(err, errEliminar) {
		t.Fatalf("se esperaba que el error al eliminar el token se propagara, obtuvo %v", err)
	}
	if len(m.eventos.LlamadasPublicar) != 0 {
		t.Error("una verificación abortada no debe publicar eventos")
	}
}

func TestVerificarCorreoCasoDeUso_FalloAlEliminarTokenExpirado_SePropaga(t *testing.T) {
	id := idDePrueba(t, idUsuarioValido1)
	m := nuevosMocksVerificar(t, nil)
	m.tokensVerificacion.FnBuscarPorHash = func(ctx context.Context, hashToken string) (dominio.IDUsuario, time.Time, bool, error) {
		return id, ahoraDePrueba().Add(-1 * time.Hour), true, nil
	}
	errEliminar := errors.New("bd de tokens caída al eliminar")
	m.tokensVerificacion.FnEliminar = func(ctx context.Context, usuarioID dominio.IDUsuario) error { return errEliminar }
	caso := m.casoDeUso()

	err := caso.Verificar(context.Background(), comandoVerificarValido(t))
	if !errors.Is(err, errEliminar) {
		t.Fatalf("se esperaba que el error al eliminar el token expirado se propagara, obtuvo %v", err)
	}
}

func TestVerificarCorreoCasoDeUso_FalloAlPublicarEventos_NoBloqueaLaVerificacion(t *testing.T) {
	id := idDePrueba(t, idUsuarioValido1)
	correo := correoDePrueba(t, correoValidoValor)
	hash := hashDePrueba(t, "hashvigente")
	usuario := usuarioPendienteDePrueba(t, id, correo, hash)

	m := nuevosMocksVerificar(t, usuario)
	m.eventos.FnPublicar = func(ctx context.Context, eventos ...dominio.EventoDominio) error {
		return errors.New("cola de eventos no disponible")
	}
	caso := m.casoDeUso()

	err := caso.Verificar(context.Background(), comandoVerificarValido(t))
	if err != nil {
		t.Fatalf("un fallo al publicar eventos no debe abortar la verificación ya persistida: %v", err)
	}
	if usuario.Estado().String() != "activo" {
		t.Errorf("Estado tras verificar = %q, esperado activo", usuario.Estado().String())
	}
}

func TestVerificarCorreoCasoDeUso_FalloDeAuditoriaTrasUsuarioNoEncontrado_SePropaga(t *testing.T) {
	m := nuevosMocksVerificar(t, nil)
	errAuditoria := errors.New("bd de auditoría caída")
	m.auditoria.FnRegistrar = func(ctx context.Context, e dominio.EventoDominio, origen dominio.OrigenSolicitud) error {
		return errAuditoria
	}
	caso := m.casoDeUso()

	// nuevosMocksVerificar(t, nil) ya deja BuscarPorID devolviendo nil,nil
	// para cualquier ID: el token es válido y vigente pero el usuario no
	// existe.
	err := caso.Verificar(context.Background(), comandoVerificarValido(t))
	if !errors.Is(err, errAuditoria) {
		t.Fatalf("se esperaba que el error de auditoría se propagara, obtuvo %v", err)
	}
}

func TestVerificarCorreoCasoDeUso_FalloDeAuditoriaTrasTransicionInvalida_SePropaga(t *testing.T) {
	id := idDePrueba(t, idUsuarioValido1)
	correo := correoDePrueba(t, correoValidoValor)
	hash := hashDePrueba(t, "hashvigente")
	usuario := usuarioActivoDePrueba(t, id, correo, hash)

	m := nuevosMocksVerificar(t, usuario)
	errAuditoria := errors.New("bd de auditoría caída")
	m.auditoria.FnRegistrar = func(ctx context.Context, e dominio.EventoDominio, origen dominio.OrigenSolicitud) error {
		return errAuditoria
	}
	caso := m.casoDeUso()

	err := caso.Verificar(context.Background(), comandoVerificarValido(t))
	if !errors.Is(err, errAuditoria) {
		t.Fatalf("se esperaba que el error de auditoría se propagara, obtuvo %v", err)
	}
}

// TestVerificarCorreoCasoDeUso_FalloDeAuditoriaDentroDeLaTransaccionExitosaAborta
// cubre INV-ID-15 en el camino feliz: si Guardar tuvo éxito pero la
// auditoría de CorreoVerificado no pudo registrarse en la misma
// transacción, la UnidadDeTrabajo debe abortar.
func TestVerificarCorreoCasoDeUso_FalloDeAuditoriaDentroDeLaTransaccionExitosaAborta(t *testing.T) {
	id := idDePrueba(t, idUsuarioValido1)
	correo := correoDePrueba(t, correoValidoValor)
	hash := hashDePrueba(t, "hashvigente")
	usuario := usuarioPendienteDePrueba(t, id, correo, hash)

	m := nuevosMocksVerificar(t, usuario)
	errAuditoria := errors.New("bd de auditoría caída")
	m.auditoria.FnRegistrar = func(ctx context.Context, e dominio.EventoDominio, origen dominio.OrigenSolicitud) error {
		return errAuditoria
	}
	caso := m.casoDeUso()

	err := caso.Verificar(context.Background(), comandoVerificarValido(t))
	if !errors.Is(err, errAuditoria) {
		t.Fatalf("se esperaba que el error de auditoría se propagara, obtuvo %v", err)
	}
	if len(m.eventos.LlamadasPublicar) != 0 {
		t.Error("una verificación abortada no debe publicar eventos")
	}
}

// TestVerificarCorreoCasoDeUso_ImplementaPuertoEntrada es una salvaguarda de
// compilación: el caso de uso debe satisfacer la interfaz de entrada del
// diseño (sección 3.4).
func TestVerificarCorreoCasoDeUso_ImplementaPuertoEntrada(t *testing.T) {
	var _ puertos.VerificadorDeCorreo = (*aplicacion.VerificarCorreoCasoDeUso)(nil)
}
