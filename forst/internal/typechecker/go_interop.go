package typechecker

import (
	"errors"
	"fmt"
	"go/types"
	"path/filepath"
	"strings"

	"forst/internal/ast"
	"forst/internal/forstdep"
	"forst/internal/goload"
	"forst/internal/typechecker/gointerop"

	"github.com/sirupsen/logrus"
	"golang.org/x/tools/go/packages"
)

func fallbackImportLocal(imp ast.ImportNode) (path, local string) {
	return gointerop.FallbackImportLocal(imp)
}

func goIdentifierExported(name string) bool {
	return gointerop.IdentifierExported(name)
}

func (tc *TypeChecker) mapGoType(t types.Type) (ast.TypeNode, bool) {
	if tc == nil {
		return gointerop.TypeToForstType(t)
	}
	return gointerop.MapGoType(tc.goInteropHost(), t)
}

func goTypeToForstType(t types.Type) (ast.TypeNode, bool) {
	return gointerop.TypeToForstType(t)
}

func goTypeAtFieldPath(recv types.Type, fieldPath []string) (types.Type, error) {
	return gointerop.TypeAtFieldPath(recv, fieldPath)
}

func goNamedTypeRoot(g types.Type) (*types.Named, bool) {
	return gointerop.NamedTypeRoot(g)
}

// goPackagesLoadDir returns the directory passed to go/packages (module root when set, else ".").
func (tc *TypeChecker) goPackagesLoadDir() string {
	if tc.GoWorkspaceDir != "" {
		return tc.GoWorkspaceDir
	}
	return "."
}

// initGoImportPackages loads Go packages for Forst import lines via go/packages.
func (tc *TypeChecker) initGoImportPackages() {
	tc.ensureImportPathByLocal()
	missing := tc.missingGoImportPaths()
	if len(missing) == 0 {
		if tc.allGoImportLocalsLoaded() {
			tc.goPackagesPreloaded = true
		}
		return
	}
	loaded, err := goload.LoadByPkgPath(tc.goPackagesLoadDir(), missing)
	if err != nil {
		tc.recordGoPackagesLoadFailure(missing, err)
		tc.log.WithFields(logrus.Fields{
			"function": "initGoImportPackages",
			"dir":      tc.goPackagesLoadDir(),
			"missing":  missing,
		}).WithError(err).Debug("go/packages load failed")
		return
	}
	tc.seedGoImportPackagesFromLoaded(loaded)
	if tc.allGoImportLocalsLoaded() {
		tc.goPackagesPreloaded = true
	}
}

// InitGoPackagesFromBatch maps import locals from a preloaded go/packages batch (module-wide).
func (tc *TypeChecker) InitGoPackagesFromBatch(loaded map[string]*packages.Package) error {
	tc.ensureImportPathByLocal()
	if len(loaded) > 0 {
		tc.seedGoImportPackagesFromLoaded(loaded)
	}
	if tc.allGoImportLocalsLoaded() {
		tc.goPackagesPreloaded = true
	}
	if tc.samePackageGoImportPath != "" {
		if forstMap := tc.importPathToForstPkgMap(); forstMap != nil && forstMap[tc.samePackageGoImportPath] != "" {
			if err := tc.loadSamePackageGoWithGenStub(); err != nil {
				tc.log.WithError(err).Debug("same-package Go load with gen stub failed")
			}
		} else if pkg, ok := loaded[tc.samePackageGoImportPath]; ok && goload.PackageLoadOK(pkg, tc.samePackageGoImportPath) {
			tc.samePackageGo = pkg.Types
		}
	}
	return tc.registerSamePackageGoNamedTypes()
}

func (tc *TypeChecker) loadSamePackageGoWithGenStub() error {
	path := tc.samePackageGoImportPath
	if path == "" || tc.GoWorkspaceDir == "" {
		return nil
	}
	locs, err := goload.LocatePackageDirs(tc.GoWorkspaceDir, []string{path})
	if err != nil {
		return err
	}
	loc, ok := locs[path]
	if !ok || loc.Dir == "" {
		return nil
	}
	pkgName := tc.ForstPackage()
	if pkgName == "" {
		pkgName = filepath.Base(loc.Dir)
	}
	overlay, err := forstdep.GenGoStubOverlay(loc.Dir, pkgName)
	if err != nil {
		return err
	}
	var opts []goload.LoadOpt
	if len(overlay) > 0 {
		opts = append(opts, goload.WithOverlay(overlay))
	}
	loaded, err := goload.LoadByPkgPath(tc.GoWorkspaceDir, []string{path}, opts...)
	if err != nil {
		return nil // Forst-only dirs often fail go/packages; not an error for Forst
	}
	pkg, ok := loaded[path]
	if !ok || !goload.PackageLoadOK(pkg, path) {
		return nil
	}
	tc.samePackageGo = pkg.Types
	return nil
}

func (tc *TypeChecker) ensureImportPathByLocal() {
	if tc.importPathByLocal == nil {
		tc.importPathByLocal = make(map[string]string)
	}
	tc.registerImportLocalsFromAST()
}

