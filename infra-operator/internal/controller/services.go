/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	infrav1alpha1 "github.com/sandbox0-ai/infra-operator/api/v1alpha1"
)

// Service definitions
type ServiceDefinition struct {
	Name               string
	Port               int32
	TargetPort         int32
	Ports              []corev1.ContainerPort
	Image              string
	Command            []string
	Args               []string
	EnvVars            []corev1.EnvVar
	VolumeMounts       []corev1.VolumeMount
	Volumes            []corev1.Volume
	LivenessProbe      *corev1.Probe
	ReadinessProbe     *corev1.Probe
	ServiceAccountName string
}

// reconcileEdgeGateway reconciles the edge-gateway deployment
func (r *Sandbox0InfraReconciler) reconcileEdgeGateway(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) error {
	logger := log.FromContext(ctx)

	// Skip if not enabled
	if infra.Spec.Services != nil && infra.Spec.Services.EdgeGateway != nil && !infra.Spec.Services.EdgeGateway.Enabled {
		logger.Info("Edge gateway is disabled, skipping")
		return nil
	}

	deploymentName := fmt.Sprintf("%s-edge-gateway", infra.Name)
	serviceName := deploymentName

	replicas := int32(1)
	if infra.Spec.Services != nil && infra.Spec.Services.EdgeGateway != nil {
		replicas = infra.Spec.Services.EdgeGateway.Replicas
	}

	labels := r.getServiceLabels(infra.Name, "edge-gateway")
	keySecretName, privateKeyKey, _ := r.getControlPlaneKeyRefs(infra)

	config, err := r.buildEdgeGatewayConfig(ctx, infra)
	if err != nil {
		return err
	}
	if err := r.reconcileServiceConfigMap(ctx, infra, deploymentName, labels, config); err != nil {
		return err
	}

	// Create deployment
	if err := r.reconcileDeployment(ctx, infra, deploymentName, labels, replicas, ServiceDefinition{
		Name:       "edge-gateway",
		Port:       8080,
		TargetPort: 8080,
		Ports: []corev1.ContainerPort{
			{
				Name:          "http",
				ContainerPort: 8080,
			},
		},
		Image: fmt.Sprintf("%s:%s", defaultImageRepo, infra.Spec.Version),
		EnvVars: []corev1.EnvVar{
			{
				Name:  "SERVICE",
				Value: "edge-gateway",
			},
			{
				Name:  "CONFIG_PATH",
				Value: "/config/config.yaml",
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "config",
				MountPath: "/config/config.yaml",
				SubPath:   "config.yaml",
				ReadOnly:  true,
			},
			{
				Name:      "internal-jwt-private-key",
				MountPath: "/secrets/internal_jwt_private.key",
				SubPath:   "internal_jwt_private.key",
				ReadOnly:  true,
			},
		},
		Volumes: []corev1.Volume{
			{
				Name: "config",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{Name: deploymentName},
					},
				},
			},
			{
				Name: "internal-jwt-private-key",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: keySecretName,
						Items: []corev1.KeyToPath{
							{
								Key:  privateKeyKey,
								Path: "internal_jwt_private.key",
							},
						},
					},
				},
			},
		},
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/healthz",
					Port: intstr.FromString("http"),
				},
			},
			InitialDelaySeconds: 10,
			PeriodSeconds:       10,
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/readyz",
					Port: intstr.FromString("http"),
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       5,
		},
	}); err != nil {
		return err
	}

	// Create service
	serviceType := corev1.ServiceTypeClusterIP
	servicePort := int32(80)
	if infra.Spec.Services != nil && infra.Spec.Services.EdgeGateway != nil && infra.Spec.Services.EdgeGateway.Service != nil {
		serviceType = infra.Spec.Services.EdgeGateway.Service.Type
		servicePort = infra.Spec.Services.EdgeGateway.Service.Port
	}
	if err := r.reconcileService(ctx, infra, serviceName, labels, serviceType, servicePort, 8080); err != nil {
		return err
	}

	// Create ingress if enabled
	if infra.Spec.Services != nil && infra.Spec.Services.EdgeGateway != nil &&
		infra.Spec.Services.EdgeGateway.Ingress != nil && infra.Spec.Services.EdgeGateway.Ingress.Enabled {
		if err := r.reconcileIngress(ctx, infra, serviceName, infra.Spec.Services.EdgeGateway.Ingress); err != nil {
			return err
		}
	}

	// Update endpoints in status
	r.updateEndpoints(ctx, infra, serviceName, servicePort)

	logger.Info("Edge gateway reconciled successfully")
	return nil
}

