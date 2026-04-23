/*
 * This file is part of the KubeVirt project
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * Copyright The KubeVirt Authors.
 *
 */

package cgroup

import (
	"os"
	"path"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	runc_cgroups "github.com/opencontainers/runc/libcontainer/cgroups"
	runc_configs "github.com/opencontainers/runc/libcontainer/configs"
	"github.com/opencontainers/runc/libcontainer/devices"
	"go.uber.org/mock/gomock"

	v1 "kubevirt.io/api/core/v1"

	"kubevirt.io/kubevirt/pkg/safepath"
	"kubevirt.io/kubevirt/pkg/unsafepath"
	"kubevirt.io/kubevirt/pkg/virt-handler/isolation"
)

var _ = Describe("cgroup manager", func() {

	var (
		ctrl                  *gomock.Controller
		rulesDefined          []*devices.Rule
		v2DirPath             string
		subsystemPathsDefined map[string]string
	)

	newMockManagerFromCtrl := func(ctrl *gomock.Controller, version CgroupVersion) (Manager, error) {
		mockRuncCgroupManager := NewMockruncManager(ctrl)
		mockRuncCgroupManager.EXPECT().GetPaths().DoAndReturn(func() map[string]string {
			paths := make(map[string]string)

			// See documentation here for more info: https://github.com/opencontainers/runc/blob/release-1.0/libcontainer/cgroups/cgroups.go#L48
			if version == V1 {
				paths["devices"] = "/sys/fs/cgroup/devices"
			} else {
				paths[""] = v2DirPath
			}

			return paths
		}).AnyTimes()

		execVirtChrootFunc := func(r *runc_configs.Resources, subsystemPaths map[string]string, rootless bool, version CgroupVersion) error {
			rulesDefined = r.Devices
			subsystemPathsDefined = subsystemPaths
			return nil
		}

		getCurrentlyDefinedRulesFunc := func(runcManager runc_cgroups.Manager) ([]*devices.Rule, error) {
			return rulesDefined, nil
		}

		if version == V1 {
			return newCustomizedV1Manager(mockRuncCgroupManager, false, execVirtChrootFunc, getCurrentlyDefinedRulesFunc)
		} else {
			return newCustomizedV2Manager(mockRuncCgroupManager, false, nil, execVirtChrootFunc)
		}
	}

	newMockManager := func(version CgroupVersion) (Manager, error) {
		return newMockManagerFromCtrl(ctrl, version)
	}

	newResourcesWithRule := func(rule *devices.Rule) *runc_configs.Resources {
		return &runc_configs.Resources{
			Devices: []*devices.Rule{
				rule,
			},
		}
	}

	newDeviceRule := func(UID int64) *devices.Rule {
		return &devices.Rule{
			Type:        'z',
			Major:       UID,
			Minor:       UID,
			Permissions: "fakePermissions",
			Allow:       true,
		}
	}

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		rulesDefined = make([]*devices.Rule, 0)
		v2DirPath = "/sys/fs/cgroup/"
	})

	AfterEach(func() {
		v2DirPath = ""
	})

	DescribeTable("ensure that default rules are added", func(version CgroupVersion) {
		manager, err := newMockManager(version)
		Expect(err).ShouldNot(HaveOccurred())

		fakeRule := newDeviceRule(123)

		err = manager.Set(newResourcesWithRule(fakeRule))
		Expect(err).ShouldNot(HaveOccurred())

		Expect(rulesDefined).To(ContainElement(fakeRule), "defined rule is expected to exist")

		defaultDeviceRules := GenerateDefaultDeviceRules()
		for _, defaultRule := range defaultDeviceRules {
			Expect(rulesDefined).To(ContainElement(defaultRule), "default rules are expected to be defined")
		}
		Expect(rulesDefined).To(HaveLen(len(defaultDeviceRules) + 1))
	},
		Entry("for v1", V1),
		Entry("for v2", V2),
	)

	DescribeTable("ensure that past rules are not overridden", func(version CgroupVersion) {
		manager, err := newMockManager(version)
		Expect(err).ShouldNot(HaveOccurred())

		fakeRule1 := newDeviceRule(123)
		fakeRule2 := newDeviceRule(456)

		err = manager.Set(newResourcesWithRule(fakeRule1))
		Expect(err).ShouldNot(HaveOccurred())

		err = manager.Set(newResourcesWithRule(fakeRule2))
		Expect(err).ShouldNot(HaveOccurred())

		Expect(rulesDefined).To(ContainElement(fakeRule1), "previous rule is expected to not be overridden")

	},
		Entry("for v1", V1),
		Entry("for v2", V2),
	)

	DescribeTable("ensure that past rules are overridden if explicitly set", func(version CgroupVersion) {
		manager, err := newMockManager(version)
		Expect(err).ShouldNot(HaveOccurred())

		fakeRule := newDeviceRule(123)
		fakeRule.Permissions = "fake-permissions-123"

		err = manager.Set(newResourcesWithRule(fakeRule))
		Expect(err).ShouldNot(HaveOccurred())
		Expect(rulesDefined).To(ContainElement(fakeRule), "defined rule is expected to exist")

		fakeRule.Permissions = "fake-permissions-456"
		Expect(rulesDefined).To(ContainElement(fakeRule), "rule needs to be overridden since explicitly re-set")

	},
		Entry("for v1", V1),
		Entry("for v2", V2),
	)

	DescribeTable("ensure that correct set of cgroups is configured", func(dirPath string, expectedPaths []string) {
		v2DirPath = dirPath
		manager, err := newMockManager(V2)
		Expect(err).ShouldNot(HaveOccurred())

		fakeRule := newDeviceRule(123)

		err = manager.Set(newResourcesWithRule(fakeRule))
		Expect(err).ShouldNot(HaveOccurred())

		Expect(rulesDefined).To(ContainElement(fakeRule), "defined rule is expected to exist")

		defaultDeviceRules := GenerateDefaultDeviceRules()
		for _, defaultRule := range defaultDeviceRules {
			Expect(rulesDefined).To(ContainElement(defaultRule), "default rules are expected to be defined")
		}
		Expect(rulesDefined).To(HaveLen(len(defaultDeviceRules) + 1))
		Expect(subsystemPathsDefined).To(ConsistOf(expectedPaths))
	},
		Entry("for crun installation",
			"/sys/fs/cgroup/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod123.slice/crio-456.scope/container",
			[]string{
				"/sys/fs/cgroup/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod123.slice/crio-456.scope/container",
				"/sys/fs/cgroup/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod123.slice/crio-456.scope",
			},
		),
		Entry("for runc installation",
			"/sys/fs/cgroup/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod123.slice/crio-456.scope",
			[]string{
				"/sys/fs/cgroup/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod123.slice/crio-456.scope",
			},
		),
	)
})