func (tc *TypeChecker) missingGoImportPaths() []string {
	seen := make(map[string]struct{})
	var paths []string
	forstMap := tc.importPathToForstPkgMap()
	for _, imp := range tc.imports {
		ip := goload.ImportPathFromForst(imp.Path)
		if ip == "" {
			continue
		}
		if forstMap != nil && forstMap[ip] != "" {
			continue
		}
		if imp.Alias != nil && string(imp.Alias.ID) == "." {
			continue
		}
		path, local := fallbackImportLocal(imp)
		if local == "" || local == "." {
			continue
		}
		if tc.goPkgsByLocal != nil && tc.goPkgsByLocal[local] != nil {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}
	return paths
}

func (tc *TypeChecker) allGoImportLocalsLoaded() bool {
	forstMap := tc.importPathToForstPkgMap()
	for _, imp := range tc.imports {
		ip := goload.ImportPathFromForst(imp.Path)
		if ip == "" {
			continue
		}
		if forstMap != nil && forstMap[ip] != "" {
			continue
		}
		if imp.Alias != nil && string(imp.Alias.ID) == "." {
			continue
		}
		_, local := fallbackImportLocal(imp)
		if local == "" || local == "." {
			continue
		}
		if tc.goPkgsByLocal == nil || tc.goPkgsByLocal[local] == nil {
			return false
		}
	}
	return true
}

func (tc *TypeChecker) hasDotImportPath(path string) bool {
	for _, pkg := range tc.dotImportPkgs {
		if pkg != nil && pkg.Path() == path {
			return true
		}
	}
	return false
}

func (tc *TypeChecker) seedGoImportPackagesFromLoaded(loaded map[string]*packages.Package) {
	if tc.goPkgsByLocal == nil {
		tc.goPkgsByLocal = make(map[string]*types.Package)
	}
	forstMap := tc.importPathToForstPkgMap()
	for _, imp := range tc.imports {
		ip := goload.ImportPathFromForst(imp.Path)
		if ip == "" {
			continue
		}
		// Forst sibling packages are resolved from .ft, not go/packages (may be Forst-only or have *.gen.go).
		if forstMap != nil && forstMap[ip] != "" {
			continue
		}
		pkgp, ok := loaded[ip]
		if !ok || !goload.PackageLoadOK(pkgp, ip) {
			continue
		}
		tp := pkgp.Types
		if tp == nil {
			continue
		}
		var local string
		b := goBindingFromLoaded(imp, loaded)
		if b.Skip {
			if imp.Alias != nil && string(imp.Alias.ID) == "." {
				if !tc.hasDotImportPath(tp.Path()) {
					tc.dotImportPkgs = append(tc.dotImportPkgs, tp)
				}
			}
			continue
		}
		local = b.Local
		if local == "" || local == "." {
			continue
		}
		tc.goPkgsByLocal[local] = tp
	}
}

func collectGoImportPaths(tcs []*TypeChecker) []string {
	pathSet := make(map[string]struct{})
	for _, tc := range tcs {
		if tc == nil {
			continue
		}
		forstMap := tc.importPathToForstPkgMap()
		for _, p := range gointerop.ImportPathsFromForstImports(tc.imports) {
			if forstMap != nil && forstMap[p] != "" {
				continue
			}
			pathSet[p] = struct{}{}
		}
		if tc.samePackageGoImportPath != "" {
			if forstMap != nil && forstMap[tc.samePackageGoImportPath] != "" {
				continue
			}
			pathSet[tc.samePackageGoImportPath] = struct{}{}
		}
	}
	if len(pathSet) == 0 {
		return nil
	}
	paths := make([]string, 0, len(pathSet))
	for p := range pathSet {
		paths = append(paths, p)
	}
	return paths
}

// BatchLoadGoPackagesForModule unions Go import paths from typecheckers and loads once.
func BatchLoadGoPackagesForModule(moduleRoot string, tcs []*TypeChecker) (map[string]*packages.Package, error) {
	return gointerop.LoadPackages(moduleRoot, collectGoImportPaths(tcs), nil)
}

// BatchLoadGoPackagesForModuleWithLoader is like BatchLoadGoPackagesForModule but accepts a custom loader.
func BatchLoadGoPackagesForModuleWithLoader(moduleRoot string, tcs []*TypeChecker, loader goload.PackagesLoader) (map[string]*packages.Package, error) {
	return gointerop.LoadPackages(moduleRoot, collectGoImportPaths(tcs), loader)
}

func (tc *TypeChecker) initSamePackageGoExports() error {
	if tc.goPackagesPreloaded {
		return tc.registerSamePackageGoNamedTypes()
	}
	tc.samePackageGo = nil
	if tc.samePackageGoImportPath == "" || tc.GoWorkspaceDir == "" {
		return nil
	}
	loaded, err := goload.LoadByPkgPath(tc.GoWorkspaceDir, []string{tc.samePackageGoImportPath})
	if err != nil {
		tc.log.WithFields(logrus.Fields{
			"function": "initSamePackageGoExports",
			"path":     tc.samePackageGoImportPath,
		}).WithError(err).Debug("go/packages load failed for same-package Go exports")
		return nil
	}
	pkg, ok := loaded[tc.samePackageGoImportPath]
	if !ok || !goload.PackageLoadOK(pkg, tc.samePackageGoImportPath) {
		return nil
	}
	tc.samePackageGo = pkg.Types
	return tc.registerSamePackageGoNamedTypes()
}

// registerSamePackageGoNamedTypes imports same-package Go named types into Defs so Forst
// signatures and literals can refer to them without re-emitting the Go type declarations.
// An existing Forst definition for the same ident must be structurally compatible with the
// Go type; otherwise a duplicate-type diagnostic is returned.
func (tc *TypeChecker) registerSamePackageGoNamedTypes() error {
	if tc.samePackageGo == nil {
		return nil
	}
	if tc.goPackageTypeIdents == nil {
		tc.goPackageTypeIdents = make(map[ast.TypeIdent]struct{})
	}
	scope := tc.samePackageGo.Scope()
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		tn, ok := obj.(*types.TypeName)
		if !ok || tn == nil {
			continue
		}
		ident := ast.TypeIdent(name)
		def, ok := tc.typeDefFromSamePackageGoType(tn)
		if !ok {
			tc.log.WithFields(logrus.Fields{
				"function": "registerSamePackageGoNamedTypes",
				"type":     name,
			}).Debug("skipping same-package Go type that could not be mapped to a Forst typedef")
			continue
		}
		if existing, exists := tc.Defs[ident]; exists {
			if tc.IsGoPackageType(ident) {
				continue
			}
			if err := tc.ensureForstDefCompatibleWithSamePackageGo(ident, existing, def); err != nil {
				return err
			}
			// Compatible Forst alias of the same-package Go type: omit re-emit.
			tc.goPackageTypeIdents[ident] = struct{}{}
			continue
		}
		tc.Defs[ident] = def
		tc.goPackageTypeIdents[ident] = struct{}{}
		tc.log.WithFields(logrus.Fields{
			"function": "registerSamePackageGoNamedTypes",
			"type":     name,
		}).Debug("registered same-package Go named type into Forst Defs")
	}
	return nil
}

