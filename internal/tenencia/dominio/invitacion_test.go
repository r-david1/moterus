package dominio

import (
	"errors"
	"testing"
	"time"
)

func destinatarioDePrueba(t *testing.T) CorreoDestinatario {
	t.Helper()
	c, err := NuevoCorreoDestinatario("invitado@ejemplo.com")
	if err != nil {
		t.Fatalf("no se pudo construir el destinatario de prueba: %v", err)
	}
	return c
}

func hashTokenDePrueba(t *testing.T, relleno string) HashTokenInvitacion {
	t.Helper()
	h, err := NuevoHashTokenInvitacion(repetir(relleno, 64))
	if err != nil {
		t.Fatalf("no se pudo construir el hash de prueba: %v", err)
	}
	return h
}

func repetir(s string, n int) string {
	out := make([]byte, 0, n)
	for len(out) < n {
		out = append(out, s...)
	}
	return string(out[:n])
}

func politicaDePrueba(t *testing.T) PoliticaOrganizacion {
	t.Helper()
	p, err := NuevaPoliticaOrganizacion(7*24*time.Hour, 200, 50, 20)
	if err != nil {
		t.Fatalf("no se pudo construir la política de prueba: %v", err)
	}
	return p
}

func invitacionDePrueba(t *testing.T) (*Invitacion, time.Time) {
	t.Helper()
	ahora := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	inv, err := CrearInvitacion(
		idInvitacionDePrueba(t),
		idOrganizacionDePrueba(t),
		destinatarioDePrueba(t),
		RolMiembro,
		hashTokenDePrueba(t, "a"),
		idUsuarioDePrueba(t),
		ahora,
		politicaDePrueba(t),
	)
	if err != nil {
		t.Fatalf("no se pudo crear la invitación de prueba: %v", err)
	}
	return inv, ahora
}

func idInvitacionDePrueba(t *testing.T) IDInvitacion {
	t.Helper()
	id, err := IDInvitacionDesde("018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b6000")
	if err != nil {
		t.Fatalf("no se pudo construir IDInvitacion de prueba: %v", err)
	}
	return id
}

// --- CrearInvitacion ----------------------------------------------------------

func TestCrearInvitacion_NacePendiente(t *testing.T) {
	inv, ahora := invitacionDePrueba(t)
	if !inv.Estado().EsIgual(EstadoInvitacionPendiente) {
		t.Errorf("Estado() = %v, esperado pendiente", inv.Estado())
	}
	esperadoExpira := ahora.Add(politicaDePrueba(t).VigenciaInvitacion())
	if !inv.ExpiraEn().Equal(esperadoExpira) {
		t.Errorf("ExpiraEn() = %v, esperado %v", inv.ExpiraEn(), esperadoExpira)
	}
	if !inv.CreadaEn().Equal(ahora) {
		t.Errorf("CreadaEn() = %v, esperado %v", inv.CreadaEn(), ahora)
	}
	if _, resuelta := inv.ResueltaEn(); resuelta {
		t.Error("una invitación recién creada no debe estar resuelta")
	}
}

func TestInvitacion_Getters(t *testing.T) {
	inv, _ := invitacionDePrueba(t)
	if !inv.OrganizacionID().EsIgual(idOrganizacionDePrueba(t)) {
		t.Errorf("OrganizacionID() = %v", inv.OrganizacionID())
	}
	if !inv.Destinatario().EsIgual(destinatarioDePrueba(t)) {
		t.Errorf("Destinatario() = %v", inv.Destinatario())
	}
	if !inv.RolPropuesto().EsIgual(RolMiembro) {
		t.Errorf("RolPropuesto() = %v, esperado miembro", inv.RolPropuesto())
	}
	if !inv.HashToken().EsIgual(hashTokenDePrueba(t, "a")) {
		t.Errorf("HashToken() = %v", inv.HashToken())
	}
	if !inv.InvitadaPor().EsIgual(idUsuarioDePrueba(t)) {
		t.Errorf("InvitadaPor() = %v", inv.InvitadaPor())
	}
}

