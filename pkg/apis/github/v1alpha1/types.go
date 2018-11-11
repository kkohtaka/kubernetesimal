package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// Repository is a specification for a Repository resource
type Repository struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Status RepositoryStatus `json:"status"`
	Spec   RepositorySpec   `json:"spec"`
}

// RepositoryStatus is the status for a Repository resource
type RepositoryStatus struct {
}

// RepositorySpec is the spec for a Repository resource
type RepositorySpec struct {
	Name string `json:"name"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RepositoryList is a list of Repository resources
type RepositoryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Repository `json:"items"`
}
