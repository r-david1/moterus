package dominio

import (
	"errors"
	"testing"
	"time"
)

func fundarMembresiaDePrueba(t *testing.T) (*Membresia, time.Time) {
	t.Helper()
	ahora := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	m, err := FundarMembresia(idMembresiaDePrueba(t), idOrganizacionDePrueba(t), idUsuarioDePrueba(t), ahora)
	if err != nil {
		t.Fatalf("no se pudo fundar la membresía de prueba: %v", err)
	}
	return m, ahora
}

// --- constructores -----------------------------------------------------------

func TestMembresia_Getters(t *testing.T) {
	m, ahora := fundarMembresiaDePrueba(t)
	if !m.ID().EsIgual(idMembresiaDePrueba(t)) {
		t.Errorf("ID() = %v", m.ID())
	}
	if !m.OrganizacionID().EsIgual(idOrganizacionDePrueba(t)) {
		t.Errorf("OrganizacionID() = %v", m.OrganizacionID())
	}
	if !m.UsuarioID().EsIgual(idUsuarioDePrueba(t)) {
		t.Errorf("UsuarioID() = %v", m.UsuarioID())
	}
	if !m.ActualizadaEn().Equal(ahora) {
		t.Errorf("ActualizadaEn() = %v, esperado %v", m.ActualizadaEn(), ahora)
	}
	if _, removida := m.RemovidaEn(); removida {
		t.Error("una membresía activa no debe estar removida")
	}
}

func TestFundarMembresia(t *testing.T) {
	m, ahora := fundarMembresiaDePrueba(t)
	if !m.Rol().EsIgual(RolPropietario) {
		t.Errorf("Rol() = %v, esperado propietario", m.Rol())
	}
	if !m.Estado().EsIgual(EstadoMembresiaActiva) {
		t.Errorf("Estado() = %v, esperado activa", m.Estado())
	}
	if _, tiene := m.OtorgadaPor(); tiene {
		t.Error("la membresía fundacional no debe tener OtorgadaPor")
	}
	if !m.CreadaEn().Equal(ahora) {
		t.Errorf("CreadaEn() = %v, esperado %v", m.CreadaEn(), ahora)
	}
	eventos := m.EventosPendientes()
	if len(eventos) != 1 {
		t.Fatalf("se esperaba 1 evento, hay %d", len(eventos))
	}
	ev, ok := eventos[0].(MiembroAgregado)
	if !ok {
		t.Fatalf("se esperaba MiembroAgregado, obtuvo %T", eventos[0])
	}
	if ev.Via != ViaFundacion {
		t.Errorf("Via = %q, esperado %q", ev.Via, ViaFundacion)
	}
}

func TestFundarMembresia_RechazaCamposVacios(t *testing.T) {
	ahora := time.Now()
	var idVacio IDMembresia
	if _, err := FundarMembresia(idVacio, idOrganizacionDePrueba(t), idUsuarioDePrueba(t), ahora); err == nil {
		t.Error("se esperaba error con IDMembresia vacío")
	}
	var orgVacia IDOrganizacion
	if _, err := FundarMembresia(idMembresiaDePrueba(t), orgVacia, idUsuarioDePrueba(t), ahora); err == nil {
		t.Error("se esperaba error con IDOrganizacion vacío")
	}
	var usuarioVacio IDUsuario
	if _, err := FundarMembresia(idMembresiaDePrueba(t), idOrganizacionDePrueba(t), usuarioVacio, ahora); err == nil {
		t.Error("se esperaba error con IDUsuario vacío")
	}
}

func TestAgregarMiembro(t *testing.T) {
	ahora := time.Now()
	m, err := AgregarMiembro(idMembresiaDePrueba(t), idOrganizacionDePrueba(t), idUsuarioDePrueba(t), RolMiembro, otroIDUsuarioDePrueba(t), ahora)
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	otorgadaPor, tiene := m.OtorgadaPor()
	if !tiene || otorgadaPor.EsVacio() {
		t.Error("AgregarMiembro debe registrar quién otorgó la membresía")
	}
	eventos := m.EventosPendientes()
	ev := eventos[0].(MiembroAgregado)
	if ev.Via != ViaAltaDirecta {
		t.Errorf("Via = %q, esperado %q", ev.Via, ViaAltaDirecta)
	}
}

