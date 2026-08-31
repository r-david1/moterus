package aplicacion_test

import (
	"context"
	"errors"
	"testing"

	"github.com/r-david1/moterus/internal/identidad/aplicacion"
	"github.com/r-david1/moterus/internal/identidad/dominio"
	"github.com/r-david1/moterus/internal/identidad/puertos/mocks"
)

type mocksObtener struct {
	usuarios  *mocks.RepositorioUsuarios
	auditoria *mocks.RegistroAuditoria
	reloj     *mocks.Reloj
}

func nuevosMocksObtener(t *testing.T, usuario *dominio.Usuario) *mocksObtener {
	t.Helper()
	return &mocksObtener{
		usuarios: &mocks.RepositorioUsuarios{
			FnBuscarPorID: func(ctx context.Context, id dominio.IDUsuario) (*dominio.Usuario, error) {
				if usuario != nil && usuario.ID().EsIgual(id) {
					return usuario, nil
				}
				return nil, nil
			},
		},
		auditoria: &mocks.RegistroAuditoria{},
		reloj:     &mocks.Reloj{Fija: ahoraDePrueba()},
	}
}

func (m *mocksObtener) casoDeUso() *aplicacion.ObtenerUsuarioCasoDeUso {
	return aplicacion.NuevoObtenerUsuarioCasoDeUso(m.usuarios, m.auditoria, m.reloj)
}

func usuarioDePruebaParaConsulta(t *testing.T) *dominio.Usuario {
	t.Helper()
	id := idDePrueba(t, idUsuarioValido1)
	correo := correoDePrueba(t, correoValidoValor)
	hash := hashDePrueba(t, "hashvigente")
	return usuarioActivoDePrueba(t, id, correo, hash)
}

// TestObtenerUsuarioCasoDeUso_ConsultaPropia_NoAudita cubre la sección 3.3:
// consultar el perfil propio no se audita.
func TestObtenerUsuarioCasoDeUso_ConsultaPropia_NoAudita(t *testing.T) {
	usuario := usuarioDePruebaParaConsulta(t)
	m := nuevosMocksObtener(t, usuario)
	caso := m.casoDeUso()

	vista, err := caso.ObtenerPorID(context.Background(), aplicacion.ConsultaUsuarioPorID{
		IDUsuario:     idUsuarioValido1,
		IDSolicitante: idUsuarioValido1, // el propio usuario pregunta por sí mismo
		Origen:        origenDePrueba(t),
	})
	if err != nil {
		t.Fatalf("ObtenerPorID() devolvió error inesperado: %v", err)
	}
	if vista.ID != idUsuarioValido1 {
		t.Errorf("ID = %q, esperado %q", vista.ID, idUsuarioValido1)
	}
	if len(m.auditoria.LlamadasRegistrar) != 0 {
		t.Error("consultar el perfil propio no debe auditarse")
	}
}

// TestObtenerUsuarioCasoDeUso_ConsultaPorTercero_Audita cubre que un
// solicitante distinto del usuario consultado dispara usuario.consultado.
func TestObtenerUsuarioCasoDeUso_ConsultaPorTercero_Audita(t *testing.T) {
	usuario := usuarioDePruebaParaConsulta(t)
	m := nuevosMocksObtener(t, usuario)
	caso := m.casoDeUso()

	vista, err := caso.ObtenerPorID(context.Background(), aplicacion.ConsultaUsuarioPorID{
		IDUsuario:     idUsuarioValido1,
		IDSolicitante: idUsuarioValido2, // un tercero pregunta
		Origen:        origenDePrueba(t),
	})
	if err != nil {
		t.Fatalf("ObtenerPorID() devolvió error inesperado: %v", err)
	}
	if vista.ID != idUsuarioValido1 {
		t.Errorf("ID = %q, esperado %q", vista.ID, idUsuarioValido1)
	}
	if len(m.auditoria.LlamadasRegistrar) != 1 {
		t.Fatalf("se esperaba una llamada de auditoría, hubo %d", len(m.auditoria.LlamadasRegistrar))
	}
	evento := m.auditoria.LlamadasRegistrar[0].Evento
	if evento.NombreEvento() != "UsuarioConsultado" {
		t.Errorf("evento auditado = %q, esperado UsuarioConsultado", evento.NombreEvento())
	}
	consultado, ok := evento.(dominio.UsuarioConsultado)
	if !ok {
		t.Fatalf("el evento no es dominio.UsuarioConsultado: %T", evento)
	}
	if consultado.IDUsuario != idUsuarioValido1 {
		t.Errorf("IDUsuario del evento = %q, esperado %q", consultado.IDUsuario, idUsuarioValido1)
	}
	if consultado.IDSolicitante != idUsuarioValido2 {
		t.Errorf("IDSolicitante del evento = %q, esperado %q", consultado.IDSolicitante, idUsuarioValido2)
	}
}

