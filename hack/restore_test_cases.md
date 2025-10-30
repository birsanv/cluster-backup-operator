# ACM Restore Test Cases - Label Selector Scenarios

This document describes the 8 comprehensive test cases that validate the correct behavior of label selectors during ACM restore operations.

## Overview

The restore process uses label selectors to control which resources are restored based on:
- Whether managed clusters are being restored (`latest`/specific name vs `skip`)
- Whether sync mode is enabled (`syncRestoreWithNewBackups: true`)
- Whether credentials/resources were originally set to `skip`

### Label Selector Types

- **`NotIn cluster-activation`**: Restores only passive (non-activation) resources
- **`In cluster-activation`**: Restores only activation resources
- **No label selector**: Restores ALL resources (both passive and activation)

---

## Test Case 1: Skip Clusters with Sync Enabled

**Configuration:**
```yaml
apiVersion: cluster.open-cluster-management.io/v1beta1
kind: Restore
metadata:
  name: restore-acm-all-sync
  namespace: open-cluster-management-backup
spec:
  syncRestoreWithNewBackups: true
  restoreSyncInterval: 10m
  cleanupBeforeRestore: None
  veleroManagedClustersBackupName: skip
  veleroCredentialsBackupName: latest
  veleroResourcesBackupName: latest
```

**Expected Behavior:**

The restore is enabled and syncing, but no activation data should be restored yet since managed clusters are set to `skip`.

**Credentials Restore:**
```yaml
labelSelector:
  matchExpressions:
    - key: cluster.open-cluster-management.io/backup
      operator: NotIn
      values:
        - cluster-activation
scheduleName: acm-credentials-schedule
```
- ✅ No `-active` suffix

**Generic Resources Restore:**
```yaml
labelSelector:
  matchExpressions:
    - key: cluster.open-cluster-management.io/backup
      operator: NotIn
      values:
        - cluster-activation
scheduleName: acm-resources-generic-schedule
```
- ✅ No `-active` suffix

**When Updated to `veleroManagedClustersBackupName: latest`:**

Both credentials and generic resources should create `-active` restores with:
```yaml
labelSelector:
  matchExpressions:
    - key: cluster.open-cluster-management.io/backup
      operator: In
      values:
        - cluster-activation
```

---

## Test Case 2: Skip Clusters without Sync

**Configuration:**
```yaml
apiVersion: cluster.open-cluster-management.io/v1beta1
kind: Restore
metadata:
  name: restore-acm-all-sync
  namespace: open-cluster-management-backup
spec:
  cleanupBeforeRestore: None
  veleroManagedClustersBackupName: skip
  veleroCredentialsBackupName: latest
  veleroResourcesBackupName: latest
```

**Expected Behavior:**

No `-active` restores should be created for credentials and generic resources.

**Both Credentials and Generic Resources Restore:**
```yaml
labelSelector:
  matchExpressions:
    - key: cluster.open-cluster-management.io/backup
      operator: NotIn
      values:
        - cluster-activation
```
- ✅ No `-active` suffix

---

## Test Case 3: Latest Clusters and Resources without Sync

**Configuration:**
```yaml
apiVersion: cluster.open-cluster-management.io/v1beta1
kind: Restore
metadata:
  name: restore-acm-all-sync
  namespace: open-cluster-management-backup
spec:
  cleanupBeforeRestore: None
  veleroManagedClustersBackupName: latest
  veleroCredentialsBackupName: latest
  veleroResourcesBackupName: latest
```

**Expected Behavior:**

Only one restore should be created for credentials and generic resources with **NO label selector** (restores all resources).

**Both Credentials and Generic Resources Restore:**
- ✅ No `In cluster-activation` label selector
- ✅ No `NotIn cluster-activation` label selector
- ✅ No `-active` suffix
- ✅ Restores ALL resources (both passive and activation)

---

## Test Case 4: Latest Clusters, Skip Credentials, Latest Resources

**Configuration:**
```yaml
apiVersion: cluster.open-cluster-management.io/v1beta1
kind: Restore
metadata:
  name: restore-acm-all-sync
  namespace: open-cluster-management-backup
spec:
  cleanupBeforeRestore: None
  veleroManagedClustersBackupName: latest
  veleroCredentialsBackupName: skip
  veleroResourcesBackupName: latest
```

