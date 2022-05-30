package e2e

// the values normally you do not need to change
var (
	testResourceToDelete []*TestResource
	svfGroup             = "microservice.slime.io"
	svfVersion           = "v1alpha1"
	svfResource          = "servicefences"
	svfName              = "productpage"
	sidecarGroup         = "networking.istio.io"
	sidecarVersion       = "v1beta1"
	sidecarResource      = "sidecars"
	sidecarName          = "productpage"
	istioRevKey          = "istio.io/rev"
)

// the values you can change
// these are only example values, you can change them according to your situation
// you can also use env to override these default values avoiding changing values.go all the time, see func substituteValue in lazyload_test.go
// latest image tag link https://github.com/slime-io/slime/wiki/Slime-Project-Tag-and-Image-Tag-Mapping-Table
var (
	nsSlime               = "mesh-operator"                     // namespace deployed slime
	nsApps                = "example-apps"                      // namespace deployed demo apps
	test                  = "test/e2e/testdata/install"         // testdata path
	slimebootName         = "slime-boot"                        // consistent with your deployment_slime-boot.yaml
	istioRevValue         = "1-10-2"                            // istio revision
	slimebootTag          = "v0.2.3-5bf313f"                    // slime-boot image tag, using the link above as reference
	lazyloadTag           = "master-2946c4a"                    // lazyload image tag, using the link above as reference
	globalSidecarTag      = "1.7.0"                             // global-sidecar image tag, using the link above as reference
	globalSidecarPilotTag = "globalPilot-7.0-v0.0.3-713c611962" // global-sidecar-pilot image tag, using the link above as reference
)

type TestResource struct {
	Namespace string
	Contents  string
	Selectors []string
}
