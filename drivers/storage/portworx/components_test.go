package portworx

import (
	"context"
	"testing"

	corev1alpha1 "github.com/libopenstorage/operator/pkg/apis/core/v1alpha1"
	"github.com/portworx/sched-ops/k8s"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"
	fakediscovery "k8s.io/client-go/discovery/fake"
	fakek8sclient "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/record"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestBasicComponentsInstall(t *testing.T) {
	k8s.Instance().SetClient(
		fakek8sclient.NewSimpleClientset(),
		nil, nil, nil, nil, nil,
	)
	k8sClient := fake.NewFakeClient()
	driver := portworx{
		volumePlacementStrategyCRDCreated: true,
	}
	driver.Init(k8sClient, record.NewFakeRecorder(0))
	startPort := uint32(10001)

	cluster := &corev1alpha1.StorageCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "px-cluster",
			Namespace: "kube-test",
		},
		Spec: corev1alpha1.StorageClusterSpec{
			StartPort: &startPort,
			Placement: &corev1alpha1.PlacementSpec{
				NodeAffinity: &v1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
						NodeSelectorTerms: []v1.NodeSelectorTerm{
							{
								MatchExpressions: []v1.NodeSelectorRequirement{
									{
										Key:      "px/enabled",
										Operator: v1.NodeSelectorOpNotIn,
										Values:   []string{"false"},
									},
									{
										Key:      "node-role.kubernetes.io/master",
										Operator: v1.NodeSelectorOpDoesNotExist,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	err := driver.PreInstall(cluster)

	require.NoError(t, err)

	// Portworx ServiceAccount
	serviceAccountList := &v1.ServiceAccountList{}
	err = list(k8sClient, serviceAccountList)
	require.NoError(t, err)
	require.Len(t, serviceAccountList.Items, 1)

	sa := serviceAccountList.Items[0]
	require.Equal(t, pxServiceAccountName, sa.Name)
	require.Equal(t, cluster.Namespace, sa.Namespace)
	require.Len(t, sa.OwnerReferences, 1)
	require.Equal(t, cluster.Name, sa.OwnerReferences[0].Name)

	// Portworx ClusterRole
	clusterRoleList := &rbacv1.ClusterRoleList{}
	err = list(k8sClient, clusterRoleList)
	require.NoError(t, err)
	require.Len(t, clusterRoleList.Items, 1)

	expectedCR := getExpectedClusterRole(t, "portworxClusterRole.yaml")
	actualCR := clusterRoleList.Items[0]
	require.Equal(t, expectedCR.Name, actualCR.Name)
	require.Len(t, actualCR.OwnerReferences, 1)
	require.Equal(t, cluster.Name, actualCR.OwnerReferences[0].Name)
	require.ElementsMatch(t, expectedCR.Rules, actualCR.Rules)

	// Portworx ClusterRoleBinding
	crbList := &rbacv1.ClusterRoleBindingList{}
	err = list(k8sClient, crbList)
	require.NoError(t, err)
	require.Len(t, crbList.Items, 1)

	expectedCRB := getExpectedClusterRoleBinding(t, "portworxClusterRoleBinding.yaml")
	actualCRB := crbList.Items[0]
	require.Equal(t, expectedCRB.Name, actualCRB.Name)
	require.Len(t, actualCRB.OwnerReferences, 1)
	require.Equal(t, cluster.Name, actualCRB.OwnerReferences[0].Name)
	require.ElementsMatch(t, expectedCRB.Subjects, actualCRB.Subjects)
	require.Equal(t, expectedCRB.RoleRef, actualCRB.RoleRef)

	// Portworx Role
	roleList := &rbacv1.RoleList{}
	err = list(k8sClient, roleList)
	require.NoError(t, err)
	require.Len(t, roleList.Items, 1)

	expectedRole := getExpectedRole(t, "portworxRole.yaml")
	actualRole := roleList.Items[0]
	require.Equal(t, expectedRole.Name, actualRole.Name)
	require.Equal(t, expectedRole.Namespace, actualRole.Namespace)
	require.Len(t, actualRole.OwnerReferences, 1)
	require.Equal(t, cluster.Name, actualRole.OwnerReferences[0].Name)
	require.ElementsMatch(t, expectedRole.Rules, actualRole.Rules)

	// Portworx RoleBinding
	rbList := &rbacv1.RoleBindingList{}
	err = list(k8sClient, rbList)
	require.NoError(t, err)
	require.Len(t, rbList.Items, 1)

	expectedRB := getExpectedRoleBinding(t, "portworxRoleBinding.yaml")
	actualRB := rbList.Items[0]
	require.Equal(t, expectedRB.Name, actualRB.Name)
	require.Equal(t, expectedRB.Namespace, actualRB.Namespace)
	require.Len(t, actualRB.OwnerReferences, 1)
	require.Equal(t, cluster.Name, actualRB.OwnerReferences[0].Name)
	require.ElementsMatch(t, expectedRB.Subjects, actualRB.Subjects)
	require.Equal(t, expectedRB.RoleRef, actualRB.RoleRef)

	// Portworx Services
	serviceList := &v1.ServiceList{}
	err = list(k8sClient, serviceList)
	require.NoError(t, err)
	require.Len(t, serviceList.Items, 2)

	// Portworx Service
	expectedPXService := getExpectedService(t, "portworxService.yaml")
	pxService := &v1.Service{}
	err = get(k8sClient, pxService, pxServiceName, cluster.Namespace)
	require.NoError(t, err)
	require.Equal(t, expectedPXService.Name, pxService.Name)
	require.Equal(t, expectedPXService.Namespace, pxService.Namespace)
	require.Len(t, pxService.OwnerReferences, 1)
	require.Equal(t, cluster.Name, pxService.OwnerReferences[0].Name)
	require.Equal(t, expectedPXService.Labels, pxService.Labels)
	require.Equal(t, expectedPXService.Spec, pxService.Spec)

	// Portworx API Service
	expectedPxAPIService := getExpectedService(t, "portworxAPIService.yaml")
	pxAPIService := &v1.Service{}
	err = get(k8sClient, pxAPIService, pxAPIServiceName, cluster.Namespace)
	require.NoError(t, err)
	require.Equal(t, expectedPxAPIService.Name, pxAPIService.Name)
	require.Equal(t, expectedPxAPIService.Namespace, pxAPIService.Namespace)
	require.Len(t, pxAPIService.OwnerReferences, 1)
	require.Equal(t, cluster.Name, pxAPIService.OwnerReferences[0].Name)
	require.Equal(t, expectedPxAPIService.Labels, pxAPIService.Labels)
	require.Equal(t, expectedPxAPIService.Spec, pxAPIService.Spec)

	// Portworx API DaemonSet
	dsList := &appsv1.DaemonSetList{}
	err = list(k8sClient, dsList)
	require.NoError(t, err)
	require.Len(t, dsList.Items, 1)

	expectedDaemonSet := getExpectedDaemonSet(t, "portworxAPIDaemonSet.yaml")
	ds := dsList.Items[0]
	require.Equal(t, expectedDaemonSet.Name, ds.Name)
	require.Equal(t, expectedDaemonSet.Namespace, ds.Namespace)
	require.Len(t, ds.OwnerReferences, 1)
	require.Equal(t, cluster.Name, ds.OwnerReferences[0].Name)
	require.Equal(t, expectedDaemonSet.Spec, ds.Spec)
}

func TestPortworxServiceTypeForAKS(t *testing.T) {
	k8s.Instance().SetClient(
		fakek8sclient.NewSimpleClientset(),
		nil, nil, nil, nil, nil,
	)
	k8sClient := fake.NewFakeClient()
	driver := portworx{
		volumePlacementStrategyCRDCreated: true,
	}
	driver.Init(k8sClient, record.NewFakeRecorder(0))

	cluster := &corev1alpha1.StorageCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "px-cluster",
			Namespace: "kube-test",
			Annotations: map[string]string{
				annotationIsAKS: "true",
			},
		},
	}

	err := driver.PreInstall(cluster)

	require.NoError(t, err)

	pxService := &v1.Service{}
	err = get(k8sClient, pxService, pxServiceName, cluster.Namespace)
	require.NoError(t, err)
	require.Equal(t, pxServiceName, pxService.Name)
	require.Equal(t, cluster.Namespace, pxService.Namespace)
	require.Equal(t, v1.ServiceTypeLoadBalancer, pxService.Spec.Type)

	pxAPIService := &v1.Service{}
	err = get(k8sClient, pxAPIService, pxAPIServiceName, cluster.Namespace)
	require.NoError(t, err)
	require.Equal(t, pxAPIServiceName, pxAPIService.Name)
	require.Equal(t, cluster.Namespace, pxAPIService.Namespace)
	require.Equal(t, v1.ServiceTypeLoadBalancer, pxAPIService.Spec.Type)
}

func TestPortworxServiceTypeForGKE(t *testing.T) {
	k8s.Instance().SetClient(
		fakek8sclient.NewSimpleClientset(),
		nil, nil, nil, nil, nil,
	)
	k8sClient := fake.NewFakeClient()
	driver := portworx{
		volumePlacementStrategyCRDCreated: true,
	}
	driver.Init(k8sClient, record.NewFakeRecorder(0))

	cluster := &corev1alpha1.StorageCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "px-cluster",
			Namespace: "kube-test",
			Annotations: map[string]string{
				annotationIsGKE: "true",
			},
		},
	}

	err := driver.PreInstall(cluster)

	require.NoError(t, err)

	pxService := &v1.Service{}
	err = get(k8sClient, pxService, pxServiceName, cluster.Namespace)
	require.NoError(t, err)
	require.Equal(t, pxServiceName, pxService.Name)
	require.Equal(t, cluster.Namespace, pxService.Namespace)
	require.Equal(t, v1.ServiceTypeLoadBalancer, pxService.Spec.Type)

	pxAPIService := &v1.Service{}
	err = get(k8sClient, pxAPIService, pxAPIServiceName, cluster.Namespace)
	require.NoError(t, err)
	require.Equal(t, pxAPIServiceName, pxAPIService.Name)
	require.Equal(t, cluster.Namespace, pxAPIService.Namespace)
	require.Equal(t, v1.ServiceTypeLoadBalancer, pxAPIService.Spec.Type)
}

func TestPortworxServiceTypeForEKS(t *testing.T) {
	k8s.Instance().SetClient(
		fakek8sclient.NewSimpleClientset(),
		nil, nil, nil, nil, nil,
	)
	k8sClient := fake.NewFakeClient()
	driver := portworx{
		volumePlacementStrategyCRDCreated: true,
	}
	driver.Init(k8sClient, record.NewFakeRecorder(0))

	cluster := &corev1alpha1.StorageCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "px-cluster",
			Namespace: "kube-test",
			Annotations: map[string]string{
				annotationIsEKS: "true",
			},
		},
	}

	err := driver.PreInstall(cluster)
	require.NoError(t, err)

	pxService := &v1.Service{}
	err = get(k8sClient, pxService, pxServiceName, cluster.Namespace)
	require.NoError(t, err)
	require.Equal(t, pxServiceName, pxService.Name)
	require.Equal(t, cluster.Namespace, pxService.Namespace)
	require.Equal(t, v1.ServiceTypeLoadBalancer, pxService.Spec.Type)

	pxAPIService := &v1.Service{}
	err = get(k8sClient, pxAPIService, pxAPIServiceName, cluster.Namespace)
	require.NoError(t, err)
	require.Equal(t, pxAPIServiceName, pxAPIService.Name)
	require.Equal(t, cluster.Namespace, pxAPIService.Namespace)
	require.Equal(t, v1.ServiceTypeLoadBalancer, pxAPIService.Spec.Type)
}

