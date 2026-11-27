/*
MIT License

Copyright (c) 2022 Kazumasa Kohtaka

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package v1alpha1

import (
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// These tests claim ETCD-001 to ETCD-006 of docs/specs/etcd/spec.md. Every one of them also claims ETCD-006, which
// says that the others hold while the operator is not running: in this suite it never runs.
//
// The objects are built as unstructured data, so that a test decides which fields the request contains. The Go type
// cannot leave out a field that is not a pointer.

var etcdSequence int

// newEtcdObject returns an Etcd object with a name of its own and the given spec.
func newEtcdObject(spec map[string]any) *unstructured.Unstructured {
	etcdSequence++
	object := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": GroupVersion.String(),
		"kind":       "Etcd",
		"metadata": map[string]any{
			"namespace": "default",
			"name":      fmt.Sprintf("example-%d", etcdSequence),
		},
		"spec": spec,
	}}
	return object
}

// validSpec returns a spec that sets every field that has a default or a rule.
func validSpec() map[string]any {
	return map[string]any{
		"version":                       "3.5.6",
		"replicas":                      int64(3),
		"imagePersistentVolumeClaimRef": map[string]any{"name": "boot-image"},
	}
}

// with returns a valid spec with one field replaced, or removed when the value is nil.
func with(field string, value any) map[string]any {
	spec := validSpec()
	if value == nil {
		delete(spec, field)
	} else {
		spec[field] = value
	}
	return spec
}

// expectRejected expects that a request was refused as invalid, with an error that names a field.
func expectRejected(err error, field string) {
	ExpectWithOffset(1, err).To(HaveOccurred(), "the request was accepted")
	ExpectWithOffset(1, apierrors.IsInvalid(err)).To(BeTrue(), "the request failed for another reason: %v", err)
	ExpectWithOffset(1, err.Error()).To(ContainSubstring(field))
}

// expectRejectedOnCreateAndUpdate expects that an object with the given spec can neither be created, nor be the
// result of updating a valid object.
func expectRejectedOnCreateAndUpdate(spec map[string]any, field string) {
	By("creating the object")
	expectRejected(k8sClient.Create(ctx, newEtcdObject(spec)), field)

	By("updating an object to it")
	object := newEtcdObject(validSpec())
	ExpectWithOffset(1, k8sClient.Create(ctx, object)).To(Succeed())
	object.Object["spec"] = spec
	expectRejected(k8sClient.Update(ctx, object), field)
}

var _ = Describe("An Etcd object, while the operator is not running", func() {
	It("[ETCD-001] [ETCD-006] gets the version 3.5.1 when it is created without one", func() {
		object := newEtcdObject(with("version", nil))
		Expect(k8sClient.Create(ctx, object)).To(Succeed())

		var stored Etcd
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(object), &stored)).To(Succeed())
		Expect(stored.Spec.Version).NotTo(BeNil())
		Expect(*stored.Spec.Version).To(Equal("3.5.1"))
	})

	It("[ETCD-002] [ETCD-006] gets one replica when it is created without a number of replicas", func() {
		object := newEtcdObject(with("replicas", nil))
		Expect(k8sClient.Create(ctx, object)).To(Succeed())

		var stored Etcd
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(object), &stored)).To(Succeed())
		Expect(stored.Spec.Replicas).NotTo(BeNil())
		Expect(*stored.Spec.Replicas).To(Equal(int32(1)))
	})

	It("[ETCD-001] [ETCD-002] [ETCD-006] keeps a version and a number of replicas that it is created with", func() {
		object := newEtcdObject(validSpec())
		Expect(k8sClient.Create(ctx, object)).To(Succeed())

		var stored Etcd
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(object), &stored)).To(Succeed())
		Expect(*stored.Spec.Version).To(Equal("3.5.6"))
		Expect(*stored.Spec.Replicas).To(Equal(int32(3)))
	})

	Describe("[ETCD-003] [ETCD-006] with a version that is not of the form MAJOR.MINOR.PATCH", func() {
		for _, version := range []string{
			"",
			"3",
			"3.5",
			"3.5.6.1",
			"v3.5.6",
			"3.5.x",
			"3.5.6-rc.1",
			"3.5.6+build.1",
			"03.5.6",
			" 3.5.6",
			"latest",
		} {
			It(fmt.Sprintf("[ETCD-003] [ETCD-006] is rejected with the version %q", version), func() {
				expectRejectedOnCreateAndUpdate(with("version", version), "spec.version")
			})
		}

		for _, version := range []string{"3.5.6", "0.0.0", "10.20.30"} {
			It(fmt.Sprintf("[ETCD-003] [ETCD-006] is accepted with the version %q", version), func() {
				Expect(k8sClient.Create(ctx, newEtcdObject(with("version", version)))).To(Succeed())
			})
		}
	})

	Describe("[ETCD-004] [ETCD-006] without the name of the image", func() {
		It("[ETCD-004] [ETCD-006] is rejected without a reference to the image", func() {
			expectRejectedOnCreateAndUpdate(
				with("imagePersistentVolumeClaimRef", nil),
				"spec.imagePersistentVolumeClaimRef",
			)
		})

		It("[ETCD-004] [ETCD-006] is rejected with a reference that has no name", func() {
			expectRejectedOnCreateAndUpdate(
				with("imagePersistentVolumeClaimRef", map[string]any{}),
				"spec.imagePersistentVolumeClaimRef",
			)
		})

		It("[ETCD-004] [ETCD-006] is rejected with a reference that has an empty name", func() {
			expectRejectedOnCreateAndUpdate(
				with("imagePersistentVolumeClaimRef", map[string]any{"name": ""}),
				"spec.imagePersistentVolumeClaimRef",
			)
		})
	})

	Describe("[ETCD-005] [ETCD-006] with less than one replica", func() {
		for _, replicas := range []int64{0, -1} {
			It(fmt.Sprintf("[ETCD-005] [ETCD-006] is rejected with %d replicas", replicas), func() {
				expectRejectedOnCreateAndUpdate(with("replicas", replicas), "spec.replicas")
			})
		}

		It("[ETCD-005] [ETCD-006] is accepted with one replica", func() {
			Expect(k8sClient.Create(ctx, newEtcdObject(with("replicas", int64(1))))).To(Succeed())
		})
	})
})
