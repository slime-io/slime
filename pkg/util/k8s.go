package util

import "strings"

func PodNameToDeployName(name string) string {
	if strings.LastIndex(name, "-") != -1 {
		name = name[:strings.LastIndex(name, "-")]
	}
	if strings.LastIndex(name, "-") != -1 {
		name = name[:strings.LastIndex(name, "-")]
	}
	return name
}
