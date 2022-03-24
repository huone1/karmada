package clusterpropagationpolicy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	policyv1alpha1 "github.com/karmada-io/karmada/pkg/apis/policy/v1alpha1"
	"github.com/karmada-io/karmada/pkg/util/helper"
	"github.com/karmada-io/karmada/pkg/util/validation"
)

// MutatingAdmission mutates API request if necessary.
type MutatingAdmission struct {
	decoder *admission.Decoder
}

// Check if our MutatingAdmission implements necessary interface
var _ admission.Handler = &MutatingAdmission{}
var _ admission.DecoderInjector = &MutatingAdmission{}

// Handle yields a response to an AdmissionRequest.
func (a *MutatingAdmission) Handle(ctx context.Context, req admission.Request) admission.Response {
	policy := &policyv1alpha1.ClusterPropagationPolicy{}

	err := a.decoder.Decode(req, policy)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	if len(policy.Name) > validation.LabelValueMaxLength {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("ClusterPropagationPolicy's name should be no more than %d characters", validation.LabelValueMaxLength))
	}

	// Set default spread constraints if both 'SpreadByField' and 'SpreadByLabel' not set.
	helper.SetDefaultSpreadConstraints(policy.Spec.Placement.SpreadConstraints)

	// Set default scheduling strategy
	policy.Spec.Placement.ReplicaScheduling = helper.SetDefaultSchedulingStrategy(policy.Spec.Placement.ReplicaScheduling)

	addedResourceSelectors := helper.GetFollowedResourceSelectorsWhenMatchServiceImport(policy.Spec.ResourceSelectors)
	if addedResourceSelectors != nil {
		policy.Spec.ResourceSelectors = append(policy.Spec.ResourceSelectors, addedResourceSelectors...)
	}

	marshaledBytes, err := json.Marshal(policy)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledBytes)
}

// InjectDecoder implements admission.DecoderInjector interface.
// A decoder will be automatically injected.
func (a *MutatingAdmission) InjectDecoder(d *admission.Decoder) error {
	a.decoder = d
	return nil
}
