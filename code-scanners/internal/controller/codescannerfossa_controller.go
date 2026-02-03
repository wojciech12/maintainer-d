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

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	maintainerdcncfiov1alpha1 "github.com/cncf/maintainer-d/code-scanners/api/v1alpha1"
	"github.com/cncf/maintainer-d/plugins/fossa"
)

// FossaClient defines the interface for FOSSA operations needed by the controller
type FossaClient interface {
	CreateTeam(name string) (*fossa.Team, error)
	FetchTeam(name string) (*fossa.Team, error)
}

// Ensure the real client implements the interface
var _ FossaClient = (*fossa.Client)(nil)

// CodeScannerFossaReconciler reconciles a CodeScannerFossa object
type CodeScannerFossaReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// FossaClientFactory creates FOSSA clients (injectable for testing)
	FossaClientFactory func(token string) FossaClient
}

// +kubebuilder:rbac:groups=maintainer-d.cncf.io,resources=codescannerfossas,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=maintainer-d.cncf.io,resources=codescannerfossas/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=maintainer-d.cncf.io,resources=codescannerfossas/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *CodeScannerFossaReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// 1. Fetch CR
	fossaCR := &maintainerdcncfiov1alpha1.CodeScannerFossa{}
	if err := r.Get(ctx, req.NamespacedName, fossaCR); err != nil {
		if errors.IsNotFound(err) {
			log.Info("CodeScannerFossa resource not found, ignoring")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get CodeScannerFossa")
		return ctrl.Result{}, err
	}

	// 2. Handle deletion (finalizer logic will be added later)
	// For now, ConfigMap is deleted via owner reference

	// 3. Get FOSSA credentials
	token, orgID, err := r.getFossaCredentials(ctx, fossaCR.Namespace)
	if err != nil {
		log.Error(err, "Failed to get FOSSA credentials")
		r.setCondition(fossaCR, ConditionTypeFossaTeamReady, metav1.ConditionFalse,
			ReasonCredentialsNotFound, err.Error())
		if updateErr := r.Status().Update(ctx, fossaCR); updateErr != nil {
			log.Error(updateErr, "Failed to update status")
			return ctrl.Result{}, updateErr
		}
		r.Recorder.Event(fossaCR, corev1.EventTypeWarning, ReasonCredentialsNotFound, err.Error())
		// Don't requeue - requires manual intervention
		return ctrl.Result{}, nil
	}

	// 4. Create FOSSA client
	fossaClient := r.FossaClientFactory(token)

	// 5. Ensure FOSSA Team exists
	team, err := r.ensureFossaTeam(ctx, fossaClient, fossaCR.Spec.ProjectName)
	if err != nil {
		log.Error(err, "Failed to ensure FOSSA team")
		r.setCondition(fossaCR, ConditionTypeFossaTeamReady, metav1.ConditionFalse,
			ReasonFossaAPIError, err.Error())
		if updateErr := r.Status().Update(ctx, fossaCR); updateErr != nil {
			log.Error(updateErr, "Failed to update status")
		}
		r.Recorder.Event(fossaCR, corev1.EventTypeWarning, ReasonFossaAPIError, err.Error())
		// Requeue for transient errors
		return ctrl.Result{RequeueAfter: time.Minute}, err
	}

	// 6. Update status with FOSSA team details
	orgIDInt, _ := strconv.Atoi(orgID)
	fossaCR.Status.ObservedGeneration = fossaCR.Generation
	fossaCR.Status.FossaTeam = &maintainerdcncfiov1alpha1.FossaTeamReference{
		ID:             team.ID,
		Name:           team.Name,
		OrganizationID: orgIDInt,
		URL:            fmt.Sprintf("https://app.fossa.com/account/settings/organization/teams/%d", team.ID),
		CreatedAt:      &metav1.Time{Time: team.CreatedAt},
	}
	r.setCondition(fossaCR, ConditionTypeFossaTeamReady, metav1.ConditionTrue,
		ReasonTeamCreated, fmt.Sprintf("FOSSA team %q ready (ID: %d)", team.Name, team.ID))

	// 7. Create/update ConfigMap
	configMap := r.configMapForFossa(fossaCR, team)
	if err := ctrl.SetControllerReference(fossaCR, configMap, r.Scheme); err != nil {
		log.Error(err, "Failed to set owner reference on ConfigMap")
		return ctrl.Result{}, err
	}

	existingCM := &corev1.ConfigMap{}
	err = r.Get(ctx, client.ObjectKeyFromObject(configMap), existingCM)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating ConfigMap", "name", configMap.Name, "namespace", configMap.Namespace)
		if err := r.Create(ctx, configMap); err != nil {
			log.Error(err, "Failed to create ConfigMap")
			return ctrl.Result{}, err
		}
		r.Recorder.Event(fossaCR, corev1.EventTypeNormal, ReasonConfigMapCreated,
			fmt.Sprintf("ConfigMap %s created", configMap.Name))
	} else if err == nil {
		// Only update if data changed
		if !reflect.DeepEqual(existingCM.Data, configMap.Data) {
			existingCM.Data = configMap.Data
			if err := r.Update(ctx, existingCM); err != nil {
				log.Error(err, "Failed to update ConfigMap")
				return ctrl.Result{}, err
			}
			log.Info("Updated ConfigMap", "name", configMap.Name)
		}
	} else {
		log.Error(err, "Failed to get ConfigMap")
		return ctrl.Result{}, err
	}

	r.setCondition(fossaCR, ConditionTypeConfigMapReady, metav1.ConditionTrue,
		ReasonConfigMapCreated, "ConfigMap ready")

	// 8. Update annotations
	configMapRef := fmt.Sprintf("%s/%s", configMap.Namespace, configMap.Name)
	if fossaCR.Annotations == nil {
		fossaCR.Annotations = make(map[string]string)
	}
	if fossaCR.Annotations[AnnotationConfigMapRef] != configMapRef {
		patch := client.MergeFrom(fossaCR.DeepCopy())
		fossaCR.Annotations[AnnotationConfigMapRef] = configMapRef
		if err := r.Patch(ctx, fossaCR, patch); err != nil {
			log.Error(err, "Failed to update annotation")
			return ctrl.Result{}, err
		}
	}

	// 9. Update status
	fossaCR.Status.ConfigMapRef = configMapRef
	if err := r.Status().Update(ctx, fossaCR); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	log.Info("Reconciliation complete",
		"fossaTeamID", team.ID,
		"fossaTeamName", team.Name,
		"configMap", configMapRef)
	r.Recorder.Event(fossaCR, corev1.EventTypeNormal, "Reconciled",
		fmt.Sprintf("FOSSA team %q (ID: %d) ready", team.Name, team.ID))
	return ctrl.Result{}, nil
}