func (tc *TypeChecker) ensureForstDefCompatibleWithSamePackageGo(ident ast.TypeIdent, existing ast.Node, goDef ast.TypeDefNode) error {
	existingTD, ok := existing.(ast.TypeDefNode)
	if !ok {
		return reportBodyf(ast.FakeSpan(), "duplicate-type",
			"type %s conflicts with same-package Go type %s", ident, ident)
	}
	existingShape, existingOK := tc.getShapeFromTypeDef(existingTD)
	goShape, goOK := tc.getShapeFromTypeDef(goDef)
	if existingOK && goOK {
		if tc.shapesAreStructurallyIdentical(*existingShape, *goShape) {
			return nil
		}
		return reportBodyf(ast.FakeSpan(), "duplicate-type",
			"Forst type %s conflicts with same-package Go type %s (incompatible fields)", ident, ident)
	}
	// Non-shape typedefs: accept only when both map to the same assertion base type.
	if existingOK != goOK {
		return reportBodyf(ast.FakeSpan(), "duplicate-type",
			"Forst type %s conflicts with same-package Go type %s", ident, ident)
	}
	if ea, ok := existingTD.Expr.(ast.TypeDefAssertionExpr); ok {
		if ga, ok := goDef.Expr.(ast.TypeDefAssertionExpr); ok {
			if ea.Assertion != nil && ga.Assertion != nil &&
				ea.Assertion.BaseType != nil && ga.Assertion.BaseType != nil &&
				*ea.Assertion.BaseType == *ga.Assertion.BaseType {
				return nil
			}
		}
	}
	return reportBodyf(ast.FakeSpan(), "duplicate-type",
		"Forst type %s conflicts with same-package Go type %s", ident, ident)
}

// SamePackageGoDefinesType reports whether same-package Go already defines ident as a type name.
func (tc *TypeChecker) SamePackageGoDefinesType(ident ast.TypeIdent) bool {
	if tc == nil || tc.samePackageGo == nil {
		return false
	}
	name := string(ident)
	if name == "" || strings.Contains(name, ".") {
		return false
	}
	obj := tc.samePackageGo.Scope().Lookup(name)
	if obj == nil {
		return false
	}
	_, ok := obj.(*types.TypeName)
	return ok
}

// IsGoPackageType reports whether ident was registered from same-package Go (do not emit).
func (tc *TypeChecker) IsGoPackageType(ident ast.TypeIdent) bool {
	if tc == nil || tc.goPackageTypeIdents == nil {
		return false
	}
	_, ok := tc.goPackageTypeIdents[ident]
	return ok
}