// reconcileScheduler reconciles the scheduler deployment
func (r *Sandbox0InfraReconciler) reconcileScheduler(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) error {
	logger := log.FromContext(ctx)

	// Skip if not enabled (scheduler is optional by default)
	if infra.Spec.Services == nil || infra.Spec.Services.Scheduler == nil || !infra.Spec.Services.Scheduler.Enabled {
		logger.Info("Scheduler is disabled, skipping")
		return nil
	}

	deploymentName := fmt.Sprintf("%s-scheduler", infra.Name)
	replicas := infra.Spec.Services.Scheduler.Replicas
	labels := r.getServiceLabels(infra.Name, "scheduler")
	keySecretName, privateKeyKey, publicKeyKey := r.getControlPlaneKeyRefs(infra)

	config, err := r.buildSchedulerConfig(ctx, infra)
	if err != nil {
		return err
	}
	if err := r.reconcileServiceConfigMap(ctx, infra, deploymentName, labels, config); err != nil {
		return err
	}

	// Create deployment
	if err := r.reconcileDeployment(ctx, infra, deploymentName, labels, replicas, ServiceDefinition{
		Name:               "scheduler",
		Port:               8080,
		TargetPort:         8080,
		ServiceAccountName: fmt.Sprintf("%s-scheduler", infra.Name),
		Ports: []corev1.ContainerPort{
			{
				Name:          "http",
				ContainerPort: 8080,
			},
		},
		Image: fmt.Sprintf("%s:%s", defaultImageRepo, infra.Spec.Version),
		EnvVars: []corev1.EnvVar{
			{
				Name:  "SERVICE",
				Value: "scheduler",
			},
			{
				Name:  "CONFIG_PATH",
				Value: "/config/config.yaml",
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "config",
				MountPath: "/config/config.yaml",
				SubPath:   "config.yaml",
				ReadOnly:  true,
			},
			{
				Name:      "internal-jwt-private-key",
				MountPath: "/secrets/internal_jwt_private.key",
				SubPath:   "internal_jwt_private.key",
				ReadOnly:  true,
			},
			{
				Name:      "internal-jwt-public-key",
				MountPath: "/config/internal_jwt_public.key",
				SubPath:   "internal_jwt_public.key",
				ReadOnly:  true,
			},
		},
		Volumes: []corev1.Volume{
			{
				Name: "config",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{Name: deploymentName},
					},
				},
			},
			{
				Name: "internal-jwt-private-key",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: keySecretName,
						Items: []corev1.KeyToPath{
							{
								Key:  privateKeyKey,
								Path: "internal_jwt_private.key",
							},
						},
					},
				},
			},
			{
				Name: "internal-jwt-public-key",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: keySecretName,
						Items: []corev1.KeyToPath{
							{
								Key:  publicKeyKey,
								Path: "internal_jwt_public.key",
							},
						},
					},
				},
			},
		},
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/healthz",
					Port: intstr.FromString("http"),
				},
			},
			InitialDelaySeconds: 10,
			PeriodSeconds:       10,
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/readyz",
					Port: intstr.FromString("http"),
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       5,
		},
	}); err != nil {
		return err
	}

	// Create service
	serviceType := corev1.ServiceTypeClusterIP
	servicePort := int32(8080)
	if infra.Spec.Services != nil && infra.Spec.Services.Scheduler != nil && infra.Spec.Services.Scheduler.Service != nil {
		serviceType = infra.Spec.Services.Scheduler.Service.Type
		servicePort = infra.Spec.Services.Scheduler.Service.Port
	}
	if err := r.reconcileService(ctx, infra, deploymentName, labels, serviceType, servicePort, 8080); err != nil {
		return err
	}

	logger.Info("Scheduler reconciled successfully")
	return nil
}

