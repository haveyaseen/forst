package hasher

import (
	"encoding/binary"
	"fmt"
	"forst/internal/ast"
	"hash/fnv"
	"io"
	"math"
	"reflect"
	"sort"
	"strconv"
	"unsafe"
)

// NodeHash is a unique identifier for an AST node
type NodeHash uint64

// StructuralHasher generates structural hashes for AST nodes.
type StructuralHasher struct{}

// New creates a new StructuralHasher
func New() *StructuralHasher {
	return &StructuralHasher{}
}

// NodeKind maps AST node types to unique uint8 identifiers for hashing
var NodeKind = map[string]uint8{
	"BinaryExpression": 1,
	"IntLiteral":       2,
	"FloatLiteral":     3,
	"StringLiteral":    4,
	"RuneLiteral":      36,
	"Variable":         5,
	"UnaryExpression":  6,
	"FunctionCall":     7,
	"BoolLiteral":      8,
	"Function":         9,
	"Ensure":           10,
	"TypeGuard":        11,
	"TypeDefAssertion": 12,
	"If":               13,
	"Reference":        14,
	"MapLiteral":       15,
	"NilLiteral":       16,
	"For":              17,
	"Break":            18,
	"Continue":         19,
	"Goto":             43,
	"LabeledStmt":      44,
	"ElseIf":           20,
	"ElseBlock":        21,
	"Comment":          22,
	"Defer":            23,
	"GoStmt":           24,
	"IndexExpression":  25,
	"OkExpr":           26,
	"ErrExpr":          27,
	"TypeDefErrorExpr": 28,
	"Use":              29,
	"With":             30,
	"MethodCall":       31,
	"SliceExpression":  32,
	"SpreadExpression": 33,
	"FieldAccess":      34,
	"Switch":           37,
	"Fallthrough":      38,
	"TypeExpression":   39,
	"ConstGroup":       40,
	"IotaLiteral":      41,
	"FunctionLiteral":  42,
}

// hashWalk carries per-top-level-HashNode memo state; safe for concurrent HashNode calls.
type hashWalk struct {
	h    *StructuralHasher
	memo map[NodeIdentity]NodeHash
}

func newHashWalk(h *StructuralHasher) *hashWalk {
	return &hashWalk{
		h:    h,
		memo: make(map[NodeIdentity]NodeHash),
	}
}

func (w *hashWalk) hashOptional(wr io.Writer, node ast.Node) error {
	if node == nil {
		return writeHash(wr, NilHash)
	}
	hash, err := w.hash(node)
	if err != nil {
		return err
	}
	return writeHash(wr, hash)
}

// hashNodes generates a structural hash for multiple AST nodes
func (w *hashWalk) hashNodes(nodes []ast.Node) (NodeHash, error) {
	hasher := fnv.New64a()
	for _, node := range nodes {
		hash, err := w.hash(node)
		if err != nil {
			return 0, err
		}
		if err := writeHash(hasher, hash); err != nil {
			return 0, err
		}
	}
	return NodeHash(hasher.Sum64()), nil
}

// writeHash writes data in encoding/binary little-endian form without reflect on the
// types HashNode actually passes (uint8, bool, int64, uint64, NodeHash, float64, []byte).
func writeHash(w io.Writer, data any) error {
	var buf [8]byte
	switch v := data.(type) {
	case uint8:
		buf[0] = v
		if _, err := w.Write(buf[:1]); err != nil {
			return fmt.Errorf("failed to write hash: %v", err)
		}
		return nil
	case bool:
		if v {
			buf[0] = 1
		}
		if _, err := w.Write(buf[:1]); err != nil {
			return fmt.Errorf("failed to write hash: %v", err)
		}
		return nil
	case int64:
		binary.LittleEndian.PutUint64(buf[:], uint64(v))
		if _, err := w.Write(buf[:]); err != nil {
			return fmt.Errorf("failed to write hash: %v", err)
		}
		return nil
	case uint64:
		binary.LittleEndian.PutUint64(buf[:], v)
		if _, err := w.Write(buf[:]); err != nil {
			return fmt.Errorf("failed to write hash: %v", err)
		}
		return nil
	case NodeHash:
		binary.LittleEndian.PutUint64(buf[:], uint64(v))
		if _, err := w.Write(buf[:]); err != nil {
			return fmt.Errorf("failed to write hash: %v", err)
		}
		return nil
	case float64:
		binary.LittleEndian.PutUint64(buf[:], math.Float64bits(v))
		if _, err := w.Write(buf[:]); err != nil {
			return fmt.Errorf("failed to write hash: %v", err)
		}
		return nil
	case []byte:
		if len(v) == 0 {
			return nil
		}
		if _, err := w.Write(v); err != nil {
			return fmt.Errorf("failed to write hash: %v", err)
		}
		return nil
	default:
		if err := binary.Write(w, binary.LittleEndian, data); err != nil {
			return fmt.Errorf("failed to write hash: %v", err)
		}
		return nil
	}
}

// writeHashes writes multiple values to the hasher, handling errors
func (h *StructuralHasher) writeHashes(w io.Writer, values ...any) error {
	for _, v := range values {
		if err := writeHash(w, v); err != nil {
			return err
		}
	}
	return nil
}

