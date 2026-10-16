package controller

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workloadsv1alpha1 "example.com/custom-controller-deployment/api/v1alpha1"
)

var _ = Describe("CustomDeployment", func() {
	var workload *workloadsv1alpha1.CustomDeployment

	BeforeEach(func() {
		workload = &workloadsv1alpha1.CustomDeployment{
			ObjectMeta: metav1.ObjectMeta{GenerateName: "workload-", Namespace: testNamespace},
			Spec: workloadsv1alpha1.CustomDeploymentSpec{
				Image: "nginx:1.28.0", Replicas: 2,
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m"), corev1.ResourceMemory: resource.MustParse("128Mi")},
					Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m"), corev1.ResourceMemory: resource.MustParse("256Mi")},
				},
			},
		}
	})

	AfterEach(func() {
		if workload.Name != "" {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, workload))).To(Succeed())
			// envtest has no garbage collector; delete the fixture Deployment explicitly.
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: workload.Name, Namespace: workload.Namespace},
			}))).To(Succeed())
		}
	})

	It("creates one same-named Deployment with the configured container and controller owner", func() {
		Expect(k8sClient.Create(ctx, workload)).To(Succeed())
		deployment := &appsv1.Deployment{}
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKeyFromObject(workload), deployment)
		}, 10*time.Second, 50*time.Millisecond).Should(Succeed())

		Expect(*deployment.Spec.Replicas).To(Equal(int32(2)))
		Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))
		container := deployment.Spec.Template.Spec.Containers[0]
		Expect(container.Image).To(Equal("nginx:1.28.0"))
		Expect(container.Resources).To(Equal(workload.Spec.Resources))
		Expect(metav1.GetControllerOf(deployment)).To(Equal(&metav1.OwnerReference{
			APIVersion: "workloads.example.com/v1alpha1", Kind: "CustomDeployment",
			Name: workload.Name, UID: workload.UID, Controller: new(true), BlockOwnerDeletion: new(true),
		}))
		deployments := &appsv1.DeploymentList{}
		Expect(k8sClient.List(ctx, deployments, client.InNamespace(testNamespace))).To(Succeed())
		Expect(deployments.Items).To(HaveLen(1))

		By("preserving a later replica count instead of resetting initial replicas")
		deployment.Spec.Replicas = new(int32(4))
		Expect(k8sClient.Update(ctx, deployment)).To(Succeed())
		reconciler := &CustomDeploymentReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
		_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(workload)})
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workload), deployment)).To(Succeed())
		Expect(*deployment.Spec.Replicas).To(Equal(int32(4)))
	})

	It("reports an ownership conflict and preserves an unrelated same-name Deployment", func() {
		const unrelatedName = "unrelated"
		workload.Name = unrelatedName
		workload.GenerateName = ""
		unrelated := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: workload.Name, Namespace: testNamespace},
			Spec: appsv1.DeploymentSpec{
				Replicas: new(int32(3)),
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": unrelatedName}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": unrelatedName}},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "existing", Image: "nginx:1.27.5"}}},
				},
			},
		}
		Expect(k8sClient.Create(ctx, unrelated)).To(Succeed())
		before := unrelated.DeepCopy()
		Expect(k8sClient.Create(ctx, workload)).To(Succeed())
		reconciler := &CustomDeploymentReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
		_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(workload)})
		Expect(err).To(MatchError(ContainSubstring("not controlled by CustomDeployment")))
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(unrelated), unrelated)).To(Succeed())
		Expect(unrelated).To(Equal(before))
	})

	DescribeTable("rejects invalid creation settings", func(image string, replicas int32) {
		workload.Name, workload.GenerateName = "invalid-settings", ""
		workload.Spec.Image, workload.Spec.Replicas = image, replicas
		Expect(k8sClient.Create(ctx, workload)).NotTo(Succeed())
	},
		Entry("empty image", "", int32(1)),
		Entry("zero initial replicas", "nginx:1.28.0", int32(0)),
		Entry("negative initial replicas", "nginx:1.28.0", int32(-1)),
	)

	It("defaults omitted initial replicas to one", func() {
		workload.Name, workload.GenerateName = "default-replicas", ""
		object, err := runtime.DefaultUnstructuredConverter.ToUnstructured(workload)
		Expect(err).NotTo(HaveOccurred())
		resource := &unstructured.Unstructured{Object: object}
		resource.SetAPIVersion("workloads.example.com/v1alpha1")
		resource.SetKind("CustomDeployment")
		unstructured.RemoveNestedField(resource.Object, "spec", "replicas")
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workload), workload)).To(Succeed())
		Expect(workload.Spec.Replicas).To(Equal(int32(1)))
	})
})
