package observe

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"

	"github.com/sttts/e2e-observability/internal/jsonwriter"
	"github.com/sttts/e2e-observability/internal/loki_reporter"
)

type Command struct {
	LokiEnabled bool   `default:"true" name:"loki-enabled"`
	LokiURL     string `default:"http://localhost:30002" name:"loki-url"`

	Arguments []string `arg:"" help:"Test command to observe and its arguments."`
}

func (c *Command) Run() error {
	target := io.Discard
	if c.LokiEnabled {
		loki, err := loki_reporter.New(c.LokiURL, os.Stderr)
		if err != nil {
			return fmt.Errorf("error setting up loki: %w", err)
		}
		target = loki
		defer loki.Stop()

	}

	return forwardTo(target, c.Arguments...)
}

func forwardTo(target io.Writer, cmdArgs ...string) error {
	cmd := exec.Command(cmdArgs[0], append(cmdArgs[1:])...)

	cmdStdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get cmdStdout pipe: %w", err)
	}

	cmdStderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to get cmdStdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start command: %w", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	for _, dest := range []struct {
		writer io.Writer
		reader io.Reader
	}{
		{
			writer: io.MultiWriter(os.Stdout, jsonwriter.NewWithLevel(target, "info")),
			reader: cmdStdout,
		},
		{
			writer: io.MultiWriter(os.Stderr, jsonwriter.NewWithLevel(target, "error")),
			reader: cmdStderr,
		},
	} {
		go func() {
			defer wg.Done()
			scanner := bufio.NewScanner(dest.reader)
			for scanner.Scan() {
				_, err := dest.writer.Write(append(scanner.Bytes(), '\n'))
				if err != nil {
					fmt.Fprintln(os.Stderr, err)
					return
				}
			}
		}()
	}
	wg.Wait()

	return nil
}
