package aplicacion_test

import (
	"context"
	"errors"
	"testing"

	"github.com/r-david1/moterus/internal/identidad/aplicacion"
	"github.com/r-david1/moterus/internal/identidad/dominio"
	"github.com/r-david1/moterus/internal/identidad/puertos"
	"github.com/r-david1/moterus/internal/identidad/puertos/mocks"
)

// mocksAutenticar agrupa los test doubles de las siete dependencias de
// AutenticarUsuarioCasoDeUso, configurados con un camino feliz razonable:
// Confianza permite, el repositorio devuelve un usuario activo cuya
// contraseña coincide, y no hace falta rehash.
type mocksAutenticar struct {
	usuarios  *mocks.RepositorioUsuarios
	hasher    *mocks.HasherContrasenas
	confianza *mocks.EvaluadorConfianza
	auditoria *mocks.RegistroAuditoria
	eventos   *mocks.PublicadorEventos
	reloj     *mocks.Reloj
	uow       *mocks.UnidadDeTrabajo
}

// nuevosMocksAutenticar construye los mocks con un usuario activo de
// fixture cuya contraseña en claro es contrasenaFuerteValor: Verificar
// devuelve true solo si la contraseña recibida coincide con esa constante,
// simulando un hasher real de forma barata.
func nuevosMocksAutenticar(t *testing.T, usuario *dominio.Usuario) *mocksAutenticar {
	t.Helper()
	return &mocksAutenticar{
		usuarios: &mocks.RepositorioUsuarios{
			FnBuscarPorCorreo: func(ctx context.Context, c dominio.Correo) (*dominio.Usuario, error) {
				if usuario != nil && usuario.Correo().Normalizado() == c.Normalizado() {
					return usuario, nil
				}
				return nil, &dominio.ErrUsuarioNoEncontrado{IDUsuario: ""}
			},
		},
		hasher: &mocks.HasherContrasenas{
			FnVerificar: func(ctx context.Context, h dominio.HashContrasena, p dominio.ContrasenaPlana) (bool, error) {
				return p.Valor() == contrasenaFuerteValor, nil
			},
		},
		confianza: &mocks.EvaluadorConfianza{},
		auditoria: &mocks.RegistroAuditoria{},
		eventos:   &mocks.PublicadorEventos{},
		reloj:     &mocks.Reloj{Fija: ahoraDePrueba()},
		uow:       &mocks.UnidadDeTrabajo{},
	}
}

func (m *mocksAutenticar) casoDeUso() *aplicacion.AutenticarUsuarioCasoDeUso {
	return aplicacion.NuevoAutenticarUsuarioCasoDeUso(
		m.usuarios, m.hasher, m.confianza, m.auditoria, m.eventos, m.reloj, m.uow,
	)
}

func comandoAutenticarValido(t *testing.T) aplicacion.ComandoAutenticar {
	t.Helper()
	return aplicacion.ComandoAutenticar{
		Correo:     correoValidoValor,
		Contrasena: contrasenaFuerteValor,
		Origen:     origenDePrueba(t),
	}
}

func TestAutenticarUsuarioCasoDeUso_FlujoFeliz(t *testing.T) {
	id := idDePrueba(t, idUsuarioValido1)
	correo := correoDePrueba(t, correoValidoValor)
	hash := hashDePrueba(t, "hashvigente")
	usuario := usuarioActivoDePrueba(t, id, correo, hash)

	m := nuevosMocksAutenticar(t, usuario)
	caso := m.casoDeUso()

	resultado, err := caso.Autenticar(context.Background(), comandoAutenticarValido(t))
	if err != nil {
		t.Fatalf("Autenticar() devolvió error inesperado: %v", err)
	}
	if resultado.IDUsuario != idUsuarioValido1 {
		t.Errorf("IDUsuario = %q, esperado %q", resultado.IDUsuario, idUsuarioValido1)
	}
	if resultado.CorreoNormalizado != correoValidoValor {
		t.Errorf("CorreoNormalizado = %q, esperado %q", resultado.CorreoNormalizado, correoValidoValor)
	}
	if resultado.Estado != "activo" {
		t.Errorf("Estado = %q, esperado activo", resultado.Estado)
	}
	if resultado.RequiereSegundoFactor {
		t.Error("no se esperaba requerir segundo factor")
	}
	if resultado.MotivoStepUp != "" {
		t.Errorf("MotivoStepUp = %q, esperado vacío", resultado.MotivoStepUp)
	}

	if len(m.usuarios.LlamadasGuardar) != 1 {
		t.Fatalf("se esperaba una llamada a Guardar, hubo %d", len(m.usuarios.LlamadasGuardar))
	}
	if _, ok := m.usuarios.LlamadasGuardar[0].UltimoAccesoEn(); !ok {
		t.Error("RegistrarAcceso debía haberse invocado antes de persistir")
	}
	if nombres := m.auditoria.NombresEventos(); len(nombres) != 1 || nombres[0] != "AutenticacionExitosa" {
		t.Errorf("eventos auditados = %v, esperado [AutenticacionExitosa]", nombres)
	}
	if len(m.confianza.LlamadasRegistrarResultado) != 1 || !m.confianza.LlamadasRegistrarResultado[0].Exitoso {
		t.Error("se esperaba EvaluadorConfianza.RegistrarResultado(exitoso=true)")
	}
	if m.hasher.LlamadasConsumirTiempoEquivalente != 0 {
		t.Error("un login exitoso no debe consumir el tiempo equivalente señuelo")
	}
}