func TestPVCControllerInstall(t *testing.T) {
	// Set fake kubernetes client for k8s version
	k8s.Instance().SetClient(
		fakek8sclient.NewSimpleClientset(),
		nil, nil, nil, nil, nil,
	)
	k8sClient := fake.NewFakeClient()
	driver := portworx{
		volumePlacementStrategyCRDCreated: true,
	}
	driver.Init(k8sClient, record.NewFakeRecorder(0))

	cluster := &corev1alpha1.StorageCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "px-cluster",
			Namespace: "kube-test",
			Annotations: map[string]string{
				annotationPVCController: "true",
			},
		},
	}

	err := driver.PreInstall(cluster)
	require.NoError(t, err)

	verifyPVCControllerInstall(t, cluster, k8sClient)
	verifyPVCControllerDeployment(t, cluster, k8sClient, "pvcControllerDeployment.yaml")
}

func TestPVCControllerWithInvalidValue(t *testing.T) {
	// Set fake kubernetes client for k8s version
	k8s.Instance().SetClient(
		fakek8sclient.NewSimpleClientset(),
		nil, nil, nil, nil, nil,
	)
	k8sClient := fake.NewFakeClient()
	driver := portworx{
		volumePlacementStrategyCRDCreated: true,
	}
	driver.Init(k8sClient, record.NewFakeRecorder(0))

	cluster := &corev1alpha1.StorageCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "px-cluster",
			Namespace: "kube-test",
			Annotations: map[string]string{
				annotationPVCController: "invalid",
			},
		},
	}

	err := driver.PreInstall(cluster)
	require.NoError(t, err)

	sa := &v1.ServiceAccount{}
	err = get(k8sClient, sa, pvcServiceAccountName, cluster.Namespace)
	require.True(t, errors.IsNotFound(err))

	cr := &rbacv1.ClusterRole{}
	err = get(k8sClient, cr, pvcClusterRoleName, "")
	require.True(t, errors.IsNotFound(err))

	crb := &rbacv1.ClusterRoleBinding{}
	err = get(k8sClient, crb, pvcClusterRoleBindingName, "")
	require.True(t, errors.IsNotFound(err))

	deployment := &appsv1.Deployment{}
	err = get(k8sClient, deployment, pvcDeploymentName, cluster.Namespace)
	require.True(t, errors.IsNotFound(err))
}

func TestPVCControllerInstallForOpenshift(t *testing.T) {
	// Set fake kubernetes client for k8s version
	k8s.Instance().SetClient(
		fakek8sclient.NewSimpleClientset(),
		nil, nil, nil, nil, nil,
	)
	k8sClient := fake.NewFakeClient()
	driver := portworx{
		volumePlacementStrategyCRDCreated: true,
	}
	driver.Init(k8sClient, record.NewFakeRecorder(0))

	cluster := &corev1alpha1.StorageCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "px-cluster",
			Namespace: "kube-test",
			Annotations: map[string]string{
				annotationIsOpenshift: "true",
			},
		},
	}

	err := driver.PreInstall(cluster)
	require.NoError(t, err)

	verifyPVCControllerInstall(t, cluster, k8sClient)
	verifyPVCControllerDeployment(t, cluster, k8sClient, "pvcControllerDeploymentOpenshift.yaml")

	// Despite invalid pvc controller annotation, install for openshift
	cluster.Annotations[annotationPVCController] = "invalid"

	err = driver.PreInstall(cluster)
	require.NoError(t, err)

	verifyPVCControllerInstall(t, cluster, k8sClient)
	verifyPVCControllerDeployment(t, cluster, k8sClient, "pvcControllerDeploymentOpenshift.yaml")
}

func TestPVCControllerInstallForOpenshiftInKubeSystem(t *testing.T) {
	// Set fake kubernetes client for k8s version
	k8s.Instance().SetClient(
		fakek8sclient.NewSimpleClientset(),
		nil, nil, nil, nil, nil,
	)
	k8sClient := fake.NewFakeClient()
	driver := portworx{
		volumePlacementStrategyCRDCreated: true,
	}
	driver.Init(k8sClient, record.NewFakeRecorder(0))

	cluster := &corev1alpha1.StorageCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "px-cluster",
			Namespace: "kube-system",
			Annotations: map[string]string{
				annotationIsOpenshift: "true",
			},
		},
	}

	err := driver.PreInstall(cluster)
	require.NoError(t, err)

	// PVC controller should not get installed if running in kube-system
	// namespace in openshift
	sa := &v1.ServiceAccount{}
	err = get(k8sClient, sa, pvcServiceAccountName, cluster.Namespace)
	require.True(t, errors.IsNotFound(err))

	cr := &rbacv1.ClusterRole{}
	err = get(k8sClient, cr, pvcClusterRoleName, "")
	require.True(t, errors.IsNotFound(err))

	crb := &rbacv1.ClusterRoleBinding{}
	err = get(k8sClient, crb, pvcClusterRoleBindingName, "")
	require.True(t, errors.IsNotFound(err))

	deployment := &appsv1.Deployment{}
	err = get(k8sClient, deployment, pvcDeploymentName, cluster.Namespace)
	require.True(t, errors.IsNotFound(err))
}

func TestPVCControllerInstallForPKS(t *testing.T) {
	// Set fake kubernetes client for k8s version
	k8s.Instance().SetClient(
		fakek8sclient.NewSimpleClientset(),
		nil, nil, nil, nil, nil,
	)
	k8sClient := fake.NewFakeClient()
	driver := portworx{
		volumePlacementStrategyCRDCreated: true,
	}
	driver.Init(k8sClient, record.NewFakeRecorder(0))

	cluster := &corev1alpha1.StorageCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "px-cluster",
			Namespace: "kube-test",
			Annotations: map[string]string{
				annotationIsPKS: "true",
			},
		},
	}

	err := driver.PreInstall(cluster)
	require.NoError(t, err)

	verifyPVCControllerInstall(t, cluster, k8sClient)
	verifyPVCControllerDeployment(t, cluster, k8sClient, "pvcControllerDeployment.yaml")

	// Despite invalid pvc controller annotation, install for PKS
	cluster.Annotations[annotationPVCController] = "invalid"

	err = driver.PreInstall(cluster)
	require.NoError(t, err)

	verifyPVCControllerInstall(t, cluster, k8sClient)
	verifyPVCControllerDeployment(t, cluster, k8sClient, "pvcControllerDeployment.yaml")
}

func TestPVCControllerInstallForEKS(t *testing.T) {
	// Set fake kubernetes client for k8s version
	k8s.Instance().SetClient(
		fakek8sclient.NewSimpleClientset(),
		nil, nil, nil, nil, nil,
	)
	k8sClient := fake.NewFakeClient()
	driver := portworx{
		volumePlacementStrategyCRDCreated: true,
	}
	driver.Init(k8sClient, record.NewFakeRecorder(0))

	cluster := &corev1alpha1.StorageCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "px-cluster",
			Namespace: "kube-test",
			Annotations: map[string]string{
				annotationIsEKS: "true",
			},
		},
	}

	err := driver.PreInstall(cluster)
	require.NoError(t, err)

	verifyPVCControllerInstall(t, cluster, k8sClient)
	verifyPVCControllerDeployment(t, cluster, k8sClient, "pvcControllerDeployment.yaml")

	// Despite invalid pvc controller annotation, install for EKS
	cluster.Annotations[annotationPVCController] = "invalid"

	err = driver.PreInstall(cluster)
	require.NoError(t, err)

	verifyPVCControllerInstall(t, cluster, k8sClient)
	verifyPVCControllerDeployment(t, cluster, k8sClient, "pvcControllerDeployment.yaml")
}

func TestPVCControllerInstallForGKE(t *testing.T) {
	// Set fake kubernetes client for k8s version
	k8s.Instance().SetClient(
		fakek8sclient.NewSimpleClientset(),
		nil, nil, nil, nil, nil,
	)
	k8sClient := fake.NewFakeClient()
	driver := portworx{
		volumePlacementStrategyCRDCreated: true,
	}
	driver.Init(k8sClient, record.NewFakeRecorder(0))

	cluster := &corev1alpha1.StorageCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "px-cluster",
			Namespace: "kube-test",
			Annotations: map[string]string{
				annotationIsGKE: "true",
			},
		},
	}

	err := driver.PreInstall(cluster)
	require.NoError(t, err)

	verifyPVCControllerInstall(t, cluster, k8sClient)
	verifyPVCControllerDeployment(t, cluster, k8sClient, "pvcControllerDeployment.yaml")

	// Despite invalid pvc controller annotation, install for GKE
	cluster.Annotations[annotationPVCController] = "invalid"

	err = driver.PreInstall(cluster)
	require.NoError(t, err)

	verifyPVCControllerInstall(t, cluster, k8sClient)
	verifyPVCControllerDeployment(t, cluster, k8sClient, "pvcControllerDeployment.yaml")
}

func TestPVCControllerInstallForAKS(t *testing.T) {
	// Set fake kubernetes client for k8s version
	k8s.Instance().SetClient(
		fakek8sclient.NewSimpleClientset(),
		nil, nil, nil, nil, nil,
	)
	k8sClient := fake.NewFakeClient()
	driver := portworx{
		volumePlacementStrategyCRDCreated: true,
	}
	driver.Init(k8sClient, record.NewFakeRecorder(0))

	cluster := &corev1alpha1.StorageCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "px-cluster",
			Namespace: "kube-test",
			Annotations: map[string]string{
				annotationIsAKS: "true",
			},
		},
	}

	err := driver.PreInstall(cluster)
	require.NoError(t, err)

	verifyPVCControllerInstall(t, cluster, k8sClient)
	verifyPVCControllerDeployment(t, cluster, k8sClient, "pvcControllerDeployment.yaml")

	// Despite invalid pvc controller annotation, install for AKS
	cluster.Annotations[annotationPVCController] = "invalid"

	err = driver.PreInstall(cluster)
	require.NoError(t, err)

	verifyPVCControllerInstall(t, cluster, k8sClient)
	verifyPVCControllerDeployment(t, cluster, k8sClient, "pvcControllerDeployment.yaml")
}

