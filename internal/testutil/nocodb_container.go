//go:build integration

// This file is compiled only under the `integration` build tag, so the
// testcontainers-go dependency never enters the default unit build. It starts a
// real NocoDB (pinned to a tag that exposes Meta API v3) and bootstraps a usable
// API token + base, the way an integration test needs to reach the tool's live
// integration boundary.
package testutil

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
)

// NocoDBImage is the pinned NocoDB image used by the integration suite. It must
// expose the Meta API v3 endpoints the tool calls.
const NocoDBImage = "nocodb/nocodb:2026.06.1"

// Container images for the external SQL backends.
const (
	mySQLImage    = "mysql:8"
	postgresImage = "postgres:16"
)

const (
	bootstrapEmail    = "admin@migrator.local"
	bootstrapPassword = "Password123!"
	// dbPassword is the root/superuser password for the external DB backends.
	// Kept alphanumeric so it needs no escaping inside the NC_DB URL.
	dbPassword = "ncpass1234"
)

// Backend selects the storage NocoDB runs on. SQLite is the bundled store (a
// single container); MySQL and Postgres are external SQL sources, each started
// as a second container on a shared network and wired via NC_DB.
type Backend string

const (
	SQLite   Backend = "sqlite"
	MySQL    Backend = "mysql"
	Postgres Backend = "postgres"
)

// NocoDB is a running NocoDB container plus the credentials a test feeds to the
// tool (NOCODB_URL / NOCODB_API_TOKEN / NOCODB_BASE_ID).
type NocoDB struct {
	URL    string
	Token  string
	BaseID string

	container testcontainers.Container
}

// StartNocoDB brings up a fresh NocoDB on the bundled SQLite store. It is a thin
// wrapper over StartNocoDBOn(t, SQLite).
func StartNocoDB(t *testing.T) *NocoDB {
	t.Helper()
	return StartNocoDBOn(t, SQLite)
}

// StartNocoDBOn brings up a fresh NocoDB on the given backend, waits for it to be
// ready, and bootstraps a super-admin, an API token, and an empty base. For the
// external SQL backends it first starts the DB container on a shared network and
// points NocoDB at it via NC_DB. It skips the test when Docker is unavailable.
func StartNocoDBOn(t *testing.T, backend Backend) *NocoDB {
	t.Helper()
	requireDocker(t)

	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        NocoDBImage,
		ExposedPorts: []string{"8080/tcp"},
		WaitingFor: wait.ForHTTP("/api/v1/version").
			WithPort("8080/tcp").
			WithStartupTimeout(180 * time.Second),
	}

	// External SQL backends: a shared network + a DB container reached by alias.
	if backend != SQLite {
		net, err := network.New(ctx)
		require.NoError(t, err, "create network")
		t.Cleanup(func() { _ = net.Remove(ctx) })

		alias := string(backend)
		startDB(t, ctx, backend, net.Name, alias)

		req.Networks = []string{net.Name}
		req.Env = map[string]string{"NC_DB": ncDBURL(backend, alias)}
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "start NocoDB container (%s)", backend)
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	host, err := container.Host(ctx)
	require.NoError(t, err)
	port, err := container.MappedPort(ctx, "8080/tcp")
	require.NoError(t, err)

	n := &NocoDB{
		URL:       fmt.Sprintf("http://%s:%s", host, port.Port()),
		container: container,
	}
	n.bootstrap(t)
	return n
}

// startDB starts the external SQL database for a backend on the shared network,
// reachable by the given alias, and waits for it to accept connections.
func startDB(t *testing.T, ctx context.Context, backend Backend, networkName, alias string) {
	t.Helper()

	var req testcontainers.ContainerRequest
	switch backend {
	case MySQL:
		req = testcontainers.ContainerRequest{
			Image:        mySQLImage,
			ExposedPorts: []string{"3306/tcp"},
			Env:          map[string]string{"MYSQL_ROOT_PASSWORD": dbPassword, "MYSQL_DATABASE": "nocodb"},
			WaitingFor: wait.ForLog("ready for connections").
				WithOccurrence(2).
				WithStartupTimeout(180 * time.Second),
		}
	case Postgres:
		req = testcontainers.ContainerRequest{
			Image:        postgresImage,
			ExposedPorts: []string{"5432/tcp"},
			Env:          map[string]string{"POSTGRES_PASSWORD": dbPassword, "POSTGRES_DB": "nocodb"},
			WaitingFor: wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(180 * time.Second),
		}
	default:
		t.Fatalf("startDB: unsupported backend %q", backend)
	}
	req.Networks = []string{networkName}
	req.NetworkAliases = map[string][]string{networkName: {alias}}

	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "start %s container", backend)
	t.Cleanup(func() { _ = c.Terminate(ctx) })
}

