package main

// EchoRequest: TypeDefShapeExpr({message: String})
type EchoRequest struct {
	message string
}

type T_LbdM2TnbF11 struct {
	echo      string
	timestamp int
}

func Echo(input EchoRequest) T_LbdM2TnbF11 {
	return T_LbdM2TnbF11{echo: input.message, timestamp: 1234567890}
}