// reconcileInternalGateway reconciles the internal-gateway deployment
func (r *Sandbox0InfraReconciler) reconcileInternalGateway(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) error {
	logger := log.FromContext(ctx)

	// Skip if not enabled
	if infra.Spec.Services != nil && infra.Spec.Services.InternalGateway != nil && !infra.Spec.Services.InternalGateway.Enabled {
		logger.Info("Internal gateway is disabled, skipping")
		return nil
	}

	deploymentName := fmt.Sprintf("%s-internal-gateway", infra.Name)
	serviceName := deploymentName

	replicas := int32(1)
	if infra.Spec.Services != nil && infra.Spec.Services.InternalGateway != nil {
		replicas = infra.Spec.Services.InternalGateway.Replicas
	}

	labels := r.getServiceLabels(infra.Name, "internal-gateway")
	dataPlaneSecretName, dataPlanePrivateKey, _ := r.getDataPlaneKeyRefs(infra)
	controlPlaneSecretName, _, controlPlanePublicKey := r.getControlPlaneKeyRefs(infra)

	config, err := r.buildInternalGatewayConfig(ctx, infra)
	if err != nil {
		return err
	}
	if err := r.reconcileServiceConfigMap(ctx, infra, deploymentName, labels, config); err != nil {
		return err
	}

	controlPlanePublicSecretName, controlPlanePublicKeyKey := r.getControlPlanePublicKeyRef(infra)
	if controlPlanePublicSecretName == "" {
		controlPlanePublicSecretName = controlPlaneSecretName
		controlPlanePublicKeyKey = controlPlanePublicKey
	}

	// Create deployment
	if err := r.reconcileDeployment(ctx, infra, deploymentName, labels, replicas, ServiceDefinition{
		Name:       "internal-gateway",
		Port:       8443,
		TargetPort: 8443,
		Ports: []corev1.ContainerPort{
			{
				Name:          "http",
				ContainerPort: 8443,
			},
		},
		Image: fmt.Sprintf("%s:%s", defaultImageRepo, infra.Spec.Version),
		EnvVars: []corev1.EnvVar{
			{
				Name:  "SERVICE",
				Value: "internal-gateway",
			},
			{
				Name:  "CONFIG_PATH",
				Value: "/config/config.yaml",
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "config",
				MountPath: "/config/config.yaml",
				SubPath:   "config.yaml",
				ReadOnly:  true,
			},
			{
				Name:      "internal-jwt-private-key",
				MountPath: "/secrets/internal_jwt_private.key",
				SubPath:   "internal_jwt_private.key",
				ReadOnly:  true,
			},
			{
				Name:      "internal-jwt-public-key",
				MountPath: "/config/internal_jwt_public.key",
				SubPath:   "internal_jwt_public.key",
				ReadOnly:  true,
			},
		},
		Volumes: []corev1.Volume{
			{
				Name: "config",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{Name: deploymentName},
					},
				},
			},
			{
				Name: "internal-jwt-private-key",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: dataPlaneSecretName,
						Items: []corev1.KeyToPath{
							{
								Key:  dataPlanePrivateKey,
								Path: "internal_jwt_private.key",
							},
						},
					},
				},
			},
			{
				Name: "internal-jwt-public-key",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: controlPlanePublicSecretName,
						Items: []corev1.KeyToPath{
							{
								Key:  controlPlanePublicKeyKey,
								Path: "internal_jwt_public.key",
							},
						},
					},
				},
			},
		},
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/healthz",
					Port: intstr.FromString("http"),
				},
			},
			InitialDelaySeconds: 10,
			PeriodSeconds:       10,
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/readyz",
					Port: intstr.FromString("http"),
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       5,
		},
	}); err != nil {
		return err
	}

	// Create service
	if err := r.reconcileService(ctx, infra, serviceName, labels, corev1.ServiceTypeClusterIP, 8443, 8443); err != nil {
		return err
	}

	// Update endpoints in status
	if infra.Status.Endpoints == nil {
		infra.Status.Endpoints = &infrav1alpha1.EndpointsStatus{}
	}
	infra.Status.Endpoints.InternalGateway = fmt.Sprintf("http://%s:8443", serviceName)

	logger.Info("Internal gateway reconciled successfully")
	return nil
}