**Expected Behavior:**

**Credentials Restore:**
```yaml
labelSelector:
  matchExpressions:
    - key: cluster.open-cluster-management.io/backup
      operator: In
      values:
        - cluster-activation
```
- ✅ No `-active` suffix
- ✅ Only activation credentials restored

**Generic Resources Restore:**
- ✅ No `In cluster-activation` label selector
- ✅ No `NotIn cluster-activation` label selector
- ✅ No `-active` suffix
- ✅ Restores ALL generic resources

---

## Test Case 5: Latest Clusters, Skip Credentials and Resources

**Configuration:**
```yaml
apiVersion: cluster.open-cluster-management.io/v1beta1
kind: Restore
metadata:
  name: restore-acm-all-sync
  namespace: open-cluster-management-backup
spec:
  cleanupBeforeRestore: None
  veleroManagedClustersBackupName: latest
  veleroCredentialsBackupName: skip
  veleroResourcesBackupName: skip
```

**Expected Behavior:**

Only one restore should be created for credentials and generic resources (name does not matter).

**Both Credentials and Generic Resources Restore:**
```yaml
labelSelector:
  matchExpressions:
    - key: cluster.open-cluster-management.io/backup
      operator: In
      values:
        - cluster-activation
```
- ✅ No `-active` suffix
- ✅ Only activation resources restored

**Should NOT see:**
```yaml
labelSelector:
  matchExpressions:
    - key: cluster.open-cluster-management.io/backup
      operator: NotIn
      values:
        - cluster-activation
```

---

## Test Case 6: Skip Clusters, Latest Credentials and Resources

**Configuration:**
```yaml
apiVersion: cluster.open-cluster-management.io/v1beta1
kind: Restore
metadata:
  name: restore-acm-all-sync
  namespace: open-cluster-management-backup
spec:
  cleanupBeforeRestore: None
  veleroManagedClustersBackupName: skip
  veleroCredentialsBackupName: latest
  veleroResourcesBackupName: latest
```

**Expected Behavior:**

Only one restore should be created for credentials and generic resources (name does not matter).

**Both Credentials and Generic Resources Restore:**
```yaml
labelSelector:
  matchExpressions:
    - key: cluster.open-cluster-management.io/backup
      operator: NotIn
      values:
        - cluster-activation
```
- ✅ No `-active` suffix
- ✅ Only passive resources restored

---

## Test Case 7: Specific Backup Names with PVC Wait

**Configuration:**
```yaml
apiVersion: cluster.open-cluster-management.io/v1beta1
kind: Restore
metadata:
  name: restore-acm-creds-20251029181055
  namespace: open-cluster-management-backup
spec:
  cleanupBeforeRestore: None
  veleroManagedClustersBackupName: acm-managed-clusters-schedule-20251029181055
  veleroCredentialsBackupName: acm-credentials-schedule-20251029181055
  veleroResourcesBackupName: acm-resources-schedule-20251029181055
```

**Expected Behavior:**

### Phase 1: PVC Wait

If a ConfigMap for PV is defined:
```yaml
kind: ConfigMap
apiVersion: v1
metadata:
  name: acm-pvcs-mongo-storage
  namespace: open-cluster-management-backup
  labels:
    cluster.open-cluster-management.io/backup-pvc: mongo-storage
data:
  Aa: bb
```

And no PV `mongo-storage:open-cluster-management-backup` exists, the restore should wait:

```yaml
status:
  lastMessage: 'waiting for PVC mongo-storage:open-cluster-management-backup'
  phase: Started
  veleroCredentialsRestoreName: restore-acm-creds-20251029181055-acm-credentials-schedule-20251029181055
```

### Phase 2: After PVC Creation

Once the PV `mongo-storage:open-cluster-management-backup` is created:

