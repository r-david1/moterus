package dominio

import "testing"

const uuidV7DePrueba = "018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6d"
const uuidV4DePrueba = "3fa85f64-5717-4562-b3fc-2c963f66afa6"
const uuidNulo = "00000000-0000-0000-0000-000000000000"

func TestIDUsuarioDesde_AceptaUUIDValido(t *testing.T) {
	id, err := IDUsuarioDesde(uuidV7DePrueba)
	if err != nil {
		t.Fatalf("no se esperaba error, obtuvo %v", err)
	}
	if id.EsVacio() {
		t.Error("EsVacio() = true para un UUID válido")
	}
	if id.String() != uuidV7DePrueba {
		t.Errorf("String() = %q, esperado %q", id.String(), uuidV7DePrueba)
	}
}

func TestIDUsuarioDesde_RechazaFormatoInvalido(t *testing.T) {
	casos := []string{
		"",
		"no-es-un-uuid",
		"018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6", // longitud 35, un carácter corto
		uuidNulo,
		"018e6f2a=9c3d-7c3a-8b3a-1e2f3a4b5c6d", // guion faltante en la posición 8
		"018e6f2g-9c3d-7c3a-8b3a-1e2f3a4b5c6d", // carácter 'g' no hexadecimal
	}
	for _, c := range casos {
		if _, err := IDUsuarioDesde(c); err == nil {
			t.Errorf("IDUsuarioDesde(%q): se esperaba error", c)
		}
	}
}

func TestIDUsuario_EsIgual(t *testing.T) {
	a, _ := IDUsuarioDesde(uuidV7DePrueba)
	b, _ := IDUsuarioDesde(uuidV7DePrueba)
	c, _ := IDUsuarioDesde(uuidV4DePrueba)
	if !a.EsIgual(b) {
		t.Error("dos IDUsuario construidos desde el mismo valor deben ser iguales")
	}
	if a.EsIgual(c) {
		t.Error("dos IDUsuario de valores distintos no deben ser iguales")
	}
}

func TestIDUsuarioDesde_NormalizaAMinusculas(t *testing.T) {
	id, err := IDUsuarioDesde("018E6F2A-9C3D-7C3A-8B3A-1E2F3A4B5C6D")
	if err != nil {
		t.Fatalf("no se esperaba error, obtuvo %v", err)
	}
	if id.String() != uuidV7DePrueba {
		t.Errorf("String() = %q, esperado en minúsculas %q", id.String(), uuidV7DePrueba)
	}
}

func TestIDSesionDesde_AceptaUUIDValido(t *testing.T) {
	id, err := IDSesionDesde(uuidV7DePrueba)
	if err != nil {
		t.Fatalf("no se esperaba error, obtuvo %v", err)
	}
	if id.EsVacio() {
		t.Error("EsVacio() = true para un UUID válido")
	}
}

func TestIDSesionDesde_RechazaFormatoInvalido(t *testing.T) {
	casos := []string{"", "no-es-un-uuid", uuidNulo}
	for _, c := range casos {
		if _, err := IDSesionDesde(c); err == nil {
			t.Errorf("IDSesionDesde(%q): se esperaba error", c)
		}
	}
}

func TestIDSesion_EsIgual(t *testing.T) {
	a, _ := IDSesionDesde(uuidV7DePrueba)
	b, _ := IDSesionDesde(uuidV7DePrueba)
	if !a.EsIgual(b) {
		t.Error("dos IDSesion construidos desde el mismo valor deben ser iguales")
	}
}

func TestIDTokenAccesoDesde_AceptaUUIDValido(t *testing.T) {
	id, err := IDTokenAccesoDesde(uuidV4DePrueba)
	if err != nil {
		t.Fatalf("no se esperaba error, obtuvo %v", err)
	}
	if id.EsVacio() {
		t.Error("EsVacio() = true para un UUID válido")
	}
}

func TestIDTokenAccesoDesde_RechazaFormatoInvalido(t *testing.T) {
	casos := []string{"", "no-es-un-uuid", uuidNulo}
	for _, c := range casos {
		if _, err := IDTokenAccesoDesde(c); err == nil {
			t.Errorf("IDTokenAccesoDesde(%q): se esperaba error", c)
		}
	}
}

func idUsuarioDePrueba(t *testing.T) IDUsuario {
	t.Helper()
	id, err := IDUsuarioDesde(uuidV7DePrueba)
	if err != nil {
		t.Fatalf("no se pudo construir el IDUsuario de prueba: %v", err)
	}
	return id
}

func idSesionDePrueba(t *testing.T) IDSesion {
	t.Helper()
	id, err := IDSesionDesde("018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6e")
	if err != nil {
		t.Fatalf("no se pudo construir el IDSesion de prueba: %v", err)
	}
	return id
}

func idTokenAccesoDePrueba(t *testing.T) IDTokenAcceso {
	t.Helper()
	id, err := IDTokenAccesoDesde(uuidV4DePrueba)
	if err != nil {
		t.Fatalf("no se pudo construir el IDTokenAcceso de prueba: %v", err)
	}
	return id
}
