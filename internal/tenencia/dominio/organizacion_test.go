package dominio

import (
	"errors"
	"testing"
	"time"
)

func aliasDePrueba(t *testing.T) AliasOrganizacion {
	t.Helper()
	a, err := NuevoAlias("acme")
	if err != nil {
		t.Fatalf("no se pudo construir alias de prueba: %v", err)
	}
	return a
}

func nombreDePrueba(t *testing.T) NombreOrganizacion {
	t.Helper()
	n, err := NuevoNombre("Acme Corp")
	if err != nil {
		t.Fatalf("no se pudo construir nombre de prueba: %v", err)
	}
	return n
}

func motivoDePrueba(t *testing.T) MotivoCambioEstado {
	t.Helper()
	m, err := NuevoMotivo("incumplimiento de términos de servicio")
	if err != nil {
		t.Fatalf("no se pudo construir motivo de prueba: %v", err)
	}
	return m
}

func organizacionDePrueba(t *testing.T) (*Organizacion, time.Time) {
	t.Helper()
	ahora := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	o, err := CrearOrganizacion(idOrganizacionDePrueba(t), aliasDePrueba(t), nombreDePrueba(t), idUsuarioDePrueba(t), ahora)
	if err != nil {
		t.Fatalf("no se pudo crear la organización de prueba: %v", err)
	}
	return o, ahora
}

// TestINV_TEN_01_CrearOrganizacion_NaceActiva verifica que una Organizacion
// siempre nace en estado activa, con alias normalizado, nombre no vacío y
// creadaPor no nulo (INV-TEN-01).
func TestINV_TEN_01_CrearOrganizacion_NaceActiva(t *testing.T) {
	o, ahora := organizacionDePrueba(t)
	if !o.Estado().EsIgual(EstadoOrganizacionActiva) {
		t.Errorf("Estado() = %s, esperado activa", o.Estado())
	}
	if o.Alias().EsVacio() {
		t.Error("Alias() no debe estar vacío")
	}
	if o.Nombre().EsVacio() {
		t.Error("Nombre() no debe estar vacío")
	}
	if o.CreadaPor().EsVacio() {
		t.Error("CreadaPor() no debe estar vacío")
	}
	if !o.CreadaEn().Equal(ahora) {
		t.Errorf("CreadaEn() = %v, esperado %v", o.CreadaEn(), ahora)
	}
	if !o.ActualizadaEn().Equal(ahora) {
		t.Errorf("ActualizadaEn() = %v, esperado %v (INV-TEN-14)", o.ActualizadaEn(), ahora)
	}
	if _, archivada := o.ArchivadaEn(); archivada {
		t.Error("una organización recién creada no debe estar archivada")
	}
}

func TestCrearOrganizacion_AcumulaOrganizacionCreada(t *testing.T) {
	o, ahora := organizacionDePrueba(t)
	eventos := o.EventosPendientes()
	if len(eventos) != 1 {
		t.Fatalf("se esperaba 1 evento acumulado, hay %d", len(eventos))
	}
	ev, ok := eventos[0].(OrganizacionCreada)
	if !ok {
		t.Fatalf("se esperaba OrganizacionCreada, obtuvo %T", eventos[0])
	}
	if ev.IDOrganizacion != o.ID().String() {
		t.Errorf("IDOrganizacion = %q", ev.IDOrganizacion)
	}
	if !ev.OcurridoEn().Equal(ahora) {
		t.Errorf("OcurridoEn() = %v, esperado %v", ev.OcurridoEn(), ahora)
	}
	// Drenar de nuevo debe devolver un slice vacío.
	if eventos2 := o.EventosPendientes(); len(eventos2) != 0 {
		t.Errorf("EventosPendientes() tras drenar debe estar vacío, hay %d", len(eventos2))
	}
}

