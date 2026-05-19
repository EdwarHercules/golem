package executor

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
)

var dangerousPatterns = map[string]string{

	"os.Remove":    "eliminacion de arcvhivos del sistema",
	"os.RemoveAll": "eliminacion recursiva del directorio",
	"os.WriteFile": "escrituria arbitraria en el sistema de archivos",
	"os.MkdirAll":  "creacion de directorios arbitrarios",
	"os.Rename":    "movimiento/renombrado de archivos",

	"exec.Command":        "ejecucion de subrocesos del sistema",
	"exec.CommandContext": "ejecucion de subprocesos del sistema",

	"http.Get":  "peticiones HTTP arbitrarias",
	"http.Post": "peticiones HTTP arbitrarias",

	"os.Getenv":  "lectura de variables de entorno",
	"os.Setenv":  "modificacion de variables de entorno",
	"os.Environ": "lectura de todas las variables de entorno",

	"os.Exit": "terminacion forzada del proceso GOLEM",
}

func ValidateCode(code string) error {
	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, "", code, 0)
	if err != nil {
		return fmt.Errorf("codigo Go inavilido: %w", err)
	}

	var violation error
	ast.Inspect(file, func(n ast.Node) bool {
		if violation != nil {
			return false
		}

		callExpr, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		selectorExpr, ok := callExpr.Fun.(*ast.SelectorExpr)
		if !ok {
			return true // llamada sin selector (ej: miFuncion()), no es peligrosa aquí
		}

		pkgIdent, ok := selectorExpr.X.(*ast.Ident)
		if !ok {
			return true
		}

		pattern := pkgIdent.Name + "." + selectorExpr.Sel.Name
		if risk, found := dangerousPatterns[pattern]; found {
			violation = fmt.Errorf(
				"codigo rechazado por sguridad: uso de %s (%s) no permitido en el sandbox",
				pattern, risk,
			)
			return false
		}

		return true
	})

	return violation

}
