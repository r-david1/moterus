package dominio

import (
	"errors"
	"testing"
	"time"
)

func idOrganizacionDePrueba(t *testing.T) IDOrganizacion {
	t.Helper()
	id, err := IDOrganizacionDesde("018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6d")
	if err != nil {
		t.Fatalf("no se pudo construir IDOrganizacion de prueba: %v", err)
	}
	return id
}

func idMembresiaDePrueba(t *testing.T) IDMembresia {
	t.Helper()
	id, err := IDMembresiaDesde("018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6e")
	if err != nil {
		t.Fatalf("no se pudo construir IDMembresia de prueba: %v", err)
	}
	return id
}

func idUsuarioDePrueba(t *testing.T) IDUsuario {
	t.Helper()
	id, err := IDUsuarioDesde("018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6f")
	if err != nil {
		t.Fatalf("no se pudo construir IDUsuario de prueba: %v", err)
	}
	return id
}

func otroIDUsuarioDePrueba(t *testing.T) IDUsuario {
	t.Helper()
	id, err := IDUsuarioDesde("018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5000")
	if err != nil {
		t.Fatalf("no se pudo construir IDUsuario de prueba: %v", err)
	}
	return id
}

func membresiaActivaDePrueba(t *testing.T, rol Rol) *Membresia {
	t.Helper()
	ahora := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	if rol.EsIgual(RolPropietario) {
		m, err := FundarMembresia(idMembresiaDePrueba(t), idOrganizacionDePrueba(t), idUsuarioDePrueba(t), ahora)
		if err != nil {
			t.Fatalf("no se pudo fundar la membresía de prueba: %v", err)
		}
		m.EventosPendientes()
		return m
	}
	m, err := AgregarMiembro(idMembresiaDePrueba(t), idOrganizacionDePrueba(t), idUsuarioDePrueba(t), rol, otroIDUsuarioDePrueba(t), ahora)
	if err != nil {
		t.Fatalf("no se pudo agregar la membresía de prueba: %v", err)
	}
	m.EventosPendientes()
	return m
}

func membresiaSuspendidaDePrueba(t *testing.T, rol Rol) *Membresia {
	t.Helper()
	m := membresiaActivaDePrueba(t, rol)
	if err := m.Suspender(RolPropietario, 5, time.Now()); err != nil {
		t.Fatalf("no se pudo suspender la membresía de prueba: %v", err)
	}
	m.EventosPendientes()
	return m
}

