package dominio

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// TestINV_ACC_18_SoloImportaStdlib recorre cada archivo .go del paquete
// (excepto los propios *_test.go, que sí pueden usar utilidades de test de
// la stdlib como reflect/testing sin que eso diga nada sobre el paquete en
// producción) y verifica que ningún import pertenece a un módulo de
// terceros ni a otro paquete del proyecto — en particular, nunca
// identidad/dominio ni ninguna biblioteca de JWT (INV-ACC-18). Se detecta
// heurísticamente: un import path de la stdlib de Go nunca contiene un
// punto en su primer segmento (p. ej. "crypto/sha256"), mientras que los
// módulos de terceros y del propio proyecto sí (p. ej.
// "github.com/r-david1/moterus/...", "golang.org/x/...").
func TestINV_ACC_18_SoloImportaStdlib(t *testing.T) {
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
			if strings.Contains(primerSegmento, ".") {
				t.Errorf("%s importa %q: acceso/dominio no puede depender de módulos externos ni de otros paquetes del proyecto (INV-ACC-18)", archivo, ruta)
			}
		}
	}
	if !huboArchivoDeProduccion {
		t.Fatal("no se encontró ningún archivo de producción (*.go que no sea _test.go)")
	}
}