func TestPVCControllerWhenPVCControllerDisabledExplicitly(t *testing.T) {
	k8s.Instance().SetClient(
		fakek8sclient.NewSimpleClientset(),
		nil, nil, nil, nil, nil,
	)
	k8sClient := fake.NewFakeClient()
	driver := portworx{
		volumePlacementStrategyCRDCreated: true,
	}
	driver.Init(k8sClient, record.NewFakeRecorder(0))

	cluster := &corev1alpha1.StorageCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "px-cluster",
			Namespace: "kube-test",
			Annotations: map[string]string{
				annotationPVCController: "false",
				annotationIsPKS:         "true",
				annotationIsEKS:         "true",
				annotationIsGKE:         "true",
				annotationIsAKS:         "true",
				annotationIsOpenshift:   "true",
			},
		},
	}

	err := driver.PreInstall(cluster)
	require.NoError(t, err)

	sa := &v1.ServiceAccount{}
	err = get(k8sClient, sa, pvcServiceAccountName, cluster.Namespace)
	require.True(t, errors.IsNotFound(err))

	cr := &rbacv1.ClusterRole{}
	err = get(k8sClient, cr, pvcClusterRoleName, "")
	require.True(t, errors.IsNotFound(err))

	crb := &rbacv1.ClusterRoleBinding{}
	err = get(k8sClient, crb, pvcClusterRoleBindingName, "")
	require.True(t, errors.IsNotFound(err))

	deployment := &appsv1.Deployment{}
	err = get(k8sClient, deployment, pvcDeploymentName, cluster.Namespace)
	require.True(t, errors.IsNotFound(err))
}

func verifyPVCControllerInstall(
	t *testing.T,
	cluster *corev1alpha1.StorageCluster,
	k8sClient client.Client,
) {
	// PVC Controller ServiceAccount
	serviceAccountList := &v1.ServiceAccountList{}
	err := list(k8sClient, serviceAccountList)
	require.NoError(t, err)
	require.Len(t, serviceAccountList.Items, 2)

	sa := &v1.ServiceAccount{}
	err = get(k8sClient, sa, pvcServiceAccountName, cluster.Namespace)
	require.NoError(t, err)
	require.Equal(t, pvcServiceAccountName, sa.Name)
	require.Equal(t, cluster.Namespace, sa.Namespace)
	require.Len(t, sa.OwnerReferences, 1)
	require.Equal(t, cluster.Name, sa.OwnerReferences[0].Name)

	// PVC Controller ClusterRole
	clusterRoleList := &rbacv1.ClusterRoleList{}
	err = list(k8sClient, clusterRoleList)
	require.NoError(t, err)
	require.Len(t, clusterRoleList.Items, 2)

	expectedCR := getExpectedClusterRole(t, "pvcControllerClusterRole.yaml")
	actualCR := &rbacv1.ClusterRole{}
	err = get(k8sClient, actualCR, pvcClusterRoleName, "")
	require.NoError(t, err)
	require.Equal(t, expectedCR.Name, actualCR.Name)
	require.Len(t, actualCR.OwnerReferences, 1)
	require.Equal(t, cluster.Name, actualCR.OwnerReferences[0].Name)
	require.ElementsMatch(t, expectedCR.Rules, actualCR.Rules)

	// PVC Controller ClusterRoleBinding
	crbList := &rbacv1.ClusterRoleBindingList{}
	err = list(k8sClient, crbList)
	require.NoError(t, err)
	require.Len(t, crbList.Items, 2)

	expectedCRB := getExpectedClusterRoleBinding(t, "pvcControllerClusterRoleBinding.yaml")
	actualCRB := &rbacv1.ClusterRoleBinding{}
	err = get(k8sClient, actualCRB, pvcClusterRoleBindingName, "")
	require.NoError(t, err)
	require.Equal(t, expectedCRB.Name, actualCRB.Name)
	require.Len(t, actualCRB.OwnerReferences, 1)
	require.Equal(t, cluster.Name, actualCRB.OwnerReferences[0].Name)
	require.ElementsMatch(t, expectedCRB.Subjects, actualCRB.Subjects)
	require.Equal(t, expectedCRB.RoleRef, actualCRB.RoleRef)
}

func verifyPVCControllerDeployment(
	t *testing.T,
	cluster *corev1alpha1.StorageCluster,
	k8sClient client.Client,
	specFileName string,
) {
	// PVC Controller Deployment
	deploymentList := &appsv1.DeploymentList{}
	err := list(k8sClient, deploymentList)
	require.NoError(t, err)
	require.Len(t, deploymentList.Items, 1)

	expectedDeployment := getExpectedDeployment(t, specFileName)
	actualDeployment := deploymentList.Items[0]
	require.Equal(t, expectedDeployment.Name, actualDeployment.Name)
	require.Equal(t, expectedDeployment.Namespace, actualDeployment.Namespace)
	require.Len(t, actualDeployment.OwnerReferences, 1)
	require.Equal(t, cluster.Name, actualDeployment.OwnerReferences[0].Name)
	require.Equal(t, expectedDeployment.Labels, actualDeployment.Labels)
	require.Equal(t, expectedDeployment.Annotations, actualDeployment.Annotations)
	require.Equal(t, expectedDeployment.Spec, actualDeployment.Spec)
}

func TestPVCControllerCustomCPU(t *testing.T) {
	// Set fake kubernetes client for k8s version
	k8s.Instance().SetClient(
		fakek8sclient.NewSimpleClientset(),
		nil, nil, nil, nil, nil,
	)
	k8sClient := fake.NewFakeClient()
	driver := portworx{
		volumePlacementStrategyCRDCreated: true,
	}
	driver.Init(k8sClient, record.NewFakeRecorder(0))

	cluster := &corev1alpha1.StorageCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "px-cluster",
			Namespace: "kube-test",
			Annotations: map[string]string{
				annotationPVCController: "true",
			},
		},
	}

	err := driver.PreInstall(cluster)
	require.NoError(t, err)

	expectedCPUQuantity := resource.MustParse(defaultPVCControllerCPU)
	deployment := &appsv1.Deployment{}
	err = get(k8sClient, deployment, pvcDeploymentName, cluster.Namespace)
	require.NoError(t, err)
	require.Zero(t, expectedCPUQuantity.Cmp(deployment.Spec.Template.Spec.Containers[0].Resources.Requests[v1.ResourceCPU]))

	// Change the CPU resource required for PVC controller deployment
	expectedCPU := "300m"
	cluster.Annotations[annotationPVCControllerCPU] = expectedCPU

	err = driver.PreInstall(cluster)
	require.NoError(t, err)

	expectedCPUQuantity = resource.MustParse(expectedCPU)
	err = get(k8sClient, deployment, pvcDeploymentName, cluster.Namespace)
	require.NoError(t, err)
	require.Zero(t, expectedCPUQuantity.Cmp(deployment.Spec.Template.Spec.Containers[0].Resources.Requests[v1.ResourceCPU]))
}

func TestPVCControllerInvalidCPU(t *testing.T) {
	k8s.Instance().SetClient(
		fakek8sclient.NewSimpleClientset(),
		nil, nil, nil, nil, nil,
	)
	k8sClient := fake.NewFakeClient()
	driver := portworx{
		volumePlacementStrategyCRDCreated: true,
	}
	driver.Init(k8sClient, record.NewFakeRecorder(0))

	cluster := &corev1alpha1.StorageCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "px-cluster",
			Namespace: "kube-test",
			Annotations: map[string]string{
				annotationPVCController:    "true",
				annotationPVCControllerCPU: "invalid-cpu",
			},
		},
	}

	err := driver.PreInstall(cluster)
	require.Error(t, err)
}

func TestPVCControllerRollbackImageChanges(t *testing.T) {
	// Set fake kubernetes client for k8s version
	k8s.Instance().SetClient(
		fakek8sclient.NewSimpleClientset(),
		nil, nil, nil, nil, nil,
	)
	k8sClient := fake.NewFakeClient()
	driver := portworx{
		volumePlacementStrategyCRDCreated: true,
	}
	driver.Init(k8sClient, record.NewFakeRecorder(0))

	cluster := &corev1alpha1.StorageCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "px-cluster",
			Namespace: "kube-test",
			Annotations: map[string]string{
				annotationPVCController: "true",
			},
		},
	}

	err := driver.PreInstall(cluster)
	require.NoError(t, err)

	deployment := &appsv1.Deployment{}
	err = get(k8sClient, deployment, pvcDeploymentName, cluster.Namespace)
	require.NoError(t, err)
	require.Equal(t,
		"gcr.io/google_containers/kube-controller-manager-amd64:v0.0.0",
		deployment.Spec.Template.Spec.Containers[0].Image,
	)

	// Change the image of pvc controller deployment
	deployment.Spec.Template.Spec.Containers[0].Image = "foo/bar:v1"
	err = k8sClient.Update(context.TODO(), deployment)
	require.NoError(t, err)

	err = driver.PreInstall(cluster)
	require.NoError(t, err)

	err = get(k8sClient, deployment, pvcDeploymentName, cluster.Namespace)
	require.NoError(t, err)
	require.Equal(t,
		"gcr.io/google_containers/kube-controller-manager-amd64:v0.0.0",
		deployment.Spec.Template.Spec.Containers[0].Image,
	)
}

func TestPVCControllerRollbackCommandChanges(t *testing.T) {
	// Set fake kubernetes client for k8s version
	k8s.Instance().SetClient(
		fakek8sclient.NewSimpleClientset(),
		nil, nil, nil, nil, nil,
	)
	k8sClient := fake.NewFakeClient()
	driver := portworx{
		volumePlacementStrategyCRDCreated: true,
	}
	driver.Init(k8sClient, record.NewFakeRecorder(0))

	cluster := &corev1alpha1.StorageCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "px-cluster",
			Namespace: "kube-test",
			Annotations: map[string]string{
				annotationPVCController: "true",
			},
		},
	}

	err := driver.PreInstall(cluster)
	require.NoError(t, err)

	deployment := &appsv1.Deployment{}
	err = get(k8sClient, deployment, pvcDeploymentName, cluster.Namespace)
	require.NoError(t, err)
	expectedCommand := deployment.Spec.Template.Spec.Containers[0].Command

	// Change the command of pvc controller deployment
	deployment.Spec.Template.Spec.Containers[0].Command = append(
		deployment.Spec.Template.Spec.Containers[0].Command,
		"--new-arg=test",
	)
	err = k8sClient.Update(context.TODO(), deployment)
	require.NoError(t, err)

	err = driver.PreInstall(cluster)
	require.NoError(t, err)

	err = get(k8sClient, deployment, pvcDeploymentName, cluster.Namespace)
	require.NoError(t, err)
	require.ElementsMatch(t, expectedCommand, deployment.Spec.Template.Spec.Containers[0].Command)
}

