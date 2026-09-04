package dominio

import (
	"errors"
	"testing"
)

func TestPermisoDesde(t *testing.T) {
	catalogo := []string{
		"organizacion.ver",
		"organizacion.editar",
		"organizacion.archivar",
		"miembro.ver",
		"miembro.invitar",
		"miembro.cambiar_rol",
		"miembro.remover",
		"propiedad.transferir",
	}
	if len(catalogo) != 8 {
		t.Fatalf("se esperaban 8 permisos en el catálogo cerrado, hay %d en el test", len(catalogo))
	}
	for _, v := range catalogo {
		p, err := PermisoDesde(v)
		if err != nil {
			t.Errorf("%q debería ser válido: %v", v, err)
		}
		if p.Valor() != v {
			t.Errorf("Valor() = %q, esperado %q", p.Valor(), v)
		}
	}
	if _, err := PermisoDesde("permiso.inexistente"); err == nil {
		t.Fatal("se esperaba error para un permiso fuera del catálogo")
	} else {
		var errPermiso *ErrPermisoDesconocido
		if !errors.As(err, &errPermiso) {
			t.Errorf("se esperaba *ErrPermisoDesconocido, obtuvo %T", err)
		}
	}
}

func TestPermiso_EsIgualYEsVacio(t *testing.T) {
	var vacio Permiso
	if !vacio.EsVacio() {
		t.Error("el zero value debe estar vacío")
	}
	if !PermisoOrganizacionVer.EsIgual(PermisoOrganizacionVer) {
		t.Error("un permiso debe ser igual a sí mismo")
	}
	if PermisoOrganizacionVer.EsIgual(PermisoMiembroVer) {
		t.Error("permisos distintos no deben ser iguales")
	}
}
