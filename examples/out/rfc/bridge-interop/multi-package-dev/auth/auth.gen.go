package auth

// HashInput: TypeDefShapeExpr({password: String})
type HashInput struct {
	Password string `json:"password"`
}

type T_SS6D5Sxb9uV struct {
	Hash string `json:"hash"`
}

func Hash(input HashInput) T_SS6D5Sxb9uV {
	return T_SS6D5Sxb9uV{Hash: input.Password}
}