func TestAgregarMiembro_ExigeOtorgadaPor(t *testing.T) {
	var otorgadaPorVacio IDUsuario
	if _, err := AgregarMiembro(idMembresiaDePrueba(t), idOrganizacionDePrueba(t), idUsuarioDePrueba(t), RolMiembro, otorgadaPorVacio, time.Now()); err == nil {
		t.Fatal("se esperaba error: AgregarMiembro exige otorgadaPor")
	}
}

func TestCrearMembresiaDesdeInvitacion(t *testing.T) {
	ahora := time.Now()
	m, err := CrearMembresiaDesdeInvitacion(idMembresiaDePrueba(t), idOrganizacionDePrueba(t), idUsuarioDePrueba(t), RolAdministrador, otroIDUsuarioDePrueba(t), ahora)
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	eventos := m.EventosPendientes()
	ev := eventos[0].(MiembroAgregado)
	if ev.Via != ViaInvitacion {
		t.Errorf("Via = %q, esperado %q", ev.Via, ViaInvitacion)
	}
}

func TestReconstituirMembresia_NoAcumulaEventos(t *testing.T) {
	ahora := time.Now()
	removidaEn := ahora.Add(time.Hour)
	otorgadaPor := otroIDUsuarioDePrueba(t)
	m := ReconstituirMembresia(
		idMembresiaDePrueba(t),
		idOrganizacionDePrueba(t),
		idUsuarioDePrueba(t),
		RolMiembro,
		EstadoMembresiaRemovida,
		&otorgadaPor,
		ahora,
		ahora,
		&removidaEn,
	)
	if eventos := m.EventosPendientes(); len(eventos) != 0 {
		t.Errorf("Reconstituir no debe acumular eventos, hay %d", len(eventos))
	}
	got, ok := m.RemovidaEn()
	if !ok || !got.Equal(removidaEn) {
		t.Errorf("RemovidaEn() = %v, %v; esperado %v", got, ok, removidaEn)
	}
}

// --- CambiarRol ---------------------------------------------------------------

func TestMembresia_CambiarRol_NoOpIdempotente(t *testing.T) {
	m, ahora := fundarMembresiaDePrueba(t)
	m.EventosPendientes()
	if err := m.CambiarRol(RolPropietario, RolPropietario, 3, ahora.Add(time.Hour)); err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if !m.ActualizadaEn().Equal(ahora) {
		t.Error("un no-op no debe actualizar ActualizadaEn()")
	}
	if eventos := m.EventosPendientes(); len(eventos) != 0 {
		t.Errorf("un no-op no debe acumular eventos (§3.2 del diseño), hay %d", len(eventos))
	}
}

func TestMembresia_CambiarRol_AplicaReglaDeDominancia(t *testing.T) {
	ahora := time.Now()
	// m.Rol() == administrador: el objetivo del intento de promoción.
	objetivoAdministrador, err := AgregarMiembro(idMembresiaDePrueba(t), idOrganizacionDePrueba(t), idUsuarioDePrueba(t), RolAdministrador, otroIDUsuarioDePrueba(t), ahora)
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	objetivoAdministrador.EventosPendientes()

	// Un administrador no puede otorgarse (ni otorgar a otro) el rol
	// propietario: es un rol estrictamente superior al propio.
	if err := objetivoAdministrador.CambiarRol(RolPropietario, RolAdministrador, 5, ahora); err == nil {
		t.Fatal("se esperaba ErrRolSuperiorAlPropio")
	} else {
		var errSup *ErrRolSuperiorAlPropio
		if !errors.As(err, &errSup) {
			t.Errorf("se esperaba *ErrRolSuperiorAlPropio, obtuvo %T", err)
		}
	}

	// Un administrador no puede modificar a un propietario, incluso
	m, _ := fundarMembresiaDePrueba(t) // m.Rol() == propietario
	m.EventosPendientes()
	// otorgando un rol igual o inferior.
	if err := m.CambiarRol(RolMiembro, RolAdministrador, 5, ahora); err == nil {
		t.Fatal("se esperaba ErrMembresiaDominante")
	} else {
		var errDom *ErrMembresiaDominante
		if !errors.As(err, &errDom) {
			t.Errorf("se esperaba *ErrMembresiaDominante, obtuvo %T", err)
		}
	}
}

