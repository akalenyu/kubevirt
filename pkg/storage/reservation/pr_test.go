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

package reservation

import (
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "kubevirt.io/api/core/v1"
)

func TestPersistentReservationPVCLabels_NoPRDisks(t *testing.T) {
	vmi := &v1.VirtualMachineInstance{
		Spec: v1.VirtualMachineInstanceSpec{
			Domain: v1.DomainSpec{
				Devices: v1.Devices{
					Disks: []v1.Disk{
						{
							Name:       "disk0",
							DiskDevice: v1.DiskDevice{Disk: &v1.DiskTarget{Bus: v1.DiskBusVirtio}},
						},
					},
				},
			},
			Volumes: []v1.Volume{
				{
					Name: "disk0",
					VolumeSource: v1.VolumeSource{
						PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{},
					},
				},
			},
		},
	}

	labels := PersistentReservationPVCLabels(vmi)
	if len(labels) != 0 {
		t.Errorf("expected no labels, got %v", labels)
	}
}

func TestPersistentReservationPVCLabels_LUNWithoutReservation(t *testing.T) {
	vmi := &v1.VirtualMachineInstance{
		Spec: v1.VirtualMachineInstanceSpec{
			Domain: v1.DomainSpec{
				Devices: v1.Devices{
					Disks: []v1.Disk{
						{
							Name:       "lun0",
							DiskDevice: v1.DiskDevice{LUN: &v1.LunTarget{Bus: v1.DiskBusSCSI, Reservation: false}},
						},
					},
				},
			},
			Volumes: []v1.Volume{
				{
					Name: "lun0",
					VolumeSource: v1.VolumeSource{
						PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{},
					},
				},
			},
		},
	}

	labels := PersistentReservationPVCLabels(vmi)
	if len(labels) != 0 {
		t.Errorf("expected no labels, got %v", labels)
	}
}

func TestPersistentReservationPVCLabels_OnePRPVC(t *testing.T) {
	vmi := newVMIWithPRPVC("lun0", "my-shared-pvc")

	labels := PersistentReservationPVCLabels(vmi)
	if len(labels) != 1 {
		t.Fatalf("expected 1 label, got %d: %v", len(labels), labels)
	}

	expectedKey := pvcNameToLabelKey("my-shared-pvc")
	val, ok := labels[expectedKey]
	if !ok {
		t.Errorf("expected label key %q, got keys %v", expectedKey, labels)
	}
	if val != "my-shared-pvc" {
		t.Errorf("expected label value %q, got %q", "my-shared-pvc", val)
	}
}

func TestPersistentReservationPVCLabels_DataVolume(t *testing.T) {
	vmi := &v1.VirtualMachineInstance{
		Spec: v1.VirtualMachineInstanceSpec{
			Domain: v1.DomainSpec{
				Devices: v1.Devices{
					Disks: []v1.Disk{
						{
							Name:       "lun0",
							DiskDevice: v1.DiskDevice{LUN: &v1.LunTarget{Bus: v1.DiskBusSCSI, Reservation: true}},
						},
					},
				},
			},
			Volumes: []v1.Volume{
				{
					Name: "lun0",
					VolumeSource: v1.VolumeSource{
						DataVolume: &v1.DataVolumeSource{Name: "my-dv"},
					},
				},
			},
		},
	}

	labels := PersistentReservationPVCLabels(vmi)
	if len(labels) != 1 {
		t.Fatalf("expected 1 label, got %d", len(labels))
	}

	expectedKey := pvcNameToLabelKey("my-dv")
	if _, ok := labels[expectedKey]; !ok {
		t.Errorf("expected label key %q", expectedKey)
	}
}