func TestAutenticarUsuarioCasoDeUso_ConfianzaDeniega(t *testing.T) {
	id := idDePrueba(t, idUsuarioValido1)
	correo := correoDePrueba(t, correoValidoValor)
	hash := hashDePrueba(t, "hashvigente")
	usuario := usuarioActivoDePrueba(t, id, correo, hash)

	m := nuevosMocksAutenticar(t, usuario)
	m.confianza.FnEvaluar = func(ctx context.Context, s puertos.SolicitudEvaluacion) (puertos.DecisionConfianza, error) {
		return puertos.DecisionConfianza{Permitido: false, Motivo: "credential_stuffing"}, nil
	}
	caso := m.casoDeUso()

	_, err := caso.Autenticar(context.Background(), comandoAutenticarValido(t))
	var errConfianza *dominio.ErrAccesoDenegadoPorConfianza
	if !errors.As(err, &errConfianza) {
		t.Fatalf("se esperaba *ErrAccesoDenegadoPorConfianza, obtuvo %T: %v", err, err)
	}
	// INV-ID-12: ningún intento llega al repositorio sin pasar antes por
	// Confianza.
	if len(m.usuarios.LlamadasBuscarPorCorreo) != 0 {
		t.Error("un intento denegado por Confianza no debe llegar a consultar el repositorio")
	}
	if nombres := m.auditoria.NombresEventos(); len(nombres) != 1 || nombres[0] != "AutenticacionDenegada" {
		t.Errorf("eventos auditados = %v, esperado [AutenticacionDenegada]", nombres)
	}
}

func TestAutenticarUsuarioCasoDeUso_ConfianzaDeniega_FalloAuditoriaSePropaga(t *testing.T) {
	m := nuevosMocksAutenticar(t, nil)
	errAuditoria := errors.New("bd de auditoría caída")
	m.confianza.FnEvaluar = func(ctx context.Context, s puertos.SolicitudEvaluacion) (puertos.DecisionConfianza, error) {
		return puertos.DecisionConfianza{Permitido: false, Motivo: "riesgo_alto"}, nil
	}
	m.auditoria.FnRegistrar = func(ctx context.Context, e dominio.EventoDominio, origen dominio.OrigenSolicitud) error {
		return errAuditoria
	}
	caso := m.casoDeUso()

	_, err := caso.Autenticar(context.Background(), comandoAutenticarValido(t))
	if !errors.Is(err, errAuditoria) {
		t.Fatalf("INV-ID-15: se esperaba que el error de auditoría se propagara, obtuvo %v", err)
	}
}

func TestAutenticarUsuarioCasoDeUso_ConfianzaEvaluarError_SePropaga(t *testing.T) {
	m := nuevosMocksAutenticar(t, nil)
	errConexion := errors.New("confianza no disponible")
	m.confianza.FnEvaluar = func(ctx context.Context, s puertos.SolicitudEvaluacion) (puertos.DecisionConfianza, error) {
		return puertos.DecisionConfianza{}, errConexion
	}
	caso := m.casoDeUso()

	_, err := caso.Autenticar(context.Background(), comandoAutenticarValido(t))
	if !errors.Is(err, errConexion) {
		t.Fatalf("se esperaba que el error de EvaluadorConfianza se propagara, obtuvo %v", err)
	}
}

// TestAutenticarUsuarioCasoDeUso_CorreoInvalido_MismoErrorGenerico cubre
// INV-ID-11 parcialmente: un correo malformado no debe distinguirse de uno
// inexistente, ni siquiera en el tipo de error.
func TestAutenticarUsuarioCasoDeUso_CorreoInvalido_MismoErrorGenerico(t *testing.T) {
	m := nuevosMocksAutenticar(t, nil)
	caso := m.casoDeUso()
	cmd := comandoAutenticarValido(t)
	cmd.Correo = "no-es-un-correo"

	_, err := caso.Autenticar(context.Background(), cmd)
	var errCred *dominio.ErrCredencialesInvalidas
	if !errors.As(err, &errCred) {
		t.Fatalf("se esperaba *ErrCredencialesInvalidas (no ErrCorreoInvalido), obtuvo %T: %v", err, err)
	}
	if m.hasher.LlamadasConsumirTiempoEquivalente != 1 {
		t.Errorf("se esperaba ConsumirTiempoEquivalente exactamente una vez, hubo %d", m.hasher.LlamadasConsumirTiempoEquivalente)
	}
	if len(m.usuarios.LlamadasBuscarPorCorreo) != 0 {
		t.Error("un correo malformado no debe llegar a consultar el repositorio")
	}
}

