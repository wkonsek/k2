package kube

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"k2/internal/shell"

	"gopkg.in/yaml.v3"
)

type Token struct {
	secret string
}

func (t Token) String() string {
	if t.secret == "" {
		return "[no token]"
	}
	return "[redacted]"
}

func (t Token) GoString() string { return t.String() }

func (t Token) Empty() bool { return t.secret == "" }

func (t Token) OpenShift() bool { return strings.HasPrefix(t.secret, "sha256~") }

func (t Token) Fingerprint() string {
	if t.secret == "" {
		return ""
	}
	sum := sha256.Sum256([]byte("k2 token fingerprint\x00" + t.secret))
	return hex.EncodeToString(sum[:8])
}

func (t Token) OAuthName() string {
	if !t.OpenShift() {
		return ""
	}
	return OAuthTokenName(t.secret)
}

func OAuthTokenName(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimPrefix(token, "sha256~")))
	return "sha256~" + base64.RawURLEncoding.EncodeToString(sum[:])
}

type TokenLife struct {
	Known bool

	Expired bool

	// Invalid is a token the API rejected and that has no expiry of its own.
	Invalid bool

	Start, End time.Time

	Idle time.Duration
}

type kubeconfig struct {
	CurrentContext string `yaml:"current-context"`
	Contexts       []struct {
		Name    string `yaml:"name"`
		Context struct {
			User string `yaml:"user"`
		} `yaml:"context"`
	} `yaml:"contexts"`
	Users []struct {
		Name string `yaml:"name"`
		User struct {
			Token     string `yaml:"token"`
			TokenFile string `yaml:"tokenFile"`
		} `yaml:"user"`
	} `yaml:"users"`
}

func KubeconfigPaths(kubeconfigPath string) []string {
	if kubeconfigPath != "" {
		return []string{kubeconfigPath}
	}
	if env := os.Getenv("KUBECONFIG"); env != "" {
		var out []string
		for _, p := range filepath.SplitList(env) {
			if p != "" {
				out = append(out, p)
			}
		}
		return out
	}
	if home, err := os.UserHomeDir(); err == nil {
		return []string{filepath.Join(home, ".kube", "config")}
	}
	return nil
}

func TokenOf(kubeconfigPath, context string) Token {
	var files []kubeconfig
	var dirs []string
	for _, p := range KubeconfigPaths(kubeconfigPath) {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var kc kubeconfig
		if yaml.Unmarshal(data, &kc) != nil {
			continue
		}
		files, dirs = append(files, kc), append(dirs, filepath.Dir(p))
	}
	if context == "" {
		for _, f := range files {
			if f.CurrentContext != "" {
				context = f.CurrentContext
				break
			}
		}
	}
	user, found := "", false
	for _, f := range files {
		for _, c := range f.Contexts {
			if c.Name == context && !found {
				user, found = c.Context.User, true
			}
		}
	}
	if !found || user == "" {
		return Token{}
	}
	for i, f := range files {
		for _, u := range f.Users {
			if u.Name != user {
				continue
			}
			tok := u.User.Token
			if tok == "" && u.User.TokenFile != "" {
				path := u.User.TokenFile
				if !filepath.IsAbs(path) {
					path = filepath.Join(dirs[i], path)
				}
				if data, err := os.ReadFile(path); err == nil {
					tok = string(data)
				}
			}
			return Token{secret: strings.TrimSpace(tok)}
		}
	}
	return Token{}
}

