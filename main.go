package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"aerf.io/podenvs/k8s"
	"github.com/andreazorzetto/yh/highlight"
	"gopkg.in/yaml.v3"

	"github.com/neilotoole/jsoncolor"
	"github.com/spf13/pflag"
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
	pflag.StringVarP(&container, "container", "c", "", "Container inside that pod from which to extract envs. Unused if there's only 1 container")
	pflag.BoolVarP(&prettyPrint, "pretty", "p", false, "pretty print output if json")
	pflag.BoolVarP(&yamlOut, "yaml", "y", false, "switch from json to yaml output")
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

	podContainer := k8s.FindContainerWithName(pod.Spec.Containers, name)
	envs := make(map[string]string, len(podContainer.Env))

	for _, env := range podContainer.Env {
		if env.Value != "" {
			envs[env.Name] = env.Value
		} else if env.ValueFrom != nil {
			store := envutil.NewResourceStore()
			val, err := envutil.GetEnvVarRefValue(kc, namespace, store, env.ValueFrom, pod, &podContainer)
			if err != nil {
				panic(err)
			}
			envs[env.Name] = val
		} else {
			envs[env.Name] = ""
		}
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