// TestINV_ID_11_UsuarioNoEncontradoYContrasenaIncorrecta_MismoErrorYTiempoEquivalente
// es el test explícito de INV-ID-11: correo inexistente y contraseña
// incorrecta deben devolver el mismo tipo de error, y solo el primer caso
// consume el tiempo equivalente señuelo (Verificar ya cuesta lo suficiente
// en el segundo).
func TestINV_ID_11_UsuarioNoEncontradoYContrasenaIncorrecta_MismoErrorYTiempoEquivalente(t *testing.T) {
	id := idDePrueba(t, idUsuarioValido1)
	correo := correoDePrueba(t, correoValidoValor)
	hash := hashDePrueba(t, "hashvigente")
	usuarioExistente := usuarioActivoDePrueba(t, id, correo, hash)

	// Caso A: el correo no existe.
	mNoExiste := nuevosMocksAutenticar(t, nil)
	casoNoExiste := mNoExiste.casoDeUso()
	_, errNoExiste := casoNoExiste.Autenticar(context.Background(), comandoAutenticarValido(t))

	var errCredNoExiste *dominio.ErrCredencialesInvalidas
	if !errors.As(errNoExiste, &errCredNoExiste) {
		t.Fatalf("caso 'usuario no encontrado': se esperaba *ErrCredencialesInvalidas, obtuvo %T: %v", errNoExiste, errNoExiste)
	}
	if mNoExiste.hasher.LlamadasConsumirTiempoEquivalente != 1 {
		t.Errorf("caso 'usuario no encontrado': se esperaba ConsumirTiempoEquivalente una vez, hubo %d", mNoExiste.hasher.LlamadasConsumirTiempoEquivalente)
	}

	// Caso B: el correo existe pero la contraseña es incorrecta.
	mIncorrecta := nuevosMocksAutenticar(t, usuarioExistente)
	casoIncorrecta := mIncorrecta.casoDeUso()
	cmdIncorrecta := comandoAutenticarValido(t)
	cmdIncorrecta.Contrasena = "esta-contrasena-no-es-la-correcta"
	_, errIncorrecta := casoIncorrecta.Autenticar(context.Background(), cmdIncorrecta)

	var errCredIncorrecta *dominio.ErrCredencialesInvalidas
	if !errors.As(errIncorrecta, &errCredIncorrecta) {
		t.Fatalf("caso 'contraseña incorrecta': se esperaba *ErrCredencialesInvalidas, obtuvo %T: %v", errIncorrecta, errIncorrecta)
	}
	// El diseño (sección 3.2, paso 4) documenta que en este caso no hace
	// falta tiempo equivalente adicional: Verificar ya consumió un coste
	// comparable.
	if mIncorrecta.hasher.LlamadasConsumirTiempoEquivalente != 0 {
		t.Errorf("caso 'contraseña incorrecta': no se esperaba ConsumirTiempoEquivalente, hubo %d", mIncorrecta.hasher.LlamadasConsumirTiempoEquivalente)
	}

	// El punto central de INV-ID-11: ambos casos son indistinguibles desde
	// fuera del dominio.
	if errNoExiste.Error() != errIncorrecta.Error() {
		t.Errorf("los mensajes de error deben ser idénticos: %q vs %q", errNoExiste.Error(), errIncorrecta.Error())
	}
}

// TestAutenticarUsuarioCasoDeUso_UsuarioNoEncontrado_ViaNilSinError cubre la
// otra convención posible del puerto (nil, nil) para "no encontrado", sin
// pasar por el tipo de error ErrUsuarioNoEncontrado.
func TestAutenticarUsuarioCasoDeUso_UsuarioNoEncontrado_ViaNilSinError(t *testing.T) {
	m := nuevosMocksAutenticar(t, nil)
	m.usuarios.FnBuscarPorCorreo = func(ctx context.Context, c dominio.Correo) (*dominio.Usuario, error) {
		return nil, nil
	}
	caso := m.casoDeUso()

	_, err := caso.Autenticar(context.Background(), comandoAutenticarValido(t))
	var errCred *dominio.ErrCredencialesInvalidas
	if !errors.As(err, &errCred) {
		t.Fatalf("se esperaba *ErrCredencialesInvalidas, obtuvo %T: %v", err, err)
	}
	if m.hasher.LlamadasConsumirTiempoEquivalente != 1 {
		t.Error("se esperaba ConsumirTiempoEquivalente")
	}
}

// TestAutenticarUsuarioCasoDeUso_BuscarPorCorreoErrorNoTipado_SePropagaSinSenuelo
// cubre que un error de infraestructura real (no "no encontrado") se
// propaga tal cual, sin gastar el señuelo criptográfico ni auditar como
// fallo de login: es un error operativo, no un intento de autenticación
// resuelto.
func TestAutenticarUsuarioCasoDeUso_BuscarPorCorreoErrorNoTipado_SePropagaSinSenuelo(t *testing.T) {
	m := nuevosMocksAutenticar(t, nil)
	errBD := errors.New("conexión a la base de datos perdida")
	m.usuarios.FnBuscarPorCorreo = func(ctx context.Context, c dominio.Correo) (*dominio.Usuario, error) {
		return nil, errBD
	}
	caso := m.casoDeUso()

	_, err := caso.Autenticar(context.Background(), comandoAutenticarValido(t))
	if !errors.Is(err, errBD) {
		t.Fatalf("se esperaba que el error de infraestructura se propagara tal cual, obtuvo %v", err)
	}
	if m.hasher.LlamadasConsumirTiempoEquivalente != 0 {
		t.Error("un error de infraestructura no debe consumir el tiempo equivalente señuelo")
	}
}