// SetContextToken replaces the current context user's token. A token: value
// is rewritten in place. A tokenFile is rewritten when that is the only
// credential. The rest of the kubeconfig is left byte for byte.
func SetContextToken(kubeconfigPath, context, token string) (string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", errors.New("empty token")
	}
	if strings.ContainsAny(token, "\r\n") {
		return "", errors.New("token must be a single line")
	}
	path, user, err := userFile(kubeconfigPath, context)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return "", err
	}
	userNode := userMapping(docRoot(&doc), user)
	if userNode == nil {
		return "", fmt.Errorf("user %q is not in the kubeconfig", user)
	}
	tokNode := mappingGet(userNode, "token")
	fileNode := mappingGet(userNode, "tokenFile")
	liveFile := tokNode == nil || strings.TrimSpace(tokNode.Value) == ""
	if liveFile && fileNode != nil && strings.TrimSpace(fileNode.Value) != "" {
		file := fileNode.Value
		if !filepath.IsAbs(file) {
			file = filepath.Join(filepath.Dir(path), file)
		}
		if err := writeSecret(file, token+"\n"); err != nil {
			return "", err
		}
		return file, nil
	}
	if tokNode == nil || tokNode.Kind != yaml.ScalarNode {
		return "", fmt.Errorf("user %q has no token to replace", user)
	}
	if tokNode.Style == yaml.LiteralStyle || tokNode.Style == yaml.FoldedStyle {
		return "", fmt.Errorf("user %q token is multi-line", user)
	}
	if tokNode.Line < 1 {
		return "", errors.New("token line not found")
	}
	nl := "\n"
	raw := string(data)
	if strings.Contains(raw, "\r\n") {
		nl = "\r\n"
		raw = strings.ReplaceAll(raw, "\r\n", "\n")
	}
	lines := strings.Split(raw, "\n")
	idx := tokNode.Line - 1
	if idx >= len(lines) {
		return "", errors.New("token line not found")
	}
	lines[idx] = rewriteTokenLine(lines[idx], token)
	out := strings.Join(lines, nl)
	if strings.HasSuffix(string(data), "\n") || strings.HasSuffix(string(data), "\r\n") {
		if !strings.HasSuffix(out, "\n") {
			out += nl
		}
	}
	if err := writeSecret(path, out); err != nil {
		return "", err
	}
	return path, nil
}

func userFile(kubeconfigPath, context string) (path, user string, err error) {
	var files []kubeconfig
	var paths []string
	for _, p := range KubeconfigPaths(kubeconfigPath) {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var kc kubeconfig
		if yaml.Unmarshal(data, &kc) != nil {
			continue
		}
		files, paths = append(files, kc), append(paths, p)
	}
	if len(files) == 0 {
		return "", "", errors.New("kubeconfig not found")
	}
	if context == "" {
		for _, f := range files {
			if f.CurrentContext != "" {
				context = f.CurrentContext
				break
			}
		}
	}
	if context == "" {
		return "", "", errors.New("no context selected")
	}
	found := false
	for _, f := range files {
		for _, c := range f.Contexts {
			if c.Name == context && !found {
				user, found = c.Context.User, true
			}
		}
	}
	if !found {
		return "", "", fmt.Errorf("context %q is not in the kubeconfig", context)
	}
	if user == "" {
		return "", "", fmt.Errorf("context %q has no user", context)
	}
	for i, f := range files {
		for _, u := range f.Users {
			if u.Name == user {
				return paths[i], user, nil
			}
		}
	}
	return "", "", fmt.Errorf("user %q is not in the kubeconfig", user)
}

func docRoot(doc *yaml.Node) *yaml.Node {
	if doc != nil && doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		return doc.Content[0]
	}
	return doc
}

func mappingGet(n *yaml.Node, key string) *yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}

func userMapping(root *yaml.Node, user string) *yaml.Node {
	users := mappingGet(root, "users")
	if users == nil || users.Kind != yaml.SequenceNode {
		return nil
	}
	for _, item := range users.Content {
		name := mappingGet(item, "name")
		if name != nil && name.Value == user {
			return mappingGet(item, "user")
		}
	}
	return nil
}

func rewriteTokenLine(line, token string) string {
	i := strings.Index(line, "token:")
	if i < 0 {
		return line
	}
	prefix := line[:i+len("token:")]
	rest := line[i+len("token:"):]
	comment := ""
	if h := strings.Index(rest, " #"); h >= 0 {
		comment = rest[h:]
	}
	return prefix + " " + strconv.Quote(token) + comment
}

