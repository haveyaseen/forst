package parser

import (
	"fmt"
	"forst/internal/ast"
	"strconv"

	"github.com/sirupsen/logrus"
)

// ShapeLiteralOpts configures parseShapeLiteral.
type ShapeLiteralOpts struct {
	// BaseType is the optional type this shape extends (e.g. AppContext in `AppContext{...}`).
	BaseType *ast.TypeIdent
	// ParseAsTypes parses field annotations as types (String, Int, nested `{...}`) rather than literal values.
	ParseAsTypes bool
	// StartSpan, when set, starts the shape span at the type name (typed `User{…}`) instead of `{`.
	StartSpan ast.SourceSpan
}

// ShapeFieldTypeOpts configures parseShapeFieldTypeAfterColon.
type ShapeFieldTypeOpts struct {
	// InShapeLiteral is true when parsing `{ name: T }` inside a shape literal used as a type context.
	InShapeLiteral bool
}

// parseShapeMemberName reads a shape field or method name. The `error` keyword is allowed
// when it starts a method signature (`error(msg String)`).
func (p *Parser) parseShapeMemberName() string {
	tok := p.current()
	switch tok.Type {
	case ast.TokenIdentifier:
		p.advance()
		return tok.Value
	case ast.TokenError:
		if p.peek().Type == ast.TokenLParen {
			p.advance()
			return tok.Value
		}
		p.FailWithParseError(tok, "expected shape member name")
	default:
		p.FailWithParseError(tok, "expected shape member name")
	}
	panic("unreachable")
}

func (p *Parser) parseShapeMethodSignature() ([]ast.ParamNode, []ast.TypeNode) {
	p.expect(ast.TokenLParen)
	params := []ast.ParamNode{}
	if p.current().Type != ast.TokenRParen {
		for {
			params = append(params, p.parseParameter())
			if p.current().Type == ast.TokenComma {
				p.advance()
			} else {
				break
			}
		}
	}
	p.expect(ast.TokenRParen)
	var returnTypes []ast.TypeNode
	if p.current().Type == ast.TokenColon {
		returnTypes = p.parseReturnType()
	}
	return params, returnTypes
}

// tokenCanStartAssertionBaseType reports whether tok can begin Type.Constraint(...) (identifier or builtin keyword).
func tokenCanStartAssertionBaseType(tok ast.Token) bool {
	if tok.Type == ast.TokenIdentifier || isPossibleConstraintIdentifier(tok) {
		return true
	}
	switch tok.Type {
	case ast.TokenString, ast.TokenInt, ast.TokenFloat, ast.TokenBool:
		return true
	default:
		return false
	}
}

func parseStructTagLiteral(tokenValue string) string {
	if len(tokenValue) >= 2 && tokenValue[0] == '`' && tokenValue[len(tokenValue)-1] == '`' {
		return tokenValue[1 : len(tokenValue)-1]
	}
	if unquoted, err := strconv.Unquote(tokenValue); err == nil {
		return unquoted
	}
	return tokenValue
}

func (p *Parser) attachOptionalStructTag(field ast.ShapeFieldNode) ast.ShapeFieldNode {
	if p.current().Type == ast.TokenStringLiteral {
		tok := p.current()
		field.Tag = parseStructTagLiteral(tok.Value)
		field.TagSpan = ast.SpanFromToken(tok)
		p.advance()
	}
	return field
}

// parseShapeFieldTypeAfterColon parses a type annotation after `:` in a shape field (typedef or literal-as-types).
func (p *Parser) parseShapeFieldTypeAfterColon(name string, opts ShapeFieldTypeOpts) ast.ShapeFieldNode {
	var field ast.ShapeFieldNode
	if p.current().Type == ast.TokenLBrace {
		var shape ast.ShapeNode
		if opts.InShapeLiteral {
			shape = p.parseShapeLiteral(ShapeLiteralOpts{ParseAsTypes: true})
			field = ast.ShapeFieldNode{Shape: &shape}
		} else {
			shape = p.parseShapeType()
			field = ast.ShapeFieldNode{
				Type: &ast.TypeNode{
					Ident: ast.TypeShape,
					Assertion: &ast.AssertionNode{
						BaseType: nil,
						Constraints: []ast.ConstraintNode{{
							Name: "Shape",
							Args: []ast.ConstraintArgumentNode{{
								Shape: &shape,
							}},
						}},
					},
				},
			}
		}
		return p.attachOptionalStructTag(field)
	}
	tok := p.current()
	if p.peek().Type == ast.TokenDot && tokenCanStartAssertionBaseType(tok) {
		assertion := p.parseAssertionChain(true)
		field = ast.ShapeFieldNode{
			Type: &ast.TypeNode{
				Ident:     ast.TypeAssertion,
				Assertion: &assertion,
			},
		}
		return p.attachOptionalStructTag(field)
	}
	if isPossibleTypeIdentifier(p.current(), TypeIdentOpts{AllowLowercaseTypes: false}) ||
		p.current().Type == ast.TokenStar ||
		p.current().Type == ast.TokenArray {
		typ := p.parseType(TypeIdentOpts{AllowLowercaseTypes: true})
		typeIdent := typ.Ident
		p.logParsedNodeWithMessage(typ, fmt.Sprintf("Parsed type for shape field %s and type ident %s (type: %+v)", name, typeIdent, typ))
		field = ast.ShapeFieldNode{
			Type: &typ,
		}
		return p.attachOptionalStructTag(field)
	}
	p.FailWithReport(p.current(), "shape-type-required", "expected type annotation in shape type context",
		"Expected type annotation in shape type context.",
		"add a type after each field name, e.g. `{ name: String }`")
	panic("unreachable")
}

