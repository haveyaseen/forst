package main
// EchoRequest: TypeDefShapeExpr({message: String})
type EchoRequest struct {
	message string
}

// T_LbdM2TnbF11: TypeDefShapeExpr({echo: Value(Variable(input.message)), timestamp: Value(1234567890)})
type T_LbdM2TnbF11 struct {
	echo      string
	timestamp int
}

func Echo(input EchoRequest) T_LbdM2TnbF11 {
	return T_LbdM2TnbF11{echo: input.message, timestamp: 1234567890}
}
