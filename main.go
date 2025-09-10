package main

import (
	"fmt"
	"os"
	"time"
)

func main() {
	for {
		fmt.Println("TEST=", os.Getenv("TEST"))
		time.Sleep(2 * time.Second)
	}
}
