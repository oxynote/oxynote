// Package buildinfo names the executable and reads its environment.
package buildinfo

import (
	"fmt"
	"os"
	"strings"
)

// Name is the name of the executable. It prefixes every environment
// variable it reads and every metric it exports.
const Name = "oxynote_core"

// EnvName combines the provided key with executable name.
func EnvName(key string) string {
	return fmt.Sprintf("%s_%s", strings.ToUpper(Name), key)
}

// Getenv retrieves an environment variable value by the provided key
// and name combination.
func Getenv(key string) string {
	return os.Getenv(EnvName(key))
}
