package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"go/types"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/packages"
)

func main() {
	flag.Parse()
	dir := flag.Arg(0)
	if dir == "" {
		dir = "."
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		log.Fatalf("failed to get absolute path for %s: %v", dir, err)
	}

	cfg := &packages.Config{
		Fset:  token.NewFileSet(),
		Dir:   absDir,
		Mode:  packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo,
		Tests: false,
	}

	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		log.Fatalf("failed to load packages: %v", err)
	}

	for _, pkg := range pkgs {
		for i, fileAst := range pkg.Syntax {
			file := pkg.GoFiles[i]
			if !strings.HasPrefix(file, absDir) {
				continue
			}
			err := processFile(pkg, file, fileAst, cfg.Fset)
			if err != nil {
				log.Printf("failed to process file %s: %v", file, err)
			}
		}
	}
}

func processFile(pkg *packages.Package, filePath string, node *ast.File, fset *token.FileSet) error {
	info := pkg.TypesInfo

	type modification struct {
		pos     token.Pos
		isDemotion bool
		assign  *ast.AssignStmt
		errIdx  int
	}
	var mods []modification

	// PASS 1 (pre-computation): Collect all potential modifications
	astutil.Apply(node, func(cursor *astutil.Cursor) bool {
		stmt, ok := cursor.Node().(ast.Stmt)
		if !ok { return true }
		assign, ok := stmt.(*ast.AssignStmt)
		if !ok { return true }
		if assign.Tok != token.DEFINE && assign.Tok != token.ASSIGN { return true }

		// Check for ignored errors
		if idx := getErrorIndex(assign, info); idx != -1 && isIgnored(assign.Lhs[idx]) {
			mods = append(mods, modification{pos: assign.Pos(), assign: assign, errIdx: idx})
			return true 
		}
		
		// Check for `err :=` that might need demotion
		if assign.Tok == token.DEFINE && len(assign.Lhs) == 1 {
			if id, ok := assign.Lhs[0].(*ast.Ident); ok && id.Name == "err" {
				mods = append(mods, modification{pos: assign.Pos(), isDemotion: true, assign: assign})
			}
		}

		return true
	}, nil)

	if len(mods) == 0 {
		return nil
	}

	// Sort modifications by source code position
	sort.Slice(mods, func(i, j int) bool {
		return mods[i].pos < mods[j].pos
	})

	// PASS 2: Apply modifications in order, using a new traversal to get a valid cursor
	modIndex := 0
	errIntroducedInFunc := make(map[*ast.FuncDecl]bool)
	var modified bool

	astutil.Apply(node, func(cursor *astutil.Cursor) bool {
		if modIndex >= len(mods) {
			return false // Stop traversal if all mods are done
		}

		stmt, ok := cursor.Node().(ast.Stmt)
		if !ok { return true }
		
		currentMod := mods[modIndex]
		if stmt != currentMod.assign {
			return true // Not the statement we're looking for yet
		}
		
		// We are at the right statement, apply the modification
		assign := currentMod.assign
		scope := innermostScope(node, assign, info)
		enclosingFunc := findEnclosingFunc(node, assign.Pos())
		if enclosingFunc == nil {
			log.Printf("Skipping modification at %s because enclosing function could not be found.", fset.Position(assign.Pos()))
			modIndex++
			return true
		}

		if currentMod.isDemotion {
			isDeclaredInTypes := scope != nil && scope.Lookup("err") != nil
			isDeclaredByUs := errIntroducedInFunc[enclosingFunc]

			if isDeclaredByUs || isDeclaredInTypes {
				fmt.Printf("Demoting `err :=` to `err =` in %s at line %d\n", fset.File(assign.Pos()).Name(), fset.Position(assign.Pos()).Line)
				assign.Tok = token.ASSIGN
				modified = true
			}
			errIntroducedInFunc[enclosingFunc] = true
		} else {
			fmt.Printf("Found ignored error in %s at line %d\n", fset.File(assign.Pos()).Name(), fset.Position(assign.Pos()).Line)
			modified = true

			isErrAlreadyDeclared := errIntroducedInFunc[enclosingFunc] || (scope != nil && scope.Lookup("err") != nil)
			tok := assign.Tok
			if tok == token.ASSIGN && !isErrAlreadyDeclared {
				tok = token.DEFINE
			} else if tok == token.DEFINE && isErrAlreadyDeclared {
				if !anyOtherNewVariables(assign, currentMod.errIdx, info) {
					tok = token.ASSIGN
				}
			}
			if tok == token.DEFINE && !isErrAlreadyDeclared {
				errIntroducedInFunc[enclosingFunc] = true
			}

			assign.Lhs[currentMod.errIdx] = &ast.Ident{Name: "err"}
			assign.Tok = tok
			
			if enclosingFunc.Name.Name == "main" {
				astutil.AddImport(fset, node, "log")
				cursor.InsertAfter(createErrorCheckForMain())
			} else if canReturnError(enclosingFunc) {
				cursor.InsertAfter(createErrorCheck(enclosingFunc))
			}
		}
		
		modIndex++ // Move to the next modification
		return true
	}, nil)

	if !modified {
		return nil
	}

	var buf bytes.Buffer
	if err := format.Node(&buf, fset, node); err != nil {
		return fmt.Errorf("failed to format node: %w", err)
	}

	fmt.Printf("Writing modified file: %s\n", filePath)
	return os.WriteFile(filePath, buf.Bytes(), 0644)
}

func renderNode(fset *token.FileSet, node ast.Node) string {
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, node); err != nil {
		return fmt.Sprintf("error formatting node: %v", err)
	}
	return buf.String()
}