func TestAutenticarUsuarioCasoDeUso_ContrasenaEstructuralmenteInvalida(t *testing.T) {
	id := idDePrueba(t, idUsuarioValido1)
	correo := correoDePrueba(t, correoValidoValor)
	hash := hashDePrueba(t, "hashvigente")
	usuario := usuarioActivoDePrueba(t, id, correo, hash)

	m := nuevosMocksAutenticar(t, usuario)
	caso := m.casoDeUso()
	cmd := comandoAutenticarValido(t)
	cmd.Contrasena = ""

	_, err := caso.Autenticar(context.Background(), cmd)
	var errCred *dominio.ErrCredencialesInvalidas
	if !errors.As(err, &errCred) {
		t.Fatalf("se esperaba *ErrCredencialesInvalidas, obtuvo %T: %v", err, err)
	}
	if m.hasher.LlamadasVerificar != 0 {
		t.Error("una contraseña estructuralmente inválida no debe llegar a Verificar")
	}
	if nombres := m.auditoria.NombresEventos(); len(nombres) != 1 || nombres[0] != "AutenticacionFallida" {
		t.Errorf("eventos auditados = %v, esperado [AutenticacionFallida]", nombres)
	}
}

// TestAutenticarUsuarioCasoDeUso_VerificarDevuelveError_SePropagaSinAuditar
// cubre que un fallo del propio hasher (no "contraseña incorrecta") se
// propaga directamente sin pasar por registrarFalloLogin.
func TestAutenticarUsuarioCasoDeUso_VerificarDevuelveError_SePropagaSinAuditar(t *testing.T) {
	id := idDePrueba(t, idUsuarioValido1)
	correo := correoDePrueba(t, correoValidoValor)
	hash := hashDePrueba(t, "hashvigente")
	usuario := usuarioActivoDePrueba(t, id, correo, hash)

	m := nuevosMocksAutenticar(t, usuario)
	errHasher := errors.New("motor de hashing no disponible")
	m.hasher.FnVerificar = func(ctx context.Context, h dominio.HashContrasena, p dominio.ContrasenaPlana) (bool, error) {
		return false, errHasher
	}
	caso := m.casoDeUso()

	_, err := caso.Autenticar(context.Background(), comandoAutenticarValido(t))
	if !errors.Is(err, errHasher) {
		t.Fatalf("se esperaba que el error del hasher se propagara, obtuvo %v", err)
	}
	if len(m.auditoria.LlamadasRegistrar) != 0 {
		t.Error("un fallo del hasher (no del intento de login) no debe auditarse como AutenticacionFallida")
	}
}

func TestAutenticarUsuarioCasoDeUso_ContrasenaIncorrecta(t *testing.T) {
	id := idDePrueba(t, idUsuarioValido1)
	correo := correoDePrueba(t, correoValidoValor)
	hash := hashDePrueba(t, "hashvigente")
	usuario := usuarioActivoDePrueba(t, id, correo, hash)

	m := nuevosMocksAutenticar(t, usuario)
	caso := m.casoDeUso()
	cmd := comandoAutenticarValido(t)
	cmd.Contrasena = "definitivamente-no-es-la-contrasena"

	_, err := caso.Autenticar(context.Background(), cmd)
	var errCred *dominio.ErrCredencialesInvalidas
	if !errors.As(err, &errCred) {
		t.Fatalf("se esperaba *ErrCredencialesInvalidas, obtuvo %T: %v", err, err)
	}
	if len(m.usuarios.LlamadasGuardar) != 0 {
		t.Error("un login fallido no debe persistir el usuario")
	}
	if len(m.confianza.LlamadasRegistrarResultado) != 1 || m.confianza.LlamadasRegistrarResultado[0].Exitoso {
		t.Error("se esperaba RegistrarResultado(exitoso=false) en Confianza")
	}
}

func TestAutenticarUsuarioCasoDeUso_CorreoNoVerificado(t *testing.T) {
	id := idDePrueba(t, idUsuarioValido1)
	correo := correoDePrueba(t, correoValidoValor)
	hash := hashDePrueba(t, "hashvigente")
	usuario := usuarioPendienteDePrueba(t, id, correo, hash) // no confirmado

	m := nuevosMocksAutenticar(t, usuario)
	caso := m.casoDeUso()

	_, err := caso.Autenticar(context.Background(), comandoAutenticarValido(t))
	var errNoVerificado *dominio.ErrCorreoNoVerificado
	if !errors.As(err, &errNoVerificado) {
		t.Fatalf("se esperaba *ErrCorreoNoVerificado (INV-ID-06, error real tras verificar contraseña), obtuvo %T: %v", err, err)
	}
	if nombres := m.auditoria.NombresEventos(); len(nombres) != 1 || nombres[0] != "AutenticacionFallida" {
		t.Errorf("eventos auditados = %v, esperado [AutenticacionFallida]", nombres)
	}
}

func TestAutenticarUsuarioCasoDeUso_CuentaSuspendida(t *testing.T) {
	id := idDePrueba(t, idUsuarioValido1)
	correo := correoDePrueba(t, correoValidoValor)
	hash := hashDePrueba(t, "hashvigente")
	usuario := usuarioSuspendidoDePrueba(t, id, correo, hash)

	m := nuevosMocksAutenticar(t, usuario)
	caso := m.casoDeUso()

	_, err := caso.Autenticar(context.Background(), comandoAutenticarValido(t))
	var errSuspendida *dominio.ErrCuentaSuspendida
	if !errors.As(err, &errSuspendida) {
		t.Fatalf("se esperaba *ErrCuentaSuspendida, obtuvo %T: %v", err, err)
	}
}