// writeHashAndNode is a helper that writes a hash and a node's hash, handling errors
func (w *hashWalk) writeHashAndNode(wr io.Writer, kind uint8, node ast.Node) error {
	if err := writeHash(wr, kind); err != nil {
		return err
	}
	hash, err := w.hash(node)
	if err != nil {
		return err
	}
	return writeHash(wr, hash)
}

// HashNode generates a structural hash for an AST node.
func (h *StructuralHasher) HashNode(node ast.Node) (NodeHash, error) {
	return newHashWalk(h).hash(node)
}

// hash memoizes by node identity within a single top-level HashNode walk.
func (w *hashWalk) hash(node ast.Node) (NodeHash, error) {
	if node == nil || isNilPointer(node) {
		return NodeHash(NilHash), nil
	}
	key, ok := NodeIdentityKey(node)
	if ok {
		if cached, hit := w.memo[key]; hit {
			return cached, nil
		}
	}
	hash, err := w.hashUncached(node)
	if err != nil {
		return 0, err
	}
	if ok {
		w.memo[key] = hash
	}
	return hash, nil
}

// hashUncached generates a structural hash without consulting the walk-local memo.
//
//nolint:errcheck // Many branches hash into an in-memory FNV writer; unchecked writeHashes calls mirror checked ones nearby.
func (w *hashWalk) hashUncached(node ast.Node) (NodeHash, error) {
	hasher := fnv.New64a()

	// Handle nil and typed nils
	if node == nil || (isNilPointer(node)) {
		return NodeHash(NilHash), nil
	}

	switch n := node.(type) {
	case ast.TypeDefAssertionExpr:
		if err := w.h.writeHashes(hasher, NodeKind["TypeDefAssertion"]); err != nil {
			return 0, err
		}
		hash, err := w.hash(n.Assertion)
		if err != nil {
			return 0, err
		}
		if err := w.h.writeHashes(hasher, hash); err != nil {
			return 0, err
		}

	case *ast.TypeDefAssertionExpr:
		return w.hash(*n)

	case ast.TypeDefBinaryExpr:
		if err := w.h.writeHashes(hasher,
			NodeKind["TypeDefBinaryExpr"],
			w.h.HashTokenType(n.Op),
		); err != nil {
			return 0, err
		}
		leftHash, err := w.hash(n.Left.(ast.Node))
		if err != nil {
			return 0, err
		}
		rightHash, err := w.hash(n.Right.(ast.Node))
		if err != nil {
			return 0, err
		}
		if err := w.h.writeHashes(hasher, leftHash, rightHash); err != nil {
			return 0, err
		}

	case ast.BinaryExpressionNode:
		if err := w.h.writeHashes(hasher,
			NodeKind["BinaryExpression"],
			w.h.HashTokenType(n.Operator),
		); err != nil {
			return 0, err
		}
		leftHash, err := w.hash(n.Left)
		if err != nil {
			return 0, err
		}
		rightHash, err := w.hash(n.Right)
		if err != nil {
			return 0, err
		}
		if err := w.h.writeHashes(hasher, leftHash, rightHash); err != nil {
			return 0, err
		}

	case ast.UnaryExpressionNode:
		if err := w.h.writeHashes(hasher,
			NodeKind["UnaryExpression"],
			w.h.HashTokenType(n.Operator),
		); err != nil {
			return 0, err
		}
		operandHash, err := w.hash(n.Operand)
		if err != nil {
			return 0, err
		}
		if err := w.h.writeHashes(hasher, operandHash); err != nil {
			return 0, err
		}

	case ast.IntLiteralNode:
		if err := w.h.writeHashes(hasher,
			NodeKind["IntLiteral"],
			n.Value,
		); err != nil {
			return 0, err
		}

	case ast.BoolLiteralNode:
		if err := w.h.writeHashes(hasher,
			NodeKind["BoolLiteral"],
			n.Value,
		); err != nil {
			return 0, err
		}

	case ast.FloatLiteralNode:
		if err := w.h.writeHashes(hasher,
			NodeKind["FloatLiteral"],
			n.Value,
		); err != nil {
			return 0, err
		}

	case ast.StringLiteralNode:
		if err := w.h.writeHashes(hasher,
			NodeKind["StringLiteral"],
			[]byte(n.Value),
		); err != nil {
			return 0, err
		}

	case ast.RuneLiteralNode:
		if err := w.h.writeHashes(hasher,
			NodeKind["RuneLiteral"],
			n.Value,
		); err != nil {
			return 0, err
		}

	case ast.VariableNode:
		if err := w.h.writeHashes(hasher,
			NodeKind["Variable"],
			[]byte(n.Ident.ID),
		); err != nil {
			return 0, err
		}

	case ast.FunctionNode:
		if err := w.h.writeHashes(hasher, NodeKind["Function"]); err != nil {
			return 0, err
		}
		hash, err := w.hashNodes(n.Body)
		if err != nil {
			return 0, err
		}
		if err := w.h.writeHashes(hasher, hash); err != nil {
			return 0, err
		}

	case ast.FunctionLiteralNode:
		if err := w.h.writeHashes(hasher, NodeKind["FunctionLiteral"]); err != nil {
			return 0, err
		}
		params := make([]ast.Node, len(n.Params))
		for i, p := range n.Params {
			params[i] = p
		}
		paramHash, err := w.hashNodes(params)
		if err != nil {
			return 0, err
		}
		if err := w.h.writeHashes(hasher, paramHash); err != nil {
			return 0, err
		}
		for _, rt := range n.ReturnTypes {
			rtHash, err := w.hash(rt)
			if err != nil {
				return 0, err
			}
			if err := w.h.writeHashes(hasher, rtHash); err != nil {
				return 0, err
			}
		}
		bodyHash, err := w.hashNodes(n.Body)
		if err != nil {
			return 0, err
		}
		if err := w.h.writeHashes(hasher, bodyHash); err != nil {
			return 0, err
		}

	case ast.FunctionCallNode:
		if n.Callee != nil {
			if err := w.h.writeHashes(hasher, NodeKind["FunctionCall"]); err != nil {
				return 0, err
			}
			calleeHash, err := w.hash(n.Callee)
			if err != nil {
				return 0, err
			}
			if err := w.h.writeHashes(hasher, calleeHash); err != nil {
				return 0, err
			}
		} else if err := w.h.writeHashes(hasher,
			NodeKind["FunctionCall"],
			[]byte(n.Function.ID),
		); err != nil {
			return 0, err
		}
		nodes := make([]ast.Node, len(n.Arguments))
		for i, arg := range n.Arguments {
			nodes[i] = arg
		}
		hash, err := w.hashNodes(nodes)
		if err != nil {
			return 0, err
		}
		if err := w.h.writeHashes(hasher, hash); err != nil {
			return 0, err
		}

	case ast.MethodCallNode:
		if err := w.h.writeHashes(hasher, NodeKind["MethodCall"], []byte(n.Method.ID)); err != nil {
			return 0, err
		}
		recvHash, err := w.hash(n.Receiver)
		if err != nil {
			return 0, err
		}
		if err := w.h.writeHashes(hasher, recvHash); err != nil {
			return 0, err
		}
		nodes := make([]ast.Node, len(n.Arguments))
		for i, arg := range n.Arguments {
			nodes[i] = arg
		}
		hash, err := w.hashNodes(nodes)
		if err != nil {
			return 0, err
		}
		if err := w.h.writeHashes(hasher, hash); err != nil {
			return 0, err
		}

	case ast.IndexExpressionNode:
		if err := w.h.writeHashes(hasher, NodeKind["IndexExpression"]); err != nil {
			return 0, err
		}
		th, err := w.hash(n.Target)
		if err != nil {
			return 0, err
		}
		ih, err := w.hash(n.Index)
		if err != nil {
			return 0, err
		}
		if err := w.h.writeHashes(hasher, th, ih); err != nil {
			return 0, err
		}

	case ast.SliceExpressionNode:
		if err := w.h.writeHashes(hasher, NodeKind["SliceExpression"]); err != nil {
			return 0, err
		}
		th, err := w.hash(n.Target)
		if err != nil {
			return 0, err
		}
		if err := w.h.writeHashes(hasher, th); err != nil {
			return 0, err
		}
		if err := w.hashOptional(hasher, n.Low); err != nil {
			return 0, err
		}
		if err := w.hashOptional(hasher, n.High); err != nil {
			return 0, err
		}

	case ast.SpreadExpressionNode:
		if err := w.h.writeHashes(hasher, NodeKind["SpreadExpression"]); err != nil {
			return 0, err
		}
		eh, err := w.hash(n.Expr)
		if err != nil {
			return 0, err
		}
		if err := w.h.writeHashes(hasher, eh); err != nil {
			return 0, err
		}

	case ast.FieldAccessNode:
		if err := w.h.writeHashes(hasher, NodeKind["FieldAccess"], []byte(n.Field.ID)); err != nil {
			return 0, err
		}
		th, err := w.hash(n.Target)
		if err != nil {
			return 0, err
		}
		if err := w.h.writeHashes(hasher, th); err != nil {
			return 0, err
		}

	case ast.EnsureNode:
		if err := w.h.writeHashes(hasher, NodeKind["Ensure"]); err != nil {
			return 0, err
		}
		// Subject variable must participate in the hash; otherwise distinct ensures
		// with the same assertion (e.g. `ensure a.name is Min(1)` vs `ensure b.name is Min(1)`)
		// collide in scopeStack.scopes and restoreScope picks the wrong Ensure scope.
		vh, err := w.hash(n.Variable)
		if err != nil {
			return 0, err
		}
		if err := w.h.writeHashes(hasher, vh); err != nil {
			return 0, err
		}
		if err := w.h.writeHashes(hasher, uint8(n.Implicit)); err != nil {
			return 0, err
		}
		hash, err := w.hash(n.Assertion)
		if err != nil {
			return 0, err
		}
		if err := w.h.writeHashes(hasher, hash); err != nil {
			return 0, err
		}
		if n.Error != nil {
			if err := w.h.writeHashes(hasher, []byte((*n.Error).String())); err != nil {
				return 0, err
			}
		}
		if n.Block != nil {
			hash, err := w.hashNodes(n.Block.Body)
			if err != nil {
				return 0, err
			}
			if err := w.h.writeHashes(hasher, hash); err != nil {
				return 0, err
			}
		}

	case ast.ShapeNode:
		if err := w.h.writeHashes(hasher, NodeKind["Shape"]); err != nil {
			return 0, err
		}
		// Convert map to sorted slice of fields for deterministic ordering
		fields := make([]struct {
			name  string
			field ast.ShapeFieldNode
		}, 0, len(n.Fields))
		for name, field := range n.Fields {
			fields = append(fields, struct {
				name  string
				field ast.ShapeFieldNode
			}{name, field})
		}
		sort.Slice(fields, func(i, j int) bool {
			return fields[i].name < fields[j].name
		})
		// Hash each field in sorted order
		for _, f := range fields {
			if err := w.h.writeHashes(hasher, []byte(f.name)); err != nil {
				return 0, err
			}
			hash, err := w.hash(f.field)
			if err != nil {
				return 0, err
			}
			if err := w.h.writeHashes(hasher, hash); err != nil {
				return 0, err
			}
		}

	case ast.ShapeFieldNode:
		if err := w.h.writeHashes(hasher, NodeKind["ShapeField"]); err != nil {
			return 0, err
		}
		if n.Embedded {
			if err := w.h.writeHashes(hasher, []byte{1}); err != nil {
				return 0, err
			}
		}
		if n.Tag != "" {
			if err := w.h.writeHashes(hasher, []byte(n.Tag)); err != nil {
				return 0, err
			}
		}
		if n.Assertion != nil {
			hash, err := w.hash(*n.Assertion)
			if err != nil {
				return 0, err
			}
			if err := w.h.writeHashes(hasher, hash); err != nil {
				return 0, err
			}
		}
		if n.Shape != nil {
			hash, err := w.hash(*n.Shape)
			if err != nil {
				return 0, err
			}
			if err := w.h.writeHashes(hasher, hash); err != nil {
				return 0, err
			}
		}

	case ast.AssertionNode:
		if err := w.h.writeHashes(hasher, NodeKind["Assertion"]); err != nil {
			return 0, err
		}
		if n.BaseType != nil {
			if err := w.h.writeHashes(hasher, []byte(*n.BaseType)); err != nil {
				return 0, err
			}
		}
		for _, tp := range n.TypeParams {
			hash, err := w.hash(tp)
			if err != nil {
				return 0, err
			}
			if err := w.h.writeHashes(hasher, hash); err != nil {
				return 0, err
			}
		}
		if n.ArrayLen != nil {
			if err := w.h.writeHashes(hasher, []byte(fmt.Sprintf("%d", *n.ArrayLen))); err != nil {
				return 0, err
			}
		}
		// Sort constraints for deterministic ordering
		constraints := make([]ast.ConstraintNode, len(n.Constraints))
		copy(constraints, n.Constraints)
		sort.Slice(constraints, func(i, j int) bool {
			return constraints[i].Name < constraints[j].Name
		})
		// Hash each constraint in sorted order
		for _, constraint := range constraints {
			hash, err := w.hash(constraint)
			if err != nil {
				return 0, err
			}
			if err := w.h.writeHashes(hasher, hash); err != nil {
				return 0, err
			}
		}
		for _, alt := range n.OrChains {
			hash, err := w.hash(alt)
			if err != nil {
				return 0, err
			}
			if err := w.h.writeHashes(hasher, hash); err != nil {
				return 0, err
			}
		}

	case ast.ConstraintNode:
		if err := w.h.writeHashes(hasher, NodeKind["Constraint"]); err != nil {
			return 0, err
		}
		if err := w.h.writeHashes(hasher, []byte(n.Name)); err != nil {
			return 0, err
		}
		nodes := make([]ast.Node, len(n.Args))
		for i, arg := range n.Args {
			nodes[i] = arg
		}
		hash, err := w.hashNodes(nodes)
		if err != nil {
			return 0, err
		}
		if err := w.h.writeHashes(hasher, hash); err != nil {
			return 0, err
		}

	case ast.ConstraintArgumentNode:
		if err := w.h.writeHashes(hasher, NodeKind["ConstraintArgument"]); err != nil {
			return 0, err
		}
		if n.Value != nil {
			hash, err := w.hash(*n.Value)
			if err != nil {
				return 0, err
			}
			if err := w.h.writeHashes(hasher, hash); err != nil {
				return 0, err
			}
		}
		if n.Shape != nil {
			hash, err := w.hash(*n.Shape)
			if err != nil {
				return 0, err
			}
			if err := w.h.writeHashes(hasher, hash); err != nil {
				return 0, err
			}
		}
	case ast.PackageNode:
		w.h.writeHashes(hasher, NodeKind["Package"])
		w.h.writeHashes(hasher, []byte(n.Ident.ID))
	case ast.ImportNode:
		w.h.writeHashes(hasher, NodeKind["Import"])
		w.h.writeHashes(hasher, []byte(n.Path))
		if n.Alias != nil {
			w.h.writeHashes(hasher, []byte(n.Alias.ID))
		}
	case ast.TypeDefNode:
		if n.Ident != "" {
			// For named types, hash only the identifier
			w.h.writeHashes(hasher, NodeKind["TypeDef"])
			w.h.writeHashes(hasher, []byte(n.Ident))
			break
		}
		w.h.writeHashes(hasher, NodeKind["TypeDef"])
		w.h.writeHashes(hasher, []byte(n.Expr.String()))
		w.h.writeHashes(hasher, []byte(n.Ident))
	case ast.ReturnNode:
		w.h.writeHashes(hasher, NodeKind["Return"])
		// Hash all return values
		for _, value := range n.Values {
			hash, err := w.hash(value)
			if err != nil {
				return 0, err
			}
			w.h.writeHashes(hasher, hash)
		}
	case ast.TypeNode:
		w.h.writeHashes(hasher, NodeKind["Type"])
		w.h.writeHashes(hasher, []byte(n.Ident))
		if n.Ident == ast.TypeArray && n.ArrayLen != nil {
			w.h.writeHashes(hasher, []byte(strconv.FormatInt(*n.ArrayLen, 10)))
		}
		for _, tp := range n.TypeParams {
			hash, err := w.hash(tp)
			if err != nil {
				return 0, err
			}
			if err := w.h.writeHashes(hasher, hash); err != nil {
				return 0, err
			}
		}
	case ast.SimpleParamNode:
		w.h.writeHashes(hasher, NodeKind["SimpleParam"])
		w.h.writeHashes(hasher, []byte(n.Ident.ID))
		hash, err := w.hash(n.Type)
		if err != nil {
			return 0, err
		}
		w.h.writeHashes(hasher, hash)
	case ast.DestructuredParamNode:
		w.h.writeHashes(hasher, NodeKind["DestructuredParam"])
		// Sort fields for deterministic ordering
		fields := make([]string, len(n.Fields))
		copy(fields, n.Fields)
		sort.Strings(fields)
		for _, field := range fields {
			w.h.writeHashes(hasher, []byte(field))
		}
		hash, err := w.hash(n.Type)
		if err != nil {
			return 0, err
		}
		w.h.writeHashes(hasher, hash)
	case *ast.AssertionNode:
		w.h.writeHashes(hasher, NodeKind["Assertion"])
		if n.BaseType != nil {
			w.h.writeHashes(hasher, []byte(*n.BaseType))
		}
		for _, tp := range n.TypeParams {
			hash, err := w.hash(tp)
			if err != nil {
				return 0, err
			}
			w.h.writeHashes(hasher, hash)
		}
		if n.ArrayLen != nil {
			w.h.writeHashes(hasher, []byte(fmt.Sprintf("%d", *n.ArrayLen)))
		}
		// Sort constraints for deterministic ordering
		constraints := make([]ast.ConstraintNode, len(n.Constraints))
		copy(constraints, n.Constraints)
		sort.Slice(constraints, func(i, j int) bool {
			return constraints[i].Name < constraints[j].Name
		})
		// Hash each constraint in sorted order
		for _, constraint := range constraints {
			hash, err := w.hash(constraint)
			if err != nil {
				return 0, err
			}
			w.h.writeHashes(hasher, hash)
		}
		for _, alt := range n.OrChains {
			hash, err := w.hash(alt)
			if err != nil {
				return 0, err
			}
			w.h.writeHashes(hasher, hash)
		}
	case *ast.ShapeNode:
		if err := w.h.writeHashes(hasher, NodeKind["Shape"]); err != nil {
			return 0, err
		}
		// Convert map to sorted slice of fields for deterministic ordering
		fields := make([]struct {
			name  string
			field ast.ShapeFieldNode
		}, 0, len(n.Fields))
		for name, field := range n.Fields {
			fields = append(fields, struct {
				name  string
				field ast.ShapeFieldNode
			}{name, field})
		}
		sort.Slice(fields, func(i, j int) bool {
			return fields[i].name < fields[j].name
		})
		// Hash each field in sorted order
		for _, f := range fields {
			if err := w.h.writeHashes(hasher, []byte(f.name)); err != nil {
				return 0, err
			}
			hash, err := w.hash(f.field)
			if err != nil {
				return 0, err
			}
			if err := w.h.writeHashes(hasher, hash); err != nil {
				return 0, err
			}
		}
	case *ast.ShapeFieldNode:
		w.h.writeHashes(hasher, NodeKind["ShapeField"])
		if n.Assertion != nil {
			hash, err := w.hash(*n.Assertion)
			if err != nil {
				return 0, err
			}
			w.h.writeHashes(hasher, hash)
		}
		if n.Shape != nil {
			hash, err := w.hash(*n.Shape)
			if err != nil {
				return 0, err
			}
			w.h.writeHashes(hasher, hash)
		}
	case ast.ImportGroupNode:
		w.h.writeHashes(hasher, NodeKind["ImportGroup"])
		// Sort imports for deterministic ordering
		imports := make([]ast.ImportNode, len(n.Imports))
		copy(imports, n.Imports)
		sort.Slice(imports, func(i, j int) bool {
			return imports[i].Path < imports[j].Path
		})
		for _, importNode := range imports {
			hash, err := w.hash(importNode)
			if err != nil {
				return 0, err
			}
			w.h.writeHashes(hasher, hash)
		}
	case ast.AssignmentNode:
		w.h.writeHashes(hasher, NodeKind["Assignment"])
		w.h.writeHashes(hasher, string(n.CompoundOp))
		for _, lValue := range n.LValues {
			hash, err := w.hash(lValue)
			if err != nil {
				return 0, err
			}
			w.h.writeHashes(hasher, hash)
		}
		for _, rValue := range n.RValues {
			hash, err := w.hash(rValue)
			if err != nil {
				return 0, err
			}
			w.h.writeHashes(hasher, hash)
		}
	case *ast.EnsureBlockNode:
		w.h.writeHashes(hasher, NodeKind["EnsureBlock"])
		for _, node := range n.Body {
			hash, err := w.hash(node)
			if err != nil {
				return 0, err
			}
			w.h.writeHashes(hasher, hash)
		}
	case ast.TypeGuardNode:
		w.h.writeHashes(hasher, NodeKind["TypeGuard"])
		w.h.writeHashes(hasher, []byte(n.Ident))
		// Sort parameters for deterministic ordering
		params := make([]ast.ParamNode, len(n.Parameters()))
		copy(params, n.Parameters())
		sort.Slice(params, func(i, j int) bool {
			var iName, jName string
			switch p := params[i].(type) {
			case ast.SimpleParamNode:
				iName = string(p.Ident.ID)
			case ast.DestructuredParamNode:
				iName = p.Fields[0] // Use first field name for sorting
			}
			switch p := params[j].(type) {
			case ast.SimpleParamNode:
				jName = string(p.Ident.ID)
			case ast.DestructuredParamNode:
				jName = p.Fields[0] // Use first field name for sorting
			}
			return iName < jName
		})
		for _, param := range params {
			hash, err := w.hash(param)
			if err != nil {
				return 0, err
			}
			w.h.writeHashes(hasher, hash)
		}
		hash, err := w.hashNodes(n.Body)
		if err != nil {
			return 0, err
		}
		w.h.writeHashes(hasher, hash)
	case *ast.TypeGuardNode:
		return w.hash(*n)
	case ast.IfNode:
		w.h.writeHashes(hasher, NodeKind["If"])
		if n.Init != nil {
			hash, err := w.hash(n.Init)
			if err != nil {
				return 0, err
			}
			w.h.writeHashes(hasher, hash)
		}
		hash, err := w.hash(n.Condition)
		if err != nil {
			return 0, err
		}
		w.h.writeHashes(hasher, hash)
		hash, err = w.hashNodes(n.Body)
		if err != nil {
			return 0, err
		}
		w.h.writeHashes(hasher, hash)
		for _, elseIf := range n.ElseIfs {
			hash, err := w.hash(elseIf.Condition)
			if err != nil {
				return 0, err
			}
			w.h.writeHashes(hasher, hash)
			hash, err = w.hashNodes(elseIf.Body)
			if err != nil {
				return 0, err
			}
			w.h.writeHashes(hasher, hash)
		}
		if n.Else != nil {
			hash, err = w.hashNodes(n.Else.Body)
			if err != nil {
				return 0, err
			}
			w.h.writeHashes(hasher, hash)
		}
	case *ast.IfNode:
		return w.hash(*n)
	case ast.ElseIfNode:
		w.h.writeHashes(hasher, NodeKind["ElseIf"])
		if err := w.hashOptional(hasher, n.Condition); err != nil {
			return 0, err
		}
		hash, err := w.hashNodes(n.Body)
		if err != nil {
			return 0, err
		}
		w.h.writeHashes(hasher, hash)
	case *ast.ElseIfNode:
		if n == nil {
			return NodeHash(NilHash), nil
		}
		return w.hash(*n)
	case ast.ElseBlockNode:
		w.h.writeHashes(hasher, NodeKind["ElseBlock"])
		hash, err := w.hashNodes(n.Body)
		if err != nil {
			return 0, err
		}
		w.h.writeHashes(hasher, hash)
	case *ast.ElseBlockNode:
		if n == nil {
			return NodeHash(NilHash), nil
		}
		return w.hash(*n)
	case *ast.ForNode:
		fn := *n
		w.h.writeHashes(hasher, NodeKind["For"])
		if err := w.hashOptional(hasher, fn.Init); err != nil {
			return 0, err
		}
		if fn.Cond != nil {
			hash, err := w.hash(fn.Cond)
			if err != nil {
				return 0, err
			}
			w.h.writeHashes(hasher, hash)
		}
		if err := w.hashOptional(hasher, fn.Post); err != nil {
			return 0, err
		}
		if fn.IsRange {
			w.h.writeHashes(hasher, byte(1))
			if fn.RangeX != nil {
				hash, err := w.hash(fn.RangeX)
				if err != nil {
					return 0, err
				}
				w.h.writeHashes(hasher, hash)
			}
			if fn.RangeKey != nil {
				w.h.writeHashes(hasher, string(fn.RangeKey.ID))
			}
			if fn.RangeValue != nil {
				w.h.writeHashes(hasher, string(fn.RangeValue.ID))
			}
			var short byte
			if fn.RangeShort {
				short = 1
			}
			w.h.writeHashes(hasher, short)
		} else {
			w.h.writeHashes(hasher, byte(0))
		}
		hash, err := w.hashNodes(fn.Body)
		if err != nil {
			return 0, err
		}
		w.h.writeHashes(hasher, hash)
	case *ast.SwitchNode:
		sw := *n
		w.h.writeHashes(hasher, NodeKind["Switch"])
		if err := w.hashOptional(hasher, sw.Init); err != nil {
			return 0, err
		}
		if sw.Tag != nil {
			hash, err := w.hash(sw.Tag)
			if err != nil {
				return 0, err
			}
			w.h.writeHashes(hasher, hash)
		}
		for _, clause := range sw.Clauses {
			for _, val := range clause.Values {
				hash, err := w.hash(val)
				if err != nil {
					return 0, err
				}
				w.h.writeHashes(hasher, hash)
			}
			hash, err := w.hashNodes(clause.Body)
			if err != nil {
				return 0, err
			}
			w.h.writeHashes(hasher, hash)
		}
	case ast.SwitchNode:
		return w.hash(&n)
	case ast.FallthroughNode:
		w.h.writeHashes(hasher, NodeKind["Fallthrough"])
	case ast.TypeExpressionNode:
		w.h.writeHashes(hasher, NodeKind["TypeExpression"])
		hash, err := w.hash(n.Type)
		if err != nil {
			return 0, err
		}
		w.h.writeHashes(hasher, hash)
	case ast.ConstGroupNode:
		if err := w.h.writeHashes(hasher, NodeKind["ConstGroup"]); err != nil {
			return 0, err
		}
		for _, spec := range n.Specs {
			if err := w.h.writeHashes(hasher, []byte(spec.Name.ID)); err != nil {
				return 0, err
			}
			if spec.Type != nil {
				typeHash, err := w.hash(*spec.Type)
				if err != nil {
					return 0, err
				}
				if err := w.h.writeHashes(hasher, typeHash); err != nil {
					return 0, err
				}
			}
			if spec.Value != nil {
				valueHash, err := w.hash(spec.Value)
				if err != nil {
					return 0, err
				}
				if err := w.h.writeHashes(hasher, valueHash); err != nil {
					return 0, err
				}
			}
		}
	case ast.IotaLiteralNode:
		if err := w.h.writeHashes(hasher, NodeKind["IotaLiteral"]); err != nil {
			return 0, err
		}
		return NodeHash(hasher.Sum64()), nil
	case *ast.BreakNode:
		w.h.writeHashes(hasher, NodeKind["Break"])
		if n != nil && n.Label != nil {
			w.h.writeHashes(hasher, string(n.Label.ID))
		}
	case *ast.ContinueNode:
		w.h.writeHashes(hasher, NodeKind["Continue"])
		if n != nil && n.Label != nil {
			w.h.writeHashes(hasher, string(n.Label.ID))
		}
	case *ast.GotoNode:
		w.h.writeHashes(hasher, NodeKind["Goto"])
		if n != nil && n.Label != nil {
			w.h.writeHashes(hasher, string(n.Label.ID))
		}
	case *ast.LabeledStmtNode:
		w.h.writeHashes(hasher, NodeKind["LabeledStmt"])
		if n != nil && n.Label != nil {
			w.h.writeHashes(hasher, string(n.Label.ID))
		}
		if n != nil && n.Stmt != nil {
			hash, err := w.hash(n.Stmt)
			if err != nil {
				return 0, err
			}
			w.h.writeHashes(hasher, hash)
		}
	case *ast.DeferNode:
		w.h.writeHashes(hasher, NodeKind["Defer"])
		hash, err := w.hash(n.Call)
		if err != nil {
			return 0, err
		}
		w.h.writeHashes(hasher, hash)
	case *ast.GoStmtNode:
		w.h.writeHashes(hasher, NodeKind["GoStmt"])
		hash, err := w.hash(n.Call)
		if err != nil {
			return 0, err
		}
		w.h.writeHashes(hasher, hash)
	case ast.ReferenceNode:
		w.h.writeHashes(hasher, NodeKind["Reference"])
		hash, err := w.hash(n.Value)
		if err != nil {
			return 0, err
		}
		w.h.writeHashes(hasher, hash)
	case ast.MapLiteralNode:
		w.h.writeHashes(hasher, NodeKind["MapLiteral"])
		// Hash the type
		hash, err := w.hash(n.Type)
		if err != nil {
			return 0, err
		}
		w.h.writeHashes(hasher, hash)
		// Sort entries by key for deterministic ordering
		entries := make([]struct {
			key   ast.Node
			value ast.Node
		}, len(n.Entries))
		for i, entry := range n.Entries {
			entries[i] = struct {
				key   ast.Node
				value ast.Node
			}{entry.Key, entry.Value}
		}
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].key.String() < entries[j].key.String()
		})
		// Hash each entry in sorted order
		for _, entry := range entries {
			hash, err := w.hash(entry.key)
			if err != nil {
				return 0, err
			}
			w.h.writeHashes(hasher, hash)
			hash, err = w.hash(entry.value)
			if err != nil {
				return 0, err
			}
			w.h.writeHashes(hasher, hash)
		}
	case *ast.MapLiteralNode:
		return w.hash(*n)
	case *ast.VariableNode:
		return w.hash(*n)
	case *ast.BinaryExpressionNode:
		return w.hash(*n)
	case *ast.UnaryExpressionNode:
		return w.hash(*n)
	case *ast.IntLiteralNode:
		return w.hash(*n)
	case *ast.FloatLiteralNode:
		return w.hash(*n)
	case *ast.StringLiteralNode:
		return w.hash(*n)
	case *ast.RuneLiteralNode:
		return w.hash(*n)
	case *ast.BoolLiteralNode:
		return w.hash(*n)
	case *ast.FunctionNode:
		return w.hash(*n)
	case *ast.FunctionCallNode:
		return w.hash(*n)
	case *ast.MethodCallNode:
		return w.hash(*n)
	case *ast.EnsureNode:
		return w.hash(*n)
	case *ast.ReferenceNode:
		return w.hash(*n)
	case ast.ArrayLiteralNode:
		w.h.writeHashes(hasher, NodeKind["ArrayLiteral"])
		hash, err := w.hash(n.Type)
		if err != nil {
			return 0, err
		}
		w.h.writeHashes(hasher, hash)
		for _, value := range n.Value {
			hash, err := w.hash(value)
			if err != nil {
				return 0, err
			}
			w.h.writeHashes(hasher, hash)
		}
	case *ast.ArrayLiteralNode:
		return w.hash(*n)
	case ast.DereferenceNode:
		w.h.writeHashes(hasher, NodeKind["Dereference"])
		hash, err := w.hash(n.Value)
		if err != nil {
			return 0, err
		}
		w.h.writeHashes(hasher, hash)
	case ast.TypeDefShapeExpr:
		w.h.writeHashes(hasher, NodeKind["TypeDefShapeExpr"])
		hash, err := w.hash(n.Shape)
		if err != nil {
			return 0, err
		}
		w.h.writeHashes(hasher, hash)
	case ast.TypeDefErrorExpr:
		w.h.writeHashes(hasher, NodeKind["TypeDefErrorExpr"])
		hash, err := w.hash(n.Payload)
		if err != nil {
			return 0, err
		}
		w.h.writeHashes(hasher, hash)
	case ast.OkExprNode:
		if err := w.h.writeHashes(hasher, NodeKind["OkExpr"]); err != nil {
			return 0, err
		}
		hash, err := w.hash(n.Value)
		if err != nil {
			return 0, err
		}
		if err := w.h.writeHashes(hasher, hash); err != nil {
			return 0, err
		}
	case ast.ErrExprNode:
		if err := w.h.writeHashes(hasher, NodeKind["ErrExpr"]); err != nil {
			return 0, err
		}
		hash, err := w.hash(n.Value)
		if err != nil {
			return 0, err
		}
		if err := w.h.writeHashes(hasher, hash); err != nil {
			return 0, err
		}
	case ast.NilLiteralNode:
		if err := w.h.writeHashes(hasher, NodeKind["NilLiteral"]); err != nil {
			return 0, err
		}
		// All NilLiteralNode instances are structurally identical
		return NodeHash(hasher.Sum64()), nil
	case ast.CommentNode:
		if err := writeHash(hasher, NodeKind["Comment"]); err != nil {
			return 0, err
		}
		hasher.Write([]byte(n.Text))
		return NodeHash(hasher.Sum64()), nil
	case ast.UseNode:
		if err := w.h.writeHashes(hasher, NodeKind["Use"]); err != nil {
			return 0, err
		}
		if n.Ident != nil {
			if err := w.h.writeHashes(hasher, []byte(n.Ident.ID)); err != nil {
				return 0, err
			}
		}
		if err := w.h.writeHashes(hasher, []byte(n.ContractType.Ident)); err != nil {
			return 0, err
		}
	case ast.WithNode:
		if err := w.h.writeHashes(hasher, NodeKind["With"]); err != nil {
			return 0, err
		}
		wiringHash, err := w.hash(n.Wiring)
		if err != nil {
			return 0, err
		}
		if err := w.h.writeHashes(hasher, wiringHash); err != nil {
			return 0, err
		}
		bodyHash, err := w.hashNodes(n.Body)
		if err != nil {
			return 0, err
		}
		if err := w.h.writeHashes(hasher, bodyHash); err != nil {
			return 0, err
		}
	default:
		return 0, fmt.Errorf("unsupported node type: %T", n)
	}

	return NodeHash(hasher.Sum64()), nil
}