func (tc *TypeChecker) typeDefFromSamePackageGoType(tn *types.TypeName) (ast.TypeDefNode, bool) {
	if tn == nil {
		return ast.TypeDefNode{}, false
	}
	ident := ast.TypeIdent(tn.Name())
	goTyp := types.Unalias(tn.Type())
	switch u := goTyp.Underlying().(type) {
	case *types.Struct:
		shape, ok := shapeFromSamePackageGoStruct(u)
		if !ok {
			return ast.TypeDefNode{}, false
		}
		return ast.TypeDefNode{
			Ident: ident,
			Expr:  ast.TypeDefShapeExpr{Shape: shape},
		}, true
	case *types.Basic:
		ft, ok := tc.mapGoType(u)
		if !ok || ft.Ident == ast.TypeImplicit {
			return ast.TypeDefNode{}, false
		}
		base := ft.Ident
		return ast.TypeDefNode{
			Ident: ident,
			Expr: ast.TypeDefAssertionExpr{
				Assertion: &ast.AssertionNode{BaseType: &base},
			},
		}, true
	case *types.Slice:
		elem, ok := tc.mapGoType(u.Elem())
		if !ok || elem.Ident == ast.TypeImplicit {
			return ast.TypeDefNode{}, false
		}
		base := ast.TypeArray
		return ast.TypeDefNode{
			Ident: ident,
			Expr: ast.TypeDefAssertionExpr{
				Assertion: &ast.AssertionNode{
					BaseType:   &base,
					TypeParams: []ast.TypeNode{elem},
				},
			},
		}, true
	default:
		return ast.TypeDefNode{}, false
	}
}

// shapeFromSamePackageGoStruct builds a Forst shape preserving Go field names (same-package).
func shapeFromSamePackageGoStruct(st *types.Struct) (ast.ShapeNode, bool) {
	if st == nil {
		return ast.ShapeNode{}, false
	}
	fields := make(map[string]ast.ShapeFieldNode, st.NumFields())
	order := make([]string, 0, st.NumFields())
	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)
		if f == nil || f.Anonymous() {
			continue
		}
		ft, ok := gointerop.TypeToForstType(f.Type())
		if !ok {
			continue
		}
		ftCopy := ft
		name := f.Name()
		fields[name] = ast.ShapeFieldNode{Type: &ftCopy, GoExport: f.Exported()}
		order = append(order, name)
	}
	return ast.ShapeNode{Fields: fields, FieldOrder: order}, true
}

func (tc *TypeChecker) trySamePackageGoCall(funcName string, e ast.FunctionCallNode, argTypes [][]ast.TypeNode, wantSingleValue bool) ([]ast.TypeNode, bool, error) {
	if tc.samePackageGo == nil {
		return nil, false, nil
	}
	ret, err := tc.checkGoFuncCall(tc.samePackageGo, funcName, funcName, e, argTypes, wantSingleValue)
	if err != nil {
		var diag *Diagnostic
		if errors.As(err, &diag) {
			switch {
			case diag.Code == "go-member-missing" || strings.Contains(diag.Error(), "not found"):
				// Soft miss: fall through to Forst builtins (e.g. println).
				return nil, false, nil
			case strings.Contains(diag.Error(), "is not a function"):
				return nil, false, nil
			default:
				return nil, true, err
			}
		}
		var mm *gointerop.MemberMissingError
		if errors.As(err, &mm) {
			return nil, false, nil
		}
		return nil, true, err
	}
	return ret, true, nil
}

func (tc *TypeChecker) goPackageForImportLocal(local string) *types.Package {
	if local == "" || local == "." {
		return nil
	}
	if tc.goPkgsByLocal != nil {
		if p := tc.goPkgsByLocal[local]; p != nil {
			return p
		}
	}
	path := ""
	if tc.importPathByLocal != nil {
		path = tc.importPathByLocal[local]
	}
	if path == "" {
		return nil
	}
	loaded, err := goload.LoadByPkgPath(tc.goPackagesLoadDir(), []string{path})
	if err != nil || len(loaded) == 0 {
		return nil
	}
	pkgp, ok := loaded[path]
	if !ok || !goload.PackageLoadOK(pkgp, path) {
		return nil
	}
	gp := pkgp.Types
	if tc.goPkgsByLocal == nil {
		tc.goPkgsByLocal = make(map[string]*types.Package)
	}
	tc.goPkgsByLocal[local] = gp
	return gp
}

func (tc *TypeChecker) lookupDotImportFunc(funcName string, sp ast.SourceSpan) (*types.Package, error) {
	if len(tc.dotImportPkgs) == 0 {
		return nil, nil
	}
	if !goIdentifierExported(funcName) {
		return nil, nil
	}
	var matched []*types.Package
	for _, pkg := range tc.dotImportPkgs {
		if pkg == nil {
			continue
		}
		obj := pkg.Scope().Lookup(funcName)
		if obj == nil {
			continue
		}
		if _, ok := obj.(*types.Func); !ok {
			continue
		}
		matched = append(matched, pkg)
	}
	if len(matched) == 0 {
		return nil, nil
	}
	if len(matched) > 1 {
		return nil, reportBodyf(sp, "dot-import", "%s is ambiguous (multiple dot-imported packages)", funcName)
	}
	return matched[0], nil
}

