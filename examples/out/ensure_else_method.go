package main

import (
	"fmt"
	os "os"
)

// E: TypeDefErrorExpr({message: String})
type E struct {
	message string
}

// P: TypeDefShapeExpr({n: Int})
type P struct {
	n int
}

// T_CQ83zP8NNan: TypeDefShapeExpr({})
type T_CQ83zP8NNan struct {
}

func (e E) Error() string {
	return "error"
}

func (e E) ForstErrorTag() string {
	return "main/E"
}

func (p *P) badConcat(ok bool, kw string) error {
	if !ok {
		_ = E{message: "bad " + kw}
	}
	return nil
}

func (p *P) badMethod(ok bool) error {
	if !ok {
		_ = p.errMsg("bad")
	}
	return nil
}

func (p *P) errMsg(msg string) E {
	return E{message: msg}
}

func main() {
	r, rErr := run(true)
	if rErr != nil {
		{
			fmt.Fprintf(os.Stderr, "ensure failed: %v\n", rErr)
			os.Exit(1)
		}
	}
	fmt.Println(r)
	fmt.Println(sibling())
}

func run(ok bool) (int, error) {
	p := &P{n: 0}
	err := p.badMethod(ok)
	if err != nil {
		return 0, err
	}
	return 1, nil
}

func sibling() string {
	return "alive"
}
