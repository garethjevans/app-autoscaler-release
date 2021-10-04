package setup_test

import (
	"github.com/cloudfoundry-incubator/cf-test-helpers/workflowhelpers"
	"testing"
	. "acceptance/helpers"
	"acceptance/config"
	_ "github.com/cloudfoundry-incubator/cf-test-helpers/workflowhelpers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestSetup(t *testing.T) {
	RegisterFailHandler(Fail)
	testConfig := config.LoadConfig(t)
	testSetup := workflowhelpers.NewSmokeTestSuiteSetup(testConfig)

	SynchronizedBeforeSuite(func() []byte {
		return nil
	}, func(data []byte) {
		testSetup.TestUser.Create()
		testSetup.Setup()

		workflowhelpers.AsUser(testSetup.AdminUserContext(), testConfig.DefaultTimeoutDuration(), func() {
			if testConfig.ShouldEnableServiceAccess() {
				EnableServiceAccess(testConfig, testSetup.GetOrganizationName())
			}
		})

	})

	RunSpecs(t, "Setup Suite")
}
