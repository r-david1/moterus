package aplicacion_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/r-david1/moterus/internal/confianza/aplicacion"
	"github.com/r-david1/moterus/internal/confianza/dominio"
	"github.com/r-david1/moterus/internal/confianza/puertos"
	"github.com/r-david1/moterus/internal/confianza/puertos/mocks"
)

type mocksAbrirSala struct {
	salas      *mocks.RepositorioSalasDeEspera
	estadoCola *mocks.EstadoDeCola
	auditoria  *mocks.RegistroAuditoria
	reloj      *mocks.Reloj
	ids        *mocks.GeneradorIDs
	uow        *mocks.UnidadDeTrabajo
}

func nuevosMocksAbrirSala() *mocksAbrirSala {
	return &mocksAbrirSala{
		salas:      &mocks.RepositorioSalasDeEspera{},
		estadoCola: &mocks.EstadoDeCola{},
		auditoria:  &mocks.RegistroAuditoria{},
		reloj:      &mocks.Reloj{Fija: ahoraDePrueba()},
		ids:        &mocks.GeneradorIDs{},
		uow:        &mocks.UnidadDeTrabajo{},
	}
}

func (m *mocksAbrirSala) casoDeUso() *aplicacion.AbrirSalaCasoDeUso {
	return aplicacion.NuevoAbrirSalaCasoDeUso(m.salas, m.estadoCola, m.auditoria, m.reloj, m.ids, m.uow)
}

func comandoAbrirSalaSistemaValido(t *testing.T) puertos.ComandoAbrirSala {
	t.Helper()
	return puertos.ComandoAbrirSala{
		Alias:               "inscripciones-2026",
		Ruta:                dominio.RutaAccesoIniciarSesion.String(),
		RitmoAdmision:       50,
		CapacidadMaximaCola: 500_000,
		VentanaReclamo:      2 * time.Minute,
		ModoDegradado:       dominio.ModoDegradadoPermitir.String(),
		Origen:              origenDePrueba(t),
	}
}

func TestAbrirSalaCasoDeUso_FlujoFeliz(t *testing.T) {
	m := nuevosMocksAbrirSala()
	// LlamadasGuardar captura punteros al MISMO agregado mutado en el
	// lugar: no sirve para observar su estado EN EL MOMENTO de cada
	// llamada. FnGuardar sí, porque corre sincrónicamente en cada
	// invocación.
	var estadosAlGuardar []string
	m.salas.FnGuardar = func(ctx context.Context, s *dominio.SalaDeEspera) error {
		estadosAlGuardar = append(estadosAlGuardar, s.Estado().String())
		return nil
	}
	caso := m.casoDeUso()

	vista, err := caso.Abrir(context.Background(), comandoAbrirSalaSistemaValido(t))
	if err != nil {
		t.Fatalf("Abrir() devolvió error inesperado: %v", err)
	}
	if vista.Alias != "inscripciones-2026" {
		t.Errorf("Alias = %q, esperado inscripciones-2026", vista.Alias)
	}
	if vista.Estado != dominio.EstadoSalaAbierta.String() {
		t.Errorf("Estado = %q, esperado abierta", vista.Estado)
	}
	if vista.AlcanceTipo != dominio.TipoAlcanceSistema.String() {
		t.Errorf("AlcanceTipo = %q, esperado sistema", vista.AlcanceTipo)
	}

	// Se persiste dos veces: en "programada" (paso 4) y ya "abierta" tras
	// proyectar con éxito (paso 7, dentro de la UoW).
	esperados := []string{dominio.EstadoSalaProgramada.String(), dominio.EstadoSalaAbierta.String()}
	if len(estadosAlGuardar) != len(esperados) || estadosAlGuardar[0] != esperados[0] || estadosAlGuardar[1] != esperados[1] {
		t.Fatalf("estados al momento de cada Guardar = %v, esperado %v", estadosAlGuardar, esperados)
	}

	if len(m.estadoCola.LlamadasProyectar) != 1 {
		t.Fatalf("se esperaba 1 llamada a Proyectar, hubo %d", len(m.estadoCola.LlamadasProyectar))
	}
	proy := m.estadoCola.LlamadasProyectar[0]
	if proy.Estado != dominio.EstadoSalaAbierta.String() {
		t.Errorf("ProyeccionSala.Estado = %q, esperado abierta", proy.Estado)
	}
	if proy.Clave != "sistema:"+dominio.RutaAccesoIniciarSesion.String() {
		t.Errorf("ProyeccionSala.Clave = %q", proy.Clave)
	}

	nombres := m.auditoria.NombresEventos()
	if len(nombres) != 1 || nombres[0] != "SalaDeEsperaAbierta" {
		t.Errorf("eventos auditados = %v, esperado [SalaDeEsperaAbierta]", nombres)
	}
}

