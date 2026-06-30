package main

import "fmt"

// version is overridden at build time via -ldflags.
var version = "dev"

func main() {
	fmt.Printf("cdkm %s\n", version)
}