var _ = Describe("GetMiscCapacity", func() {
	var originalMiscCapacityPath string
	var tempDir string

	BeforeEach(func() {
		tempDir = GinkgoT().TempDir()
		originalMiscCapacityPath = miscCapacityPath
		miscCapacityPath = path.Join(tempDir, "misc.capacity")
	})

	AfterEach(func() {
		miscCapacityPath = originalMiscCapacityPath
	})

	DescribeTable("should return correct capacity",
		func(fileContent string, key string, expectedCapacity int, expectError bool) {
			if fileContent != "" {
				err := os.WriteFile(path.Join(tempDir, "misc.capacity"), []byte(fileContent), 0644)
				Expect(err).ToNot(HaveOccurred())
			}
			capacity, err := GetMiscCapacity(key)
			Expect(capacity).To(Equal(expectedCapacity))
			if expectError {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
			}
		},
		Entry("returns capacity for matching key",
			"tdx 10\nsev 5\n", "tdx", 10, false,
		),
		Entry("produces error when key not found",
			"tdx 10\nsev 5\n", "nonexistent", 0, true,
		),
		Entry("produces error for malformed line",
			"tdx\n", "tdx", 0, true,
		),
		Entry("produces error for non-numeric capacity",
			"tdx abc\n", "tdx", 0, true,
		),
	)
})

