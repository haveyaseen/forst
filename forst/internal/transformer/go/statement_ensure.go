package transformergo

import (
	"fmt"
	"forst/internal/ast"
	goast "go/ast"
	"go/token"

	logrus "github.com/sirupsen/logrus"
)

func (t *Transformer) transformEnsureStatement(ensureNode ast.EnsureNode, originalNode ast.Node) (goast.Stmt, error) {
	fnNode, err := t.closestFunction()
	if err != nil {
		return nil, fmt.Errorf("could not find enclosing function for EnsureNode: %w", err)
	}
	fn, ok := fnNode.(ast.FunctionNode)
	if !ok {
		return nil, fmt.Errorf("enclosing node is not a FunctionNode")
	}
	t.functionsWithEnsure[string(fn.Ident.ID)] = true
	if t.log != nil {
		t.log.WithFields(logrus.Fields{
			"function": "transformStatement",
			"action":   "tracking function with ensure",
			"fnName":   string(fn.Ident.ID),
		}).Debug("[PINPOINT] Function has ensure statement")
	}

	if t.log != nil {
		t.log.WithFields(logrus.Fields{
			"function": "transformStatement",
			"stmtType": "EnsureNode",
			"stmt":     ensureNode.String(),
		}).Debug("[PINPOINT] Processing EnsureNode")
	}
	if t.log != nil {
		t.log.WithFields(logrus.Fields{
			"function":           "transformStatement",
			"scopeBeforeRestore": fmt.Sprintf("%v", t.currentScope()),
			"nodeType":           fmt.Sprintf("%T", originalNode),
		}).Debug("[DEBUG] Before restoreScope(EnsureNode)")
	}
	if err := t.restoreScope(originalNode); err != nil {
		return nil, fmt.Errorf("failed to restore ensure statement scope: %s", err)
	}

	ensureNode, err = t.specializeEnsureForEmit(ensureNode)
	if err != nil {
		return nil, err
	}

	if ensureNode.IsCallSubject() {
		return t.transformEnsureCallSubject(fn, ensureNode)
	}

	stmts, err := t.transformEnsureCondition(&ensureNode)
	if err != nil {
		return nil, err
	}
	if len(stmts) == 0 {
		return nil, fmt.Errorf("transformEnsureCondition returned no statements")
	}

	exprStmt, ok := stmts[0].(*goast.ExprStmt)
	if !ok {
		return nil, fmt.Errorf("expected ExprStmt from transformEnsureCondition, got %T", stmts[0])
	}

	// Builtin, type-guard, Ok/Err, and Meet/Join expressions all use success polarity.
	// Ensure fails when the success condition is false.
	finalCondition := negateCondition(exprStmt.X)

	finallyStmts := []goast.Stmt{}
	errorStmt := t.transformErrorStatement(fn, ensureNode)
	if ensureNode.Block != nil {
		if err := t.restoreScope(ensureNode.Block); err != nil {
			return nil, fmt.Errorf("failed to restore ensure block scope: %w", err)
		}
		for _, blockStatement := range ensureNode.Block.Body {
			if _, isReturn := blockStatement.(ast.ReturnNode); isReturn {
				continue
			}
			goStmt, err := t.transformStatement(blockStatement)
			if err != nil {
				return nil, err
			}
			if goStmt != nil {
				finallyStmts = append(finallyStmts, goStmt)
			}
		}
	}

	return &goast.IfStmt{
		Cond: finalCondition,
		Body: &goast.BlockStmt{List: append(finallyStmts, errorStmt)},
	}, nil
}