func (p *Parser) parseShapeTypeField(name string) ast.ShapeFieldNode {
	if p.current().Type == ast.TokenLParen {
		params, returnTypes := p.parseShapeMethodSignature()
		return ast.ShapeFieldNode{
			IsMethod:          true,
			MethodParams:      params,
			MethodReturnTypes: returnTypes,
		}
	}
	switch p.current().Type {
	case ast.TokenColon:
		p.advance() // Consume the colon

		return p.parseShapeFieldTypeAfterColon(name, ShapeFieldTypeOpts{})
	case ast.TokenStar:
		// Handle pointer types
		fieldType := p.parseType(TypeIdentOpts{AllowLowercaseTypes: true})
		return p.attachOptionalStructTag(ast.ShapeFieldNode{
			Type: &fieldType,
		})
	case ast.TokenLBrace:
		shape := p.parseShapeType()
		// For shape types, nested shapes are stored on Shape and as Type with Assertion.
		return p.attachOptionalStructTag(ast.ShapeFieldNode{
			Shape: &shape,
			Type: &ast.TypeNode{
				Ident: ast.TypeShape,
				Assertion: &ast.AssertionNode{
					BaseType: nil,
					Constraints: []ast.ConstraintNode{{
						Name: "Shape",
						Args: []ast.ConstraintArgumentNode{{
							Shape: &shape,
						}},
					}},
				},
			},
		})
	}
	// If no colon, type-only member in a shape type context is an embedded field.
	typeIdent := ast.TypeIdent(name)
	return ast.ShapeFieldNode{
		Type: &ast.TypeNode{
			Ident: typeIdent,
		},
		Embedded: true,
	}
}

func (p *Parser) parseShapeType() ast.ShapeNode {
	return p.parseShapeTypeInternal(false)
}

func (p *Parser) parseShapeTypeAllowEmpty() ast.ShapeNode {
	return p.parseShapeTypeInternal(true)
}

func (p *Parser) parseShapeTypeInternal(allowEmpty bool) ast.ShapeNode {
	p.log.WithField("token", p.current()).Trace("Entering parseShapeType")
	p.expect(ast.TokenLBrace)

	fields := make(map[string]ast.ShapeFieldNode)
	var fieldOrder []string
	// Parse fields until closing brace
	for p.current().Type != ast.TokenRBrace {
		p.log.WithField("token", p.current()).Trace("parseShapeType: parsing field")
		// Parse field name
		name := p.parseShapeMemberName()

		fields[name] = p.parseShapeTypeField(name)
		fieldOrder = append(fieldOrder, name)

		p.log.WithField("token", p.current()).Trace("parseShapeType: after field parse")
		// Handle commas between fields
		if p.current().Type == ast.TokenComma {
			p.advance() // Consume the comma
		}
	}

	p.expect(ast.TokenRBrace)

	if len(fields) == 0 && !allowEmpty {
		p.FailWithReport(p.current(), "shape-empty", "shapes need at least one field",
			"Empty `{ }` types are not allowed.",
			"add a field, e.g. `{ name: String }`")
	}

	baseType := ast.TypeIdent(ast.TypeShape)
	return ast.ShapeNode{
		Fields:     fields,
		FieldOrder: fieldOrder,
		BaseType:   &baseType,
	}
}

// parseShapeTypeForError parses `{ ... }` for `error Name { ... }`. Empty payloads are allowed
// (e.g. `error RateLimited {}` for `RateLimited()` on `ensure … or`).
func (p *Parser) parseShapeTypeForError() ast.ShapeNode {
	p.expect(ast.TokenLBrace)
	fields := make(map[string]ast.ShapeFieldNode)
	var fieldOrder []string
	for p.current().Type != ast.TokenRBrace {
		name := p.parseShapeMemberName()
		fields[name] = p.parseShapeTypeField(name)
		fieldOrder = append(fieldOrder, name)
		if p.current().Type == ast.TokenComma {
			p.advance()
		}
	}
	p.expect(ast.TokenRBrace)
	baseType := ast.TypeIdent(ast.TypeShape)
	return ast.ShapeNode{
		Fields:     fields,
		FieldOrder: fieldOrder,
		BaseType:   &baseType,
	}
}

