package framework

import (
	"flag"
	"fmt"

	"k8s.io/client-go/tools/clientcmd"
)

const defaultHost = "http://127.0.0.1:8080"

type TestContextType struct {
	CertDir                  string
	DeleteNamespace          bool
	DeleteNamespaceOnFailure bool
	Host                     string
	IstioRevison             string
	KubectlPath              string
	KubeContext              string
	KubeConfig               string
	ReportDir                string
	RepoRoot                 string
}

var TestContext TestContextType

func RegisterFlags() {
	flag.StringVar(&TestContext.KubeConfig, clientcmd.RecommendedConfigPathFlag, clientcmd.RecommendedHomeFile, "Path to kubeconfig containing embedded authinfo.")
	flag.StringVar(&TestContext.ReportDir, "report-dir", "", "Path to the directory where the JUnit XML reports should be saved. Default is empty, which doesn't generate these reports.")
	flag.StringVar(&TestContext.Host, "host", "", fmt.Sprintf("The host, or apiserver, to connect to. Will default to %s if this argument and --kubeconfig are not set", defaultHost))
	flag.BoolVar(&TestContext.DeleteNamespace, "delete-namespace", true, "If true, tests will delete namespace after completion. It is only designed to make debugging easier, DO NOT turn it off by default.")
	flag.BoolVar(&TestContext.DeleteNamespaceOnFailure, "delete-namespace-on-failure", false, "If true, framework will delete test namespace on failure. Used only during test debugging.")

	flag.StringVar(&TestContext.CertDir, "cert-dir", "", "Path to the directory containing the certs. Default is empty, which doesn't use certs.")
	flag.StringVar(&TestContext.KubeContext, clientcmd.FlagContext, "", "kubeconfig context to use/override. If unset, will use value from 'current-context'")
	flag.StringVar(&TestContext.KubectlPath, "kubectl-path", "kubectl", "The kubectl binary to use. For development, you might use 'cluster/kubectl.sh' here.")
	flag.StringVar(&TestContext.RepoRoot, "repo-root", "../../", "Root directory of kubernetes repository, for finding test files.")
	flag.StringVar(&TestContext.IstioRevison, "istio-revision", "1-10-2", "Istio revision to e2e test. The default value is 1-10-2.")
}