// ncDBURL builds the NC_DB connection string NocoDB uses to reach the external
// database by its network alias.
func ncDBURL(backend Backend, alias string) string {
	switch backend {
	case MySQL:
		return fmt.Sprintf("mysql2://%s:3306?u=root&p=%s&d=nocodb", alias, dbPassword)
	case Postgres:
		return fmt.Sprintf("pg://%s:5432?u=postgres&p=%s&d=nocodb", alias, dbPassword)
	default:
		return ""
	}
}

// bootstrap performs the version-specific auth dance, discovered against the
// pinned image (see the task log): signup the first user (becomes super admin),
// mint an API token, then create a base under the default workspace. Every
// management call after the token is minted uses the API token, exactly like the
// tool does.
func (n *NocoDB) bootstrap(t *testing.T) {
	t.Helper()

	// 1. Sign up the first user -> JWT (xc-auth).
	var signup struct {
		Token string `json:"token"`
	}
	n.do(t, http.MethodPost, "/api/v1/auth/user/signup", "", "",
		map[string]string{"email": bootstrapEmail, "password": bootstrapPassword}, &signup)
	require.NotEmpty(t, signup.Token, "signup JWT")

	// 2. Mint an API token (xc-token) using the JWT.
	var tok struct {
		Token string `json:"token"`
	}
	n.do(t, http.MethodPost, "/api/v1/tokens", "", signup.Token,
		map[string]string{"description": "integration"}, &tok)
	require.NotEmpty(t, tok.Token, "API token")
	n.Token = tok.Token

	// 3. Resolve the default workspace (using the API token from here on).
	var workspaces struct {
		List []struct {
			ID string `json:"id"`
		} `json:"list"`
	}
	n.do(t, http.MethodGet, "/api/v3/meta/workspaces", n.Token, "", nil, &workspaces)
	require.NotEmpty(t, workspaces.List, "at least one workspace")
	wsID := workspaces.List[0].ID

	// 4. Create an empty base for the test.
	var base struct {
		ID string `json:"id"`
	}
	n.do(t, http.MethodPost, "/api/v3/meta/workspaces/"+wsID+"/bases", n.Token, "",
		map[string]string{"title": "migrator_it"}, &base)
	require.NotEmpty(t, base.ID, "base id")
	n.BaseID = base.ID
}

// do issues a JSON request to the container. Exactly one of xcToken / xcAuth may
// be set for authenticated calls. A non-2xx response fails the test.
func (n *NocoDB) do(t *testing.T, method, path, xcToken, xcAuth string, body, out interface{}) {
	t.Helper()

	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(raw)
	}

	req, err := http.NewRequest(method, n.URL+path, reader)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if xcToken != "" {
		req.Header.Set("xc-token", xcToken)
	}
	if xcAuth != "" {
		req.Header.Set("xc-auth", xcAuth)
	}

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "%s %s", method, path)
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Less(t, resp.StatusCode, 300, "%s %s -> %d: %s", method, path, resp.StatusCode, raw)

	if out != nil {
		require.NoError(t, json.Unmarshal(raw, out), "decode %s %s: %s", method, path, raw)
	}
}

// requireDocker skips the test when Docker is not configured on the machine, so
// a `-tags=integration` run without Docker skips cleanly instead of failing. A
// configured-but-broken daemon still surfaces as a real container-start error.
func requireDocker(t *testing.T) {
	t.Helper()
	if os.Getenv("DOCKER_HOST") != "" {
		return
	}
	for _, sock := range []string{"/var/run/docker.sock", os.Getenv("HOME") + "/.docker/run/docker.sock"} {
		if _, err := os.Stat(sock); err == nil {
			return
		}
	}
	t.Skip("skipping integration test: Docker not available (no DOCKER_HOST and no docker socket)")
}