func TestCrearOrganizacion_RechazaCamposVacios(t *testing.T) {
	ahora := time.Now()
	var idVacio IDOrganizacion
	if _, err := CrearOrganizacion(idVacio, aliasDePrueba(t), nombreDePrueba(t), idUsuarioDePrueba(t), ahora); err == nil {
		t.Error("se esperaba error con IDOrganizacion vacío")
	}
	var aliasVacio AliasOrganizacion
	if _, err := CrearOrganizacion(idOrganizacionDePrueba(t), aliasVacio, nombreDePrueba(t), idUsuarioDePrueba(t), ahora); err == nil {
		t.Error("se esperaba error con alias vacío")
	}
	var nombreVacio NombreOrganizacion
	if _, err := CrearOrganizacion(idOrganizacionDePrueba(t), aliasDePrueba(t), nombreVacio, idUsuarioDePrueba(t), ahora); err == nil {
		t.Error("se esperaba error con nombre vacío")
	}
	var creadaPorVacio IDUsuario
	if _, err := CrearOrganizacion(idOrganizacionDePrueba(t), aliasDePrueba(t), nombreDePrueba(t), creadaPorVacio, ahora); err == nil {
		t.Error("se esperaba error con creadaPor vacío")
	}
}

func TestOrganizacion_Renombrar(t *testing.T) {
	o, ahora := organizacionDePrueba(t)
	o.EventosPendientes()
	despues := ahora.Add(time.Hour)
	nuevoNombre, _ := NuevoNombre("Acme International")
	if err := o.Renombrar(nuevoNombre, despues); err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if !o.Nombre().EsIgual(nuevoNombre) {
		t.Errorf("Nombre() = %v, esperado %v", o.Nombre(), nuevoNombre)
	}
	if !o.ActualizadaEn().Equal(despues) {
		t.Errorf("ActualizadaEn() = %v, esperado %v", o.ActualizadaEn(), despues)
	}
	eventos := o.EventosPendientes()
	if len(eventos) != 1 {
		t.Fatalf("se esperaba 1 evento, hay %d", len(eventos))
	}
	ev, ok := eventos[0].(OrganizacionActualizada)
	if !ok {
		t.Fatalf("se esperaba OrganizacionActualizada, obtuvo %T", eventos[0])
	}
	if len(ev.Campos) != 1 || ev.Campos[0] != "nombre" {
		t.Errorf("Campos = %v, esperado [nombre]", ev.Campos)
	}
}

func TestOrganizacion_Renombrar_NoOpIdempotente(t *testing.T) {
	o, ahora := organizacionDePrueba(t)
	o.EventosPendientes()
	despues := ahora.Add(time.Hour)
	if err := o.Renombrar(o.Nombre(), despues); err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if !o.ActualizadaEn().Equal(ahora) {
		t.Error("un no-op no debe actualizar ActualizadaEn()")
	}
	if eventos := o.EventosPendientes(); len(eventos) != 0 {
		t.Errorf("un no-op no debe acumular eventos, hay %d", len(eventos))
	}
}

func TestOrganizacion_CambiarAlias(t *testing.T) {
	o, ahora := organizacionDePrueba(t)
	o.EventosPendientes()
	despues := ahora.Add(time.Hour)
	nuevoAlias, _ := NuevoAlias("acme-international")
	if err := o.CambiarAlias(nuevoAlias, despues); err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if !o.Alias().EsIgual(nuevoAlias) {
		t.Errorf("Alias() = %v, esperado %v", o.Alias(), nuevoAlias)
	}
	eventos := o.EventosPendientes()
	if len(eventos) != 1 {
		t.Fatalf("se esperaba 1 evento, hay %d", len(eventos))
	}
	ev, ok := eventos[0].(OrganizacionActualizada)
	if !ok {
		t.Fatalf("se esperaba OrganizacionActualizada, obtuvo %T", eventos[0])
	}
	if len(ev.Campos) != 1 || ev.Campos[0] != "alias" {
		t.Errorf("Campos = %v, esperado [alias]", ev.Campos)
	}
}