// TestINV_TEN_06_CambiarRol_UltimoPropietario cubre el invariante del
// último propietario para CambiarRol: degradar al único propietario activo
// debe rechazarse; con otro propietario activo disponible, debe permitirse.
func TestINV_TEN_06_CambiarRol_UltimoPropietario(t *testing.T) {
	m, ahora := fundarMembresiaDePrueba(t)
	m.EventosPendientes()

	// propietariosActivos=1: esta es la única. Degradarla debe rechazarse.
	if err := m.CambiarRol(RolAdministrador, RolPropietario, 1, ahora); err == nil {
		t.Fatal("se esperaba ErrUltimoPropietario")
	} else {
		var errUltimo *ErrUltimoPropietario
		if !errors.As(err, &errUltimo) {
			t.Errorf("se esperaba *ErrUltimoPropietario, obtuvo %T", err)
		}
	}
	if !m.Rol().EsIgual(RolPropietario) {
		t.Error("un intento rechazado no debe mutar el rol")
	}

	// propietariosActivos=2: hay otro propietario. Degradarla debe permitirse.
	if err := m.CambiarRol(RolAdministrador, RolPropietario, 2, ahora.Add(time.Hour)); err != nil {
		t.Fatalf("no se esperaba error con otro propietario disponible: %v", err)
	}
	if !m.Rol().EsIgual(RolAdministrador) {
		t.Errorf("Rol() = %v, esperado administrador", m.Rol())
	}
	eventos := m.EventosPendientes()
	if len(eventos) != 1 {
		t.Fatalf("se esperaba 1 evento, hay %d", len(eventos))
	}
	ev, ok := eventos[0].(RolDeMiembroCambiado)
	if !ok {
		t.Fatalf("se esperaba RolDeMiembroCambiado, obtuvo %T", eventos[0])
	}
	if ev.RolAnterior != "propietario" || ev.RolNuevo != "administrador" {
		t.Errorf("RolAnterior=%q RolNuevo=%q", ev.RolAnterior, ev.RolNuevo)
	}
}

func TestMembresia_CambiarRol_NoAplicaUltimoPropietarioSiNoEraPropietario(t *testing.T) {
	m, err := AgregarMiembro(idMembresiaDePrueba(t), idOrganizacionDePrueba(t), idUsuarioDePrueba(t), RolMiembro, otroIDUsuarioDePrueba(t), time.Now())
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	m.EventosPendientes()
	// propietariosActivos=0 en la organización (ninguno todavía, caso
	// artificial), pero esta membresía no es propietaria: no debe activarse
	// el invariante del último propietario.
	if err := m.CambiarRol(RolAdministrador, RolPropietario, 0, time.Now()); err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
}

func TestMembresia_CambiarRol_RechazaSobreMembresiaRemovida(t *testing.T) {
	m, ahora := fundarMembresiaDePrueba(t)
	// removerla primero con otro propietario disponible.
	if err := m.Remover(RolPropietario, 2, ahora); err != nil {
		t.Fatalf("no se esperaba error al remover: %v", err)
	}
	if err := m.CambiarRol(RolAdministrador, RolPropietario, 5, ahora); err == nil {
		t.Fatal("no se debe poder cambiar el rol de una membresía removida")
	}
}

func TestMembresia_CambiarRol_RechazaRolVacio(t *testing.T) {
	m, ahora := fundarMembresiaDePrueba(t)
	var rolVacio Rol
	if err := m.CambiarRol(rolVacio, RolPropietario, 5, ahora); err == nil {
		t.Fatal("se esperaba error con rol nuevo vacío")
	}
}

// --- Suspender / Reactivar ----------------------------------------------------

