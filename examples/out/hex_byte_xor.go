package main

import "fmt"

func main() {
	b := []byte("ab")
	fmt.Println(xorMask(b)[0])
	fmt.Println(0x36)
}

func xorMask(k []byte) []byte {
	i := 0
	for i < len(k) {
		k[i] = k[i] ^ 0x36
		i = i + 1
	}
	return k
}