// TestAbrirSalaCasoDeUso_AuditaEnLaMismaUnidadDeTrabajoQuePersiste verifica
// INV-COLA-11: si la escritura del segundo Guardar (dentro de la UoW) falla,
// la auditoría nunca se registra, porque ambas llamadas viven en el mismo
// closure pasado a UnidadDeTrabajo.Ejecutar.
func TestAbrirSalaCasoDeUso_AuditaEnLaMismaUnidadDeTrabajoQuePersiste(t *testing.T) {
	m := nuevosMocksAbrirSala()
	fallo := errors.New("fallo de escritura simulado")
	llamadasGuardar := 0
	m.salas.FnGuardar = func(ctx context.Context, s *dominio.SalaDeEspera) error {
		llamadasGuardar++
		if llamadasGuardar == 2 {
			return fallo
		}
		return nil
	}
	caso := m.casoDeUso()

	_, err := caso.Abrir(context.Background(), comandoAbrirSalaSistemaValido(t))
	if !errors.Is(err, fallo) {
		t.Fatalf("se esperaba el error simulado, obtuvo %v", err)
	}
	if len(m.auditoria.LlamadasRegistrar) != 0 {
		t.Error("no debía auditarse nada si el segundo Guardar (dentro de la UoW) falló")
	}
	if m.uow.FnEjecutar == nil {
		// Confirma que efectivamente se invocó Ejecutar (el mock por
		// defecto simplemente corre la función, sin registrar llamadas por
		// separado): si Ejecutar nunca se hubiera llamado, llamadasGuardar
		// sería 1, no 2.
		if llamadasGuardar != 2 {
			t.Fatalf("se esperaban 2 llamadas a Guardar (una fuera y una dentro de la UoW), hubo %d", llamadasGuardar)
		}
	}
}

// TestAbrirSalaCasoDeUso_ProyeccionFallidaNoPersisteAbierta verifica el paso
// 6 de §3.1 del diseño: si EstadoDeCola.Proyectar falla, la sala queda
// "programada" en Postgres — nunca se vuelve a llamar a Guardar con la
// mutación a "abierta".
func TestAbrirSalaCasoDeUso_ProyeccionFallidaNoPersisteAbierta(t *testing.T) {
	m := nuevosMocksAbrirSala()
	falloProyeccion := errors.New("redis no disponible")
	m.estadoCola.FnProyectar = func(ctx context.Context, p puertos.ProyeccionSala) error {
		return falloProyeccion
	}
	var estadosAlGuardar []string
	m.salas.FnGuardar = func(ctx context.Context, s *dominio.SalaDeEspera) error {
		estadosAlGuardar = append(estadosAlGuardar, s.Estado().String())
		return nil
	}
	caso := m.casoDeUso()

	_, err := caso.Abrir(context.Background(), comandoAbrirSalaSistemaValido(t))
	if !errors.Is(err, falloProyeccion) {
		t.Fatalf("se esperaba el error de proyección, obtuvo %v", err)
	}
	if len(estadosAlGuardar) != 1 || estadosAlGuardar[0] != dominio.EstadoSalaProgramada.String() {
		t.Fatalf("se esperaba exactamente 1 Guardar, dejando la fila en programada; estados observados = %v", estadosAlGuardar)
	}
	if len(m.auditoria.LlamadasRegistrar) != 0 {
		t.Error("no debía auditarse nada si la proyección a Redis falló")
	}
}

