package dominio

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// importsExternosPermitidos es la lista cerrada de imports fuera de la
// stdlib de Go que confianza/dominio puede usar. Es una única excepción
// documentada, la misma que ya usan identidad/dominio.Correo y
// tenencia/dominio.AliasOrganizacion: x/text es mantenida por el propio
// equipo de Go y la normalización NFC de AliasSala no puede implementarse
// correctamente con unicode.ToLower de la stdlib. Cualquier otro import con
// un punto en su primer segmento — en particular
// "github.com/r-david1/moterus/internal/identidad/...",
// ".../acceso/...", ".../tenencia/..." y cualquier cliente de Redis o
// Postgres — está prohibido.
var importsExternosPermitidos = map[string]bool{
	"golang.org/x/text/unicode/norm": true,
}

// TestDominioConfianza_SoloImportaStdlibYExcepcionesDocumentadas recorre
// cada archivo .go del paquete (excepto los propios *_test.go) y verifica
// que ningún import pertenece a un módulo de terceros ni a otro paquete del
// proyecto salvo la excepción documentada — en particular, nunca el
// dominio de otro contexto, ni pgx, ni un cliente Redis: el dominio es
// puro. Se detecta heurísticamente: un import path de la stdlib de Go nunca
// contiene un punto en su primer segmento.
func TestDominioConfianza_SoloImportaStdlibYExcepcionesDocumentadas(t *testing.T) {
	archivos, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("no se pudo listar el directorio: %v", err)
	}
	if len(archivos) == 0 {
		t.Fatal("no se encontró ningún archivo .go en el paquete")
	}

	fset := token.NewFileSet()
	huboArchivoDeProduccion := false
	for _, archivo := range archivos {
		if strings.HasSuffix(archivo, "_test.go") {
			continue
		}
		huboArchivoDeProduccion = true
		f, err := parser.ParseFile(fset, archivo, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("no se pudo parsear %s: %v", archivo, err)
		}
		for _, imp := range f.Imports {
			ruta := strings.Trim(imp.Path.Value, `"`)
			primerSegmento := strings.SplitN(ruta, "/", 2)[0]
			if !strings.Contains(primerSegmento, ".") {
				continue // stdlib
			}
			if importsExternosPermitidos[ruta] {
				continue // excepción documentada (norm NFC)
			}
			t.Errorf("%s importa %q: confianza/dominio no puede depender de módulos externos ni del dominio de otro contexto, salvo la excepción documentada de norm NFC", archivo, ruta)
		}
	}
	if !huboArchivoDeProduccion {
		t.Fatal("no se encontró ningún archivo de producción (*.go que no sea _test.go)")
	}
}

// TestDominioConfianza_NuncaLlamaATimeNow verifica que ningún archivo de
// producción del paquete invoca time.Now(): el reloj siempre entra como
// parámetro "ahora" (mismo criterio documentado en el diseño §"reglas de
// oro" y ya aplicado en identidad/dominio, acceso/dominio y
// tenencia/dominio), condición necesaria para que CursorEn/TurnoDe sean
// funciones puras testeables sin esperar al reloj real.
func TestDominioConfianza_NuncaLlamaATimeNow(t *testing.T) {
	archivos, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("no se pudo listar el directorio: %v", err)
	}
	fset := token.NewFileSet()
	for _, archivo := range archivos {
		if strings.HasSuffix(archivo, "_test.go") {
			continue
		}
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
				t.Errorf("%s:%d llama a time.Now(): el dominio nunca debe leer el reloj real, siempre lo recibe como parámetro", archivo, pos.Line)
			}
			return true
		})
	}
}
