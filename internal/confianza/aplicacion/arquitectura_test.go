package aplicacion

// Tests de arquitectura para la extensión de colas de acceso virtual
// (docs/design/colas-virtuales.md). White-box (package aplicacion, no
// aplicacion_test): recorren el código fuente del propio paquete con
// go/ast, mismo patrón que confianza/dominio/arquitectura_test.go.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// TestPorteroDeSala_NuncaMencionaRepositorioSalasDeEspera verifica
// INV-COLA-08 (el camino caliente nunca toca Postgres) de forma estática:
// portero_sala.go —el único archivo que implementa puertos.PorteroDeSala—
// no puede ni siquiera nombrar el tipo puertos.RepositorioSalasDeEspera, así
// que Ingresar/ConsultarTurno/Reclamar/SalaVigentePara no tienen forma de
// invocarlo. Es una garantía más fuerte que un mock que panickea si se
// llama: ni siquiera compilaría el intento de usarlo.
func TestPorteroDeSala_NuncaMencionaRepositorioSalasDeEspera(t *testing.T) {
	const archivo = "portero_sala.go"
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, archivo, nil, 0)
	if err != nil {
		t.Fatalf("no se pudo parsear %s: %v", archivo, err)
	}
	ast.Inspect(f, func(n ast.Node) bool {
		ident, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		if ident.Name == "RepositorioSalasDeEspera" {
			pos := fset.Position(ident.Pos())
			t.Errorf("%s:%d menciona RepositorioSalasDeEspera: el camino caliente (PorteroDeSala) no puede depender del puerto que habla con Postgres (INV-COLA-08)", archivo, pos.Line)
		}
		return true
	})
}

// archivosDeProduccionDeColasVirtuales enumera los archivos de este cambio
// aditivo (no incluye evaluar_trust_signal.go/comandos.go, preexistentes,
// ni los *_test.go).
var archivosDeProduccionDeColasVirtuales = []string{
	"soporte.go",
	"abrir_sala.go",
	"cambiar_ritmo_admision.go",
	"cambiar_estado_sala.go",
	"portero_sala.go",
	"reconciliar_salas.go",
}

// TestColasVirtuales_NuncaLlamaATimeNow verifica que ningún archivo nuevo de
// este cambio invoca time.Now(): el reloj siempre entra por el puerto
// puertos.Reloj (mismo criterio que confianza/dominio y el resto de
// aplicacion en el repositorio).
func TestColasVirtuales_NuncaLlamaATimeNow(t *testing.T) {
	fset := token.NewFileSet()
	for _, archivo := range archivosDeProduccionDeColasVirtuales {
		f, err := parser.ParseFile(fset, archivo, nil, 0)
		if err != nil {
			t.Fatalf("no se pudo parsear %s: %v", archivo, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkgIdent, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			if pkgIdent.Name == "time" && sel.Sel.Name == "Now" {
				pos := fset.Position(call.Pos())
				t.Errorf("%s:%d llama a time.Now(): el reloj siempre debe entrar por puertos.Reloj", archivo, pos.Line)
			}
			return true
		})
	}
}

// TestColasVirtuales_NuncaImportaAdaptadores verifica que ningún archivo
// nuevo de este cambio importa un paquete de adaptadores (de Confianza ni
// de ningún otro contexto): aplicacion solo conoce dominio y puertos.
func TestColasVirtuales_NuncaImportaAdaptadores(t *testing.T) {
	fset := token.NewFileSet()
	for _, archivo := range archivosDeProduccionDeColasVirtuales {
		f, err := parser.ParseFile(fset, archivo, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("no se pudo parsear %s: %v", archivo, err)
		}
		for _, imp := range f.Imports {
			ruta := strings.Trim(imp.Path.Value, `"`)
			if strings.Contains(ruta, "/adaptadores/") {
				t.Errorf("%s importa %q: aplicacion nunca debe importar un paquete de adaptadores", archivo, ruta)
			}
		}
	}
}