func writeSecret(path, data string) error {
	mode := os.FileMode(0o600)
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".k2-token-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	ok := false
	defer func() {
		if !ok {
			os.Remove(tmp)
		}
	}()
	if _, err := f.WriteString(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	ok = true
	return nil
}

func JWTLife(t Token) TokenLife {
	parts := strings.Split(t.secret, ".")
	if len(parts) != 3 {
		return TokenLife{}
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return TokenLife{}
	}
	var claims struct {
		Exp, Iat, Nbf float64
	}
	if json.Unmarshal(raw, &claims) != nil || claims.Exp <= 0 {
		return TokenLife{}
	}
	life := TokenLife{Known: true, End: unix(claims.Exp)}
	switch {
	case claims.Iat > 0 && claims.Iat < claims.Exp:
		life.Start = unix(claims.Iat)
	case claims.Nbf > 0 && claims.Nbf < claims.Exp:
		life.Start = unix(claims.Nbf)
	}
	return life
}

func unix(sec float64) time.Time {
	return time.Unix(int64(sec), int64((sec-float64(int64(sec)))*1e9))
}

const oauthResource = "useroauthaccesstokens.oauth.openshift.io"

func (c Client) OAuthLife(kubeconfigPath, context string, t Token) TokenLife {
	name := t.OAuthName()
	if name == "" {
		return TokenLife{}
	}
	base := append(targetArgs(kubeconfigPath, context), "get", oauthResource)
	tail := []string{"-o", "json", "--request-timeout=" + InfoTimeout.String()}
	get := c.Run.Run(shell.Request{Name: "kubectl", Args: append(append(append([]string{}, base...), name), tail...)})
	if get.Err == nil {
		if life, ok := parseOAuthToken(get.Stdout); ok {
			return life
		}
	} else if tokenRejected(commandText(get)) {
		return TokenLife{Expired: true}
	}
	list := c.Run.Run(shell.Request{Name: "kubectl", Args: append(append([]string{}, base...), tail...)})
	if list.Err != nil {
		if tokenRejected(commandText(list)) {
			return TokenLife{Expired: true}
		}
		return TokenLife{}
	}
	var items struct {
		Items []json.RawMessage `json:"items"`
	}
	if json.Unmarshal(list.Stdout, &items) != nil {
		return TokenLife{}
	}
	for _, item := range items.Items {
		var meta struct {
			Metadata struct{ Name string } `json:"metadata"`
		}
		if json.Unmarshal(item, &meta) == nil && meta.Metadata.Name == name {
			life, _ := parseOAuthToken(item)
			return life
		}
	}
	return TokenLife{}
}

func commandText(res shell.Result) string {
	return string(res.Stdout) + "\n" + string(res.Stderr)
}

func tokenRejected(text string) bool {
	s := strings.ToLower(text)
	switch {
	case strings.Contains(s, "unauthorized"):
		return true
	case strings.Contains(s, "must be logged in"):
		return true
	case strings.Contains(s, "invalid bearer"):
		return true
	case strings.Contains(s, "token has expired"), strings.Contains(s, "token expired"):
		return true
	case strings.Contains(s, "provide credentials"):
		return true
	default:
		return false
	}
}

func parseOAuthToken(data []byte) (TokenLife, bool) {
	var obj struct {
		Metadata struct {
			CreationTimestamp string `json:"creationTimestamp"`
		} `json:"metadata"`
		ExpiresIn                int64 `json:"expiresIn"`
		InactivityTimeoutSeconds int64 `json:"inactivityTimeoutSeconds"`
	}
	if json.Unmarshal(data, &obj) != nil {
		return TokenLife{}, false
	}
	created, err := time.Parse(time.RFC3339, obj.Metadata.CreationTimestamp)
	if err != nil {
		return TokenLife{}, false
	}
	if obj.ExpiresIn <= 0 {
		return TokenLife{}, true
	}
	return TokenLife{
		Known: true,
		Start: created,
		End:   created.Add(time.Duration(obj.ExpiresIn) * time.Second),
		Idle:  time.Duration(obj.InactivityTimeoutSeconds) * time.Second,
	}, true
}

func (c Client) Life(kubeconfigPath, context string, t Token) TokenLife {
	switch {
	case t.Empty():
		return TokenLife{}
	case t.OpenShift():
		if life := c.OAuthLife(kubeconfigPath, context, t); life.Known || life.Expired {
			return life
		}
	default:
		if life := JWTLife(t); life.Known {
			return life
		}
	}
	return c.tokenAccepted(kubeconfigPath, context)
}

// tokenAccepted asks the API once. A token kubectl accepts, but whose
// lifetime we cannot read, stays unknown (no sidebar row). A rejected
// token is invalid.
func (c Client) tokenAccepted(kubeconfigPath, context string) TokenLife {
	args := append(targetArgs(kubeconfigPath, context), "get", "--raw", "/api", "--request-timeout="+InfoTimeout.String())
	res := c.Run.Run(shell.Request{Name: "kubectl", Args: args})
	if res.Err == nil {
		return TokenLife{}
	}
	if tokenRejected(commandText(res)) {
		return TokenLife{Invalid: true}
	}
	return TokenLife{}
}
