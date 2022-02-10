package common

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"text/template"

	"gopkg.in/yaml.v2"

	"slime.io/slime/framework/test/e2e/framework"
)

var testImages testImagesStruct

type testImagesStruct struct {
	AgnhostImage  string
	BusyBoxImage  string
	KittenImage   string
	NautilusImage string
	NginxImage    string
	NginxNewImage string
	HttpdImage    string
	HttpdNewImage string
	PauseImage    string
	RedisImage    string
}

func init() {
	testImages = testImagesStruct{
		GetE2EImage(Agnhost),
		GetE2EImage(BusyBox),
		GetE2EImage(Kitten),
		GetE2EImage(Nautilus),
		GetE2EImage(Nginx),
		GetE2EImage(NginxNew),
		GetE2EImage(Httpd),
		GetE2EImage(HttpdNew),
		GetE2EImage(Pause),
		GetE2EImage(Redis),
	}
}

func initReg() RegistryList {
	registry := RegistryList{
		GcAuthenticatedRegistry: "gcr.io/authenticated-image-pulling",
		DockerLibraryRegistry:   "docker.io/library",
		DockerGluster:           "docker.io/gluster",
		E2eRegistry:             "gcr.io/kubernetes-e2e-test-images",
		E2eVolumeRegistry:       "gcr.io/kubernetes-e2e-test-images/volume",
		PromoterE2eRegistry:     "k8s.gcr.io/e2e-test-images",
		BuildImageRegistry:      "k8s.gcr.io/build-image",
		InvalidRegistry:         "invalid.com/invalid",
		GcRegistry:              "k8s.gcr.io",
		SigStorageRegistry:      "k8s.gcr.io/sig-storage",
		GcrReleaseRegistry:      "gcr.io/gke-release",
		PrivateRegistry:         "gcr.io/k8s-authenticated-test",
		SampleRegistry:          "gcr.io/google-samples",
	}
	repoList := os.Getenv("KUBE_TEST_REPO_LIST")
	if repoList == "" {
		return registry
	}

	fileContent, err := ioutil.ReadFile(repoList)
	if err != nil {
		panic(fmt.Errorf("Error reading '%v' file contents: %v", repoList, err))
	}

	err = yaml.Unmarshal(fileContent, &registry)
	if err != nil {
		panic(fmt.Errorf("Error unmarshalling '%v' YAML file: %v", repoList, err))
	}
	return registry
}

// RegistryList holds public and private image registries
type RegistryList struct {
	GcAuthenticatedRegistry string `yaml:"gcAuthenticatedRegistry"`
	DockerLibraryRegistry   string `yaml:"dockerLibraryRegistry"`
	DockerGluster           string `yaml:"dockerGluster"`
	E2eRegistry             string `yaml:"e2eRegistry"`
	E2eVolumeRegistry       string `yaml:"e2eVolumeRegistry"`
	PromoterE2eRegistry     string `yaml:"promoterE2eRegistry"`
	BuildImageRegistry      string `yaml:"buildImageRegistry"`
	InvalidRegistry         string `yaml:"invalidRegistry"`
	GcRegistry              string `yaml:"gcRegistry"`
	SigStorageRegistry      string `yaml:"sigStorageRegistry"`
	GcrReleaseRegistry      string `yaml:"gcrReleaseRegistry"`
	PrivateRegistry         string `yaml:"privateRegistry"`
	SampleRegistry          string `yaml:"sampleRegistry"`
}

// SubstituteImageName replaces image name in content.
func SubstituteImageName(content string) string {
	contentWithImageName := new(bytes.Buffer)
	tmpl, err := template.New("imagemanifest").Parse(content)
	if err != nil {
		framework.Failf("Failed Parse the template: %v", err)
	}
	err = tmpl.Execute(contentWithImageName, testImages)
	if err != nil {
		framework.Failf("Failed executing template: %v", err)
	}
	return contentWithImageName.String()
}

// GetE2EImage returns the fully qualified URI to an image (including version)
func GetE2EImage(image int) string {
	return fmt.Sprintf("%s/%s:%s", imageConfigs[image].registry, imageConfigs[image].name, imageConfigs[image].version)
}

var (
	registry                = initReg()
	dockerLibraryRegistry   = registry.DockerLibraryRegistry
	dockerGluster           = registry.DockerGluster
	e2eRegistry             = registry.E2eRegistry
	e2eVolumeRegistry       = registry.E2eVolumeRegistry
	promoterE2eRegistry     = registry.PromoterE2eRegistry
	buildImageRegistry      = registry.BuildImageRegistry
	gcAuthenticatedRegistry = registry.GcAuthenticatedRegistry
	gcRegistry              = registry.GcRegistry
	sigStorageRegistry      = registry.SigStorageRegistry
	gcrReleaseRegistry      = registry.GcrReleaseRegistry
	invalidRegistry         = registry.InvalidRegistry
	// PrivateRegistry is an image repository that requires authentication
	PrivateRegistry = registry.PrivateRegistry
	sampleRegistry  = registry.SampleRegistry

	// Preconfigured image configs
	imageConfigs = initImageConfigs()
)