func (tc *TypeChecker) checkGoFuncCall(pkg *types.Package, qualDisplay, funcName string, e ast.FunctionCallNode, argTypes [][]ast.TypeNode, wantSingleValue bool) ([]ast.TypeNode, error) {
	out, err := gointerop.CheckFuncCall(tc.goInteropHost(), tc.goInteropDiag(), gointerop.FuncCall{
		Pkg:             pkg,
		QualDisplay:     qualDisplay,
		FuncName:        funcName,
		Call:            e,
		ArgTypes:        argTypes,
		WantSingleValue: wantSingleValue,
	})
	return out, tc.mapGoInteropError(err)
}

func (tc *TypeChecker) checkGoQualifiedCall(pkg *types.Package, pkgDisplay, funcName string, e ast.FunctionCallNode, argTypes [][]ast.TypeNode, wantSingleValue bool) ([]ast.TypeNode, error) {
	out, err := gointerop.CheckFuncCall(tc.goInteropHost(), tc.goInteropDiag(), gointerop.FuncCall{
		Pkg:             pkg,
		QualDisplay:     pkgDisplay,
		FuncName:        funcName,
		Call:            e,
		ArgTypes:        argTypes,
		WantSingleValue: wantSingleValue,
		RequireExported: true,
	})
	return out, tc.mapGoInteropError(err)
}

func (tc *TypeChecker) mapGoInteropError(err error) error {
	if err == nil {
		return nil
	}
	var mm *gointerop.MemberMissingError
	if errors.As(err, &mm) && mm != nil {
		return goMemberMissingError(mm.Pkg, mm.Member, mm.Exports, mm.Span)
	}
	return err
}

func (tc *TypeChecker) checkGoSignature(sig *types.Signature, qual string, e ast.FunctionCallNode, argTypes [][]ast.TypeNode, wantSingleValue bool) ([]ast.TypeNode, error) {
	return gointerop.CheckSignature(tc.goInteropHost(), tc.goInteropDiag(), gointerop.SignatureCheck{
		Sig:             sig,
		Qual:            qual,
		Call:            e,
		ArgTypes:        argTypes,
		WantSingleValue: wantSingleValue,
	})
}

func (tc *TypeChecker) forstAssignableToGoType(f ast.TypeNode, g types.Type) bool {
	return gointerop.ForstAssignableToGoType(tc.goInteropHost(), f, g)
}

func (tc *TypeChecker) lookupGoImportedPackageSelector(local ast.Identifier, fieldPath []string, span ast.SourceSpan) (ast.TypeNode, error) {
	if len(fieldPath) == 0 {
		return ast.TypeNode{}, fmt.Errorf("package %s used as value", local)
	}
	gp := tc.goPackageForImportLocal(string(local))
	if gp == nil {
		return ast.TypeNode{}, fmt.Errorf("not an imported Go package: %s", local)
	}
	obj := gp.Scope().Lookup(fieldPath[0])
	if obj == nil {
		return ast.TypeNode{}, goMemberMissingError(string(local), fieldPath[0], goExportedNames(gp.Scope()), span)
	}
	var goTyp types.Type
	switch o := obj.(type) {
	case *types.Var:
		goTyp = o.Type()
	case *types.Const:
		goTyp = o.Type()
	case *types.Func:
		goTyp = o.Type()
	case *types.TypeName:
		return ast.TypeNode{}, fmt.Errorf("%s.%s is a type, not a value", local, fieldPath[0])
	default:
		return ast.TypeNode{}, fmt.Errorf("%s.%s is not a package variable", local, fieldPath[0])
	}
	if len(fieldPath) > 1 {
		return tc.lookupFieldPathFromGoType(goTyp, fieldPath[1:])
	}
	ft, ok := tc.mapGoType(goTyp)
	if !ok {
		return ast.TypeNode{}, fmt.Errorf("cannot map Go type %s", goTyp)
	}
	return ft, nil
}

func (tc *TypeChecker) lookupFieldPathFromGoType(goBase types.Type, fieldPath []string) (ast.TypeNode, error) {
	last, err := goTypeAtFieldPath(goBase, fieldPath)
	if err != nil {
		return ast.TypeNode{}, err
	}
	t, ok := tc.mapGoType(last)
	if !ok {
		return ast.TypeNode{}, fmt.Errorf("cannot map Go type %s", last)
	}
	return t, nil
}

func (tc *TypeChecker) goTypeDisplayStringForVariablePath(id ast.Identifier) (string, bool) {
	return tc.goTypeDisplayStringForVariable(id, ast.SourceSpan{})
}