func TestAutenticarUsuarioCasoDeUso_CuentaBloqueada(t *testing.T) {
	id := idDePrueba(t, idUsuarioValido1)
	correo := correoDePrueba(t, correoValidoValor)
	hash := hashDePrueba(t, "hashvigente")
	usuario := usuarioBloqueadoDePrueba(t, id, correo, hash)

	m := nuevosMocksAutenticar(t, usuario)
	caso := m.casoDeUso()

	_, err := caso.Autenticar(context.Background(), comandoAutenticarValido(t))
	var errBloqueada *dominio.ErrCuentaBloqueada
	if !errors.As(err, &errBloqueada) {
		t.Fatalf("se esperaba *ErrCuentaBloqueada, obtuvo %T: %v", err, err)
	}
}

// TestAutenticarUsuarioCasoDeUso_EstadoInvalido_FalloDeAuditoriaSePropaga
// cubre INV-ID-15 en el camino de "estado no operativo": si falla la
// auditoría del intento fallido, ese error se prioriza sobre el de negocio.
func TestAutenticarUsuarioCasoDeUso_EstadoInvalido_FalloDeAuditoriaSePropaga(t *testing.T) {
	id := idDePrueba(t, idUsuarioValido1)
	correo := correoDePrueba(t, correoValidoValor)
	hash := hashDePrueba(t, "hashvigente")
	usuario := usuarioSuspendidoDePrueba(t, id, correo, hash)

	m := nuevosMocksAutenticar(t, usuario)
	errAuditoria := errors.New("bd de auditoría caída")
	m.auditoria.FnRegistrar = func(ctx context.Context, e dominio.EventoDominio, origen dominio.OrigenSolicitud) error {
		return errAuditoria
	}
	caso := m.casoDeUso()

	_, err := caso.Autenticar(context.Background(), comandoAutenticarValido(t))
	if !errors.Is(err, errAuditoria) {
		t.Fatalf("se esperaba que el error de auditoría se propagara en vez de ErrCuentaSuspendida, obtuvo %v", err)
	}
}

// TestINV_ID_13_RehashOportunista_SeInvocaYPersiste cubre INV-ID-13: tras un
// login exitoso, si NecesitaRehash indica true, se rehashea y se persiste el
// nuevo hash, y se emite/audita CredencialRehasheada.
func TestINV_ID_13_RehashOportunista_SeInvocaYPersiste(t *testing.T) {
	id := idDePrueba(t, idUsuarioValido1)
	correo := correoDePrueba(t, correoValidoValor)
	hashViejo := hashDePrueba(t, "hashviejo")
	usuario := usuarioActivoDePrueba(t, id, correo, hashViejo)

	m := nuevosMocksAutenticar(t, usuario)
	hashNuevo := hashDePrueba(t, "hashnuevo")
	m.hasher.FnNecesitaRehash = func(h dominio.HashContrasena) bool { return true }
	m.hasher.FnHashear = func(ctx context.Context, p dominio.ContrasenaPlana) (dominio.HashContrasena, error) {
		return hashNuevo, nil
	}
	caso := m.casoDeUso()

	if _, err := caso.Autenticar(context.Background(), comandoAutenticarValido(t)); err != nil {
		t.Fatalf("Autenticar() devolvió error inesperado: %v", err)
	}

	if m.hasher.LlamadasHashear != 1 {
		t.Fatalf("se esperaba invocar Hashear para el rehash, hubo %d llamadas", m.hasher.LlamadasHashear)
	}
	if len(m.usuarios.LlamadasGuardar) != 1 {
		t.Fatalf("se esperaba una llamada a Guardar, hubo %d", len(m.usuarios.LlamadasGuardar))
	}
	guardado := m.usuarios.LlamadasGuardar[0]
	if guardado.Credencial().Hash().Valor() != hashNuevo.Valor() {
		t.Error("el rehash oportunista debía reemplazar y persistir el nuevo hash")
	}
	nombres := m.auditoria.NombresEventos()
	tieneRehash := false
	for _, n := range nombres {
		if n == "CredencialRehasheada" {
			tieneRehash = true
		}
	}
	if !tieneRehash {
		t.Errorf("eventos auditados = %v, se esperaba que incluyeran CredencialRehasheada", nombres)
	}
}

