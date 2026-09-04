package dominio

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// importsExternosPermitidos es la lista cerrada de imports fuera de la
// stdlib de Go que tenencia/dominio puede usar. Es una única excepción
// documentada y explícita a INV-TEN-27 ("cero dependencias externas en el
// dominio"), la misma que ya usa identidad/dominio.Correo: x/text es
// mantenida por el propio equipo de Go y la normalización NFC (alias,
// nombre de organización, correo destinatario) no puede implementarse
// correctamente con unicode.ToLower de la stdlib. Cualquier otro import con
// un punto en su primer segmento — en particular
// "github.com/r-david1/moterus/internal/identidad/..." y
// "github.com/r-david1/moterus/internal/acceso/..." — está prohibido.
var importsExternosPermitidos = map[string]bool{
	"golang.org/x/text/unicode/norm": true,
}

// TestINV_TEN_27_SoloImportaStdlibYExcepcionesDocumentadas recorre cada
// archivo .go del paquete (excepto los propios *_test.go, que sí pueden
// usar utilidades de test de la stdlib sin que eso diga nada sobre el
// paquete en producción) y verifica que ningún import pertenece a un
// módulo de terceros ni a otro paquete del proyecto salvo la excepción
// documentada de importsExternosPermitidos — en particular, nunca
// identidad/dominio, acceso/dominio, ni ninguna otra dependencia externa
// (INV-TEN-27). Se detecta heurísticamente: un import path de la stdlib de
// Go nunca contiene un punto en su primer segmento (p. ej.
// "crypto/sha256"), mientras que los módulos de terceros y del propio
// proyecto sí (p. ej. "github.com/r-david1/moterus/...",
// "golang.org/x/...").
func TestINV_TEN_27_SoloImportaStdlibYExcepcionesDocumentadas(t *testing.T) {
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
			t.Errorf("%s importa %q: tenencia/dominio no puede depender de módulos externos, ni de identidad/dominio, ni de acceso/dominio, salvo la excepción documentada de norm NFC (INV-TEN-27)", archivo, ruta)
		}
	}
	if !huboArchivoDeProduccion {
		t.Fatal("no se encontró ningún archivo de producción (*.go que no sea _test.go)")
	}
}