func (tc *TypeChecker) goTypeDisplayStringForVariable(id ast.Identifier, span ast.SourceSpan) (string, bool) {
	if tc == nil {
		return "", false
	}
	parts := strings.Split(string(id), ".")
	if len(parts) == 0 {
		return "", false
	}
	base := ast.Identifier(parts[0])
	var gt types.Type
	if span.IsSet() {
		gt = tc.goTypeForVariableNode(ast.VariableNode{Ident: ast.Ident{ID: id, Span: span}})
	} else {
		gt = tc.goTypeForVariableIdent(base)
	}
	if gt == nil {
		return "", false
	}
	last, err := goTypeAtFieldPath(gt, parts[1:])
	if err != nil {
		return "", false
	}
	return last.String(), true
}

func (tc *TypeChecker) forstTypeForGoType(g types.Type) (ast.TypeNode, bool) {
	named, ok := goNamedTypeRoot(g)
	if !ok {
		return ast.TypeNode{}, false
	}
	pkg := named.Obj().Pkg()
	if pkg == nil {
		return ast.TypeNode{}, false
	}
	pkgPath := pkg.Path()
	typeName := named.Obj().Name()

	if tc.samePackageGoImportPath == pkgPath {
		if _, ok := tc.Defs[ast.TypeIdent(typeName)]; ok {
			return ast.TypeNode{Ident: ast.TypeIdent(typeName)}, true
		}
	}

	modPath := goload.ModulePath(tc.GoWorkspaceDir)
	if modPath != "" && strings.HasPrefix(pkgPath, modPath+"/") {
		if tc.samePackageGoImportPath == pkgPath {
			if _, ok := tc.Defs[ast.TypeIdent(typeName)]; ok {
				return ast.TypeNode{Ident: ast.TypeIdent(typeName)}, true
			}
		}

		local, ok := tc.ImportLocalForPath(pkgPath)
		if !ok {
			return ast.TypeNode{}, false
		}
		qualified := ast.TypeIdent(local + "." + typeName)

		if importMap := tc.importPathToForstPkgMap(); importMap != nil {
			if importMap[pkgPath] == "" {
				return ast.TypeNode{}, false
			}
			return ast.TypeNode{Ident: qualified}, true
		}

		if tc.goPackageForImportLocal(local) == nil {
			return ast.TypeNode{}, false
		}
		return ast.TypeNode{Ident: qualified}, true
	}

	local, ok := tc.importLocalForGoPackagePath(pkgPath)
	if !ok {
		return ast.TypeNode{}, false
	}
	return ast.TypeNode{Ident: ast.TypeIdent(local + "." + typeName)}, true
}

func (tc *TypeChecker) importLocalForGoPackagePath(pkgPath string) (string, bool) {
	if local, ok := tc.ImportLocalForPath(pkgPath); ok {
		return local, true
	}
	if tc.goPkgsByLocal != nil {
		for local, p := range tc.goPkgsByLocal {
			if p != nil && p.Path() == pkgPath {
				return local, true
			}
		}
	}
	return "", false
}

func (tc *TypeChecker) goTypeForQualifiedImportTypeIdent(typeIdent ast.TypeIdent) types.Type {
	importLocal, symbol, ok := parseForstSiblingTypeRef(typeIdent)
	if !ok {
		return nil
	}
	if importMap := tc.importPathToForstPkgMap(); importMap != nil {
		if path, ok := tc.ImportPathForLocal(importLocal); ok && importMap[path] != "" {
			return nil
		}
	}
	gp := tc.goPackageForImportLocal(importLocal)
	if gp == nil {
		return nil
	}
	obj := gp.Scope().Lookup(symbol)
	if obj == nil {
		return nil
	}
	return obj.Type()
}

func (tc *TypeChecker) goTypeForParamTypeNode(typ ast.TypeNode) types.Type {
	if typ.Ident == ast.TypePointer && len(typ.TypeParams) == 1 {
		if gt := tc.goTypeForQualifiedImportTypeIdent(typ.TypeParams[0].Ident); gt != nil {
			return types.NewPointer(gt)
		}
	}
	return tc.goTypeForQualifiedImportTypeIdent(typ.Ident)
}

func (tc *TypeChecker) bindVariableGoTypeFromParamType(ident ast.Ident, typ ast.TypeNode) {
	if normalized, ok := tc.normalizeGoImportParamType(typ); ok {
		typ = normalized
	}
	if gt := tc.goTypeForParamTypeNode(typ); gt != nil {
		if named, ok := gt.(*types.Named); ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == "testing" && named.Obj().Name() == "T" {
			gt = types.NewPointer(gt)
		}
		vn := ast.VariableNode{Ident: ident}
		tc.bindVariableGoType(ident.ID, &vn, gt)
	}
}

