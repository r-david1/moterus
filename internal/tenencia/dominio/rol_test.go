package dominio

import (
	"errors"
	"testing"
)

func TestRolDesde(t *testing.T) {
	for _, v := range []string{"propietario", "administrador", "miembro"} {
		if _, err := RolDesde(v); err != nil {
			t.Errorf("%q debería ser válido: %v", v, err)
		}
	}
	if _, err := RolDesde("superadmin"); err == nil {
		t.Fatal("se esperaba error para un rol fuera del catálogo")
	} else {
		var errRol *ErrRolInvalido
		if !errors.As(err, &errRol) {
			t.Errorf("se esperaba *ErrRolInvalido, obtuvo %T", err)
		}
	}
}

// TestINV_TEN_10_OrdenTotal verifica el orden total propietario(30) >
// administrador(20) > miembro(10) (INV-TEN-10).
func TestINV_TEN_10_OrdenTotal(t *testing.T) {
	if RolPropietario.Nivel() != 30 {
		t.Errorf("RolPropietario.Nivel() = %d, esperado 30", RolPropietario.Nivel())
	}
	if RolAdministrador.Nivel() != 20 {
		t.Errorf("RolAdministrador.Nivel() = %d, esperado 20", RolAdministrador.Nivel())
	}
	if RolMiembro.Nivel() != 10 {
		t.Errorf("RolMiembro.Nivel() = %d, esperado 10", RolMiembro.Nivel())
	}
	if !RolPropietario.DominaA(RolAdministrador) {
		t.Error("propietario debe dominar a administrador")
	}
	if !RolPropietario.DominaA(RolMiembro) {
		t.Error("propietario debe dominar a miembro")
	}
	if !RolAdministrador.DominaA(RolMiembro) {
		t.Error("administrador debe dominar a miembro")
	}
	if RolMiembro.DominaA(RolAdministrador) || RolMiembro.DominaA(RolPropietario) || RolAdministrador.DominaA(RolPropietario) {
		t.Error("un rol de menor nivel nunca debe dominar a uno de mayor nivel")
	}
	if RolPropietario.DominaA(RolPropietario) || RolAdministrador.DominaA(RolAdministrador) || RolMiembro.DominaA(RolMiembro) {
		t.Error("ningún rol se domina a sí mismo")
	}
}

// TestMatrizDePermisos_TablaCompleta verifica la matriz literal de §1.4 del
// diseño, rol por rol y permiso por permiso.
func TestMatrizDePermisos_TablaCompleta(t *testing.T) {
	casos := []struct {
		rol     Rol
		permiso Permiso
		concede bool
	}{
		{RolPropietario, PermisoOrganizacionVer, true},
		{RolPropietario, PermisoOrganizacionEditar, true},
		{RolPropietario, PermisoOrganizacionArchivar, true},
		{RolPropietario, PermisoMiembroVer, true},
		{RolPropietario, PermisoMiembroInvitar, true},
		{RolPropietario, PermisoMiembroCambiarRol, true},
		{RolPropietario, PermisoMiembroRemover, true},
		{RolPropietario, PermisoPropiedadTransferir, true},

		{RolAdministrador, PermisoOrganizacionVer, true},
		{RolAdministrador, PermisoOrganizacionEditar, true},
		{RolAdministrador, PermisoOrganizacionArchivar, false},
		{RolAdministrador, PermisoMiembroVer, true},
		{RolAdministrador, PermisoMiembroInvitar, true},
		{RolAdministrador, PermisoMiembroCambiarRol, true},
		{RolAdministrador, PermisoMiembroRemover, true},
		{RolAdministrador, PermisoPropiedadTransferir, false},

		{RolMiembro, PermisoOrganizacionVer, true},
		{RolMiembro, PermisoOrganizacionEditar, false},
		{RolMiembro, PermisoOrganizacionArchivar, false},
		{RolMiembro, PermisoMiembroVer, true},
		{RolMiembro, PermisoMiembroInvitar, false},
		{RolMiembro, PermisoMiembroCambiarRol, false},
		{RolMiembro, PermisoMiembroRemover, false},
		{RolMiembro, PermisoPropiedadTransferir, false},
	}
	if len(casos) != 24 {
		t.Fatalf("se esperaban 24 combinaciones (3 roles x 8 permisos), hay %d", len(casos))
	}
	for _, c := range casos {
		t.Run(c.rol.Valor()+"/"+c.permiso.Valor(), func(t *testing.T) {
			got := c.rol.TienePermiso(c.permiso)
			if got != c.concede {
				t.Errorf("TienePermiso() = %v, esperado %v", got, c.concede)
			}
		})
	}
}

