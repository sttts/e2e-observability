package local

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/sttts/e2e-observability/internal/exec"
)

type Command struct {
	SnapshotURL string `arg:"" help:"URL of the CI snapshot artifact."`
}

func (c *Command) Run() error {
	return installLocalDevelopment(os.Stderr, c.SnapshotURL)
}

func installLocalDevelopment(logger io.Writer, snapshotURL string) error {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return fmt.Errorf("error getting caller information")
	}
	if err := os.Chdir(filepath.Dir(file)); err != nil {
		return fmt.Errorf("error changing directory: %w", err)
	}

	for _, cmdArgs := range [][]string{
		{"helm", "repo", "add", "prometheus-community", "https://prometheus-community.github.io/helm-charts"},
		{"helm", "repo", "add", "grafana", "https://grafana.github.io/helm-charts"},
		{"helm", "repo", "update"},
		{"helm", "upgrade", "--values", "../shared/kube-prometheus-helm.yaml", "--install", "kube-prometheus", "prometheus-community/kube-prometheus-stack", "-n", "monitoring", "--create-namespace"},
		{"helm", "upgrade", "--values", "../shared/loki-helm.yaml", "--install", "loki", "grafana/loki", "-n", "loki", "--create-namespace"},
		{"kubectl", "apply", "-f", "../shared/nodeports.yaml"},
		{"kubectl", "apply", "-f", "../shared/grafana-config.yaml"},
		{"kubectl", "-n", "monitoring", "delete", "--ignore-not-found", "configmap", "artifact-urls"},
		{"kubectl", "-n", "monitoring", "create", "configmap", "artifact-urls",
			fmt.Sprintf(`--from-literal=PROMETHEUS_SNAPSHOT_URL='%v/prometheus.tar.gz'`, snapshotURL),
		},
		{"kubectl", "apply", "--server-side", "--force-conflicts", "-f", "prometheus-local.yaml"},
		{"kubectl", "-n", "loki", "delete", "--ignore-not-found", "configmap", "artifact-urls"},
		{"kubectl", "-n", "loki", "create", "configmap", "artifact-urls",
			fmt.Sprintf(`--from-literal=LOKI_SNAPSHOT_URL='%v/loki.tar.gz'`, snapshotURL),
		},
		{"kubectl", "-n", "loki", "scale", "--replicas=0", "sts/loki"},
		{"kubectl", "apply", "--server-side", "--force-conflicts", "-f", "loki-sts-local.yaml"},
		{"kubectl", "-n", "loki", "scale", "--replicas=1", "sts/loki"},
		{"kubectl", "-n", "loki", "wait", "pods", "-l", `app.kubernetes.io/name=loki`, "--for", "condition=Ready", "--timeout=600s"},
		{"kubectl", "-n", "monitoring", "wait", "pods", "-l", `app.kubernetes.io/instance=kube-prometheus-kube-prome-prometheus`, "--for", "condition=Ready", "--timeout=600s"},
		// flush loki, as it was disconnected.
		{"curl", "-XPOST", "-v", `http://localhost:30002/flush`},
	} {
		if err := exec.ExecCommand(logger, cmdArgs...); err != nil {
			return err
		}
	}

	return nil
}