func isErrorType(t types.Type) bool {
	if t == nil {
		return false
	}
	// The `error` type is a named type, which is an interface.
	// We can check if a type implements the error interface.
	errorInterface, ok := types.Universe.Lookup("error").Type().Underlying().(*types.Interface)
	if !ok {
		return false // Should not happen
	}
	return types.Implements(t, errorInterface)
}

func canReturnError(fn *ast.FuncDecl) bool {
	if fn.Type.Results == nil {
		return false
	}
	if len(fn.Type.Results.List) == 0 {
		return false
	}
	lastResult := fn.Type.Results.List[len(fn.Type.Results.List)-1]
	// This is a simplistic check. A proper check would use type information.
	if id, ok := lastResult.Type.(*ast.Ident); ok {
		return id.Name == "error"
	}
	return false
}

func findEnclosingFunc(file *ast.File, pos token.Pos) *ast.FuncDecl {
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			if pos >= fn.Pos() && pos < fn.End() {
				return fn
			}
		}
	}
	return nil
}

func createErrorCheck(enclosingFunc *ast.FuncDecl) *ast.IfStmt {
	// if err != nil
	ifStmt := &ast.IfStmt{
		Cond: &ast.BinaryExpr{
			X:  &ast.Ident{Name: "err"},
			Op: token.NEQ,
			Y:  &ast.Ident{Name: "nil"},
		},
		Body: &ast.BlockStmt{},
	}

	// Create return statement
	retStmt := &ast.ReturnStmt{}
	if enclosingFunc.Type.Results != nil {
		for _, field := range enclosingFunc.Type.Results.List {
			retStmt.Results = append(retStmt.Results, zeroValue(field.Type))
		}
	}
	// The last return value should be the error
	if len(retStmt.Results) > 0 {
		last := len(retStmt.Results) - 1
		// Only replace if it is an error type
		if id, ok := enclosingFunc.Type.Results.List[last].Type.(*ast.Ident); ok && id.Name == "error" {
			retStmt.Results[last] = &ast.Ident{Name: "err"}
		}
	}

	ifStmt.Body.List = []ast.Stmt{retStmt}

	return ifStmt
}

func zeroValue(t ast.Expr) ast.Expr {
	switch v := t.(type) {
	case *ast.Ident:
		switch v.Name {
		case "string":
			return &ast.BasicLit{Kind: token.STRING, Value: `""`}
		case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "uintptr", "byte", "rune":
			return &ast.BasicLit{Kind: token.INT, Value: "0"}
		case "float32", "float64":
			return &ast.BasicLit{Kind: token.FLOAT, Value: "0"}
		case "bool":
			return &ast.Ident{Name: "false"}
		default:
			// Assume it's a struct type
			return &ast.CompositeLit{Type: v}
		}
	case *ast.StarExpr, *ast.ArrayType, *ast.MapType, *ast.ChanType, *ast.InterfaceType, *ast.FuncType:
		return &ast.Ident{Name: "nil"}
	case *ast.SelectorExpr: // For types like a.B
		return &ast.CompositeLit{Type: t}
	default:
		return &ast.Ident{Name: "nil"} // best guess
	}
}

func createErrorCheckForMain() *ast.IfStmt {
	// if err != nil { log.Fatalf("error: %v", err) }
	return &ast.IfStmt{
		Cond: &ast.BinaryExpr{
			X:  &ast.Ident{Name: "err"},
			Op: token.NEQ,
			Y:  &ast.Ident{Name: "nil"},
		},
		Body: &ast.BlockStmt{
			List: []ast.Stmt{
				&ast.ExprStmt{
					X: &ast.CallExpr{
						Fun: &ast.SelectorExpr{
							X:   &ast.Ident{Name: "log"},
							Sel: &ast.Ident{Name: "Fatalf"},
						},
						Args: []ast.Expr{
							&ast.BasicLit{Kind: token.STRING, Value: `"error: %v"`},
							&ast.Ident{Name: "err"},
						},
					},
				},
			},
		},
	}
}

func getErrorIndex(assign *ast.AssignStmt, info *types.Info) int {
	if len(assign.Rhs) != 1 {
		return -1
	}
	call, ok := assign.Rhs[0].(*ast.CallExpr)
	if !ok {
		return -1
	}
	var sig *types.Tuple
	if callType := info.TypeOf(call); callType != nil {
		switch t := callType.(type) {
		case *types.Signature:
			sig = t.Results()
		case *types.Tuple:
			sig = t
		}
	}
	if sig == nil {
		return -1
	}

	for i := 0; i < sig.Len(); i++ {
		if isErrorType(sig.At(i).Type()) {
			return i
		}
	}
	return -1
}

func anyOtherNewVariables(assign *ast.AssignStmt, errIdx int, info *types.Info) bool {
	for i, lhsExpr := range assign.Lhs {
		if i == errIdx {
			continue
		}
		if id, ok := lhsExpr.(*ast.Ident); ok {
			if info.Defs[id] != nil {
				return true
			}
		}
	}
	return false
}

func isErrAlreadyDeclared(scope *types.Scope) bool {
	if scope == nil {
		return false
	}
	// If not in our map, check the type info by looking in parent scopes
	if scope.Lookup("err") != nil {
		return true
	}
	return false
}

// innermostScope finds the narrowest scope enclosing a statement.
func innermostScope(file *ast.File, stmt ast.Stmt, info *types.Info) *types.Scope {
	path, _ := astutil.PathEnclosingInterval(file, stmt.Pos(), stmt.End())
	if path == nil {
		return nil
	}

	for i := len(path) - 1; i >= 0; i-- {
		if scope, ok := info.Scopes[path[i]]; ok {
			return scope
		}
	}
	return nil
}

func isIgnored(e ast.Expr) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == "_"
} 