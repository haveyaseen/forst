package main

import (
	"flag"
	"fmt"
	"forst/pkg/ast"
	"forst/pkg/generators"
	"forst/pkg/lexer"
	"forst/pkg/parser"
	transformer_go "forst/pkg/transformer/go"
	"forst/pkg/typechecker"
	goast "go/ast"
	"os"
)

type ProgramArgs struct {
	filePath string
	debug    bool
}

func parseArgs() ProgramArgs {
	debug := flag.Bool("debug", false, "Enable debug output")
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		fmt.Println("Usage: forst <filename>.ft")
		return ProgramArgs{}
	}
	return ProgramArgs{
		filePath: args[0],
		debug:    *debug,
	}
}

func readSourceFile(filePath string) ([]byte, error) {
	source, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("error reading file: %v", err)
	}
	return source, nil
}

func debugPrintTokens(tokens []ast.Token) {
	fmt.Println("\n=== Tokens ===")
	for _, t := range tokens {
		fmt.Printf("%s:%d:%d: %-12s '%s'\n",
			t.Path, t.Line, t.Column,
			t.Type, t.Value)
	}
}

func debugPrintForstAST(forstAST []ast.Node) {
	fmt.Println("\n=== Forst AST ===")
	for _, node := range forstAST {
		switch n := node.(type) {
		case ast.PackageNode:
			fmt.Printf("  Package: %s\n", n.Ident)
		case ast.ImportNode:
			fmt.Printf("  Import: %s\n", n.Path)
		case ast.ImportGroupNode:
			fmt.Printf("  ImportGroup: %v\n", n.Imports)
		case ast.FunctionNode:
			if n.HasExplicitReturnType() {
				fmt.Printf("  Function: %s -> %s\n", n.Ident.Id, n.ReturnType)
			} else {
				fmt.Printf("  Function: %s -> (?)\n", n.Ident.Id)
			}
		case ast.EnsureNode:
			if n.Error != nil {
				fmt.Printf("  Ensure: %s or %s\n", n.Assertion, (*n.Error).String())
			} else {
				fmt.Printf("  Ensure: %s\n", n.Assertion)
			}
		case ast.ReturnNode:
			fmt.Printf("  Return: %s\n", n.Value)
		}
	}
}

func debugPrintGoAST(goFile *goast.File) {
	fmt.Println("\n=== Go AST ===")
	fmt.Printf("  Package: %s\n", goFile.Name)

	fmt.Println("  Imports:")
	for _, imp := range goFile.Imports {
		fmt.Printf("    %s\n", imp.Path.Value)
	}

	fmt.Println("  Declarations:")
	for _, decl := range goFile.Decls {
		switch d := decl.(type) {
		case *goast.FuncDecl:
			fmt.Printf("    Function: %s\n", d.Name.Name)
		case *goast.GenDecl:
			fmt.Printf("    GenDecl: %s\n", d.Tok)
		}
	}
}

func debugPrintTypeInfo(tc *typechecker.TypeChecker) {
	fmt.Println("\n=== Type Check Results ===")

	fmt.Println("  Functions:")
	for id, sig := range tc.Functions {
		fmt.Printf("    %s(", id)
		for i, param := range sig.Parameters {
			if i > 0 {
				fmt.Print(", ")
			}
			fmt.Printf("%s: %s", param.Id(), param.Type)
		}
		fmt.Printf(") -> %s\n", sig.ReturnType)
	}

	fmt.Println("  Definitions:")
	for id, def := range tc.Defs {
		fmt.Printf("  %s -> %T\n", id, def)
	}
}

func main() {
	args := parseArgs()
	if args.filePath == "" {
		return
	}

	source, err := readSourceFile(args.filePath)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Lexical Analysis
	tokens := lexer.Lexer(source, lexer.Context{FilePath: args.filePath})
	if args.debug {
		debugPrintTokens(tokens)
	}

	// Parsing
	forstNodes := parser.NewParser(tokens).ParseFile()
	if args.debug {
		debugPrintForstAST(forstNodes)
	}

	// Semantic Analysis
	checker := typechecker.New()

	// Collect, infer and check type
	if err := checker.CheckTypes(forstNodes); err != nil {
		fmt.Printf("Type checking error: %v\n", err)
		return
	}

	if args.debug {
		debugPrintTypeInfo(checker)
	}

	// Transform to Go AST with type information
	transformer := transformer_go.New()
	goAST, err := transformer.TransformForstFileToGo(forstNodes, checker)
	if err != nil {
		fmt.Printf("Transformation error: %v\n", err)
		return
	}

	if args.debug {
		debugPrintGoAST(goAST)
	}

	// Generate Go code
	goCode := generators.GenerateGoCode(goAST)

	if args.debug {
		fmt.Println("\n=== Generated Go Code ===")
	}
	fmt.Println(goCode)
}
