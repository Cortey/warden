package certs

import (
	"context"
	"github.com/kyma-project/warden/internal/webhook/defaulting"
	"reflect"

	"github.com/pkg/errors"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	ctlrclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type WebHookType string

const (
	MutatingWebhook   WebHookType = "Mutating"
	ValidatingWebHook WebHookType = "Validating"

	DefaultingWebhookName = "defaulting.webhook.warden.kyma-project.io"
	ValidationWebhookName = "validation.webhook.warden.kyma-project.io"

	WebhookTimeout = 15

	PodValidationPath = "/validation/pods"
)

func EnsureWebhookConfigurationFor(ctx context.Context, client ctlrclient.Client, config WebhookConfig, wt WebHookType) error {
	if wt == MutatingWebhook {
		return ensureMutatingWebhookConfigFor(ctx, client, config)
	}
	return ensureValidatingWebhookConfigFor(ctx, client, config)
}

func ensureMutatingWebhookConfigFor(ctx context.Context, client ctlrclient.Client, config WebhookConfig) error {
	mwhc := &admissionregistrationv1.MutatingWebhookConfiguration{}
	if err := client.Get(ctx, types.NamespacedName{Name: DefaultingWebhookName}, mwhc); err != nil {
		if apiErrors.IsNotFound(err) {
			return errors.Wrap(client.Create(ctx, createMutatingWebhookConfiguration(config)), "while creating webhook mutation configuration")
		}
		return errors.Wrapf(err, "failed to get defaulting MutatingWebhookConfiguration: %s", DefaultingWebhookName)
	}
	ensuredMwhc := createMutatingWebhookConfiguration(config)

	if !reflect.DeepEqual(ensuredMwhc.Webhooks, mwhc.Webhooks) {
		ensuredMwhc.ObjectMeta = *mwhc.ObjectMeta.DeepCopy()
		return errors.Wrap(client.Update(ctx, ensuredMwhc), "while updating webhook mutation configuration")
	}
	return nil
}

func ensureValidatingWebhookConfigFor(ctx context.Context, client ctlrclient.Client, config WebhookConfig) error {
	vwhc := &admissionregistrationv1.ValidatingWebhookConfiguration{}
	if err := client.Get(ctx, types.NamespacedName{Name: ValidationWebhookName}, vwhc); err != nil {
		if apiErrors.IsNotFound(err) {
			return client.Create(ctx, createValidatingWebhookConfiguration(config))
		}
		return errors.Wrapf(err, "failed to get validation ValidatingWebhookConfiguration: %s", ValidationWebhookName)
	}
	ensuredVwhc := createValidatingWebhookConfiguration(config)
	if !reflect.DeepEqual(ensuredVwhc.Webhooks, vwhc.Webhooks) {
		ensuredVwhc.ObjectMeta = *vwhc.ObjectMeta.DeepCopy()
		return client.Update(ctx, ensuredVwhc)
	}
	return nil
}

func createMutatingWebhookConfiguration(config WebhookConfig) *admissionregistrationv1.MutatingWebhookConfiguration {
	return &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: DefaultingWebhookName,
		},
		Webhooks: []admissionregistrationv1.MutatingWebhook{
			getFunctionMutatingWebhookCfg(config),
		},
	}
}

func getFunctionMutatingWebhookCfg(config WebhookConfig) admissionregistrationv1.MutatingWebhook {
	failurePolicy := admissionregistrationv1.Fail
	matchPolicy := admissionregistrationv1.Exact
	reinvocationPolicy := admissionregistrationv1.NeverReinvocationPolicy
	scope := admissionregistrationv1.AllScopes
	sideEffects := admissionregistrationv1.SideEffectClassNone

	return admissionregistrationv1.MutatingWebhook{
		Name: DefaultingWebhookName,
		AdmissionReviewVersions: []string{
			"v1beta1",
			"v1",
		},
		ClientConfig: admissionregistrationv1.WebhookClientConfig{
			CABundle: config.CABundel,
			Service: &admissionregistrationv1.ServiceReference{
				Namespace: config.ServiceNamespace,
				Name:      config.ServiceName,
				Path:      pointer.String(defaulting.WebhookPath),
				Port:      pointer.Int32(443),
			},
		},
		FailurePolicy:      &failurePolicy,
		MatchPolicy:        &matchPolicy,
		ReinvocationPolicy: &reinvocationPolicy,
		Rules: []admissionregistrationv1.RuleWithOperations{
			{
				Rule: admissionregistrationv1.Rule{
					APIGroups: []string{
						corev1.GroupName,
					},
					APIVersions: []string{corev1.SchemeGroupVersion.Version},
					Resources:   []string{string(corev1.ResourcePods)},
					Scope:       &scope,
				},
				Operations: []admissionregistrationv1.OperationType{
					admissionregistrationv1.Create,
					admissionregistrationv1.Update,
				},
			},
		},
		SideEffects:    &sideEffects,
		TimeoutSeconds: pointer.Int32(WebhookTimeout),
	}
}

func createValidatingWebhookConfiguration(config WebhookConfig) *admissionregistrationv1.ValidatingWebhookConfiguration {
	failurePolicy := admissionregistrationv1.Ignore
	matchPolicy := admissionregistrationv1.Exact
	scope := admissionregistrationv1.AllScopes
	sideEffects := admissionregistrationv1.SideEffectClassNone

	return &admissionregistrationv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: ValidationWebhookName,
		},
		Webhooks: []admissionregistrationv1.ValidatingWebhook{
			{
				Name: ValidationWebhookName,
				AdmissionReviewVersions: []string{
					"v1beta1",
					"v1",
				},
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					CABundle: config.CABundel,
					Service: &admissionregistrationv1.ServiceReference{
						Namespace: config.ServiceNamespace,
						Name:      config.ServiceName,
						Path:      pointer.String(PodValidationPath),
						Port:      pointer.Int32(443),
					},
				},
				FailurePolicy: &failurePolicy,
				MatchPolicy:   &matchPolicy,
				Rules: []admissionregistrationv1.RuleWithOperations{
					{
						Rule: admissionregistrationv1.Rule{
							APIGroups: []string{
								corev1.GroupName,
							},
							APIVersions: []string{corev1.SchemeGroupVersion.Version},
							Resources:   []string{string(corev1.ResourcePods)},
							Scope:       &scope,
						},
						Operations: []admissionregistrationv1.OperationType{
							admissionregistrationv1.Create,
							admissionregistrationv1.Update,
						}},
				},

				SideEffects:    &sideEffects,
				TimeoutSeconds: pointer.Int32(WebhookTimeout),
			},
		},
	}
}
