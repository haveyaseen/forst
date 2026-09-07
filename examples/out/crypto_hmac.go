package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

func FingerprintTag(pepper string, value string) string {
	h := sha256.New()
	h.Write([]byte(pepper + "|" + value))
	sum := h.Sum([]byte{})
	return hex.EncodeToString(sum)
}

func main() {
	fmt.Println(FingerprintTag("pepper", "tag"))
	fmt.Println(useHMAC("k", "m"))
}

func useHMAC(key string, msg string) int {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(msg))
	out := mac.Sum([]byte{})
	return len(out)
}
