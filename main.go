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
	errIntroducedInScope := make(map[*types.Scope]bool)

	var modified bool
	astutil.Apply(node, func(cursor *astutil.Cursor) bool {
		stmt, ok := cursor.Node().(ast.Stmt)
		if !ok {
			return true
		}

		assign, ok := stmt.(*ast.AssignStmt)
		if !ok {
			return true
		}
		if assign.Tok != token.DEFINE && assign.Tok != token.ASSIGN {
			return true
		}

		for i, expr := range assign.Lhs {
			if id, ok := expr.(*ast.Ident); ok && id.Name == "_" {
				// We have an ignored value
				if len(assign.Rhs) != 1 {
					continue
				}
				call, ok := assign.Rhs[0].(*ast.CallExpr)
				if !ok {
					continue
				}

				// == BEGIN DIAGNOSTICS ==
				fmt.Printf("\n[DEBUG] Analyzing call in %s: %s\n", fset.File(call.Pos()).Name(), renderNode(fset, call))
				callType := info.TypeOf(call)
				if callType == nil {
					fmt.Println("[DEBUG] -> Type info is nil. Skipping.")
					return true
				}
				fmt.Printf("[DEBUG] -> Call type: %T | %s\n", callType, callType.String())
				// == END DIAGNOSTICS ==

				var sig *types.Tuple
				switch t := callType.(type) {
				case *types.Signature:
					sig = t.Results()
				case *types.Tuple:
					sig = t
				default:
					return true // Not a function call we can analyze.
				}

				if sig.Len() != len(assign.Lhs) {
					continue
				}

				if i >= sig.Len() {
					continue
				}
				v := sig.At(i)

				// == BEGIN DIAGNOSTICS ==
				vType := v.Type()
				isErr := isErrorType(vType)
				fmt.Printf("[DEBUG] -> Return value %d: Type=`%s`, IsError=%v\n", i, vType.String(), isErr)
				// == END DIAGNOSTICS ==

				if isErr {
					modified = true

					// Find the current scope and check if `err` is already declared there or in a parent scope.
					scope := innermostScope(node, stmt, info)
					isErrDeclared := false
					if scope != nil {
						tempScope := scope
						for tempScope != nil {
							if tempScope.Lookup("err") != nil || errIntroducedInScope[tempScope] {
								isErrDeclared = true
								break
							}
							tempScope = tempScope.Parent()
						}
					}

					// Decide whether to use `=` or `:=`
					tok := assign.Tok
					if tok == token.ASSIGN {
						if !isErrDeclared {
							tok = token.DEFINE
							if scope != nil {
								errIntroducedInScope[scope] = true
							}
						}
					} else { // token.DEFINE
						if isErrDeclared {
							// If `err` is already declared, and no other variables on the LHS are new,
							// we must demote to `=`.
							anyOtherNew := false
							for j, lhsExpr := range assign.Lhs {
								if i == j {
									continue
								}
								if id, ok := lhsExpr.(*ast.Ident); ok {
									if info.Defs[id] != nil {
										anyOtherNew = true
										break
									}
								}
							}
							if !anyOtherNew {
								tok = token.ASSIGN
							}
						} else {
							// It was `:=` and we are introducing a new `err` variable.
							if scope != nil {
								errIntroducedInScope[scope] = true
							}
						}
					}

					// It's an error type!
					fmt.Printf("Found ignored error in %s at line %d\n", fset.File(assign.Pos()).Name(), fset.Position(assign.Pos()).Line)

					// Rewrite
					assign.Lhs[i] = &ast.Ident{Name: "err"}
					assign.Tok = tok

					// Find enclosing function
					enclosingFunc := findEnclosingFunc(node, assign.Pos())
					if enclosingFunc == nil {
						log.Printf("Could not find enclosing function for assignment at %s", fset.Position(assign.Pos()))
						return true
					}

					if enclosingFunc.Name.Name == "main" {
						// Special handling for main
						modified = true
						astutil.AddImport(fset, node, "log")
						assign.Lhs[i] = &ast.Ident{Name: "err"}
						ifStmt := createErrorCheckForMain()
						cursor.InsertAfter(ifStmt)
						return true
					}

					// Check if enclosing function can return an error
					if !canReturnError(enclosingFunc) {
						log.Printf("Skipping ignored error in %s at line %d because enclosing function does not return an error", fset.File(assign.Pos()).Name(), fset.Position(assign.Pos()).Line)
						return true
					}

					// Create the if statement
					ifStmt := createErrorCheck(enclosingFunc)

					// Insert the if statement
					cursor.InsertAfter(ifStmt)
				}
			}
		}

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