func TestRol_Permisos_DevuelveCopia(t *testing.T) {
	permisos := RolMiembro.Permisos()
	permisos[0] = PermisoPropiedadTransferir // muta la copia local
	permisosDeNuevo := RolMiembro.Permisos()
	if permisosDeNuevo[0].EsIgual(PermisoPropiedadTransferir) {
		t.Error("Permisos() debe devolver una copia; mutar el slice devuelto no debe afectar llamadas futuras")
	}
}

// TestINV_TEN_20_ReglaDeDominancia_Otorgamiento cubre el primer paso:
// nadie puede otorgar un rol estrictamente superior al propio.
func TestINV_TEN_20_ReglaDeDominancia_Otorgamiento(t *testing.T) {
	casos := []struct {
		ejecutor, nuevo Rol
		esperaError     bool
	}{
		{RolPropietario, RolPropietario, false},
		{RolPropietario, RolAdministrador, false},
		{RolPropietario, RolMiembro, false},
		{RolAdministrador, RolAdministrador, false},
		{RolAdministrador, RolMiembro, false},
		{RolAdministrador, RolPropietario, true}, // no puede promoverse a sí mismo ni a otro
		{RolMiembro, RolMiembro, false},
		{RolMiembro, RolAdministrador, true},
		{RolMiembro, RolPropietario, true},
	}
	for _, c := range casos {
		t.Run(c.ejecutor.Valor()+"->otorga->"+c.nuevo.Valor(), func(t *testing.T) {
			err := ValidarOtorgamiento(c.ejecutor, c.nuevo)
			if c.esperaError {
				if err == nil {
					t.Fatal("se esperaba error")
				}
				var errRolSuperior *ErrRolSuperiorAlPropio
				if !errors.As(err, &errRolSuperior) {
					t.Errorf("se esperaba *ErrRolSuperiorAlPropio, obtuvo %T", err)
				}
				return
			}
			if err != nil {
				t.Errorf("no se esperaba error: %v", err)
			}
		})
	}
}

// TestINV_TEN_20_ReglaDeDominancia_Modificacion cubre el segundo paso:
// nadie puede modificar ni remover una membresía cuyo rol sea estrictamente
// superior al propio. Un administrador puede degradar a otro administrador
// (nivel igual) pero nunca tocar a un propietario; un propietario puede
// remover a otro propietario (nivel igual).
func TestINV_TEN_20_ReglaDeDominancia_Modificacion(t *testing.T) {
	casos := []struct {
		ejecutor, objetivo Rol
		esperaError        bool
	}{
		{RolPropietario, RolPropietario, false},
		{RolPropietario, RolAdministrador, false},
		{RolPropietario, RolMiembro, false},
		{RolAdministrador, RolAdministrador, false},
		{RolAdministrador, RolMiembro, false},
		{RolAdministrador, RolPropietario, true},
		{RolMiembro, RolMiembro, false},
		{RolMiembro, RolAdministrador, true},
		{RolMiembro, RolPropietario, true},
	}
	for _, c := range casos {
		t.Run(c.ejecutor.Valor()+"->actua_sobre->"+c.objetivo.Valor(), func(t *testing.T) {
			err := ValidarDominancia(c.ejecutor, c.objetivo)
			if c.esperaError {
				if err == nil {
					t.Fatal("se esperaba error")
				}
				var errDominante *ErrMembresiaDominante
				if !errors.As(err, &errDominante) {
					t.Errorf("se esperaba *ErrMembresiaDominante, obtuvo %T", err)
				}
				return
			}
			if err != nil {
				t.Errorf("no se esperaba error: %v", err)
			}
		})
	}
}

func TestRol_EsIgualYEsVacio(t *testing.T) {
	var vacio Rol
	if !vacio.EsVacio() {
		t.Error("el zero value debe estar vacío")
	}
	if !RolPropietario.EsIgual(RolPropietario) {
		t.Error("un rol debe ser igual a sí mismo")
	}
	if RolPropietario.EsIgual(RolAdministrador) {
		t.Error("roles distintos no deben ser iguales")
	}
}
