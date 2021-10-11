package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	framework2 "slime.io/slime/slime-framework/test/e2e/framework"
	"slime.io/slime/slime-framework/test/e2e/framework/testfiles"
	"testing"

	"github.com/golang/glog"
	"github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/config"
	"github.com/onsi/ginkgo/reporters"
	"github.com/onsi/gomega"
)

func init() {
	framework2.RegisterFlags()
	if framework2.TestContext.RepoRoot != "" {
		testfiles.AddFileSource(testfiles.RootFileSource{Root: framework2.TestContext.RepoRoot})
	}
}

func TestE2E(t *testing.T) {
	RunE2ETests(t)
}

func RunE2ETests(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)

	var r []ginkgo.Reporter
	var ReportDir = "reports"

	if framework2.TestContext.ReportDir != "" {
		ReportDir = framework2.TestContext.ReportDir
	}

	if err := os.Mkdir(ReportDir, os.ModePerm); err != nil && !os.IsExist(err) {
		glog.Fatalf("Failed creating report directory %s ", ReportDir)
	}

	r = append(r, reporters.NewJUnitReporter(filepath.Join(ReportDir, fmt.Sprintf("service_%02d.xml", config.GinkgoConfig.ParallelNode))))

	framework2.Logf("Starting e2e run %q on ginkgo node %d \n", framework2.RunId, config.GinkgoConfig.ParallelNode)
	ginkgo.RunSpecsWithDefaultAndCustomReporters(t, "e2e test suite", r)
}
