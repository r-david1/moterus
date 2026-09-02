package dominio

import "testing"

// TestINV_ACC_08_CatalogoCerradoDeMotivos verifica que MotivoRevocacion solo
// admite los siete valores del catálogo cerrado de la tabla 1.3 del diseño;
// nada de texto libre.
func TestINV_ACC_08_CatalogoCerradoDeMotivos(t *testing.T) {
	validos := []string{
		"cierre_usuario",
		"cierre_masivo_usuario",
		"reuso_refresco_detectado",
		"cuenta_no_operativa",
		"contrasena_cambiada",
		"limite_sesiones_excedido",
		"revocacion_administrativa",
	}
	for _, v := range validos {
		if _, err := MotivoRevocacionDesde(v); err != nil {
			t.Errorf("MotivoRevocacionDesde(%q): no se esperaba error, obtuvo %v", v, err)
		}
	}
	invalidos := []string{"", "otro_motivo", "el usuario decidió cerrar sesión"}
	for _, v := range invalidos {
		if _, err := MotivoRevocacionDesde(v); err == nil {
			t.Errorf("MotivoRevocacionDesde(%q): se esperaba error (catálogo cerrado)", v)
		}
	}
}

func TestMotivoRevocacion_EsVacio(t *testing.T) {
	var vacio MotivoRevocacion
	if !vacio.EsVacio() {
		t.Error("el zero value de MotivoRevocacion debe reportarse vacío")
	}
	if MotivoCierreUsuario.EsVacio() {
		t.Error("MotivoCierreUsuario no debe reportarse vacío")
	}
}

func TestMotivoRevocacion_EsIniciativaPropia(t *testing.T) {
	propios := []MotivoRevocacion{MotivoCierreUsuario, MotivoCierreMasivoUsuario}
	for _, m := range propios {
		if !m.EsIniciativaPropia() {
			t.Errorf("%s debe reportarse como iniciativa propia del usuario", m)
		}
	}
	ajenos := []MotivoRevocacion{
		MotivoReusoRefrescoDetectado,
		MotivoCuentaNoOperativa,
		MotivoContrasenaCambiada,
		MotivoLimiteSesionesExcedido,
		MotivoRevocacionAdministrativa,
	}
	for _, m := range ajenos {
		if m.EsIniciativaPropia() {
			t.Errorf("%s no debe reportarse como iniciativa propia del usuario", m)
		}
	}
}