func (tc *TypeChecker) normalizeGoImportParamType(typ ast.TypeNode) (ast.TypeNode, bool) {
	if !ast.IsTestingTParamType(typ) {
		return ast.TypeNode{}, false
	}
	if typ.Ident == ast.TypePointer {
		if len(typ.TypeParams) == 1 {
			tc.registerGoQualifiedTypeAlias(typ.TypeParams[0].Ident, typ.TypeParams[0].Ident)
		}
		return typ, true
	}
	qualified := typ.Ident
	normalized := ast.TypeNode{
		Ident:      ast.TypePointer,
		TypeParams: []ast.TypeNode{{Ident: qualified}},
	}
	tc.registerGoQualifiedTypeAlias(qualified, qualified)
	return normalized, true
}

func (tc *TypeChecker) goTypeForExpression(expr ast.ExpressionNode) types.Type {
	gt, _ := tc.goTypeInfoForExpression(expr)
	return gt
}

// goTypeInfoForExpression returns the tracked Go type and whether the expression is addressable.
func (tc *TypeChecker) goTypeInfoForExpression(expr ast.ExpressionNode) (types.Type, bool) {
	if tc == nil || expr == nil {
		return nil, false
	}
	switch e := expr.(type) {
	case ast.VariableNode:
		parts := strings.Split(string(e.Ident.ID), ".")
		if len(parts) == 1 {
			if gt := tc.goTypeForVariableNode(e); gt != nil {
				if e.Ident.Span.IsSet() {
					k := variableOccurrenceKey{ident: e.Ident.ID, span: e.Ident.Span}
					tc.variableGoTypesByOccurrence[k] = gt
				}
				return gt, true
			}
			break
		}
		if base := tc.goTypeForVariableIdent(ast.Identifier(parts[0])); base != nil {
			last, err := goTypeAtFieldPath(base, parts[1:])
			if err == nil {
				return last, true
			}
		}
	case ast.FunctionCallNode:
		if gt := tc.goTypeFromBuiltinNewCall(e); gt != nil {
			return gt, false
		}
		if sig := tc.goFuncSignatureFromCall(e); sig != nil && sig.Results().Len() > 0 {
			return sig.Results().At(0).Type(), false
		}
	case ast.MethodCallNode:
		goRecv, addr := tc.goTypeInfoForExpression(e.Receiver)
		if goRecv != nil {
			obj, _, _ := types.LookupFieldOrMethod(goRecv, addr, nil, string(e.Method.ID))
			if fn, ok := obj.(*types.Func); ok {
				if sig, ok := fn.Type().(*types.Signature); ok && sig.Results().Len() > 0 {
					return sig.Results().At(0).Type(), false
				}
			}
		}
	case ast.FieldAccessNode:
		goRecv, addr := tc.goTypeInfoForExpression(e.Target)
		if goRecv != nil {
			obj, _, _ := types.LookupFieldOrMethod(goRecv, false, nil, string(e.Field.ID))
			if obj != nil {
				return obj.Type(), addr
			}
		}
	case ast.SliceExpressionNode:
		if goT := tc.goTypeForExpression(e.Target); goT != nil {
			switch u := goT.Underlying().(type) {
			case *types.Basic:
				if u.Info()&types.IsString != 0 {
					return goT, false
				}
			case *types.Slice:
				return types.NewSlice(u.Elem()), false
			case *types.Array:
				return types.NewSlice(u.Elem()), false
			}
		}
	case ast.IndexExpressionNode:
		if goT := tc.goTypeForExpression(e.Target); goT != nil {
			switch u := goT.Underlying().(type) {
			case *types.Slice:
				return u.Elem(), true
			case *types.Array:
				return u.Elem(), true
			case *types.Map:
				return u.Elem(), false
			}
		}
	case ast.ReferenceNode:
		if inner := tc.goTypeForExpression(e.Value); inner != nil {
			return types.NewPointer(inner), false
		}
	case ast.ShapeNode:
		if e.BaseType != nil {
			if gt := tc.goTypeForQualifiedImportTypeIdent(*e.BaseType); gt != nil {
				return gt, false
			}
			if gt := tc.goNamedTypeForForstIdent(*e.BaseType); gt != nil {
				return gt, false
			}
		}
	}
	return nil, false
}

func (tc *TypeChecker) checkGoMethodCall(recv types.Type, method ast.Ident, e ast.FunctionCallNode, argTypes [][]ast.TypeNode, wantSingleValue bool) ([]ast.TypeNode, error) {
	return tc.checkGoMethodCallAddr(recv, true, method, e, argTypes, wantSingleValue)
}

func (tc *TypeChecker) checkGoMethodCallAddr(recv types.Type, addressable bool, method ast.Ident, e ast.FunctionCallNode, argTypes [][]ast.TypeNode, wantSingleValue bool) ([]ast.TypeNode, error) {
	methodName := string(method.ID)
	if methodName == "" {
		methodName = string(e.Function.ID)
	}
	if i := strings.LastIndex(methodName, "."); i >= 0 {
		methodName = methodName[i+1:]
	}
	return gointerop.CheckMethodCall(tc.goInteropHost(), tc.goInteropDiag(), gointerop.MethodCall{
		Recv:            recv,
		MethodName:      methodName,
		Method:          method,
		Call:            e,
		ArgTypes:        argTypes,
		WantSingleValue: wantSingleValue,
		Addressable:     addressable,
	})
}