// transformErrorStatement converts an ensure statement to an error return statement
func (t *Transformer) transformErrorStatement(fn ast.FunctionNode, stmt ast.EnsureNode) goast.Stmt {
	// PINPOINT: Log when this function is called
	if t.log != nil {
		t.log.WithFields(logrus.Fields{
			"function": "transformErrorStatement",
			"stmt":     stmt.String(),
		}).Debug("[PINPOINT] transformErrorStatement called")
	}

	functionName := string(fn.Ident.ID)

	// Get the inferred function signature from the typechecker
	var returnTypes []ast.TypeNode
	if sig, exists := t.TypeChecker.Functions[fn.Ident.ID]; exists {
		returnTypes = sig.ReturnTypes
		// PINPOINT: Log the function signature found
		if t.log != nil {
			t.log.WithFields(logrus.Fields{
				"function":     "transformErrorStatement",
				"functionName": functionName,
				"found":        true,
				"returnTypes":  returnTypes,
			}).Debug("[PINPOINT] Found function signature in typechecker")
		}
	} else {
		// Fallback to the raw AST node return types
		returnTypes = fn.ReturnTypes
		// PINPOINT: Log the fallback
		if t.log != nil {
			t.log.WithFields(logrus.Fields{
				"function":     "transformErrorStatement",
				"functionName": functionName,
				"found":        false,
				"returnTypes":  returnTypes,
			}).Debug("[PINPOINT] Using fallback return types from AST")
		}
	}

	// PINPOINT: Log function signature for error return
	if t.log != nil {
		t.log.WithFields(logrus.Fields{
			"function":     "transformErrorStatement",
			"functionName": functionName,
			"returnCount":  len(returnTypes),
			"returnTypes":  returnTypes,
		}).Debug("[PINPOINT] Function signature for error return")
	}

	// PINPOINT: Log each return type for debugging
	for i, retType := range returnTypes {
		if t.log != nil {
			t.log.WithFields(logrus.Fields{
				"function":     "transformErrorStatement",
				"functionName": functionName,
				"returnIndex":  i,
				"returnType":   retType.Ident,
				"typeKind":     retType.TypeKind,
				"isError":      retType.IsError(),
			}).Debug("[PINPOINT] Processing return type in transformErrorStatement")
		}
	}

	// For main function, print failure context then os.Exit(1).
	if t.isMainFunction() {
		return t.mainEnsureExitStmt(stmt)
	}

	if t.isTestFunction() {
		paramID, ok := t.testingTParamIdent(fn)
		if ok {
			t.Output.EnsureImport("testing")
			testIdent := goast.NewIdent(string(paramID))
			helperCall := &goast.ExprStmt{
				X: &goast.CallExpr{
					Fun: &goast.SelectorExpr{
						X:   testIdent,
						Sel: goast.NewIdent("Helper"),
					},
				},
			}
			if stmt.Error != nil {
				if errExpr, err := t.transformEnsureErrorFallback(*stmt.Error); err == nil {
					fatalCall := testFatalfCall(testIdent, "%v", errExpr)
					return &goast.BlockStmt{List: []goast.Stmt{helperCall, fatalCall}}
				}
			}
			fatalCall := t.ensureTestFatalfCall(testIdent, stmt)
			return &goast.BlockStmt{List: []goast.Stmt{helperCall, fatalCall}}
		}
	}

	// For void functions or functions with no return values, use panic (or invoke custom fallback).
	if len(returnTypes) == 0 || (len(returnTypes) == 1 && returnTypes[0].Ident == ast.TypeVoid) {
		if stmt.Error != nil {
			errExpr, err := t.transformEnsureErrorFallback(*stmt.Error)
			if err != nil {
				return &goast.ExprStmt{
					X: &goast.CallExpr{
						Fun: goast.NewIdent("panic"),
						Args: []goast.Expr{
							&goast.BasicLit{Kind: token.STRING, Value: "\"assertion failed\""},
						},
					},
				}
			}
			return &goast.AssignStmt{
				Tok: token.ASSIGN,
				Lhs: []goast.Expr{goast.NewIdent("_")},
				Rhs: []goast.Expr{errExpr},
			}
		}
		return &goast.ExprStmt{
			X: &goast.CallExpr{
				Fun: goast.NewIdent("panic"),
				Args: []goast.Expr{
					&goast.BasicLit{
						Kind:  token.STRING,
						Value: "\"assertion failed\"",
					},
				},
			},
		}
	}

	// Build error return values based on the function's return types
	// Result(S, Error) is one Forst return type but lowers to (S, error) in Go.
	// Result(Void, Error) lowers to a single error return.
	if len(returnTypes) == 1 && returnTypes[0].IsResultType() && len(returnTypes[0].TypeParams) >= 2 {
		succT := returnTypes[0].TypeParams[0]
		errExpr, err := t.ensureFailureErrorExpr(stmt)
		if err != nil {
			errExpr = t.defaultAssertionErrorExpr(stmt)
		}
		if succT.Ident == ast.TypeVoid {
			return &goast.ReturnStmt{
				Results: []goast.Expr{errExpr},
			}
		}
		zeroSucc, err := t.zeroValueExprForASTType(succT)
		if err != nil {
			zeroSucc = t.buildZeroCompositeLiteral(&succT)
		}
		return &goast.ReturnStmt{
			Results: []goast.Expr{zeroSucc, errExpr},
		}
	}

	results := make([]goast.Expr, 0, len(returnTypes))
	for i, returnType := range returnTypes {
		// PINPOINT: Log processing return type for zero value
		if t.log != nil {
			t.log.WithFields(logrus.Fields{
				"function":     "transformErrorStatement",
				"functionName": functionName,
				"returnIndex":  i,
				"returnType":   returnType.Ident,
				"typeKind":     returnType.TypeKind,
			}).Debug("[PINPOINT] Processing return type for zero value")
		}

		var result goast.Expr
		if returnType.IsError() {
			errExpr, err := t.ensureFailureErrorExpr(stmt)
			if err != nil {
				errExpr = t.defaultAssertionErrorExpr(stmt)
			}
			result = errExpr
		} else {
			// For non-error types, always use the function's declared return type (including hash-based types)
			// Never structurally alias to a user-defined type for error returns
			if returnType.TypeKind == ast.TypeKindUserDefined || returnType.TypeKind == ast.TypeKindHashBased {
				// PINPOINT: Log when calling buildZeroCompositeLiteral for user-defined or hash-based type
				if t.log != nil {
					t.log.WithFields(logrus.Fields{
						"function":     "transformErrorStatement",
						"functionName": functionName,
						"returnIndex":  i,
						"returnType":   returnType.Ident,
						"typeKind":     returnType.TypeKind,
					}).Debug("[PINPOINT] Calling buildZeroCompositeLiteral for user-defined or hash-based type")
				}
				result = t.buildZeroCompositeLiteral(&returnType)
			} else {
				zv, err := t.zeroValueExprForASTType(returnType)
				if err != nil {
					goType, _ := t.transformType(returnType)
					result = getZeroValue(goType)
				} else {
					result = zv
				}
			}
		}
		results = append(results, result)
	}

	return &goast.ReturnStmt{
		Results: results,
	}
}