func TestOrganizacion_CambiarAlias_NoOpIdempotente(t *testing.T) {
	o, ahora := organizacionDePrueba(t)
	o.EventosPendientes()
	if err := o.CambiarAlias(o.Alias(), ahora.Add(time.Hour)); err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if eventos := o.EventosPendientes(); len(eventos) != 0 {
		t.Errorf("un no-op no debe acumular eventos, hay %d", len(eventos))
	}
}

// TestINV_TEN_04_Organizacion_MaquinaEstados verifica que Suspender,
// Reactivar y Archivar respetan la máquina de estados de §1.4 del diseño y
// que archivada es terminal e irreversible (INV-TEN-04).
func TestINV_TEN_04_Organizacion_MaquinaEstados(t *testing.T) {
	t.Run("activa -> suspendida -> activa", func(t *testing.T) {
		o, ahora := organizacionDePrueba(t)
		o.EventosPendientes()
		if err := o.Suspender(motivoDePrueba(t), ahora.Add(time.Hour)); err != nil {
			t.Fatalf("no se esperaba error al suspender: %v", err)
		}
		if !o.Estado().EsIgual(EstadoOrganizacionSuspendida) {
			t.Errorf("Estado() = %s, esperado suspendida", o.Estado())
		}
		motivo, ok := o.MotivoEstado()
		if !ok || motivo.Valor() == "" {
			t.Error("una organización suspendida debe llevar un motivo")
		}
		eventos := o.EventosPendientes()
		if len(eventos) != 1 {
			t.Fatalf("se esperaba 1 evento, hay %d", len(eventos))
		}
		if _, ok := eventos[0].(EstadoOrganizacionCambiado); !ok {
			t.Fatalf("se esperaba EstadoOrganizacionCambiado, obtuvo %T", eventos[0])
		}

		if err := o.Reactivar(ahora.Add(2 * time.Hour)); err != nil {
			t.Fatalf("no se esperaba error al reactivar: %v", err)
		}
		if !o.Estado().EsIgual(EstadoOrganizacionActiva) {
			t.Errorf("Estado() = %s, esperado activa", o.Estado())
		}
		if _, tiene := o.MotivoEstado(); tiene {
			t.Error("al reactivar, el motivo de suspensión debe limpiarse")
		}
	})

	t.Run("archivar desde activa es terminal", func(t *testing.T) {
		o, ahora := organizacionDePrueba(t)
		o.EventosPendientes()
		if err := o.Archivar(motivoDePrueba(t), ahora.Add(time.Hour)); err != nil {
			t.Fatalf("no se esperaba error al archivar: %v", err)
		}
		if !o.Estado().EsIgual(EstadoOrganizacionArchivada) {
			t.Errorf("Estado() = %s, esperado archivada", o.Estado())
		}
		archivadaEn, ok := o.ArchivadaEn()
		if !ok || !archivadaEn.Equal(ahora.Add(time.Hour)) {
			t.Errorf("ArchivadaEn() = %v, %v", archivadaEn, ok)
		}
		if err := o.Reactivar(ahora.Add(2 * time.Hour)); err == nil {
			t.Fatal("archivada no debe poder reactivarse (INV-TEN-04)")
		}
		if err := o.Suspender(motivoDePrueba(t), ahora.Add(2*time.Hour)); err == nil {
			t.Fatal("archivada no debe poder suspenderse")
		}
		if err := o.Archivar(motivoDePrueba(t), ahora.Add(2*time.Hour)); err == nil {
			t.Fatal("archivada no debe poder archivarse de nuevo")
		}
	})

	t.Run("archivar desde suspendida", func(t *testing.T) {
		o, ahora := organizacionDePrueba(t)
		if err := o.Suspender(motivoDePrueba(t), ahora.Add(time.Hour)); err != nil {
			t.Fatalf("no se esperaba error: %v", err)
		}
		if err := o.Archivar(motivoDePrueba(t), ahora.Add(2*time.Hour)); err != nil {
			t.Fatalf("no se esperaba error al archivar desde suspendida: %v", err)
		}
		if !o.Estado().EsIgual(EstadoOrganizacionArchivada) {
			t.Errorf("Estado() = %s, esperado archivada", o.Estado())
		}
	})

	t.Run("transición inválida devuelve el error tipado", func(t *testing.T) {
		o, ahora := organizacionDePrueba(t)
		if err := o.Reactivar(ahora); err == nil {
			t.Fatal("se esperaba error: no se puede reactivar una organización activa")
		} else {
			var errTransicion *ErrTransicionEstadoOrganizacionInvalida
			if !errors.As(err, &errTransicion) {
				t.Errorf("se esperaba *ErrTransicionEstadoOrganizacionInvalida, obtuvo %T", err)
			}
		}
	})
}