```yaml
status:
  completionTimestamp: '2025-10-30T19:26:26Z'
  lastMessage: All Velero restores have run successfully
  messages:
    - managed cluster vb-hub-b already available
    - No suitable MSA secret found for cluster (vb-managed-1)
  phase: Finished
  veleroCredentialsRestoreName: restore-acm-creds-20251029181055-acm-credentials-schedule-20251029181055
  veleroGenericResourcesRestoreName: restore-acm-creds-20251029181055-acm-resources-generic-schedule-20251029181055
  veleroManagedClustersRestoreName: restore-acm-creds-20251029181055-acm-managed-clusters-schedule-20251029181055
  veleroResourcesRestoreName: restore-acm-creds-20251029181055-acm-resources-schedule-20251029181055
```

**Generic Resources Restore:**
- ✅ No `In cluster-activation` label selector
- ✅ No `NotIn cluster-activation` label selector
- ✅ No `-active` suffix
- ✅ Restores ALL generic resources

---

## Test Case 8: Skip Clusters with Specific Backup Names

**Configuration:**
```yaml
apiVersion: cluster.open-cluster-management.io/v1beta1
kind: Restore
metadata:
  name: restore-acm-creds-20251029181055
  namespace: open-cluster-management-backup
spec:
  cleanupBeforeRestore: None
  veleroManagedClustersBackupName: skip
  veleroCredentialsBackupName: acm-credentials-schedule-20251029181055
  veleroResourcesBackupName: acm-resources-schedule-20251029181055
```

**Expected Behavior:**

Only one restore should be created for credentials and generic resources (name does not matter).

**Both Credentials and Generic Resources Restore:**
```yaml
labelSelector:
  matchExpressions:
    - key: cluster.open-cluster-management.io/backup
      operator: NotIn
      values:
        - cluster-activation
```
- ✅ No `-active` suffix
- ✅ Only passive resources restored

---

## Summary Table

| Case | Managed Clusters | Credentials | Resources | Sync | Label Selector | -active Suffix |
|------|-----------------|-------------|-----------|------|----------------|----------------|
| 1    | skip            | latest      | latest    | ✓    | NotIn          | ✗              |
| 2    | skip            | latest      | latest    | ✗    | NotIn          | ✗              |
| 3    | latest          | latest      | latest    | ✗    | none           | ✗              |
| 4    | latest          | skip        | latest    | ✗    | In (creds only)| ✗              |
| 5    | latest          | skip        | skip      | ✗    | In             | ✗              |
| 6    | skip            | latest      | latest    | ✗    | NotIn          | ✗              |
| 7    | name            | name        | name      | ✗    | none           | ✗              |
| 8    | skip            | name        | name      | ✗    | NotIn          | ✗              |

---

## Key Rules

### Rule 1: Managed Clusters = skip
**Only non-activation resources should be restored** (credentials or generic resources with label `NotIn cluster-activation`)

### Rule 2: Managed Clusters = latest/name
**Activation resources should be restored.** Non-activation resources should be restored if not already restored.

### Sync Mode Behavior
When `syncRestoreWithNewBackups: true` and managed clusters are set to `latest`:
- First sync cycle: Creates regular restore (without activation filter) to restore ALL credentials
- Subsequent sync cycles: Creates `-active` restore with activation filter to avoid:
  1. Name collisions with existing restores
  2. Re-restoring regular credentials that were already restored

---

## Test Implementation

All 8 test cases are implemented in `restore_test.go`:

1. `Test_restoreCase1_SkipClustersLatestCredsSync`
2. `Test_restoreCase2_SkipClustersLatestCredsNoSync`
3. `Test_restoreCase3_LatestClustersLatestCredsNoSync`
4. `Test_restoreCase4_LatestClustersSkipCredsLatestResourcesNoSync`
5. `Test_restoreCase5_LatestClustersSkipCredsSkipResourcesNoSync`
6. `Test_restoreCase6_SkipClustersLatestCredsLatestResourcesNoSync`
7. `Test_restoreCase7_SpecificBackupNamesNoSync`
8. `Test_restoreCase8_SkipClustersSpecificBackupNamesNoSync`

Each test validates:
- Correct label selector (`In`, `NotIn`, or `none`)
- Correct label selector operator
- Correct `-active` suffix presence/absence
- Correct `isCredsClsOnActiveStep` return value

Total: **8 test functions** with **16 subtests** (Credentials and ResourcesGeneric for each case)

