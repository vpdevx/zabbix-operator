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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	monitoringv1alpha1 "github.com/vpdevx/zabbix-operator/api/v1alpha1"
)

// ZabbixReconciler reconciles a Zabbix object
type ZabbixReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const finalizerName = "zabbix.io/zabbix_controller_finalizer"

// +kubebuilder:rbac:groups=monitoring.zabbix.io,resources=zabbixes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.zabbix.io,resources=zabbixes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=monitoring.zabbix.io,resources=zabbixes/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete

func (r *ZabbixReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("zabbix", req.NamespacedName)

	zabbix := &monitoringv1alpha1.Zabbix{}
	if err := r.Get(ctx, req.NamespacedName, zabbix); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "failed to get Zabbix resource")
		return ctrl.Result{}, err
	}

	if !controllerutil.ContainsFinalizer(zabbix, finalizerName) {
		logger.Info("adding finalizer")
		controllerutil.AddFinalizer(zabbix, finalizerName)
		return ctrl.Result{}, r.Update(ctx, zabbix)
	}

	if !zabbix.DeletionTimestamp.IsZero() {
		logger.Info("deleting Zabbix resource")
		return r.reconcileDelete(ctx, zabbix)
	}

	logger.Info("Creating Zabbix resource")
	return r.reconcileCreate(ctx, zabbix)
}

func (r *ZabbixReconciler) reconcileDelete(ctx context.Context, zabbix *monitoringv1alpha1.Zabbix) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Deleting Zabbix resource")
	controllerutil.RemoveFinalizer(zabbix, finalizerName)
	err := r.Update(ctx, zabbix)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
	}
	return ctrl.Result{}, nil
}

func (r *ZabbixReconciler) reconcileCreate(ctx context.Context, zabbix *monitoringv1alpha1.Zabbix) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Creating Zabbix Server")
	err := r.createOrUpdateZabbixServer(ctx, zabbix)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.createService(ctx, zabbix, "server")
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *ZabbixReconciler) createOrUpdateZabbixServer(ctx context.Context, zabbix *monitoringv1alpha1.Zabbix) error {
	var deployment appsv1.Deployment
	deploymentName := types.NamespacedName{Name: zabbix.ObjectMeta.Name + "-server", Namespace: zabbix.ObjectMeta.Namespace}
	if err := r.Get(ctx, deploymentName, &deployment); err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to fetch deployment: %w", err)
		}

		if apierrors.IsNotFound(err) {
			deployment := appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      zabbix.ObjectMeta.Name + "-server",
					Namespace: zabbix.ObjectMeta.Namespace,
					Labels:    r.componentLabels(zabbix, "server"),
					// Fix zabbix_types.go to support annotations
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: zabbix.APIVersion,
							Kind:       zabbix.Kind,
							Name:       zabbix.Name,
							UID:        zabbix.UID,
						},
					},
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: r.componentLabels(zabbix, "server"),
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: r.componentLabels(zabbix, "server"),
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  zabbix.ObjectMeta.Name + "-server",
									Image: zabbix.Spec.Server.Image,
									Ports: []corev1.ContainerPort{
										{
											Name:          "server",
											ContainerPort: 10051,
										},
									},
									Env:       r.buildServerEnvVars(zabbix),
									Resources: zabbix.Spec.Server.Resources,
								},
							},
						},
					},
				},
			}
			err := r.Create(ctx, &deployment)
			if err != nil {
				return fmt.Errorf("failed to create zabbix server deployment: %w", err)
			}
			return nil
		}
	}

	deployment.Spec.Template.Spec.Containers[0].Image = zabbix.Spec.Server.Image
	deployment.Spec.Template.Spec.Containers[0].Env = r.buildServerEnvVars(zabbix)
	deployment.Spec.Template.Spec.Containers[0].Resources = zabbix.Spec.Server.Resources
	err := r.Update(ctx, &deployment)
	if err != nil {
		return fmt.Errorf("failed to update zabbix server deployment: %w", err)
	}

	return nil
}

func (r *ZabbixReconciler) createService(ctx context.Context, zabbix *monitoringv1alpha1.Zabbix, component string) error {

	var port int
	var portName string

	switch component {
	case "server":
		port = 10051
		portName = "server"
	case "web":
		port = 80
		portName = "http"
	}

	service := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      zabbix.ObjectMeta.Name + "-" + component,
			Namespace: zabbix.ObjectMeta.Namespace,
			Labels:    r.componentLabels(zabbix, component),
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: zabbix.APIVersion,
					Kind:       zabbix.Kind,
					Name:       zabbix.Name,
					UID:        zabbix.UID,
				},
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: r.componentLabels(zabbix, component),
			Ports: []corev1.ServicePort{
				{
					Name:       portName,
					Port:       int32(port),
					TargetPort: intstr.FromInt(port),
				},
			},
		},
		Status: corev1.ServiceStatus{},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, &service, func() error {
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create zabbix %s service: %w", component, err)
	}
	return nil
}

func (r *ZabbixReconciler) componentLabels(zabbix *monitoringv1alpha1.Zabbix, component string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       "zabbix",
		"app.kubernetes.io/instance":   zabbix.Name,
		"app.kubernetes.io/component":  component,
		"app.kubernetes.io/managed-by": "zabbix-operator",
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

func (r *ZabbixReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&monitoringv1alpha1.Zabbix{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}