// TestINV_TEN_08_MembresiaMaquinaEstados verifica Suspender/Reactivar/
// Remover contra la máquina de estados y que removida es terminal.
func TestINV_TEN_08_MembresiaMaquinaEstados(t *testing.T) {
	m, err := AgregarMiembro(idMembresiaDePrueba(t), idOrganizacionDePrueba(t), idUsuarioDePrueba(t), RolMiembro, otroIDUsuarioDePrueba(t), time.Now())
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	m.EventosPendientes()

	if err := m.Suspender(RolPropietario, 5, time.Now()); err != nil {
		t.Fatalf("no se esperaba error al suspender: %v", err)
	}
	if !m.Estado().EsIgual(EstadoMembresiaSuspendida) {
		t.Errorf("Estado() = %v, esperado suspendida", m.Estado())
	}
	eventos := m.EventosPendientes()
	if _, ok := eventos[0].(EstadoMembresiaCambiado); !ok {
		t.Fatalf("se esperaba EstadoMembresiaCambiado, obtuvo %T", eventos[0])
	}

	if err := m.Reactivar(time.Now()); err != nil {
		t.Fatalf("no se esperaba error al reactivar: %v", err)
	}
	if !m.Estado().EsIgual(EstadoMembresiaActiva) {
		t.Errorf("Estado() = %v, esperado activa", m.Estado())
	}

	if err := m.Remover(RolPropietario, 5, time.Now()); err != nil {
		t.Fatalf("no se esperaba error al remover: %v", err)
	}
	if !m.Estado().EsIgual(EstadoMembresiaRemovida) {
		t.Errorf("Estado() = %v, esperado removida", m.Estado())
	}
	if _, tiene := m.RemovidaEn(); !tiene {
		t.Error("RemovidaEn() debe estar poblado tras Remover")
	}

	// removida es terminal: ninguna transición más es válida.
	if err := m.Reactivar(time.Now()); err == nil {
		t.Fatal("no se debe poder reactivar una membresía removida")
	}
	if err := m.Suspender(RolPropietario, 5, time.Now()); err == nil {
		t.Fatal("no se debe poder suspender una membresía removida")
	}
	if err := m.Remover(RolPropietario, 5, time.Now()); err == nil {
		t.Fatal("no se debe poder remover una membresía ya removida")
	}
}

func TestMembresia_Suspender_AplicaReglaDeDominancia(t *testing.T) {
	m, _ := fundarMembresiaDePrueba(t) // propietario
	if err := m.Suspender(RolAdministrador, 5, time.Now()); err == nil {
		t.Fatal("un administrador no debe poder suspender a un propietario")
	} else {
		var errDom *ErrMembresiaDominante
		if !errors.As(err, &errDom) {
			t.Errorf("se esperaba *ErrMembresiaDominante, obtuvo %T", err)
		}
	}
}

// TestINV_TEN_06_Suspender_UltimoPropietario cubre el invariante del último
// propietario para Suspender.
func TestINV_TEN_06_Suspender_UltimoPropietario(t *testing.T) {
	m, ahora := fundarMembresiaDePrueba(t)
	if err := m.Suspender(RolPropietario, 1, ahora); err == nil {
		t.Fatal("se esperaba ErrUltimoPropietario")
	} else {
		var errUltimo *ErrUltimoPropietario
		if !errors.As(err, &errUltimo) {
			t.Errorf("se esperaba *ErrUltimoPropietario, obtuvo %T", err)
		}
	}
	if !m.Estado().EsIgual(EstadoMembresiaActiva) {
		t.Error("un intento rechazado no debe mutar el estado")
	}
	if err := m.Suspender(RolPropietario, 2, ahora); err != nil {
		t.Fatalf("con otro propietario disponible no debería fallar: %v", err)
	}
}

// --- Remover / Abandonar ------------------------------------------------------

func TestMembresia_Remover_AplicaReglaDeDominancia(t *testing.T) {
	m, _ := fundarMembresiaDePrueba(t)
	if err := m.Remover(RolAdministrador, 5, time.Now()); err == nil {
		t.Fatal("un administrador no debe poder remover a un propietario")
	}
}

