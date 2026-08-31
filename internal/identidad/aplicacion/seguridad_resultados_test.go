package aplicacion_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/r-david1/moterus/internal/identidad/puertos"
)

// TestResultados_NoExponenCredenciales es una salvaguarda estructural
// (INV-ID-04, INV-ID-21): ninguna estructura de resultado/vista devuelta
// por los puertos de entrada de Identidad debe tener un campo cuyo nombre
// sugiera que transporta la contraseña en claro, su hash, o el token de
// verificación de correo (INV-ID-21: RegistrarUsuario nunca debe exponer el
// token en su tipo de retorno). Recorre por reflexión los tipos exportados
// en lugar de fijar una lista de campos permitidos, para que la prueba
// falle también si alguien añade un campo nuevo mal nombrado en el futuro.
func TestResultados_NoExponenCredenciales(t *testing.T) {
	tipos := []struct {
		nombre string
		valor  any
	}{
		{"ResultadoRegistro", puertos.ResultadoRegistro{}},
		{"ResultadoAutenticacion", puertos.ResultadoAutenticacion{}},
		{"VistaUsuario", puertos.VistaUsuario{}},
	}

	fragmentosProhibidos := []string{"contrasena", "password", "hash", "secreto", "secret", "token"}

	for _, tc := range tipos {
		tipo := reflect.TypeOf(tc.valor)
		for i := 0; i < tipo.NumField(); i++ {
			campo := tipo.Field(i)
			nombreMin := strings.ToLower(campo.Name)
			for _, fragmento := range fragmentosProhibidos {
				if strings.Contains(nombreMin, fragmento) {
					t.Errorf("%s.%s: el nombre del campo sugiere que transporta una credencial (INV-ID-04); no debe existir en un resultado/vista", tc.nombre, campo.Name)
				}
			}
		}
	}
}
