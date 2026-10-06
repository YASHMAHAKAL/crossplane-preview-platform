package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

type PreviewXR struct {
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Spec struct {
		Crossplane struct {
			CompositionRef struct {
				Name string `json:"name"`
			} `json:"compositionRef"`
		} `json:"crossplane"`
		ServiceRef string `json:"serviceRef"`
		PR         struct {
			Number  int    `json:"number"`
			HeadSHA string `json:"headSHA"`
		} `json:"pr"`
		Image struct {
			Digest string `json:"digest"`
		} `json:"image"`
		Request struct {
			Size       string `json:"size"`
			TTLMinutes int    `json:"ttlMinutes"`
		} `json:"request"`
		Preview struct {
			Host string `json:"host"`
		} `json:"preview"`
		Decision struct {
			Mode        string   `json:"mode"`
			ReasonCodes []string `json:"reasonCodes"`
		} `json:"decision"`
		Capabilities []string `json:"capabilities"`
	} `json:"spec"`
}

type NamedResource struct {
	Name   string
	Object map[string]any
}

var (
	nameRE  = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	shaRE   = regexp.MustCompile(`^[a-f0-9]{40}$`)
	imageRE = regexp.MustCompile(`^ghcr\.io/[a-z0-9._/-]+@sha256:[a-f0-9]{64}$`)
)

func obj(parts map[string]any) map[string]any { return parts }

func Render(xr PreviewXR, mode string) ([]NamedResource, error) {
	name := xr.Metadata.Name
	if !nameRE.MatchString(name) || !shaRE.MatchString(xr.Spec.PR.HeadSHA) || xr.Spec.PR.Number < 1 ||
		!imageRE.MatchString(xr.Spec.Image.Digest) || xr.Spec.Preview.Host != name+".localhost" ||
		xr.Spec.Decision.Mode != mode || xr.Spec.Crossplane.CompositionRef.Name != "preview-"+mode ||
		(xr.Spec.Request.Size != "small" && xr.Spec.Request.Size != "medium") ||
		xr.Spec.Request.TTLMinutes < 5 || xr.Spec.Request.TTLMinutes > 240 {
		return nil, errors.New("XR fields do not match the selected preview composition")
	}
	if mode != "namespace" && mode != "vcluster" {
		return nil, errors.New("unsupported preview mode")
	}
	if mode == "namespace" && len(xr.Spec.Capabilities) != 0 {
		return nil, errors.New("namespace preview cannot carry cluster capabilities")
	}
	for _, capability := range xr.Spec.Capabilities {
		if capability != "incident-policy" || mode != "vcluster" {
			return nil, fmt.Errorf("unsupported capability %q", capability)
		}
	}
	labels := map[string]any{
		"preview.platform.example.org/service":  xr.Spec.ServiceRef,
		"preview.platform.example.org/pr":       fmt.Sprint(xr.Spec.PR.Number),
		"preview.platform.example.org/head-sha": xr.Spec.PR.HeadSHA,
		"app.kubernetes.io/managed-by":          "crossplane",
	}
	namespace := NamedResource{"namespace", obj(map[string]any{
		"apiVersion": "v1", "kind": "Namespace", "metadata": obj(map[string]any{"name": name, "labels": labels}),
	})}
	app := appObjects(xr, mode == "namespace", labels)
	if mode == "namespace" {
		cpu, memory := "500m", "512Mi"
		if xr.Spec.Request.Size == "medium" {
			cpu, memory = "1000m", "1Gi"
		}
		quota := NamedResource{"quota", obj(map[string]any{
			"apiVersion": "v1", "kind": "ResourceQuota",
			"metadata": obj(map[string]any{"name": "preview-quota", "namespace": name, "labels": labels}),
			"spec":     obj(map[string]any{"hard": obj(map[string]any{"requests.cpu": cpu, "requests.memory": memory, "pods": "4"})}),
		})}
		return []NamedResource{namespace, quota, {"deployment", app[0]}, {"service", app[1]}, {"ingress", app[2]}}, nil
	}

	documents := []map[string]any{app[0], app[1], app[2]}
	if len(xr.Spec.Capabilities) > 0 {
		documents = append([]map[string]any{incidentPolicyCRD()}, documents...)
	}
	var manifestParts []string
	for _, doc := range documents {
		encoded, err := json.Marshal(doc) // JSON is valid YAML for vCluster's manifests field.
		if err != nil {
			return nil, err
		}
		manifestParts = append(manifestParts, string(encoded))
	}
	release := NamedResource{"vcluster-release", obj(map[string]any{
		"apiVersion": "helm.crossplane.io/v1beta1", "kind": "Release",
		"metadata": obj(map[string]any{"name": name, "labels": labels}),
		"spec": obj(map[string]any{
			"providerConfigRef": obj(map[string]any{"name": "in-cluster"}),
			"forProvider": obj(map[string]any{
				"namespace": name, "skipCreateNamespace": true, "wait": true,
				"chart": obj(map[string]any{"name": "vcluster", "repository": "https://charts.loft.sh", "version": "0.36.0"}),
				"values": obj(map[string]any{
					"controlPlane": obj(map[string]any{"image": obj(map[string]any{"registry": "ghcr.io", "repository": "loft-sh/vcluster-oss"})}),
					"sync":         obj(map[string]any{"toHost": obj(map[string]any{"ingresses": obj(map[string]any{"enabled": true})})}),
					"experimental": obj(map[string]any{"deploy": obj(map[string]any{"vcluster": obj(map[string]any{"manifests": strings.Join(manifestParts, "\n---\n")})})}),
				}),
			}),
		}),
	})}
	return []NamedResource{namespace, release}, nil
}

