package controllers

import (
	"context"
	"time"

	"github.com/openshift/hive/apis/hive/v1/aws"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	v1beta1 "github.com/stolostron/cluster-backup-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	veleroapi "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	chnv1 "open-cluster-management.io/multicloud-operators-channel/pkg/apis/apps/v1"

	ocinfrav1 "github.com/openshift/api/config/v1"
)

var _ = Describe("BackupSchedule controller", func() {
	logger := zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))

	// Test Constants
	const (
		// Test timing configuration
		defaultTimeout  = time.Second * 9        // Maximum wait time for async operations
		defaultInterval = time.Millisecond * 250 // Polling interval for Eventually/Consistently checks
		longTimeout     = time.Second * 65       // Extended timeout for complex operations

		// Namespace names
		defaultVeleroNamespace     = "velero-ns"
		defaultACMNamespace        = "acm-ns"
		defaultChartsNamespace     = "acm-channel-ns"
		defaultManagedClusterNS    = "managed1"
		defaultClusterPoolNS       = "app"
		defaultClusterDeploymentNS = "vb-pool-fhbjs"
		defaultMachineAPINS        = "openshift-machine-api"

		// Schedule configuration
		defaultBackupScheduleName = "the-backup-schedule-name"
		defaultCronSchedule       = "0 */6 * * *"
		invalidCronExpression     = "invalid-cron-exp"

		// TTL values
		defaultVeleroTTL = time.Hour * 72
		extendedTTL      = time.Hour * 150
		reducedTTL       = time.Hour * 50
		msaTTL           = time.Hour * 90
		shortTTL         = time.Second * 5
		zeroTTL          = time.Duration(0) // Zero duration for TTL

		// Test timing delays
		backupCollisionDelay = time.Second * 7 // Sleep duration to trigger backup collision detection

		// Expected test counts
		expectedACMScheduleCount = 5 // Minimum expected number of ACM schedules in tests

		// Storage location names
		defaultStorageLocationName = "default"
		newStorageLocationName     = "default-new"

		// Test data timestamps
		timestamp1 = "20210910181336"
		timestamp2 = "20210910181337"
		timestamp3 = "20210910181338"

		// Secret names
		poolCredsSecretName      = "app-prow-47-aws-creds"
		autoImportSecretName     = "auto-import-account"
		autoImportPairSecretName = "auto-import-account-pair"
		otherMSASecretName       = "some-other-msa-account"
		baremetalSecretName      = "baremetal"
		baremetalAPISecretName   = "baremetal-api-secret"
		aiSecretName             = "ai-secret"
	)

	var (
		ctx                     context.Context
		managedClusters         []clusterv1.ManagedCluster
		managedClustersAddons   []addonv1alpha1.ManagedClusterAddOn
		clusterDeployments      []hivev1.ClusterDeployment
		channels                []chnv1.Channel
		clusterPools            []hivev1.ClusterPool
		clusterVersions         []ocinfrav1.ClusterVersion
		backupStorageLocation   *veleroapi.BackupStorageLocation
		veleroBackups           []veleroapi.Backup
		veleroNamespaceName     string
		acmNamespaceName        string
		chartsv1NSName          string
		managedClusterNSName    string
		clusterPoolNSName       string
		veleroNamespace         *corev1.Namespace
		acmNamespace            *corev1.Namespace
		chartsv1NS              *corev1.Namespace
		clusterPoolNS           *corev1.Namespace
		aINS                    *corev1.Namespace
		managedClusterNS        *corev1.Namespace
		clusterPoolSecrets      []corev1.Secret
		clusterDeplSecrets      []corev1.Secret
		clusterDeploymentNSName string
		clusterDeploymentNS     *corev1.Namespace

		backupTimestamps = []string{
			timestamp1,
			timestamp2,
			timestamp3,
		}

		backupScheduleName = defaultBackupScheduleName
		backupSchedule     = defaultCronSchedule
		timeout            = defaultTimeout
		interval           = defaultInterval
	)

	BeforeEach(func() {
		ctx = context.Background()
		veleroNamespaceName = defaultVeleroNamespace
		acmNamespaceName = defaultACMNamespace
		chartsv1NSName = defaultChartsNamespace
		managedClusterNSName = defaultManagedClusterNS
		clusterPoolNSName = defaultClusterPoolNS
		clusterDeploymentNSName = defaultClusterDeploymentNS

		clusterVersions = []ocinfrav1.ClusterVersion{
			*createClusterVersion("version", "aaa", nil),
		}
		managedClustersAddons = []addonv1alpha1.ManagedClusterAddOn{
			{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "addon.open-cluster-management.io/v1alpha1",
					Kind:       "ManagedClusterAddOn",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "managed1-addon",
					Namespace: managedClusterNSName,
				},
				Spec: addonv1alpha1.ManagedClusterAddOnSpec{
					InstallNamespace: "managed1",
				},
			},
			{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "addon.open-cluster-management.io/v1alpha1",
					Kind:       "ManagedClusterAddOn",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "managed2-addon",
					Namespace: managedClusterNSName,
				},
				Spec: addonv1alpha1.ManagedClusterAddOnSpec{},
			},
		}

		managedClusters = []clusterv1.ManagedCluster{
			*createManagedCluster("lcluster", true /* the local cluster */).object,
			*createManagedCluster(managedClusterNSName, false).object,
		}
		managedClusterNS = createNamespace(managedClusterNSName)
		chartsv1NS = createNamespace(chartsv1NSName)
		clusterPoolNS = createNamespace(clusterPoolNSName)
		aINS = createNamespace(defaultMachineAPINS)
		clusterDeploymentNS = createNamespace(clusterDeploymentNSName)
		veleroNamespace = createNamespace(veleroNamespaceName)
		acmNamespace = createNamespace(acmNamespaceName)

		clusterPoolSecrets = []corev1.Secret{
			*createSecret(poolCredsSecretName, clusterPoolNSName,
				nil, nil, nil),
			*createSecret(autoImportSecretName, clusterPoolNSName,
				map[string]string{
					"authentication.open-cluster-management.io/is-managed-serviceaccount": "true",
				}, map[string]string{
					"expirationTimestamp":  "2024-08-05T15:25:34Z",
					"lastRefreshTimestamp": "2022-07-26T15:25:34Z",
				}, nil),
			*createSecret(autoImportPairSecretName, clusterPoolNSName,
				map[string]string{
					"authentication.open-cluster-management.io/is-managed-serviceaccount": "true",
				}, map[string]string{
					"expirationTimestamp":  "2024-08-05T15:25:34Z",
					"lastRefreshTimestamp": "2022-07-26T15:25:34Z",
				}, nil),
			*createSecret(otherMSASecretName, clusterPoolNSName,
				map[string]string{
					"authentication.open-cluster-management.io/is-managed-serviceaccount": "true",
				}, map[string]string{
					"expirationTimestamp":  "2024-08-05T15:25:34Z",
					"lastRefreshTimestamp": "2022-07-26T15:25:34Z",
				}, nil),
			*createSecret(baremetalSecretName, clusterPoolNSName,
				map[string]string{
					"environment.metal3.io": "baremetal",
				}, nil, nil),
			*createSecret(aiSecretName, clusterPoolNSName,
				map[string]string{
					"agent-install.openshift.io/watch": "true",
				}, nil, nil),
			*createSecret(baremetalAPISecretName, defaultMachineAPINS,
				map[string]string{
					"environment.metal3.io": "baremetal",
				}, nil, nil),
		}
		clusterDeplSecrets = []corev1.Secret{
			*createSecret(clusterDeploymentNSName+"-abcd", clusterDeploymentNSName,
				nil, nil, nil),
		}
		clusterPools = []hivev1.ClusterPool{
			{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "hive.openshift.io/v1",
					Kind:       "ClusterPool",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "app-prow-47",
					Namespace: clusterPoolNSName,
				},
				Spec: hivev1.ClusterPoolSpec{
					Platform: hivev1.Platform{
						AWS: &aws.Platform{
							Region: "us-east-2",
						},
					},
					Size:       4,
					BaseDomain: "d.red-c.com",
				},
			},
		}
		clusterDeployments = []hivev1.ClusterDeployment{
			{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "hive.openshift.io/v1",
					Kind:       "ClusterDeployment",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      clusterDeploymentNSName,
					Namespace: clusterDeploymentNSName,
				},
				Spec: hivev1.ClusterDeploymentSpec{
					ClusterPoolRef: &hivev1.ClusterPoolReference{
						Namespace: clusterPoolNSName,
						PoolName:  clusterPoolNSName,
					},
					BaseDomain: "d.red-c.com",
				},
			},
			{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "hive.openshift.io/v1",
					Kind:       "ClusterDeployment",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      veleroNamespaceName,
					Namespace: veleroNamespaceName,
					Labels: map[string]string{
						"hive.openshift.io/disable-creation-webhook-for-dr": "true",
					},
				},
				Spec: hivev1.ClusterDeploymentSpec{
					ClusterPoolRef: &hivev1.ClusterPoolReference{
						Namespace: clusterPoolNSName + "1",
						PoolName:  clusterPoolNSName + "1",
					},
					BaseDomain: "d.red-c.com",
				},
			},
		}
		channels = []chnv1.Channel{
			*createChannel("charts-v1", chartsv1NSName,
				chnv1.ChannelTypeHelmRepo, "http://test.svc.cluster.local:3000/charts").object,
			*createChannel("user-channel", "default",
				chnv1.ChannelTypeGit, "https://github.com/test/app-samples").object,
		}

		veleroBackups = []veleroapi.Backup{}
		oneHourAgo := metav1.NewTime(time.Now().Add(-1 * time.Hour))
		aFewSecondsAgo := metav1.NewTime(time.Now().Add(-2 * time.Second))

		// create 3 sets of backups, for each timestamp
		for _, timestampStr := range backupTimestamps {
			for key, value := range veleroScheduleNames {

				backup := *createBackup(value+"-"+timestampStr, veleroNamespaceName).
					labels(map[string]string{
						"velero.io/schedule-name":  value,
						BackupScheduleClusterLabel: "abcd",
					}).
					phase(veleroapi.BackupPhaseCompleted).startTimestamp(aFewSecondsAgo).errors(0).
					object

				if key == ValidationSchedule {

					// mark it as expired
					backup.Spec.TTL = metav1.Duration{Duration: shortTTL}
					backup.Status.Expiration = &oneHourAgo

				}

				veleroBackups = append(veleroBackups, backup)
			}
		}

		// create some dummy backups
		veleroBackups = append(
			veleroBackups,
			*createBackup(veleroScheduleNames[Resources]+"-new", veleroNamespaceName).
				phase(veleroapi.BackupPhaseCompleted).startTimestamp(oneHourAgo).errors(0).
				object,
		)
	})

	AfterEach(func() {
		if managedClusterNS != nil {
			for i := range managedClustersAddons {
				Expect(k8sClient.Delete(ctx, &managedClustersAddons[i])).Should(Succeed())
			}
		}

		for i := range managedClusters {
			Expect(k8sClient.Delete(ctx, &managedClusters[i])).Should(Succeed())
		}
		for i := range channels {
			Expect(k8sClient.Delete(ctx, &channels[i])).Should(Succeed())
		}
		for i := range veleroBackups {
			Expect(k8sClient.Delete(ctx, &veleroBackups[i])).Should(Succeed())
		}
		Expect(k8sClient.Delete(ctx, backupStorageLocation)).Should(Succeed())

		var zero int64 = 0

		if clusterDeploymentNS != nil {

			for i := range clusterDeplSecrets {
				Expect(k8sClient.Delete(ctx, &clusterDeplSecrets[i])).Should(Succeed())
			}

			for i := range clusterDeployments {
				Expect(k8sClient.Delete(ctx, &clusterDeployments[i])).Should(Succeed())
			}

			Expect(
				k8sClient.Delete(
					ctx,
					clusterDeploymentNS,
					&client.DeleteOptions{GracePeriodSeconds: &zero},
				),
			).Should(Succeed())
		}

		if aINS != nil {
			Expect(k8sClient.Delete(ctx, aINS,
				&client.DeleteOptions{GracePeriodSeconds: &zero})).Should(Succeed())
		}

		if clusterPoolNS != nil {

			for i := range clusterPoolSecrets {
				if clusterPoolSecrets[i].Name == "auto-import-account" || clusterPoolSecrets[i].Name == "auto-import-account-pair" {
					// this should be already cleaned up by the MSA disabled function
					Expect(k8sClient.Delete(ctx, &clusterPoolSecrets[i])).ShouldNot(Succeed())
				} else {
					Expect(k8sClient.Delete(ctx, &clusterPoolSecrets[i])).Should(Succeed())
				}
			}
			for i := range clusterPools {
				Expect(k8sClient.Delete(ctx, &clusterPools[i])).Should(Succeed())
			}
			Expect(
				k8sClient.Delete(
					ctx,
					clusterPoolNS,
					&client.DeleteOptions{GracePeriodSeconds: &zero},
				),
			).Should(Succeed())
		}
		Expect(
			k8sClient.Delete(
				ctx,
				veleroNamespace,
				&client.DeleteOptions{GracePeriodSeconds: &zero},
			),
		).Should(Succeed())
		Expect(
			k8sClient.Delete(
				ctx,
				acmNamespace,
				&client.DeleteOptions{GracePeriodSeconds: &zero},
			),
		).Should(Succeed())
		Expect(
			k8sClient.Delete(
				ctx,
				chartsv1NS,
				&client.DeleteOptions{GracePeriodSeconds: &zero},
			),
		).Should(Succeed())
	})

	JustBeforeEach(func() {
		for i := range managedClusters {
			Expect(k8sClient.Create(ctx, &managedClusters[i])).Should(Succeed())
		}
		Expect(k8sClient.Create(ctx, veleroNamespace)).Should(Succeed())
		Expect(k8sClient.Create(ctx, acmNamespace)).Should(Succeed())
		Expect(k8sClient.Create(ctx, chartsv1NS)).Should(Succeed())

		if managedClusterNS != nil {
			Expect(k8sClient.Create(ctx, managedClusterNS)).Should(Succeed())

			for i := range managedClustersAddons {
				Expect(k8sClient.Create(ctx, &managedClustersAddons[i])).Should(Succeed())
			}
			for i := range clusterVersions {
				Expect(k8sClient.Create(ctx, &clusterVersions[i])).Should(Succeed())
			}

		}

		if clusterDeploymentNS != nil {
			Expect(k8sClient.Create(ctx, clusterDeploymentNS)).Should(Succeed())

			for i := range clusterDeplSecrets {
				Expect(k8sClient.Create(ctx, &clusterDeplSecrets[i])).Should(Succeed())
			}

			for i := range clusterDeployments {
				Expect(k8sClient.Create(ctx, &clusterDeployments[i])).Should(Succeed())
			}
		}

		if aINS != nil {
			Expect(k8sClient.Create(ctx, aINS)).Should(Succeed())
		}
		if clusterPoolNS != nil {
			Expect(k8sClient.Create(ctx, clusterPoolNS)).Should(Succeed())

			for i := range clusterPoolSecrets {
				Expect(k8sClient.Create(ctx, &clusterPoolSecrets[i])).Should(Succeed())
			}
			for i := range clusterPools {
				Expect(k8sClient.Create(ctx, &clusterPools[i])).Should(Succeed())
			}
		}

		for i := range channels {
			Expect(k8sClient.Create(ctx, &channels[i])).Should(Succeed())
		}

		for i := range veleroBackups {
			Expect(k8sClient.Create(ctx, &veleroBackups[i])).Should(Succeed())
		}

		// TODO: look for those expired validation schedule backups and ensure they got deleted? (DeleteBackupRequest)
	})
	Context("When creating a BackupSchedule", func() {
		It("Should be creating a Velero Schedule updating the Status", func() {
			backupStorageLocation = createStorageLocation(defaultStorageLocationName, veleroNamespaceName).
				setOwner().
				phase(veleroapi.BackupStorageLocationPhaseAvailable).object
			Expect(k8sClient.Create(ctx, backupStorageLocation)).Should(Succeed())
			storageLookupKey := createLookupKey(backupStorageLocation.Name, backupStorageLocation.Namespace)
			Expect(k8sClient.Get(ctx, storageLookupKey, backupStorageLocation)).Should(Succeed())
			backupStorageLocation.Status.Phase = veleroapi.BackupStorageLocationPhaseAvailable
			// Velero CRD doesn't have status subresource set, so simply update the
			// status with a normal update() call.
			Expect(k8sClient.Update(ctx, backupStorageLocation)).Should(Succeed())

			managedClusterList := clusterv1.ManagedClusterList{}
			Eventually(func() bool {
				err := k8sClient.List(ctx, &managedClusterList, &client.ListOptions{})
				return err == nil
			}, timeout, interval).Should(BeTrue())
			Expect(len(managedClusterList.Items)).To(BeNumerically("==", 2))

			createdVeleroNamespace := corev1.Namespace{}
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, createLookupKey(veleroNamespaceName, ""), &createdVeleroNamespace); err != nil {
					return false
				}
				if createdVeleroNamespace.Status.Phase == "Active" {
					return true
				}
				return false
			}, timeout, interval).Should(BeTrue())

			createdACMNamespace := corev1.Namespace{}
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, createLookupKey(acmNamespaceName, ""), &createdACMNamespace); err != nil {
					return false
				}
				if createdACMNamespace.Status.Phase == "Active" {
					return true
				}
				return false
			}, timeout, interval).Should(BeTrue())

			rhacmBackupSchedule := *createBackupScheduleWithDefaults(
				backupScheduleName,
				veleroNamespaceName,
				backupSchedule,
				metav1.Duration{Duration: defaultVeleroTTL},
				metav1.Duration{Duration: msaTTL},
			)
			Expect(k8sClient.Create(ctx, &rhacmBackupSchedule)).Should(Succeed())

			backupLookupKey := createLookupKey(backupScheduleName, veleroNamespaceName)
			createdBackupSchedule := v1beta1.BackupSchedule{}
			waitForObjectCreation(ctx, k8sClient, backupLookupKey, &createdBackupSchedule, timeout, interval)

			// validate baremetal secret has backup annotation
			baremetalSecret := corev1.Secret{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, createLookupKey(baremetalSecretName, clusterPoolNSName), &baremetalSecret)
				return err == nil &&
					baremetalSecret.GetLabels()["cluster.open-cluster-management.io/backup"] == "baremetal"
			}, timeout, interval).Should(BeTrue())

			// and the ones under openshift-machine-api dont
			baremetalSecretAPI := corev1.Secret{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, createLookupKey(baremetalAPISecretName, defaultMachineAPINS), &baremetalSecretAPI)
				return err == nil &&
					baremetalSecretAPI.GetLabels()["cluster.open-cluster-management.io/backup"] == "baremetal"
			}, timeout, interval).Should(BeFalse())

			// validate auto-import secret secret has backup annotation
			// if the UseManagedServiceAccount is set to true
			autoImportSecret := corev1.Secret{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, createLookupKey(autoImportSecretName, clusterPoolNSName), &autoImportSecret)
				return err == nil &&
					autoImportSecret.GetLabels()["cluster.open-cluster-management.io/backup"] == "msa"
			}, timeout, interval).Should(Equal(rhacmBackupSchedule.Spec.UseManagedServiceAccount))

			// verify pair secret gets backup label
			Eventually(func() bool {
				err := k8sClient.Get(ctx, createLookupKey(autoImportPairSecretName, clusterPoolNSName), &autoImportSecret)
				return err == nil &&
					autoImportSecret.GetLabels()["cluster.open-cluster-management.io/backup"] == "msa"
			}, timeout, interval).Should(Equal(rhacmBackupSchedule.Spec.UseManagedServiceAccount))

			// verify other type of msa secret DOES NOT get the backup label
			Eventually(func() bool {
				err := k8sClient.Get(ctx, createLookupKey(otherMSASecretName, clusterPoolNSName), &autoImportSecret)
				return err == nil &&
					autoImportSecret.GetLabels()["cluster.open-cluster-management.io/backup"] == "msa"
			}, timeout, interval).ShouldNot(Equal(rhacmBackupSchedule.Spec.UseManagedServiceAccount))

			// validate that the managedserviceaccount ManagedClusterAddOn is created for managed clusters since
			// useManagedServiceAccount is true
			var managedSvcAccountMCAOs []addonv1alpha1.ManagedClusterAddOn
			for i := range managedClusters {
				// managed-serviceaccount ManagedClusterAddOn should not be created for local-cluster
				if managedClusters[i].GetName() != "lcluster" { // The name of the local-cluster we created above
					managedSvcAccountMCAOs = append(managedSvcAccountMCAOs, addonv1alpha1.ManagedClusterAddOn{
						ObjectMeta: metav1.ObjectMeta{
							Name:      msa_addon,
							Namespace: managedClusters[i].GetName(),
						},
					})
				}
			}
			localClusterName := ""
			Eventually(func() bool {
				localCls, err := getLocalClusterName(ctx, k8sClient)
				localClusterName = localCls
				return err == nil
			}, timeout, interval).Should(BeTrue())

			Eventually(func() error {
				for i := range managedSvcAccountMCAOs {
					err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&managedSvcAccountMCAOs[i]),
						&managedSvcAccountMCAOs[i])
					if err != nil {
						return err
					}
				}
				return nil
			}, timeout, interval).Should(Succeed())
			for i := range managedSvcAccountMCAOs {
				mgdSvcAccountMCAO := managedSvcAccountMCAOs[i]
				Expect(mgdSvcAccountMCAO.GetName()).To(Equal(msa_addon))
				Expect(mgdSvcAccountMCAO.GetLabels()).To(Equal(map[string]string{msa_label: msa_service_name}))
				Expect(mgdSvcAccountMCAO.Spec.InstallNamespace).To(Equal("")) // Should not be set
			}

			// validate AI secret has backup annotation
			secretAI := corev1.Secret{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, createLookupKey(aiSecretName, clusterPoolNSName), &secretAI)
				return err == nil &&
					secretAI.GetLabels()["cluster.open-cluster-management.io/backup"] == "agent-install"
			}, timeout, interval).Should(BeTrue())

			Expect(createdBackupSchedule.CreationTimestamp.Time).NotTo(BeNil())

			Expect(createdBackupSchedule.Spec.VeleroSchedule).Should(Equal(backupSchedule))

			Expect(
				createdBackupSchedule.Spec.VeleroTTL,
			).Should(Equal(metav1.Duration{Duration: defaultVeleroTTL}))

			By("created backup schedule should contain velero schedules in status")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, backupLookupKey, &createdBackupSchedule)
				if err != nil {
					return false
				}

				schedulesCreated := createdBackupSchedule.Status.VeleroScheduleCredentials != nil &&
					createdBackupSchedule.Status.VeleroScheduleManagedClusters != nil &&
					createdBackupSchedule.Status.VeleroScheduleResources != nil

				if schedulesCreated {
					// verify the acm charts channel ns is excluded
					_, chartsNSOK := find(
						createdBackupSchedule.Status.VeleroScheduleResources.Spec.Template.ExcludedNamespaces,
						chartsv1NSName,
					)
					return chartsNSOK
				}

				return schedulesCreated
			}, timeout, interval).Should(BeTrue())

			Expect(
				createdBackupSchedule.Status.VeleroScheduleResources.Spec.Schedule,
			).Should(Equal(backupSchedule))

			Expect(
				createdBackupSchedule.Status.VeleroScheduleResources.Spec.Template.TTL,
			).Should(Equal(metav1.Duration{Duration: defaultVeleroTTL}))

			// update schedule, it should NOT trigger velero schedules deletion
			Eventually(func() error {
				// Re-load createdBackupSchedule to avoid timing issues with the schedule controller updating it
				// at the same time as this test
				err := k8sClient.Get(ctx, backupLookupKey, &createdBackupSchedule)
				if err != nil {
					return err
				}
				createdBackupSchedule.Spec.VeleroTTL = metav1.Duration{Duration: extendedTTL}
				return k8sClient.Update(context.Background(), &createdBackupSchedule, &client.UpdateOptions{})
			}, timeout, interval).Should(Succeed())
			Expect(
				k8sClient.
					Update(context.Background(), &createdBackupSchedule, &client.UpdateOptions{}),
			).Should(Succeed())

			Eventually(func() metav1.Duration {
				err := k8sClient.Get(ctx, backupLookupKey, &createdBackupSchedule)
				if err != nil {
					return metav1.Duration{Duration: zeroTTL}
				}
				return createdBackupSchedule.Spec.VeleroTTL
			}, timeout, interval).Should(BeIdenticalTo(metav1.Duration{Duration: extendedTTL}))

			// delete one schedule, it should trigger velero schedules recreation
			veleroSchedulesList := veleroapi.ScheduleList{}
			Eventually(func() bool {
				err := k8sClient.List(ctx, &veleroSchedulesList, &client.ListOptions{})
				return err == nil
			}, timeout, interval).Should(BeTrue())
			// check the volumeSnapshotLocation property
			Expect(
				veleroSchedulesList.Items[1].Spec.Template.VolumeSnapshotLocations,
			).Should(Equal([]string{"dpa-1"}))
			// check the UseOwnerReferencesInBackup property
			Expect(
				*veleroSchedulesList.Items[1].Spec.UseOwnerReferencesInBackup,
			).Should(BeTrue())
			// check the SkipImmediately property
			Expect(
				*veleroSchedulesList.Items[1].Spec.SkipImmediately,
			).Should(BeTrue())
			// verify clusterpool.other.hive.openshift.io to be in the managed cluster schedule and not in resources backup
			// because the includedActivationAPIGroupsByName contains other.hive.openshift.io
			for i := range veleroSchedulesList.Items {
				if veleroSchedulesList.Items[i].Name == veleroScheduleNames[Resources] {
					Expect(findValue(veleroSchedulesList.Items[i].Spec.Template.IncludedResources,
						"clusterpool.other.hive.openshift.io")).Should(BeFalse())
					// expect to find the local cluster ns in the ExcludedNamespaces list
					Expect(findValue(veleroSchedulesList.Items[i].Spec.Template.ExcludedNamespaces,
						localClusterName)).Should(BeTrue())

				}
				if veleroSchedulesList.Items[i].Name == veleroScheduleNames[ManagedClusters] {
					Expect(findValue(veleroSchedulesList.Items[i].Spec.Template.IncludedResources,
						"clusterpool.other.hive.openshift.io")).Should(BeTrue())
				}
			}

			Expect(k8sClient.Delete(ctx, &veleroSchedulesList.Items[1])).To(Succeed()) // Manually delete a schedule
			// count velero schedules, should be still len(veleroScheduleNames)
			Eventually(func() int {
				if err := k8sClient.List(ctx, &veleroSchedulesList, &client.ListOptions{}); err == nil {
					return len(veleroSchedulesList.Items)
				}
				return 0
			}, longTimeout, interval).Should(BeNumerically("==", len(veleroScheduleNames)))

			// check that the velero schedules have now 150h for ttl
			Eventually(func() metav1.Duration {
				err := k8sClient.Get(ctx, backupLookupKey, &createdBackupSchedule)
				if err != nil {
					return metav1.Duration{Duration: zeroTTL}
				}
				if createdBackupSchedule.Status.VeleroScheduleManagedClusters == nil {
					return metav1.Duration{Duration: zeroTTL}
				}
				return createdBackupSchedule.Status.VeleroScheduleManagedClusters.Spec.Template.TTL
			}, timeout, interval).Should(BeIdenticalTo(metav1.Duration{Duration: extendedTTL}))

			// count velero schedules, should be still len(veleroScheduleNames)
			waitForListOperation(ctx, k8sClient, &veleroSchedulesList, timeout, interval, &client.ListOptions{})
			Expect(len(veleroSchedulesList.Items)).To(BeNumerically("==", len(veleroScheduleNames)))
			//

			// new backup with no TTL
			backupScheduleNameNoTTL := backupScheduleName + "-nottl"
			rhacmBackupScheduleNoTTL := *createBackupSchedule(backupScheduleNameNoTTL, veleroNamespaceName).
				schedule(backupSchedule).
				object
			Expect(k8sClient.Create(ctx, &rhacmBackupScheduleNoTTL)).Should(Succeed())

			// execute a backup collision validation
			// first make sure the schedule is in enabled state
			Eventually(func() error {
				err := k8sClient.Get(ctx, backupLookupKey, &createdBackupSchedule)
				if err != nil {
					return err
				}
				createdBackupSchedule.Status.Phase = v1beta1.SchedulePhaseEnabled
				return k8sClient.Status().Update(ctx, &createdBackupSchedule)
			}, timeout, interval).Should(Succeed())
			Eventually(func() string {
				err := k8sClient.Get(ctx, backupLookupKey, &createdBackupSchedule)
				if err != nil {
					return err.Error()
				}
				return string(createdBackupSchedule.Status.Phase)
			}, timeout, interval).Should(BeIdenticalTo(string(v1beta1.SchedulePhaseEnabled)))
			// then sleep to let the schedule timestamp be older then 5 sec from now
			// then update the schedule, which will try to create a new set of velero schedules
			// when the clusterID is checked, it is going to be (unknown) - since we have no cluster resource on test
			// and the previous schedules had used abcd as clusterId
			time.Sleep(backupCollisionDelay)
			// get the schedule again
			Expect(k8sClient.Get(ctx, backupLookupKey, &createdBackupSchedule)).To(Succeed())
			createdBackupSchedule.Spec.VeleroTTL = metav1.Duration{Duration: reducedTTL}
			Eventually(func() bool {
				err := k8sClient.Update(
					context.Background(),
					&createdBackupSchedule,
					&client.UpdateOptions{},
				)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			// schedule should be in backup collision because the latest schedules were using a
			// different clusterID, so they look as they are generated by another cluster
			Eventually(func() string {
				err := k8sClient.Get(ctx, backupLookupKey, &createdBackupSchedule)
				if err != nil {
					return "unknown"
				}
				return string(createdBackupSchedule.Status.Phase)
			}, longTimeout, interval).Should(BeIdenticalTo(string(v1beta1.SchedulePhaseBackupCollision)))

			// new schedule backup
			backupScheduleName3 := backupScheduleName + "-3"
			backupSchedule3 := *createBackupSchedule(backupScheduleName3, veleroNamespaceName).
				schedule(backupSchedule).
				object
			Expect(k8sClient.Create(ctx, &backupSchedule3)).Should(Succeed())

			backupLookupKeyNoTTL := createLookupKey(backupScheduleNameNoTTL, veleroNamespaceName)
			createdBackupScheduleNoTTL := v1beta1.BackupSchedule{}
			waitForObjectCreation(ctx, k8sClient, backupLookupKeyNoTTL, &createdBackupScheduleNoTTL, timeout, interval)

			Expect(createdBackupScheduleNoTTL.CreationTimestamp.Time).NotTo(BeNil())

			Expect(
				createdBackupScheduleNoTTL.Spec.VeleroTTL,
			).Should(Equal(metav1.Duration{Duration: zeroTTL}))

			Expect(createdBackupScheduleNoTTL.Spec.VeleroSchedule).Should(Equal(backupSchedule))

			// schedules cannot be created because there already some running from the above schedule
			By(
				"created backup schedule should NOT contain velero schedules, acm-credentials-schedule already exists error",
			)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, backupLookupKeyNoTTL, &createdBackupScheduleNoTTL)
				if err != nil {
					return false
				}
				return createdBackupScheduleNoTTL.Status.VeleroScheduleCredentials != nil &&
					createdBackupScheduleNoTTL.Status.VeleroScheduleManagedClusters != nil &&
					createdBackupScheduleNoTTL.Status.VeleroScheduleResources != nil
			}, timeout, interval).ShouldNot(BeTrue())
			waitForSchedulePhase(ctx, k8sClient, backupScheduleNameNoTTL, veleroNamespaceName, v1beta1.SchedulePhaseFailed, timeout, interval)
			Expect(
				createdBackupScheduleNoTTL.Status.LastMessage,
			).Should(ContainSubstring("already exists"))

			// backup not created in velero namespace, should fail validation
			acmBackupName := backupScheduleName
			rhacmBackupScheduleACM := *createBackupSchedule(acmBackupName, acmNamespaceName /* NOT velero ns */).
				schedule(backupSchedule).veleroTTL(metav1.Duration{Duration: defaultVeleroTTL}).
				object
			Expect(k8sClient.Create(ctx, &rhacmBackupScheduleACM)).Should(Succeed())

			backupLookupKeyACM := createLookupKey(acmBackupName, acmNamespaceName)
			createdBackupScheduleACM := v1beta1.BackupSchedule{}
			waitForObjectCreation(ctx, k8sClient, backupLookupKeyACM, &createdBackupScheduleACM, timeout, interval)

			By(
				"backup schedule in acm ns should be in failed validation status - since it must be in the velero ns",
			)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, backupLookupKeyACM, &createdBackupScheduleACM)
				if err != nil {
					return false
				}
				return createdBackupScheduleACM.Status.Phase == v1beta1.SchedulePhaseFailedValidation
			}, timeout*2, interval).Should(BeTrue())
			logger.Info("backup schedule in wrong ns", "createdBackupScheduleACM", &createdBackupScheduleACM)
			Expect(
				createdBackupScheduleACM.Status.LastMessage,
			).Should(ContainSubstring("location is not available"))

			// backup with invalid cron job schedule, should fail validation
			invalidCronExpBackupName := backupScheduleName + "-invalid-cron-exp"
			invalidCronExpBackupScheduleACM := *createBackupSchedule(invalidCronExpBackupName, veleroNamespaceName).
				schedule(invalidCronExpression).veleroTTL(metav1.Duration{Duration: defaultVeleroTTL}).
				object
			Expect(k8sClient.Create(ctx, &invalidCronExpBackupScheduleACM)).Should(Succeed())

			backupLookupKeyInvalidCronExp := createLookupKey(invalidCronExpBackupName, veleroNamespaceName)
			createdBackupScheduleInvalidCronExp := v1beta1.BackupSchedule{}
			waitForObjectCreation(ctx, k8sClient, backupLookupKeyInvalidCronExp, &createdBackupScheduleInvalidCronExp, timeout, interval)

			By(
				"backup schedule with invalid cron exp should be in failed validation status",
			)
			Eventually(func() bool {
				err := k8sClient.Get(
					ctx,
					backupLookupKeyInvalidCronExp,
					&createdBackupScheduleInvalidCronExp,
				)
				if err != nil {
					return false
				}
				return createdBackupScheduleInvalidCronExp.Status.Phase == v1beta1.SchedulePhaseFailedValidation
			}, timeout, interval).Should(BeTrue())
			Expect(
				createdBackupScheduleInvalidCronExp.Status.LastMessage,
			).Should(ContainSubstring("invalid schedule: expected exactly 5 fields, found 1"))

			// update backup schedule with invalid exp to sth valid
			Eventually(func() bool {
				scheduleObj := createdBackupScheduleInvalidCronExp.DeepCopy()
				scheduleObj.Spec.VeleroSchedule = backupSchedule
				err := k8sClient.Update(ctx, scheduleObj, &client.UpdateOptions{})
				return err == nil
			}, timeout, interval).Should(BeTrue())

			createdBackupScheduleValidCronExp := v1beta1.BackupSchedule{}
			waitForObjectCreation(ctx, k8sClient, backupLookupKeyInvalidCronExp, &createdBackupScheduleValidCronExp, timeout, interval)

			By(
				"backup schedule now with valid cron exp should pass cron exp validation",
			)
			Eventually(func() string {
				err := k8sClient.Get(
					ctx,
					backupLookupKeyInvalidCronExp,
					&createdBackupScheduleValidCronExp,
				)
				if err != nil {
					return ""
				}
				return createdBackupScheduleValidCronExp.Status.LastMessage
			}, timeout*4, interval).Should(ContainSubstring("already exists"))
			Expect(
				createdBackupScheduleValidCronExp.Spec.VeleroSchedule,
			).Should(BeIdenticalTo(backupSchedule))

			// count ACM ( NOT velero) schedules, should still be just 5
			// this nb is NOT the nb of velero backup schedules created from
			// the acm backupschedule object ( these velero schedules are len(veleroScheduleNames))
			// acmSchedulesList represents the ACM - BackupSchedule.cluster.open-cluster-management.io - schedules
			// created by the tests
			acmSchedulesList := v1beta1.BackupScheduleList{}
			waitForListOperation(ctx, k8sClient, &acmSchedulesList, timeout, interval, &client.ListOptions{})
			Expect(len(acmSchedulesList.Items)).To(BeNumerically(">=", expectedACMScheduleCount))

			// count velero schedules
			veleroScheduleList := veleroapi.ScheduleList{}
			waitForListOperation(ctx, k8sClient, &veleroScheduleList, timeout, interval, &client.ListOptions{})
			Expect(len(veleroScheduleList.Items)).To(BeNumerically("==", len(veleroScheduleNames)))

			for i := range veleroScheduleList.Items {

				veleroSchedule := veleroScheduleList.Items[i]
				// validate resources schedule content
				switch veleroSchedule.Name {
				case "acm-resources-schedule":
					Expect(findValue(veleroSchedule.Spec.Template.IncludedResources,
						"placement.cluster.open-cluster-management.io")).Should(BeTrue())
					Expect(findValue(veleroSchedule.Spec.Template.IncludedResources,
						"clusterdeployment.hive.openshift.io")).Should(BeFalse())
					Expect(findValue(
						veleroSchedule.Spec.Template.IncludedResources, // excludedGroup
						"managedclustermutators.proxy.open-cluster-management.io",
					)).ShouldNot(BeTrue())
					Expect(findValue(veleroSchedule.Spec.Template.IncludedResources,
						"clusterpool.other.hive.openshift.io")).Should(BeFalse())

				case "acm-resources-generic-schedule": // generic resources, using backup label
					Expect(findValue(veleroSchedule.Spec.Template.ExcludedResources, // secrets are in the creds backup
						"secret")).Should(BeTrue())

					// resources excluded from backup should still be allowed to be backed up by the generic backup,
					// if they have a backup label
					Expect(findValue(veleroSchedule.Spec.Template.ExcludedResources,
						"clustermanagementaddon.addon.open-cluster-management.io")).ShouldNot(BeTrue())

					Expect(findValue(veleroSchedule.Spec.Template.ExcludedResources, // already in cluster resources backup
						"klusterletaddonconfig.agent.open-cluster-management.io")).Should(BeTrue())
					Expect(findValue(veleroSchedule.Spec.Template.ExcludedResources, // exclude this, part of mannged cluster
						"clusterpool.other.hive.openshift.io")).Should(BeTrue())

				case "acm-managed-clusters-schedule": // generic resources, using backup label
					Expect(findValue(veleroSchedule.Spec.Template.IncludedResources,
						"clusterdeployment.hive.openshift.io")).Should(BeTrue())
					//.other.hive.openshift.io included here
					Expect(findValue(veleroSchedule.Spec.Template.IncludedResources,
						"clusterpool.other.hive.openshift.io")).Should(BeTrue())
				}
			}

			// delete existing acm schedules
			for i := range acmSchedulesList.Items {
				Eventually(func() bool {
					scheduleObj := acmSchedulesList.Items[i].DeepCopy()
					err := k8sClient.Delete(ctx, scheduleObj)
					return err == nil
				}, timeout, interval).Should(BeTrue())
			}

			// acm schedules are 0 now
			waitForListOperation(ctx, k8sClient, &acmSchedulesList, timeout, interval, &client.ListOptions{})
			Expect(len(acmSchedulesList.Items)).To(BeNumerically("==", 0))
		})
	})

	Context("When BackupStorageLocation without OwnerReference is invalid", func() {
		newVeleroNamespace := "velero-ns-new"
		newAcmNamespace := "acm-ns-new"
		newChartsv1NSName := "acm-channel-ns-new"

		oneHourAgo := metav1.NewTime(time.Now().Add(-1 * time.Hour))

		BeforeEach(func() {
			clusterPoolNS = nil
			aINS = nil
			clusterDeploymentNS = nil
			managedClusterNS = nil
			chartsv1NS = createNamespace(newChartsv1NSName)
			acmNamespace = createNamespace(newAcmNamespace)
			veleroNamespace = createNamespace(newVeleroNamespace)

			channels = []chnv1.Channel{
				*createChannel("charts-v1", newChartsv1NSName,
					chnv1.ChannelTypeHelmRepo, "http://test.svc.cluster.local:3000/charts").object,
			}
			veleroBackups = []veleroapi.Backup{
				*createBackup(veleroScheduleNames[Resources], newVeleroNamespace).
					phase(veleroapi.BackupPhaseCompleted).startTimestamp(oneHourAgo).errors(0).
					object,
			}
			backupStorageLocation = createStorageLocation(newStorageLocationName, veleroNamespace.Name).
				phase(veleroapi.BackupStorageLocationPhaseUnavailable).object
		})
		It(
			"Should not create any velero schedule resources, BackupStorageLocation doesnt exist or is invalid",
			func() {
				rhacmBackupSchedule := *createBackupSchedule(backupScheduleName+"-new", newVeleroNamespace).
					schedule(backupSchedule).veleroTTL(metav1.Duration{Duration: defaultVeleroTTL}).
					object

				Expect(k8sClient.Create(ctx, &rhacmBackupSchedule)).Should(Succeed())
				// there is no storage location object created
				veleroSchedules := veleroapi.ScheduleList{}
				Eventually(func() bool {
					if err := k8sClient.List(ctx, &veleroSchedules, client.InNamespace(newVeleroNamespace)); err != nil {
						return false
					}
					return len(veleroSchedules.Items) == 0
				}, timeout, interval).Should(BeTrue())
				createdSchedule := v1beta1.BackupSchedule{}
				waitForSchedulePhase(ctx, k8sClient, backupScheduleName+"-new", newVeleroNamespace, v1beta1.SchedulePhaseFailedValidation, timeout, interval)
				// Get the schedule to check the status message
				Eventually(func() error {
					return k8sClient.Get(ctx, createLookupKey(backupScheduleName+"-new", newVeleroNamespace), &createdSchedule)
				}, timeout, interval).Should(Succeed())
				Expect(
					createdSchedule.Status.LastMessage,
				).Should(BeIdenticalTo("velero.io.BackupStorageLocation resources not found. " +
					"Verify you have created a konveyor.openshift.io.Velero or oadp.openshift.io.DataProtectionApplications " +
					"resource."))

				// create the storage location now but in the wrong ns
				Expect(k8sClient.Create(ctx, backupStorageLocation)).Should(Succeed())

				storageLookupKey := createLookupKey(backupStorageLocation.Name, backupStorageLocation.Namespace)
				Expect(k8sClient.Get(ctx, storageLookupKey, backupStorageLocation)).To(Succeed())
				backupStorageLocation.Status.Phase = veleroapi.BackupStorageLocationPhaseAvailable
				// Velero CRD doesn't have status subresource set, so simply update the
				// status with a normal update() call.
				Expect(k8sClient.Update(ctx, backupStorageLocation)).To(Succeed())

				rhacmBackupScheduleNew := *createBackupSchedule(backupScheduleName+"-new-1", newVeleroNamespace).
					schedule(backupSchedule).veleroTTL(metav1.Duration{Duration: defaultVeleroTTL}).
					object
				Expect(k8sClient.Create(ctx, &rhacmBackupScheduleNew)).Should(Succeed())
				createdScheduleNew := v1beta1.BackupSchedule{}
				waitForSchedulePhase(ctx, k8sClient, backupScheduleName+"-new-1", newVeleroNamespace, v1beta1.SchedulePhaseFailedValidation, timeout, interval)
				// Get the schedule to check the status message
				Eventually(func() error {
					return k8sClient.Get(ctx, createLookupKey(backupScheduleName+"-new-1", newVeleroNamespace), &createdScheduleNew)
				}, timeout, interval).Should(Succeed())
				Expect(
					createdScheduleNew.Status.LastMessage,
				).Should(BeIdenticalTo("Backup storage location is not available. " +
					"Check velero.io.BackupStorageLocation and validate storage credentials."))
			},
		)
	})
})
