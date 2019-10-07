/*
Copyright 2019
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
package options

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/diff"
	componentbaseconfig "k8s.io/component-base/config"
	kubeschedulerconfig "k8s.io/kubernetes/pkg/scheduler/apis/config"
)

func TestSchedulerOptions(t *testing.T) {
	// temp dir
	tmpDir, err := ioutil.TempDir("", "scheduler-options")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// https server
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`ok`))
	}))
	defer server.Close()

	// config file and kubeconfig
	configFile := filepath.Join(tmpDir, "scheduler.yaml")
	configKubeconfig := filepath.Join(tmpDir, "config.kubeconfig")
	if err := ioutil.WriteFile(configFile, []byte(fmt.Sprintf(`
apiVersion: kubescheduler.config.k8s.io/v1alpha1
kind: KubeSchedulerConfiguration
clientConnection:
  kubeconfig: "%s"`, configKubeconfig)), os.FileMode(0600)); err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile(configKubeconfig, []byte(fmt.Sprintf(`
apiVersion: v1
kind: Config
clusters:
- cluster:
    insecure-skip-tls-verify: true
    server: %s
  name: default
contexts:
- context:
    cluster: default
    user: default
  name: default
current-context: default
users:
- name: default
  user:
    username: config
`, server.URL)), os.FileMode(0600)); err != nil {
		t.Fatal(err)
	}

	invalidconfigFile := filepath.Join(tmpDir, "scheduler_invalid.yaml")
	if err := ioutil.WriteFile(invalidconfigFile, []byte(fmt.Sprintf(`
apiVersion: componentconfig/v1alpha2
kind: KubeSchedulerConfiguration
clientConnection:
  kubeconfig: "%s"`, configKubeconfig)), os.FileMode(0600)); err != nil {
		t.Fatal(err)
	}

	defaultSource := "DefaultProvider"
	defaultBindTimeoutSeconds := int64(600)

	testcases := []struct {
		name           string
		options        *Options
		expectedError  string
		expectedConfig kubeschedulerconfig.KubeSchedulerConfiguration
	}{
		{
			name: "config file",
			options: &Options{
				ConfigFile: configFile,
				ComponentConfig: func() kubeschedulerconfig.KubeSchedulerConfiguration {
					cfg, err := newDefaultComponentConfig()
					if err != nil {
						t.Fatal(err)
					}
					return *cfg
				}(),
			},
			expectedConfig: kubeschedulerconfig.KubeSchedulerConfiguration{
				SchedulerName:                  "default-scheduler",
				AlgorithmSource:                kubeschedulerconfig.SchedulerAlgorithmSource{Provider: &defaultSource},
				HardPodAffinitySymmetricWeight: 1,
				HealthzBindAddress:             "0.0.0.0:10251",
				MetricsBindAddress:             "0.0.0.0:10251",
				LeaderElection: kubeschedulerconfig.KubeSchedulerLeaderElectionConfiguration{
					LeaderElectionConfiguration: componentbaseconfig.LeaderElectionConfiguration{
						LeaderElect:       true,
						LeaseDuration:     metav1.Duration{Duration: 15 * time.Second},
						RenewDeadline:     metav1.Duration{Duration: 10 * time.Second},
						RetryPeriod:       metav1.Duration{Duration: 2 * time.Second},
						ResourceLock:      "endpoints",
						ResourceNamespace: "kube-system",
						ResourceName:      "kube-scheduler",
					},
				},
				ClientConnection: componentbaseconfig.ClientConnectionConfiguration{
					Kubeconfig:  configKubeconfig,
					QPS:         50,
					Burst:       100,
					ContentType: "application/vnd.kubernetes.protobuf",
				},
				BindTimeoutSeconds: &defaultBindTimeoutSeconds,
				Plugins:            nil,
			},
		},
		{
			name:          "invalid config file in componentconfig/v1alpha2",
			options:       &Options{ConfigFile: invalidconfigFile},
			expectedError: "no kind \"KubeSchedulerConfiguration\" is registered for version \"componentconfig/v1alpha2\"",
		},
		{
			name:          "no config",
			options:       &Options{},
			expectedError: "no configuration has been provided",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			// create the config
			config, err := tc.options.Config()

			// handle errors
			if err != nil {
				if tc.expectedError == "" {
					t.Error(err)
				} else if !strings.Contains(err.Error(), tc.expectedError) {
					t.Errorf("expected %q, got %q", tc.expectedError, err.Error())
				}
				return
			}

			if !reflect.DeepEqual(config.ComponentConfig, tc.expectedConfig) {
				t.Errorf("config.diff:\n%s", diff.ObjectReflectDiff(tc.expectedConfig, config.ComponentConfig))
			}

			// ensure we have a client
			if config.Client == nil {
				t.Error("unexpected nil client")
				return
			}
		})
	}
}
