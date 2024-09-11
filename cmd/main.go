package main

import (
	"fmt"
	"os"

	"github.com/alecthomas/kong"
	"github.com/sttts/e2e-observability/internal/cmd/install"
	"github.com/sttts/e2e-observability/internal/cmd/local"
	"github.com/sttts/e2e-observability/internal/cmd/observe"
	"github.com/sttts/e2e-observability/internal/cmd/snapshot"
)

type Command struct {
	Install  install.Command  `cmd:"" name:"install" help:"Install observability stack into Kubernetes cluster in CI."`
	Observe  observe.Command  `cmd:"" name:"observe" help:"Run a command and observe its output in CI."`
	Snapshot snapshot.Command `cmd:"" name:"snapshot" help:"Take a snapshot of the observability stack. This is usually run in CI."`
	Local    local.Command    `cmd:"" name:"local" help:"Install observability stack locally for debugging."`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, `available commands: "install", "snapshot", "local", "observe"`)
		os.Exit(1)
	}

	ctx := kong.Parse(&Command{},
		kong.Name("e2e-observability"),
		kong.Description("Spaces API Proxy Server."),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact:             true,
			NoExpandSubcommands: true,
		}),
	)

	ctx.FatalIfErrorf(ctx.Run())
}