// reconcileManager reconciles the manager deployment
func (r *Sandbox0InfraReconciler) reconcileManager(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) error {
	logger := log.FromContext(ctx)

	// Skip if not enabled
	if infra.Spec.Services != nil && infra.Spec.Services.Manager != nil && !infra.Spec.Services.Manager.Enabled {
		logger.Info("Manager is disabled, skipping")
		return nil
	}

	deploymentName := fmt.Sprintf("%s-manager", infra.Name)

	replicas := int32(1)
	if infra.Spec.Services != nil && infra.Spec.Services.Manager != nil {
		replicas = infra.Spec.Services.Manager.Replicas
	}

	labels := r.getServiceLabels(infra.Name, "manager")
	keySecretName, privateKeyKey, publicKeyKey := r.getDataPlaneKeyRefs(infra)

	config, err := r.buildManagerConfig(ctx, infra)
	if err != nil {
		return err
	}
	if err := r.reconcileServiceConfigMap(ctx, infra, deploymentName, labels, config); err != nil {
		return err
	}

	// Create deployment
	if err := r.reconcileDeployment(ctx, infra, deploymentName, labels, replicas, ServiceDefinition{
		Name:               "manager",
		Port:               8080,
		TargetPort:         8080,
		ServiceAccountName: fmt.Sprintf("%s-manager", infra.Name),
		Ports: []corev1.ContainerPort{
			{
				Name:          "http",
				ContainerPort: 8080,
			},
			{
				Name:          "metrics",
				ContainerPort: 9090,
			},
			{
				Name:          "webhook",
				ContainerPort: 9443,
			},
		},
		Image: fmt.Sprintf("%s:%s", defaultImageRepo, infra.Spec.Version),
		EnvVars: []corev1.EnvVar{
			{
				Name:  "SERVICE",
				Value: "manager",
			},
			{
				Name:  "CONFIG_PATH",
				Value: "/config/config.yaml",
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "config",
				MountPath: "/config/config.yaml",
				SubPath:   "config.yaml",
				ReadOnly:  true,
			},
			{
				Name:      "internal-jwt-private-key",
				MountPath: "/secrets/internal_jwt_private.key",
				SubPath:   "internal_jwt_private.key",
				ReadOnly:  true,
			},
			{
				Name:      "internal-jwt-public-key",
				MountPath: "/config/internal_jwt_public.key",
				SubPath:   "internal_jwt_public.key",
				ReadOnly:  true,
			},
		},
		Volumes: []corev1.Volume{
			{
				Name: "config",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{Name: deploymentName},
					},
				},
			},
			{
				Name: "internal-jwt-private-key",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: keySecretName,
						Items: []corev1.KeyToPath{
							{
								Key:  privateKeyKey,
								Path: "internal_jwt_private.key",
							},
						},
					},
				},
			},
			{
				Name: "internal-jwt-public-key",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: keySecretName,
						Items: []corev1.KeyToPath{
							{
								Key:  publicKeyKey,
								Path: "internal_jwt_public.key",
							},
						},
					},
				},
			},
		},
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/healthz",
					Port: intstr.FromString("http"),
				},
			},
			InitialDelaySeconds: 10,
			PeriodSeconds:       10,
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/readyz",
					Port: intstr.FromString("http"),
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       5,
		},
	}); err != nil {
		return err
	}

	// Create service
	serviceType := corev1.ServiceTypeClusterIP
	servicePort := int32(8080)
	if infra.Spec.Services != nil && infra.Spec.Services.Manager != nil && infra.Spec.Services.Manager.Service != nil {
		serviceType = infra.Spec.Services.Manager.Service.Type
		servicePort = infra.Spec.Services.Manager.Service.Port
	}
	if err := r.reconcileService(ctx, infra, deploymentName, labels, serviceType, servicePort, 8080); err != nil {
		return err
	}
	if err := r.reconcileService(ctx, infra, fmt.Sprintf("%s-metrics", deploymentName), labels, corev1.ServiceTypeClusterIP, 9090, 9090); err != nil {
		return err
	}
	if err := r.reconcileService(ctx, infra, fmt.Sprintf("%s-webhook", deploymentName), labels, corev1.ServiceTypeClusterIP, 9443, 9443); err != nil {
		return err
	}

	logger.Info("Manager reconciled successfully")
	return nil
}