// TestINV_ID_13_RehashOportunista_FalloAlHashearNoAbortaElLogin cubre que un
// fallo del rehash oportunista nunca invalida el login ya autenticado.
func TestINV_ID_13_RehashOportunista_FalloAlHashearNoAbortaElLogin(t *testing.T) {
	id := idDePrueba(t, idUsuarioValido1)
	correo := correoDePrueba(t, correoValidoValor)
	hashViejo := hashDePrueba(t, "hashviejo")
	usuario := usuarioActivoDePrueba(t, id, correo, hashViejo)

	m := nuevosMocksAutenticar(t, usuario)
	m.hasher.FnNecesitaRehash = func(h dominio.HashContrasena) bool { return true }
	m.hasher.FnHashear = func(ctx context.Context, p dominio.ContrasenaPlana) (dominio.HashContrasena, error) {
		return dominio.HashContrasena{}, errors.New("hasher no disponible para rehash")
	}
	caso := m.casoDeUso()

	resultado, err := caso.Autenticar(context.Background(), comandoAutenticarValido(t))
	if err != nil {
		t.Fatalf("un fallo del rehash oportunista no debe abortar el login: %v", err)
	}
	if resultado.IDUsuario != idUsuarioValido1 {
		t.Error("se esperaba un login exitoso pese al fallo del rehash")
	}
	guardado := m.usuarios.LlamadasGuardar[0]
	if guardado.Credencial().Hash().Valor() != hashViejo.Valor() {
		t.Error("si el rehash falla, el hash persistido debe seguir siendo el original")
	}
	nombres := m.auditoria.NombresEventos()
	for _, n := range nombres {
		if n == "CredencialRehasheada" {
			t.Error("no debía auditarse CredencialRehasheada si el rehash falló")
		}
	}
}

// TestINV_ID_13_RehashOportunista_ReemplazarHashFalla_NoAbortaElLogin cubre
// la rama defensiva en la que Hashear tiene éxito pero devuelve un
// HashContrasena vacío (ReemplazarHash lo rechaza): tampoco debe abortar el
// login.
func TestINV_ID_13_RehashOportunista_ReemplazarHashFalla_NoAbortaElLogin(t *testing.T) {
	id := idDePrueba(t, idUsuarioValido1)
	correo := correoDePrueba(t, correoValidoValor)
	hashViejo := hashDePrueba(t, "hashviejo")
	usuario := usuarioActivoDePrueba(t, id, correo, hashViejo)

	m := nuevosMocksAutenticar(t, usuario)
	m.hasher.FnNecesitaRehash = func(h dominio.HashContrasena) bool { return true }
	m.hasher.FnHashear = func(ctx context.Context, p dominio.ContrasenaPlana) (dominio.HashContrasena, error) {
		return dominio.HashContrasena{}, nil // "éxito" pero con hash vacío
	}
	caso := m.casoDeUso()

	resultado, err := caso.Autenticar(context.Background(), comandoAutenticarValido(t))
	if err != nil {
		t.Fatalf("un fallo al reemplazar el hash no debe abortar el login: %v", err)
	}
	if resultado.IDUsuario != idUsuarioValido1 {
		t.Error("se esperaba un login exitoso")
	}
}

// TestAutenticarUsuarioCasoDeUso_SegundoFactor_PorMFAPropio cubre que un
// usuario con tieneMFA=true exige segundo factor con motivo "mfa_habilitado"
// y emite SegundoFactorRequerido.
func TestAutenticarUsuarioCasoDeUso_SegundoFactor_PorMFAPropio(t *testing.T) {
	id := idDePrueba(t, idUsuarioValido1)
	correo := correoDePrueba(t, correoValidoValor)
	hash := hashDePrueba(t, "hashvigente")
	usuario := usuarioConMFADePrueba(t, id, correo, hash)

	m := nuevosMocksAutenticar(t, usuario)
	caso := m.casoDeUso()

	resultado, err := caso.Autenticar(context.Background(), comandoAutenticarValido(t))
	if err != nil {
		t.Fatalf("Autenticar() devolvió error inesperado: %v", err)
	}
	if !resultado.RequiereSegundoFactor {
		t.Error("se esperaba RequiereSegundoFactor=true")
	}
	if resultado.MotivoStepUp != "mfa_habilitado" {
		t.Errorf("MotivoStepUp = %q, esperado mfa_habilitado", resultado.MotivoStepUp)
	}
	nombres := m.auditoria.NombresEventos()
	tieneSegundoFactor := false
	for _, n := range nombres {
		if n == "SegundoFactorRequerido" {
			tieneSegundoFactor = true
		}
	}
	if !tieneSegundoFactor {
		t.Errorf("eventos auditados = %v, se esperaba SegundoFactorRequerido", nombres)
	}
}

// TestAutenticarUsuarioCasoDeUso_SegundoFactor_PorConfianzaBaja cubre que,
// sin MFA propio, un RequiereStepUp de Confianza también exige segundo
// factor, con motivo "confianza_baja".
func TestAutenticarUsuarioCasoDeUso_SegundoFactor_PorConfianzaBaja(t *testing.T) {
	id := idDePrueba(t, idUsuarioValido1)
	correo := correoDePrueba(t, correoValidoValor)
	hash := hashDePrueba(t, "hashvigente")
	usuario := usuarioActivoDePrueba(t, id, correo, hash)

	m := nuevosMocksAutenticar(t, usuario)
	m.confianza.FnEvaluar = func(ctx context.Context, s puertos.SolicitudEvaluacion) (puertos.DecisionConfianza, error) {
		return puertos.DecisionConfianza{Permitido: true, RequiereStepUp: true, Puntaje: 0.42}, nil
	}
	caso := m.casoDeUso()

	resultado, err := caso.Autenticar(context.Background(), comandoAutenticarValido(t))
	if err != nil {
		t.Fatalf("Autenticar() devolvió error inesperado: %v", err)
	}
	if !resultado.RequiereSegundoFactor {
		t.Error("se esperaba RequiereSegundoFactor=true")
	}
	if resultado.MotivoStepUp != "confianza_baja" {
		t.Errorf("MotivoStepUp = %q, esperado confianza_baja", resultado.MotivoStepUp)
	}
	if resultado.PuntajeConfianza != 0.42 {
		t.Errorf("PuntajeConfianza = %v, esperado 0.42", resultado.PuntajeConfianza)
	}
}

