package main

import (
	"github.com/itmo-mobile-app/tac-os/server/dbms/internal/cli"
	"os"
)

func main() { os.Exit(cli.Run(os.Stdin, os.Stdout, os.Stderr)) }