// TestINV_TEN_17_18_EvaluadorDeAutorizacion_OrdenNormativo cubre los cinco
// pasos, EN ORDEN, del evaluador de autorización de §1.4 del diseño. El
// orden es normativo: determina el MotivoDenegacion, que aguas arriba
// determina el código HTTP (404 vs 403, INV-TEN-17).
func TestINV_TEN_17_18_EvaluadorDeAutorizacion_OrdenNormativo(t *testing.T) {
	t.Run("paso 1: sin membresía -> sin_membresia", func(t *testing.T) {
		decision := Autorizar(EstadoOrganizacionActiva, nil, PermisoOrganizacionVer)
		if decision.Permitido() {
			t.Fatal("no debería permitirse sin membresía")
		}
		motivo, ok := decision.Motivo()
		if !ok || !motivo.EsIgual(MotivoDenegacionSinMembresia) {
			t.Errorf("Motivo() = %v, %v; esperado sin_membresia", motivo, ok)
		}
		if _, tieneRol := decision.Rol(); tieneRol {
			t.Error("sin membresía no debe reportar ningún rol")
		}
	})

	t.Run("paso 2: membresía suspendida -> membresia_suspendida, incluso si la org no es operativa", func(t *testing.T) {
		m := membresiaSuspendidaDePrueba(t, RolMiembro)
		// La organización está suspendida TAMBIÉN, para probar que el paso 2
		// gana sobre el paso 3 (el orden es normativo: si el sujeto ni
		// siquiera tiene membresía activa, no debe poder inferir el estado
		// interno de la organización).
		decision := Autorizar(EstadoOrganizacionSuspendida, m, PermisoOrganizacionVer)
		if decision.Permitido() {
			t.Fatal("no debería permitirse con membresía suspendida")
		}
		motivo, ok := decision.Motivo()
		if !ok || !motivo.EsIgual(MotivoDenegacionMembresiaSuspendida) {
			t.Errorf("Motivo() = %v, %v; esperado membresia_suspendida", motivo, ok)
		}
	})

	t.Run("paso 3: organización no operativa -> organizacion_no_operativa", func(t *testing.T) {
		m := membresiaActivaDePrueba(t, RolMiembro)
		decision := Autorizar(EstadoOrganizacionSuspendida, m, PermisoOrganizacionVer)
		if decision.Permitido() {
			t.Fatal("no debería permitirse con organización no operativa")
		}
		motivo, ok := decision.Motivo()
		if !ok || !motivo.EsIgual(MotivoDenegacionOrganizacionNoOperativa) {
			t.Errorf("Motivo() = %v, %v; esperado organizacion_no_operativa", motivo, ok)
		}
	})

	t.Run("paso 4: rol insuficiente -> rol_insuficiente", func(t *testing.T) {
		m := membresiaActivaDePrueba(t, RolMiembro)
		decision := Autorizar(EstadoOrganizacionActiva, m, PermisoOrganizacionArchivar)
		if decision.Permitido() {
			t.Fatal("un miembro no debe poder archivar la organización")
		}
		motivo, ok := decision.Motivo()
		if !ok || !motivo.EsIgual(MotivoDenegacionRolInsuficiente) {
			t.Errorf("Motivo() = %v, %v; esperado rol_insuficiente", motivo, ok)
		}
	})

	t.Run("paso 5: permitido", func(t *testing.T) {
		m := membresiaActivaDePrueba(t, RolPropietario)
		decision := Autorizar(EstadoOrganizacionActiva, m, PermisoOrganizacionArchivar)
		if !decision.Permitido() {
			t.Fatal("un propietario en organización activa debe poder archivar")
		}
		if _, tieneMotivo := decision.Motivo(); tieneMotivo {
			t.Error("una concesión no debe reportar motivo de denegación")
		}
		rol, ok := decision.Rol()
		if !ok || !rol.EsIgual(RolPropietario) {
			t.Errorf("Rol() = %v, %v; esperado propietario", rol, ok)
		}
	})
}

func TestMotivoDenegacionDesde(t *testing.T) {
	for _, v := range []string{"sin_membresia", "membresia_suspendida", "organizacion_no_operativa", "rol_insuficiente"} {
		if _, err := MotivoDenegacionDesde(v); err != nil {
			t.Errorf("%q debería ser válido: %v", v, err)
		}
	}
	if _, err := MotivoDenegacionDesde("inexistente"); err == nil {
		t.Fatal("se esperaba error para un motivo fuera del catálogo")
	} else {
		var errMotivo *ErrMotivoDenegacionInvalido
		if !errors.As(err, &errMotivo) {
			t.Errorf("se esperaba *ErrMotivoDenegacionInvalido, obtuvo %T", err)
		}
	}
}

func TestMembresia_Permite_DelegaEnAutorizar(t *testing.T) {
	m := membresiaActivaDePrueba(t, RolAdministrador)
	if !m.Permite(PermisoMiembroInvitar, EstadoOrganizacionActiva) {
		t.Error("un administrador en organización activa debe poder invitar")
	}
	if m.Permite(PermisoOrganizacionArchivar, EstadoOrganizacionActiva) {
		t.Error("un administrador no debe poder archivar")
	}
	if m.Permite(PermisoMiembroInvitar, EstadoOrganizacionSuspendida) {
		t.Error("una organización no operativa no debe autorizar nada (INV-TEN-18)")
	}
}
