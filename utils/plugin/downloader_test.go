package plugin

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/argoproj/argo-rollouts/utils/config"

	"github.com/tj/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/argoproj/argo-rollouts/utils/defaults"
)

type MockFileDownloader struct {
	FileDownloader
}

func (m MockFileDownloader) Get(url string, header http.Header) (*http.Response, error) {
	if url == "https://test/plugin/fail" {
		return &http.Response{
			Status:        "404",
			StatusCode:    404,
			Proto:         "HTTP/1.1",
			ProtoMajor:    1,
			ProtoMinor:    1,
			Header:        nil,
			ContentLength: 4,
		}, nil
	}
	responseBody := io.NopCloser(bytes.NewReader([]byte(`test`)))
	return &http.Response{
		Status:        "200",
		StatusCode:    200,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        nil,
		Body:          responseBody,
		ContentLength: 4,
	}, nil
}

func TestPlugin(t *testing.T) {
	t.Run("try to get config without being initialized", func(t *testing.T) {
		_, err := config.GetConfig()
		assert.Error(t, err)
	})

	t.Run("test initializing and downloading plugins successfully", func(t *testing.T) {
		cm := &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "argo-rollouts-config",
				Namespace: "argo-rollouts",
			},
			Data: map[string]string{"metricProviderPlugins": "\n  - name: argoproj-labs/http\n    location: https://test/plugin\n  - name: argoproj-labs/http-sha\n    location: https://test/plugin\n    sha256: 74657374e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855\n  - name: argoproj-labs/http-sha-correct\n    location: https://test/plugin\n    sha256: 9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08\n  - name: argoproj-labs/http-headers-correct\n    location: https://test/plugin\n    headersFrom:\n      - secretRef:\n          name: secret-name"},
		}
		secret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "secret-name",
				Namespace: "argo-rollouts",
			},
			Data: map[string][]byte{"Authorization": []byte("Basic VE9LRU4=")},
		}
		client := fake.NewSimpleClientset(cm, secret)

		config.UnInitializeConfig()

		_, err := config.InitializeConfig(client, "argo-rollouts-config")
		assert.NoError(t, err)

		err = DownloadPlugins(MockFileDownloader{}, client)
		assert.NoError(t, err)

		dir, filename, err := config.GetPluginDirectoryAndFilename("argoproj-labs/http")
		assert.NoError(t, err)

		err = os.Remove(filepath.Join(defaults.DefaultRolloutPluginFolder, dir, filename))
		assert.NoError(t, err)

		dir, filename, err = config.GetPluginDirectoryAndFilename("argoproj-labs/http-sha")
		assert.NoError(t, err)

		err = os.Remove(filepath.Join(defaults.DefaultRolloutPluginFolder, dir, filename))
		assert.NoError(t, err)

		dir, filename, err = config.GetPluginDirectoryAndFilename("argoproj-labs/http-headers-correct")
		assert.NoError(t, err)

		err = os.Remove(filepath.Join(defaults.DefaultRolloutPluginFolder, dir, filename))
		assert.NoError(t, err)
		err = os.RemoveAll(defaults.DefaultRolloutPluginFolder)
		assert.NoError(t, err)
	})

	t.Run("test failed download", func(t *testing.T) {
		cm := &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "argo-rollouts-config",
				Namespace: "argo-rollouts",
			},
			Data: map[string]string{"metricProviderPlugins": "\n  - name: argoproj-labs/http-fail\n    location: https://test/plugin/fail\n"},
		}
		client := fake.NewSimpleClientset(cm)

		config.UnInitializeConfig()

		_, err := config.InitializeConfig(client, "argo-rollouts-config")
		assert.NoError(t, err)

		err = DownloadPlugins(MockFileDownloader{}, client)
		assert.Error(t, err)
		err = os.RemoveAll(defaults.DefaultRolloutPluginFolder)
		assert.NoError(t, err)
	})

	t.Run("test failed finding secret for header", func(t *testing.T) {
		cm := &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "argo-rollouts-config",
				Namespace: "argo-rollouts",
			},
			Data: map[string]string{"metricProviderPlugins": "  - name: argoproj-labs/http-headers-correct\n    location: https://test/plugin\n    headersFrom:\n      - secretRef:\n          name: secret-name"},
		}
		client := fake.NewSimpleClientset(cm)

		config.UnInitializeConfig()

		_, err := config.InitializeConfig(client, "argo-rollouts-config")
		assert.NoError(t, err)

		err = DownloadPlugins(MockFileDownloader{}, client)
		assert.Error(t, err)
		err = os.RemoveAll(defaults.DefaultRolloutPluginFolder)
		assert.NoError(t, err)
	})

	t.Run("test bad sha", func(t *testing.T) {
		cm := &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      defaults.DefaultRolloutsConfigMapName,
				Namespace: defaults.Namespace(),
			},
			Data: map[string]string{"metricProviderPlugins": "\n  - name: argoproj-labs/http-badsha\n    location: https://test/plugin\n    sha256: badsha352"},
		}
		client := fake.NewSimpleClientset(cm)

		config.UnInitializeConfig()

		_, err := config.InitializeConfig(client, defaults.DefaultRolloutsConfigMapName)
		assert.NoError(t, err)

		err = DownloadPlugins(MockFileDownloader{}, client)
		assert.Error(t, err)

		dir, filename, err := config.GetPluginDirectoryAndFilename("argoproj-labs/http-badsha")
		assert.NoError(t, err)
		err = os.Remove(filepath.Join(defaults.DefaultRolloutPluginFolder, dir, filename))
		assert.NoError(t, err)
		err = os.RemoveAll(defaults.DefaultRolloutPluginFolder)
		assert.NoError(t, err)
	})

	t.Run("test plugin initialization with no configmap found", func(t *testing.T) {
		client := fake.NewSimpleClientset()

		config.UnInitializeConfig()

		cm, err := config.InitializeConfig(client, defaults.DefaultRolloutsConfigMapName)
		assert.NoError(t, err)
		assert.Equal(t, 0, len(cm.GetAllPlugins()))

		err = DownloadPlugins(MockFileDownloader{}, client)
		assert.NoError(t, err)
		err = os.RemoveAll(defaults.DefaultRolloutPluginFolder)
		assert.NoError(t, err)
	})

	t.Run("test moving file to plugin location", func(t *testing.T) {
		cm := &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      defaults.DefaultRolloutsConfigMapName,
				Namespace: defaults.Namespace(),
			},
			Data: map[string]string{"metricProviderPlugins": "\n  - name: argoproj-labs/file-plugin\n    location: file://./plugin.go"},
		}
		client := fake.NewSimpleClientset(cm)

		config.UnInitializeConfig()

		_, err := config.InitializeConfig(client, defaults.DefaultRolloutsConfigMapName)
		assert.NoError(t, err)

		err = DownloadPlugins(MockFileDownloader{}, client)
		assert.NoError(t, err)

		dir, filename, err := config.GetPluginDirectoryAndFilename("argoproj-labs/file-plugin")
		assert.NoError(t, err)
		err = os.Remove(filepath.Join(defaults.DefaultRolloutPluginFolder, dir, filename))
		assert.NoError(t, err)
		err = os.RemoveAll(defaults.DefaultRolloutPluginFolder)
		assert.NoError(t, err)
	})

	t.Run("test initialzing the config system twice", func(t *testing.T) {
		cm := &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      defaults.DefaultRolloutsConfigMapName,
				Namespace: defaults.Namespace(),
			},
			Data: map[string]string{"metricProviderPlugins": "\n  - name: namespace/file-plugin\n    location: file://./plugin.go"},
		}
		client := fake.NewSimpleClientset(cm)

		config.UnInitializeConfig()

		_, err := config.InitializeConfig(client, defaults.DefaultRolloutsConfigMapName)
		assert.NoError(t, err)

		_, err = config.InitializeConfig(client, defaults.DefaultRolloutsConfigMapName)
		assert.NoError(t, err)

		err = DownloadPlugins(MockFileDownloader{}, client)
		assert.NoError(t, err)

		dir, filename, err := config.GetPluginDirectoryAndFilename("namespace/file-plugin")
		assert.NoError(t, err)

		err = os.Remove(filepath.Join(defaults.DefaultRolloutPluginFolder, dir, filename))
		assert.NoError(t, err)
		err = os.RemoveAll(defaults.DefaultRolloutPluginFolder)
		assert.NoError(t, err)
	})

	t.Run("test a maformed configmap", func(t *testing.T) {
		cm := &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "argo-rollouts-config",
				Namespace: "argo-rollouts",
			},
			Data: map[string]string{"trafficRouterPlugins": "badconfigmap"},
		}
		client := fake.NewSimpleClientset(cm)

		config.UnInitializeConfig()

		_, err := config.InitializeConfig(client, "argo-rollouts-config")
		assert.Error(t, err)

		err = DownloadPlugins(MockFileDownloader{}, client)
		assert.Error(t, err)
	})

	t.Run("test malformed pluginLocation", func(t *testing.T) {
		cm := &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "argo-rollouts-config",
				Namespace: "argo-rollouts",
			},
			Data: map[string]string{"metricProviderPlugins": "\n  - name: argoproj-labs/http\n    location: agwegasdlkjf2324"},
		}
		client := fake.NewSimpleClientset(cm)

		config.UnInitializeConfig()

		_, err := config.InitializeConfig(client, "argo-rollouts-config")
		assert.NoError(t, err)

		err = DownloadPlugins(MockFileDownloader{}, client)
		assert.Error(t, err)

		err = os.RemoveAll(defaults.DefaultRolloutPluginFolder)
		assert.NoError(t, err)
	})
}