// HashTokenType generates a structural hash for a token type
func (h *StructuralHasher) HashTokenType(tokenType ast.TokenIdent) NodeHash {
	hasher := fnv.New64a()
	hasher.Write([]byte(string(tokenType)))
	return NodeHash(hasher.Sum64())
}

// NilHash is the initial hash value from fnv.New64a() when no data is written
// It represents a nil/empty hash, so we give it a special type name
const NilHash = uint64(14695981039346656037)

// toBase58 converts a NodeHash to a base58 string
func (h NodeHash) toBase58() string {
	const alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
	num := uint64(h)
	b58 := ""

	for num > 0 {
		remainder := num % 58
		b58 = string(alphabet[remainder]) + b58
		num = num / 58
	}

	if b58 == "" {
		b58 = string(alphabet[0])
	}
	return b58
}

// ToTypeIdent generates a string name for a type based on its hash value
// with T_ prefix
func (h NodeHash) ToTypeIdent() ast.TypeIdent {
	if uint64(h) == NilHash {
		return ast.TypeIdent("T_Invalid")
	}
	return ast.TypeIdent("T_" + h.toBase58())
}

// ToGuardIdent generates a string name for a guard function based on its hash value
// with G_ prefix
func (h NodeHash) ToGuardIdent() ast.TypeIdent {
	if uint64(h) == NilHash {
		return ast.TypeIdent("G_Invalid")
	}
	return ast.TypeIdent("G_" + h.toBase58())
}

// HashSortedStrings returns a stable hash of the given strings (sorted before hashing).
func HashSortedStrings(parts ...string) NodeHash {
	sorted := append([]string(nil), parts...)
	sort.Strings(sorted)
	h := fnv.New64a()
	for _, p := range sorted {
		_, _ = io.WriteString(h, p)
		h.Write([]byte{0})
	}
	return NodeHash(h.Sum64())
}

// ToProvidersIdent names a deduped Providers struct from a slot-set hash (ADR-013).
func (h NodeHash) ToProvidersIdent() string {
	return "Providers_" + h.toBase58()
}

// isNilPointer reports whether i is a typed nil pointer (or untyped nil).
func isNilPointer(i any) bool {
	if i == nil {
		return true
	}
	word := (*ifaceWords)(unsafe.Pointer(&i))
	if word.data != nil {
		return false
	}
	// Rare: data is nil. Distinguish typed-nil pointers from inlined zero values.
	return reflect.TypeOf(i).Kind() == reflect.Pointer
}