func TestCrearInvitacion_AcumulaMiembroInvitado(t *testing.T) {
	inv, ahora := invitacionDePrueba(t)
	eventos := inv.EventosPendientes()
	if len(eventos) != 1 {
		t.Fatalf("se esperaba 1 evento, hay %d", len(eventos))
	}
	ev, ok := eventos[0].(MiembroInvitado)
	if !ok {
		t.Fatalf("se esperaba MiembroInvitado, obtuvo %T", eventos[0])
	}
	if ev.IDInvitacion != inv.ID().String() {
		t.Errorf("IDInvitacion = %q", ev.IDInvitacion)
	}
	if !ev.OcurridoEn().Equal(ahora) {
		t.Errorf("OcurridoEn() = %v, esperado %v", ev.OcurridoEn(), ahora)
	}
}

func TestCrearInvitacion_RechazaCamposVacios(t *testing.T) {
	ahora := time.Now()
	politica := politicaDePrueba(t)
	var idVacio IDInvitacion
	if _, err := CrearInvitacion(idVacio, idOrganizacionDePrueba(t), destinatarioDePrueba(t), RolMiembro, hashTokenDePrueba(t, "a"), idUsuarioDePrueba(t), ahora, politica); err == nil {
		t.Error("se esperaba error con IDInvitacion vacío")
	}
	var destinatarioVacio CorreoDestinatario
	if _, err := CrearInvitacion(idInvitacionDePrueba(t), idOrganizacionDePrueba(t), destinatarioVacio, RolMiembro, hashTokenDePrueba(t, "a"), idUsuarioDePrueba(t), ahora, politica); err == nil {
		t.Error("se esperaba error con destinatario vacío")
	}
	var rolVacio Rol
	if _, err := CrearInvitacion(idInvitacionDePrueba(t), idOrganizacionDePrueba(t), destinatarioDePrueba(t), rolVacio, hashTokenDePrueba(t, "a"), idUsuarioDePrueba(t), ahora, politica); err == nil {
		t.Error("se esperaba error con rol propuesto vacío")
	}
	var hashVacio HashTokenInvitacion
	if _, err := CrearInvitacion(idInvitacionDePrueba(t), idOrganizacionDePrueba(t), destinatarioDePrueba(t), RolMiembro, hashVacio, idUsuarioDePrueba(t), ahora, politica); err == nil {
		t.Error("se esperaba error con hash vacío (INV-TEN-23: el dominio nunca ve el token en claro)")
	}
	var invitadaPorVacio IDUsuario
	if _, err := CrearInvitacion(idInvitacionDePrueba(t), idOrganizacionDePrueba(t), destinatarioDePrueba(t), RolMiembro, hashTokenDePrueba(t, "a"), invitadaPorVacio, ahora, politica); err == nil {
		t.Error("se esperaba error con invitadaPor vacío")
	}
}

func TestReconstituirInvitacion_NoAcumulaEventos(t *testing.T) {
	ahora := time.Now()
	resueltaEn := ahora.Add(time.Hour)
	inv := ReconstituirInvitacion(
		idInvitacionDePrueba(t),
		idOrganizacionDePrueba(t),
		destinatarioDePrueba(t),
		RolMiembro,
		EstadoInvitacionAceptada,
		hashTokenDePrueba(t, "a"),
		idUsuarioDePrueba(t),
		ahora,
		ahora.Add(24*time.Hour),
		&resueltaEn,
	)
	if eventos := inv.EventosPendientes(); len(eventos) != 0 {
		t.Errorf("Reconstituir no debe acumular eventos, hay %d", len(eventos))
	}
	got, ok := inv.ResueltaEn()
	if !ok || !got.Equal(resueltaEn) {
		t.Errorf("ResueltaEn() = %v, %v; esperado %v", got, ok, resueltaEn)
	}
}

// --- EstaVigente ----------------------------------------------------------

func TestInvitacion_EstaVigente(t *testing.T) {
	inv, ahora := invitacionDePrueba(t)
	if !inv.EstaVigente(ahora) {
		t.Error("una invitación recién creada debe estar vigente")
	}
	if inv.EstaVigente(inv.ExpiraEn().Add(time.Second)) {
		t.Error("una invitación pasada su expiración no debe estar vigente")
	}
	if inv.EstaVigente(inv.ExpiraEn()) {
		t.Error("el instante exacto de expiración no debe considerarse vigente")
	}
}