// TestObtenerUsuarioCasoDeUso_ConsultaInterna_IDSolicitanteVacio_NoAudita
// cubre el caso "llamada interna del sistema" (IDSolicitante vacío).
func TestObtenerUsuarioCasoDeUso_ConsultaInterna_IDSolicitanteVacio_NoAudita(t *testing.T) {
	usuario := usuarioDePruebaParaConsulta(t)
	m := nuevosMocksObtener(t, usuario)
	caso := m.casoDeUso()

	_, err := caso.ObtenerPorID(context.Background(), aplicacion.ConsultaUsuarioPorID{
		IDUsuario:     idUsuarioValido1,
		IDSolicitante: "",
		Origen:        origenDePrueba(t),
	})
	if err != nil {
		t.Fatalf("ObtenerPorID() devolvió error inesperado: %v", err)
	}
	if len(m.auditoria.LlamadasRegistrar) != 0 {
		t.Error("una llamada interna del sistema (IDSolicitante vacío) no debe auditarse")
	}
}

// TestObtenerUsuarioCasoDeUso_FalloDeAuditoria_NoBloqueaLaRespuesta cubre
// explícitamente el comentario del código: a diferencia de registro y
// autenticación, un fallo al auditar usuario.consultado no aborta la
// consulta (no hay negocio que "confirmar", es una lectura).
func TestObtenerUsuarioCasoDeUso_FalloDeAuditoria_NoBloqueaLaRespuesta(t *testing.T) {
	usuario := usuarioDePruebaParaConsulta(t)
	m := nuevosMocksObtener(t, usuario)
	m.auditoria.FnRegistrar = func(ctx context.Context, e dominio.EventoDominio, origen dominio.OrigenSolicitud) error {
		return errors.New("bd de auditoría caída")
	}
	caso := m.casoDeUso()

	vista, err := caso.ObtenerPorID(context.Background(), aplicacion.ConsultaUsuarioPorID{
		IDUsuario:     idUsuarioValido1,
		IDSolicitante: idUsuarioValido2,
		Origen:        origenDePrueba(t),
	})
	if err != nil {
		t.Fatalf("un fallo al auditar usuario.consultado no debe bloquear la respuesta: %v", err)
	}
	if vista.ID != idUsuarioValido1 {
		t.Error("se esperaba obtener la vista del usuario pese al fallo de auditoría")
	}
}

func TestObtenerUsuarioCasoDeUso_IDInvalido(t *testing.T) {
	m := nuevosMocksObtener(t, nil)
	caso := m.casoDeUso()

	_, err := caso.ObtenerPorID(context.Background(), aplicacion.ConsultaUsuarioPorID{
		IDUsuario: "no-es-un-uuid",
		Origen:    origenDePrueba(t),
	})
	var errID *dominio.ErrIDUsuarioInvalido
	if !errors.As(err, &errID) {
		t.Fatalf("se esperaba *ErrIDUsuarioInvalido, obtuvo %T: %v", err, err)
	}
	if len(m.usuarios.LlamadasBuscarPorID) != 0 {
		t.Error("un ID inválido no debe llegar a consultar el repositorio")
	}
}