// mainEnsureExitStmt emits stderr diagnostics before os.Exit(1) for failed ensure in main.
func (t *Transformer) mainEnsureExitStmt(stmt ast.EnsureNode) goast.Stmt {
	t.Output.EnsureImport("os")

	exitCall := &goast.ExprStmt{
		X: &goast.CallExpr{
			Fun: &goast.SelectorExpr{
				X:   goast.NewIdent("os"),
				Sel: goast.NewIdent("Exit"),
			},
			Args: []goast.Expr{
				&goast.BasicLit{
					Kind:  token.INT,
					Value: "1",
				},
			},
		},
	}

	var errExpr goast.Expr
	if t.resultLocalSplit != nil {
		if split, ok := t.resultLocalSplit[string(stmt.Variable.Ident.ID)]; ok && split.errGoName != "" {
			errExpr = goast.NewIdent(split.errGoName)
		}
	}
	if errExpr == nil {
		if e, err := t.ensureFailureErrorExpr(stmt); err == nil {
			errExpr = e
		}
	}
	if errExpr == nil {
		return exitCall
	}

	t.Output.EnsureImport("fmt")
	fprintfCall := &goast.ExprStmt{
		X: &goast.CallExpr{
			Fun: &goast.SelectorExpr{
				X:   goast.NewIdent("fmt"),
				Sel: goast.NewIdent("Fprintf"),
			},
			Args: []goast.Expr{
				&goast.SelectorExpr{
					X:   goast.NewIdent("os"),
					Sel: goast.NewIdent("Stderr"),
				},
				goQuotedStringLit("ensure failed: %v\n"),
				errExpr,
			},
		},
	}
	return &goast.BlockStmt{List: []goast.Stmt{fprintfCall, exitCall}}
}

// transformEnsureCallSubject lowers `ensure need(ok)` / `ensure need(ok) is Ok()` and similar
// fire-and-forget call subjects. Result Ok/Err uses an if-Init so the call runs once.
func (t *Transformer) transformEnsureCallSubject(fn ast.FunctionNode, ensure ast.EnsureNode) (goast.Stmt, error) {
	subjectType, err := t.lookupEnsureSubjectTypeForEmit(ensure)
	if err != nil {
		return nil, fmt.Errorf("ensure call subject type: %w", err)
	}
	if t.log != nil {
		t.log.WithFields(logrus.Fields{
			"function":    "transformEnsureCallSubject",
			"subject":     ensure.Subject.String(),
			"subjectType": subjectType.Ident,
			"assertion":   ensure.Assertion.String(),
		}).Debug("emitting ensure with call subject")
	}

	if subjectType.IsResultType() && (ensureIsOnlyOkAssertion(ensure) || ensureIsOnlyErrAssertion(ensure)) {
		return t.transformEnsureResultCallSubject(fn, ensure, subjectType)
	}

	// Bool / type-guard / other constraints: condition uses the call expression directly.
	stmts, err := t.transformEnsureConditionForCall(&ensure, subjectType)
	if err != nil {
		return nil, err
	}
	if len(stmts) == 0 {
		return nil, fmt.Errorf("transformEnsureConditionForCall returned no statements")
	}
	exprStmt, ok := stmts[0].(*goast.ExprStmt)
	if !ok {
		return nil, fmt.Errorf("expected ExprStmt from transformEnsureConditionForCall, got %T", stmts[0])
	}
	finalCondition := negateCondition(exprStmt.X)
	finallyStmts := []goast.Stmt{}
	errorStmt := t.transformErrorStatement(fn, ensure)
	if ensure.Block != nil {
		if err := t.restoreScope(ensure.Block); err != nil {
			return nil, fmt.Errorf("failed to restore ensure block scope: %w", err)
		}
		for _, blockStatement := range ensure.Block.Body {
			if _, isReturn := blockStatement.(ast.ReturnNode); isReturn {
				continue
			}
			goStmt, err := t.transformStatement(blockStatement)
			if err != nil {
				return nil, err
			}
			if goStmt != nil {
				finallyStmts = append(finallyStmts, goStmt)
			}
		}
	}
	return &goast.IfStmt{
		Cond: finalCondition,
		Body: &goast.BlockStmt{List: append(finallyStmts, errorStmt)},
	}, nil
}