func appObjects(xr PreviewXR, hostNamespace bool, labels map[string]any) []map[string]any {
	ns := xr.Metadata.Name
	if !hostNamespace {
		ns = "default"
	}
	cpu, memory := "100m", "128Mi"
	if xr.Spec.Request.Size == "medium" {
		cpu, memory = "250m", "256Mi"
	}
	metadata := func(name string) map[string]any {
		return obj(map[string]any{"name": name, "namespace": ns, "labels": labels})
	}
	selector := obj(map[string]any{"app": "incident-tracker"})
	deployment := obj(map[string]any{
		"apiVersion": "apps/v1", "kind": "Deployment", "metadata": metadata("incident-tracker"),
		"spec": obj(map[string]any{
			"replicas": 1, "selector": obj(map[string]any{"matchLabels": selector}),
			"template": obj(map[string]any{
				"metadata": obj(map[string]any{"labels": selector}),
				"spec": obj(map[string]any{
					"securityContext": obj(map[string]any{"runAsNonRoot": true, "runAsUser": 1000}),
					"containers": []any{obj(map[string]any{
						"name": "app", "image": xr.Spec.Image.Digest, "imagePullPolicy": "IfNotPresent",
						"ports": []any{obj(map[string]any{"name": "http", "containerPort": 8080})},
						"env": []any{
							obj(map[string]any{"name": "DATA_FILE", "value": "/data/incidents.json"}),
							obj(map[string]any{"name": "PREVIEW_MODE", "value": map[bool]string{true: "namespace", false: "vcluster"}[hostNamespace]}),
						},
						"volumeMounts":   []any{obj(map[string]any{"name": "data", "mountPath": "/data"})},
						"readinessProbe": obj(map[string]any{"httpGet": obj(map[string]any{"path": "/healthz", "port": 8080}), "initialDelaySeconds": 2}),
						"resources":      obj(map[string]any{"requests": obj(map[string]any{"cpu": cpu, "memory": memory}), "limits": obj(map[string]any{"cpu": "500m", "memory": "512Mi"})}),
					})},
					"volumes": []any{obj(map[string]any{"name": "data", "emptyDir": obj(map[string]any{})})},
				}),
			}),
		}),
	})
	service := obj(map[string]any{
		"apiVersion": "v1", "kind": "Service", "metadata": metadata("incident-tracker"),
		"spec": obj(map[string]any{"selector": selector, "ports": []any{obj(map[string]any{"name": "http", "port": 80, "targetPort": 8080})}}),
	})
	ingress := obj(map[string]any{
		"apiVersion": "networking.k8s.io/v1", "kind": "Ingress", "metadata": metadata("incident-tracker"),
		"spec": obj(map[string]any{
			"ingressClassName": "nginx",
			"rules": []any{obj(map[string]any{
				"host": xr.Spec.Preview.Host,
				"http": obj(map[string]any{"paths": []any{obj(map[string]any{
					"path": "/", "pathType": "Prefix",
					"backend": obj(map[string]any{"service": obj(map[string]any{"name": "incident-tracker", "port": obj(map[string]any{"number": 80})})}),
				})}}),
			})},
		}),
	})
	return []map[string]any{deployment, service, ingress}
}

func incidentPolicyCRD() map[string]any {
	return obj(map[string]any{
		"apiVersion": "apiextensions.k8s.io/v1", "kind": "CustomResourceDefinition",
		"metadata": obj(map[string]any{"name": "incidentpolicies.incidents.demo.local"}),
		"spec": obj(map[string]any{
			"group": "incidents.demo.local", "scope": "Namespaced",
			"names": obj(map[string]any{"kind": "IncidentPolicy", "plural": "incidentpolicies", "singular": "incidentpolicy"}),
			"versions": []any{obj(map[string]any{
				"name": "v1alpha1", "served": true, "storage": true,
				"schema": obj(map[string]any{"openAPIV3Schema": obj(map[string]any{
					"type": "object", "properties": obj(map[string]any{"spec": obj(map[string]any{
						"type": "object", "properties": obj(map[string]any{"severity": obj(map[string]any{"type": "string", "enum": []any{"low", "high"}})}),
					})}),
				})}),
			})},
		}),
	})
}