// TestAutenticarUsuarioCasoDeUso_SegundoFactor_MFATienePrioridadSobreConfianza
// cubre el orden del switch: si ambas condiciones son true, el motivo
// reportado es "mfa_habilitado".
func TestAutenticarUsuarioCasoDeUso_SegundoFactor_MFATienePrioridadSobreConfianza(t *testing.T) {
	id := idDePrueba(t, idUsuarioValido1)
	correo := correoDePrueba(t, correoValidoValor)
	hash := hashDePrueba(t, "hashvigente")
	usuario := usuarioConMFADePrueba(t, id, correo, hash)

	m := nuevosMocksAutenticar(t, usuario)
	m.confianza.FnEvaluar = func(ctx context.Context, s puertos.SolicitudEvaluacion) (puertos.DecisionConfianza, error) {
		return puertos.DecisionConfianza{Permitido: true, RequiereStepUp: true}, nil
	}
	caso := m.casoDeUso()

	resultado, err := caso.Autenticar(context.Background(), comandoAutenticarValido(t))
	if err != nil {
		t.Fatalf("Autenticar() devolvió error inesperado: %v", err)
	}
	if resultado.MotivoStepUp != "mfa_habilitado" {
		t.Errorf("MotivoStepUp = %q, esperado mfa_habilitado (prioridad sobre confianza_baja)", resultado.MotivoStepUp)
	}
}

// TestAutenticarUsuarioCasoDeUso_FalloDePersistenciaAbortaElLogin cubre el
// mecanismo de INV-ID-15 en este caso de uso: si Guardar falla dentro de la
// UnidadDeTrabajo, el login se aborta y nada aguas abajo se ejecuta.
func TestAutenticarUsuarioCasoDeUso_FalloDePersistenciaAbortaElLogin(t *testing.T) {
	id := idDePrueba(t, idUsuarioValido1)
	correo := correoDePrueba(t, correoValidoValor)
	hash := hashDePrueba(t, "hashvigente")
	usuario := usuarioActivoDePrueba(t, id, correo, hash)

	m := nuevosMocksAutenticar(t, usuario)
	errGuardar := errors.New("conflicto de concurrencia")
	m.usuarios.FnGuardar = func(ctx context.Context, u *dominio.Usuario) error { return errGuardar }
	caso := m.casoDeUso()

	_, err := caso.Autenticar(context.Background(), comandoAutenticarValido(t))
	if !errors.Is(err, errGuardar) {
		t.Fatalf("se esperaba que el error de persistencia se propagara, obtuvo %v", err)
	}
	if len(m.auditoria.LlamadasRegistrar) != 0 {
		t.Error("si Guardar falla dentro de la UnidadDeTrabajo, no debe auditarse AutenticacionExitosa")
	}
	if len(m.eventos.LlamadasPublicar) != 0 {
		t.Error("un login abortado no debe publicar eventos")
	}
	if len(m.confianza.LlamadasRegistrarResultado) != 0 {
		t.Error("un login abortado no debe avisar a Confianza del resultado")
	}
}

func TestAutenticarUsuarioCasoDeUso_FalloAlPublicarEventos_NoBloqueaElLogin(t *testing.T) {
	id := idDePrueba(t, idUsuarioValido1)
	correo := correoDePrueba(t, correoValidoValor)
	hash := hashDePrueba(t, "hashvigente")
	usuario := usuarioActivoDePrueba(t, id, correo, hash)

	m := nuevosMocksAutenticar(t, usuario)
	m.eventos.FnPublicar = func(ctx context.Context, eventos ...dominio.EventoDominio) error {
		return errors.New("cola de eventos no disponible")
	}
	caso := m.casoDeUso()

	if _, err := caso.Autenticar(context.Background(), comandoAutenticarValido(t)); err != nil {
		t.Fatalf("un fallo al publicar eventos no debe abortar un login ya persistido: %v", err)
	}
}

func TestAutenticarUsuarioCasoDeUso_FalloAlRegistrarResultadoEnConfianza_NoBloquea(t *testing.T) {
	id := idDePrueba(t, idUsuarioValido1)
	correo := correoDePrueba(t, correoValidoValor)
	hash := hashDePrueba(t, "hashvigente")
	usuario := usuarioActivoDePrueba(t, id, correo, hash)

	m := nuevosMocksAutenticar(t, usuario)
	m.confianza.FnRegistrarResultado = func(ctx context.Context, r puertos.ResultadoIntento) error {
		return errors.New("confianza no disponible")
	}
	caso := m.casoDeUso()

	if _, err := caso.Autenticar(context.Background(), comandoAutenticarValido(t)); err != nil {
		t.Fatalf("un fallo al avisar a Confianza no debe abortar un login ya persistido: %v", err)
	}
}

