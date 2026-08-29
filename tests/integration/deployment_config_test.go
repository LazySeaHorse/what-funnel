package integration

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// ComposeConfig is a minimal structural mapping for docker-compose validation.
type ComposeConfig struct {
	Services map[string]struct {
		Image       string            `yaml:"image"`
		Build       any               `yaml:"build"`
		Ports       []string          `yaml:"ports"`
		Networks    []string          `yaml:"networks"`
		Environment map[string]string `yaml:"environment"`
	} `yaml:"services"`
	Networks map[string]struct {
		Driver   string `yaml:"driver"`
		Internal bool   `yaml:"internal"`
	} `yaml:"networks"`
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "docker-compose.yml")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not locate repository root")
		}
		dir = parent
	}
}

func TestProductionDockerComposeSecurityInvariants(t *testing.T) {
	root := findRepoRoot(t)
	prodComposePath := filepath.Join(root, "docker-compose.prod.yml")

	data, err := os.ReadFile(prodComposePath)
	require.NoError(t, err, "docker-compose.prod.yml must exist")

	var cfg ComposeConfig
	err = yaml.Unmarshal(data, &cfg)
	require.NoError(t, err, "docker-compose.prod.yml must be valid YAML")

	t.Run("Network isolation and internal data network", func(t *testing.T) {
		require.Contains(t, cfg.Networks, "data_net", "data_net network must be defined")
		require.Contains(t, cfg.Networks, "backend_net", "backend_net network must be defined")
		require.Contains(t, cfg.Networks, "public_net", "public_net network must be defined")
		assert.True(t, cfg.Networks["data_net"].Internal, "data_net must be internal: true to prevent direct outside routing")
	})

	t.Run("Zero host port leakage for internal services", func(t *testing.T) {
		internalServices := []string{
			"postgres",
			"redis",
			"identity-svc",
			"workspace-svc",
			"conversation-svc",
			"notification-svc",
			"ai-answer-svc",
			"ai-kb-compiler",
			"synapse",
		}

		for _, svcName := range internalServices {
			svc, ok := cfg.Services[svcName]
			if assert.True(t, ok, "service %s should exist in prod compose", svcName) {
				assert.Empty(t, svc.Ports, "service %s must NOT publish host ports in production", svcName)
			}
		}
	})

	t.Run("Only web ingress publishes host ports", func(t *testing.T) {
		webSvc, ok := cfg.Services["web"]
		require.True(t, ok, "web service must be defined in prod compose")
		assert.NotEmpty(t, webSvc.Ports, "web service should publish ingress port")
	})

	t.Run("Secrets are parameterized and not committed as hardcoded plain text", func(t *testing.T) {
		raw := string(data)
		assert.NotContains(t, raw, "POSTGRES_PASSWORD: whatfunnel", "hardcoded db password must not be present in prod compose")
		assert.NotContains(t, raw, "change-me-in-production", "dummy default session secret must not be hardcoded in prod compose")
		assert.NotContains(t, raw, "change-me-32-byte-hex-key-padded", "dummy default encryption key must not be hardcoded in prod compose")
		assert.NotContains(t, raw, "B4t@Ss,gB8^0gRoFBG*A", "hardcoded Matrix shared secret must not be present in prod compose")
	})

	t.Run("Environment template documents required variables", func(t *testing.T) {
		envExamplePath := filepath.Join(root, ".env.example")
		envData, err := os.ReadFile(envExamplePath)
		require.NoError(t, err, ".env.example must exist")

		content := string(envData)
		requiredVars := []string{
			"POSTGRES_USER",
			"POSTGRES_PASSWORD",
			"POSTGRES_DB",
			"SESSION_SECRET",
			"ENCRYPTION_KEY",
			"MATRIX_REGISTRATION_SHARED_SECRET",
			"ALLOWED_ORIGINS",
			"APP_ENV",
		}

		for _, v := range requiredVars {
			assert.Contains(t, content, v+"=", ".env.example must document %s", v)
		}
	})
}

func TestFrontendProductionBuildSetup(t *testing.T) {
	root := findRepoRoot(t)

	t.Run("Svelte config uses adapter-static with SPA fallback", func(t *testing.T) {
		svelteConfigPath := filepath.Join(root, "apps", "web", "svelte.config.js")
		data, err := os.ReadFile(svelteConfigPath)
		require.NoError(t, err, "svelte.config.js must exist")

		content := string(data)
		assert.Contains(t, content, "@sveltejs/adapter-static", "must import adapter-static")
		assert.Contains(t, content, "fallback: 'index.html'", "must specify fallback index.html for SPA routing")
	})

	t.Run("Root layout specifies SPA rendering options", func(t *testing.T) {
		layoutTsPath := filepath.Join(root, "apps", "web", "src", "routes", "+layout.ts")
		data, err := os.ReadFile(layoutTsPath)
		require.NoError(t, err, "+layout.ts must exist")

		content := string(data)
		assert.True(t, regexp.MustCompile(`export\s+const\s+ssr\s*=\s*false`).MatchString(content), "ssr must be false")
		assert.True(t, regexp.MustCompile(`export\s+const\s+prerender\s*=\s*false`).MatchString(content), "prerender must be false")
	})

	t.Run("Frontend Dockerfile uses multi-stage Nginx build", func(t *testing.T) {
		dockerfilePath := filepath.Join(root, "apps", "web", "Dockerfile")
		data, err := os.ReadFile(dockerfilePath)
		require.NoError(t, err, "apps/web/Dockerfile must exist")

		content := string(data)
		assert.Contains(t, strings.ToLower(content), "from node:", "must have node build stage")
		assert.Contains(t, strings.ToLower(content), "from nginx:", "must have nginx runtime stage")
		assert.Contains(t, content, "/usr/share/nginx/html", "must copy static output to nginx html directory")
	})

	t.Run("Nginx configuration proxies API and WebSocket with upgrade headers", func(t *testing.T) {
		nginxConfPath := filepath.Join(root, "apps", "web", "nginx.conf")
		data, err := os.ReadFile(nginxConfPath)
		require.NoError(t, err, "apps/web/nginx.conf must exist")

		content := string(data)
		assert.Contains(t, content, "location /api-gateway/", "must proxy /api-gateway/")
		assert.Contains(t, content, "location /ws", "must handle /ws endpoint")
		assert.Contains(t, content, "proxy_set_header Upgrade $http_upgrade;", "must include WebSocket upgrade header")
		assert.Contains(t, content, "try_files $uri $uri/ /index.html;", "must include SPA route fallback")
	})
}