func TestLighthouseInstall(t *testing.T) {
	k8s.Instance().SetClient(
		fakek8sclient.NewSimpleClientset(),
		nil, nil, nil, nil, nil,
	)
	k8sClient := fake.NewFakeClient()
	driver := portworx{
		volumePlacementStrategyCRDCreated: true,
	}
	driver.Init(k8sClient, record.NewFakeRecorder(0))

	cluster := &corev1alpha1.StorageCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "px-cluster",
			Namespace: "kube-test",
			Annotations: map[string]string{
				annotationPVCController: "true",
			},
		},
		Spec: corev1alpha1.StorageClusterSpec{
			UserInterface: &corev1alpha1.UserInterfaceSpec{
				Enabled: true,
				Image:   "portworx/px-lighthouse:2.1.1",
			},
		},
	}

	err := driver.PreInstall(cluster)

	require.NoError(t, err)

	// Lighthouse ServiceAccount
	serviceAccountList := &v1.ServiceAccountList{}
	err = list(k8sClient, serviceAccountList)
	require.NoError(t, err)
	require.Len(t, serviceAccountList.Items, 3)

	sa := &v1.ServiceAccount{}
	err = get(k8sClient, sa, lhServiceAccountName, cluster.Namespace)
	require.NoError(t, err)
	require.Equal(t, lhServiceAccountName, sa.Name)
	require.Equal(t, cluster.Namespace, sa.Namespace)
	require.Len(t, sa.OwnerReferences, 1)
	require.Equal(t, cluster.Name, sa.OwnerReferences[0].Name)

	// Lighthouse ClusterRole
	clusterRoleList := &rbacv1.ClusterRoleList{}
	err = list(k8sClient, clusterRoleList)
	require.NoError(t, err)
	require.Len(t, clusterRoleList.Items, 3)

	expectedCR := getExpectedClusterRole(t, "lighthouseClusterRole.yaml")
	actualCR := &rbacv1.ClusterRole{}
	err = get(k8sClient, actualCR, lhClusterRoleName, "")
	require.NoError(t, err)
	require.Equal(t, expectedCR.Name, actualCR.Name)
	require.Len(t, actualCR.OwnerReferences, 1)
	require.Equal(t, cluster.Name, actualCR.OwnerReferences[0].Name)
	require.ElementsMatch(t, expectedCR.Rules, actualCR.Rules)

	// Lighthouse ClusterRoleBinding
	crbList := &rbacv1.ClusterRoleBindingList{}
	err = list(k8sClient, crbList)
	require.NoError(t, err)
	require.Len(t, crbList.Items, 3)

	expectedCRB := getExpectedClusterRoleBinding(t, "lighthouseClusterRoleBinding.yaml")
	actualCRB := &rbacv1.ClusterRoleBinding{}
	err = get(k8sClient, actualCRB, lhClusterRoleBindingName, "")
	require.NoError(t, err)
	require.Equal(t, expectedCRB.Name, actualCRB.Name)
	require.Len(t, actualCRB.OwnerReferences, 1)
	require.Equal(t, cluster.Name, actualCRB.OwnerReferences[0].Name)
	require.ElementsMatch(t, expectedCRB.Subjects, actualCRB.Subjects)
	require.Equal(t, expectedCRB.RoleRef, actualCRB.RoleRef)

	// Lighthouse Service
	serviceList := &v1.ServiceList{}
	err = list(k8sClient, serviceList)
	require.NoError(t, err)
	require.Len(t, serviceList.Items, 3)

	expectedLhService := getExpectedService(t, "lighthouseService.yaml")
	lhService := &v1.Service{}
	err = get(k8sClient, lhService, lhServiceName, cluster.Namespace)
	require.NoError(t, err)
	require.Equal(t, expectedLhService.Name, lhService.Name)
	require.Equal(t, expectedLhService.Namespace, lhService.Namespace)
	require.Len(t, lhService.OwnerReferences, 1)
	require.Equal(t, cluster.Name, lhService.OwnerReferences[0].Name)
	require.Equal(t, expectedLhService.Labels, lhService.Labels)
	require.Equal(t, expectedLhService.Spec, lhService.Spec)

	// Lighthouse Deployment
	deploymentList := &appsv1.DeploymentList{}
	err = list(k8sClient, deploymentList)
	require.NoError(t, err)
	require.Len(t, deploymentList.Items, 2)

	expectedDeployment := getExpectedDeployment(t, "lighthouseDeployment.yaml")
	lhDeployment := &appsv1.Deployment{}
	err = get(k8sClient, lhDeployment, lhDeploymentName, cluster.Namespace)
	require.NoError(t, err)
	require.Equal(t, expectedDeployment.Name, lhDeployment.Name)
	require.Equal(t, expectedDeployment.Namespace, lhDeployment.Namespace)
	require.Len(t, lhDeployment.OwnerReferences, 1)
	require.Equal(t, cluster.Name, lhDeployment.OwnerReferences[0].Name)
	require.Equal(t, expectedDeployment.Labels, lhDeployment.Labels)
	require.Equal(t, expectedDeployment.Annotations, lhDeployment.Annotations)
	require.Equal(t, expectedDeployment.Spec, lhDeployment.Spec)
}

func TestLighthouseServiceTypeForAKS(t *testing.T) {
	k8s.Instance().SetClient(
		fakek8sclient.NewSimpleClientset(),
		nil, nil, nil, nil, nil,
	)
	k8sClient := fake.NewFakeClient()
	driver := portworx{
		volumePlacementStrategyCRDCreated: true,
	}
	driver.Init(k8sClient, record.NewFakeRecorder(0))

	cluster := &corev1alpha1.StorageCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "px-cluster",
			Namespace: "kube-test",
			Annotations: map[string]string{
				annotationIsAKS: "true",
			},
		},
		Spec: corev1alpha1.StorageClusterSpec{
			UserInterface: &corev1alpha1.UserInterfaceSpec{
				Enabled: true,
				Image:   "portworx/px-lighthouse:test",
			},
		},
	}

	err := driver.PreInstall(cluster)

	require.NoError(t, err)

	lhService := &v1.Service{}
	err = get(k8sClient, lhService, lhServiceName, cluster.Namespace)
	require.NoError(t, err)
	require.Equal(t, lhServiceName, lhService.Name)
	require.Equal(t, cluster.Namespace, lhService.Namespace)
	require.Equal(t, v1.ServiceTypeLoadBalancer, lhService.Spec.Type)
}

func TestLighthouseServiceTypeForGKE(t *testing.T) {
	k8s.Instance().SetClient(
		fakek8sclient.NewSimpleClientset(),
		nil, nil, nil, nil, nil,
	)
	k8sClient := fake.NewFakeClient()
	driver := portworx{
		volumePlacementStrategyCRDCreated: true,
	}
	driver.Init(k8sClient, record.NewFakeRecorder(0))

	cluster := &corev1alpha1.StorageCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "px-cluster",
			Namespace: "kube-test",
			Annotations: map[string]string{
				annotationIsGKE: "true",
			},
		},
		Spec: corev1alpha1.StorageClusterSpec{
			UserInterface: &corev1alpha1.UserInterfaceSpec{
				Enabled: true,
				Image:   "portworx/px-lighthouse:test",
			},
		},
	}

	err := driver.PreInstall(cluster)

	require.NoError(t, err)

	lhService := &v1.Service{}
	err = get(k8sClient, lhService, lhServiceName, cluster.Namespace)
	require.NoError(t, err)
	require.Equal(t, lhServiceName, lhService.Name)
	require.Equal(t, cluster.Namespace, lhService.Namespace)
	require.Equal(t, v1.ServiceTypeLoadBalancer, lhService.Spec.Type)
}

func TestLighthouseServiceTypeForEKS(t *testing.T) {
	k8s.Instance().SetClient(
		fakek8sclient.NewSimpleClientset(),
		nil, nil, nil, nil, nil,
	)
	k8sClient := fake.NewFakeClient()
	driver := portworx{
		volumePlacementStrategyCRDCreated: true,
	}
	driver.Init(k8sClient, record.NewFakeRecorder(0))

	cluster := &corev1alpha1.StorageCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "px-cluster",
			Namespace: "kube-test",
			Annotations: map[string]string{
				annotationIsEKS: "true",
			},
		},
		Spec: corev1alpha1.StorageClusterSpec{
			UserInterface: &corev1alpha1.UserInterfaceSpec{
				Enabled: true,
				Image:   "portworx/px-lighthouse:test",
			},
		},
	}

	err := driver.PreInstall(cluster)

	require.NoError(t, err)

	lhService := &v1.Service{}
	err = get(k8sClient, lhService, lhServiceName, cluster.Namespace)
	require.NoError(t, err)
	require.Equal(t, lhServiceName, lhService.Name)
	require.Equal(t, cluster.Namespace, lhService.Namespace)
	require.Equal(t, v1.ServiceTypeLoadBalancer, lhService.Spec.Type)
}

func TestLighthouseWithoutImage(t *testing.T) {
	k8s.Instance().SetClient(
		fakek8sclient.NewSimpleClientset(),
		nil, nil, nil, nil, nil,
	)
	k8sClient := fake.NewFakeClient()
	driver := portworx{
		volumePlacementStrategyCRDCreated: true,
	}
	driver.Init(k8sClient, record.NewFakeRecorder(0))

	cluster := &corev1alpha1.StorageCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "px-cluster",
			Namespace: "kube-test",
		},
		Spec: corev1alpha1.StorageClusterSpec{
			UserInterface: &corev1alpha1.UserInterfaceSpec{
				Enabled: true,
			},
		},
	}

	err := driver.PreInstall(cluster)
	require.EqualError(t, err, "lighthouse image cannot be empty")

	cluster.Spec.UserInterface.Image = ""
	err = driver.PreInstall(cluster)
	require.EqualError(t, err, "lighthouse image cannot be empty")
}

func TestLighthouseImageChange(t *testing.T) {
	k8s.Instance().SetClient(
		fakek8sclient.NewSimpleClientset(),
		nil, nil, nil, nil, nil,
	)
	k8sClient := fake.NewFakeClient()
	driver := portworx{
		volumePlacementStrategyCRDCreated: true,
	}
	driver.Init(k8sClient, record.NewFakeRecorder(0))

	cluster := &corev1alpha1.StorageCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "px-cluster",
			Namespace: "kube-test",
		},
		Spec: corev1alpha1.StorageClusterSpec{
			UserInterface: &corev1alpha1.UserInterfaceSpec{
				Enabled: true,
				Image:   "portworx/px-lighthouse:v1",
			},
		},
	}

	err := driver.PreInstall(cluster)
	require.NoError(t, err)

	lhDeployment := &appsv1.Deployment{}
	err = get(k8sClient, lhDeployment, lhDeploymentName, cluster.Namespace)
	require.NoError(t, err)
	image := getLighthouseImage(lhDeployment, lhContainerName)
	require.Equal(t, "portworx/px-lighthouse:v1", image)

	// Change the lighthouse image
	cluster.Spec.UserInterface.Image = "portworx/px-lighthouse:v2"

	err = driver.PreInstall(cluster)
	require.NoError(t, err)

	err = get(k8sClient, lhDeployment, lhDeploymentName, cluster.Namespace)
	require.NoError(t, err)
	image = getLighthouseImage(lhDeployment, lhContainerName)
	require.Equal(t, "portworx/px-lighthouse:v2", image)
}