func TestOrganizacion_Suspender_ExigeMotivo(t *testing.T) {
	o, ahora := organizacionDePrueba(t)
	var motivoVacio MotivoCambioEstado
	if err := o.Suspender(motivoVacio, ahora); err == nil {
		t.Fatal("se esperaba error: suspender exige motivo")
	}
}

func TestOrganizacion_Archivar_ExigeMotivo(t *testing.T) {
	o, ahora := organizacionDePrueba(t)
	var motivoVacio MotivoCambioEstado
	if err := o.Archivar(motivoVacio, ahora); err == nil {
		t.Fatal("se esperaba error: archivar exige motivo")
	}
}

// TestINV_TEN_05_Suspender_NoMutaMembresias documenta, a nivel de tipo, que
// Organizacion.Suspender no tiene forma de tocar ninguna Membresia: el
// agregado no contiene ninguna colección de membresías (INV-TEN-05, §1.2
// del diseño). No hay estado que verificar aquí más allá del propio de
// Organizacion, que es justamente el punto.
func TestINV_TEN_05_Suspender_NoMutaMembresias(t *testing.T) {
	o, ahora := organizacionDePrueba(t)
	if err := o.Suspender(motivoDePrueba(t), ahora); err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	// El único estado mutado es el de la propia organización.
	if !o.Estado().EsIgual(EstadoOrganizacionSuspendida) {
		t.Error("solo el estado de la organización debe cambiar")
	}
}

// TestINV_TEN_18_EstaOperativa verifica que solo una organización activa se
// reporta como operativa.
func TestINV_TEN_18_EstaOperativa(t *testing.T) {
	o, ahora := organizacionDePrueba(t)
	if err := o.EstaOperativa(); err != nil {
		t.Errorf("una organización activa debe ser operativa: %v", err)
	}
	if err := o.Suspender(motivoDePrueba(t), ahora); err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if err := o.EstaOperativa(); err == nil {
		t.Fatal("una organización suspendida no debe ser operativa")
	} else {
		var errNoOperativa *ErrOrganizacionNoOperativa
		if !errors.As(err, &errNoOperativa) {
			t.Errorf("se esperaba *ErrOrganizacionNoOperativa, obtuvo %T", err)
		}
	}
}

func TestReconstituirOrganizacion_NoAcumulaEventos(t *testing.T) {
	ahora := time.Now()
	archivadaEn := ahora.Add(time.Hour)
	o := ReconstituirOrganizacion(
		idOrganizacionDePrueba(t),
		aliasDePrueba(t),
		nombreDePrueba(t),
		EstadoOrganizacionArchivada,
		idUsuarioDePrueba(t),
		ahora,
		ahora,
		&archivadaEn,
		motivoDePrueba(t),
	)
	if eventos := o.EventosPendientes(); len(eventos) != 0 {
		t.Errorf("Reconstituir no debe acumular eventos, hay %d", len(eventos))
	}
	got, ok := o.ArchivadaEn()
	if !ok || !got.Equal(archivadaEn) {
		t.Errorf("ArchivadaEn() = %v, %v; esperado %v", got, ok, archivadaEn)
	}
}
