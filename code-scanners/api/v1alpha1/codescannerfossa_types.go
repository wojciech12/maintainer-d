/*
Copyright 2026.

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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CodeScannerFossaSpec defines the desired state of CodeScannerFossa
type CodeScannerFossaSpec struct {
	// ProjectName is the name of the CNCF project to scan
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ProjectName string `json:"projectName"`

	// ConfigMapName is the name of the ConfigMap to create for this scanner
	// +optional
	ConfigMapName string `json:"configMapName,omitempty"`

	// FossaUserEmails is a list of email addresses to invite to FOSSA
	// +optional
	FossaUserEmails []string `json:"fossaUserEmails,omitempty"`
}

// CodeScannerFossaStatus defines the observed state of CodeScannerFossa.
type CodeScannerFossaStatus struct {
	// ObservedGeneration is the generation observed by the controller
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// ConfigMapRef is the namespace/name reference to the created ConfigMap
	// +optional
	ConfigMapRef string `json:"configMapRef,omitempty"`

	// FossaTeam contains details about the created FOSSA Team
	// +optional
	FossaTeam *FossaTeamReference `json:"fossaTeam,omitempty"`

	// UserInvitations tracks the status of user invitations
	// +optional
	UserInvitations []FossaUserInvitation `json:"userInvitations,omitempty"`

	// Conditions represent the latest available observations of the resource's state
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// FossaUserInvitation tracks the invitation status for a user
type FossaUserInvitation struct {
	// Email is the user's email address
	Email string `json:"email"`

	// Status is the current invitation status (Pending, Accepted, Failed)
	Status string `json:"status"`

	// Message provides additional context about the status
	// +optional
	Message string `json:"message,omitempty"`

	// InvitedAt is when the invitation was sent
	// +optional
	InvitedAt *metav1.Time `json:"invitedAt,omitempty"`

	// AcceptedAt is when the user accepted the invitation
	// +optional
	AcceptedAt *metav1.Time `json:"acceptedAt,omitempty"`
}

// FossaTeamReference contains details about the FOSSA Team
type FossaTeamReference struct {
	// ID is the FOSSA team ID
	ID int `json:"id"`

	// Name is the team name (matches projectName)
	Name string `json:"name"`

	// OrganizationID is the FOSSA organization ID
	OrganizationID int `json:"organizationId"`

	// URL is a link to the team in FOSSA UI
	// +optional
	URL string `json:"url,omitempty"`

	// CreatedAt is when the team was created in FOSSA
	// +optional
	CreatedAt *metav1.Time `json:"createdAt,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Project",type=string,JSONPath=`.spec.projectName`
// +kubebuilder:printcolumn:name="FossaTeamID",type=integer,JSONPath=`.status.fossaTeam.id`
// +kubebuilder:printcolumn:name="ConfigMap",type=string,JSONPath=`.status.configMapRef`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="FossaTeamReady")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// CodeScannerFossa is the Schema for the codescannerfossas API
type CodeScannerFossa struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of CodeScannerFossa
	// +required
	Spec CodeScannerFossaSpec `json:"spec"`

	// status defines the observed state of CodeScannerFossa
	// +optional
	Status CodeScannerFossaStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// CodeScannerFossaList contains a list of CodeScannerFossa
type CodeScannerFossaList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []CodeScannerFossa `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CodeScannerFossa{}, &CodeScannerFossaList{})
}