func TestLighthouseConfigInitImageChange(t *testing.T) {
	k8s.Instance().SetClient(
		fakek8sclient.NewSimpleClientset(),
		nil, nil, nil, nil, nil,
	)
	k8sClient := fake.NewFakeClient()
	driver := portworx{
		volumePlacementStrategyCRDCreated: true,
	}
	driver.Init(k8sClient, record.NewFakeRecorder(0))

	cluster := &corev1alpha1.StorageCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "px-cluster",
			Namespace: "kube-test",
		},
		Spec: corev1alpha1.StorageClusterSpec{
			UserInterface: &corev1alpha1.UserInterfaceSpec{
				Enabled: true,
				Image:   "portworx/px-lighthouse:v1",
			},
		},
	}

	err := driver.PreInstall(cluster)
	require.NoError(t, err)

	lhDeployment := &appsv1.Deployment{}
	err = get(k8sClient, lhDeployment, lhDeploymentName, cluster.Namespace)
	require.NoError(t, err)
	image := getLighthouseImage(lhDeployment, lhConfigInitContainerName)
	require.Equal(t, "portworx/lh-config-sync:v1", image)

	// Change the lighthouse config init container image
	cluster.Spec.UserInterface.Env = []v1.EnvVar{
		{
			Name:  envKeyLhConfigSyncImage,
			Value: "test/config-sync:v2",
		},
	}

	err = driver.PreInstall(cluster)
	require.NoError(t, err)

	err = get(k8sClient, lhDeployment, lhDeploymentName, cluster.Namespace)
	require.NoError(t, err)
	image = getLighthouseImage(lhDeployment, lhConfigInitContainerName)
	require.Equal(t, "test/config-sync:v2", image)
}

func TestLighthouseStorkConnectorImageChange(t *testing.T) {
	k8s.Instance().SetClient(
		fakek8sclient.NewSimpleClientset(),
		nil, nil, nil, nil, nil,
	)
	k8sClient := fake.NewFakeClient()
	driver := portworx{
		volumePlacementStrategyCRDCreated: true,
	}
	driver.Init(k8sClient, record.NewFakeRecorder(0))

	cluster := &corev1alpha1.StorageCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "px-cluster",
			Namespace: "kube-test",
		},
		Spec: corev1alpha1.StorageClusterSpec{
			UserInterface: &corev1alpha1.UserInterfaceSpec{
				Enabled: true,
				Image:   "portworx/px-lighthouse:v1",
			},
		},
	}

	err := driver.PreInstall(cluster)
	require.NoError(t, err)

	lhDeployment := &appsv1.Deployment{}
	err = get(k8sClient, lhDeployment, lhDeploymentName, cluster.Namespace)
	require.NoError(t, err)
	image := getLighthouseImage(lhDeployment, lhStorkConnectorContainerName)
	require.Equal(t, "portworx/lh-stork-connector:v1", image)

	// Change the lighthouse config sync container image
	cluster.Spec.UserInterface.Env = []v1.EnvVar{
		{
			Name:  envKeyLhStorkConnectorImage,
			Value: "test/stork-connector:v2",
		},
	}

	err = driver.PreInstall(cluster)
	require.NoError(t, err)

	err = get(k8sClient, lhDeployment, lhDeploymentName, cluster.Namespace)
	require.NoError(t, err)
	image = getLighthouseImage(lhDeployment, lhStorkConnectorContainerName)
	require.Equal(t, "test/stork-connector:v2", image)
}

func TestLighthouseWithoutImageTag(t *testing.T) {
	k8s.Instance().SetClient(
		fakek8sclient.NewSimpleClientset(),
		nil, nil, nil, nil, nil,
	)
	k8sClient := fake.NewFakeClient()
	driver := portworx{
		volumePlacementStrategyCRDCreated: true,
	}
	driver.Init(k8sClient, record.NewFakeRecorder(0))

	cluster := &corev1alpha1.StorageCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "px-cluster",
			Namespace: "kube-test",
		},
		Spec: corev1alpha1.StorageClusterSpec{
			UserInterface: &corev1alpha1.UserInterfaceSpec{
				Enabled: true,
				Image:   "portworx/px-lighthouse",
			},
		},
	}

	err := driver.PreInstall(cluster)
	require.NoError(t, err)

	// Lighthouse image should remain unchanged but sidecar container images
	// should use the default image tag
	lhDeployment := &appsv1.Deployment{}
	err = get(k8sClient, lhDeployment, lhDeploymentName, cluster.Namespace)
	require.NoError(t, err)
	image := getLighthouseImage(lhDeployment, lhContainerName)
	require.Equal(t, "portworx/px-lighthouse", image)
	image = getLighthouseImage(lhDeployment, lhConfigInitContainerName)
	require.Equal(t, "portworx/lh-config-sync:"+defaultLighthouseImageTag, image)
	image = getLighthouseImage(lhDeployment, lhConfigSyncContainerName)
	require.Equal(t, "portworx/lh-config-sync:"+defaultLighthouseImageTag, image)
	image = getLighthouseImage(lhDeployment, lhStorkConnectorContainerName)
	require.Equal(t, "portworx/lh-stork-connector:"+defaultLighthouseImageTag, image)
}

func TestLighthouseSidecarsOverrideWithEnv(t *testing.T) {
	k8s.Instance().SetClient(
		fakek8sclient.NewSimpleClientset(),
		nil, nil, nil, nil, nil,
	)
	k8sClient := fake.NewFakeClient()
	driver := portworx{
		volumePlacementStrategyCRDCreated: true,
	}
	driver.Init(k8sClient, record.NewFakeRecorder(0))

	cluster := &corev1alpha1.StorageCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "px-cluster",
			Namespace: "kube-test",
		},
		Spec: corev1alpha1.StorageClusterSpec{
			UserInterface: &corev1alpha1.UserInterfaceSpec{
				Enabled: true,
				Image:   "portworx/px-lighthouse",
				Env: []v1.EnvVar{
					{
						Name:  envKeyLhConfigSyncImage,
						Value: "test/config-sync:t1",
					},
					{
						Name:  envKeyLhStorkConnectorImage,
						Value: "test/stork-connector:t2",
					},
				},
			},
		},
	}

	err := driver.PreInstall(cluster)
	require.NoError(t, err)

	// Lighthouse image should remain unchanged but sidecar container images
	// should use the default image tag
	lhDeployment := &appsv1.Deployment{}
	err = get(k8sClient, lhDeployment, lhDeploymentName, cluster.Namespace)
	require.NoError(t, err)
	image := getLighthouseImage(lhDeployment, lhContainerName)
	require.Equal(t, "portworx/px-lighthouse", image)
	image = getLighthouseImage(lhDeployment, lhConfigInitContainerName)
	require.Equal(t, "test/config-sync:t1", image)
	image = getLighthouseImage(lhDeployment, lhConfigSyncContainerName)
	require.Equal(t, "test/config-sync:t1", image)
	image = getLighthouseImage(lhDeployment, lhStorkConnectorContainerName)
	require.Equal(t, "test/stork-connector:t2", image)
}

func TestCompleteInstallWithCustomRepoRegistry(t *testing.T) {
	// Set fake kubernetes client for k8s version
	k8s.Instance().SetClient(
		fakek8sclient.NewSimpleClientset(),
		nil, nil, nil, nil, nil,
	)
	k8sClient := fake.NewFakeClient()
	driver := portworx{
		volumePlacementStrategyCRDCreated: true,
	}
	driver.Init(k8sClient, record.NewFakeRecorder(0))
	customRepo := "test-registry:1111/test-repo"

	cluster := &corev1alpha1.StorageCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "px-cluster",
			Namespace: "kube-test",
			Annotations: map[string]string{
				annotationPVCController: "true",
			},
		},
		Spec: corev1alpha1.StorageClusterSpec{
			CustomImageRegistry: customRepo,
			UserInterface: &corev1alpha1.UserInterfaceSpec{
				Enabled: true,
				Image:   "portworx/px-lighthouse:test",
			},
		},
	}

	err := driver.PreInstall(cluster)

	require.NoError(t, err)

	pxAPIDaemonSet := &appsv1.DaemonSet{}
	err = get(k8sClient, pxAPIDaemonSet, pxAPIDaemonSetName, cluster.Namespace)
	require.NoError(t, err)
	require.Equal(t, customRepo+"/pause:3.1", pxAPIDaemonSet.Spec.Template.Spec.Containers[0].Image)

	pvcDeployment := &appsv1.Deployment{}
	err = get(k8sClient, pvcDeployment, pvcDeploymentName, cluster.Namespace)
	require.NoError(t, err)
	require.Equal(t,
		customRepo+"/kube-controller-manager-amd64:v0.0.0",
		pvcDeployment.Spec.Template.Spec.Containers[0].Image,
	)

	lhDeployment := &appsv1.Deployment{}
	err = get(k8sClient, lhDeployment, lhDeploymentName, cluster.Namespace)
	require.NoError(t, err)
	require.Equal(t,
		customRepo+"/px-lighthouse:test",
		getLighthouseImage(lhDeployment, lhContainerName),
	)
	require.Equal(t,
		customRepo+"/lh-config-sync:test",
		getLighthouseImage(lhDeployment, lhConfigSyncContainerName),
	)
	require.Equal(t,
		customRepo+"/lh-stork-connector:test",
		getLighthouseImage(lhDeployment, lhStorkConnectorContainerName),
	)
	require.Equal(t,
		customRepo+"/lh-config-sync:test",
		getLighthouseImage(lhDeployment, lhConfigInitContainerName),
	)
}

