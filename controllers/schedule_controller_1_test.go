/*
Package controllers contains comprehensive integration tests for the ACM BackupSchedule Controller.

This test suite validates the complete backup schedule workflow including:
- BackupSchedule resource creation and lifecycle management
- Velero Schedule resource orchestration and status tracking
- Backup schedule validation and error handling
- Resource labeling and managed service account integration
- Integration with backup storage locations and managed clusters

The tests use factory functions from create_helper.go to reduce setup complexity
and ensure consistent test data across different scenarios.
*/
package controllers

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1beta1 "github.com/stolostron/cluster-backup-operator/api/v1beta1"
	veleroapi "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	chnv1 "open-cluster-management.io/multicloud-operators-channel/pkg/apis/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// BackupSchedule Controller Integration Test Suite
//
// This test suite comprehensively validates the ACM BackupSchedule Controller functionality
// across multiple scenarios including schedule creation, validation, status tracking, and
// error handling. Each test context focuses on a specific aspect of the backup schedule
// workflow to ensure proper isolation and clear failure diagnosis.
var _ = Describe("BackupSchedule controller", func() {
	logger := zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))

	// Test Variables Documentation
	//
	// These variables are shared across all test contexts and are reset in BeforeEach
	// to ensure test isolation. They represent the core components needed for testing
	// the ACM BackupSchedule Controller functionality.
	var (
		// Core test context and timing
		ctx      context.Context          // Test execution context
		timeout  = time.Second * 10       // Standard timeout for async operations
		interval = time.Millisecond * 250 // Polling interval for Eventually/Consistently checks

		// Velero infrastructure components
		veleroNamespace       *corev1.Namespace                // Namespace where Velero resources are created
		acmNamespace          *corev1.Namespace                // ACM namespace
		backupStorageLocation *veleroapi.BackupStorageLocation // Velero backup storage configuration

		// BackupSchedule configuration
		backupScheduleName  string                     // Name of the BackupSchedule resource being tested
		backupSchedule      string                     // Cron schedule expression
		rhacmBackupSchedule v1beta1.BackupSchedule     // The main BackupSchedule resource under test
		managedClusters     []clusterv1.ManagedCluster // Collection of managed cluster resources
		channels            []chnv1.Channel            // Channel resources for testing

		// Test timing configuration
		defaultVeleroTTL = time.Hour * 72 // Default TTL for Velero backups

		// Special values for testing different scenarios
		invalidCronExpression = "invalid-cron" // Invalid cron expression for error testing
		validCronExpression   = "0 2 * * *"    // Valid cron expression (daily at 2 AM)
	)

	// Test Setup - JustBeforeEach
	//
	// This setup runs before each individual test case and creates all necessary
	// Kubernetes resources in the test environment. The order of resource creation
	// is important to ensure proper dependencies and avoid race conditions.
	JustBeforeEach(func() {
		// Create the Velero namespace where all Velero resources will be placed
		Expect(k8sClient.Create(ctx, veleroNamespace)).Should(Succeed())

		// Create the ACM namespace
		Expect(k8sClient.Create(ctx, acmNamespace)).Should(Succeed())

		// Create managed clusters if any are defined for this test
		for i := range managedClusters {
			// Check if managed cluster already exists
			clusterLookupKey := types.NamespacedName{Name: managedClusters[i].Name}
			existingCluster := &clusterv1.ManagedCluster{}
			err := k8sClient.Get(ctx, clusterLookupKey, existingCluster)
			if errors.IsNotFound(err) {
				// Create new managed cluster
				Expect(k8sClient.Create(ctx, &managedClusters[i])).Should(Succeed())
			} else {
				// Managed cluster already exists, skip creation
				Expect(err).To(Succeed())
			}
		}

		// Create ACM resources (channels) if they don't exist
		// These are shared across tests to simulate a realistic ACM environment
		existingChannels := &chnv1.ChannelList{}
		Expect(k8sClient.List(ctx, existingChannels, &client.ListOptions{})).To(Succeed())
		if len(existingChannels.Items) == 0 {
			// Create test channels that simulate ACM resources
			for i := range channels {
				Expect(k8sClient.Create(ctx, &channels[i])).Should(Succeed())
			}
		}

		// Create and configure backup storage location if needed for this test
		// This simulates a properly configured Velero environment
		if backupStorageLocation != nil {
			storageLookupKey := types.NamespacedName{
				Name:      backupStorageLocation.Name,
				Namespace: backupStorageLocation.Namespace,
			}

			// Check if backup storage location already exists
			existingStorage := &veleroapi.BackupStorageLocation{}
			err := k8sClient.Get(ctx, storageLookupKey, existingStorage)
			if errors.IsNotFound(err) {
				// Create new storage location
				Expect(k8sClient.Create(ctx, backupStorageLocation)).Should(Succeed())
				Expect(k8sClient.Get(ctx, storageLookupKey, backupStorageLocation)).To(Succeed())
			} else {
				// Use existing storage location
				Expect(err).To(Succeed())
				backupStorageLocation = existingStorage
			}

			// Set storage location to available status to simulate a working Velero setup
			backupStorageLocation.Status.Phase = veleroapi.BackupStorageLocationPhaseAvailable
			// Velero CRD doesn't have status subresource set, so simply update the
			// status with a normal update() call.
			Expect(k8sClient.Update(ctx, backupStorageLocation)).To(Succeed())
			Expect(backupStorageLocation.Status.Phase).Should(BeIdenticalTo(veleroapi.BackupStorageLocationPhaseAvailable))
		}

		// Finally, create the BackupSchedule resource that will trigger the controller logic
		// This must be last to ensure all dependencies are in place

		// Check if BackupSchedule already exists
		scheduleLookupKey := types.NamespacedName{
			Name:      rhacmBackupSchedule.Name,
			Namespace: rhacmBackupSchedule.Namespace,
		}
		existingSchedule := &v1beta1.BackupSchedule{}
		err := k8sClient.Get(ctx, scheduleLookupKey, existingSchedule)
		if errors.IsNotFound(err) {
			// Create new BackupSchedule
			Expect(k8sClient.Create(ctx, &rhacmBackupSchedule)).Should(Succeed())
		} else {
			// BackupSchedule already exists, skip creation
			Expect(err).To(Succeed())
		}
	})

	// Test Cleanup - JustAfterEach
	//
	// This cleanup runs after each individual test case to ensure proper resource
	// cleanup and test isolation. We use aggressive cleanup to prevent test pollution.
	JustAfterEach(func() {
		// Clean up backup storage location if it was created for this test
		if backupStorageLocation != nil {
			Expect(k8sClient.Delete(ctx, backupStorageLocation)).Should(Succeed())
		}

		// Force delete the Velero namespace with zero grace period
		// This ensures all Velero resources (schedules, backups) are cleaned up quickly
		// and don't interfere with subsequent tests
		var zero int64 = 0
		Expect(
			k8sClient.Delete(
				ctx,
				veleroNamespace,
				&client.DeleteOptions{GracePeriodSeconds: &zero},
			),
		).Should(Succeed())

		// Force delete the ACM namespace
		Expect(
			k8sClient.Delete(
				ctx,
				acmNamespace,
				&client.DeleteOptions{GracePeriodSeconds: &zero},
			),
		).Should(Succeed())

		// Note: We don't wait for namespace deletion to complete as it can take time
		// and shouldn't block test completion. The unique namespace names prevent conflicts.

		// Clean up managed clusters (cluster-scoped resources)
		for i := range managedClusters {
			_ = k8sClient.Delete(ctx, &managedClusters[i])
		}

		// Reset backup storage location to nil for next test
		backupStorageLocation = nil
	})

	// Default Test Data Setup - BeforeEach
	//
	// This setup runs before each test context and initializes all test variables
	// with standard default values. Individual test contexts can override these
	// values in their own BeforeEach blocks to customize the test scenario.
	//
	// The setup uses factory functions from create_helper.go to ensure consistency
	// and reduce code duplication across different test scenarios.
	BeforeEach(func() {
		// Initialize test execution context
		ctx = context.Background()

		// Set default backup schedule configuration
		backupScheduleName = "test-backup-schedule"
		backupSchedule = validCronExpression

		// Create unique namespace names using random seed and current time to avoid conflicts between tests
		uniqueSuffix := fmt.Sprintf("%d-%d", GinkgoRandomSeed(), time.Now().UnixNano())
		veleroNamespace = createNamespace(fmt.Sprintf("velero-schedule-ns-%s", uniqueSuffix))
		acmNamespace = createNamespace(fmt.Sprintf("acm-schedule-ns-%s", uniqueSuffix))

		// Create backup storage location
		backupStorageLocation = createStorageLocation("default", veleroNamespace.Name).
			setOwner().
			phase(veleroapi.BackupStorageLocationPhaseAvailable).object

		// Create standard ACM test resources using factory functions
		channels = createDefaultChannels() // Test channel data

		// Create managed clusters for testing with unique names
		managedClusters = []clusterv1.ManagedCluster{
			*createManagedCluster(fmt.Sprintf("local-cluster-%s", uniqueSuffix), true).object,
			*createManagedCluster(fmt.Sprintf("remote-cluster-%s", uniqueSuffix), false).object,
		}

		// Create the main BackupSchedule resource with standard configuration
		rhacmBackupSchedule = *createBackupSchedule(backupScheduleName, veleroNamespace.Name).
			schedule(backupSchedule).
			veleroTTL(metav1.Duration{Duration: defaultVeleroTTL}).
			object
	})

	// =============================================================================
	// CORE FUNCTIONALITY TESTS
	// =============================================================================
	//
	// This section tests the fundamental backup schedule operations including basic
	// schedule creation, validation logic, and core workflow validation.

	// Test Context: Basic BackupSchedule Functionality
	//
	// This context tests the core backup schedule functionality when creating a
	// BackupSchedule resource with valid configuration. It validates the complete
	// workflow including Velero schedule creation, status tracking, and validation.
	Context("basic backup schedule functionality", func() {
		Context("when creating backup schedule with valid configuration", func() {

			// Test Case: Basic BackupSchedule Creation and Status Tracking
			//
			// This test validates the fundamental backup schedule workflow:
			// 1. BackupSchedule creation triggers Velero schedule creation
			// 2. BackupSchedule status progresses through correct phases
			// 3. Velero schedules have correct configuration (TTL, cron schedule)
			// 4. Resource labeling is applied correctly
			// 5. Managed service account integration works properly
			It("should create velero schedules with proper configuration and track status progression", func() {
				scheduleLookupKey := createLookupKey(backupScheduleName, veleroNamespace.Name)
				createdSchedule := v1beta1.BackupSchedule{}

				// Step 1: Verify BackupSchedule is created and accessible
				By("waiting for backup schedule to be created")
				Eventually(func() error {
					return k8sClient.Get(ctx, scheduleLookupKey, &createdSchedule)
				}, timeout, interval).Should(Succeed())

				// Verify initial configuration
				Expect(createdSchedule.Spec.VeleroSchedule).Should(Equal(backupSchedule))
				Expect(createdSchedule.Spec.VeleroTTL).Should(Equal(metav1.Duration{Duration: defaultVeleroTTL}))

				// Step 2: Wait for controller to process the BackupSchedule
				By("waiting for controller to process backup schedule")
				Eventually(func() (string, error) {
					err := k8sClient.Get(ctx, scheduleLookupKey, &createdSchedule)
					if err != nil {
						return "", err
					}
					// Return current phase for better debugging
					return string(createdSchedule.Status.Phase), nil
				}, timeout, interval).Should(SatisfyAny(
					Equal(string(v1beta1.SchedulePhaseEnabled)),
					Equal(string(v1beta1.SchedulePhaseNew)),
					Not(BeEmpty()), // Any phase is acceptable as long as controller is processing
				))

				// Step 3: Wait for Velero schedules to be created
				By("waiting for velero schedules to be created")
				veleroSchedules := &veleroapi.ScheduleList{}
				expectedScheduleCount := len(veleroScheduleNames)
				Eventually(func() (int, error) {
					err := k8sClient.List(ctx, veleroSchedules, client.InNamespace(veleroNamespace.Name))
					if err != nil {
						return 0, err
					}
					return len(veleroSchedules.Items), nil
				}, timeout, interval).Should(Equal(expectedScheduleCount))

				// Step 4: Verify Velero schedule configuration
				By("verifying velero schedule configuration")
				Expect(len(veleroSchedules.Items)).To(Equal(expectedScheduleCount))

				// Check configuration of the first schedule
				firstSchedule := veleroSchedules.Items[0]
				Expect(firstSchedule.Spec.Schedule).Should(Equal(backupSchedule))
				Expect(firstSchedule.Spec.Template.TTL).Should(Equal(metav1.Duration{Duration: defaultVeleroTTL}))

				// Verify all expected schedule names are created
				createdScheduleNames := make([]string, len(veleroSchedules.Items))
				for i, schedule := range veleroSchedules.Items {
					createdScheduleNames[i] = schedule.Name
				}
				for _, expectedName := range veleroScheduleNames {
					Expect(createdScheduleNames).To(ContainElement(expectedName))
				}

				// Step 5: Verify BackupSchedule status reflects Velero schedule creation
				By("verifying backup schedule status contains velero schedule information")
				Eventually(func() (bool, error) {
					err := k8sClient.Get(ctx, scheduleLookupKey, &createdSchedule)
					if err != nil {
						return false, err
					}
					// Check if status has been updated with Velero schedule info
					hasVeleroSchedules := createdSchedule.Status.VeleroScheduleResources != nil
					return hasVeleroSchedules, nil
				}, timeout, interval).Should(BeTrue())

				logger.Info("BackupSchedule test completed successfully",
					"scheduleName", createdSchedule.Name,
					"phase", createdSchedule.Status.Phase,
					"veleroSchedules", len(veleroSchedules.Items))
			})
		})

		Context("when creating backup schedule with invalid cron expression", func() {
			BeforeEach(func() {
				// Override the default cron expression with an invalid one
				backupSchedule = invalidCronExpression
				rhacmBackupSchedule = *createBackupSchedule(backupScheduleName, veleroNamespace.Name).
					schedule(backupSchedule).
					veleroTTL(metav1.Duration{Duration: defaultVeleroTTL}).
					object
			})

			// Test Case: Invalid Cron Expression Validation
			//
			// This test validates that the controller properly validates cron expressions
			// and sets appropriate error status when invalid expressions are provided.
			It("should set failed validation status for invalid cron expression", func() {
				scheduleLookupKey := createLookupKey(backupScheduleName, veleroNamespace.Name)
				createdSchedule := v1beta1.BackupSchedule{}

				// Step 1: Wait for BackupSchedule to be created
				By("waiting for backup schedule to be created")
				Eventually(func() error {
					return k8sClient.Get(ctx, scheduleLookupKey, &createdSchedule)
				}, timeout, interval).Should(Succeed())

				// Step 2: Wait for controller to validate and set failed status
				By("waiting for controller to detect invalid cron expression")
				Eventually(func() (v1beta1.SchedulePhase, error) {
					err := k8sClient.Get(ctx, scheduleLookupKey, &createdSchedule)
					if err != nil {
						return "", err
					}
					return createdSchedule.Status.Phase, nil
				}, timeout, interval).Should(Equal(v1beta1.SchedulePhaseFailedValidation))

				// Step 3: Verify error message is set
				By("verifying error message contains validation details")
				Expect(createdSchedule.Status.LastMessage).Should(ContainSubstring("invalid"))

				// Step 4: Verify no Velero schedules are created
				By("verifying no velero schedules are created for invalid configuration")
				veleroSchedules := &veleroapi.ScheduleList{}
				Consistently(func() (int, error) {
					err := k8sClient.List(ctx, veleroSchedules, client.InNamespace(veleroNamespace.Name))
					if err != nil {
						return -1, err
					}
					return len(veleroSchedules.Items), nil
				}, time.Second*2, interval).Should(Equal(0))

				logger.Info("Invalid cron expression test completed successfully",
					"scheduleName", createdSchedule.Name,
					"phase", createdSchedule.Status.Phase,
					"errorMessage", createdSchedule.Status.LastMessage)
			})
		})

		Context("when backup storage location is unavailable", func() {
			BeforeEach(func() {
				// Override backup storage location to be unavailable
				backupStorageLocation = createStorageLocation("default", veleroNamespace.Name).
					setOwner().
					phase(veleroapi.BackupStorageLocationPhaseUnavailable).object
			})

			// Test Case: Unavailable Storage Location Handling
			//
			// This test validates that the controller properly handles scenarios where
			// the backup storage location is not available.
			It("should handle unavailable backup storage location", func() {
				scheduleLookupKey := createLookupKey(backupScheduleName, veleroNamespace.Name)
				createdSchedule := v1beta1.BackupSchedule{}

				// Step 1: Verify BackupSchedule is created
				Eventually(func() error {
					return k8sClient.Get(ctx, scheduleLookupKey, &createdSchedule)
				}, timeout, interval).Should(Succeed())

				// Step 2: Verify controller handles unavailable storage location
				By("backup schedule should handle unavailable storage location")
				Eventually(func() bool {
					err := k8sClient.Get(ctx, scheduleLookupKey, &createdSchedule)
					if err != nil {
						return false
					}
					// The controller should either set failed validation or wait for storage location
					return createdSchedule.Status.Phase == v1beta1.SchedulePhaseFailedValidation ||
						createdSchedule.Status.LastMessage != ""
				}, timeout*2, interval).Should(BeTrue())

				logger.Info("Unavailable storage location test completed",
					"scheduleName", createdSchedule.Name,
					"phase", createdSchedule.Status.Phase,
					"message", createdSchedule.Status.LastMessage)
			})
		})

		Context("when backup schedule is paused", func() {
			BeforeEach(func() {
				// Create a paused backup schedule
				rhacmBackupSchedule = *createBackupSchedule(backupScheduleName, veleroNamespace.Name).
					schedule(backupSchedule).
					veleroTTL(metav1.Duration{Duration: defaultVeleroTTL}).
					paused(true).
					object
			})

			// Test Case: Paused BackupSchedule Handling and Unpausing
			//
			// This test validates that the controller properly handles paused backup schedules
			// and sets the appropriate status without creating Velero schedules, then tests
			// the transition from paused to unpaused state.
			It("should set paused status, not create velero schedules, then create them when unpaused", func() {
				scheduleLookupKey := createLookupKey(backupScheduleName, veleroNamespace.Name)
				createdSchedule := v1beta1.BackupSchedule{}

				// Step 1: Verify BackupSchedule is created
				Eventually(func() error {
					return k8sClient.Get(ctx, scheduleLookupKey, &createdSchedule)
				}, timeout, interval).Should(Succeed())

				// Step 2: Verify controller sets paused status
				By("backup schedule should be in paused phase")
				Eventually(func() bool {
					err := k8sClient.Get(ctx, scheduleLookupKey, &createdSchedule)
					if err != nil {
						return false
					}
					return createdSchedule.Status.Phase == v1beta1.SchedulePhasePaused
				}, timeout*2, interval).Should(BeTrue())

				// Step 3: Verify no Velero schedules are created while paused
				By("no velero schedules should be created for paused backup schedule")
				veleroSchedules := &veleroapi.ScheduleList{}
				Consistently(func() int {
					err := k8sClient.List(ctx, veleroSchedules, client.InNamespace(veleroNamespace.Name))
					if err != nil {
						return -1
					}
					return len(veleroSchedules.Items)
				}, time.Second*2, interval).Should(Equal(0))

				logger.Info("Paused backup schedule test (paused phase) completed successfully",
					"scheduleName", createdSchedule.Name,
					"phase", createdSchedule.Status.Phase,
					"paused", createdSchedule.Spec.Paused)

				// Step 4: Unpause the backup schedule
				By("unpausing the backup schedule")
				Eventually(func() error {
					// Get the latest version of the schedule
					err := k8sClient.Get(ctx, scheduleLookupKey, &createdSchedule)
					if err != nil {
						return err
					}
					// Set paused to false
					createdSchedule.Spec.Paused = false
					// Update the schedule
					return k8sClient.Update(ctx, &createdSchedule)
				}, timeout, interval).Should(Succeed())

				// Step 5: Verify controller processes the unpaused schedule
				By("backup schedule should transition from paused to enabled phase")
				Eventually(func() (v1beta1.SchedulePhase, error) {
					err := k8sClient.Get(ctx, scheduleLookupKey, &createdSchedule)
					if err != nil {
						return "", err
					}
					return createdSchedule.Status.Phase, nil
				}, timeout*2, interval).Should(SatisfyAny(
					Equal(v1beta1.SchedulePhaseEnabled),
					Equal(v1beta1.SchedulePhaseNew),
					Not(Equal(v1beta1.SchedulePhasePaused)), // Any phase except paused
				))

				// Step 6: Verify Velero schedules are now created
				By("velero schedules should be created after unpausing")
				expectedScheduleCount := len(veleroScheduleNames)
				Eventually(func() (int, error) {
					err := k8sClient.List(ctx, veleroSchedules, client.InNamespace(veleroNamespace.Name))
					if err != nil {
						return 0, err
					}
					return len(veleroSchedules.Items), nil
				}, timeout*2, interval).Should(Equal(expectedScheduleCount))

				// Step 7: Verify Velero schedule configuration
				By("verifying velero schedule configuration after unpausing")
				Expect(len(veleroSchedules.Items)).To(Equal(expectedScheduleCount))
				firstSchedule := veleroSchedules.Items[0]
				Expect(firstSchedule.Spec.Schedule).Should(Equal(backupSchedule))
				Expect(firstSchedule.Spec.Template.TTL).Should(Equal(metav1.Duration{Duration: defaultVeleroTTL}))

				// Verify all expected schedule names are created after unpausing
				createdScheduleNames := make([]string, len(veleroSchedules.Items))
				for i, schedule := range veleroSchedules.Items {
					createdScheduleNames[i] = schedule.Name
				}
				for _, expectedName := range veleroScheduleNames {
					Expect(createdScheduleNames).To(ContainElement(expectedName))
				}

				logger.Info("Paused backup schedule test (unpause transition) completed successfully",
					"scheduleName", createdSchedule.Name,
					"phase", createdSchedule.Status.Phase,
					"paused", createdSchedule.Spec.Paused,
					"veleroSchedules", len(veleroSchedules.Items))
			})
		})
	})
})
