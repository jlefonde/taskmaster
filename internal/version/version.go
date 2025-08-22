package version

import (
	"fmt"
	"os"
)

const (
	Version = "1.0.0"
)

func PrintVersion(value string) error {
	fmt.Println(Version)
	os.Exit(0)
	return nil
}