func TestCompleteInstallWithCustomRegistry(t *testing.T) {
	// Set fake kubernetes client for k8s version
	k8s.Instance().SetClient(
		fakek8sclient.NewSimpleClientset(),
		nil, nil, nil, nil, nil,
	)
	k8sClient := fake.NewFakeClient()
	driver := portworx{
		volumePlacementStrategyCRDCreated: true,
	}
	driver.Init(k8sClient, record.NewFakeRecorder(0))
	customRegistry := "test-registry:1111"

	cluster := &corev1alpha1.StorageCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "px-cluster",
			Namespace: "kube-test",
			Annotations: map[string]string{
				annotationPVCController: "true",
			},
		},
		Spec: corev1alpha1.StorageClusterSpec{
			CustomImageRegistry: customRegistry,
			ImagePullPolicy:     v1.PullIfNotPresent,
			UserInterface: &corev1alpha1.UserInterfaceSpec{
				Enabled: true,
				Image:   "portworx/px-lighthouse:test",
			},
		},
	}

	err := driver.PreInstall(cluster)

	require.NoError(t, err)

	pxAPIDaemonSet := &appsv1.DaemonSet{}
	err = get(k8sClient, pxAPIDaemonSet, pxAPIDaemonSetName, cluster.Namespace)
	require.NoError(t, err)
	require.Equal(t,
		customRegistry+"/k8s.gcr.io/pause:3.1",
		pxAPIDaemonSet.Spec.Template.Spec.Containers[0].Image,
	)

	pvcDeployment := &appsv1.Deployment{}
	err = get(k8sClient, pvcDeployment, pvcDeploymentName, cluster.Namespace)
	require.NoError(t, err)
	require.Equal(t,
		customRegistry+"/gcr.io/google_containers/kube-controller-manager-amd64:v0.0.0",
		pvcDeployment.Spec.Template.Spec.Containers[0].Image,
	)

	lhDeployment := &appsv1.Deployment{}
	err = get(k8sClient, lhDeployment, lhDeploymentName, cluster.Namespace)
	require.NoError(t, err)
	require.Equal(t,
		customRegistry+"/portworx/px-lighthouse:test",
		getLighthouseImage(lhDeployment, lhContainerName),
	)
	require.Equal(t,
		customRegistry+"/portworx/lh-config-sync:test",
		getLighthouseImage(lhDeployment, lhConfigSyncContainerName),
	)
	require.Equal(t,
		customRegistry+"/portworx/lh-stork-connector:test",
		getLighthouseImage(lhDeployment, lhStorkConnectorContainerName),
	)
	require.Equal(t,
		customRegistry+"/portworx/lh-config-sync:test",
		getLighthouseImage(lhDeployment, lhConfigInitContainerName),
	)
	require.Equal(t, v1.PullIfNotPresent, getPullPolicyForContainer(lhDeployment, lhContainerName))
	require.Equal(t, v1.PullIfNotPresent, getPullPolicyForContainer(lhDeployment, "config-sync"))
	require.Equal(t, v1.PullIfNotPresent, getPullPolicyForContainer(lhDeployment, "stork-connector"))
	require.Equal(t, v1.PullIfNotPresent, lhDeployment.Spec.Template.Spec.InitContainers[0].ImagePullPolicy)
}

func TestRemovePVCController(t *testing.T) {
	// Set fake kubernetes client for k8s version
	k8s.Instance().SetClient(
		fakek8sclient.NewSimpleClientset(),
		nil, nil, nil, nil, nil,
	)
	k8sClient := fake.NewFakeClient()
	driver := portworx{
		volumePlacementStrategyCRDCreated: true,
	}
	driver.Init(k8sClient, record.NewFakeRecorder(0))

	cluster := &corev1alpha1.StorageCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "px-cluster",
			Namespace: "kube-test",
			Annotations: map[string]string{
				annotationPVCController: "true",
			},
		},
	}

	err := driver.PreInstall(cluster)

	require.NoError(t, err)

	sa := &v1.ServiceAccount{}
	err = get(k8sClient, sa, pvcServiceAccountName, cluster.Namespace)
	require.NoError(t, err)

	cr := &rbacv1.ClusterRole{}
	err = get(k8sClient, cr, pvcClusterRoleName, "")
	require.NoError(t, err)

	crb := &rbacv1.ClusterRoleBinding{}
	err = get(k8sClient, crb, pvcClusterRoleBindingName, "")
	require.NoError(t, err)

	deployment := &appsv1.Deployment{}
	err = get(k8sClient, deployment, pvcDeploymentName, cluster.Namespace)
	require.NoError(t, err)

	// Remove PVC Controller
	delete(cluster.Annotations, annotationPVCController)
	err = driver.PreInstall(cluster)
	require.NoError(t, err)

	// Keep the service account
	sa = &v1.ServiceAccount{}
	err = get(k8sClient, sa, pvcServiceAccountName, cluster.Namespace)
	require.NoError(t, err)

	cr = &rbacv1.ClusterRole{}
	err = get(k8sClient, cr, pvcClusterRoleName, "")
	require.True(t, errors.IsNotFound(err))

	crb = &rbacv1.ClusterRoleBinding{}
	err = get(k8sClient, crb, pvcClusterRoleBindingName, "")
	require.True(t, errors.IsNotFound(err))

	deployment = &appsv1.Deployment{}
	err = get(k8sClient, deployment, pvcDeploymentName, cluster.Namespace)
	require.True(t, errors.IsNotFound(err))
}

func TestDisablePVCController(t *testing.T) {
	// Set fake kubernetes client for k8s version
	k8s.Instance().SetClient(
		fakek8sclient.NewSimpleClientset(),
		nil, nil, nil, nil, nil,
	)
	k8sClient := fake.NewFakeClient()
	driver := portworx{
		volumePlacementStrategyCRDCreated: true,
	}
	driver.Init(k8sClient, record.NewFakeRecorder(0))

	cluster := &corev1alpha1.StorageCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "px-cluster",
			Namespace: "kube-test",
			Annotations: map[string]string{
				annotationPVCController: "true",
			},
		},
	}

	err := driver.PreInstall(cluster)

	require.NoError(t, err)

	sa := &v1.ServiceAccount{}
	err = get(k8sClient, sa, pvcServiceAccountName, cluster.Namespace)
	require.NoError(t, err)

	cr := &rbacv1.ClusterRole{}
	err = get(k8sClient, cr, pvcClusterRoleName, "")
	require.NoError(t, err)

	crb := &rbacv1.ClusterRoleBinding{}
	err = get(k8sClient, crb, pvcClusterRoleBindingName, "")
	require.NoError(t, err)

	deployment := &appsv1.Deployment{}
	err = get(k8sClient, deployment, pvcDeploymentName, cluster.Namespace)
	require.NoError(t, err)

	// Disable PVC Controller
	cluster.Annotations[annotationPVCController] = "false"
	err = driver.PreInstall(cluster)
	require.NoError(t, err)

	// Keep the service account
	sa = &v1.ServiceAccount{}
	err = get(k8sClient, sa, pvcServiceAccountName, cluster.Namespace)
	require.NoError(t, err)

	cr = &rbacv1.ClusterRole{}
	err = get(k8sClient, cr, pvcClusterRoleName, "")
	require.True(t, errors.IsNotFound(err))

	crb = &rbacv1.ClusterRoleBinding{}
	err = get(k8sClient, crb, pvcClusterRoleBindingName, "")
	require.True(t, errors.IsNotFound(err))

	deployment = &appsv1.Deployment{}
	err = get(k8sClient, deployment, pvcDeploymentName, cluster.Namespace)
	require.True(t, errors.IsNotFound(err))
}

func TestRemoveLighthouse(t *testing.T) {
	// Set fake kubernetes client for k8s version
	k8sClient := fake.NewFakeClient()
	driver := portworx{
		volumePlacementStrategyCRDCreated: true,
	}
	driver.Init(k8sClient, record.NewFakeRecorder(0))

	cluster := &corev1alpha1.StorageCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "px-cluster",
			Namespace: "kube-test",
		},
		Spec: corev1alpha1.StorageClusterSpec{
			UserInterface: &corev1alpha1.UserInterfaceSpec{
				Enabled: true,
				Image:   "portworx/px-lighthouse:test",
			},
		},
	}

	err := driver.PreInstall(cluster)

	require.NoError(t, err)

	sa := &v1.ServiceAccount{}
	err = get(k8sClient, sa, lhServiceAccountName, cluster.Namespace)
	require.NoError(t, err)

	cr := &rbacv1.ClusterRole{}
	err = get(k8sClient, cr, lhClusterRoleName, "")
	require.NoError(t, err)

	crb := &rbacv1.ClusterRoleBinding{}
	err = get(k8sClient, crb, lhClusterRoleBindingName, "")
	require.NoError(t, err)

	service := &v1.Service{}
	err = get(k8sClient, service, lhServiceName, cluster.Namespace)
	require.NoError(t, err)

	deployment := &appsv1.Deployment{}
	err = get(k8sClient, deployment, lhDeploymentName, cluster.Namespace)
	require.NoError(t, err)

	// Remove lighthouse config
	cluster.Spec.UserInterface = nil
	err = driver.PreInstall(cluster)
	require.NoError(t, err)

	// Keep the service account
	sa = &v1.ServiceAccount{}
	err = get(k8sClient, sa, lhServiceAccountName, cluster.Namespace)
	require.NoError(t, err)

	cr = &rbacv1.ClusterRole{}
	err = get(k8sClient, cr, lhClusterRoleName, "")
	require.True(t, errors.IsNotFound(err))

	crb = &rbacv1.ClusterRoleBinding{}
	err = get(k8sClient, crb, lhClusterRoleBindingName, "")
	require.True(t, errors.IsNotFound(err))

	service = &v1.Service{}
	err = get(k8sClient, service, lhServiceName, cluster.Namespace)
	require.True(t, errors.IsNotFound(err))

	deployment = &appsv1.Deployment{}
	err = get(k8sClient, deployment, lhDeploymentName, cluster.Namespace)
	require.True(t, errors.IsNotFound(err))
}

func TestDisableLighthouse(t *testing.T) {
	// Set fake kubernetes client for k8s version
	k8sClient := fake.NewFakeClient()
	driver := portworx{
		volumePlacementStrategyCRDCreated: true,
	}
	driver.Init(k8sClient, record.NewFakeRecorder(0))

	cluster := &corev1alpha1.StorageCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "px-cluster",
			Namespace: "kube-test",
		},
		Spec: corev1alpha1.StorageClusterSpec{
			UserInterface: &corev1alpha1.UserInterfaceSpec{
				Enabled: true,
				Image:   "portworx/px-lighthouse:test",
			},
		},
	}

	err := driver.PreInstall(cluster)

	require.NoError(t, err)

	sa := &v1.ServiceAccount{}
	err = get(k8sClient, sa, lhServiceAccountName, cluster.Namespace)
	require.NoError(t, err)

	cr := &rbacv1.ClusterRole{}
	err = get(k8sClient, cr, lhClusterRoleName, "")
	require.NoError(t, err)

	crb := &rbacv1.ClusterRoleBinding{}
	err = get(k8sClient, crb, lhClusterRoleBindingName, "")
	require.NoError(t, err)

	service := &v1.Service{}
	err = get(k8sClient, service, lhServiceName, cluster.Namespace)
	require.NoError(t, err)

	deployment := &appsv1.Deployment{}
	err = get(k8sClient, deployment, lhDeploymentName, cluster.Namespace)
	require.NoError(t, err)

	// Disable lighthouse
	cluster.Spec.UserInterface.Enabled = false
	err = driver.PreInstall(cluster)
	require.NoError(t, err)

	// Keep the service account
	sa = &v1.ServiceAccount{}
	err = get(k8sClient, sa, lhServiceAccountName, cluster.Namespace)
	require.NoError(t, err)

	cr = &rbacv1.ClusterRole{}
	err = get(k8sClient, cr, lhClusterRoleName, "")
	require.True(t, errors.IsNotFound(err))

	crb = &rbacv1.ClusterRoleBinding{}
	err = get(k8sClient, crb, lhClusterRoleBindingName, "")
	require.True(t, errors.IsNotFound(err))

	service = &v1.Service{}
	err = get(k8sClient, service, lhServiceName, cluster.Namespace)
	require.True(t, errors.IsNotFound(err))

	deployment = &appsv1.Deployment{}
	err = get(k8sClient, deployment, lhDeploymentName, cluster.Namespace)
	require.True(t, errors.IsNotFound(err))
}