func (r *CodeScannerFossaReconciler) configMapForFossa(cr *maintainerdcncfiov1alpha1.CodeScannerFossa, team *fossa.Team) *corev1.ConfigMap {
	data := map[string]string{
		ConfigMapKeyCodeScanner: ScannerTypeFossa,
		ConfigMapKeyProjectName: cr.Spec.ProjectName,
	}

	if team != nil {
		data["FossaTeamID"] = strconv.Itoa(team.ID)
		data["FossaTeamName"] = team.Name
		data["FossaTeamURL"] = fmt.Sprintf("https://app.fossa.com/account/settings/organization/teams/%d", team.ID)
	}

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.Namespace,
		},
		Data: data,
	}
}

// getFossaCredentials retrieves FOSSA credentials from the secret
func (r *CodeScannerFossaReconciler) getFossaCredentials(ctx context.Context, namespace string) (token, orgID string, err error) {
	log := logf.FromContext(ctx)

	secret := &corev1.Secret{}
	key := client.ObjectKey{
		Name:      SecretName,
		Namespace: namespace,
	}

	if err := r.Get(ctx, key, secret); err != nil {
		if errors.IsNotFound(err) {
			return "", "", fmt.Errorf("secret %s not found in namespace %s", SecretName, namespace)
		}
		return "", "", fmt.Errorf("failed to get secret %s: %w", SecretName, err)
	}

	token = string(secret.Data[SecretKeyFossaToken])
	orgID = string(secret.Data[SecretKeyFossaOrgID])

	if token == "" {
		return "", "", fmt.Errorf("missing %s in secret", SecretKeyFossaToken)
	}
	if orgID == "" {
		return "", "", fmt.Errorf("missing %s in secret", SecretKeyFossaOrgID)
	}

	log.V(1).Info("Retrieved FOSSA credentials", "orgID", orgID)
	// NEVER log token value
	return token, orgID, nil
}

// ensureFossaTeam creates or fetches a FOSSA team
func (r *CodeScannerFossaReconciler) ensureFossaTeam(ctx context.Context, client FossaClient, teamName string) (*fossa.Team, error) {
	log := logf.FromContext(ctx)

	// Try to get existing team
	log.V(1).Info("Checking if FOSSA team exists", "teamName", teamName)
	team, err := client.FetchTeam(teamName)
	if err == nil {
		log.Info("FOSSA team already exists", "teamName", teamName, "teamID", team.ID)
		return team, nil
	}

	// Create new team
	log.Info("Creating FOSSA team", "teamName", teamName)
	team, err = client.CreateTeam(teamName)
	if err != nil {
		return nil, fmt.Errorf("failed to create FOSSA team: %w", err)
	}

	log.Info("FOSSA team created", "teamName", team.Name, "teamID", team.ID)
	return team, nil
}

// setCondition updates or adds a condition to the CR status
func (r *CodeScannerFossaReconciler) setCondition(cr *maintainerdcncfiov1alpha1.CodeScannerFossa, condType string, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: cr.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}

	// Find and update existing condition or append new one
	found := false
	for i, c := range cr.Status.Conditions {
		if c.Type == condType {
			// Only update if status or reason changed
			if c.Status != status || c.Reason != reason {
				cr.Status.Conditions[i] = condition
			}
			found = true
			break
		}
	}
	if !found {
		cr.Status.Conditions = append(cr.Status.Conditions, condition)
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *CodeScannerFossaReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Initialize event recorder
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("codescannerfossa-controller")
	}

	// Initialize FOSSA client factory
	if r.FossaClientFactory == nil {
		r.FossaClientFactory = func(token string) FossaClient {
			return fossa.NewClient(token)
		}
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&maintainerdcncfiov1alpha1.CodeScannerFossa{}).
		Owns(&corev1.ConfigMap{}).
		Named("codescannerfossa").
		Complete(r)
}