const (
	// None is to be used for unset/default images
	None = iota
	// Agnhost image
	Agnhost
	// AgnhostPrivate image
	AgnhostPrivate
	// APIServer image
	APIServer
	// AppArmorLoader image
	AppArmorLoader
	// AuthenticatedAlpine image
	AuthenticatedAlpine
	// AuthenticatedWindowsNanoServer image
	AuthenticatedWindowsNanoServer
	// BusyBox image
	BusyBox
	// CheckMetadataConcealment image
	CheckMetadataConcealment
	// CudaVectorAdd image
	CudaVectorAdd
	// CudaVectorAdd2 image
	CudaVectorAdd2
	// DebianIptables Image
	DebianIptables
	// EchoServer image
	EchoServer
	// Etcd image
	Etcd
	// GlusterDynamicProvisioner image
	GlusterDynamicProvisioner
	// Httpd image
	Httpd
	// HttpdNew image
	HttpdNew
	// InvalidRegistryImage image
	InvalidRegistryImage
	// IpcUtils image
	IpcUtils
	// JessieDnsutils image
	JessieDnsutils
	// Kitten image
	Kitten
	// Nautilus image
	Nautilus
	// NFSProvisioner image
	NFSProvisioner
	// Nginx image
	Nginx
	// NginxNew image
	NginxNew
	// Nonewprivs image
	Nonewprivs
	// NonRoot runs with a default user of 1234
	NonRoot
	// Pause - when these values are updated, also update cmd/kubelet/app/options/container_runtime.go
	// Pause image
	Pause
	// Perl image
	Perl
	// PrometheusDummyExporter image
	PrometheusDummyExporter
	// PrometheusToSd image
	PrometheusToSd
	// Redis image
	Redis
	// RegressionIssue74839 image
	RegressionIssue74839
	// ResourceConsumer image
	ResourceConsumer
	// SdDummyExporter image
	SdDummyExporter
	// VolumeNFSServer image
	VolumeNFSServer
	// VolumeISCSIServer image
	VolumeISCSIServer
	// VolumeGlusterServer image
	VolumeGlusterServer
	// VolumeRBDServer image
	VolumeRBDServer
)

// Config holds an images registry, name, and version
type Config struct {
	registry string
	name     string
	version  string
}

func initImageConfigs() map[int]Config {
	configs := map[int]Config{}
	configs[Agnhost] = Config{promoterE2eRegistry, "agnhost", "2.20"}
	configs[AgnhostPrivate] = Config{PrivateRegistry, "agnhost", "2.6"}
	configs[AuthenticatedAlpine] = Config{gcAuthenticatedRegistry, "alpine", "3.7"}
	configs[AuthenticatedWindowsNanoServer] = Config{gcAuthenticatedRegistry, "windows-nanoserver", "v1"}
	configs[APIServer] = Config{e2eRegistry, "sample-apiserver", "1.17"}
	configs[AppArmorLoader] = Config{e2eRegistry, "apparmor-loader", "1.0"}
	configs[BusyBox] = Config{dockerLibraryRegistry, "busybox", "1.29"}
	configs[CheckMetadataConcealment] = Config{promoterE2eRegistry, "metadata-concealment", "1.6"}
	configs[CudaVectorAdd] = Config{e2eRegistry, "cuda-vector-add", "1.0"}
	configs[CudaVectorAdd2] = Config{e2eRegistry, "cuda-vector-add", "2.0"}
	configs[DebianIptables] = Config{buildImageRegistry, "debian-iptables", "buster-v1.6.5"}
	configs[EchoServer] = Config{e2eRegistry, "echoserver", "2.2"}
	configs[Etcd] = Config{gcRegistry, "etcd", "3.4.13-0"}
	configs[GlusterDynamicProvisioner] = Config{dockerGluster, "glusterdynamic-provisioner", "v1.0"}
	configs[Httpd] = Config{dockerLibraryRegistry, "httpd", "2.4.38-alpine"}
	configs[HttpdNew] = Config{dockerLibraryRegistry, "httpd", "2.4.39-alpine"}
	configs[InvalidRegistryImage] = Config{invalidRegistry, "alpine", "3.1"}
	configs[IpcUtils] = Config{e2eRegistry, "ipc-utils", "1.0"}
	configs[JessieDnsutils] = Config{e2eRegistry, "jessie-dnsutils", "1.0"}
	configs[Kitten] = Config{e2eRegistry, "kitten", "1.0"}
	configs[Nautilus] = Config{e2eRegistry, "nautilus", "1.0"}
	configs[NFSProvisioner] = Config{sigStorageRegistry, "nfs-provisioner", "v2.2.2"}
	configs[Nginx] = Config{dockerLibraryRegistry, "nginx", "1.14-alpine"}
	configs[NginxNew] = Config{dockerLibraryRegistry, "nginx", "1.15-alpine"}
	configs[Nonewprivs] = Config{e2eRegistry, "nonewprivs", "1.0"}
	configs[NonRoot] = Config{e2eRegistry, "nonroot", "1.0"}
	// Pause - when these values are updated, also update cmd/kubelet/app/options/container_runtime.go
	configs[Pause] = Config{gcRegistry, "pause", "3.2"}
	configs[Perl] = Config{dockerLibraryRegistry, "perl", "5.26"}
	configs[PrometheusDummyExporter] = Config{gcRegistry, "prometheus-dummy-exporter", "v0.1.0"}
	configs[PrometheusToSd] = Config{gcRegistry, "prometheus-to-sd", "v0.5.0"}
	configs[Redis] = Config{dockerLibraryRegistry, "redis", "5.0.5-alpine"}
	configs[RegressionIssue74839] = Config{e2eRegistry, "regression-issue-74839-amd64", "1.0"}
	configs[ResourceConsumer] = Config{e2eRegistry, "resource-consumer", "1.5"}
	configs[SdDummyExporter] = Config{gcRegistry, "sd-dummy-exporter", "v0.2.0"}
	configs[VolumeNFSServer] = Config{e2eVolumeRegistry, "nfs", "1.0"}
	configs[VolumeISCSIServer] = Config{e2eVolumeRegistry, "iscsi", "2.0"}
	configs[VolumeGlusterServer] = Config{e2eVolumeRegistry, "gluster", "1.0"}
	configs[VolumeRBDServer] = Config{e2eVolumeRegistry, "rbd", "1.0.1"}
	return configs
}
