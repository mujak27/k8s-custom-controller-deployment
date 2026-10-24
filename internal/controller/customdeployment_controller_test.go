package controller

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
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
		observed := &workloadsv1alpha1.CustomDeployment{}
		Eventually(func() *metav1.Condition {
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(workload), observed); err != nil {
				return nil
			}
			return apimeta.FindStatusCondition(observed.Status.Conditions, "Ready")
		}, 10*time.Second, 50*time.Millisecond).ShouldNot(BeNil())
		condition := apimeta.FindStatusCondition(observed.Status.Conditions, "Ready")
		Expect(condition.Status).To(Equal(metav1.ConditionFalse))
		Expect(condition.Reason).To(Equal("OwnershipConflict"))
		Expect(observed.Status.ObservedGeneration).To(Equal(observed.Generation))
		Expect(observed.Status.DesiredReplicas).To(BeZero())
		Expect(observed.Status.ReadyReplicas).To(BeZero())
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

	DescribeTable("requires positive CPU and memory requests", func(requests corev1.ResourceList) {
		workload.Spec.Resources.Requests = requests
		Expect(k8sClient.Create(ctx, workload)).NotTo(Succeed())
	},
		Entry("missing requests", nil),
		Entry("missing CPU request", corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("128Mi"),
		}),
		Entry("missing memory request", corev1.ResourceList{
			corev1.ResourceCPU: resource.MustParse("100m"),
		}),
		Entry("zero CPU request", corev1.ResourceList{
			corev1.ResourceCPU: resource.MustParse("0"), corev1.ResourceMemory: resource.MustParse("128Mi"),
		}),
		Entry("negative memory request", corev1.ResourceList{
			corev1.ResourceCPU: resource.MustParse("100m"), corev1.ResourceMemory: resource.MustParse("-1Mi"),
		}),
	)

	It("rejects malformed resource quantities through schema validation", func() {
		workload.Name, workload.GenerateName = "malformed-quantity", ""
		object, err := runtime.DefaultUnstructuredConverter.ToUnstructured(workload)
		Expect(err).NotTo(HaveOccurred())
		resource := &unstructured.Unstructured{Object: object}
		resource.SetAPIVersion("workloads.example.com/v1alpha1")
		resource.SetKind("CustomDeployment")
		Expect(unstructured.SetNestedField(resource.Object, "not-a-quantity", "spec", "resources", "requests", "cpu")).To(Succeed())
		Expect(k8sClient.Create(ctx, resource)).NotTo(Succeed())
	})

	It("accepts integer-form CPU quantities", func() {
		workload.Name, workload.GenerateName = "integer-quantity", ""
		object, err := runtime.DefaultUnstructuredConverter.ToUnstructured(workload)
		Expect(err).NotTo(HaveOccurred())
		resource := &unstructured.Unstructured{Object: object}
		resource.SetAPIVersion("workloads.example.com/v1alpha1")
		resource.SetKind("CustomDeployment")
		unstructured.RemoveNestedField(resource.Object, "spec", "resources", "limits")
		Expect(unstructured.SetNestedField(resource.Object, int64(1), "spec", "resources", "requests", "cpu")).To(Succeed())
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())
	})

	It("accepts requests without limits", func() {
		workload.Name, workload.GenerateName = "requests-only", ""
		workload.Spec.Resources.Limits = nil
		Expect(k8sClient.Create(ctx, workload)).To(Succeed())
	})

	DescribeTable("rejects invalid resource limits", func(limits corev1.ResourceList) {
		workload.Spec.Resources.Limits = limits
		Expect(k8sClient.Create(ctx, workload)).NotTo(Succeed())
	},
		Entry("zero CPU limit", corev1.ResourceList{
			corev1.ResourceCPU: resource.MustParse("0"), corev1.ResourceMemory: resource.MustParse("256Mi"),
		}),
		Entry("negative memory limit", corev1.ResourceList{
			corev1.ResourceCPU: resource.MustParse("500m"), corev1.ResourceMemory: resource.MustParse("-1Mi"),
		}),
		Entry("CPU limit below request", corev1.ResourceList{
			corev1.ResourceCPU: resource.MustParse("50m"), corev1.ResourceMemory: resource.MustParse("256Mi"),
		}),
		Entry("memory limit below request", corev1.ResourceList{
			corev1.ResourceCPU: resource.MustParse("500m"), corev1.ResourceMemory: resource.MustParse("64Mi"),
		}),
	)

	DescribeTable("rejects invalid replica bounds", func(minReplicas, maxReplicas int32) {
		workload.Spec.MinReplicas, workload.Spec.MaxReplicas = minReplicas, maxReplicas
		Expect(k8sClient.Create(ctx, workload)).NotTo(Succeed())
	},
		Entry("negative minimum replicas", int32(-1), int32(5)),
		Entry("maximum below minimum", int32(5), int32(4)),
	)

	It("rejects an explicitly zero minimum replica count", func() {
		workload.Name, workload.GenerateName = "zero-min-replicas", ""
		object, err := runtime.DefaultUnstructuredConverter.ToUnstructured(workload)
		Expect(err).NotTo(HaveOccurred())
		resource := &unstructured.Unstructured{Object: object}
		resource.SetAPIVersion("workloads.example.com/v1alpha1")
		resource.SetKind("CustomDeployment")
		Expect(unstructured.SetNestedField(resource.Object, int64(0), "spec", "minReplicas")).To(Succeed())
		Expect(k8sClient.Create(ctx, resource)).NotTo(Succeed())
	})

	DescribeTable("clamps the initial replica count when creating the Deployment", func(initial, minReplicas, maxReplicas, expected int32) {
		workload.Spec.Replicas, workload.Spec.MinReplicas, workload.Spec.MaxReplicas = initial, minReplicas, maxReplicas
		Expect(k8sClient.Create(ctx, workload)).To(Succeed())
		deployment := &appsv1.Deployment{}
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKeyFromObject(workload), deployment)
		}, 10*time.Second, 50*time.Millisecond).Should(Succeed())
		Expect(*deployment.Spec.Replicas).To(Equal(expected))
	},
		Entry("above maximum", int32(9), int32(2), int32(5), int32(5)),
		Entry("below minimum", int32(1), int32(2), int32(5), int32(2)),
		Entry("within bounds", int32(3), int32(2), int32(5), int32(3)),
	)

	It("updates image and resources without resetting the Deployment replica count", func() {
		Expect(k8sClient.Create(ctx, workload)).To(Succeed())
		deployment := &appsv1.Deployment{}
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKeyFromObject(workload), deployment)
		}, 10*time.Second, 50*time.Millisecond).Should(Succeed())

		deployment.Spec.Replicas = new(int32(4))
		Expect(k8sClient.Update(ctx, deployment)).To(Succeed())
		Eventually(func() error {
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workload), workload)).To(Succeed())
			workload.Spec.Image = "nginx:1.29.0"
			workload.Spec.Resources = corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("200m"), corev1.ResourceMemory: resource.MustParse("256Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("1"), corev1.ResourceMemory: resource.MustParse("512Mi"),
				},
			}
			return k8sClient.Update(ctx, workload)
		}, 10*time.Second, 50*time.Millisecond).Should(Succeed())

		Eventually(func() string {
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workload), deployment)).To(Succeed())
			return deployment.Spec.Template.Spec.Containers[0].Image
		}, 10*time.Second, 50*time.Millisecond).Should(Equal("nginx:1.29.0"))
		Expect(deployment.Spec.Template.Spec.Containers[0].Resources).To(Equal(workload.Spec.Resources))
		Expect(*deployment.Spec.Replicas).To(Equal(int32(4)))
	})

	It("reports Ready after the Deployment observes the current generation and rollout", func() {
		Expect(k8sClient.Create(ctx, workload)).To(Succeed())
		deployment := &appsv1.Deployment{}
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKeyFromObject(workload), deployment)
		}, 10*time.Second, 50*time.Millisecond).Should(Succeed())

		deployment.Status.ObservedGeneration = deployment.Generation
		deployment.Status.Replicas = *deployment.Spec.Replicas
		deployment.Status.UpdatedReplicas = *deployment.Spec.Replicas
		deployment.Status.ReadyReplicas = *deployment.Spec.Replicas
		deployment.Status.AvailableReplicas = *deployment.Spec.Replicas
		Expect(k8sClient.Status().Update(ctx, deployment)).To(Succeed())

		observed := &workloadsv1alpha1.CustomDeployment{}
		Eventually(func() metav1.ConditionStatus {
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(workload), observed); err != nil {
				return metav1.ConditionUnknown
			}
			condition := apimeta.FindStatusCondition(observed.Status.Conditions, "Ready")
			if condition == nil {
				return metav1.ConditionUnknown
			}
			return condition.Status
		}, 10*time.Second, 50*time.Millisecond).Should(Equal(metav1.ConditionTrue))
		condition := apimeta.FindStatusCondition(observed.Status.Conditions, "Ready")
		Expect(condition.Reason).To(Equal("RolloutComplete"))
		Expect(observed.Status.ObservedGeneration).To(Equal(observed.Generation))
		Expect(observed.Status.DesiredReplicas).To(Equal(int32(2)))
		Expect(observed.Status.ReadyReplicas).To(Equal(int32(2)))
		Expect(observed.Status.Conditions).To(HaveLen(1))

		resourceVersion := observed.ResourceVersion
		transitionTime := condition.LastTransitionTime
		reconciler := &CustomDeploymentReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
		_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(workload)})
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workload), observed)).To(Succeed())
		Expect(observed.ResourceVersion).To(Equal(resourceVersion))
		Expect(apimeta.FindStatusCondition(observed.Status.Conditions, "Ready").LastTransitionTime).To(Equal(transitionTime))
	})

	It("does not report stale Deployment readiness after a spec update", func() {
		Expect(k8sClient.Create(ctx, workload)).To(Succeed())
		deployment := &appsv1.Deployment{}
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKeyFromObject(workload), deployment)
		}, 10*time.Second, 50*time.Millisecond).Should(Succeed())
		deployment.Status.ObservedGeneration = deployment.Generation
		deployment.Status.Replicas = *deployment.Spec.Replicas
		deployment.Status.UpdatedReplicas = *deployment.Spec.Replicas
		deployment.Status.ReadyReplicas = *deployment.Spec.Replicas
		deployment.Status.AvailableReplicas = *deployment.Spec.Replicas
		Expect(k8sClient.Status().Update(ctx, deployment)).To(Succeed())

		observed := &workloadsv1alpha1.CustomDeployment{}
		Eventually(func() metav1.ConditionStatus {
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workload), observed)).To(Succeed())
			condition := apimeta.FindStatusCondition(observed.Status.Conditions, "Ready")
			if condition == nil {
				return metav1.ConditionUnknown
			}
			return condition.Status
		}, 10*time.Second, 50*time.Millisecond).Should(Equal(metav1.ConditionTrue))

		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workload), workload)).To(Succeed())
		workload.Spec.Image = "nginx:1.29.0"
		Expect(k8sClient.Update(ctx, workload)).To(Succeed())
		Eventually(func() string {
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workload), deployment)).To(Succeed())
			return deployment.Spec.Template.Spec.Containers[0].Image
		}, 10*time.Second, 50*time.Millisecond).Should(Equal("nginx:1.29.0"))
		Expect(deployment.Status.ObservedGeneration).To(BeNumerically("<", deployment.Generation))
		Eventually(func() metav1.ConditionStatus {
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workload), observed)).To(Succeed())
			return apimeta.FindStatusCondition(observed.Status.Conditions, "Ready").Status
		}, 10*time.Second, 50*time.Millisecond).Should(Equal(metav1.ConditionFalse))
		condition := apimeta.FindStatusCondition(observed.Status.Conditions, "Ready")
		Expect(condition.Reason).To(Equal("RolloutInProgress"))
		Expect(condition.Message).To(ContainSubstring("has not observed generation"))

		deployment.Status.ObservedGeneration = deployment.Generation
		Expect(k8sClient.Status().Update(ctx, deployment)).To(Succeed())
		reconciler := &CustomDeploymentReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
		_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(workload)})
		Expect(err).NotTo(HaveOccurred())
		Eventually(func() string {
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workload), observed)).To(Succeed())
			condition := apimeta.FindStatusCondition(observed.Status.Conditions, "Ready")
			return string(condition.Status) + ": " + condition.Message
		}, 10*time.Second, 50*time.Millisecond).Should(HavePrefix(string(metav1.ConditionTrue)))
	})

	It("reports pending rollout status without rewriting unchanged status", func() {
		Expect(k8sClient.Create(ctx, workload)).To(Succeed())
		observed := &workloadsv1alpha1.CustomDeployment{}
		Eventually(func() *metav1.Condition {
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(workload), observed); err != nil {
				return nil
			}
			return apimeta.FindStatusCondition(observed.Status.Conditions, "Ready")
		}, 10*time.Second, 50*time.Millisecond).ShouldNot(BeNil())

		Expect(observed.Status.ObservedGeneration).To(Equal(observed.Generation))
		Expect(observed.Status.DesiredReplicas).To(Equal(int32(2)))
		Expect(observed.Status.ReadyReplicas).To(BeZero())
		Expect(observed.Status.Conditions).To(HaveLen(1))
		condition := apimeta.FindStatusCondition(observed.Status.Conditions, "Ready")
		Expect(condition.Status).To(Equal(metav1.ConditionFalse))
		Expect(condition.Reason).To(Equal("RolloutInProgress"))

		resourceVersion := observed.ResourceVersion
		transitionTime := condition.LastTransitionTime
		reconciler := &CustomDeploymentReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
		_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(workload)})
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workload), observed)).To(Succeed())
		Expect(observed.ResourceVersion).To(Equal(resourceVersion))
		Expect(apimeta.FindStatusCondition(observed.Status.Conditions, "Ready").LastTransitionTime).To(Equal(transitionTime))
	})

	It("does not reset replicas or rewrite the Deployment when only bounds change", func() {
		Expect(k8sClient.Create(ctx, workload)).To(Succeed())
		deployment := &appsv1.Deployment{}
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKeyFromObject(workload), deployment)
		}, 10*time.Second, 50*time.Millisecond).Should(Succeed())
		deployment.Spec.Replicas = new(int32(4))
		Expect(k8sClient.Update(ctx, deployment)).To(Succeed())
		resourceVersion := deployment.ResourceVersion

		Eventually(func() error {
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workload), workload)).To(Succeed())
			workload.Spec.MinReplicas, workload.Spec.MaxReplicas = 2, 7
			return k8sClient.Update(ctx, workload)
		}, 10*time.Second, 50*time.Millisecond).Should(Succeed())
		reconciler := &CustomDeploymentReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
		_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(workload)})
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workload), deployment)).To(Succeed())
		Expect(deployment.ResourceVersion).To(Equal(resourceVersion))
		Expect(*deployment.Spec.Replicas).To(Equal(int32(4)))
	})

	It("keeps the initial replica count immutable", func() {
		workload.Name, workload.GenerateName = "immutable-replicas", ""
		workload.Spec.Replicas = 3
		Expect(k8sClient.Create(ctx, workload)).To(Succeed())

		Eventually(func() string {
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(workload), workload); err != nil {
				return err.Error()
			}
			workload.Spec.Replicas = 4
			if err := k8sClient.Update(ctx, workload); err != nil {
				return err.Error()
			}
			return ""
		}, 10*time.Second, 50*time.Millisecond).Should(ContainSubstring("immutable"))

		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workload), workload)).To(Succeed())
		Expect(workload.Spec.Replicas).To(Equal(int32(3)))
		Eventually(func() string {
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(workload), workload); err != nil {
				return err.Error()
			}
			object, err := runtime.DefaultUnstructuredConverter.ToUnstructured(workload)
			if err != nil {
				return err.Error()
			}
			resource := &unstructured.Unstructured{Object: object}
			resource.SetAPIVersion("workloads.example.com/v1alpha1")
			resource.SetKind("CustomDeployment")
			unstructured.RemoveNestedField(resource.Object, "spec", "replicas")
			if err := k8sClient.Update(ctx, resource); err != nil {
				return err.Error()
			}
			return ""
		}, 10*time.Second, 50*time.Millisecond).Should(ContainSubstring("immutable"))

		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workload), workload)).To(Succeed())
		Expect(workload.Spec.Replicas).To(Equal(int32(3)))
	})

	It("defaults omitted replica settings", func() {
		workload.Name, workload.GenerateName = "default-replicas", ""
		object, err := runtime.DefaultUnstructuredConverter.ToUnstructured(workload)
		Expect(err).NotTo(HaveOccurred())
		resource := &unstructured.Unstructured{Object: object}
		resource.SetAPIVersion("workloads.example.com/v1alpha1")
		resource.SetKind("CustomDeployment")
		unstructured.RemoveNestedField(resource.Object, "spec", "replicas")
		unstructured.RemoveNestedField(resource.Object, "spec", "minReplicas")
		unstructured.RemoveNestedField(resource.Object, "spec", "maxReplicas")
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workload), workload)).To(Succeed())
		Expect(workload.Spec.Replicas).To(Equal(int32(1)))
		Expect(workload.Spec.MinReplicas).To(Equal(int32(1)))
		Expect(workload.Spec.MaxReplicas).To(Equal(int32(5)))
	})
})
