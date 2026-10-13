package daemon

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type appAuth struct {
	appID string
	key   *rsa.PrivateKey

	mu     sync.Mutex
	tokens map[string]appToken
}

type appToken struct {
	token   string
	expires time.Time
}

func newAppAuth(appID, keyPath string) (*appAuth, error) {
	b, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(b)
	if block == nil {
		return nil, fmt.Errorf("%s: not a PEM file", keyPath)
	}
	var key *rsa.PrivateKey
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		key = k
	} else {
		k8, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", keyPath, err)
		}
		rk, ok := k8.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("%s: not an RSA key", keyPath)
		}
		key = rk
	}
	return &appAuth{appID: appID, key: key, tokens: map[string]appToken{}}, nil
}

func (a *appAuth) jwt() (string, error) {
	now := time.Now()
	enc := base64.RawURLEncoding
	header := enc.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims, _ := json.Marshal(map[string]any{
		"iat": now.Add(-time.Minute).Unix(),
		"exp": now.Add(9 * time.Minute).Unix(),
		"iss": a.appID,
	})
	unsigned := header + "." + enc.EncodeToString(claims)
	sum := sha256.Sum256([]byte(unsigned))
	sig, err := rsa.SignPKCS1v15(rand.Reader, a.key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return unsigned + "." + enc.EncodeToString(sig), nil
}

func (a *appAuth) Token(repo string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if t, ok := a.tokens[repo]; ok && time.Until(t.expires) > 5*time.Minute {
		return t.token, nil
	}
	jwt, err := a.jwt()
	if err != nil {
		return "", err
	}
	var inst struct {
		ID int64 `json:"id"`
	}
	if err := appCall("GET", "https://api.github.com/repos/"+repo+"/installation", jwt, &inst); err != nil {
		return "", fmt.Errorf("app not installed on %s? %w", repo, err)
	}
	var tok struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := appCall("POST", fmt.Sprintf("https://api.github.com/app/installations/%d/access_tokens", inst.ID), jwt, &tok); err != nil {
		return "", err
	}
	a.tokens[repo] = appToken{token: tok.Token, expires: tok.ExpiresAt}
	return tok.Token, nil
}

func appCall(method, url, jwt string, out any) error {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s: %s: %s", method, url, resp.Status, strings.TrimSpace(string(b)))
	}
	return json.Unmarshal(b, out)
}

func (d *Daemon) postEnv(repo string) ([]string, error) {
	if d.app == nil {
		return nil, nil
	}
	t, err := d.app.Token(repo)
	if err != nil {
		return nil, err
	}
	return []string{"GH_TOKEN=" + t}, nil
}
