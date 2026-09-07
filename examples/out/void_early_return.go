package main

func early(ok bool) {
	if !ok {
		return
	}
	println("ok")
}

func main() {
	early(false)
	early(true)
}
