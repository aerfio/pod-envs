# podenvs

Print environment variables for specific container inside the particular pod. Extract needed data from secrets, configmaps, fieldRef and resourceFieldRef.

## Installation 

```bash
go install aerf.io/podenvs@main
```

## Usage
```
Usage of podenvs:
This binary connects to the current-context from the kubeconfig to read referenced secrets/configmaps, even if the pod is supplied by stdin.

Available flags: 
  -c, --container string   Container inside that pod from which to extract envs. Unused if there's only 1 container
  -e, --exportable         Prints envs in a format ready to copy and paste into terminal to export them
  -h, --help               print help msg
      --name string        Name of the pod. Ignored if the input is set to stdin.
      --namespace string   Namespace of the pod. Ignored if the input is set to stdin. (default "default")
  -y, --yaml               yaml output instead of json

"--exportable" flag takes precedence before "--yaml" flag. 
"--container" flag is only required if the targeted pod has more than 1 container, otherwise it's ignored.
You can pass the yaml/json representation of pod to stdin of podenvs using "-" as last argument, example:
  $ kubectl get pod -n $NAMESPACE $POD_NAME -oyaml | podenvs -
The "--name" flag is not required in this case and is ignored, along with "--namespace" flag. 
```
