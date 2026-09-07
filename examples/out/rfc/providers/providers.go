package providers_demo

// Clock: TypeDefShapeExpr({now: ?})
type Clock interface {
	now() int
}

// FakeClock: TypeDefShapeExpr({fixedMs: Int})
type FakeClock struct {
	fixedMs int
}

// Logger: TypeDefShapeExpr({info: ?})
type Logger interface {
	info(msg string)
}

// NopLogger: TypeDefShapeExpr({})
type NopLogger struct {
}

type Providers_2TAwF8pWZKc struct {
	Logger Logger
}

type Providers_Pm6dPg3hV64 struct {
	Clock  Clock
	Logger Logger
}

// Token: TypeDefShapeExpr({id: String, expiresAt: Int})
type Token struct {
	expiresAt int
	id        string
}

func expireToken(providers Providers_Pm6dPg3hV64, token Token) {
	clock := providers.Clock
	if token.expiresAt < clock.now() {
		logExpiry(Providers_2TAwF8pWZKc{Logger: providers.Logger}, token.id)
	}
}

func (NopLogger) info(msg string) {
}

func logExpiry(providers Providers_2TAwF8pWZKc, id string) {
	logger := providers.Logger
	logger.info("expire " + id)
}

func mainWiringDemo() {
	expireToken(Providers_Pm6dPg3hV64{Clock: &FakeClock{fixedMs: 0}, Logger: &NopLogger{}}, Token{id: "x", expiresAt: 1})
}

func (c FakeClock) now() int {
	return c.fixedMs
}