// parseShapeLiteral parses a shape literal value or type.
func (p *Parser) parseShapeLiteral(opts ShapeLiteralOpts) ast.ShapeNode {
	p.log.WithFields(logrus.Fields{
		"function":     "parseShapeLiteral",
		"baseType":     opts.BaseType,
		"parseAsTypes": opts.ParseAsTypes,
	}).Debug("Starting parseShapeLiteral")

	p.log.WithField("token", p.current()).Trace("Entering parseShapeLiteral")
	lbrace := p.expect(ast.TokenLBrace)

	fields := make(map[string]ast.ShapeFieldNode)
	var fieldOrder []string
	// Parse fields until closing brace
	for p.current().Type != ast.TokenRBrace {
		p.log.WithField("token", p.current()).Trace("parseShapeLiteral: parsing field")
		// Parse field name
		name := p.parseShapeMemberName()

		// If the next token is a colon, parse the value or type
		if p.current().Type == ast.TokenColon {
			p.advance() // Consume the colon

			if opts.ParseAsTypes {
				fields[name] = p.parseShapeFieldTypeAfterColon(name, ShapeFieldTypeOpts{InShapeLiteral: true})
			} else {
				// Parse as expression so concat / calls work in shape field values
				// (e.g. E{message: "a" + b}).
				val := p.parseExpression()
				p.log.WithFields(logrus.Fields{
					"fieldName": name,
					"valType":   fmt.Sprintf("%T", val),
					"valValue":  fmt.Sprintf("%+v", val),
				}).Debug("Parsed value for shape field")

				valNode, ok := val.(ast.Node)
				if !ok {
					p.log.WithFields(logrus.Fields{
						"fieldName": name,
						"valType":   fmt.Sprintf("%T", val),
						"valValue":  fmt.Sprintf("%+v", val),
					}).Error("Value does not implement ast.Node")
					panic(fmt.Sprintf("parseShapeLiteral: value for field '%s' does not implement ast.Node: type=%T value=%+v", name, val, val))
				}
				var field ast.ShapeFieldNode
				switch v := val.(type) {
				case ast.ShapeNode:
					field = ast.ShapeFieldNode{Node: valNode, Shape: &v}
					p.log.WithFields(logrus.Fields{
						"fieldName": name,
						"nodeSet":   true,
					}).Debug("Created shape field with Node and Shape")
				default:
					field = ast.ShapeFieldNode{Node: valNode}
					p.log.WithFields(logrus.Fields{
						"fieldName": name,
						"nodeSet":   true,
					}).Debug("Created shape field with Node")
				}

				// For backward compatibility, also set the Type field for variable references
				if varNode, ok := val.(ast.VariableNode); ok {
					field.Type = &ast.TypeNode{
						Ident: ast.TypeIdent(string(varNode.Ident.ID)),
					}
					p.log.WithFields(logrus.Fields{
						"fieldName": name,
						"typeSet":   true,
						"typeIdent": string(varNode.Ident.ID),
					}).Debug("Set Type field for variable reference")
				}
				if _, isShape := val.(ast.ShapeNode); !isShape {
					if vn, ok := val.(ast.ValueNode); ok {
						field.Assertion = &ast.AssertionNode{
							BaseType: nil,
							Constraints: []ast.ConstraintNode{{
								Name: string(ast.ValueConstraint),
								Args: []ast.ConstraintArgumentNode{{
									Value: &vn,
								}},
							}},
						}
					}
					// Non-value expressions (concat, calls) live only on field.Node.
				}
				fields[name] = field
			}
		} else {
			// If no colon, use the field name as both key and value (type assertion)
			typeIdent := ast.TypeIdent(name)
			fields[name] = ast.ShapeFieldNode{
				Type: &ast.TypeNode{
					Ident: typeIdent,
				},
			}
		}
		fieldOrder = append(fieldOrder, name)

		p.log.WithField("token", p.current()).Trace("parseShapeLiteral: after field parse")
		// Handle commas between fields
		if p.current().Type == ast.TokenComma {
			p.advance() // Consume the comma
		}
	}

	rbrace := p.expect(ast.TokenRBrace)

	span := ast.SpanBetweenTokens(lbrace, rbrace)
	if opts.StartSpan.IsSet() {
		span = ast.SpanFromTo(opts.StartSpan, ast.SpanFromToken(rbrace))
	}

	return ast.ShapeNode{
		Fields:     fields,
		FieldOrder: fieldOrder,
		BaseType:   opts.BaseType,
		Span:       span,
	}
}
