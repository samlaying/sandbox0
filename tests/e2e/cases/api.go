package cases

import (
	"github.com/sandbox0-ai/infra/tests/e2e/framework"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// RegisterApiSuite defines API coverage for a scenario.
func RegisterApiSuite(envProvider func() *framework.ScenarioEnv) {
	Describe("API entrypoint", func() {
		BeforeEach(func() {
			env := envProvider()
			Expect(env).NotTo(BeNil())
		})

		Context("template lifecycle", func() {
			It("creates, updates, and deletes templates", func() {
				Skip("TODO: implement template lifecycle via API")
			})
		})

		Context("sandbox lifecycle", func() {
			It("claims, releases, and destroys sandboxes", func() {
				Skip("TODO: implement sandbox lifecycle via API")
			})
		})

		Context("snapshot and restore", func() {
			It("restores from snapshot with consistent data", func() {
				Skip("TODO: implement snapshot restore via API")
			})
		})

		Context("filesystem and process capabilities", func() {
			It("performs file operations and command execution", func() {
				Skip("TODO: implement filesystem and process APIs")
			})
		})

		Context("concurrency and isolation", func() {
			It("prevents conflicts across concurrent users", func() {
				Skip("TODO: implement concurrent claim isolation")
			})
		})

		Context("key SLOs", func() {
			It("meets cold start and restore latency targets", func() {
				Skip("TODO: implement latency assertions")
			})
		})
	})
}