// reconcileStorageProxy reconciles the storage-proxy deployment
func (r *Sandbox0InfraReconciler) reconcileStorageProxy(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) error {
	logger := log.FromContext(ctx)

	// Skip if not enabled
	if infra.Spec.Services != nil && infra.Spec.Services.StorageProxy != nil && !infra.Spec.Services.StorageProxy.Enabled {
		logger.Info("Storage proxy is disabled, skipping")
		return nil
	}

	deploymentName := fmt.Sprintf("%s-storage-proxy", infra.Name)
	serviceName := deploymentName

	replicas := int32(1)
	if infra.Spec.Services != nil && infra.Spec.Services.StorageProxy != nil {
		replicas = infra.Spec.Services.StorageProxy.Replicas
	}

	labels := r.getServiceLabels(infra.Name, "storage-proxy")
	keySecretName, _, publicKeyKey := r.getDataPlaneKeyRefs(infra)

	config, err := r.buildStorageProxyConfig(ctx, infra)
	if err != nil {
		return err
	}
	if err := r.reconcileServiceConfigMap(ctx, infra, deploymentName, labels, config); err != nil {
		return err
	}

	// Create deployment
	if err := r.reconcileDeployment(ctx, infra, deploymentName, labels, replicas, ServiceDefinition{
		Name:               "storage-proxy",
		Port:               8080,
		TargetPort:         8080,
		ServiceAccountName: fmt.Sprintf("%s-storage-proxy", infra.Name),
		Ports: []corev1.ContainerPort{
			{
				Name:          "grpc",
				ContainerPort: 8080,
			},
			{
				Name:          "http",
				ContainerPort: 8081,
			},
			{
				Name:          "metrics",
				ContainerPort: 9090,
			},
		},
		Image: fmt.Sprintf("%s:%s", defaultImageRepo, infra.Spec.Version),
		EnvVars: []corev1.EnvVar{
			{
				Name:  "SERVICE",
				Value: "storage-proxy",
			},
			{
				Name:  "CONFIG_PATH",
				Value: "/config/config.yaml",
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "config",
				MountPath: "/config/config.yaml",
				SubPath:   "config.yaml",
				ReadOnly:  true,
			},
			{
				Name:      "internal-jwt-public-key",
				MountPath: "/config/internal_jwt_public.key",
				SubPath:   "internal_jwt_public.key",
				ReadOnly:  true,
			},
			{
				Name:      "cache",
				MountPath: "/var/lib/storage-proxy/cache",
			},
			{
				Name:      "logs",
				MountPath: "/var/log/storage-proxy",
			},
		},
		Volumes: []corev1.Volume{
			{
				Name: "config",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{Name: deploymentName},
					},
				},
			},
			{
				Name: "internal-jwt-public-key",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: keySecretName,
						Items: []corev1.KeyToPath{
							{
								Key:  publicKeyKey,
								Path: "internal_jwt_public.key",
							},
						},
					},
				},
			},
			{
				Name: "cache",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
			{
				Name: "logs",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
		},
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/healthz",
					Port: intstr.FromString("http"),
				},
			},
			InitialDelaySeconds: 10,
			PeriodSeconds:       10,
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/readyz",
					Port: intstr.FromString("http"),
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       5,
		},
	}); err != nil {
		return err
	}

	// Create service (gRPC)
	if err := r.reconcileService(ctx, infra, serviceName, labels, corev1.ServiceTypeClusterIP, 8080, 8080); err != nil {
		return err
	}
	if err := r.reconcileService(ctx, infra, fmt.Sprintf("%s-http", serviceName), labels, corev1.ServiceTypeClusterIP, 8081, 8081); err != nil {
		return err
	}
	if err := r.reconcileService(ctx, infra, fmt.Sprintf("%s-metrics", serviceName), labels, corev1.ServiceTypeClusterIP, 9090, 9090); err != nil {
		return err
	}

	logger.Info("Storage proxy reconciled successfully")
	return nil
}

