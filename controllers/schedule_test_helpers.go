package controllers

import (
	"context"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1beta1 "github.com/stolostron/cluster-backup-operator/api/v1beta1"
	veleroapi "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
)

// waitForSecretLabel waits for a secret to have a specific label value
func waitForSecretLabel(
	ctx context.Context,
	k8sClient client.Client,
	secretName, namespace, labelKey, expectedValue string,
	timeout, interval time.Duration,
) {
	secret := corev1.Secret{}
	Eventually(func() bool {
		err := k8sClient.Get(ctx, createLookupKey(secretName, namespace), &secret)
		if err != nil {
			return false
		}
		labels := secret.GetLabels()
		if labels == nil {
			return expectedValue == ""
		}
		return labels[labelKey] == expectedValue
	}, timeout, interval).Should(BeTrue())
}

// waitForSecretLabelMissing waits for a secret to NOT have a specific label value
func waitForSecretLabelMissing(
	ctx context.Context,
	k8sClient client.Client,
	secretName, namespace, labelKey, unexpectedValue string,
	timeout, interval time.Duration,
) {
	secret := corev1.Secret{}
	Eventually(func() bool {
		err := k8sClient.Get(ctx, createLookupKey(secretName, namespace), &secret)
		if err != nil {
			return false
		}
		labels := secret.GetLabels()
		if labels == nil {
			return true
		}
		return labels[labelKey] != unexpectedValue
	}, timeout, interval).Should(BeTrue())
}

// waitForVeleroScheduleCount waits for a specific number of Velero schedules in a namespace
func waitForVeleroScheduleCount(
	ctx context.Context,
	k8sClient client.Client,
	namespace string,
	expectedCount int,
	timeout, interval time.Duration,
) {
	Eventually(func() int {
		scheduleList := veleroapi.ScheduleList{}
		if err := k8sClient.List(ctx, &scheduleList, client.InNamespace(namespace)); err != nil {
			return -1
		}
		return len(scheduleList.Items)
	}, timeout, interval).Should(BeNumerically("==", expectedCount))
}

// waitForVeleroScheduleCountGTE waits for at least a specific number of Velero schedules
func waitForVeleroScheduleCountGTE(
	ctx context.Context,
	k8sClient client.Client,
	namespace string,
	minCount int,
	timeout, interval time.Duration,
) {
	Eventually(func() int {
		scheduleList := veleroapi.ScheduleList{}
		if err := k8sClient.List(ctx, &scheduleList, client.InNamespace(namespace)); err != nil {
			return -1
		}
		return len(scheduleList.Items)
	}, timeout, interval).Should(BeNumerically(">=", minCount))
}

// waitForBackupScheduleCount waits for a specific number of ACM BackupSchedules
func waitForBackupScheduleCount(
	ctx context.Context,
	k8sClient client.Client,
	expectedCount int,
	timeout, interval time.Duration,
) {
	Eventually(func() int {
		scheduleList := v1beta1.BackupScheduleList{}
		if err := k8sClient.List(ctx, &scheduleList, &client.ListOptions{}); err != nil {
			return -1
		}
		return len(scheduleList.Items)
	}, timeout, interval).Should(BeNumerically("==", expectedCount))
}

// waitForBackupScheduleCountGTE waits for at least a specific number of ACM BackupSchedules
func waitForBackupScheduleCountGTE(
	ctx context.Context,
	k8sClient client.Client,
	minCount int,
	timeout, interval time.Duration,
) {
	Eventually(func() int {
		scheduleList := v1beta1.BackupScheduleList{}
		if err := k8sClient.List(ctx, &scheduleList, &client.ListOptions{}); err != nil {
			return -1
		}
		return len(scheduleList.Items)
	}, timeout, interval).Should(BeNumerically(">=", minCount))
}

// waitForBackupScheduleStatus waits for a BackupSchedule to reach a specific status
func waitForBackupScheduleStatus(
	ctx context.Context,
	k8sClient client.Client,
	name, namespace string,
	expectedPhase v1beta1.SchedulePhase,
	timeout, interval time.Duration,
) {
	Eventually(func() v1beta1.SchedulePhase {
		schedule := v1beta1.BackupSchedule{}
		err := k8sClient.Get(ctx, createLookupKey(name, namespace), &schedule)
		if err != nil {
			return ""
		}
		return schedule.Status.Phase
	}, timeout, interval).Should(Equal(expectedPhase))
}

// waitForBackupScheduleStatusMessage waits for a BackupSchedule to have a specific status message
func waitForBackupScheduleStatusMessage(
	ctx context.Context,
	k8sClient client.Client,
	name, namespace string,
	expectedMessage string,
	timeout, interval time.Duration,
) {
	Eventually(func() string {
		schedule := v1beta1.BackupSchedule{}
		err := k8sClient.Get(ctx, createLookupKey(name, namespace), &schedule)
		if err != nil {
			return ""
		}
		return schedule.Status.LastMessage
	}, timeout, interval).Should(ContainSubstring(expectedMessage))
}

// waitForBackupScheduleTTL waits for a BackupSchedule to have a specific TTL
func waitForBackupScheduleTTL(
	ctx context.Context,
	k8sClient client.Client,
	name, namespace string,
	expectedTTL time.Duration,
	timeout, interval time.Duration,
) {
	Eventually(func() time.Duration {
		schedule := v1beta1.BackupSchedule{}
		err := k8sClient.Get(ctx, createLookupKey(name, namespace), &schedule)
		if err != nil {
			return time.Duration(0)
		}
		return schedule.Spec.VeleroTTL.Duration
	}, timeout, interval).Should(Equal(expectedTTL))
}

// waitForBackupScheduleVeleroSchedules waits for a BackupSchedule to have all Velero schedules created
func waitForBackupScheduleVeleroSchedules(
	ctx context.Context,
	k8sClient client.Client,
	name, namespace string,
	timeout, interval time.Duration,
) {
	Eventually(func() bool {
		schedule := v1beta1.BackupSchedule{}
		err := k8sClient.Get(ctx, createLookupKey(name, namespace), &schedule)
		if err != nil {
			return false
		}
		return schedule.Status.VeleroScheduleCredentials != nil &&
			schedule.Status.VeleroScheduleManagedClusters != nil &&
			schedule.Status.VeleroScheduleResources != nil
	}, timeout, interval).Should(BeTrue())
}

// waitForNamespaceActive waits for a namespace to be in Active phase
func waitForNamespaceActive(
	ctx context.Context,
	k8sClient client.Client,
	namespaceName string,
	timeout, interval time.Duration,
) {
	Eventually(func() bool {
		namespace := corev1.Namespace{}
		err := k8sClient.Get(ctx, createLookupKey(namespaceName, ""), &namespace)
		if err != nil {
			return false
		}
		return namespace.Status.Phase == corev1.NamespaceActive
	}, timeout, interval).Should(BeTrue())
}

// waitForManagedClusterCount waits for a specific number of managed clusters
func waitForManagedClusterCount(
	ctx context.Context,
	k8sClient client.Client,
	expectedCount int,
	timeout, interval time.Duration,
) {
	Eventually(func() int {
		managedClusterList := &unstructured.UnstructuredList{}
		managedClusterList.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "cluster.open-cluster-management.io",
			Version: "v1",
			Kind:    "ManagedClusterList",
		})
		if err := k8sClient.List(ctx, managedClusterList, &client.ListOptions{}); err != nil {
			return -1
		}
		return len(managedClusterList.Items)
	}, timeout, interval).Should(BeNumerically("==", expectedCount))
}