func TestPersistentReservationPVCLabels_MultiplePRPVCs(t *testing.T) {
	vmi := &v1.VirtualMachineInstance{
		Spec: v1.VirtualMachineInstanceSpec{
			Domain: v1.DomainSpec{
				Devices: v1.Devices{
					Disks: []v1.Disk{
						{
							Name:       "lun0",
							DiskDevice: v1.DiskDevice{LUN: &v1.LunTarget{Bus: v1.DiskBusSCSI, Reservation: true}},
						},
						{
							Name:       "lun1",
							DiskDevice: v1.DiskDevice{LUN: &v1.LunTarget{Bus: v1.DiskBusSCSI, Reservation: true}},
						},
					},
				},
			},
			Volumes: []v1.Volume{
				{
					Name: "lun0",
					VolumeSource: v1.VolumeSource{
						PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{},
					},
				},
				{
					Name: "lun1",
					VolumeSource: v1.VolumeSource{
						PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{},
					},
				},
			},
		},
	}
	// Set PVC claim names
	vmi.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = "pvc-a"
	vmi.Spec.Volumes[1].PersistentVolumeClaim.ClaimName = "pvc-b"

	labels := PersistentReservationPVCLabels(vmi)
	if len(labels) != 2 {
		t.Fatalf("expected 2 labels, got %d: %v", len(labels), labels)
	}

	keyA := pvcNameToLabelKey("pvc-a")
	keyB := pvcNameToLabelKey("pvc-b")
	if _, ok := labels[keyA]; !ok {
		t.Errorf("missing label for pvc-a")
	}
	if _, ok := labels[keyB]; !ok {
		t.Errorf("missing label for pvc-b")
	}
}

func TestPersistentReservationPVCLabels_NoMatchingVolume(t *testing.T) {
	vmi := &v1.VirtualMachineInstance{
		Spec: v1.VirtualMachineInstanceSpec{
			Domain: v1.DomainSpec{
				Devices: v1.Devices{
					Disks: []v1.Disk{
						{
							Name:       "lun0",
							DiskDevice: v1.DiskDevice{LUN: &v1.LunTarget{Bus: v1.DiskBusSCSI, Reservation: true}},
						},
					},
				},
			},
			Volumes: []v1.Volume{}, // no volumes
		},
	}

	labels := PersistentReservationPVCLabels(vmi)
	if len(labels) != 0 {
		t.Errorf("expected no labels for missing volume, got %v", labels)
	}
}

func TestPersistentReservationPVCLabels_NonPVCVolume(t *testing.T) {
	vmi := &v1.VirtualMachineInstance{
		Spec: v1.VirtualMachineInstanceSpec{
			Domain: v1.DomainSpec{
				Devices: v1.Devices{
					Disks: []v1.Disk{
						{
							Name:       "lun0",
							DiskDevice: v1.DiskDevice{LUN: &v1.LunTarget{Bus: v1.DiskBusSCSI, Reservation: true}},
						},
					},
				},
			},
			Volumes: []v1.Volume{
				{
					Name: "lun0",
					VolumeSource: v1.VolumeSource{
						ContainerDisk: &v1.ContainerDiskSource{Image: "test"},
					},
				},
			},
		},
	}

	labels := PersistentReservationPVCLabels(vmi)
	if len(labels) != 0 {
		t.Errorf("expected no labels for non-PVC volume, got %v", labels)
	}
}

func TestPersistentReservationPVCLabels_LongPVCName(t *testing.T) {
	longName := strings.Repeat("a", 100)
	vmi := newVMIWithPRPVC("lun0", longName)

	labels := PersistentReservationPVCLabels(vmi)
	if len(labels) != 1 {
		t.Fatalf("expected 1 label, got %d", len(labels))
	}

	for key, val := range labels {
		if !strings.HasPrefix(key, v1.PersistentReservationLabelPrefix) {
			t.Errorf("label key %q missing expected prefix", key)
		}
		if len(val) > 63 {
			t.Errorf("label value length %d exceeds 63", len(val))
		}
		if val != longName[:63] {
			t.Errorf("expected truncated value, got %q", val)
		}
	}
}