// reconcileNetd reconciles the netd daemonset
func (r *Sandbox0InfraReconciler) reconcileNetd(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra) error {
	logger := log.FromContext(ctx)

	// Skip if not enabled
	if infra.Spec.Services != nil && infra.Spec.Services.Netd != nil && !infra.Spec.Services.Netd.Enabled {
		logger.Info("Netd is disabled, skipping")
		return nil
	}

	dsName := fmt.Sprintf("%s-netd", infra.Name)
	labels := r.getServiceLabels(infra.Name, "netd")
	config, err := r.buildNetdConfig(ctx, infra)
	if err != nil {
		return err
	}
	if err := r.reconcileServiceConfigMap(ctx, infra, dsName, labels, config); err != nil {
		return err
	}

	// Create DaemonSet
	if err := r.reconcileDaemonSet(ctx, infra, dsName, labels, ServiceDefinition{
		Name:               "netd",
		Port:               8080,
		TargetPort:         8080,
		ServiceAccountName: fmt.Sprintf("%s-netd", infra.Name),
		Ports: []corev1.ContainerPort{
			{
				Name:          "metrics",
				ContainerPort: 9090,
			},
			{
				Name:          "health",
				ContainerPort: 8080,
			},
			{
				Name:          "proxy-http",
				ContainerPort: 18080,
			},
			{
				Name:          "proxy-https",
				ContainerPort: 18443,
			},
		},
		Image: fmt.Sprintf("%s:%s", defaultImageRepo, infra.Spec.Version),
		EnvVars: []corev1.EnvVar{
			{
				Name:  "SERVICE",
				Value: "netd",
			},
			{
				Name:  "CONFIG_PATH",
				Value: "/config/config.yaml",
			},
			{
				Name: "NODE_NAME",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "spec.nodeName",
					},
				},
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "config",
				MountPath: "/config",
				ReadOnly:  true,
			},
			{
				Name:             "bpf-fs",
				MountPath:        "/sys/fs/bpf",
				MountPropagation: func() *corev1.MountPropagationMode { mode := corev1.MountPropagationBidirectional; return &mode }(),
			},
			{
				Name:      "cgroup",
				MountPath: "/sys/fs/cgroup",
				ReadOnly:  true,
			},
		},
		Volumes: []corev1.Volume{
			{
				Name: "config",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{Name: dsName},
					},
				},
			},
			{
				Name: "bpf-fs",
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: "/sys/fs/bpf",
						Type: func() *corev1.HostPathType { t := corev1.HostPathDirectoryOrCreate; return &t }(),
					},
				},
			},
			{
				Name: "cgroup",
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: "/sys/fs/cgroup",
						Type: func() *corev1.HostPathType { t := corev1.HostPathDirectory; return &t }(),
					},
				},
			},
		},
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/healthz",
					Port: intstr.FromString("health"),
				},
			},
			InitialDelaySeconds: 10,
			PeriodSeconds:       10,
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/readyz",
					Port: intstr.FromString("health"),
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       5,
		},
	}); err != nil {
		return err
	}

	if err := r.reconcileService(ctx, infra, fmt.Sprintf("%s-metrics", dsName), labels, corev1.ServiceTypeClusterIP, 9090, 9090); err != nil {
		return err
	}

	logger.Info("Netd reconciled successfully")
	return nil
}

// reconcileDeployment creates or updates a deployment
func (r *Sandbox0InfraReconciler) reconcileDeployment(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra, name string, labels map[string]string, replicas int32, def ServiceDefinition) error {
	deploy := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: infra.Namespace}, deploy)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	desiredDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: infra.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: def.ServiceAccountName,
					Containers: []corev1.Container{
						{
							Name:         def.Name,
							Image:        def.Image,
							Command:      def.Command,
							Args:         def.Args,
							Env:          def.EnvVars,
							VolumeMounts: def.VolumeMounts,
							Ports:        resolveContainerPorts(def),
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("256Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("512Mi"),
								},
							},
							LivenessProbe:  def.LivenessProbe,
							ReadinessProbe: def.ReadinessProbe,
						},
					},
					Volumes: def.Volumes,
				},
			},
		},
	}

	if err := ctrl.SetControllerReference(infra, desiredDeploy, r.Scheme); err != nil {
		return err
	}

	if errors.IsNotFound(err) {
		return r.Create(ctx, desiredDeploy)
	}

	deploy.Spec = desiredDeploy.Spec
	return r.Update(ctx, deploy)
}

// reconcileDaemonSet creates or updates a daemonset
func (r *Sandbox0InfraReconciler) reconcileDaemonSet(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra, name string, labels map[string]string, def ServiceDefinition) error {
	ds := &appsv1.DaemonSet{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: infra.Namespace}, ds)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	desiredDs := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: infra.Namespace,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: def.ServiceAccountName,
					HostNetwork:        true,
					HostPID:            true,
					DNSPolicy:          corev1.DNSClusterFirstWithHostNet,
					Containers: []corev1.Container{
						{
							Name:         def.Name,
							Image:        def.Image,
							Env:          def.EnvVars,
							VolumeMounts: def.VolumeMounts,
							Ports:        resolveContainerPorts(def),
							SecurityContext: &corev1.SecurityContext{
								Privileged: boolPtr(true),
								Capabilities: &corev1.Capabilities{
									Add: []corev1.Capability{"NET_ADMIN", "SYS_ADMIN"},
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("128Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("256Mi"),
								},
							},
							LivenessProbe:  def.LivenessProbe,
							ReadinessProbe: def.ReadinessProbe,
						},
					},
					Volumes: def.Volumes,
				},
			},
		},
	}

	if err := ctrl.SetControllerReference(infra, desiredDs, r.Scheme); err != nil {
		return err
	}

	if errors.IsNotFound(err) {
		return r.Create(ctx, desiredDs)
	}

	ds.Spec = desiredDs.Spec
	return r.Update(ctx, ds)
}

