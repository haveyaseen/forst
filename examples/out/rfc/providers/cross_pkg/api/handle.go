package api

import "providers_cross_pkg_demo/auth"
// Logger: TypeDefShapeExpr({Info: ?})
type Logger interface {
	Info(msg string)
}

// NopLogger: TypeDefShapeExpr({})
type NopLogger struct {
}

type Providers_2TAwF8pWZKc struct {
	Logger auth.Logger
}

func HandleRequest(providers Providers_2TAwF8pWZKc, id string) {
	auth.LogEvent(auth.Providers_2TAwF8pWZKc{Logger: providers.Logger}, id)
}
