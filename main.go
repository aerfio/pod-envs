package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/andreazorzetto/yh/highlight"
	"gopkg.in/yaml.v3"

	"github.com/neilotoole/jsoncolor"
	"github.com/spf13/pflag"
	"golang.org/x/exp/maps"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	envutil "k8s.io/kubectl/pkg/cmd/set/env"
	ctrl "sigs.k8s.io/controller-runtime"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var namespace, name, container string
	var prettyPrint, yamlOut, printHelp bool
	pflag.StringVar(&namespace, "namespace", "default", "namespace of the pod")
	pflag.StringVar(&name, "name", "", "namespace of the pod")
	pflag.StringVarP(&container, "container", "c", "", "container")
	pflag.BoolVarP(&prettyPrint, "pretty", "p", false, "pretty print output")
	pflag.BoolVarP(&yamlOut, "yaml", "y", false, "use yaml output")
	pflag.BoolVarP(&printHelp, "help", "h", false, "print help msg")
	pflag.Parse()

	if printHelp {
		pflag.Usage()
		os.Exit(0)
	}

	if name == "" {
		fmt.Fprintln(os.Stderr, "no 'name' flag provided")
		os.Exit(1)
	}

	kc := kubernetes.NewForConfigOrDie(ctrl.GetConfigOrDie())

	pod, err := kc.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		panic(err)
	}

	if len(pod.Spec.Containers) > 1 && container == "" {
		fmt.Fprintln(os.Stderr, "more than one container in the selected pod, you must provide 'container' flag")
		os.Exit(1)
	}

	envs := make(map[string]string)
	podContainer := findContainerWithName(pod.Spec.Containers, name)

	for _, env := range podContainer.Env {
		if env.Value != "" {
			maps.Copy(envs, map[string]string{
				env.Name: env.Value,
			})
			continue
		}
		if env.ValueFrom != nil {
			store := envutil.NewResourceStore()
			val, err := envutil.GetEnvVarRefValue(kc, namespace, store, env.ValueFrom, pod, &podContainer)
			if err != nil {
				panic(err)
			}
			maps.Copy(envs, map[string]string{
				env.Name: val,
			})
		}

		maps.Copy(envs, map[string]string{
			env.Name: "",
		})
	}

	if err := getEncoder(os.Stdout, yamlOut, prettyPrint).Encode(envs); err != nil {
		panic(err)
	}
}

func getEncoder(out io.Writer, yamlOut, pretty bool) Encoder {
	if yamlOut {
		return yamlEncoder{
			out:    out,
			pretty: pretty,
		}
	}
	return getJSONEnc(out, pretty)
}

type Encoder interface {
	Encode(arg any) error
}

type yamlEncoder struct {
	out    io.Writer
	pretty bool
}

func (y yamlEncoder) Encode(arg any) error {
	if !y.pretty {
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

func getJSONEnc(out io.Writer, pretty bool) *jsoncolor.Encoder {
	var enc *jsoncolor.Encoder
	if jsoncolor.IsColorTerminal(out) && pretty {
		enc = jsoncolor.NewEncoder(out)
		// DefaultColors are similar to jq
		clrs := jsoncolor.DefaultColors()
		enc.SetColors(clrs)
		enc.SetIndent("", "  ")
	} else {
		// Can't use color; but the encoder will still work
		enc = jsoncolor.NewEncoder(os.Stdout)
	}
	return enc
}

func findContainerWithName(containers []corev1.Container, name string) corev1.Container {
	if len(containers) == 1 {
		return containers[0]
	}

	containerNames := make([]string, 0, len(containers))
	for _, container := range containers {
		containerNames = append(containerNames, container.Name)
		if container.Name == name {
			return container
		}
	}
	panic(fmt.Errorf("there isn't %q container in the pod, list of container names: %+v", name, containerNames))
}