func (tc *TypeChecker) bindVariableGoTypesFromCall(assign ast.AssignmentNode) {
	if len(assign.RValues) != 1 {
		return
	}
	if fc, ok := assign.RValues[0].(ast.FunctionCallNode); ok {
		if gt := tc.goTypeFromBuiltinNewCall(fc); gt != nil && len(assign.LValues) == 1 {
			if vn, ok := assign.LValues[0].(ast.VariableNode); ok {
				tc.bindVariableGoType(vn.Ident.ID, &vn, gt)
			}
			return
		}
		if sig := tc.goFuncSignatureFromCall(fc); sig != nil && sig.Results().Len() == len(assign.LValues) {
			res := sig.Results()
			for i, lv := range assign.LValues {
				vn, ok := lv.(ast.VariableNode)
				if !ok {
					continue
				}
				tc.bindVariableGoType(vn.Ident.ID, &vn, res.At(i).Type())
			}
			return
		}
		if sig := tc.goMethodSignatureFromDottedCall(fc); sig != nil && sig.Results().Len() == len(assign.LValues) {
			res := sig.Results()
			for i, lv := range assign.LValues {
				vn, ok := lv.(ast.VariableNode)
				if !ok {
					continue
				}
				tc.bindVariableGoType(vn.Ident.ID, &vn, res.At(i).Type())
			}
			return
		}
		return
	}
	if mc, ok := assign.RValues[0].(ast.MethodCallNode); ok {
		sig := tc.goMethodSignatureFromCall(mc)
		if sig != nil && sig.Results().Len() == len(assign.LValues) {
			res := sig.Results()
			for i, lv := range assign.LValues {
				vn, ok := lv.(ast.VariableNode)
				if !ok {
					continue
				}
				tc.bindVariableGoType(vn.Ident.ID, &vn, res.At(i).Type())
			}
			return
		}
	}
	if len(assign.LValues) != 1 {
		return
	}
	vn, ok := assign.LValues[0].(ast.VariableNode)
	if !ok {
		return
	}
	if gt := tc.goTypeForExpression(assign.RValues[0]); gt != nil {
		tc.bindVariableGoType(vn.Ident.ID, &vn, gt)
	}
}

func (tc *TypeChecker) goMethodSignatureFromCall(mc ast.MethodCallNode) *types.Signature {
	goRecv, addr := tc.goTypeInfoForExpression(mc.Receiver)
	if goRecv == nil {
		return nil
	}
	obj, _, _ := types.LookupFieldOrMethod(goRecv, addr, nil, string(mc.Method.ID))
	fn, ok := obj.(*types.Func)
	if !ok {
		return nil
	}
	sig, _ := fn.Type().(*types.Signature)
	return sig
}

func (tc *TypeChecker) goMethodSignatureFromDottedCall(fc ast.FunctionCallNode) *types.Signature {
	parts := strings.Split(string(fc.Function.ID), ".")
	if len(parts) != 2 {
		return nil
	}
	goRecv := tc.goTypeForVariableIdent(ast.Identifier(parts[0]))
	if goRecv == nil {
		return nil
	}
	obj, _, _ := types.LookupFieldOrMethod(goRecv, true, nil, parts[1])
	fn, ok := obj.(*types.Func)
	if !ok {
		return nil
	}
	sig, _ := fn.Type().(*types.Signature)
	return sig
}

// goTypeFromBuiltinNewCall returns *T when call is new(T) and T maps to a Go type.
func (tc *TypeChecker) goTypeFromBuiltinNewCall(fc ast.FunctionCallNode) types.Type {
	if string(fc.Function.ID) != "new" || len(fc.Arguments) != 1 {
		return nil
	}
	te, ok := fc.Arguments[0].(ast.TypeExpressionNode)
	if !ok {
		return nil
	}
	elem := tc.goTypeForForstType(te.Type)
	if elem == nil {
		return nil
	}
	return types.NewPointer(elem)
}

func (tc *TypeChecker) goFuncSignatureFromCall(fc ast.FunctionCallNode) *types.Signature {
	parts := strings.Split(string(fc.Function.ID), ".")
	switch len(parts) {
	case 2:
		gp := tc.goPackageForImportLocal(parts[0])
		if gp == nil {
			return nil
		}
		return tc.goFuncSignatureInPackage(gp, parts[1])
	case 1:
		if tc.samePackageGo == nil {
			return nil
		}
		return tc.goFuncSignatureInPackage(tc.samePackageGo, parts[0])
	default:
		return nil
	}
}

func (tc *TypeChecker) goFuncSignatureInPackage(pkg *types.Package, funcName string) *types.Signature {
	obj := pkg.Scope().Lookup(funcName)
	if obj == nil {
		return nil
	}
	fn, ok := obj.(*types.Func)
	if !ok {
		return nil
	}
	sig, ok := fn.Type().(*types.Signature)
	if !ok {
		return nil
	}
	return sig
}




