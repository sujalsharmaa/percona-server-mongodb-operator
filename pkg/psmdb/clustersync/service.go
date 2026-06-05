package clustersync

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	api "github.com/percona/percona-server-mongodb-operator/pkg/apis/psmdb/v1"
)

// Service fronts the PCSM HTTP API.
func Service(cr *api.PerconaServerMongoDBClusterSync) *corev1.Service {
	selector := Labels(cr)

	labels := map[string]string{}
	for k, v := range selector {
		labels[k] = v
	}
	for k, v := range cr.Spec.Expose.ServiceLabels {
		if _, ok := labels[k]; !ok {
			labels[k] = v
		}
	}

	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        DeploymentName(cr),
			Namespace:   cr.Namespace,
			Labels:      labels,
			Annotations: cr.Spec.Expose.ServiceAnnotations,
		},
		Spec: ServiceSpec(cr, selector),
	}
}

func ServiceSpec(cr *api.PerconaServerMongoDBClusterSync, selector map[string]string) corev1.ServiceSpec {
	spec := corev1.ServiceSpec{
		Ports: []corev1.ServicePort{{
			Name:       HTTPPortName,
			Port:       HTTPPort,
			TargetPort: intstr.FromInt(int(HTTPPort)),
			Protocol:   corev1.ProtocolTCP,
		}},
		Selector: selector,
	}

	switch cr.Spec.Expose.ExposeType {
	case corev1.ServiceTypeNodePort:
		spec.Type = corev1.ServiceTypeNodePort
		spec.ExternalTrafficPolicy = cr.Spec.Expose.ExternalTrafficPolicy
	case corev1.ServiceTypeLoadBalancer:
		spec.Type = corev1.ServiceTypeLoadBalancer
		spec.ExternalTrafficPolicy = cr.Spec.Expose.ExternalTrafficPolicy
		spec.LoadBalancerSourceRanges = cr.Spec.Expose.LoadBalancerSourceRanges
		spec.LoadBalancerClass = cr.Spec.Expose.LoadBalancerClass
	default:
		spec.Type = corev1.ServiceTypeClusterIP
	}

	if cr.Spec.Expose.InternalTrafficPolicy != nil {
		spec.InternalTrafficPolicy = cr.Spec.Expose.InternalTrafficPolicy
	}

	return spec
}
