package system_test

import (
	"fmt"
	"io/ioutil"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

type cluster struct {
	context string
}

var (
	promisePath              = "./assets/bash-promise/promise.yaml"
	promiseWithSelectorsPath = "./assets/bash-promise/promise-with-selectors.yaml"

	workerCtx = "--context=kind-worker"
	platCtx   = "--context=kind-platform"

	timeout  = time.Second * 90
	interval = time.Second * 2

	platform = cluster{context: "--context=kind-platform"}
	worker   = cluster{context: "--context=kind-worker"}
)

var baseRequestYAML = `apiVersion: test.kratix.io/v1alpha1
kind: bash
metadata:
  name: %s
spec:
  container0Cmd: |
    %s
  container1Cmd: |
    %s`

var _ = Describe("Kratix", func() {
	BeforeSuite(func() {
		initK8sClient()
	})

	Describe("Promise lifecycle", func() {
		It("successfully manages the promise lifecycle", func() {
			By("installing the promise", func() {
				platform.kubectl("apply", "-f", promisePath)

				platform.eventuallyKubectl("get", "crd", "bash.test.kratix.io")
				worker.eventuallyKubectl("get", "namespace", "bash-wcr-namespace")
			})

			By("deleting a promise", func() {
				platform.kubectl("delete", "promise", "bash")

				Eventually(func(g Gomega) {
					g.Expect(worker.kubectl("get", "namespace")).NotTo(ContainSubstring("bash-wcr-namespace"))
					g.Expect(platform.kubectl("get", "promise")).ShouldNot(ContainSubstring("bash"))
					g.Expect(platform.kubectl("get", "crd")).ShouldNot(ContainSubstring("bash"))
				}, timeout, interval).Should(Succeed())
			})
		})

		Describe("Resource requests", func() {
			BeforeEach(func() {
				platform.kubectl("apply", "-f", promisePath)
				platform.eventuallyKubectl("get", "crd", "bash.test.kratix.io")
			})

			It("deploys the contents of /output to the worker cluster", func() {
				rrName := "rr-output"
				command := `kubectl create namespace resource-request-namespace --dry-run=client -oyaml > /output/ns.yaml`
				platform.kubectl("apply", "-f", requestWithNameAndCommand(rrName, command))

				platform.kubectl("wait", "--for=condition=PipelineCompleted", "bash", rrName, "--timeout=60s")
				Expect(platform.kubectl("get", "bash", rrName)).To(ContainSubstring("Resource requested"))
				worker.eventuallyKubectl("get", "namespace", "resource-request-namespace")
			})

			It("writes to status the contents of /metadata/status.yaml", func() {
				rrName := "rr-status"
				command := `echo "message: My awesome status message" > /metadata/status.yaml
							echo "key: value" >> /metadata/status.yaml`

				platform.kubectl("apply", "-f", requestWithNameAndCommand(rrName, command))

				platform.kubectl("wait", "--for=condition=PipelineCompleted", "bash", rrName, "--timeout=60s")

				Expect(platform.kubectl("get", "bash", rrName)).To(ContainSubstring("My awesome status message"))
				Expect(platform.kubectl("get", "bash", rrName, "-o", "jsonpath='{.status.key}'")).To(ContainSubstring("value"))
			})

			It("runs all the containers in the pipeline", func() {
				rrName := "rr-multi-container"
				commands := []string{
					`kubectl create namespace mcns --dry-run=client -oyaml > /output/ns.yaml`,
					`kubectl create configmap multi-container-config --namespace mcns --dry-run=client -oyaml > /output/configmap.yaml`,
				}

				platform.kubectl("apply", "-f", requestWithNameAndCommand(rrName, commands...))

				platform.kubectl("wait", "--for=condition=PipelineCompleted", "bash", rrName, "--timeout=60s")

				worker.eventuallyKubectl("get", "namespace", "mcns")
				worker.eventuallyKubectl("get", "configmap", "multi-container-config", "--namespace", "mcns")
			})

			It("can be deleted", func() {
				rrName := "rr-to-delete"
				command := `kubectl create namespace mcns --dry-run=client -oyaml > /output/ns.yaml`
				platform.kubectl("apply", "-f", requestWithNameAndCommand(rrName, command))
				platform.kubectl("wait", "--for=condition=PipelineCompleted", "bash", rrName, "--timeout=60s")

				platform.kubectl("delete", "bash", rrName)

				Eventually(func(g Gomega) {
					g.Expect(platform.kubectl("get", "bash")).NotTo(ContainSubstring(rrName))
					g.Expect(worker.kubectl("get", "namespace")).NotTo(ContainSubstring("mcns"))
				}, timeout, interval).Should(Succeed())
			})

			AfterEach(func() {
				platform.kubectl("delete", "promise", "bash")
				Eventually(platform.kubectl("get", "promise")).ShouldNot(ContainSubstring("bash"))
			})
		})
	})

	Describe("Scheduling", func() {
		// Worker cluster:
		// - environment: dev
		// - security: high

		// Platform cluster:
		// - environment: platform
		// - security: high

		// PromiseClusterSelectors:
		// - security: high
		BeforeEach(func() {
			platform.kubectl("label", "cluster", "worker-cluster-1", "security=high")
			//install kustomization and buckets to plat cluster
			platform.kubectl("apply", "-f", "./assets/platform_gitops-tk-resources.yaml")
		})

		It("schedules resources to the correct clusters", func() {
			By("installing the promise", func() {
				platform.kubectl("apply", "-f", promiseWithSelectorsPath)

				platform.eventuallyKubectl("get", "crd", "bash.test.kratix.io")

				By("only the worker cluster getting the WCR", func() {
					worker.eventuallyKubectl("get", "namespace", "bash-wcr-namespace")
					Expect(platform.kubectl("get", "namespace")).NotTo(ContainSubstring("bash-wcr-namespace"))
				})

				By("registering the plaform cluster and it getting the WCR assigned", func() {
					platform.kubectl("apply", "-f", "./assets/platform_kratix_cluster.yaml")
					platform.eventuallyKubectl("get", "namespace", "bash-wcr-namespace")
				})
			})

			By("setting up cluster selectors", func() {
				command := `echo "pci: true" > /metadata/cluster-selectors.yaml
				kubectl create namespace rr-2-namespace --dry-run=client -oyaml > /output/ns.yaml`
				platform.kubectl("apply", "-f", requestWithNameAndCommand("rr-2", command))

				platform.kubectl("wait", "--for=condition=PipelineCompleted", "bash", "rr-2", "--timeout=60s")

				By("only scheduling the work when a cluster label matches", func() {
					Consistently(func() string {
						return platform.kubectl("get", "namespace") + "\n" + worker.kubectl("get", "namespace")
					}, "10s").ShouldNot(ContainSubstring("rr-2-namespace"))

					platform.kubectl("label", "cluster", "worker-cluster-1", "pci=true")

					worker.eventuallyKubectl("get", "namespace", "rr-2-namespace")
				})
			})
		})
	})
})

func requestWithNameAndCommand(name string, containerCmds ...string) string {
	normalisedCmds := make([]string, 2)
	for i := range normalisedCmds {
		cmd := "cp /input/* /output;"
		if len(containerCmds) > i {
			cmd += " " + containerCmds[i]
		}
		normalisedCmds[i] = strings.ReplaceAll(cmd, "\n", ";")
	}

	lci := len(normalisedCmds) - 1
	lastCommand := normalisedCmds[lci]
	if strings.HasSuffix(normalisedCmds[lci], ";") {
		lastCommand = lastCommand[:len(lastCommand)-1]
	}
	normalisedCmds[lci] = lastCommand + "; rm /output/object.yaml"

	file, err := ioutil.TempFile("", "kratix-test")
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	args := []interface{}{name}
	for _, cmd := range normalisedCmds {
		args = append(args, cmd)
	}

	contents := fmt.Sprintf(baseRequestYAML, args...)
	fmt.Fprint(GinkgoWriter, "Resource Request:")
	fmt.Fprint(GinkgoWriter, contents)

	ExpectWithOffset(1, ioutil.WriteFile(file.Name(), []byte(contents), 644)).NotTo(HaveOccurred())

	return file.Name()
}

// run a command until it exits 0
func (c cluster) eventuallyKubectl(args ...string) string {
	args = append(args, c.context)
	var content string
	EventuallyWithOffset(1, func(g Gomega) {
		command := exec.Command("kubectl", args...)
		session, err := gexec.Start(command, GinkgoWriter, GinkgoWriter)
		g.ExpectWithOffset(1, err).ShouldNot(HaveOccurred())
		g.EventuallyWithOffset(1, session).Should(gexec.Exit(0))
		content = string(session.Out.Contents())
	}, timeout, interval).Should(Succeed(), strings.Join(args, " "))
	return content
}

// run command and return stdout. Errors if exit code non-zero
func (c cluster) kubectl(args ...string) string {
	args = append(args, c.context)
	command := exec.Command("kubectl", args...)
	session, err := gexec.Start(command, GinkgoWriter, GinkgoWriter)
	ExpectWithOffset(1, err).ShouldNot(HaveOccurred())
	EventuallyWithOffset(1, session, timeout, interval).Should(gexec.Exit(0))
	return string(session.Out.Contents())
}
