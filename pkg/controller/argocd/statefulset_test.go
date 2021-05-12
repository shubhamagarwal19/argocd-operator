package argocd

import (
	"context"
	"fmt"
	argoprojv1alpha1 "github.com/argoproj-labs/argocd-operator/pkg/apis/argoproj/v1alpha1"
	resourcev1 "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"

	"github.com/argoproj-labs/argocd-operator/pkg/common"
	"github.com/google/go-cmp/cmp"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"gotest.tools/assert"

	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	testRedisImage        = "redis"
	testRedisImageVersion = "test"
)

func controllerDefaultVolumes() []corev1.Volume {
	volumes := []corev1.Volume{
		{
			Name: "argocd-repo-server-tls",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: common.ArgoCDRepoServerTLSSecretName,
					Optional:   boolPtr(true),
				},
			},
		},
	}
	return volumes
}

func controllerDefaultVolumeMounts() []corev1.VolumeMount {
	mounts := []corev1.VolumeMount{
		{
			Name:      "argocd-repo-server-tls",
			MountPath: "/app/config/controller/tls",
		},
	}
	return mounts
}

func TestReconcileArgoCD_reconcileRedisStatefulSet_HA_disabled(t *testing.T) {
	logf.SetLogger(logf.ZapLogger(true))

	a := makeTestArgoCD()
	r := makeTestReconciler(t, a)
	s := newStatefulSetWithSuffix("redis-ha-server", "redis", a)

	assert.NilError(t, r.reconcileRedisStatefulSet(a))
	// resource Creation should fail as HA was disabled
	assert.ErrorContains(t, r.client.Get(context.TODO(), types.NamespacedName{Name: s.Name, Namespace: a.Namespace}, s), "not found")
}

func TestReconcileArgoCD_reconcileRedisStatefulSet_HA_enabled(t *testing.T) {
	logf.SetLogger(logf.ZapLogger(true))

	a := makeTestArgoCD()
	r := makeTestReconciler(t, a)
	s := newStatefulSetWithSuffix("redis-ha-server", "redis", a)

	a.Spec.HA.Enabled = true
	// test resource is Created when HA is enabled
	assert.NilError(t, r.reconcileRedisStatefulSet(a))
	assert.NilError(t, r.client.Get(context.TODO(), types.NamespacedName{Name: s.Name, Namespace: a.Namespace}, s))

	// test resource is Updated on reconciliation
	a.Spec.Redis.Image = testRedisImage
	a.Spec.Redis.Version = testRedisImageVersion
	assert.NilError(t, r.reconcileRedisStatefulSet(a))
	assert.NilError(t, r.client.Get(context.TODO(), types.NamespacedName{Name: s.Name, Namespace: a.Namespace}, s))
	assert.Equal(t, s.Spec.Template.Spec.Containers[0].Image, fmt.Sprintf("%s:%s", testRedisImage, testRedisImageVersion))

	// test resource is Deleted, when HA is disabled
	a.Spec.HA.Enabled = false
	assert.NilError(t, r.reconcileRedisStatefulSet(a))
	assert.ErrorContains(t, r.client.Get(context.TODO(), types.NamespacedName{Name: s.Name, Namespace: a.Namespace}, s), "not found")
}

func TestReconcileArgoCD_reconcileApplicationController(t *testing.T) {
	logf.SetLogger(logf.ZapLogger(true))
	a := makeTestArgoCD()
	r := makeTestReconciler(t, a)

	assert.NilError(t, r.reconcileApplicationControllerStatefulSet(a))

	ss := &appsv1.StatefulSet{}
	assert.NilError(t, r.client.Get(
		context.TODO(),
		types.NamespacedName{
			Name:      "argocd-application-controller",
			Namespace: a.Namespace,
		},
		ss))
	command := ss.Spec.Template.Spec.Containers[0].Command
	want := []string{
		"argocd-application-controller",
		"--operation-processors", "10",
		"--redis", "argocd-redis.argocd.svc.cluster.local:6379",
		"--repo-server", "argocd-repo-server.argocd.svc.cluster.local:8081",
		"--status-processors", "20"}
	if diff := cmp.Diff(want, command); diff != "" {
		t.Fatalf("reconciliation failed:\n%s", diff)
	}
	wantVolumes := controllerDefaultVolumes()
	if diff := cmp.Diff(wantVolumes, ss.Spec.Template.Spec.Volumes); diff != "" {
		t.Fatalf("reconciliation failed:\n%s", diff)
	}
	wantVolumeMounts := controllerDefaultVolumeMounts()
	if diff := cmp.Diff(wantVolumeMounts, ss.Spec.Template.Spec.Containers[0].VolumeMounts); diff != "" {
		t.Fatalf("reconciliation failed:\n%s", diff)
	}
}

