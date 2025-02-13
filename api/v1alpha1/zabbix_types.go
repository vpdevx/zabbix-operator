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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ZabbixSpec struct {
	Server ZabbixServer `json:"server,omitempty"`
	Agent  ZabbixAgent  `json:"agent,omitempty"`
	Web    ZabbixWeb    `json:"web,omitempty"`

	Database ZabbixDatabase `json:"database,omitempty"`
}

type ZabbixServer struct {
	// +kubebuilder:validation:Required
	Image       string                      `json:"image"`
	Environment map[string]string           `json:"environment,omitempty"`
	Resources   corev1.ResourceRequirements `json:"resources,omitempty"`
}

type ZabbixAgent struct {
	// +kubebuilder:validation:Required
	Image       string                      `json:"image"`
	Environment map[string]string           `json:"environment,omitempty"`
	Resources   corev1.ResourceRequirements `json:"resources,omitempty"`
}

type ZabbixWeb struct {
	// +kubebuilder:validation:Required
	Image       string                      `json:"image"`
	Environment map[string]string           `json:"environment,omitempty"`
	Resources   corev1.ResourceRequirements `json:"resources,omitempty"`
	Ingress     IngressSpec                 `json:"ingress,omitempty"`
}

type IngressSpec struct {
	Host        string            `json:"host"`
	Annotations map[string]string `json:"annotations,omitempty"`
	ClassName   string            `json:"className,omitempty"`
	Tls         IngressTLS        `json:"tls,omitempty"`
	Path        string            `json:"path,omitempty"`
	PathType    string            `json:"pathType,omitempty"`
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=true;false
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`
}

type IngressTLS struct {
	// +kubebuilder:validation:Required
	Enabled    bool     `json:"enabled,omitempty"`
	SecretName string   `json:"secretName,omitempty"`
	Hosts      []string `json:"hosts,omitempty"`
}

type ZabbixDatabase struct {
	// +kubebuilder:validation:Required
	Credentials ZabbixCredentials `json:"credentials,omitempty"`

	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=postgres;mysql
	Type string `json:"type"`
}

type ZabbixCredentials struct {
	Username string `json:"username,omitempty"`

	Password string `json:"password,omitempty"`

	FromSecret string `json:"fromSecret,omitempty"`
}

// ZabbixStatus defines the observed state of Zabbix
type ZabbixStatus struct {
	Status     string             `json:"status,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type Zabbix struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ZabbixSpec   `json:"spec,omitempty"`
	Status ZabbixStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

type ZabbixList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Zabbix `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Zabbix{}, &ZabbixList{})
}
