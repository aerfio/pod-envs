package main

import (
	"context"
	"fmt"
	"golang.org/x/exp/maps"
	"gopkg.in/yaml.v3"
	"io"
	corev1 "k8s.io/api/core/v1"
	"os"
	"slices"
	"strings"

	"aerf.io/podenvs/k8s"
	"github.com/andreazorzetto/yh/highlight"

	"github.com/neilotoole/jsoncolor"
	"github.com/spf13/pflag"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	envutil "k8s.io/kubectl/pkg/cmd/set/env"
	ctrl "sigs.k8s.io/controller-runtime"
	sigsyaml "sigs.k8s.io/yaml"
)

func main() {
	var namespace, name, container string
	var yamlOut, printHelp, printExportableEnvs bool
	pflag.StringVar(&namespace, "namespace", "default", "Namespace of the pod. Ignored if the input is set to stdin.")
	pflag.StringVar(&name, "name", "", "Name of the pod. Ignored if the input is set to stdin.")
	pflag.StringVarP(&container, "container", "c", "", "Container inside that pod from which to extract envs. Unused if there's only 1 container")
	pflag.BoolVarP(&printExportableEnvs, "exportable", "e", false, "Prints envs in a format ready to copy and paste into terminal to export them")
	pflag.BoolVarP(&yamlOut, "yaml", "y", false, "yaml output instead of json")

	pflag.BoolVarP(&printHelp, "help", "h", false, "print help msg")
	pflag.Parse()

	if printHelp {
		fmt.Fprintf(os.Stderr, `Usage of podenvs:
This binary connects to the current-context from the kubeconfig to read referenced secrets/configmaps, even if the pod is supplied by stdin.

Available flags: 
%s
"--exportable" flag takes precedence before "--yaml" flag. 
"--container" flag is only required if the targeted pod has more than 1 container, otherwise it's ignored.
You can pass the yaml/json representation of pod to stdin of podenvs using "-" as last argument, example:
  $ kubectl get pod -n $NAMESPACE $POD_NAME -oyaml | podenvs -
The "--name" flag is not required in this case and is ignored, along with "--namespace" flag. 
`, pflag.CommandLine.FlagUsages())
		os.Exit(0)
	}

	kc := kubernetes.NewForConfigOrDie(ctrl.GetConfigOrDie())

	pr := podReader{
		kc:     kc,
		reader: nil,
	}
	if os.Args[len(os.Args)-1] == "-" {
		pr.reader = os.Stdin
	} else {
		if name == "" {
			fmt.Fprintln(os.Stderr, "no 'name' flag provided")
			os.Exit(1)
		}
	}

	pod, err := pr.getPod(name, namespace)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get the pod %s/%s: %s\n", name, namespace, err)
		os.Exit(1)
	}

	if len(pod.Spec.Containers) > 1 && container == "" {
		fmt.Fprintln(os.Stderr, "more than one container in the selected pod, you must provide 'container' flag")
		os.Exit(1)
	}

	podContainer := k8s.MustFindContainerWithName(pod.Spec.Containers, name)
	envs := make(map[string]string, len(podContainer.Env))

	for _, env := range podContainer.Env {
		if env.Value != "" {
			envs[env.Name] = env.Value
		} else if env.ValueFrom != nil {
			val, err := envutil.GetEnvVarRefValue(kc, namespace, envutil.NewResourceStore(), env.ValueFrom, pod, &podContainer)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to get envs from ref value: %s\n", err)
				os.Exit(1)
			}
			envs[env.Name] = val
		} else {
			envs[env.Name] = ""
		}
	}

	if err := getEncoder(os.Stdout, yamlOut, printExportableEnvs).Encode(envs); err != nil {
		fmt.Fprintf(os.Stderr, "failed to print the envs: %s\n", err)
		os.Exit(1)
	}
}

type podReader struct {
	kc     kubernetes.Interface
	reader io.Reader
}

func (p podReader) getPod(name, ns string) (*corev1.Pod, error) {
	if p.reader != nil {
		pod := &corev1.Pod{}
		podBytes, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, err
		}
		return pod, sigsyaml.Unmarshal(podBytes, pod)
	}
	return p.kc.CoreV1().Pods(ns).Get(context.Background(), name, metav1.GetOptions{})
}

func getEncoder(out io.Writer, yamlOut, printExportableEnvs bool) Encoder {
	if printExportableEnvs {
		return &exportableEncoder{out: out}
	}
	if yamlOut {
		return yamlEncoder{
			out: out,
		}
	}
	return getJSONEnc(out)
}

type Encoder interface {
	Encode(map[string]string) error
}

type yamlEncoder struct {
	out    io.Writer
	pretty bool
}

func (y yamlEncoder) Encode(arg map[string]string) error {
	if !jsoncolor.IsColorTerminal(y.out) {
		return yaml.NewEncoder(y.out).Encode(arg)
	}

	buf := strings.Builder{}
	if err := yaml.NewEncoder(&buf).Encode(arg); err != nil {
		return err
	}

	yamlStr, err := highlight.Highlight(strings.NewReader(buf.String()))
	if err != nil {
		return err
	}

	_, err = io.WriteString(y.out, yamlStr)
	return err
}

var _ Encoder = yamlEncoder{}

func getJSONEnc(out io.Writer) Encoder {
	enc := jsoncolor.NewEncoder(out)
	enc.SetIndent("", "  ")

	if jsoncolor.IsColorTerminal(out) {
		// DefaultColors are similar to jq
		clrs := jsoncolor.DefaultColors()
		enc.SetColors(clrs)
	}
	return &wrapEncoder{inner: enc}
}

type wrapEncoder struct {
	inner *jsoncolor.Encoder
}

func (w wrapEncoder) Encode(arg map[string]string) error {
	return w.inner.Encode(arg)
}

var _ Encoder = &wrapEncoder{}

type exportableEncoder struct {
	out io.Writer
}

func (e *exportableEncoder) Encode(arg map[string]string) error {
	keys := maps.Keys(arg)
	slices.Sort(keys)
	for _, key := range keys {
		val := arg[key]
		fmt.Fprintf(e.out, "export %s=%q\n", key, val)
	}
	return nil
}

var _ Encoder = &exportableEncoder{}
