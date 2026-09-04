package dominio

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

// TestINV_TEN_23_EventosNuncaTransportanElTokenNiSuHash recorre por
// reflexión los 10 eventos de dominio del catálogo de §1.6 y verifica que
// ninguno tiene un campo de tipo TokenInvitacionPlano ni HashTokenInvitacion,
// y que ningún nombre de campo sugiere transportar un token o un secreto.
// Esto es lo que exige INV-TEN-23: "el token de invitación en claro nunca
// [...] viaja en un evento de dominio", verificado con el mismo tipo de
// test que ya usan identidad/dominio y acceso/dominio.
func TestINV_TEN_23_EventosNuncaTransportanElTokenNiSuHash(t *testing.T) {
	ahora := time.Now()
	idOrg, _ := IDOrganizacionDesde("018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6d")
	idMembresia, _ := IDMembresiaDesde("018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6e")
	idInvitacion, _ := IDInvitacionDesde("018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6f")
	idUsuario, _ := IDUsuarioDesde("018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b6000")
	alias, _ := NuevoAlias("acme")
	correo, _ := NuevoCorreoDestinatario("invitado@ejemplo.com")

	eventos := []EventoDominio{
		NuevoOrganizacionCreada(idOrg, alias, idUsuario, ahora),
		NuevoOrganizacionActualizada(idOrg, []string{"nombre"}, ahora),
		NuevoEstadoOrganizacionCambiado(idOrg, EstadoOrganizacionActiva, EstadoOrganizacionSuspendida, "motivo", ahora),
		NuevoMiembroAgregado(idMembresia, idOrg, idUsuario, RolMiembro, ViaFundacion, ahora),
		NuevoRolDeMiembroCambiado(idMembresia, idOrg, idUsuario, RolMiembro, RolAdministrador, ahora),
		NuevoEstadoMembresiaCambiado(idMembresia, idOrg, idUsuario, EstadoMembresiaActiva, EstadoMembresiaSuspendida, ahora),
		NuevoMiembroRemovido(idMembresia, idOrg, idUsuario, RolMiembro, true, ahora),
		NuevoMiembroInvitado(idInvitacion, idOrg, correo, RolMiembro, ahora),
		NuevoInvitacionResuelta(idInvitacion.String(), idOrg.String(), DesenlaceAceptada, ResultadoExito, ahora),
		NuevoAutorizacionDenegada(idUsuario, idOrg, PermisoOrganizacionVer, MotivoDenegacionSinMembresia, "", ahora),
	}

	if len(eventos) != 10 {
		t.Fatalf("se esperaban 10 eventos de dominio (tabla 1.6 del diseño), hay %d", len(eventos))
	}

	tipoTokenPlano := reflect.TypeOf(TokenInvitacionPlano{})
	tipoHashToken := reflect.TypeOf(HashTokenInvitacion{})
	nombresPeligrosos := []string{"tokenplano", "hashtoken", "secreto"}

	for _, ev := range eventos {
		v := reflect.ValueOf(ev)
		tipoEv := v.Type()
		for i := 0; i < tipoEv.NumField(); i++ {
			campo := tipoEv.Field(i)
			if campo.Type == tipoTokenPlano || campo.Type == tipoHashToken {
				t.Errorf("%s.%s tiene tipo %s: un evento no puede transportar el token ni su hash (INV-TEN-23)", tipoEv.Name(), campo.Name, campo.Type)
			}
			nombreMin := strings.ToLower(campo.Name)
			for _, peligroso := range nombresPeligrosos {
				if strings.Contains(nombreMin, peligroso) {
					t.Errorf("%s.%s tiene un nombre que sugiere transportar un secreto/token", tipoEv.Name(), campo.Name)
				}
			}
		}
		if ev.NombreEvento() == "" {
			t.Errorf("%T.NombreEvento() no debe estar vacío", ev)
		}
		if !ev.OcurridoEn().Equal(ahora) {
			t.Errorf("%T.OcurridoEn() = %v, esperado %v", ev, ev.OcurridoEn(), ahora)
		}
	}
}