// TestINV_TEN_06_Remover_UltimoPropietario cubre el invariante del último
// propietario para Remover (por decisión de otro administrador/propietario).
func TestINV_TEN_06_Remover_UltimoPropietario(t *testing.T) {
	m, ahora := fundarMembresiaDePrueba(t)
	m.EventosPendientes()
	if err := m.Remover(RolPropietario, 1, ahora); err == nil {
		t.Fatal("se esperaba ErrUltimoPropietario")
	} else {
		var errUltimo *ErrUltimoPropietario
		if !errors.As(err, &errUltimo) {
			t.Errorf("se esperaba *ErrUltimoPropietario, obtuvo %T", err)
		}
	}
	if err := m.Remover(RolPropietario, 2, ahora); err != nil {
		t.Fatalf("con otro propietario disponible no debería fallar: %v", err)
	}
	if !m.Estado().EsIgual(EstadoMembresiaRemovida) {
		t.Error("Remover con propietarios suficientes debe transicionar a removida")
	}
	eventos := m.EventosPendientes()
	ev, ok := eventos[0].(MiembroRemovido)
	if !ok {
		t.Fatalf("se esperaba MiembroRemovido, obtuvo %T", eventos[0])
	}
	if ev.PorIniciativaPropia {
		t.Error("Remover (por un tercero) debe registrar PorIniciativaPropia=false")
	}
}

// TestINV_TEN_06_Abandonar_UltimoPropietario cubre el invariante del último
// propietario para Abandonar: el único propietario no puede abandonar sin
// transferir la propiedad antes (§3.2 del diseño).
func TestINV_TEN_06_Abandonar_UltimoPropietario(t *testing.T) {
	m, ahora := fundarMembresiaDePrueba(t)
	m.EventosPendientes()
	if err := m.Abandonar(1, ahora); err == nil {
		t.Fatal("se esperaba ErrUltimoPropietario: transferí la propiedad antes de salir")
	} else {
		var errUltimo *ErrUltimoPropietario
		if !errors.As(err, &errUltimo) {
			t.Errorf("se esperaba *ErrUltimoPropietario, obtuvo %T", err)
		}
	}
	if err := m.Abandonar(2, ahora); err != nil {
		t.Fatalf("con otro propietario disponible no debería fallar: %v", err)
	}
	eventos := m.EventosPendientes()
	ev, ok := eventos[0].(MiembroRemovido)
	if !ok {
		t.Fatalf("se esperaba MiembroRemovido, obtuvo %T", eventos[0])
	}
	if !ev.PorIniciativaPropia {
		t.Error("Abandonar debe registrar PorIniciativaPropia=true")
	}
}

func TestMembresia_Abandonar_NoAplicaReglaDeDominancia(t *testing.T) {
	// Abandonar no recibe ejecutor: un miembro siempre puede actuar sobre su
	// propia membresía (tercer paso de INV-TEN-20), sujeto solo a
	// INV-TEN-06.
	m, err := AgregarMiembro(idMembresiaDePrueba(t), idOrganizacionDePrueba(t), idUsuarioDePrueba(t), RolMiembro, otroIDUsuarioDePrueba(t), time.Now())
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if err := m.Abandonar(5, time.Now()); err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if !m.Estado().EsIgual(EstadoMembresiaRemovida) {
		t.Error("Abandonar debe transicionar a removida")
	}
}

func TestMembresia_Abandonar_UnaMembresiaSuspendidaNoCuentaComoPropietarioActivo(t *testing.T) {
	m, ahora := fundarMembresiaDePrueba(t)
	// Un propietario suspendido no puede abandonar directamente (su estado no
	// permite la transición activa->removida vía Suspender; pero desde
	// suspendida SÍ se puede pasar a removida). Como está suspendida, ya no
	// cuenta como "propietario activo": el invariante no debe bloquearla,
	// aunque propietariosActivos venga en 0 (ningún otro propietario activo).
	if err := m.Suspender(RolPropietario, 2, ahora); err != nil {
		t.Fatalf("no se esperaba error al suspender con otro propietario: %v", err)
	}
	if err := m.Abandonar(0, ahora.Add(time.Hour)); err != nil {
		t.Fatalf("una membresía suspendida no debe activar el invariante del último propietario: %v", err)
	}
}

// --- Permite -------------------------------------------------------------

func TestMembresia_Permite(t *testing.T) {
	m, err := AgregarMiembro(idMembresiaDePrueba(t), idOrganizacionDePrueba(t), idUsuarioDePrueba(t), RolMiembro, otroIDUsuarioDePrueba(t), time.Now())
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if !m.Permite(PermisoOrganizacionVer, EstadoOrganizacionActiva) {
		t.Error("un miembro en organización activa debe poder ver la organización")
	}
	if m.Permite(PermisoMiembroInvitar, EstadoOrganizacionActiva) {
		t.Error("un miembro no debe poder invitar")
	}
}
