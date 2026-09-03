// Package arquitectura contiene tests que verifican reglas de frontera
// entre bounded contexts que no pueden expresarse como un test unitario
// dentro de un solo paquete (a diferencia de, por ejemplo,
// internal/acceso/dominio/arquitectura_test.go, que sí puede verificar
// INV-ACC-18 mirando solo sus propios imports).
package arquitectura

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// raizModulo es la ruta del módulo Go de este proyecto (go.mod), usada
// para reconocer imports internos frente a la stdlib/terceros.
const raizModulo = "github.com/r-david1/moterus"

// TestINV_ACC_19_SoloElACLDeIdentidadImportaIdentidad verifica que, dentro
// de internal/acceso, el ÚNICO paquete autorizado a importar algo de
// internal/identidad es acceso/adaptadores/identidad (el ACL, §1.7 y
// §2.2 del diseño del contexto Acceso: "el único paquete de Acceso que
// importa algo de Identidad"). En particular, cubre INV-ACC-19
// ("acceso/aplicacion no importa ningún paquete de Identidad").
func TestINV_ACC_19_SoloElACLDeIdentidadImportaIdentidad(t *testing.T) {
	raizAcceso := repoRelativo(t, "internal/acceso")
	paqueteACLPermitido := raizModulo + "/internal/acceso/adaptadores/identidad"

	fset := token.NewFileSet()
	err := filepath.WalkDir(raizAcceso, func(ruta string, entrada fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entrada.IsDir() {
			return nil
		}
		if !strings.HasSuffix(ruta, ".go") {
			return nil
		}

		paqueteDeEsteArchivo := paqueteGoDesdeRuta(ruta, raizAcceso)
		if paqueteDeEsteArchivo == paqueteACLPermitido {
			return nil // el ACL sí puede importar identidad/*.
		}

		f, errParse := parser.ParseFile(fset, ruta, nil, parser.ImportsOnly)
		if errParse != nil {
			return errParse
		}
		for _, imp := range f.Imports {
			valor := strings.Trim(imp.Path.Value, `"`)
			if strings.HasPrefix(valor, raizModulo+"/internal/identidad") {
				t.Errorf("%s importa %q: solo %s puede importar algo de internal/identidad (INV-ACC-19)",
					ruta, valor, paqueteACLPermitido)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("recorriendo internal/acceso: %v", err)
	}
}

// paqueteGoDesdeRuta reconstruye el import path del paquete Go al que
// pertenece un archivo .go dado (asumiendo la disposición estándar de
// go.mod: internal/acceso/... -> github.com/r-david1/moterus/internal/acceso/...).
func paqueteGoDesdeRuta(rutaArchivo, raizAcceso string) string {
	dir := filepath.Dir(rutaArchivo)
	rel, err := filepath.Rel(repoRaiz(raizAcceso), dir)
	if err != nil {
		return ""
	}
	return raizModulo + "/" + filepath.ToSlash(rel)
}

// repoRaiz sube desde internal/acceso hasta la raíz del repo (dos niveles).
func repoRaiz(raizAcceso string) string {
	return filepath.Dir(filepath.Dir(raizAcceso))
}

// repoRelativo resuelve una ruta relativa a la raíz del repo, asumiendo
// que este test corre desde test/arquitectura (go test siempre fija el
// working directory al del paquete del test).
func repoRelativo(t *testing.T, sub string) string {
	t.Helper()
	return filepath.Join("..", "..", sub)
}
