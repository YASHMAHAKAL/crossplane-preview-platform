package preview

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type GuestField struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type GuestResource struct {
	Kind      string       `json:"kind"`
	Name      string       `json:"name"`
	Namespace string       `json:"namespace,omitempty"`
	State     string       `json:"state"`
	Fields    []GuestField `json:"fields"`
}

type GuestInventory struct {
	Name      string          `json:"name"`
	HeadSHA   string          `json:"headSHA"`
	Resources []GuestResource `json:"resources"`
}

type GuestInventoryReader interface {
	Read(context.Context, string, string) (GuestInventory, error)
}

// GuestInventoryHandler accepts only a ready vCluster preview. It never returns
// a kubeconfig, token, Secret, or an unfiltered Kubernetes object to Backstage.
func (store Store) GuestInventoryHandler(statusClient *http.Client, reader GuestInventoryReader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, "/api/previews/") || !strings.HasSuffix(r.URL.Path, "/guest-resources") {
			http.NotFound(w, r)
			return
		}
		name := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/previews/"), "/guest-resources")
		status, err := store.Status(name, statusClient)
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "preview status unavailable", http.StatusBadRequest)
			return
		}
		if status.Phase != "ready" || status.Mode != "vcluster" {
			http.Error(w, "no ready vCluster preview", http.StatusConflict)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
		defer cancel()
		inventory, err := reader.Read(ctx, name, status.HeadSHA)
		if err != nil {
			http.Error(w, "virtual cluster inventory unavailable", http.StatusServiceUnavailable)
			return
		}
		// Closing or updating a PR while a guest read is in flight must not expose
		// resources from an old head after the status page stops showing its URL.
		latest, err := store.Status(name, statusClient)
		if err != nil || latest.Phase != "ready" || latest.Mode != "vcluster" || latest.HeadSHA != status.HeadSHA {
			http.Error(w, "preview changed during inventory read", http.StatusConflict)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(inventory)
	})
}

type KubectlGuestInventoryReader struct {
	Context string
}

type guestKubeconfig struct {
	Clusters []struct {
		Cluster struct {
			Server string `yaml:"server"`
			CAData string `yaml:"certificate-authority-data"`
		} `yaml:"cluster"`
	} `yaml:"clusters"`
}

type guestAPIError struct{ code int }

func (err guestAPIError) Error() string { return fmt.Sprintf("guest API returned HTTP %d", err.code) }

func (reader KubectlGuestInventoryReader) Read(ctx context.Context, name, headSHA string) (GuestInventory, error) {
	if _, err := (Store{}).previewDir(name); err != nil || reader.Context == "" {
		return GuestInventory{}, errors.New("invalid virtual cluster request")
	}
	secretJSON, err := exec.CommandContext(ctx, "kubectl", "--context", reader.Context, "-n", name,
		"get", "secret", "vc-"+name, "-o", "json").Output()
	if err != nil {
		return GuestInventory{}, errors.New("vCluster connection is unavailable")
	}
	var secret struct {
		Data map[string]string `json:"data"`
	}
	if err := json.Unmarshal(secretJSON, &secret); err != nil {
		return GuestInventory{}, errors.New("invalid vCluster connection Secret")
	}
	configBytes, err := base64.StdEncoding.DecodeString(secret.Data["config"])
	if err != nil {
		return GuestInventory{}, errors.New("invalid vCluster kubeconfig encoding")
	}
	var kubeconfig guestKubeconfig
	if err := yaml.Unmarshal(configBytes, &kubeconfig); err != nil || len(kubeconfig.Clusters) != 1 || kubeconfig.Clusters[0].Cluster.Server != "https://localhost:8443" {
		return GuestInventory{}, errors.New("unexpected vCluster kubeconfig endpoint")
	}
	caBytes, err := base64.StdEncoding.DecodeString(kubeconfig.Clusters[0].Cluster.CAData)
	if err != nil {
		return GuestInventory{}, errors.New("invalid vCluster certificate authority")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caBytes) {
		return GuestInventory{}, errors.New("invalid vCluster CA certificate")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return GuestInventory{}, err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	configBytes = []byte(strings.Replace(string(configBytes), "https://localhost:8443", "https://localhost:"+strconv.Itoa(port), 1))
	configFile, err := os.CreateTemp("", "preview-guest-inventory-*.yaml")
	if err != nil {
		return GuestInventory{}, err
	}
	defer os.Remove(configFile.Name())
	defer configFile.Close()
	if _, err := configFile.Write(configBytes); err != nil {
		return GuestInventory{}, err
	}
	forward := exec.CommandContext(ctx, "kubectl", "--context", reader.Context, "-n", name,
		"port-forward", "--address=127.0.0.1", "pod/"+name+"-0", fmt.Sprintf("%d:8443", port))
	forward.Stdout, forward.Stderr = io.Discard, io.Discard
	if err := forward.Start(); err != nil {
		return GuestInventory{}, errors.New("vCluster API port forward failed")
	}
	defer func() {
		_ = forward.Process.Kill()
		_ = forward.Wait()
	}()
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	connected := false
	for attempt := 0; attempt < 60; attempt++ {
		connection, dialErr := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if dialErr == nil {
			_ = connection.Close()
			connected = true
			break
		}
		select {
		case <-ctx.Done():
			return GuestInventory{}, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	if !connected {
		return GuestInventory{}, errors.New("vCluster API did not become reachable")
	}
	// The host Secret contains a virtual admin kubeconfig. Use it only for a
	// short-lived TokenRequest; inventory GETs use the fixed read-only role.
	tokenBytes, err := exec.CommandContext(ctx, "kubectl", "--kubeconfig", configFile.Name(), "--request-timeout=5s",
		"-n", "default", "create", "token", "preview-observer", "--duration=5m").Output()
	if err != nil || strings.TrimSpace(string(tokenBytes)) == "" {
		return GuestInventory{}, errors.New("vCluster observer token is unavailable")
	}
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}}
	base := "https://localhost:" + strconv.Itoa(port)
	read := func(path string) (map[string]any, error) {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
		if err != nil {
			return nil, err
		}
		request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(tokenBytes)))
		response, err := client.Do(request)
		if err != nil {
			return nil, err
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return nil, guestAPIError{code: response.StatusCode}
		}
		var object map[string]any
		if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&object); err != nil {
			return nil, err
		}
		return object, nil
	}
	inventory := GuestInventory{Name: name, HeadSHA: headSHA, Resources: []GuestResource{}}
	selector := "?labelSelector=preview.platform.example.org%2Fservice%3Dincident-tracker"
	for _, path := range []string{"/api/v1/pods", "/apis/apps/v1/deployments", "/api/v1/services", "/apis/networking.k8s.io/v1/ingresses"} {
		object, err := read(path + selector)
		if err != nil {
			return GuestInventory{}, err
		}
		for _, item := range arrayAt(object, "items") {
			resource, ok := item.(map[string]any)
			if !ok || stringAt(resource, "metadata", "labels", "preview.platform.example.org/head-sha") != headSHA {
				continue
			}
			inventory.Resources = append(inventory.Resources, guestSummary(resource))
		}
	}
	crd, err := read("/apis/apiextensions.k8s.io/v1/customresourcedefinitions/incidentpolicies.incidents.demo.local")
	var apiError guestAPIError
	if err != nil && (!errors.As(err, &apiError) || apiError.code != http.StatusNotFound) {
		return GuestInventory{}, err
	}
	if err == nil {
		inventory.Resources = append(inventory.Resources, guestSummary(crd))
		policies, err := read("/apis/incidents.demo.local/v1alpha1/incidentpolicies")
		if err != nil {
			return GuestInventory{}, err
		}
		for _, item := range arrayAt(policies, "items") {
			if resource, ok := item.(map[string]any); ok {
				inventory.Resources = append(inventory.Resources, guestSummary(resource))
			}
		}
	}
	sort.Slice(inventory.Resources, func(i, j int) bool {
		a, b := inventory.Resources[i], inventory.Resources[j]
		return a.Kind+a.Namespace+a.Name < b.Kind+b.Namespace+b.Name
	})
	return inventory, nil
}

