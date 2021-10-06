package pre_upgrade_test

import (
	"acceptance/config"
	"acceptance/helpers"
	"fmt"
	"github.com/cloudfoundry-incubator/cf-test-helpers/cf"
	"github.com/cloudfoundry-incubator/cf-test-helpers/workflowhelpers"
	"net/http"
	"testing"
)

import (
	_ "github.com/cloudfoundry-incubator/cf-test-helpers/workflowhelpers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
)

var (
	client *http.Client
	cfg    *config.Config
	setup  *workflowhelpers.ReproducibleTestSuiteSetup
)

func TestSetup(t *testing.T) {
	RegisterFailHandler(Fail)
	cfg = config.LoadConfig(t)
	setup = workflowhelpers.NewSmokeTestSuiteSetup(cfg)
	RunSpecs(t, "Post Upgrade Test Suite")
}

var _ = BeforeSuite(func() {

	setup = workflowhelpers.NewTestSuiteSetup(cfg)

	workflowhelpers.AsUser(setup.AdminUserContext(), cfg.DefaultTimeoutDuration(), func() {
		orgs := helpers.GetTestOrgs(cfg)
		Expect(len(orgs)).To(Equal(1), "there should be one org at this point")
	})
})

var _ = AfterSuite(func() {
	fmt.Println("Clearing down existing test orgs/spaces...")
	setup = workflowhelpers.NewTestSuiteSetup(cfg)

	workflowhelpers.AsUser(setup.AdminUserContext(), cfg.DefaultTimeoutDuration(), func() {
		orgs := helpers.GetTestOrgs(cfg)

		for _, org := range orgs {
			orgName, orgGuid, spaceName, spaceGuid := helpers.GetOrgSpaceNamesAndGuids(cfg, org)
			if spaceName != "" {
				target := cf.Cf("target", "-o", orgName, "-s", spaceName).Wait(cfg.DefaultTimeoutDuration())
				Expect(target).To(Exit(0), fmt.Sprintf("failed to target %s and %s", orgName, spaceName))

				apps := helpers.GetApps(cfg, orgGuid, spaceGuid, "autoscaler-")
				helpers.DeleteApps(cfg, apps, 0)

				services := helpers.GetServices(cfg, orgGuid, spaceGuid, "autoscaler-")
				helpers.DeleteServices(cfg, services)
			}

			helpers.DeleteOrg(cfg, org)
		}
	})

	fmt.Println("Clearing down existing test orgs/spaces... Complete")
})
