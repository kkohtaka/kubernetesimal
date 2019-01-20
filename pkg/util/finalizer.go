/*
Copyright 2019 Kazumasa Kohtaka <kkohtaka@gmail.com>.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package util

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	defaultFinalizer = "finalizer.packetdevices.packet.kkohtaka.org"
)

func IsDeleted(m *metav1.ObjectMeta) bool {
	return m.GetDeletionTimestamp() != nil
}

func HasFinalizer(m *metav1.ObjectMeta) bool {
	for _, finalizer := range m.GetFinalizers() {
		if finalizer == defaultFinalizer {
			return true
		}
	}
	return false
}

func SetFinalizer(m *metav1.ObjectMeta) {
	if !HasFinalizer(m) {
		m.SetFinalizers(append(m.GetFinalizers(), defaultFinalizer))
	}
}

func RemoveFinalizer(m *metav1.ObjectMeta) {
	for i := range m.Finalizers {
		if m.Finalizers[i] == defaultFinalizer {
			m.SetFinalizers(append(m.Finalizers[:i], m.Finalizers[i+1:]...))
			return
		}
	}
}