func TestCSIInstall(t *testing.T) {
	versionClient := fakek8sclient.NewSimpleClientset()
	k8s.Instance().SetClient(versionClient, nil, nil, nil, nil, nil)
	versionClient.Discovery().(*fakediscovery.FakeDiscovery).FakedServerVersion = &version.Info{
		GitVersion: "v1.11.4",
	}
	k8sClient := fake.NewFakeClient()
	driver := portworx{
		volumePlacementStrategyCRDCreated: true,
	}
	driver.Init(k8sClient, record.NewFakeRecorder(0))

	cluster := &corev1alpha1.StorageCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "px-cluster",
			Namespace: "kube-test",
		},
		Spec: corev1alpha1.StorageClusterSpec{
			FeatureGates: map[string]string{
				string(FeatureCSI): "true",
			},
		},
	}

	err := driver.PreInstall(cluster)
	require.NoError(t, err)

	// CSI ServiceAccount
	serviceAccountList := &v1.ServiceAccountList{}
	err = list(k8sClient, serviceAccountList)
	require.NoError(t, err)
	require.Len(t, serviceAccountList.Items, 2)

	sa := &v1.ServiceAccount{}
	err = get(k8sClient, sa, csiServiceAccountName, cluster.Namespace)
	require.NoError(t, err)
	require.Equal(t, csiServiceAccountName, sa.Name)
	require.Equal(t, cluster.Namespace, sa.Namespace)
	require.Len(t, sa.OwnerReferences, 1)
	require.Equal(t, cluster.Name, sa.OwnerReferences[0].Name)

	// CSI ClusterRole
	clusterRoleList := &rbacv1.ClusterRoleList{}
	err = list(k8sClient, clusterRoleList)
	require.NoError(t, err)
	require.Len(t, clusterRoleList.Items, 2)

	expectedCR := getExpectedClusterRole(t, "csiClusterRole_k8s_1.11.yaml")
	actualCR := &rbacv1.ClusterRole{}
	err = get(k8sClient, actualCR, csiClusterRoleName, "")
	require.NoError(t, err)
	require.Equal(t, expectedCR.Name, actualCR.Name)
	require.Len(t, actualCR.OwnerReferences, 1)
	require.Equal(t, cluster.Name, actualCR.OwnerReferences[0].Name)
	require.ElementsMatch(t, expectedCR.Rules, actualCR.Rules)

	// CSI ClusterRoleBinding
	crbList := &rbacv1.ClusterRoleBindingList{}
	err = list(k8sClient, crbList)
	require.NoError(t, err)
	require.Len(t, crbList.Items, 2)

	expectedCRB := getExpectedClusterRoleBinding(t, "csiClusterRoleBinding.yaml")
	actualCRB := &rbacv1.ClusterRoleBinding{}
	err = get(k8sClient, actualCRB, csiClusterRoleBindingName, "")
	require.NoError(t, err)
	require.Equal(t, expectedCRB.Name, actualCRB.Name)
	require.Len(t, actualCRB.OwnerReferences, 1)
	require.Equal(t, cluster.Name, actualCRB.OwnerReferences[0].Name)
	require.ElementsMatch(t, expectedCRB.Subjects, actualCRB.Subjects)
	require.Equal(t, expectedCRB.RoleRef, actualCRB.RoleRef)

	// CSI Service
	serviceList := &v1.ServiceList{}
	err = list(k8sClient, serviceList)
	require.NoError(t, err)
	require.Len(t, serviceList.Items, 3)

	expectedService := getExpectedService(t, "csiService.yaml")
	service := &v1.Service{}
	err = get(k8sClient, service, csiServiceName, cluster.Namespace)
	require.NoError(t, err)
	require.Equal(t, expectedService.Name, service.Name)
	require.Equal(t, expectedService.Namespace, service.Namespace)
	require.Len(t, service.OwnerReferences, 1)
	require.Equal(t, cluster.Name, service.OwnerReferences[0].Name)
	require.Equal(t, expectedService.Labels, service.Labels)
	require.Equal(t, expectedService.Spec, service.Spec)

	// CSI StatefulSet
	statefulSetList := &appsv1.StatefulSetList{}
	err = list(k8sClient, statefulSetList)
	require.NoError(t, err)
	require.Len(t, statefulSetList.Items, 1)

	expectedStatefulSet := getExpectedStatefulSet(t, "csiStatefulSet_0.3.yaml")
	statefulSet := &appsv1.StatefulSet{}
	err = get(k8sClient, statefulSet, csiStatefulSetName, cluster.Namespace)
	require.NoError(t, err)
	require.Equal(t, expectedStatefulSet.Name, statefulSet.Name)
	require.Equal(t, expectedStatefulSet.Namespace, statefulSet.Namespace)
	require.Len(t, statefulSet.OwnerReferences, 1)
	require.Equal(t, cluster.Name, statefulSet.OwnerReferences[0].Name)
	require.Equal(t, expectedStatefulSet.Spec, statefulSet.Spec)
}

func TestCSIInstallWithNewerCSIVersion(t *testing.T) {
	versionClient := fakek8sclient.NewSimpleClientset()
	k8s.Instance().SetClient(versionClient, nil, nil, nil, nil, nil)
	versionClient.Discovery().(*fakediscovery.FakeDiscovery).FakedServerVersion = &version.Info{
		GitVersion: "v1.13.0",
	}
	k8sClient := fake.NewFakeClient()
	driver := portworx{
		volumePlacementStrategyCRDCreated: true,
	}
	driver.Init(k8sClient, record.NewFakeRecorder(0))

	cluster := &corev1alpha1.StorageCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "px-cluster",
			Namespace: "kube-test",
		},
		Spec: corev1alpha1.StorageClusterSpec{
			FeatureGates: map[string]string{
				string(FeatureCSI): "true",
			},
		},
	}

	err := driver.PreInstall(cluster)
	require.NoError(t, err)

	expectedCR := getExpectedClusterRole(t, "csiClusterRole_k8s_1.13.yaml")
	actualCR := &rbacv1.ClusterRole{}
	err = get(k8sClient, actualCR, csiClusterRoleName, "")
	require.NoError(t, err)
	require.Equal(t, expectedCR.Name, actualCR.Name)
	require.Len(t, actualCR.OwnerReferences, 1)
	require.Equal(t, cluster.Name, actualCR.OwnerReferences[0].Name)
	require.ElementsMatch(t, expectedCR.Rules, actualCR.Rules)

	expectedStatefulSet := getExpectedStatefulSet(t, "csiStatefulSet_1.0.yaml")
	statefulSet := &appsv1.StatefulSet{}
	err = get(k8sClient, statefulSet, csiStatefulSetName, cluster.Namespace)
	require.NoError(t, err)
	require.Equal(t, expectedStatefulSet.Name, statefulSet.Name)
	require.Equal(t, expectedStatefulSet.Namespace, statefulSet.Namespace)
	require.Len(t, statefulSet.OwnerReferences, 1)
	require.Equal(t, cluster.Name, statefulSet.OwnerReferences[0].Name)
	require.Equal(t, expectedStatefulSet.Spec, statefulSet.Spec)
}

func TestCSIClusterRoleK8sVersionGreaterThan_1_14(t *testing.T) {
	versionClient := fakek8sclient.NewSimpleClientset()
	k8s.Instance().SetClient(versionClient, nil, nil, nil, nil, nil)
	versionClient.Discovery().(*fakediscovery.FakeDiscovery).FakedServerVersion = &version.Info{
		GitVersion: "v1.14.0",
	}
	k8sClient := fake.NewFakeClient()
	driver := portworx{
		volumePlacementStrategyCRDCreated: true,
	}
	driver.Init(k8sClient, record.NewFakeRecorder(0))

	cluster := &corev1alpha1.StorageCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "px-cluster",
			Namespace: "kube-test",
		},
		Spec: corev1alpha1.StorageClusterSpec{
			FeatureGates: map[string]string{
				string(FeatureCSI): "true",
			},
		},
	}

	err := driver.PreInstall(cluster)
	require.NoError(t, err)

	expectedCR := getExpectedClusterRole(t, "csiClusterRole_k8s_1.14.yaml")
	actualCR := &rbacv1.ClusterRole{}
	err = get(k8sClient, actualCR, csiClusterRoleName, "")
	require.NoError(t, err)
	require.Equal(t, expectedCR.Name, actualCR.Name)
	require.Len(t, actualCR.OwnerReferences, 1)
	require.Equal(t, cluster.Name, actualCR.OwnerReferences[0].Name)
	require.ElementsMatch(t, expectedCR.Rules, actualCR.Rules)
}

func TestCSIChangeImageVersions(t *testing.T) {
	versionClient := fakek8sclient.NewSimpleClientset()
	k8s.Instance().SetClient(versionClient, nil, nil, nil, nil, nil)
	versionClient.Discovery().(*fakediscovery.FakeDiscovery).FakedServerVersion = &version.Info{
		GitVersion: "v1.13.0",
	}
	k8sClient := fake.NewFakeClient()
	driver := portworx{
		volumePlacementStrategyCRDCreated: true,
	}
	driver.Init(k8sClient, record.NewFakeRecorder(0))

	cluster := &corev1alpha1.StorageCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "px-cluster",
			Namespace: "kube-test",
		},
		Spec: corev1alpha1.StorageClusterSpec{
			FeatureGates: map[string]string{
				string(FeatureCSI): "true",
			},
		},
	}

	err := driver.PreInstall(cluster)
	require.NoError(t, err)

	statefulSet := &appsv1.StatefulSet{}
	err = get(k8sClient, statefulSet, csiStatefulSetName, cluster.Namespace)
	require.NoError(t, err)
	require.Len(t, statefulSet.Spec.Template.Spec.Containers, 3)
	require.Equal(t, "quay.io/k8scsi/csi-provisioner:v1.1.0",
		statefulSet.Spec.Template.Spec.Containers[0].Image)
	require.Equal(t, "quay.io/k8scsi/csi-attacher:v1.1.1",
		statefulSet.Spec.Template.Spec.Containers[1].Image)
	require.Equal(t, "quay.io/k8scsi/csi-cluster-driver-registrar:v1.0.1",
		statefulSet.Spec.Template.Spec.Containers[2].Image)

	// Change provisioner image
	statefulSet.Spec.Template.Spec.Containers[0].Image = "my-csi-provisioner:test"
	err = k8sClient.Update(context.TODO(), statefulSet)
	require.NoError(t, err)

	err = driver.PreInstall(cluster)
	require.NoError(t, err)

	err = get(k8sClient, statefulSet, csiStatefulSetName, cluster.Namespace)
	require.NoError(t, err)
	require.Equal(t, "quay.io/k8scsi/csi-provisioner:v1.1.0",
		statefulSet.Spec.Template.Spec.Containers[0].Image)

	// Change attacher image
	statefulSet.Spec.Template.Spec.Containers[0].Image = "my-csi-attacher:test"
	err = k8sClient.Update(context.TODO(), statefulSet)
	require.NoError(t, err)

	err = driver.PreInstall(cluster)
	require.NoError(t, err)

	err = get(k8sClient, statefulSet, csiStatefulSetName, cluster.Namespace)
	require.NoError(t, err)
	require.Equal(t, "quay.io/k8scsi/csi-attacher:v1.1.1",
		statefulSet.Spec.Template.Spec.Containers[1].Image)

	// Change cluster registrar image
	statefulSet.Spec.Template.Spec.Containers[0].Image = "my-csi-cluster-registrar:test"
	err = k8sClient.Update(context.TODO(), statefulSet)
	require.NoError(t, err)

	err = driver.PreInstall(cluster)
	require.NoError(t, err)

	err = get(k8sClient, statefulSet, csiStatefulSetName, cluster.Namespace)
	require.NoError(t, err)
	require.Equal(t, "quay.io/k8scsi/csi-cluster-driver-registrar:v1.0.1",
		statefulSet.Spec.Template.Spec.Containers[2].Image)
}

