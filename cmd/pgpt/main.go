// Command pgpt is the PowerGPT AI CLI tool.
package main

import (
	"os"

	"github.com/peterwwillis/pgpt/internal/cli"
)

func main() {
	cli.Execute(os.Args[1:])
}
