package main
// EchoRequest: TypeDefShapeExpr({message: String})
type EchoRequest struct {
	Message string `json:"message"`
}

// EchoResponse: TypeDefShapeExpr({echo: String, timestamp: Int})
type EchoResponse struct {
	Echo      string `json:"echo"`
	Timestamp int    `json:"timestamp"`
}

func Echo(input EchoRequest) EchoResponse {
	return EchoResponse{Echo: input.Message, Timestamp: 42}
}

func main() {
	println("embedded invoke listening on :6321")
	ForstInvokeWaitForShutdown()
}