func TestCSIChangeKubernetesVersions(t *testing.T) {
	versionClient := fakek8sclient.NewSimpleClientset()
	k8s.Instance().SetClient(versionClient, nil, nil, nil, nil, nil)
	versionClient.Discovery().(*fakediscovery.FakeDiscovery).FakedServerVersion = &version.Info{
		GitVersion: "v1.13.0",
	}
	k8sClient := fake.NewFakeClient()
	driver := portworx{
		volumePlacementStrategyCRDCreated: true,
	}
	driver.Init(k8sClient, record.NewFakeRecorder(0))

	cluster := &corev1alpha1.StorageCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "px-cluster",
			Namespace: "kube-test",
		},
		Spec: corev1alpha1.StorageClusterSpec{
			FeatureGates: map[string]string{
				string(FeatureCSI): "true",
			},
		},
	}

	err := driver.PreInstall(cluster)
	require.NoError(t, err)

	statefulSet := &appsv1.StatefulSet{}
	err = get(k8sClient, statefulSet, csiStatefulSetName, cluster.Namespace)
	require.NoError(t, err)
	require.Len(t, statefulSet.Spec.Template.Spec.Containers, 3)
	require.Equal(t, "quay.io/k8scsi/csi-provisioner:v1.1.0",
		statefulSet.Spec.Template.Spec.Containers[0].Image)
	require.Equal(t, "quay.io/k8scsi/csi-attacher:v1.1.1",
		statefulSet.Spec.Template.Spec.Containers[1].Image)
	require.Equal(t, "quay.io/k8scsi/csi-cluster-driver-registrar:v1.0.1",
		statefulSet.Spec.Template.Spec.Containers[2].Image)

	// Use older kubernetes version. Cluster registrar sidecar should not run.
	versionClient.Discovery().(*fakediscovery.FakeDiscovery).FakedServerVersion = &version.Info{
		GitVersion: "v1.12.0",
	}

	err = driver.PreInstall(cluster)
	require.NoError(t, err)

	err = get(k8sClient, statefulSet, csiStatefulSetName, cluster.Namespace)
	require.NoError(t, err)
	require.Len(t, statefulSet.Spec.Template.Spec.Containers, 2)
	require.Equal(t, "quay.io/k8scsi/csi-provisioner:v0.4.2",
		statefulSet.Spec.Template.Spec.Containers[0].Image)
	require.Equal(t, "quay.io/k8scsi/csi-attacher:v0.4.2",
		statefulSet.Spec.Template.Spec.Containers[1].Image)
}

func TestCSIInstallWithCustomRegistry(t *testing.T) {
	versionClient := fakek8sclient.NewSimpleClientset()
	k8s.Instance().SetClient(versionClient, nil, nil, nil, nil, nil)
	versionClient.Discovery().(*fakediscovery.FakeDiscovery).FakedServerVersion = &version.Info{
		GitVersion: "v1.13.0",
	}
	k8sClient := fake.NewFakeClient()
	driver := portworx{
		volumePlacementStrategyCRDCreated: true,
	}
	driver.Init(k8sClient, record.NewFakeRecorder(0))
	customRegistry := "test-registry:1111"

	cluster := &corev1alpha1.StorageCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "px-cluster",
			Namespace: "kube-test",
		},
		Spec: corev1alpha1.StorageClusterSpec{
			CustomImageRegistry: customRegistry,
			ImagePullPolicy:     v1.PullIfNotPresent,
			FeatureGates: map[string]string{
				string(FeatureCSI): "1",
			},
		},
	}

	err := driver.PreInstall(cluster)
	require.NoError(t, err)

	statefulSet := &appsv1.StatefulSet{}
	err = get(k8sClient, statefulSet, csiStatefulSetName, cluster.Namespace)
	require.NoError(t, err)
	require.Len(t, statefulSet.Spec.Template.Spec.Containers, 3)
	require.Equal(t,
		customRegistry+"/quay.io/k8scsi/csi-provisioner:v1.1.0",
		statefulSet.Spec.Template.Spec.Containers[0].Image,
	)
	require.Equal(t,
		customRegistry+"/quay.io/k8scsi/csi-attacher:v1.1.1",
		statefulSet.Spec.Template.Spec.Containers[1].Image,
	)
	require.Equal(t,
		customRegistry+"/quay.io/k8scsi/csi-cluster-driver-registrar:v1.0.1",
		statefulSet.Spec.Template.Spec.Containers[2].Image,
	)
}

func TestCSIInstallWithCustomRepoRegistry(t *testing.T) {
	versionClient := fakek8sclient.NewSimpleClientset()
	k8s.Instance().SetClient(versionClient, nil, nil, nil, nil, nil)
	versionClient.Discovery().(*fakediscovery.FakeDiscovery).FakedServerVersion = &version.Info{
		GitVersion: "v1.13.0",
	}
	k8sClient := fake.NewFakeClient()
	driver := portworx{
		volumePlacementStrategyCRDCreated: true,
	}
	driver.Init(k8sClient, record.NewFakeRecorder(0))
	customRepo := "test-registry:1111/test-repo"

	cluster := &corev1alpha1.StorageCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "px-cluster",
			Namespace: "kube-test",
		},
		Spec: corev1alpha1.StorageClusterSpec{
			CustomImageRegistry: customRepo,
			ImagePullPolicy:     v1.PullIfNotPresent,
			FeatureGates: map[string]string{
				string(FeatureCSI): "1",
			},
		},
	}

	err := driver.PreInstall(cluster)
	require.NoError(t, err)

	statefulSet := &appsv1.StatefulSet{}
	err = get(k8sClient, statefulSet, csiStatefulSetName, cluster.Namespace)
	require.NoError(t, err)
	require.Len(t, statefulSet.Spec.Template.Spec.Containers, 3)
	require.Equal(t,
		customRepo+"/csi-provisioner:v1.1.0",
		statefulSet.Spec.Template.Spec.Containers[0].Image,
	)
	require.Equal(t,
		customRepo+"/csi-attacher:v1.1.1",
		statefulSet.Spec.Template.Spec.Containers[1].Image,
	)
	require.Equal(t,
		customRepo+"/csi-cluster-driver-registrar:v1.0.1",
		statefulSet.Spec.Template.Spec.Containers[2].Image,
	)
}

func TestDisableCSI(t *testing.T) {
	versionClient := fakek8sclient.NewSimpleClientset()
	k8s.Instance().SetClient(versionClient, nil, nil, nil, nil, nil)
	versionClient.Discovery().(*fakediscovery.FakeDiscovery).FakedServerVersion = &version.Info{
		GitVersion: "v1.11.4",
	}
	k8sClient := fake.NewFakeClient()
	driver := portworx{
		volumePlacementStrategyCRDCreated: true,
	}
	driver.Init(k8sClient, record.NewFakeRecorder(0))

	cluster := &corev1alpha1.StorageCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "px-cluster",
			Namespace: "kube-test",
		},
		Spec: corev1alpha1.StorageClusterSpec{
			FeatureGates: map[string]string{
				string(FeatureCSI): "true",
			},
		},
	}

	err := driver.PreInstall(cluster)
	require.NoError(t, err)

	sa := &v1.ServiceAccount{}
	err = get(k8sClient, sa, csiServiceAccountName, cluster.Namespace)
	require.NoError(t, err)

	cr := &rbacv1.ClusterRole{}
	err = get(k8sClient, cr, csiClusterRoleName, "")
	require.NoError(t, err)

	crb := &rbacv1.ClusterRoleBinding{}
	err = get(k8sClient, crb, csiClusterRoleBindingName, "")
	require.NoError(t, err)

	service := &v1.Service{}
	err = get(k8sClient, service, csiServiceName, cluster.Namespace)
	require.NoError(t, err)

	statefulSet := &appsv1.StatefulSet{}
	err = get(k8sClient, statefulSet, csiStatefulSetName, cluster.Namespace)
	require.NoError(t, err)

	// Disable CSI
	cluster.Spec.FeatureGates[string(FeatureCSI)] = "false"
	err = driver.PreInstall(cluster)
	require.NoError(t, err)

	// Keep the service account
	sa = &v1.ServiceAccount{}
	err = get(k8sClient, sa, csiServiceAccountName, cluster.Namespace)
	require.NoError(t, err)

	cr = &rbacv1.ClusterRole{}
	err = get(k8sClient, cr, csiClusterRoleName, "")
	require.True(t, errors.IsNotFound(err))

	crb = &rbacv1.ClusterRoleBinding{}
	err = get(k8sClient, crb, csiClusterRoleBindingName, "")
	require.True(t, errors.IsNotFound(err))

	service = &v1.Service{}
	err = get(k8sClient, service, csiServiceName, cluster.Namespace)
	require.True(t, errors.IsNotFound(err))

	statefulSet = &appsv1.StatefulSet{}
	err = get(k8sClient, statefulSet, csiStatefulSetName, cluster.Namespace)
	require.True(t, errors.IsNotFound(err))

	// Remove CSI flag. Default should be disabled.
	delete(cluster.Spec.FeatureGates, string(FeatureCSI))
	err = driver.PreInstall(cluster)
	require.NoError(t, err)

	// Keep the service account
	sa = &v1.ServiceAccount{}
	err = get(k8sClient, sa, csiServiceAccountName, cluster.Namespace)
	require.NoError(t, err)

	cr = &rbacv1.ClusterRole{}
	err = get(k8sClient, cr, csiClusterRoleName, "")
	require.True(t, errors.IsNotFound(err))

	crb = &rbacv1.ClusterRoleBinding{}
	err = get(k8sClient, crb, csiClusterRoleBindingName, "")
	require.True(t, errors.IsNotFound(err))

	service = &v1.Service{}
	err = get(k8sClient, service, csiServiceName, cluster.Namespace)
	require.True(t, errors.IsNotFound(err))

	statefulSet = &appsv1.StatefulSet{}
	err = get(k8sClient, statefulSet, csiStatefulSetName, cluster.Namespace)
	require.True(t, errors.IsNotFound(err))
}
