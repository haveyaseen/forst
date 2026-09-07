package main

import "os/exec"

func main() {
	runExecDemo()
	runCustomDemo()
}

func runCustomDemo() {
	sum := AddInts(40, 2)
	println(sum)
	message := GreetUpper("forst")
	println(message)
	b := Bag{Name: "forst"}
	println(BagName(b))
}

func runExecDemo() int {
	argv := []string{"true", "extra"}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Run()
	return cmd.ProcessState.ExitCode()
}
