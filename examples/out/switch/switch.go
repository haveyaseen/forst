package main

func classify(n int) string {
	switch n {
	case 1, 2:
		return "small"
	case 3:
		return "three"
	default:
		return "other"
	}
}

func fallthroughDemo() string {
	var out string = ""
	switch 1 {
	case 1:
		out = out + "one"
		fallthrough
	case 2:
		out = out + "+two"
	}
	return out
}

func main() {
	println(classify(1))
	println(classify(5))
	println(pick(true))
	println(fallthroughDemo())
}

func pick(flag bool) string {
	switch {
	case flag:
		return "yes"
	default:
		return "no"
	}
}
