package service

import (
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/Kcchouette/cardigann-go/definition"
	"gopkg.in/yaml.v3"

	"github.com/Kcchouette/gowlarr/internal/netutil"
	"github.com/Kcchouette/gowlarr/internal/store"
)

// rotationMu guards concurrent rewrites of the same local definition file.
var rotationMu sync.Map // defID -> *sync.Mutex

var defIDSlugRe = regexp.MustCompile(`^[a-zA-Z0-9-]+$`)

// hostCharsetRe allowlists the characters a rotated host may contain:
// url.Parse accepts quotes inside a host, which would corrupt the YAML.
var hostCharsetRe = regexp.MustCompile(`^[a-zA-Z0-9.\-\[\]]+$`)

// maybePersistRotation rewrites links[0] in the matching definitions-local
// YAML and upserts the DB cache when the observed effective origin differs
// from the configured one. It never returns an error: a failed rotation
// persist must not fail the search that discovered it.
func maybePersistRotation(st *store.Store, defID, configuredBase, effectiveBase string) {
	if defID == "" || configuredBase == "" || effectiveBase == "" {
		return
	}
	cfgOrigin, ok := canonicalOrigin(configuredBase)
	if !ok {
		return
	}
	effOrigin, ok := canonicalOrigin(effectiveBase)
	if !ok || effOrigin == cfgOrigin {
		return
	}

	effURL, err := url.Parse(effectiveBase)
	if err != nil || (effURL.Scheme != "http" && effURL.Scheme != "https") || effURL.Host == "" {
		slog.Warn("domain rotation persist skipped: invalid effective base", "indexer", defID, "effective", effectiveBase)
		return
	}
	if !hostCharsetRe.MatchString(effURL.Host) {
		slog.Warn("domain rotation persist skipped: host charset not allowed", "indexer", defID, "host", effURL.Host)
		return
	}
	if !defIDSlugRe.MatchString(defID) || filepath.Base(defID) != defID {
		slog.Warn("domain rotation persist skipped: def id not a plain slug", "indexer", defID)
		return
	}
	if err := netutil.ValidateURL(effectiveBase); err != nil {
		slog.Warn("domain rotation persist skipped: validation failed", "indexer", defID, "effective", effectiveBase, "err", err)
		return
	}

	muIface, _ := rotationMu.LoadOrStore(defID, &sync.Mutex{})
	mu := muIface.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()

	path, raw, ok := findLocalDefinition(defID)
	if !ok {
		// Upstream/synced definition: runtime rewrite only.
		return
	}
	newRaw, ok := rewriteLinksFirstEntry(raw, effOrigin, cfgOrigin)
	if !ok {
		return
	}
	if newRaw == raw {
		return
	}
	if err := writeFileAtomic(path, newRaw); err != nil {
		slog.Warn("domain rotation persist skipped: write failed", "indexer", defID, "path", path, "err", err)
		return
	}
	if err := st.SaveDefinition(defID, "local", "", newRaw); err != nil {
		slog.Warn("domain rotation: DB cache update failed", "indexer", defID, "err", err)
		return
	}
	slog.Info("domain rotation persisted", "indexer", defID, "from", cfgOrigin, "to", effOrigin, "path", path)
}

// canonicalOrigin returns scheme://host[:port] lowercased with
// scheme-default ports stripped, or ok=false when raw is unparseable.
func canonicalOrigin(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", false
	}
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Host)
	switch {
	case scheme == "http" && strings.HasSuffix(host, ":80"):
		host = strings.TrimSuffix(host, ":80")
	case scheme == "https" && strings.HasSuffix(host, ":443"):
		host = strings.TrimSuffix(host, ":443")
	}
	return scheme + "://" + host, true
}

// findLocalDefinition scans LocalDefsDir for a file whose parsed definition
// ID matches defID (the filename is not guaranteed to equal the id).
func findLocalDefinition(defID string) (string, string, bool) {
	dir := LocalDefsDir()
	if dir == "" {
		return "", "", false
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", "", false
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yml") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		def, err := definition.Parse(raw)
		if err != nil || def.ID != defID {
			continue
		}
		return path, string(raw), true
	}
	return "", "", false
}

