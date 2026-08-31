package dominio

import "testing"

func TestEstadoUsuarioDesde_Valido(t *testing.T) {
	valores := []string{
		"pendiente_verificacion", "activo", "suspendido", "bloqueado", "anonimizado",
	}
	for _, v := range valores {
		e, err := EstadoUsuarioDesde(v)
		if err != nil {
			t.Errorf("EstadoUsuarioDesde(%q) devolvió error inesperado: %v", v, err)
		}
		if e.String() != v {
			t.Errorf("String() = %q, esperado %q", e.String(), v)
		}
	}
}

func TestEstadoUsuarioDesde_Invalido(t *testing.T) {
	if _, err := EstadoUsuarioDesde("no_existe"); err == nil {
		t.Error("se esperaba error con un valor fuera del catálogo cerrado")
	}
}

// TestINV_ID_05_MaquinaEstados verifica exhaustivamente la tabla de
// transiciones de la sección 1.4 del diseño, incluyendo que anonimizado es
// terminal.
func TestINV_ID_05_MaquinaEstados(t *testing.T) {
	todos := []EstadoUsuario{
		EstadoPendienteVerificacion, EstadoActivo, EstadoSuspendido, EstadoBloqueado, EstadoAnonimizado,
	}

	permitidas := map[string]bool{
		"pendiente_verificacion->activo":      true,
		"pendiente_verificacion->bloqueado":   true,
		"activo->suspendido":                  true,
		"activo->bloqueado":                   true,
		"suspendido->activo":                  true,
		"bloqueado->activo":                   true,
		"pendiente_verificacion->anonimizado": true,
		"activo->anonimizado":                 true,
		"suspendido->anonimizado":             true,
		"bloqueado->anonimizado":              true,
	}

	for _, origen := range todos {
		for _, destino := range todos {
			clave := origen.String() + "->" + destino.String()
			esperado := permitidas[clave]
			obtenido := origen.PuedeTransicionarA(destino)
			if obtenido != esperado {
				t.Errorf("PuedeTransicionarA: %s = %v, esperado %v", clave, obtenido, esperado)
			}
		}
	}
}

// TestINV_ID_05_AnonimizadoEsTerminal verifica que ninguna transición sale
// de anonimizado, ni siquiera hacia sí mismo.
func TestINV_ID_05_AnonimizadoEsTerminal(t *testing.T) {
	todos := []EstadoUsuario{
		EstadoPendienteVerificacion, EstadoActivo, EstadoSuspendido, EstadoBloqueado, EstadoAnonimizado,
	}
	for _, destino := range todos {
		if EstadoAnonimizado.PuedeTransicionarA(destino) {
			t.Errorf("anonimizado no debe poder transicionar a %s", destino.String())
		}
	}
}

func TestEstadoUsuario_PuedeTransicionarA_EstadoDesconocido(t *testing.T) {
	var desconocido EstadoUsuario // zero value, fuera del catálogo cerrado
	if desconocido.PuedeTransicionarA(EstadoActivo) {
		t.Error("un EstadoUsuario fuera del catálogo no debe poder transicionar a ningún destino")
	}
}

func TestMotivoCambioEstado_String(t *testing.T) {
	m, _ := NuevoMotivoCambioEstado("motivo de prueba")
	if m.String() != "motivo de prueba" {
		t.Errorf("String() = %q, esperado 'motivo de prueba'", m.String())
	}
}

func TestErroresDeConstruccion_EstadoUsuario(t *testing.T) {
	errEstado := &ErrEstadoUsuarioInvalido{Valor: "x"}
	if errEstado.Error() == "" {
		t.Error("ErrEstadoUsuarioInvalido.Error() no debe estar vacío")
	}
	errMotivo := &ErrMotivoCambioEstadoInvalido{Motivo: "x"}
	if errMotivo.Error() == "" {
		t.Error("ErrMotivoCambioEstadoInvalido.Error() no debe estar vacío")
	}
}

func TestEstadoUsuario_EsIgual(t *testing.T) {
	a, _ := EstadoUsuarioDesde("activo")
	b, _ := EstadoUsuarioDesde("activo")
	c, _ := EstadoUsuarioDesde("suspendido")
	if !a.EsIgual(b) {
		t.Error("dos estados con el mismo valor deben ser iguales")
	}
	if a.EsIgual(c) {
		t.Error("dos estados con distinto valor no deben ser iguales")
	}
}

func TestNuevoMotivoCambioEstado_Valido(t *testing.T) {
	m, err := NuevoMotivoCambioEstado("  incumplimiento de política de uso  ")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if m.Valor() != "incumplimiento de política de uso" {
		t.Errorf("Valor() = %q, no se recortaron los espacios", m.Valor())
	}
}

func TestNuevoMotivoCambioEstado_RechazaVacio(t *testing.T) {
	casos := []string{"", "   "}
	for _, c := range casos {
		if _, err := NuevoMotivoCambioEstado(c); err == nil {
			t.Errorf("NuevoMotivoCambioEstado(%q) debía fallar", c)
		}
	}
}

func TestNuevoMotivoCambioEstado_RechazaExcesivamenteLargo(t *testing.T) {
	largo := make([]rune, 281)
	for i := range largo {
		largo[i] = 'a'
	}
	if _, err := NuevoMotivoCambioEstado(string(largo)); err == nil {
		t.Error("se esperaba error con un motivo de más de 280 caracteres")
	}
}