// --- Aceptar (INV-TEN-21, INV-TEN-24) -----------------------------------------

func TestInvitacion_Aceptar_Exitosa(t *testing.T) {
	inv, ahora := invitacionDePrueba(t)
	inv.EventosPendientes()
	despues := ahora.Add(time.Hour)
	if err := inv.Aceptar(destinatarioDePrueba(t), despues); err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if !inv.Estado().EsIgual(EstadoInvitacionAceptada) {
		t.Errorf("Estado() = %v, esperado aceptada", inv.Estado())
	}
	resueltaEn, ok := inv.ResueltaEn()
	if !ok || !resueltaEn.Equal(despues) {
		t.Errorf("ResueltaEn() = %v, %v", resueltaEn, ok)
	}
	eventos := inv.EventosPendientes()
	if len(eventos) != 1 {
		t.Fatalf("se esperaba 1 evento, hay %d", len(eventos))
	}
	ev, ok := eventos[0].(InvitacionResuelta)
	if !ok {
		t.Fatalf("se esperaba InvitacionResuelta, obtuvo %T", eventos[0])
	}
	if ev.Desenlace != DesenlaceAceptada || ev.Resultado != ResultadoExito {
		t.Errorf("Desenlace=%q Resultado=%q", ev.Desenlace, ev.Resultado)
	}
}

// TestINV_TEN_21_Aceptar_CorreoDistinto verifica que un correo distinto al
// destinatario produce ErrInvitacionAjena, y que la comparación es la
// constante de tiempo de CorreoDestinatario.
func TestINV_TEN_21_Aceptar_CorreoDistinto(t *testing.T) {
	inv, ahora := invitacionDePrueba(t)
	otro, _ := NuevoCorreoDestinatario("otro@ejemplo.com")
	if err := inv.Aceptar(otro, ahora); err == nil {
		t.Fatal("se esperaba ErrInvitacionAjena")
	} else {
		var errAjena *ErrInvitacionAjena
		if !errors.As(err, &errAjena) {
			t.Errorf("se esperaba *ErrInvitacionAjena, obtuvo %T", err)
		}
	}
	if !inv.Estado().EsIgual(EstadoInvitacionPendiente) {
		t.Error("un intento rechazado no debe mutar el estado de la invitación")
	}
}

// TestINV_TEN_24_Aceptar_InvitacionNoVigente verifica que una invitación no
// pendiente, o pendiente pero expirada, produce ErrInvitacionInvalida — el
// mismo error para ambos casos (INV-TEN-24).
func TestINV_TEN_24_Aceptar_InvitacionNoVigente(t *testing.T) {
	t.Run("expirada por tiempo", func(t *testing.T) {
		inv, _ := invitacionDePrueba(t)
		despuesDeExpirar := inv.ExpiraEn().Add(time.Second)
		if err := inv.Aceptar(destinatarioDePrueba(t), despuesDeExpirar); err == nil {
			t.Fatal("se esperaba ErrInvitacionInvalida")
		} else {
			var errInvalida *ErrInvitacionInvalida
			if !errors.As(err, &errInvalida) {
				t.Errorf("se esperaba *ErrInvitacionInvalida, obtuvo %T", err)
			}
		}
	})

	t.Run("ya revocada", func(t *testing.T) {
		inv, ahora := invitacionDePrueba(t)
		if err := inv.Revocar(ahora); err != nil {
			t.Fatalf("no se esperaba error al revocar: %v", err)
		}
		if err := inv.Aceptar(destinatarioDePrueba(t), ahora.Add(time.Minute)); err == nil {
			t.Fatal("se esperaba ErrInvitacionInvalida")
		} else {
			var errInvalida *ErrInvitacionInvalida
			if !errors.As(err, &errInvalida) {
				t.Errorf("se esperaba *ErrInvitacionInvalida, obtuvo %T", err)
			}
		}
	})

	t.Run("ya aceptada", func(t *testing.T) {
		inv, ahora := invitacionDePrueba(t)
		if err := inv.Aceptar(destinatarioDePrueba(t), ahora.Add(time.Minute)); err != nil {
			t.Fatalf("no se esperaba error: %v", err)
		}
		if err := inv.Aceptar(destinatarioDePrueba(t), ahora.Add(2*time.Minute)); err == nil {
			t.Fatal("se esperaba ErrInvitacionInvalida")
		} else {
			var errInvalida *ErrInvitacionInvalida
			if !errors.As(err, &errInvalida) {
				t.Errorf("se esperaba *ErrInvitacionInvalida, obtuvo %T", err)
			}
		}
	})
}