// rewriteLinksFirstEntry locates the first scalar of the root-level links
// sequence via yaml.v3 (comments/formatting preserved: only that scalar's
// span is rewritten in the raw buffer) and replaces its origin with newOrigin.
//
// Guards (Warn + no write): flow-style collections (a trailing # comment
// would comment out the closing bracket), multi-line scalars, unexpected
// shapes. An existing line comment is preserved and the rotated-from note
// is appended with " | ".
func rewriteLinksFirstEntry(raw, newOrigin, oldOrigin string) (string, bool) {
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(raw), &root); err != nil {
		slog.Warn("domain rotation persist skipped: YAML parse failed", "err", err)
		return "", false
	}
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		slog.Warn("domain rotation persist skipped: unexpected YAML root")
		return "", false
	}
	mapping := root.Content[0]
	var linksSeq *yaml.Node
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == "links" && mapping.Content[i+1].Kind == yaml.SequenceNode {
			linksSeq = mapping.Content[i+1]
			break
		}
	}
	if linksSeq == nil || linksSeq.Style == yaml.FlowStyle || len(linksSeq.Content) == 0 {
		slog.Warn("domain rotation persist skipped: links sequence missing or flow-style")
		return "", false
	}
	scalar := linksSeq.Content[0]
	if scalar.Kind != yaml.ScalarNode || scalar.Style == yaml.FlowStyle || strings.Contains(scalar.Value, "\n") {
		slog.Warn("domain rotation persist skipped: links[0] scalar shape unsupported")
		return "", false
	}

	// Rewrite only the scalar token in the raw buffer: start at (Line,
	// Column), end at the quote-aware end of the token (EOL or " #").
	lines := strings.Split(raw, "\n")
	if scalar.Line < 1 || scalar.Line > len(lines) {
		slog.Warn("domain rotation persist skipped: scalar line out of range")
		return "", false
	}
	line := lines[scalar.Line-1]
	col := scalar.Column - 1
	if col < 0 || col >= len(line) {
		slog.Warn("domain rotation persist skipped: scalar column out of range")
		return "", false
	}
	tokenEnd := scalarTokenEnd(line, col)
	if tokenEnd <= col {
		slog.Warn("domain rotation persist skipped: could not bound scalar token")
		return "", false
	}

	oldToken := line[col:tokenEnd]
	unquoted := strings.Trim(oldToken, `"'`)
	pathSuffix := ""
	if u, err := url.Parse(unquoted); err == nil {
		pathSuffix = u.Path
		if pathSuffix == "" {
			pathSuffix = "/"
		}
	} else {
		pathSuffix = "/"
	}
	// Preserve quoting style of the original token.
	var newToken string
	switch {
	case strings.HasPrefix(oldToken, `"`):
		newToken = `"` + newOrigin + pathSuffix + `"`
	case strings.HasPrefix(oldToken, `'`):
		newToken = `'` + newOrigin + pathSuffix + `'`
	default:
		newToken = newOrigin + pathSuffix
	}

	rest := line[tokenEnd:]
	// Replace an existing rotated-from note, else append one (block style only).
	rotatedNote := " # rotated-from: " + oldOrigin
	if idx := strings.Index(rest, "rotated-from:"); idx >= 0 {
		// Keep any leading comment text before the note; drop through EOL note.
		nl := strings.IndexByte(rest[idx:], '\n')
		_ = nl // rest has no newline (we split on \n)
		before := rest[:idx]
		// Preserve trailing user comment before our note if present.
		rest = before + "rotated-from: " + oldOrigin
		// rest already ends the line; ensure single-space prefix when empty.
		if !strings.HasSuffix(before, " ") && before != "" {
			rest = before + " rotated-from: " + oldOrigin
		}
	} else if strings.TrimSpace(rest) == "" {
		rest = rotatedNote
	} else if strings.HasPrefix(strings.TrimSpace(rest), "#") {
		rest = strings.TrimRight(rest, " ") + " | rotated-from: " + oldOrigin
	} else {
		rest = rotatedNote + rest
	}

	lines[scalar.Line-1] = line[:col] + newToken + rest
	return strings.Join(lines, "\n"), true
}

// scalarTokenEnd returns the index (exclusive) in line where the scalar
// token starting at col ends: plain scalars end at whitespace before a
// comment or EOL; quoted scalars end after the matching closing quote.
func scalarTokenEnd(line string, col int) int {
	if col >= len(line) {
		return col
	}
	switch line[col] {
	case '"', '\'':
		q := line[col]
		for i := col + 1; i < len(line); i++ {
			if line[i] == '\\' && q == '"' {
				i++
				continue
			}
			if line[i] == q {
				return i + 1
			}
		}
		return len(line) // unterminated: treat to EOL
	default:
		for i := col; i < len(line); i++ {
			if line[i] == '#' && i > col && (line[i-1] == ' ' || line[i-1] == '\t') {
				return i
			}
		}
		return len(line)
	}
}

// writeFileAtomic writes data to path via a temp file in the same directory
// plus a rename.
func writeFileAtomic(path, data string) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".rotation-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.WriteString(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// rotationAware is implemented by providers that can report an origin
// rotation observed during the last search. ID is satisfied by
// search.Provider (the cardigann adapter delegates it).
type rotationAware interface {
	ID() string
	EffectiveBaseURL() string
	ConfiguredBaseURL() string
}

// persistIfRotated persists a rotation discovered by p; never fails.
func persistIfRotated(st *store.Store, p rotationAware) {
	if st == nil || p == nil {
		return
	}
	maybePersistRotation(st, p.ID(), p.ConfiguredBaseURL(), p.EffectiveBaseURL())
}