func ensureIsOnlyErrAssertion(stmt ast.EnsureNode) bool {
	if stmt.Assertion.BaseType != nil || len(stmt.Assertion.Constraints) != 1 {
		return false
	}
	c := stmt.Assertion.Constraints[0]
	return c.Name == "Err" && len(c.Args) == 0
}

// transformEnsureResultCallSubject emits:
//
//	if err := need(ok); err != nil { return …, err }           // Result(Void) / Ok
//	if _, err := fetch(); err != nil { return …, err }         // Result(T) / Ok
//	if err := need(ok); err == nil { … }                      // Result(Void) / Err
func (t *Transformer) transformEnsureResultCallSubject(fn ast.FunctionNode, ensure ast.EnsureNode, subjectType ast.TypeNode) (goast.Stmt, error) {
	callExpr, err := t.transformExpression(ensure.Subject)
	if err != nil {
		return nil, fmt.Errorf("ensure call subject: %w", err)
	}
	errIdent := goast.NewIdent("err")
	var init goast.Stmt
	succT := subjectType.TypeParams[0]
	if succT.Ident == ast.TypeVoid {
		init = &goast.AssignStmt{
			Lhs: []goast.Expr{errIdent},
			Tok: token.DEFINE,
			Rhs: []goast.Expr{callExpr},
		}
	} else {
		lhs := []goast.Expr{}
		if succT.IsTupleType() {
			for range succT.TypeParams {
				lhs = append(lhs, goast.NewIdent("_"))
			}
		} else {
			lhs = append(lhs, goast.NewIdent("_"))
		}
		lhs = append(lhs, errIdent)
		init = &goast.AssignStmt{
			Lhs: lhs,
			Tok: token.DEFINE,
			Rhs: []goast.Expr{callExpr},
		}
	}

	var cond goast.Expr
	if ensureIsOnlyOkAssertion(ensure) {
		// Failure when err != nil (success polarity of Ok is err == nil).
		cond = &goast.BinaryExpr{X: errIdent, Op: token.NEQ, Y: goast.NewIdent("nil")}
	} else {
		// ensure … is Err(): failure when err == nil.
		cond = &goast.BinaryExpr{X: errIdent, Op: token.EQL, Y: goast.NewIdent("nil")}
	}

	finallyStmts := []goast.Stmt{}
	errorStmt := t.transformErrorStatement(fn, ensure)
	if ensure.Block != nil {
		if err := t.restoreScope(ensure.Block); err != nil {
			return nil, fmt.Errorf("failed to restore ensure block scope: %w", err)
		}
		for _, blockStatement := range ensure.Block.Body {
			if _, isReturn := blockStatement.(ast.ReturnNode); isReturn {
				continue
			}
			goStmt, err := t.transformStatement(blockStatement)
			if err != nil {
				return nil, err
			}
			if goStmt != nil {
				finallyStmts = append(finallyStmts, goStmt)
			}
		}
	}
	return &goast.IfStmt{
		Init: init,
		Cond: cond,
		Body: &goast.BlockStmt{List: append(finallyStmts, errorStmt)},
	}, nil
}

// transformEnsureConditionForCall is like transformEnsureCondition but uses the call Subject
// and a pre-resolved subject type (no Variable lookup).
func (t *Transformer) transformEnsureConditionForCall(ensure *ast.EnsureNode, varType ast.TypeNode) ([]goast.Stmt, error) {
	t.logAssertionBaseType(ensure)

	result, handled, err := t.handleTypeTargetMembership(ensure, varType)
	if err != nil || handled {
		return result, err
	}

	result, handled, err = t.handleTypeGuardCall(ensure, varType)
	if err != nil {
		return nil, err
	}
	if handled {
		if len(result) == 0 {
			return nil, fmt.Errorf("type guard ensure produced no condition to emit")
		}
		return result, nil
	}

	result, handled, err = t.handleAssertionIR(ensure, varType)
	if err != nil || handled {
		return result, err
	}
	return t.handleMeetChains(ensure, varType)
}
