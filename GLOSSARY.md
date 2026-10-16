# Custom Deployment

Language for the project's custom workload and its scaling behavior.

## Language

**CustomDeployment**:
The project's custom workload declaration, distinct from Kubernetes' built-in Deployment.
_Avoid_: Deployment when referring to the custom workload

**Initial replica count**:
The number of workload instances requested at creation, before autoscaling makes subsequent adjustments.
_Avoid_: Fixed replica count, minimum replica count

**Minimum replica count**:
The lower bound on the number of workload instances that autoscaling may request.
_Avoid_: Initial replica count

**Maximum replica count**:
The upper bound on the number of workload instances that autoscaling may request.
