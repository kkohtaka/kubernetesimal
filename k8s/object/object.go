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

package k8s

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
)

type ObjectOption func(runtime.Object) error

func WithName(name string) ObjectOption {
	return func(o runtime.Object) error {
		meta, err := meta.Accessor(o)
		if err != nil {
			return err
		}
		meta.SetName(name)
		return nil
	}
}

func WithNamespace(namespace string) ObjectOption {
	return func(o runtime.Object) error {
		meta, err := meta.Accessor(o)
		if err != nil {
			return err
		}
		meta.SetNamespace(namespace)
		return nil
	}
}

func WithGeneratorName(generatorName string) ObjectOption {
	return func(o runtime.Object) error {
		meta, err := meta.Accessor(o)
		if err != nil {
			return err
		}
		meta.SetGenerateName(generatorName)
		return nil
	}
}

func WithLabel(key, value string) ObjectOption {
	return func(o runtime.Object) error {
		meta, err := meta.Accessor(o)
		if err != nil {
			return err
		}
		labels := meta.GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}
		labels[key] = value
		meta.SetLabels(labels)
		return nil
	}
}

func WithLabels(src map[string]string) ObjectOption {
	return func(o runtime.Object) error {
		meta, err := meta.Accessor(o)
		if err != nil {
			return err
		}
		labels := make(map[string]string)
		dist := meta.GetLabels()
		for key, value := range dist {
			labels[key] = value
		}
		for key, value := range src {
			labels[key] = value
		}
		meta.SetLabels(labels)
		return nil
	}
}

func WithAnnotation(key, value string) ObjectOption {
	return func(o runtime.Object) error {
		meta, err := meta.Accessor(o)
		if err != nil {
			return err
		}
		annotations := meta.GetAnnotations()
		if annotations == nil {
			annotations = make(map[string]string)
		}
		annotations[key] = value
		meta.SetAnnotations(annotations)
		return nil
	}
}

func WithAnnotations(src map[string]string) ObjectOption {
	return func(o runtime.Object) error {
		meta, err := meta.Accessor(o)
		if err != nil {
			return err
		}
		annotations := make(map[string]string)
		dist := meta.GetAnnotations()
		for key, value := range dist {
			annotations[key] = value
		}
		for key, value := range src {
			annotations[key] = value
		}
		meta.SetAnnotations(annotations)
		return nil
	}
}

func WithOwner(owner metav1.Object, scheme *runtime.Scheme) ObjectOption {
	return func(o runtime.Object) error {
		meta, err := meta.Accessor(o)
		if err != nil {
			return err
		}
		return ctrl.SetControllerReference(owner, meta, scheme)
	}
}

func ObjectName(o *metav1.ObjectMeta) string {
	if o.Namespace == "" {
		return o.Name
	}
	return o.Namespace + "/" + o.Name
}