func TestPvcNameToLabelKey_Deterministic(t *testing.T) {
	key1 := pvcNameToLabelKey("my-pvc")
	key2 := pvcNameToLabelKey("my-pvc")
	if key1 != key2 {
		t.Errorf("expected deterministic keys, got %q and %q", key1, key2)
	}
}

func TestPvcNameToLabelKey_DifferentPVCs(t *testing.T) {
	key1 := pvcNameToLabelKey("pvc-alpha")
	key2 := pvcNameToLabelKey("pvc-beta")
	if key1 == key2 {
		t.Errorf("expected different keys for different PVCs, both got %q", key1)
	}
}

func TestPvcNameToLabelKey_ValidFormat(t *testing.T) {
	key := pvcNameToLabelKey("test-pvc")
	if !strings.HasPrefix(key, v1.PersistentReservationLabelPrefix) {
		t.Errorf("key %q missing prefix", key)
	}
	// Name part (after prefix) should be 16 hex chars
	name := strings.TrimPrefix(key, v1.PersistentReservationLabelPrefix)
	if len(name) != 16 {
		t.Errorf("expected 16 char hash suffix, got %d chars: %q", len(name), name)
	}
}

func TestPersistentReservationPodAntiAffinityTerms_Empty(t *testing.T) {
	terms := PersistentReservationPodAntiAffinityTerms(map[string]string{})
	if len(terms) != 0 {
		t.Errorf("expected no terms, got %d", len(terms))
	}
}

func TestPersistentReservationPodAntiAffinityTerms_OneLabel(t *testing.T) {
	labels := map[string]string{
		"pr.kubevirt.io/abc123": "my-pvc",
	}
	terms := PersistentReservationPodAntiAffinityTerms(labels)
	if len(terms) != 1 {
		t.Fatalf("expected 1 term, got %d", len(terms))
	}
	term := terms[0]
	if term.TopologyKey != "kubernetes.io/hostname" {
		t.Errorf("expected topology key kubernetes.io/hostname, got %q", term.TopologyKey)
	}
	if term.LabelSelector == nil {
		t.Fatal("expected label selector, got nil")
	}
	if len(term.LabelSelector.MatchExpressions) != 1 {
		t.Fatalf("expected 1 match expression, got %d", len(term.LabelSelector.MatchExpressions))
	}
	expr := term.LabelSelector.MatchExpressions[0]
	if expr.Key != "pr.kubevirt.io/abc123" {
		t.Errorf("expected key pr.kubevirt.io/abc123, got %q", expr.Key)
	}
	if expr.Operator != metav1.LabelSelectorOpExists {
		t.Errorf("expected Exists operator, got %v", expr.Operator)
	}
}

func TestPersistentReservationPodAntiAffinityTerms_MultipleLabels(t *testing.T) {
	labels := map[string]string{
		"pr.kubevirt.io/aaa": "pvc-1",
		"pr.kubevirt.io/bbb": "pvc-2",
	}
	terms := PersistentReservationPodAntiAffinityTerms(labels)
	if len(terms) != 2 {
		t.Fatalf("expected 2 terms, got %d", len(terms))
	}
	for _, term := range terms {
		if term.TopologyKey != "kubernetes.io/hostname" {
			t.Errorf("expected topology key kubernetes.io/hostname, got %q", term.TopologyKey)
		}
	}
}

func newVMIWithPRPVC(diskName, pvcName string) *v1.VirtualMachineInstance {
	vmi := &v1.VirtualMachineInstance{
		Spec: v1.VirtualMachineInstanceSpec{
			Domain: v1.DomainSpec{
				Devices: v1.Devices{
					Disks: []v1.Disk{
						{
							Name:       diskName,
							DiskDevice: v1.DiskDevice{LUN: &v1.LunTarget{Bus: v1.DiskBusSCSI, Reservation: true}},
						},
					},
				},
			},
			Volumes: []v1.Volume{
				{
					Name: diskName,
					VolumeSource: v1.VolumeSource{
						PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{},
					},
				},
			},
		},
	}
	vmi.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = pvcName
	return vmi
}
