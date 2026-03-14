// Command zop is the zop AI CLI tool.
package main

import (
	"os"

	"github.com/peterwwillis/zop/internal/cli"
)

func main() {
	cli.Execute(os.Args[1:])
}
