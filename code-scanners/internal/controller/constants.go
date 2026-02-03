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

package controller

const (
	// AnnotationConfigMapRef is the annotation key for ConfigMap reference
	AnnotationConfigMapRef = "maintainer-d.cncf.io/configmap-ref"

	// ConfigMapKeyCodeScanner is the ConfigMap data key for scanner type
	ConfigMapKeyCodeScanner = "CodeScanner"

	// ConfigMapKeyProjectName is the ConfigMap data key for project name
	ConfigMapKeyProjectName = "ProjectName"

	// ScannerTypeFossa identifies the Fossa scanner
	ScannerTypeFossa = "Fossa"

	// ScannerTypeSnyk identifies the Snyk scanner
	ScannerTypeSnyk = "Snyk"

	// SecretName is the name of the secret containing scanner credentials
	SecretName = "code-scanners"

	// SecretKeyFossaToken is the key for FOSSA API token
	SecretKeyFossaToken = "fossa-api-token"

	// SecretKeyFossaOrgID is the key for FOSSA organization ID
	SecretKeyFossaOrgID = "fossa-organization-id"

	// Condition types
	ConditionTypeFossaTeamReady = "FossaTeamReady"
	ConditionTypeConfigMapReady = "ConfigMapReady"

	// Condition reasons
	ReasonTeamCreated         = "TeamCreated"
	ReasonTeamExists          = "TeamExists"
	ReasonFossaAPIError       = "APIError"
	ReasonCredentialsNotFound = "CredentialsNotFound"
	ReasonConfigMapCreated    = "ConfigMapCreated"
)