var _ = Describe("generateDeviceRulesForVMI", func() {
	var (
		ctrl    *gomock.Controller
		tempDir string
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		tempDir = GinkgoT().TempDir()
		Expect(os.MkdirAll(filepath.Join(tempDir, "dev"), 0755)).To(Succeed())
	})

	newMockIsolationWithMountRoot := func() isolation.IsolationResult {
		mountRoot, err := safepath.NewPathNoFollow(tempDir)
		Expect(err).ToNot(HaveOccurred())

		mockIso := isolation.NewMockIsolationResult(ctrl)
		mockIso.EXPECT().MountRoot().Return(mountRoot, nil)
		return mockIso
	}

	It("should skip hypervisor device rule when emulation is allowed and device is missing", func() {
		rules, err := generateDeviceRulesForVMI(&v1.VirtualMachineInstance{}, newMockIsolationWithMountRoot(), "", "kvm", true)
		Expect(err).ToNot(HaveOccurred())
		Expect(rules).To(BeEmpty())
	})

	It("should fail when hypervisor device is missing and emulation is not allowed", func() {
		_, err := generateDeviceRulesForVMI(&v1.VirtualMachineInstance{}, newMockIsolationWithMountRoot(), "", "kvm", false)
		Expect(err).To(HaveOccurred())
	})

	It("should not fail when /dev/vfio does not exist", func() {
		rules, err := generateDeviceRulesForVMI(&v1.VirtualMachineInstance{}, newMockIsolationWithMountRoot(), "", "kvm", true)
		Expect(err).ToNot(HaveOccurred())
		Expect(rules).To(BeEmpty())
	})

	It("should not fail when /dev/vfio exists but is empty", func() {
		Expect(os.MkdirAll(filepath.Join(tempDir, "dev", "vfio"), 0755)).To(Succeed())
		rules, err := generateDeviceRulesForVMI(&v1.VirtualMachineInstance{}, newMockIsolationWithMountRoot(), "", "kvm", true)
		Expect(err).ToNot(HaveOccurred())
		Expect(rules).To(BeEmpty())
	})

	It("should not fail when /dev/bus/usb exists but is empty", func() {
		Expect(os.MkdirAll(filepath.Join(tempDir, "dev", "bus", "usb"), 0755)).To(Succeed())
		rules, err := generateDeviceRulesForVMI(&v1.VirtualMachineInstance{}, newMockIsolationWithMountRoot(), "", "kvm", true)
		Expect(err).ToNot(HaveOccurred())
		Expect(rules).To(BeEmpty())
	})

	Context("with faked device rule creation", func() {
		var (
			origNewAllowedDeviceRule func(*safepath.Path, devices.Permissions) (*devices.Rule, error)
			requestedPaths           []string
		)

		BeforeEach(func() {
			origNewAllowedDeviceRule = newAllowedDeviceRule
			newAllowedDeviceRule = func(devicePath *safepath.Path, perms devices.Permissions) (*devices.Rule, error) {
				absPath := unsafepath.UnsafeAbsolute(devicePath.Raw())
				requestedPaths = append(requestedPaths, absPath)
				return &devices.Rule{
					Type:        devices.CharDevice,
					Major:       42,
					Minor:       0,
					Permissions: perms,
					Allow:       true,
				}, nil
			}
		})

		AfterEach(func() {
			newAllowedDeviceRule = origNewAllowedDeviceRule
			requestedPaths = nil
		})

		It("should create a rule for the hypervisor device", func() {
			Expect(os.WriteFile(filepath.Join(tempDir, "dev", "kvm"), nil, 0600)).To(Succeed())

			rules, err := generateDeviceRulesForVMI(&v1.VirtualMachineInstance{}, newMockIsolationWithMountRoot(), "", "kvm", false)
			Expect(err).ToNot(HaveOccurred())
			Expect(rules).To(HaveLen(1))
			Expect(requestedPaths).To(ConsistOf(
				filepath.Join(tempDir, "dev", "kvm"),
			))
		})

		It("should discover VFIO device nodes", func() {
			vfioDir := filepath.Join(tempDir, "dev", "vfio")
			Expect(os.MkdirAll(vfioDir, 0755)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(vfioDir, "vfio"), nil, 0600)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(vfioDir, "42"), nil, 0600)).To(Succeed())

			rules, err := generateDeviceRulesForVMI(&v1.VirtualMachineInstance{}, newMockIsolationWithMountRoot(), "", "kvm", true)
			Expect(err).ToNot(HaveOccurred())
			Expect(rules).To(HaveLen(2))
			Expect(requestedPaths).To(ConsistOf(
				filepath.Join(tempDir, "dev", "vfio", "42"),
				filepath.Join(tempDir, "dev", "vfio", "vfio"),
			))
		})

		It("should discover USB device nodes in nested directories", func() {
			usbBusDir := filepath.Join(tempDir, "dev", "bus", "usb", "001")
			Expect(os.MkdirAll(usbBusDir, 0755)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(usbBusDir, "001"), nil, 0600)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(usbBusDir, "002"), nil, 0600)).To(Succeed())

			rules, err := generateDeviceRulesForVMI(&v1.VirtualMachineInstance{}, newMockIsolationWithMountRoot(), "", "kvm", true)
			Expect(err).ToNot(HaveOccurred())
			Expect(rules).To(HaveLen(2))
			Expect(requestedPaths).To(ConsistOf(
				filepath.Join(tempDir, "dev", "bus", "usb", "001", "001"),
				filepath.Join(tempDir, "dev", "bus", "usb", "001", "002"),
			))
		})

		It("should discover devices from both VFIO and USB", func() {
			vfioDir := filepath.Join(tempDir, "dev", "vfio")
			Expect(os.MkdirAll(vfioDir, 0755)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(vfioDir, "vfio"), nil, 0600)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(vfioDir, "0"), nil, 0600)).To(Succeed())

			usbBus1Dir := filepath.Join(tempDir, "dev", "bus", "usb", "001")
			usbBus2Dir := filepath.Join(tempDir, "dev", "bus", "usb", "002")
			Expect(os.MkdirAll(usbBus1Dir, 0755)).To(Succeed())
			Expect(os.MkdirAll(usbBus2Dir, 0755)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(usbBus1Dir, "001"), nil, 0600)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(usbBus2Dir, "001"), nil, 0600)).To(Succeed())

			rules, err := generateDeviceRulesForVMI(&v1.VirtualMachineInstance{}, newMockIsolationWithMountRoot(), "", "kvm", true)
			Expect(err).ToNot(HaveOccurred())
			Expect(rules).To(HaveLen(4))
			Expect(requestedPaths).To(ConsistOf(
				filepath.Join(tempDir, "dev", "vfio", "vfio"),
				filepath.Join(tempDir, "dev", "vfio", "0"),
				filepath.Join(tempDir, "dev", "bus", "usb", "001", "001"),
				filepath.Join(tempDir, "dev", "bus", "usb", "002", "001"),
			))
		})

		It("should create a rule for urandom when RNG is enabled", func() {
			Expect(os.WriteFile(filepath.Join(tempDir, "dev", "urandom"), nil, 0600)).To(Succeed())

			vmi := &v1.VirtualMachineInstance{}
			vmi.Spec.Domain.Devices.Rng = &v1.Rng{}

			rules, err := generateDeviceRulesForVMI(vmi, newMockIsolationWithMountRoot(), "", "kvm", true)
			Expect(err).ToNot(HaveOccurred())
			Expect(rules).To(HaveLen(1))
			Expect(requestedPaths).To(ConsistOf(
				filepath.Join(tempDir, "dev", "urandom"),
			))
		})

		It("should create a rule for vhost-vsock when AutoattachVSOCK is enabled", func() {
			Expect(os.WriteFile(filepath.Join(tempDir, "dev", "vhost-vsock"), nil, 0600)).To(Succeed())

			autoAttach := true
			vmi := &v1.VirtualMachineInstance{}
			vmi.Spec.Domain.Devices.AutoattachVSOCK = &autoAttach

			rules, err := generateDeviceRulesForVMI(vmi, newMockIsolationWithMountRoot(), "", "kvm", true)
			Expect(err).ToNot(HaveOccurred())
			Expect(rules).To(HaveLen(1))
			Expect(requestedPaths).To(ConsistOf(
				filepath.Join(tempDir, "dev", "vhost-vsock"),
			))
		})
	})
})
