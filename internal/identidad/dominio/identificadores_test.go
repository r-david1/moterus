package dominio

import "testing"

func TestIDUsuarioDesde_Valido(t *testing.T) {
	casos := []string{
		"018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6d",
		"018E6F2A-9C3D-7C3A-8B3A-1E2F3A4B5C6D",
	}
	for _, c := range casos {
		id, err := IDUsuarioDesde(c)
		if err != nil {
			t.Errorf("IDUsuarioDesde(%q) devolvió error inesperado: %v", c, err)
			continue
		}
		if id.EsVacio() {
			t.Errorf("IDUsuarioDesde(%q) no debería resultar vacío", c)
		}
	}
}

func TestIDUsuarioDesde_Invalido(t *testing.T) {
	casos := []string{
		"",
		"no-es-un-uuid",
		"018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6",   // corto
		"018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6dd", // largo
		"018e6f2a9c3d7c3a8b3a1e2f3a4b5c6d",      // sin guiones
		"gggggggg-9c3d-7c3a-8b3a-1e2f3a4b5c6d",  // no hex
		"018e6f2a09c3d-7c3a-8b3a-1e2f3a4b5c6d",  // sin guion en la posición 8, 36 caracteres
	}
	for _, c := range casos {
		if _, err := IDUsuarioDesde(c); err == nil {
			t.Errorf("IDUsuarioDesde(%q) debía fallar", c)
		}
	}
}

func TestIDUsuarioDesde_RechazaUUIDNulo(t *testing.T) {
	if _, err := IDUsuarioDesde("00000000-0000-0000-0000-000000000000"); err == nil {
		t.Error("se esperaba error con el UUID nulo")
	}
}

func TestIDUsuario_EsIgual(t *testing.T) {
	a, _ := IDUsuarioDesde("018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6d")
	b, _ := IDUsuarioDesde("018E6F2A-9C3D-7C3A-8B3A-1E2F3A4B5C6D")
	c, _ := IDUsuarioDesde("118e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6d")
	if !a.EsIgual(b) {
		t.Error("dos IDUsuario con el mismo valor (case-insensitive) deben ser iguales")
	}
	if a.EsIgual(c) {
		t.Error("dos IDUsuario distintos no deben ser iguales")
	}
}

func TestIDUsuario_EsVacio(t *testing.T) {
	var id IDUsuario
	if !id.EsVacio() {
		t.Error("el zero value de IDUsuario debe reportarse como vacío")
	}
}

func TestErrIDUsuarioInvalido_Error(t *testing.T) {
	err := &ErrIDUsuarioInvalido{Motivo: "x"}
	if err.Error() == "" {
		t.Error("ErrIDUsuarioInvalido.Error() no debe estar vacío")
	}
}

func TestIDFactorMFADesde_Valido(t *testing.T) {
	casos := []string{
		"018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6d",
		"018E6F2A-9C3D-7C3A-8B3A-1E2F3A4B5C6D",
	}
	for _, c := range casos {
		id, err := IDFactorMFADesde(c)
		if err != nil {
			t.Errorf("IDFactorMFADesde(%q) devolvió error inesperado: %v", c, err)
			continue
		}
		if id.EsVacio() {
			t.Errorf("IDFactorMFADesde(%q) no debería resultar vacío", c)
		}
	}
}

func TestIDFactorMFADesde_Invalido(t *testing.T) {
	casos := []string{
		"",
		"no-es-un-uuid",
		"018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6",
		"018e6f2a9c3d7c3a8b3a1e2f3a4b5c6d",
	}
	for _, c := range casos {
		if _, err := IDFactorMFADesde(c); err == nil {
			t.Errorf("IDFactorMFADesde(%q) debía fallar", c)
		}
	}
}

func TestIDFactorMFADesde_RechazaUUIDNulo(t *testing.T) {
	if _, err := IDFactorMFADesde("00000000-0000-0000-0000-000000000000"); err == nil {
		t.Error("se esperaba error con el UUID nulo")
	}
}

func TestIDFactorMFA_EsIgual(t *testing.T) {
	a, _ := IDFactorMFADesde("018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6d")
	b, _ := IDFactorMFADesde("018E6F2A-9C3D-7C3A-8B3A-1E2F3A4B5C6D")
	c, _ := IDFactorMFADesde("118e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6d")
	if !a.EsIgual(b) {
		t.Error("dos IDFactorMFA con el mismo valor (case-insensitive) deben ser iguales")
	}
	if a.EsIgual(c) {
		t.Error("dos IDFactorMFA distintos no deben ser iguales")
	}
}

func TestIDFactorMFA_EsVacio(t *testing.T) {
	var id IDFactorMFA
	if !id.EsVacio() {
		t.Error("el zero value de IDFactorMFA debe reportarse como vacío")
	}
}

func TestErrIDFactorMFAInvalido_Error(t *testing.T) {
	err := &ErrIDFactorMFAInvalido{Motivo: "x"}
	if err.Error() == "" {
		t.Error("ErrIDFactorMFAInvalido.Error() no debe estar vacío")
	}
}