func TestCheckPluginExits(t *testing.T) {
	t.Run("test that non existing files on the fs return error", func(t *testing.T) {
		err := checkPluginExists("nonexistentplugin")
		assert.Error(t, err)
	})

	t.Run("test that if a file exists on the fs we dont error", func(t *testing.T) {
		realfile, err := filepath.Abs("plugin.go")
		assert.NoError(t, err)
		err = checkPluginExists(realfile)
		assert.NoError(t, err)
	})
}

func TestCheckShaOfPlugin(t *testing.T) {
	t.Run("test sha of non existing file", func(t *testing.T) {
		_, err := checkShaOfPlugin("nonexistentplugin", "")
		assert.Error(t, err)
	})

	t.Run("test sha of real file", func(t *testing.T) {
		os.WriteFile("test-sha", []byte("test"), 0700)
		realfile, err := filepath.Abs("test-sha")
		assert.NoError(t, err)

		shaNotValid, err := checkShaOfPlugin(realfile, "")
		assert.NoError(t, err)
		assert.Equal(t, false, shaNotValid)

		shaValid, err := checkShaOfPlugin(realfile, "74657374e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")
		assert.NoError(t, err)
		assert.Equal(t, true, shaValid)

		os.Remove("test-sha")
	})
}

func TestDownloadFile(t *testing.T) {
	t.Run("test sha of real file", func(t *testing.T) {
		err := downloadFile("error", " ", FileDownloaderImpl{}, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to download file from")
	})

	t.Run("test download fail with invalid url", func(t *testing.T) {
		url := "://example.com"
		err := downloadFile("error", url, FileDownloaderImpl{}, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to download file from")
	})
}

func Test_copyFile(t *testing.T) {
	t.Run("test copy file that does not exist", func(t *testing.T) {
		err := copyFile("nonexistentfile", "nonexistentfile")
		assert.Error(t, err)
	})

	t.Run("test copy file that does exist", func(t *testing.T) {
		err := os.WriteFile("test-copy", []byte("test"), 0700)
		assert.NoError(t, err)
		err = copyFile("test-copy", "test-copy")
		defer func() {
			err = os.Remove("test-copy")
			assert.NoError(t, err)
		}()

		assert.NoError(t, err)
		assert.FileExists(t, "test-copy")
	})
}
