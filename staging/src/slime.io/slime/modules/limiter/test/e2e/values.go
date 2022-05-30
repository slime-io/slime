package e2e

// the values normally you do not need to change
var (
	testResourceToDelete []*TestResource
	slGroup              = "microservice.slime.io"
	slVersion            = "v1alpha2"
	slResource           = "smartlimiters"

	efGroup    = "networking.istio.io"
	efVersion  = "v1alpha3"
	efResource = "envoyfilters"

	istioRevKey = "istio.io/rev"
)

// the values you can change
// these are only example values, you can change them according to your situation
// you can also use env to override these default values avoiding changing values.go all the time, see func substituteValue in lazyload_test.go
// latest image tag link https://github.com/slime-io/slime/wiki/Slime-Project-Tag-and-Image-Tag-Mapping-Table
var (
	nsSlime       = "mesh-operator"             // namespace deployed slime
	nsApps        = "temp"                      // namespace deployed demo apps
	test          = "test/e2e/testdata/install" // testdata path
	slimebootName = "slime-boot"                // consistent with your deployment_slime-boot.yaml
	istioRevValue = "1-10-2"                    // istio revision
	slimebootTag  = "v0.2.3-5bf313f"            // slime-boot image tag, using the link above as reference
	limitTag      = "v0.4"                      // limit image tag, using the link above as reference
)

type TestResource struct {
	Namespace string
	Contents  string
	Selectors []string
}
