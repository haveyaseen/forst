package auth

// Logger: TypeDefShapeExpr({Info: ?})
type Logger interface {
	Info(msg string)
}

// NopLogger: TypeDefShapeExpr({})
type NopLogger struct {
}

type Providers_2TAwF8pWZKc struct {
	Logger Logger
}

func (NopLogger) Info(msg string) {
}

func LogEvent(providers Providers_2TAwF8pWZKc, id string) {
	logger := providers.Logger
	logger.Info("expire " + id)
}
