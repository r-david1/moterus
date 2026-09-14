package aplicacion

// Test de arquitectura para INV-BLQ-02 (docs/design/bloqueo-cuenta.md §7):
// ningún caso de uso invoca Usuario.Bloquear. EstadoBloqueado existe en el
// dominio (usuario.go) y su transición está probada a nivel de dominio
// (usuario_test.go), pero está reservado para una decisión humana explícita
// —hoy sin ningún camino de invocación real, porque no existe rol de
// administrador de plataforma (ver docs/adr/0011-sin-bloqueo-automatico-de-cuenta.md
// y ADR 0045)— y nunca para un caso de uso automático. Blanco (package
// aplicacion, no aplicacion_test) con go/ast, mismo patrón que
// confianza/aplicacion/arquitectura_test.go y acceso/dominio/arquitectura_test.go.
//
// Es un test de custodia, no un test de comportamiento: no protege código
// que existe hoy, protege contra que alguien agregue mañana un caso de uso
// que llame u.Bloquear(...) sin pasar primero por el análisis de
// docs/design/bloqueo-cuenta.md (§10 documenta las 5 precondiciones bajo las
// que valdría la pena reabrir esa decisión).

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

func TestINV_BLQ_02_NingunCasoDeUsoInvocaUsuarioBloquear(t *testing.T) {
	entradas, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("no se pudo listar el directorio: %v", err)
	}

	fset := token.NewFileSet()
	for _, entrada := range entradas {
		nombre := entrada.Name()
		if entrada.IsDir() || !strings.HasSuffix(nombre, ".go") || strings.HasSuffix(nombre, "_test.go") {
			continue
		}

		f, err := parser.ParseFile(fset, nombre, nil, 0)
		if err != nil {
			t.Fatalf("no se pudo parsear %s: %v", nombre, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if sel.Sel.Name == "Bloquear" {
				pos := fset.Position(sel.Pos())
				t.Errorf("%s:%d invoca .Bloquear(): ningún caso de uso puede llamar a Usuario.Bloquear directamente — "+
					"es una transición reservada a decisión humana explícita, no a lógica automática (INV-BLQ-02, docs/design/bloqueo-cuenta.md)",
					nombre, pos.Line)
			}
			return true
		})
	}
}