func TestReconcileArgoCD_reconcileApplicationController_withUpdate(t *testing.T) {
	logf.SetLogger(logf.ZapLogger(true))
	a := makeTestArgoCD()
	r := makeTestReconciler(t, a)

	assert.NilError(t, r.reconcileApplicationControllerStatefulSet(a))

	a = makeTestArgoCD(controllerProcessors(30))
	assert.NilError(t, r.reconcileApplicationControllerStatefulSet(a))

	ss := &appsv1.StatefulSet{}
	assert.NilError(t, r.client.Get(
		context.TODO(),
		types.NamespacedName{
			Name:      "argocd-application-controller",
			Namespace: a.Namespace,
		},
		ss))
	command := ss.Spec.Template.Spec.Containers[0].Command
	want := []string{
		"argocd-application-controller",
		"--operation-processors", "10",
		"--redis", "argocd-redis.argocd.svc.cluster.local:6379",
		"--repo-server", "argocd-repo-server.argocd.svc.cluster.local:8081",
		"--status-processors", "30"}
	if diff := cmp.Diff(want, command); diff != "" {
		t.Fatalf("reconciliation failed:\n%s", diff)
	}
}

func TestReconcileArgoCD_reconcileApplicationController_withUpgrade(t *testing.T) {
	logf.SetLogger(logf.ZapLogger(true))
	a := makeTestArgoCD()
	r := makeTestReconciler(t, a)

	deploy := newDeploymentWithSuffix("application-controller", "application-controller", a)
	assert.NilError(t, r.client.Create(context.TODO(), deploy))

	assert.NilError(t, r.reconcileApplicationControllerStatefulSet(a))
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: deploy.Name, Namespace: deploy.Namespace}, deploy)
	assert.ErrorContains(t, err, "not found")
}

func TestReconcileArgoCD_reconcileApplicationController_withResources(t *testing.T) {
	logf.SetLogger(logf.ZapLogger(true))
	a := makeTestArgoCDWithResources(func(a *argoprojv1alpha1.ArgoCD) {
		a.Spec.Import = &argoprojv1alpha1.ArgoCDImportSpec{
			Name: "testimport",
		}
	})
	ex := argoprojv1alpha1.ArgoCDExport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testimport",
			Namespace: a.Namespace,
		},
		Spec: argoprojv1alpha1.ArgoCDExportSpec{
			Storage: &argoprojv1alpha1.ArgoCDExportStorageSpec{},
		},
	}
	r := makeTestReconciler(t, a, &ex)

	assert.NilError(t, r.reconcileApplicationControllerStatefulSet(a))

	ss := &appsv1.StatefulSet{}
	assert.NilError(t, r.client.Get(
		context.TODO(),
		types.NamespacedName{
			Name:      "argocd-application-controller",
			Namespace: a.Namespace,
		},
		ss))

	testResources := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceMemory: resourcev1.MustParse("1024Mi"),
			corev1.ResourceCPU:    resourcev1.MustParse("1000m"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceMemory: resourcev1.MustParse("2048Mi"),
			corev1.ResourceCPU:    resourcev1.MustParse("2000m"),
		},
	}
	assert.DeepEqual(t, ss.Spec.Template.Spec.Containers[0].Resources, testResources)
	assert.DeepEqual(t, ss.Spec.Template.Spec.InitContainers[0].Resources, testResources)
}
