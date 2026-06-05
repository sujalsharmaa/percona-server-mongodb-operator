package clustersync

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	api "github.com/percona/percona-server-mongodb-operator/pkg/apis/psmdb/v1"
)

// Container builds the PCSM container.
func Container(cr *api.PerconaServerMongoDBClusterSync, sourceURI, targetURI string) corev1.Container {
	return corev1.Container{
		Name:            ContainerName,
		Image:           cr.Spec.Image,
		ImagePullPolicy: cr.Spec.ImagePullPolicy,
		Ports: []corev1.ContainerPort{{
			Name:          HTTPPortName,
			ContainerPort: HTTPPort,
			Protocol:      corev1.ProtocolTCP,
		}},
		Env: []corev1.EnvVar{
			{Name: "PCSM_SOURCE_URI", Value: sourceURI},
			{Name: "PCSM_TARGET_URI", Value: targetURI},
		},
		Resources:       cr.Spec.Resources,
		SecurityContext: cr.Spec.ContainerSecurityContext,
		LivenessProbe:   livenessProbe(),
		ReadinessProbe:  readinessProbe(),
	}
}

func livenessProbe() *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt(int(HTTPPort))},
		},
		InitialDelaySeconds: 30,
		PeriodSeconds:       10,
		TimeoutSeconds:      5,
		FailureThreshold:    5,
	}
}

func readinessProbe() *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/status",
				Port: intstr.FromInt32(HTTPPort),
			},
		},
		InitialDelaySeconds: 5,
		PeriodSeconds:       10,
		TimeoutSeconds:      5,
		FailureThreshold:    3,
	}
}
