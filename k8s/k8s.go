package k8s

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

func MustFindContainerWithName(containers []corev1.Container, name string) corev1.Container {
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
	panic(fmt.Errorf("there is no %q container in the pod, list of container names: %+v", name, containerNames))
}
