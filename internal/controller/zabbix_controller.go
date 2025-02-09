/*
Copyright 2025.

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
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	monitoringv1alpha1 "github.com/vpdevx/zabbix-operator/api/v1alpha1"
)

const (
	finalizerName  = "zabbix.monitoring.zabbix.io/finalizer"
	componentName  = "zabbix-server"
	serverPort     = 10051
	serverPortName = "zabbix-server"
)

// ZabbixReconciler reconciles a Zabbix object
type ZabbixReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=monitoring.zabbix.io,resources=zabbixes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.zabbix.io,resources=zabbixes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=monitoring.zabbix.io,resources=zabbixes/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete

func (r *ZabbixReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("zabbix", req.NamespacedName)

	zabbix := &monitoringv1alpha1.Zabbix{}
	if err := r.Get(ctx, req.NamespacedName, zabbix); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !zabbix.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.handleFinalization(ctx, zabbix)
	}

	if !controllerutil.ContainsFinalizer(zabbix, finalizerName) {
		controllerutil.AddFinalizer(zabbix, finalizerName)
		if err := r.Update(ctx, zabbix); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to add finalizer: %w", err)
		}
	}

	// Reconcile server components
	if err := r.reconcileServerComponents(ctx, zabbix); err != nil {
		logger.Error(err, "failed to reconcile server components")
		return ctrl.Result{}, err
	}

	// Update status
	if err := r.updateStatus(ctx, zabbix); err != nil {
		logger.Error(err, "failed to update status")
		return ctrl.Result{}, err
	}

	logger.Info("successfully reconciled Zabbix resource")
	return ctrl.Result{}, nil
}

func (r *ZabbixReconciler) handleFinalization(ctx context.Context, zabbix *monitoringv1alpha1.Zabbix) (ctrl.Result, error) {
	if controllerutil.ContainsFinalizer(zabbix, finalizerName) {
		// Perform any cleanup logic here if needed
		controllerutil.RemoveFinalizer(zabbix, finalizerName)
		if err := r.Update(ctx, zabbix); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
		}
	}
	return ctrl.Result{}, nil
}

func (r *ZabbixReconciler) reconcileServerComponents(ctx context.Context, zabbix *monitoringv1alpha1.Zabbix) error {
	logger := log.FromContext(ctx).WithValues("component", componentName)

	// Reconcile Deployment
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-server", zabbix.Name),
			Namespace: zabbix.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, deployment, func() error {
		deployment.Labels = r.serverLabels(zabbix)
		deployment.Spec = appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: r.serverLabels(zabbix),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: r.serverLabels(zabbix),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						r.buildServerContainer(zabbix),
					},
				},
			},
		}
		return controllerutil.SetControllerReference(zabbix, deployment, r.Scheme)
	})
	if err != nil {
		return fmt.Errorf("failed to reconcile deployment: %w", err)
	}
	logger.Info("deployment reconciled", "operation", op)

	// Reconcile Service
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-server", zabbix.Name),
			Namespace: zabbix.Namespace,
		},
	}

	op, err = controllerutil.CreateOrUpdate(ctx, r.Client, service, func() error {
		service.Labels = r.serverLabels(zabbix)
		service.Spec = corev1.ServiceSpec{
			Selector: r.serverLabels(zabbix),
			Ports: []corev1.ServicePort{
				{
					Name:       serverPortName,
					Port:       serverPort,
					TargetPort: intstr.FromInt(serverPort),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		}
		return controllerutil.SetControllerReference(zabbix, service, r.Scheme)
	})
	if err != nil {
		return fmt.Errorf("failed to reconcile service: %w", err)
	}
	logger.Info("service reconciled", "operation", op)

	return nil
}

func (r *ZabbixReconciler) serverLabels(zabbix *monitoringv1alpha1.Zabbix) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       "zabbix",
		"app.kubernetes.io/instance":   zabbix.Name,
		"app.kubernetes.io/component":  componentName,
		"app.kubernetes.io/managed-by": "zabbix-operator",
	}
}

/*************  ✨ Codeium Command ⭐  *************/
// buildServerContainer builds a Container for the Zabbix server component.
//
// The returned Container runs the Zabbix server image and exposes the Zabbix
// server port on the container port named "zabbix-server".
//
// The container environment variables are configured using the
// buildServerEnvVars method.
/******  4400349c-bd24-48b6-aeff-1c5d5522f19d  *******/
func (r *ZabbixReconciler) buildServerContainer(zabbix *monitoringv1alpha1.Zabbix) corev1.Container {
	return corev1.Container{
		Name:  componentName,
		Image: zabbix.Spec.Server.Image,
		Ports: []corev1.ContainerPort{
			{
				Name:          serverPortName,
				ContainerPort: serverPort,
			},
		},
		Env: r.buildServerEnvVars(zabbix),
	}
}

func (r *ZabbixReconciler) buildServerEnvVars(zabbix *monitoringv1alpha1.Zabbix) []corev1.EnvVar {
	var envVars []corev1.EnvVar
	dbType := strings.ToUpper(zabbix.Spec.Database.Type)

	// Database credentials
	if zabbix.Spec.Database.Credentials.FromSecret != "" {
		envVars = append(envVars,
			corev1.EnvVar{
				Name: fmt.Sprintf("%s_USER", dbType),
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: zabbix.Spec.Database.Credentials.FromSecret,
						},
						Key: "username",
					},
				},
			},
			corev1.EnvVar{
				Name: fmt.Sprintf("%s_PASSWORD", dbType),
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: zabbix.Spec.Database.Credentials.FromSecret,
						},
						Key: "password",
					},
				},
			},
		)
	} else {
		envVars = append(envVars,
			corev1.EnvVar{
				Name:  fmt.Sprintf("%s_USER", dbType),
				Value: zabbix.Spec.Database.Credentials.Username,
			},
			corev1.EnvVar{
				Name:  fmt.Sprintf("%s_PASSWORD", dbType),
				Value: zabbix.Spec.Database.Credentials.Password,
			},
		)
	}

	// Add custom environment variables from spec
	for k, v := range zabbix.Spec.Server.Environment {
		envVars = append(envVars, corev1.EnvVar{
			Name:  k,
			Value: v,
		})
	}

	return envVars
}

func (r *ZabbixReconciler) updateStatus(ctx context.Context, zabbix *monitoringv1alpha1.Zabbix) error {
	status := monitoringv1alpha1.ZabbixStatus{
		Conditions: []metav1.Condition{
			{
				Type:               "Available",
				Status:             metav1.ConditionTrue,
				Reason:             "ComponentsReady",
				Message:            "All components are operational",
				LastTransitionTime: metav1.Now(),
			},
		},
	}

	zabbix.Status = status
	return r.Status().Update(ctx, zabbix)
}

func (r *ZabbixReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&monitoringv1alpha1.Zabbix{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}