func TestInvitacion_Aceptar_NoMutaNiAcumulaEventoEnFallo(t *testing.T) {
	inv, ahora := invitacionDePrueba(t)
	inv.EventosPendientes()
	otro, _ := NuevoCorreoDestinatario("otro@ejemplo.com")
	if err := inv.Aceptar(otro, ahora); err == nil {
		t.Fatal("se esperaba error")
	}
	if eventos := inv.EventosPendientes(); len(eventos) != 0 {
		t.Errorf("un intento fallido de aceptación no debe acumular un evento en el agregado (lo audita el caso de uso), hay %d", len(eventos))
	}
}

// --- Revocar / MarcarExpirada --------------------------------------------------

func TestInvitacion_Revocar(t *testing.T) {
	inv, ahora := invitacionDePrueba(t)
	inv.EventosPendientes()
	if err := inv.Revocar(ahora.Add(time.Hour)); err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if !inv.Estado().EsIgual(EstadoInvitacionRevocada) {
		t.Errorf("Estado() = %v, esperado revocada", inv.Estado())
	}
	eventos := inv.EventosPendientes()
	ev, ok := eventos[0].(InvitacionResuelta)
	if !ok {
		t.Fatalf("se esperaba InvitacionResuelta, obtuvo %T", eventos[0])
	}
	if ev.Desenlace != DesenlaceRevocada || ev.Resultado != ResultadoExito {
		t.Errorf("Desenlace=%q Resultado=%q", ev.Desenlace, ev.Resultado)
	}
}

func TestInvitacion_Revocar_RechazaSiYaResuelta(t *testing.T) {
	inv, ahora := invitacionDePrueba(t)
	if err := inv.Revocar(ahora); err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if err := inv.Revocar(ahora.Add(time.Minute)); err == nil {
		t.Fatal("se esperaba error al revocar una invitación ya revocada")
	} else {
		var errTransicion *ErrTransicionEstadoInvitacionInvalida
		if !errors.As(err, &errTransicion) {
			t.Errorf("se esperaba *ErrTransicionEstadoInvitacionInvalida, obtuvo %T", err)
		}
	}
}

func TestInvitacion_MarcarExpirada(t *testing.T) {
	inv, _ := invitacionDePrueba(t)
	inv.EventosPendientes()
	despues := inv.ExpiraEn().Add(time.Second)
	if err := inv.MarcarExpirada(despues); err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if !inv.Estado().EsIgual(EstadoInvitacionExpirada) {
		t.Errorf("Estado() = %v, esperado expirada", inv.Estado())
	}
	resueltaEn, ok := inv.ResueltaEn()
	if !ok || !resueltaEn.Equal(despues) {
		t.Errorf("ResueltaEn() = %v, %v; esperado %v", resueltaEn, ok, despues)
	}
	eventos := inv.EventosPendientes()
	ev, ok := eventos[0].(InvitacionResuelta)
	if !ok {
		t.Fatalf("se esperaba InvitacionResuelta, obtuvo %T", eventos[0])
	}
	if ev.Desenlace != DesenlaceExpirada || ev.Resultado != ResultadoExito {
		t.Errorf("Desenlace=%q Resultado=%q", ev.Desenlace, ev.Resultado)
	}
}

func TestInvitacion_MarcarExpirada_RechazaSiYaResuelta(t *testing.T) {
	inv, ahora := invitacionDePrueba(t)
	if err := inv.Aceptar(destinatarioDePrueba(t), ahora); err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if err := inv.MarcarExpirada(inv.ExpiraEn().Add(time.Second)); err == nil {
		t.Fatal("se esperaba error al expirar una invitación ya aceptada")
	}
}
