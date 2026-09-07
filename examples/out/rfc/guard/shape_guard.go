package main

import errors "errors"
// AppContext: TypeDefShapeExpr({sessionId: Pointer(String), user: Pointer(User)})
type AppContext struct {
	sessionId *string
	user      *User
}

// AppMutation: TypeDefShapeExpr({ctx: AppContext})
type AppMutation struct {
	ctx AppContext
}

// MutationArg: TypeDefShapeExpr({})
type MutationArg struct {
}

// T_488eVThFocF: TypeDefShapeExpr({ctx: AppContext, input: {name: String}})
type T_488eVThFocF struct {
	ctx   AppContext
	input User
}

type T_5ksn1adCDmh string
// T_8XMnUftkRoa: TypeDefShapeExpr({sessionId: Pointer(String), user: Pointer(User)})
type T_8XMnUftkRoa struct {
	sessionId *string
	user      *User
}

// T_BDvMhNEUYH5: TypeDefShapeExpr({ctx: ctx})
type T_BDvMhNEUYH5 struct {
	ctx T_5ksn1adCDmh
}

// T_F1jpghi8Uyp: TypeDefShapeExpr({input: {name: String}})
type T_F1jpghi8Uyp struct {
	input User
}

type T_PBoS2ej5ec7 string
type T_PFzGCZEtwX2 string
// T_U2aboJvjLFx: TypeDefShapeExpr({input: input})
type T_U2aboJvjLFx struct {
	input T_PFzGCZEtwX2
}

// T_bTwM5AcoxRu: TypeDefShapeExpr({})
type T_bTwM5AcoxRu struct {
}

// T_bWZeLfb2t2d: TypeDefShapeExpr({})
type T_bWZeLfb2t2d struct {
}

// User: TypeDefShapeExpr({name: String})
type User struct {
	name string
}

func G_HRerS5U4F3k(ctx AppContext) bool {
	if ctx.sessionId == nil {
		return false
	}
	if ctx.user == nil {
		return false
	}
	return true
}

func G_HwLFSnYiRNz(m MutationArg, input T_PBoS2ej5ec7) bool {
	return true
}

func G_NLy51AjZfNe(m MutationArg, ctx T_PBoS2ej5ec7) bool {
	return true
}

func createTask(op T_488eVThFocF) (string, error) {
	if !G_HRerS5U4F3k(op.ctx) {
		return "", errors.New("ensure op.ctx is AppContext.LoggedIn(): want AppContext.LoggedIn()")
	}
	println("Creating task, logged in with sessionId: " + *op.ctx.sessionId)
	return op.input.name, nil
}

func main() {
	sessionId := "479569ae-cbf0-471e-b849-38a698e0cb69"
	createTask(T_488eVThFocF{ctx: AppContext{sessionId: &sessionId, user: &User{}}, input: User{name: "Fix memory leak in Node.js app"}})
	println("Correctly created task (Result API)")
	createTask(T_488eVThFocF{ctx: AppContext{sessionId: nil, user: nil}, input: User{name: "Go to the gym"}})
	println("Correctly avoided creating gym task as user was not logged in")
}