// TestAutenticarUsuarioCasoDeUso_FalloDeAuditoriaExitosaAbortaElLogin cubre
// INV-ID-15 en el camino feliz: si Guardar tiene éxito pero auditar
// AutenticacionExitosa falla dentro de la misma UnidadDeTrabajo, el login
// se aborta por completo (nunca hay negocio confirmado sin su auditoría), y
// nada aguas abajo (publicación, aviso a Confianza) se ejecuta.
func TestAutenticarUsuarioCasoDeUso_FalloDeAuditoriaExitosaAbortaElLogin(t *testing.T) {
	id := idDePrueba(t, idUsuarioValido1)
	correo := correoDePrueba(t, correoValidoValor)
	hash := hashDePrueba(t, "hashvigente")
	usuario := usuarioActivoDePrueba(t, id, correo, hash)

	m := nuevosMocksAutenticar(t, usuario)
	errAuditoria := errors.New("bd de auditoría caída")
	m.auditoria.FnRegistrar = func(ctx context.Context, e dominio.EventoDominio, origen dominio.OrigenSolicitud) error {
		return errAuditoria
	}
	caso := m.casoDeUso()

	_, err := caso.Autenticar(context.Background(), comandoAutenticarValido(t))
	if !errors.Is(err, errAuditoria) {
		t.Fatalf("se esperaba que el error de auditoría se propagara, obtuvo %v", err)
	}
	if len(m.eventos.LlamadasPublicar) != 0 {
		t.Error("un login abortado por fallo de auditoría no debe publicar eventos")
	}
	if len(m.confianza.LlamadasRegistrarResultado) != 0 {
		t.Error("un login abortado por fallo de auditoría no debe avisar a Confianza del resultado")
	}
}

// TestAutenticarUsuarioCasoDeUso_FalloDeAuditoriaEnCadaRamaDeLoginFallido
// cubre INV-ID-15 en las cuatro ramas de fallo de login que pasan por
// registrarFalloLogin (correo inválido, usuario no encontrado, contraseña
// estructuralmente inválida y contraseña incorrecta): en todas ellas, si la
// auditoría del intento fallido no puede registrarse, ese error se
// prioriza sobre ErrCredencialesInvalidas.
func TestAutenticarUsuarioCasoDeUso_FalloDeAuditoriaEnCadaRamaDeLoginFallido(t *testing.T) {
	id := idDePrueba(t, idUsuarioValido1)
	correo := correoDePrueba(t, correoValidoValor)
	hash := hashDePrueba(t, "hashvigente")
	usuarioExistente := usuarioActivoDePrueba(t, id, correo, hash)

	casos := []struct {
		nombre  string
		usuario *dominio.Usuario
		mutar   func(cmd *aplicacion.ComandoAutenticar)
	}{
		{"correo_invalido", nil, func(cmd *aplicacion.ComandoAutenticar) { cmd.Correo = "no-es-un-correo" }},
		{"usuario_no_encontrado", nil, func(cmd *aplicacion.ComandoAutenticar) {}},
		{"contrasena_invalida", usuarioExistente, func(cmd *aplicacion.ComandoAutenticar) { cmd.Contrasena = "" }},
		{"contrasena_incorrecta", usuarioExistente, func(cmd *aplicacion.ComandoAutenticar) { cmd.Contrasena = "no-es-la-correcta" }},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			m := nuevosMocksAutenticar(t, c.usuario)
			errAuditoria := errors.New("bd de auditoría caída")
			m.auditoria.FnRegistrar = func(ctx context.Context, e dominio.EventoDominio, origen dominio.OrigenSolicitud) error {
				return errAuditoria
			}
			caso := m.casoDeUso()
			cmd := comandoAutenticarValido(t)
			c.mutar(&cmd)

			_, err := caso.Autenticar(context.Background(), cmd)
			if !errors.Is(err, errAuditoria) {
				t.Fatalf("caso %s: se esperaba que el error de auditoría se propagara, obtuvo %v", c.nombre, err)
			}
		})
	}
}

// TestAutenticarUsuarioCasoDeUso_RegistrarFalloLogin_ConfianzaFalla_NoBloquea
// cubre que, dentro de registrarFalloLogin, un fallo al avisar a Confianza
// del intento fallido es best-effort: no cambia el error de negocio
// devuelto (sigue siendo ErrCredencialesInvalidas, no el error de
// Confianza).
func TestAutenticarUsuarioCasoDeUso_RegistrarFalloLogin_ConfianzaFalla_NoBloquea(t *testing.T) {
	id := idDePrueba(t, idUsuarioValido1)
	correo := correoDePrueba(t, correoValidoValor)
	hash := hashDePrueba(t, "hashvigente")
	usuario := usuarioActivoDePrueba(t, id, correo, hash)

	m := nuevosMocksAutenticar(t, usuario)
	m.confianza.FnRegistrarResultado = func(ctx context.Context, r puertos.ResultadoIntento) error {
		return errors.New("confianza no disponible")
	}
	caso := m.casoDeUso()
	cmd := comandoAutenticarValido(t)
	cmd.Contrasena = "no-es-la-correcta"

	_, err := caso.Autenticar(context.Background(), cmd)
	var errCred *dominio.ErrCredencialesInvalidas
	if !errors.As(err, &errCred) {
		t.Fatalf("un fallo best-effort al avisar a Confianza no debe cambiar el error de negocio, obtuvo %T: %v", err, err)
	}
}
