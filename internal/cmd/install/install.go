package install

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	clientgoretry "k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	k8scfg "sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/yaml"

	"github.com/sttts/e2e-observability/internal/exec"
)

var defaultBackOff = wait.Backoff{
	Duration: 2 * time.Second,
	Factor:   1.0,
	Steps:    30 * 5,
}

func Install(logger io.Writer) error {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return fmt.Errorf("error getting caller information")
	}
	if err := os.Chdir(filepath.Dir(file)); err != nil {
		return fmt.Errorf("error changing directory: %w", err)
	}

	ctx := context.Background()

	cfg, err := k8scfg.GetConfig()
	if err != nil {
		return err
	}
	k8sClient, err := client.New(cfg, client.Options{})
	if err != nil {
		return err
	}

	for _, cmdArgs := range [][]string{
		{"helm", "repo", "add", "prometheus-community", "https://prometheus-community.github.io/helm-charts"},
		{"helm", "repo", "add", "grafana", "https://grafana.github.io/helm-charts"},
		{"helm", "repo", "update"},
		{"helm", "upgrade", "--values", "../shared/kube-prometheus-helm.yaml", "--install", "kube-prometheus", "prometheus-community/kube-prometheus-stack", "-n", "monitoring", "--create-namespace"},
		{"helm", "upgrade", "--values", "../loki-helm.yaml", "--install", "loki", "grafana/loki", "-n", "loki", "--create-namespace"},
		{"helm", "upgrade", "--values", "promtail-helm.yaml", "--install", "promtail", "grafana/promtail", "-n", "promtail", "--create-namespace"},
		{"kubectl", "apply", "-f", "../shared/nodeports.yaml"},
		{"kubectl", "apply", "-f", "../shared/grafana-config.yaml"},
		{"kubectl", "apply", "--server-side", "--force-conflicts", "-f", "prometheus.yaml"},
	} {
		if err := exec.ExecCommand(logger, cmdArgs...); err != nil {
			return err
		}
	}

	err = wait.ExponentialBackoffWithContext(ctx, defaultBackOff, func(ctx context.Context) (bool, error) {
		if err := exec.ExecCommand(logger, "kubectl", "-n", "monitoring", "scale", "--replicas=0", "deployment/kube-prometheus-kube-state-metrics"); err != nil {
			logger.Write([]byte(fmt.Sprintf("Error scaling down ksm: %v\n", err)))
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("error executing command: %w", err)
	}

	err = patchKubeStateMetricsDeployment(ctx, k8sClient)
	if err != nil {
		return fmt.Errorf("error patching kube state metrics: %w", err)
	}

	for _, cmdArgs := range [][]string{
		{"kubectl", "-n", "monitoring", "scale", "--replicas=1", "deployment/kube-prometheus-kube-state-metrics"},
		{"kubectl", "apply", "--server-side", "-f", "ksm-crb.yaml"},
		{"kubectl", "-n", "loki", "rollout", "status", "--watch", "statefulset/loki"},
		{"kubectl", "-n", "promtail", "rollout", "status", "--watch", "deployment/promtail"},
		{"kubectl", "-n", "monitoring", "rollout", "status", "--watch", "deployment/kube-prometheus-kube-state-metrics"},
		{"kubectl", "-n", "monitoring", "rollout", "status", "--watch", "statefulset/prometheus-kube-prometheus-kube-prome-prometheus"},
	} {
		if err := exec.ExecCommand(logger, cmdArgs...); err != nil {
			return err
		}
	}
	return nil
}

func readUnstructeredObject(filename string) (*unstructured.Unstructured, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("error reading %q: %w", filename, err)
	}

	var obj map[string]interface{}
	if err := yaml.Unmarshal(data, &obj); err != nil {
		return nil, fmt.Errorf("error unmarshalling %q: %w", filename, err)
	}

	return &unstructured.Unstructured{Object: obj}, nil
}

func patchKubeStateMetricsDeployment(ctx context.Context, k8s client.Client) error {
	ksmConfig, err := readUnstructeredObject("ksm-config.yaml")
	if err != nil {
		return err
	}
	err = k8s.Patch(ctx, ksmConfig, client.Apply, client.ForceOwnership, client.FieldOwner("ako-test"))
	if err != nil {
		return fmt.Errorf("error patching ksm config: %w", err)
	}

	if err := clientgoretry.RetryOnConflict(clientgoretry.DefaultRetry, func() error {
		ksm := &v1.Deployment{}
		if err := k8s.Get(ctx, k8stypes.NamespacedName{Namespace: "monitoring", Name: "kube-prometheus-kube-state-metrics"}, ksm); err != nil {
			return err
		}
		// TODO(sur): submit kube-prometheus upstream PR to bump ksm
		ksm.Spec.Template.Spec.Containers[0].Image = "registry.k8s.io/kube-state-metrics/kube-state-metrics:v2.12.0"
		if len(ksm.Spec.Template.Spec.Containers[0].VolumeMounts) == 0 {
			ksm.Spec.Template.Spec.Containers[0].Args = append(ksm.Spec.Template.Spec.Containers[0].Args, "--custom-resource-state-config-file=/etc/kube-state-metrics/ako.yaml")
			ksm.Spec.Template.Spec.Containers[0].VolumeMounts = append(ksm.Spec.Template.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
				Name:      "ako",
				MountPath: "/etc/kube-state-metrics",
			})
			ksm.Spec.Template.Spec.Volumes = append(ksm.Spec.Template.Spec.Volumes, corev1.Volume{
				Name: "ako",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{Name: "kube-state-metrics-config"},
						Items:                []corev1.KeyToPath{{Key: "ako", Path: "ako.yaml"}},
					},
				},
			})
		}

		return k8s.Update(ctx, ksm)
	}); err != nil {
		return fmt.Errorf("error updating ksm deployment: %w", err)
	}

	return nil
}
