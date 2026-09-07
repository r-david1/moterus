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

// TestSoloElACLDeTenenciaImportaTenenciaEnConfianza verifica que, dentro de
// internal/confianza, el ÚNICO paquete autorizado a importar algo de
// internal/tenencia es confianza/adaptadores/tenencia (el ACL, §2.2 del
// diseño docs/design/colas-virtuales.md: "único paquete de Confianza
// autorizado a importar tenencia/puertos", mismo criterio de frontera que
// INV-TEN-28/INV-ACC-19).
func TestSoloElACLDeTenenciaImportaTenenciaEnConfianza(t *testing.T) {
	verificarUnicoImportadorPermitido(t,
		repoRelativo(t, "internal/confianza"),
		raizModulo+"/internal/tenencia",
		raizModulo+"/internal/confianza/adaptadores/tenencia",
	)
}

// TestSoloElAdaptadorHTTPImportaAccesoEnConfianza verifica que, dentro de
// internal/confianza, el ÚNICO paquete autorizado a importar algo de
// internal/acceso es confianza/adaptadores/http (el middleware de
// autenticación Bearer de los endpoints de administración, §7.2 del diseño
// docs/design/colas-virtuales.md — mismo criterio que
// tenencia/adaptadores/http/middleware_autenticacion.go frente a
// internal/acceso).
func TestSoloElAdaptadorHTTPImportaAccesoEnConfianza(t *testing.T) {
	verificarUnicoImportadorPermitido(t,
		repoRelativo(t, "internal/confianza"),
		raizModulo+"/internal/acceso",
		raizModulo+"/internal/confianza/adaptadores/http",
	)
}

// verificarUnicoImportadorPermitido recorre raizContexto y falla si algún
// paquete que no sea paqueteImportadorPermitido importa un paquete cuyo
// import path empieza con prefijoImportadoProhibido.
func verificarUnicoImportadorPermitido(t *testing.T, raizContexto, prefijoImportadoProhibido, paqueteImportadorPermitido string) {
	t.Helper()

	fset := token.NewFileSet()
	err := filepath.WalkDir(raizContexto, func(ruta string, entrada fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entrada.IsDir() {
			return nil
		}
		if !strings.HasSuffix(ruta, ".go") {
			return nil
		}

		paqueteDeEsteArchivo := paqueteGoDesdeRuta(ruta, raizContexto)
		if paqueteDeEsteArchivo == paqueteImportadorPermitido {
			return nil
		}

		f, errParse := parser.ParseFile(fset, ruta, nil, parser.ImportsOnly)
		if errParse != nil {
			return errParse
		}
		for _, imp := range f.Imports {
			valor := strings.Trim(imp.Path.Value, `"`)
			if strings.HasPrefix(valor, prefijoImportadoProhibido) {
				t.Errorf("%s importa %q: solo %s puede importar algo de %s",
					ruta, valor, paqueteImportadorPermitido, prefijoImportadoProhibido)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("recorriendo %s: %v", raizContexto, err)
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
