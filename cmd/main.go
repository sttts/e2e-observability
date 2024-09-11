package main

import (
	"fmt"
	"os"

	"github.com/sttts/e2e-observability/internal/cmd/install"
	"github.com/sttts/e2e-observability/internal/cmd/local"
	"github.com/sttts/e2e-observability/internal/cmd/observe"
	"github.com/sttts/e2e-observability/internal/cmd/snapshot"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, `available commands: "install", "snapshot", "local", "observe"`)
		os.Exit(1)
	}

	var err error
	switch os.Args[1] {
	case "install":
		err = install.Install(os.Stdout)
	case "snapshot":
		err = snapshot.Snapshot(os.Stdout)
	case "local":
		err = local.InstallLocalDevelopment(os.Stdout, os.Args[2])
	case "observe":
		err = observe.Observe(os.Args[2:]...)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