func resolveContainerPorts(def ServiceDefinition) []corev1.ContainerPort {
	if len(def.Ports) > 0 {
		return def.Ports
	}
	if def.TargetPort == 0 {
		return nil
	}
	return []corev1.ContainerPort{
		{
			Name:          "http",
			ContainerPort: def.TargetPort,
		},
	}
}

// reconcileService creates or updates a service
func (r *Sandbox0InfraReconciler) reconcileService(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra, name string, labels map[string]string, serviceType corev1.ServiceType, port, targetPort int32) error {
	svc := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: infra.Namespace}, svc)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	desiredSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: infra.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Type:     serviceType,
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       port,
					TargetPort: intstr.FromInt(int(targetPort)),
				},
			},
		},
	}

	if err := ctrl.SetControllerReference(infra, desiredSvc, r.Scheme); err != nil {
		return err
	}

	if errors.IsNotFound(err) {
		return r.Create(ctx, desiredSvc)
	}

	svc.Spec = desiredSvc.Spec
	return r.Update(ctx, svc)
}

// reconcileIngress creates or updates an ingress
func (r *Sandbox0InfraReconciler) reconcileIngress(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra, serviceName string, config *infrav1alpha1.IngressConfig) error {
	ingressName := serviceName

	ingress := &networkingv1.Ingress{}
	err := r.Get(ctx, types.NamespacedName{Name: ingressName, Namespace: infra.Namespace}, ingress)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	pathType := networkingv1.PathTypePrefix
	desiredIngress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ingressName,
			Namespace: infra.Namespace,
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: &config.ClassName,
			Rules: []networkingv1.IngressRule{
				{
					Host: config.Host,
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: &pathType,
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: serviceName,
											Port: networkingv1.ServiceBackendPort{
												Number: 80,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	if config.TLSSecret != "" {
		desiredIngress.Spec.TLS = []networkingv1.IngressTLS{
			{
				Hosts:      []string{config.Host},
				SecretName: config.TLSSecret,
			},
		}
	}

	if err := ctrl.SetControllerReference(infra, desiredIngress, r.Scheme); err != nil {
		return err
	}

	if errors.IsNotFound(err) {
		return r.Create(ctx, desiredIngress)
	}

	ingress.Spec = desiredIngress.Spec
	return r.Update(ctx, ingress)
}

// getServiceLabels returns standard labels for a service
func (r *Sandbox0InfraReconciler) getServiceLabels(instanceName, componentName string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       componentName,
		"app.kubernetes.io/instance":   instanceName,
		"app.kubernetes.io/component":  componentName,
		"app.kubernetes.io/managed-by": "sandbox0infra-operator",
	}
}

// updateEndpoints updates the status endpoints
func (r *Sandbox0InfraReconciler) updateEndpoints(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra, serviceName string, servicePort int32) {
	if infra.Status.Endpoints == nil {
		infra.Status.Endpoints = &infrav1alpha1.EndpointsStatus{}
	}

	internalURL := fmt.Sprintf("http://%s:%d", serviceName, servicePort)
	infra.Status.Endpoints.EdgeGatewayInternal = internalURL

	// If ingress is configured, set external URL
	if infra.Spec.Services != nil && infra.Spec.Services.EdgeGateway != nil &&
		infra.Spec.Services.EdgeGateway.Ingress != nil && infra.Spec.Services.EdgeGateway.Ingress.Enabled {
		ingress := infra.Spec.Services.EdgeGateway.Ingress
		scheme := "http"
		if ingress.TLSSecret != "" {
			scheme = "https"
		}
		infra.Status.Endpoints.EdgeGateway = fmt.Sprintf("%s://%s", scheme, ingress.Host)
	}
}

// boolPtr returns a pointer to a bool
func boolPtr(b bool) *bool {
	return &b
}