func TestAbrirSalaCasoDeUso_AliasInvalido(t *testing.T) {
	m := nuevosMocksAbrirSala()
	caso := m.casoDeUso()

	cmd := comandoAbrirSalaSistemaValido(t)
	cmd.Alias = "AB" // fuera de rango, mayúsculas también inválidas
	_, err := caso.Abrir(context.Background(), cmd)
	var errAlias *dominio.ErrAliasSalaInvalido
	if !errors.As(err, &errAlias) {
		t.Fatalf("se esperaba *ErrAliasSalaInvalido, obtuvo %T: %v", err, err)
	}
	if len(m.salas.LlamadasGuardar) != 0 {
		t.Error("no debía persistirse nada con un alias inválido")
	}
}

func TestAbrirSalaCasoDeUso_RutaFueraDelCatalogo(t *testing.T) {
	m := nuevosMocksAbrirSala()
	caso := m.casoDeUso()

	cmd := comandoAbrirSalaSistemaValido(t)
	cmd.Ruta = "acceso.renovar_sesion" // excluida a propósito, §1.6 del diseño
	_, err := caso.Abrir(context.Background(), cmd)
	var errRuta *dominio.ErrRutaNoProtegible
	if !errors.As(err, &errRuta) {
		t.Fatalf("se esperaba *ErrRutaNoProtegible, obtuvo %T: %v", err, err)
	}
}

func TestAbrirSalaCasoDeUso_RitmoInvalido(t *testing.T) {
	m := nuevosMocksAbrirSala()
	caso := m.casoDeUso()

	cmd := comandoAbrirSalaSistemaValido(t)
	cmd.RitmoAdmision = 0
	_, err := caso.Abrir(context.Background(), cmd)
	var errRitmo *dominio.ErrRitmoAdmisionInvalido
	if !errors.As(err, &errRitmo) {
		t.Fatalf("se esperaba *ErrRitmoAdmisionInvalido, obtuvo %T: %v", err, err)
	}
}

func TestAbrirSalaCasoDeUso_OrganizacionSinCreadaPor_FallaConstruccion(t *testing.T) {
	m := nuevosMocksAbrirSala()
	caso := m.casoDeUso()

	cmd := comandoAbrirSalaSistemaValido(t)
	cmd.IDOrganizacion = idOrganizacionValido1
	// IDSujeto vacío + alcance organizacion: dominio.NuevaSalaDeEspera exige
	// creadaPor para una sala org-scoped, y lo verifica antes que
	// RutaProtegida.AdmiteAlcance (que de todos modos tampoco admitiría este
	// alcance para ninguna ruta del catálogo actual, §1.6 del diseño).
	_, err := caso.Abrir(context.Background(), cmd)
	var errCreadaPor *dominio.ErrIDUsuarioInvalido
	if !errors.As(err, &errCreadaPor) {
		t.Fatalf("se esperaba *ErrIDUsuarioInvalido, obtuvo %T: %v", err, err)
	}
	if len(m.salas.LlamadasGuardar) != 0 {
		t.Error("no debía persistirse nada si la construcción del agregado falló")
	}
}

func TestAbrirSalaCasoDeUso_SalaYaAbiertaParaLaRuta(t *testing.T) {
	m := nuevosMocksAbrirSala()
	m.salas.FnGuardar = func(ctx context.Context, s *dominio.SalaDeEspera) error {
		return &dominio.ErrSalaYaAbiertaParaLaRuta{Clave: s.Clave().String()}
	}
	caso := m.casoDeUso()

	_, err := caso.Abrir(context.Background(), comandoAbrirSalaSistemaValido(t))
	var errYaAbierta *dominio.ErrSalaYaAbiertaParaLaRuta
	if !errors.As(err, &errYaAbierta) {
		t.Fatalf("se esperaba *ErrSalaYaAbiertaParaLaRuta, obtuvo %T: %v", err, err)
	}
	if len(m.estadoCola.LlamadasProyectar) != 0 {
		t.Error("no debía proyectarse nada si el primer Guardar (índice único) falló")
	}
}
