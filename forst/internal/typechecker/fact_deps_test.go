package typechecker

import (
	"io"
	"strings"
	"testing"

	"forst/internal/ast"
	"forst/internal/parser"

	"github.com/sirupsen/logrus"
)

func TestExtractDeps_AdultAgeOnly(t *testing.T) {
	t.Parallel()
	src := `
package main
type User = { name: String, age: Int }
is (user User) Adult {
    ensure user.age is Min(18)
}
func check(user User) {
    ensure user is Adult()
}
func main() {}
`
	tc := factDepsTypecheck(t, src)
	facts := tc.ActiveFactsWithDeps()
	if len(facts) == 0 {
		t.Fatal("expected facts")
	}
	keys := PathKeysSorted(facts[0].Reads)
	joined := strings.Join(keys, ",")
	if !strings.Contains(joined, ".age") {
		t.Fatalf("want .age in %v", keys)
	}
	if strings.Contains(joined, ".name") {
		t.Fatalf("must not include .name in %v", keys)
	}
}

func TestExtractDeps_TypeTargetPlace(t *testing.T) {
	t.Parallel()
	src := `
package main
type ActiveStatus = "pending" | "processing"
error InvalidStatus {}
func check(status String) {
    ensure status is ActiveStatus else InvalidStatus{}
}
func main() {}
`
	tc := factDepsTypecheck(t, src)
	facts := tc.ActiveFactsWithDeps()
	if len(facts) == 0 {
		t.Fatal("expected type-target fact")
	}
	if facts[0].Subject == nil {
		t.Fatal("subject missing")
	}
	if len(facts[0].Reads) == 0 {
		t.Fatal("expected place dep")
	}
}

func TestMayClobber_fieldSibling(t *testing.T) {
	t.Parallel()
	pi := NewPathInterner()
	age := pi.Intern(AccessPath{Root: 1, Steps: []AccessStep{{Kind: AccessField, Field: "age"}}})
	name := pi.Intern(AccessPath{Root: 1, Steps: []AccessStep{{Kind: AccessField, Field: "name"}}})
	root := pi.Intern(AccessPath{Root: 1})
	if MayClobber(name, age) {
		t.Fatal("name write must not clobber age dep")
	}
	if !MayClobber(age, age) {
		t.Fatal("age write must clobber age dep")
	}
	if !MayClobber(root, age) {
		t.Fatal("root write must clobber age dep")
	}
}

func factDepsTypecheck(t *testing.T, src string) *TypeChecker {
	t.Helper()
	log := logrus.New()
	log.SetOutput(io.Discard)
	nodes, err := parser.NewTestParser(src, log).ParseFile()
	if err != nil {
		t.Fatal(err)
	}
	tc := New(log, false)
	if err := tc.CheckTypes(nodes); err != nil {
		t.Fatal(err)
	}
	_ = ast.TypeString // keep ast import used if needed
	return tc
}
