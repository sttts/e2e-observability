package snapshot

import (
	"io"
	"os"

	"github.com/sttts/e2e-observability/internal/exec"
)

type Command struct {
}

func (c *Command) Run() error {
	return snapshot(os.Stderr)
}

func snapshot(logger io.Writer) error {
	for _, cmdArgs := range [][]string{
		// tell prometheus to take snapshot so WAL is flushed
		{"curl", "-XPOST", "-v", `http://localhost:30000/api/v1/admin/tsdb/snapshot`},
		{"sh", "-c", `kubectl exec -n monitoring prometheus-kube-prometheus-kube-prome-prometheus-0 -- tar cvzf - -C /prometheus . >prometheus.tar.gz`},
		{"sh", "-c", `kubectl exec -n loki loki-0 -- tar cvzf - -C /var/loki . >loki.tar.gz`},
	} {
		if err := exec.ExecCommand(logger, cmdArgs...); err != nil {
			return err
		}
	}
	return nil
}
