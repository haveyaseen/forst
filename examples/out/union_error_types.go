package main
// ErrKind is a closed union of nominal errors (only these types implement it).
type ErrKind interface {
	isErrKind()
}

// IoError: TypeDefErrorExpr({path: String})
type IoError struct {
	path string
}

// ParseError: TypeDefErrorExpr({code: Int})
type ParseError struct {
	code int
}

func (e IoError) Error() string {
	return "error"
}

func (e ParseError) Error() string {
	return "error"
}

func (e IoError) ForstErrorTag() string {
	return "main/IoError"
}

func (e ParseError) ForstErrorTag() string {
	return "main/ParseError"
}

func (IoError) isErrKind() {
}

func (ParseError) isErrKind() {
}

func main() {
}