func valueAt(object map[string]any, keys ...string) any {
	var current any = object
	for _, key := range keys {
		next, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = next[key]
	}
	return current
}

func stringAt(object map[string]any, keys ...string) string {
	value, _ := valueAt(object, keys...).(string)
	return value
}

func arrayAt(object map[string]any, keys ...string) []any {
	value, _ := valueAt(object, keys...).([]any)
	return value
}

func guestSummary(object map[string]any) GuestResource {
	resource := GuestResource{Kind: stringAt(object, "kind"), Name: stringAt(object, "metadata", "name"),
		Namespace: stringAt(object, "metadata", "namespace"), State: "Present", Fields: []GuestField{}}
	add := func(name, value string) {
		if value != "" {
			resource.Fields = append(resource.Fields, GuestField{Name: name, Value: value})
		}
	}
	switch resource.Kind {
	case "Pod":
		resource.State = stringAt(object, "status", "phase")
		for _, condition := range arrayAt(object, "status", "conditions") {
			entry, _ := condition.(map[string]any)
			if entry["type"] == "Ready" && entry["status"] == "True" {
				resource.State = "Ready"
			}
		}
		for _, container := range arrayAt(object, "spec", "containers") {
			entry, _ := container.(map[string]any)
			add("Image", stringAt(entry, "image"))
		}
	case "Deployment":
		desired := valueAt(object, "spec", "replicas")
		ready := valueAt(object, "status", "readyReplicas")
		resource.State = fmt.Sprintf("%v/%v ready", numberOrZero(ready), numberOrZero(desired))
		add("Replicas", fmt.Sprint(numberOrZero(desired)))
		for _, container := range arrayAt(object, "spec", "template", "spec", "containers") {
			entry, _ := container.(map[string]any)
			add("Image", stringAt(entry, "image"))
		}
	case "Service":
		resource.State = stringAt(object, "spec", "type")
		add("Cluster IP", stringAt(object, "spec", "clusterIP"))
	case "Ingress":
		var hosts []string
		for _, rule := range arrayAt(object, "spec", "rules") {
			entry, _ := rule.(map[string]any)
			if host := stringAt(entry, "host"); host != "" {
				hosts = append(hosts, host)
			}
		}
		add("Hosts", strings.Join(hosts, ", "))
	case "CustomResourceDefinition":
		for _, condition := range arrayAt(object, "status", "conditions") {
			entry, _ := condition.(map[string]any)
			if entry["type"] == "Established" && entry["status"] == "True" {
				resource.State = "Established"
			}
		}
		add("Scope", stringAt(object, "spec", "scope"))
		for _, version := range arrayAt(object, "spec", "versions") {
			entry, _ := version.(map[string]any)
			add("Version", stringAt(entry, "name"))
			var severities []string
			for _, severity := range arrayAt(entry, "schema", "openAPIV3Schema", "properties", "spec", "properties", "severity", "enum") {
				if value, ok := severity.(string); ok {
					severities = append(severities, value)
				}
			}
			add("Severities", strings.Join(severities, ", "))
		}
	case "IncidentPolicy":
		add("Severity", stringAt(object, "spec", "severity"))
	}
	return resource
}

func numberOrZero(value any) int {
	number, _ := value.(float64)
	return int(number)
}