func TestEventos_NombresUnicos(t *testing.T) {
	ahora := time.Now()
	idOrg, _ := IDOrganizacionDesde("018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6d")
	idMembresia, _ := IDMembresiaDesde("018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6e")
	idInvitacion, _ := IDInvitacionDesde("018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6f")
	idUsuario, _ := IDUsuarioDesde("018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b6000")
	alias, _ := NuevoAlias("acme")
	correo, _ := NuevoCorreoDestinatario("invitado@ejemplo.com")

	eventos := []EventoDominio{
		NuevoOrganizacionCreada(idOrg, alias, idUsuario, ahora),
		NuevoOrganizacionActualizada(idOrg, []string{"nombre"}, ahora),
		NuevoEstadoOrganizacionCambiado(idOrg, EstadoOrganizacionActiva, EstadoOrganizacionSuspendida, "motivo", ahora),
		NuevoMiembroAgregado(idMembresia, idOrg, idUsuario, RolMiembro, ViaFundacion, ahora),
		NuevoRolDeMiembroCambiado(idMembresia, idOrg, idUsuario, RolMiembro, RolAdministrador, ahora),
		NuevoEstadoMembresiaCambiado(idMembresia, idOrg, idUsuario, EstadoMembresiaActiva, EstadoMembresiaSuspendida, ahora),
		NuevoMiembroRemovido(idMembresia, idOrg, idUsuario, RolMiembro, true, ahora),
		NuevoMiembroInvitado(idInvitacion, idOrg, correo, RolMiembro, ahora),
		NuevoInvitacionResuelta(idInvitacion.String(), idOrg.String(), DesenlaceAceptada, ResultadoExito, ahora),
		NuevoAutorizacionDenegada(idUsuario, idOrg, PermisoOrganizacionVer, MotivoDenegacionSinMembresia, "", ahora),
	}
	vistos := map[string]bool{}
	for _, ev := range eventos {
		nombre := ev.NombreEvento()
		if vistos[nombre] {
			t.Errorf("el nombre de evento %q está duplicado", nombre)
		}
		vistos[nombre] = true
	}
}

func TestEventos_IDAgregado(t *testing.T) {
	ahora := time.Now()
	idOrg, _ := IDOrganizacionDesde("018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6d")
	idMembresia, _ := IDMembresiaDesde("018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6e")
	idInvitacion, _ := IDInvitacionDesde("018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6f")
	idUsuario, _ := IDUsuarioDesde("018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b6000")
	alias, _ := NuevoAlias("acme")
	correo, _ := NuevoCorreoDestinatario("invitado@ejemplo.com")

	casos := []struct {
		ev       EventoDominio
		esperado string
	}{
		{NuevoOrganizacionCreada(idOrg, alias, idUsuario, ahora), idOrg.String()},
		{NuevoOrganizacionActualizada(idOrg, []string{"alias"}, ahora), idOrg.String()},
		{NuevoEstadoOrganizacionCambiado(idOrg, EstadoOrganizacionActiva, EstadoOrganizacionSuspendida, "m", ahora), idOrg.String()},
		{NuevoMiembroAgregado(idMembresia, idOrg, idUsuario, RolMiembro, ViaFundacion, ahora), idMembresia.String()},
		{NuevoRolDeMiembroCambiado(idMembresia, idOrg, idUsuario, RolMiembro, RolAdministrador, ahora), idMembresia.String()},
		{NuevoEstadoMembresiaCambiado(idMembresia, idOrg, idUsuario, EstadoMembresiaActiva, EstadoMembresiaSuspendida, ahora), idMembresia.String()},
		{NuevoMiembroRemovido(idMembresia, idOrg, idUsuario, RolMiembro, false, ahora), idMembresia.String()},
		{NuevoMiembroInvitado(idInvitacion, idOrg, correo, RolMiembro, ahora), idInvitacion.String()},
		{NuevoInvitacionResuelta(idInvitacion.String(), idOrg.String(), DesenlaceAceptada, ResultadoExito, ahora), idInvitacion.String()},
		{NuevoAutorizacionDenegada(idUsuario, idOrg, PermisoOrganizacionVer, MotivoDenegacionSinMembresia, "", ahora), idOrg.String()},
	}
	for _, c := range casos {
		if c.ev.IDAgregado() != c.esperado {
			t.Errorf("%s.IDAgregado() = %q, esperado %q", c.ev.NombreEvento(), c.ev.IDAgregado(), c.esperado)
		}
	}
}

func TestInvitacionResuelta_IDAgregado_VacioSiTokenNoResolvio(t *testing.T) {
	ev := NuevoInvitacionResuelta("", "", DesenlaceIntentoFallido, ResultadoFallo, time.Now())
	if ev.IDAgregado() != "" {
		t.Errorf("IDAgregado() = %q, esperado vacío cuando el token no resolvió a ninguna invitación", ev.IDAgregado())
	}
}