func TestObtenerUsuarioCasoDeUso_UsuarioNoEncontrado(t *testing.T) {
	m := nuevosMocksObtener(t, nil) // el repositorio siempre devuelve nil, nil
	caso := m.casoDeUso()

	_, err := caso.ObtenerPorID(context.Background(), aplicacion.ConsultaUsuarioPorID{
		IDUsuario: idUsuarioValido1,
		Origen:    origenDePrueba(t),
	})
	var errNoEncontrado *dominio.ErrUsuarioNoEncontrado
	if !errors.As(err, &errNoEncontrado) {
		t.Fatalf("se esperaba *ErrUsuarioNoEncontrado, obtuvo %T: %v", err, err)
	}
}

func TestObtenerUsuarioCasoDeUso_BuscarPorIDError_SePropaga(t *testing.T) {
	m := nuevosMocksObtener(t, nil)
	errBD := errors.New("conexión a la base de datos perdida")
	m.usuarios.FnBuscarPorID = func(ctx context.Context, id dominio.IDUsuario) (*dominio.Usuario, error) {
		return nil, errBD
	}
	caso := m.casoDeUso()

	_, err := caso.ObtenerPorID(context.Background(), aplicacion.ConsultaUsuarioPorID{
		IDUsuario: idUsuarioValido1,
		Origen:    origenDePrueba(t),
	})
	if !errors.Is(err, errBD) {
		t.Fatalf("se esperaba que el error del repositorio se propagara, obtuvo %v", err)
	}
}

// TestObtenerUsuarioCasoDeUso_VistaUsuario_ProyectaCorrectamente verifica
// que VistaUsuario refleje fielmente los datos del agregado, incluido el
// caso de UltimoAccesoEn ausente (usuario que nunca inició sesión).
func TestObtenerUsuarioCasoDeUso_VistaUsuario_ProyectaCorrectamente(t *testing.T) {
	usuario := usuarioDePruebaParaConsulta(t) // activo, nunca ha iniciado sesión
	m := nuevosMocksObtener(t, usuario)
	caso := m.casoDeUso()

	vista, err := caso.ObtenerPorID(context.Background(), aplicacion.ConsultaUsuarioPorID{
		IDUsuario: idUsuarioValido1,
		Origen:    origenDePrueba(t),
	})
	if err != nil {
		t.Fatalf("ObtenerPorID() devolvió error inesperado: %v", err)
	}
	if vista.Correo != correoValidoValor {
		t.Errorf("Correo = %q, esperado %q", vista.Correo, correoValidoValor)
	}
	if vista.Estado != "activo" {
		t.Errorf("Estado = %q, esperado activo", vista.Estado)
	}
	if vista.TieneMFA {
		t.Error("TieneMFA debía ser false")
	}
	if vista.UltimoAccesoEn != nil {
		t.Error("UltimoAccesoEn debía ser nil: el usuario de prueba nunca inició sesión")
	}
	if !vista.CreadoEn.Equal(ahoraDePrueba()) {
		t.Errorf("CreadoEn = %v, esperado %v", vista.CreadoEn, ahoraDePrueba())
	}
}

// TestObtenerUsuarioCasoDeUso_VistaUsuario_ConUltimoAcceso cubre el otro
// lado de la proyección: cuando el usuario sí registró un acceso,
// UltimoAccesoEn debe ir presente.
func TestObtenerUsuarioCasoDeUso_VistaUsuario_ConUltimoAcceso(t *testing.T) {
	usuario := usuarioDePruebaParaConsulta(t)
	usuario.RegistrarAcceso(ahoraDePrueba())
	m := nuevosMocksObtener(t, usuario)
	caso := m.casoDeUso()

	vista, err := caso.ObtenerPorID(context.Background(), aplicacion.ConsultaUsuarioPorID{
		IDUsuario: idUsuarioValido1,
		Origen:    origenDePrueba(t),
	})
	if err != nil {
		t.Fatalf("ObtenerPorID() devolvió error inesperado: %v", err)
	}
	if vista.UltimoAccesoEn == nil {
		t.Fatal("UltimoAccesoEn debía estar presente tras RegistrarAcceso")
	}
	if !vista.UltimoAccesoEn.Equal(ahoraDePrueba()) {
		t.Errorf("UltimoAccesoEn = %v, esperado %v", *vista.UltimoAccesoEn, ahoraDePrueba())
	}
}
