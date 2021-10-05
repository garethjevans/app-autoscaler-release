package setup_test

import (
	"acceptance/config"
	"github.com/cloudfoundry-incubator/cf-test-helpers/cf"
	"github.com/cloudfoundry-incubator/cf-test-helpers/generator"
	"github.com/cloudfoundry-incubator/cf-test-helpers/workflowhelpers"
	"net/http"
	"strconv"
	"testing"
)

import (
	. "acceptance/helpers"
	_ "github.com/cloudfoundry-incubator/cf-test-helpers/workflowhelpers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
)

const (
	InitialInstanceCount = 2
)

var (
	client *http.Client
	cfg    *config.Config
)

func TestSetup(t *testing.T) {
	RegisterFailHandler(Fail)
	cfg := config.LoadConfig(t)
	testSetup := workflowhelpers.NewSmokeTestSuiteSetup(cfg)

	SynchronizedBeforeSuite(func() []byte {
		return nil
	}, func(data []byte) {
		testSetup.TestUser.Create()
		testSetup.Setup()

		workflowhelpers.AsUser(testSetup.AdminUserContext(), cfg.DefaultTimeoutDuration(), func() {
			if cfg.ShouldEnableServiceAccess() {
				EnableServiceAccess(cfg, testSetup.GetOrganizationName())
			}
		})

		appName := generator.PrefixedRandomName("autoscaler", "nodeapp-cpu")
		countStr := strconv.Itoa(InitialInstanceCount)
		createApp := cf.Cf("push", appName, "--no-start", "--no-route", "-i", countStr, "-b", cfg.NodejsBuildpackName, "-m", "128M", "-p", "../../assets/app/nodeApp").Wait(cfg.CfPushTimeoutDuration())
		Expect(createApp).To(Exit(0), "failed creating app")

		appGUID := GetAppGuid(cfg, appName)
		policy := GenerateDynamicScaleOutPolicy(cfg, 1, 2, "cpu", 90)

		Expect(cf.Cf("start", appName).Wait(cfg.CfPushTimeoutDuration())).To(Exit(0))
		CreatePolicy(cfg, appName, appGUID, policy)

	})

	RunSpecs(t, "Setup Suite")
}
